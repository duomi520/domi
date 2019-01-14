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
var BytesPoolLenght = 32768 //8192,16384,32768,65536,131072
//BytesPoolLenght32 长度
var BytesPoolLenght32 uint32 = 32768

//SessionInternalTimeout 内部超时
var SessionInternalTimeout = DefaultDeadlineDuration

//池
var bytesPool sync.Pool
var rejectPool sync.Pool

var nilRejects = make([]func(error), BytesPoolLenght/8)

var busyLimit uint32

func init() {
	busyLimit = uint32(runtime.NumCPU()) * 2
	for i := 0; i < len(nilRejects); i++ {
		nilRejects[i] = nil
	}
	bytesPool.New = func() interface{} {
		b := make([]byte, BytesPoolLenght)
		return b[:]
	}
	rejectPool.New = func() interface{} {
		b := make([]func(error), BytesPoolLenght/8)
		return b[:]
	}
}

//定义错误
var (
	ErrFailureIORead        = errors.New("transport.SessionTCP.ioRead|rBuf缓存溢出。")
	ErrConnClose            = errors.New("ErrConnClose|SessionTCP已关闭。")
	ErrcircuitBreakerIsPass = errors.New("ErrConnClose|熔断器开启状态,服务异常。")
	ErrFailureBusy          = errors.New("transport.SessionTCP.WriteFrameDataToCache|写入缓存越界超时。")
	ErrInternalTimeout      = errors.New("transport.SessionTCP.WorkFunc|内部调度超时。")
	ErrAvailableCursor      = errors.New("transport.SessionTCP.WorkFunc|availableCursor异常。")
)

//SessionTCP 会话
type SessionTCP struct {
	Conn           *net.TCPConn
	dispatcher     *util.Dispatcher
	handler        *Handler
	circuitBreaker *util.CircuitBreaker //熔断器

	state uint32

	rBuf []byte //IO读缓存
	w    int    //rBuf 读位置序号
	r    int    //rBuf 写位置序号

	wSlot unsafe.Pointer

	closeOnce sync.Once
	sync.WaitGroup
}

//NewSessionTCP 新建
func NewSessionTCP(conn *net.TCPConn, h *Handler, cbc *util.CircuitBreakerConfigure) *SessionTCP {
	s := &SessionTCP{
		Conn:    conn,
		handler: h,
		state:   util.StateWork,
		rBuf:    bytesPool.Get().([]byte),
		r:       0,
		w:       0,
	}
	ws := newSlot(s)
	s.wSlot = unsafe.Pointer(&ws)
	//配置熔断器
	s.circuitBreaker = util.NewCircuitBreaker(cbc)
	s.circuitBreaker.HalfOpenFunc = s.sessionTCPPing
	h.HandleFunc(FrameTypePong, s.sessionTCPPong)
	return s
}
func (s *SessionTCP) sessionTCPPing() error {
	return s.WriteFrameDataPromptly(FramePing)
}
func (s *SessionTCP) sessionTCPPong(session Session) error {
	s.circuitBreaker.SetCircuitBreakerPass()
	return nil
}

