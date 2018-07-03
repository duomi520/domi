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

//定义状态
const (
	StateNil int64 = iota
	StateDie
	StateWork
	StatePause
	StateStop
)

//定义错误
var (
	ErrFailureIORead = errors.New("transport.SessionTCP.ioRead|rBuf缓存溢出。")
)

//SessionTCP 会话
type SessionTCP struct {
	state      int64
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

//SetState 设
func (s *SessionTCP) SetState(st int64) {
	atomic.StoreInt64(&s.state, st)
	switch st {
	case StateStop:
		time.AfterFunc(DefaultWaitCloseDuration, s.Close)
	}
}

//GetState 读
func (s *SessionTCP) GetState() int64 {
	return atomic.LoadInt64(&s.state)
}

//SyncState 同步
func (s *SessionTCP) SyncState(own, dst int64) error {
	buf := make([]byte, 8)
	util.CopyInt64(buf, dst)
	f := NewFrameSlice(FrameTypeState, buf, nil)
	var err error
	if err = s.WriteFrameDataPromptly(f); err != nil {
		s.SetState(own)
	}
	return err
}

//Close 关闭
func (s *SessionTCP) Close() {
	s.closeOnce.Do(func() {
		s.Conn.Close()
		util.BytesPoolPut(s.rBuf)
		if s.wSlot != nil {
			util.BytesPoolPut(s.wSlot.buf)
		}
	})
}

//GetFrameSlice 取得当前帧,线程不安全,必要时先拷贝。
func (s *SessionTCP) GetFrameSlice() *FrameSlice {
	if s.w < s.r+FrameHeadLength {
		return nil
	}
	length := int(util.BytesToUint32(s.rBuf[s.r : s.r+4]))
	if s.r+length <= s.w {
		frameSlice := DecodeByBytes(s.rBuf[s.r : s.r+length])
		return frameSlice
	}
	return nil
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
func (s *SessionTCP) WriteFrameDataPromptly(f *FrameSlice) error {
	var err error
	if f.GetFrameLength() >= FrameHeadLength {
		if err = s.Conn.SetWriteDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
			return err
		}
		_, err = s.Conn.Write(f.base)
	}
	return err
}

//DefaultDelayedSend 延迟发送的时间间隔
const DefaultDelayedSend time.Duration = time.Millisecond * 2

//WriteFrameDataToCache 写入发送缓存
func (s *SessionTCP) WriteFrameDataToCache(f *FrameSlice) error {
	myslot := (*slot)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.wSlot))))
	length := int32(f.GetFrameLength())
	end := atomic.AddInt32(&myslot.allotCursor, length)
	start := end - length
	if end >= int32(util.BytesPoolLenght) {
		//申请的地址超出边界
		if start >= int32(util.BytesPoolLenght) {
			time.Sleep(time.Millisecond) //自旋等待
			return s.WriteFrameDataToCache(f)
		}
		//刚好越界触发
		ns := newSlot(s)
		atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&s.wSlot)), unsafe.Pointer(ns))
		if start > 0 {
			//自旋等待其它协程提交
			for atomic.LoadInt32(&myslot.availableCursor) != start {
			}
			s.Add(1)
			if err := s.dispatcher.PutJob(myslot); err != nil {
				s.Done()
				return err
			}
		}
		if f.GetFrameLength() > util.BytesPoolLenght {
			return nil
		}
		return s.WriteFrameDataToCache(f)
	}
	f.WriteToBytes(myslot.buf[start:end])
	atomic.AddInt32(&myslot.availableCursor, length)
	//发出越界触发
	if start == 0 {
		time.AfterFunc(DefaultDelayedSend, func() { s.WriteFrameDataToCache(FrameOverflow) })
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
}

func newSlot(s *SessionTCP) *slot {
	return &slot{
		session:         s,
		buf:             util.BytesPoolGet(),
		allotCursor:     0,
		availableCursor: 0,
	}
}

//WorkFunc 发送
func (ws *slot) WorkFunc() {
	defer ws.session.Done()
	defer util.BytesPoolPut(ws.buf)
	if ws.availableCursor >= int32(FrameHeadLength) {
		if err := ws.session.Conn.SetWriteDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
			return
		}
		if _, err := ws.session.Conn.Write(ws.buf[:ws.availableCursor]); err != nil {
			return
		}
	}
}
