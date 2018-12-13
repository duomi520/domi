package transport

import (
	"errors"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/duomi520/domi/util"
)

//BytesPoolLenght 长度
const BytesPoolLenght int = 32768 //8192,16384,32768,65536,131072

//bytesPool bytesPool 池
var bytesPool sync.Pool

func init() {
	bytesPool.New = func() interface{} {
		b := make([]byte, BytesPoolLenght)
		return b[:]
	}
}

//BytesPoolGet 取一个
func BytesPoolGet() []byte {
	b := bytesPool.Get().([]byte)
	return b[:]
}

//BytesPoolPut 还一个
func BytesPoolPut(b []byte) {
	bytesPool.Put(b)
}

//定义错误
var (
	ErrFailureIORead = errors.New("transport.SessionTCP.ioRead|rBuf缓存溢出。")
	ErrConnClose     = errors.New("ErrConnClose|SessionTCP已关闭。")
	ErrFailureToken  = errors.New("transport.SessionTCP.WriteFrameDataToCache|写入太频繁，阻碍IO发送。")
	ErrFailureBusy   = errors.New("transport.SessionTCP.WriteFrameDataToCache|写入缓存越界超时。")
)

//SessionTCP 会话
type SessionTCP struct {
	Conn       *net.TCPConn
	dispatcher *util.Dispatcher
	state      uint32
	rBuf       []byte
	w          int //rBuf 读位置序号
	r          int //rBuf 写位置序号
	wSlot      unsafe.Pointer
	_padding0  [8]uint64
	lock       int32 //写锁定数
	_padding1  [8]uint64
	token      int64 //限流令牌
	closeOnce  sync.Once
	logger     *util.Logger
	sync.WaitGroup
}

//NewSessionTCP 新建
func NewSessionTCP(conn *net.TCPConn, log *util.Logger) *SessionTCP {
	s := &SessionTCP{
		Conn:   conn,
		state:  util.StateWork,
		rBuf:   BytesPoolGet(),
		r:      0,
		w:      0,
		logger: log,
	}
	ws := newSlot(s)
	s.wSlot = unsafe.Pointer(&ws)
	busyLimit = uint32(runtime.NumCPU()) * 2
	return s
}

//Close 关闭
func (s *SessionTCP) Close() {
	s.closeOnce.Do(func() {
		s.setState(util.StateDie)
		s.Wait()
		s.Conn.Close()
		time.AfterFunc(5*time.Second, func() {
			BytesPoolPut(s.rBuf)
			(*slot)(s.wSlot).release()
		})
	})
}

//GetFrameSlice 取得当前帧,线程不安全,必要时先拷贝。
func (s *SessionTCP) GetFrameSlice() FrameSlice {
	if s.w < s.r+FrameHeadLength {
		return FrameNil
	}
	length := int(util.BytesToUint32(s.rBuf[s.r : s.r+4]))
	if s.r+length <= s.w {
		return DecodeByBytes(s.rBuf[s.r : s.r+length])
	}
	return FrameNil
}

//getFrameType 取得当前帧类型
func (s *SessionTCP) getFrameType() uint16 {
	if s.w < s.r+FrameHeadLength {
		return FrameTypeNil
	}
	length := int(util.BytesToUint32(s.rBuf[s.r : s.r+4]))
	if s.r+length > s.w {
		return FrameTypeNil
	}
	return util.BytesToUint16(s.rBuf[s.r+6 : s.r+8])
}

//ioRead 读数据到rBuf，注意：每次ioRead,rBuf中的数据将被写入新数据。
//注意线程不安全
func (s *SessionTCP) ioRead() error {
	if s.r > 0 {
		if s.r < s.w {
			copy(s.rBuf, s.rBuf[s.r:s.w])
			s.w -= s.r
		} else {
			s.w = 0
		}
	}
	if s.w >= len(s.rBuf) {
		return ErrFailureIORead
	}
	//	if err := s.Conn.SetReadDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
	//		return err
	//	}
	n, err := s.Conn.Read(s.rBuf[s.w:])
	s.r = 0
	s.w += n
	return err
}