//Close 关闭
func (s *SessionTCP) Close() {
	s.SetState(util.StateDie)
	s.closeOnce.Do(func() {
		s.Wait()
		s.Conn.Close()
		time.AfterFunc(5*time.Second, func() {
			bytesPool.Put(s.rBuf)
			(*slot)(s.wSlot).release()
			s.circuitBreaker = nil
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
func (s *SessionTCP) ioRead() (int, error) {
	if s.r > 0 {
		if s.r < s.w {
			copy(s.rBuf, s.rBuf[s.r:s.w])
			s.w -= s.r
		} else {
			s.w = 0
		}
	}
	if s.w >= len(s.rBuf) {
		return 0, ErrFailureIORead
	}
	if err := s.Conn.SetReadDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
		return 0, err
	}
	n, err := s.Conn.Read(s.rBuf[s.w:])
	s.r = 0
	s.w += n
	return s.w - s.r, err
}

//WriteFrameDataPromptly 同步发送数据 without delay
func (s *SessionTCP) WriteFrameDataPromptly(f FrameSlice) error {
	if atomic.LoadUint32(&s.state) != util.StateWork {
		return ErrConnClose
	}
	var err error
	if f.GetFrameLength() >= FrameHeadLength {
		if err = s.Conn.SetWriteDeadline(time.Now().Add(SessionInternalTimeout)); err != nil {
			return err
		}
		_, err = s.Conn.Write(f.base)
	}
	return err
}

type simpleSlot struct {
	session *SessionTCP
	fs      FrameSlice
	errFunc func(error)
}

func (ss simpleSlot) WorkFunc() {
	if err := ss.session.WriteFrameDataPromptly(ss.fs); err != nil {
		if ss.session.circuitBreaker != nil {
			ss.session.circuitBreaker.ErrorRecord()
		}
		ss.errFunc(err)
	}
	ss.session.Done()
}

//WriteFrameDataToCache 写入发送缓存,由线程池异步发送
func (s *SessionTCP) WriteFrameDataToCache(f FrameSlice, errFunc func(error)) error {
	//会话已关闭
	if atomic.LoadUint32(&s.state) != util.StateWork {
		return ErrConnClose
	}
	//熔断器开启状态
	if s.circuitBreaker != nil && !s.circuitBreaker.IsPass() {
		return ErrcircuitBreakerIsPass
	}
	//超过缓存，直接发送
	if f.GetFrameLength() >= BytesPoolLenght {
		s.Add(1)
		ss := simpleSlot{
			session: s,
			fs:      f,
			errFunc: errFunc,
		}
		s.dispatcher.JobQueue <- ss
		return nil
	}
	length := uint32(f.GetFrameLength())
	var myslot *slot
	var end, start, busy uint32
loop:
	myslot = (*slot)(atomic.LoadPointer(&s.wSlot))
	atomic.AddInt32(&myslot.writeLock, 1)
	end = atomic.AddUint32(&myslot.allotCursor, length)
	start = end - length
	if end > BytesPoolLenght32 {
		//申请的地址超出边界
		if start > BytesPoolLenght32 {
			if busy > busyLimit {
				atomic.AddInt32(&myslot.writeLock, -1)
				if s.circuitBreaker != nil {
					s.circuitBreaker.ErrorRecord()
				}
				return ErrFailureBusy
			}
			busy++
			atomic.AddInt32(&myslot.writeLock, -1)
			runtime.Gosched()
			goto loop
		}
		//刚好越界触发
		ns := newSlot(s)
		if !atomic.CompareAndSwapPointer(&s.wSlot, unsafe.Pointer(myslot), unsafe.Pointer(&ns)) {
			ns.releaseNoClear()
		}
		busy = 0
		atomic.AddInt32(&myslot.writeLock, -1)
		goto loop
	}
	f.WriteToBytes(myslot.buf[start:end])
	atomic.AddUint32(&myslot.availableCursor, length)
	rc := atomic.AddUint32(&myslot.rejectCursor, 1)
	myslot.rejects[rc] = errFunc
	atomic.AddInt32(&myslot.writeLock, -1)
	//新的slot触发
	if start == 0 {
		s.Add(1)
		myslot.timestamp = time.Now()
		s.dispatcher.JobQueue <- myslot
	}
	return nil
}

//SetState 设置状态
func (s *SessionTCP) SetState(u uint32) {
	atomic.StoreUint32(&s.state, u)
}

//readUint32 读uint32
func (s *SessionTCP) readUint32() (uint32, error) {
	b := make([]byte, 4)
	if err := s.Conn.SetReadDeadline(time.Now().Add(SessionInternalTimeout)); err != nil {
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
	session   *SessionTCP
	buf       []byte //IO写缓存
	rejects   []func(error)
	timestamp time.Time

	_padding0       [8]uint64
	allotCursor     uint32 //申请位置
	_padding1       [8]uint64
	availableCursor uint32 //已提交位置
	_padding2       [8]uint64
	writeLock       int32 //写锁定数
	_padding3       [8]uint64
	rejectCursor    uint32 //申请位置
}

func newSlot(s *SessionTCP) slot {
	return slot{
		session: s,
		buf:     bytesPool.Get().([]byte),
		rejects: rejectPool.Get().([]func(error)),
	}
}

//release 释放资源
func (ws *slot) release() {
	bytesPool.Put(ws.buf)
	rejectPool.Put(ws.rejects)
	ws.session = nil
}
func (ws *slot) releaseNoClear() {
	bytesPool.Put(ws.buf)
	rejectPool.Put(ws.rejects)
	ws.session = nil
}

//WorkFunc 发送
func (ws *slot) WorkFunc() {
	ns := newSlot(ws.session)
	if !atomic.CompareAndSwapPointer(&ws.session.wSlot, unsafe.Pointer(ws), unsafe.Pointer(&ns)) {
		ns.releaseNoClear()
	}
	//降低同步取得wSlot时，writeLock失效的频率。
	time.Sleep(time.Microsecond)
	//自旋等待其它协程提交。
	for atomic.LoadInt32(&ws.writeLock) != 0 {
		runtime.Gosched()
	}
	//返回内部调度超时错误
	if time.Now().Sub(ws.timestamp) > SessionInternalTimeout {
		ws.rejectsRange(ErrInternalTimeout)
	}
	//IO发送
	if ws.availableCursor >= uint32(FrameHeadLength) {
		if err := ws.session.Conn.SetWriteDeadline(time.Now().Add(SessionInternalTimeout)); err != nil {
			ws.rejectsRange(err)
		}
		if _, err := ws.session.Conn.Write(ws.buf[:ws.availableCursor]); err != nil {
			ws.rejectsRange(err)
		}
	} else {
		ws.rejectsRange(ErrAvailableCursor)
	}
	ws.session.Done()
	copy(ws.rejects[:ws.rejectCursor], nilRejects[:ws.rejectCursor])
	ws.release()
}

//rejectsRange 遍历处理错误
func (ws *slot) rejectsRange(err error) {
	if ws.session.circuitBreaker != nil {
		ws.session.circuitBreaker.ErrorRecord()
	}
	for i := 0; i < int(ws.rejectCursor); i++ {
		if ws.rejects[i] != nil {
			ws.rejects[i](err)
		}
	}
}
