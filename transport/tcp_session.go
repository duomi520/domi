package transport

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/duomi520/domi/util"
)

//定义错误
var (
	ErrFailureIORead = errors.New("transport.SessionTCP.ioRead|rBuf缓存溢出。")
)

//SessionTCP 会话
type SessionTCP struct {
	Conn       *net.TCPConn
	dispatcher *util.Dispatcher
	rBuf       []byte
	w          int //rBuf 读位置序号
	r          int //rBuf 写位置序号
	wSlot      *slot
	closeOnce  sync.Once
	sync.WaitGroup
}

//NewSessionTCP 新建
func NewSessionTCP(conn *net.TCPConn) *SessionTCP {
	s := &SessionTCP{
		Conn: conn,
		rBuf: util.BytesPoolGet(),
		r:    0,
		w:    0,
	}
	s.wSlot = newSlot(s)
	return s
}

//Close 关闭
func (s *SessionTCP) Close() {
	s.closeOnce.Do(func() {
		util.BytesPoolPut(s.rBuf)
		if s.wSlot != nil {
			util.BytesPoolPut(s.wSlot.buf)
			s.wSlot = nil
		}
		s.Conn.Close()
		s = nil
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
	if err := s.Conn.SetReadDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
		return err
	}
	n, err := s.Conn.Read(s.rBuf[s.w:])
	s.r = 0
	s.w += n
	return err
}

//WriteFrameDataPromptly 立即发送数据 without delay
func (s *SessionTCP) WriteFrameDataPromptly(f FrameSlice) error {
	var err error
	if f.GetFrameLength() >= FrameHeadLength {
		if err = s.Conn.SetWriteDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
			return err
		}
		_, err = s.Conn.Write(f.base)
	}
	return err
}

//WriteFrameDataToCache 写入发送缓存
func (s *SessionTCP) WriteFrameDataToCache(f FrameSlice) error {
loop:
	myslot := (*slot)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.wSlot))))
	atomic.AddInt32(&myslot.count, 1)
	length := int32(f.GetFrameLength())
	end := atomic.AddInt32(&myslot.allotCursor, length)
	start := end - length
	if end >= int32(util.BytesPoolLenght) {
		//申请的地址超出边界
		if start >= int32(util.BytesPoolLenght) {
			atomic.AddInt32(&myslot.count, -1)
			goto loop
		}
		//刚好越界触发
		ns := newSlot(s)
		atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&s.wSlot)), unsafe.Pointer(ns))
		atomic.AddInt32(&myslot.count, -1)
		goto loop
	}
	f.WriteToBytes(myslot.buf[start:end])
	atomic.AddInt32(&myslot.availableCursor, length)
	atomic.AddInt32(&myslot.count, -1)
	//发出越界触发
	if start == 0 {
		s.Add(1)
		s.dispatcher.JobQueue <- myslot
	}
	return nil
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
	allotCursor     int32 //申请位置
	availableCursor int32 //已提交位置
	count           int32 //占用数
}

func newSlot(s *SessionTCP) *slot {
	return &slot{
		session:         s,
		buf:             util.BytesPoolGet(),
		allotCursor:     0,
		availableCursor: 0,
		count:           0,
	}
}

//WorkFunc 发送
func (ws *slot) WorkFunc() {
	ns := newSlot(ws.session)
	atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&(ws.session.wSlot))), unsafe.Pointer(ns))
	time.Sleep(time.Microsecond)
	//自旋等待其它协程提交
	for atomic.LoadInt32(&ws.count) != 0 {
		time.Sleep(time.Microsecond)
	}
	if ws.availableCursor >= int32(FrameHeadLength) {
		if err := ws.session.Conn.SetWriteDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
			goto end
		}
		if _, err := ws.session.Conn.Write(ws.buf[:ws.availableCursor]); err != nil {
			goto end
		}
	}
end:
	util.BytesPoolPut(ws.buf)
	ws.session.Done()
	ws = nil
}