//WriteFrameDataPromptly 立即发送数据 without delay
func (s *SessionTCP) WriteFrameDataPromptly(f FrameSlice) error {
	if !s.hasWork() {
		return ErrConnClose
	}
	var err error
	if f.GetFrameLength() >= FrameHeadLength {
		if err = s.Conn.SetWriteDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
			return err
		}
		_, err = s.Conn.Write(f.base)
	}
	return err
}

var busyLimit uint32

//WriteFrameDataToCache 写入发送缓存
func (s *SessionTCP) WriteFrameDataToCache(f FrameSlice) error {
	if !s.hasWork() {
		return ErrConnClose
	}
	if f.GetFrameLength() >= BytesPoolLenght {
		return s.WriteFrameDataPromptly(f)
	}
	if atomic.LoadInt64(&s.token) > 1000 {
		return ErrFailureToken
	}
	length := uint32(f.GetFrameLength())
	var myslot *slot
	var end, start, busy uint32
	bytesPoolLenght32 := uint32(BytesPoolLenght)
	atomic.AddInt32(&s.lock, 1)
loop:
	myslot = (*slot)(atomic.LoadPointer(&s.wSlot))
	end = atomic.AddUint32(&myslot.allotCursor, length)
	start = end - length
	if end > bytesPoolLenght32 {
		//申请的地址超出边界
		if start > bytesPoolLenght32 {
			if busy > busyLimit {
				atomic.AddInt32(&s.lock, -1)
				return ErrFailureBusy
			}
			busy++
			runtime.Gosched()
			goto loop
		}
		//刚好越界触发
		ns := newSlot(s)
		if !atomic.CompareAndSwapPointer(&s.wSlot, unsafe.Pointer(myslot), unsafe.Pointer(&ns)) {
			ns.release()
		}
		busy = 0
		goto loop
	}
	f.WriteToBytes(myslot.buf[start:end])
	atomic.AddUint32(&myslot.availableCursor, length)
	atomic.AddInt32(&s.lock, -1)
	//新的slot触发
	if start == 0 {
		s.Add(1)
		s.dispatcher.JobQueue <- myslot
	}
	return nil
}

//hasWork 是否工作
func (s *SessionTCP) hasWork() bool {
	return atomic.LoadUint32(&s.state) == util.StateWork
}

//setState 设置状态
func (s *SessionTCP) setState(u uint32) {
	atomic.StoreUint32(&s.state, u)
}

//readUint32 读uint32
func (s *SessionTCP) readUint32() (uint32, error) {
	b := make([]byte, 4)
	if err := s.Conn.SetWriteDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
		return 0, err
	}
	if _, err := s.Conn.Read(b); err != nil {
		return 0, err
	}
	i := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
	return i, nil
}

//slot
type slot struct {
	session         *SessionTCP
	buf             []byte
	_padding0       [8]uint64
	allotCursor     uint32 //申请位置
	_padding1       [8]uint64
	availableCursor uint32 //已提交位置
}

func newSlot(s *SessionTCP) slot {
	return slot{
		session: s,
		buf:     BytesPoolGet(),
	}
}

//WorkFunc 发送
func (ws *slot) release() {
	BytesPoolPut(ws.buf)
	ws.session = nil
}

//WorkFunc 发送
func (ws *slot) WorkFunc() {
	ns := newSlot(ws.session)
	if !atomic.CompareAndSwapPointer(&ws.session.wSlot, unsafe.Pointer(ws), unsafe.Pointer(&ns)) {
		ns.release()
	}
	start := time.Now().Unix()
	//自旋等待其它协程提交。
	for atomic.LoadInt32(&ws.session.lock) != 0 {
		atomic.StoreInt64(&ws.session.token, time.Now().Unix()-start)
		runtime.Gosched()
	}
	atomic.StoreInt64(&ws.session.token, 0)
	if ws.availableCursor >= uint32(FrameHeadLength) {
		if err := ws.session.Conn.SetWriteDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
			ws.session.logger.Error("WorkFunc|超时:", err.Error())
		}
		if _, err := ws.session.Conn.Write(ws.buf[:ws.availableCursor]); err != nil {
			ws.session.logger.Error("WorkFunc|错误:", err.Error())
		}
	} else {
		//ws.session.logger.Error("WorkFunc|:", ws.availableCursor)
	}
	ws.session.Done()
	ws.release()
}
