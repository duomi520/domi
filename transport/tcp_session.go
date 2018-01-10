package transport

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/duomi520/domi/util"
)

//定义错误
var (
	ErrFailureIORead = errors.New("transport.SessionTCP.ioRead|rBuf缓存溢出。")
)

//SessionTCP 会话
type SessionTCP struct {
	ID   int64
	Conn *net.TCPConn

	dispatcher *util.Dispatcher
	wBuf       *WriteBuf

	rBuf              []byte
	r, w              int           //rBuf 读写位置序号
	frameSlices       []*FrameSlice //每次io读取，切片指向的数据将被写入新数据。
	currentFrameSlice int

	closeOnce sync.Once
}

//NewSessionTCP 新建
func NewSessionTCP(conn *net.TCPConn) *SessionTCP {
	s := &SessionTCP{
		Conn: conn,
		rBuf: util.BytesPoolGet(),
		r:    0,
		w:    0,
	}
	s.wBuf = &WriteBuf{
		session: s,
		w:       -1,
		ready:   true,
	}
	return s
}

//GetID 取得ID
func (s *SessionTCP) GetID() int64 { return s.ID }

//GetFrameSlice 取得当前帧
func (s *SessionTCP) GetFrameSlice() *FrameSlice {
	if len(s.frameSlices) == 0 {
		return nil
	}
	return s.frameSlices[s.currentFrameSlice]
}

//Close 关闭
func (s *SessionTCP) Close() {
	s.closeOnce.Do(func() {
		util.BytesPoolPut(s.rBuf)
		for _, v := range s.frameSlices {
			v.Release()
		}
		s.frameSlices = nil
	})
}

//ioRead 读数据到rBuf，注意：每次ioRead,rBuf中的数据将被写入新数据。
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

//readFrameData 顺序从缓存中读取帧数据
func (s *SessionTCP) readFrameData() {
	s.currentFrameSlice = 0
	s.frameSlices = s.frameSlices[:0]
	for {
		if s.w < s.r+FrameHeadLength {
			return
		}
		length := int(util.BytesToUint32(s.rBuf[s.r : s.r+4]))
		//Mark
		if s.r+length <= s.w {
			frameSlice := DecodeByBytes(s.rBuf[s.r : s.r+length])
			s.frameSlices = append(s.frameSlices, frameSlice)
			s.r += length
		} else {
			return
		}
	}
}

//WriteFrameDataPromptly 立即发送数据 without delay
func (s *SessionTCP) WriteFrameDataPromptly(f *FrameSlice) error {
	var err error
	if f.GetFrameLength() >= FrameHeadLength {
		buf := util.BytesPoolGet()
		f.WriteToBytes(buf)
		if err = s.Conn.SetWriteDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
			util.BytesPoolPut(buf)
			return err
		}
		_, err = s.Conn.Write(buf[:f.GetFrameLength()])
		util.BytesPoolPut(buf)
	}
	return err
}

//WriteBuf 待发送数据的缓存
type WriteBuf struct {
	session *SessionTCP
	buf     []byte
	w       int  //buf 写位置序号
	ready   bool //延时发送信号，true：可以延时发送，false：已经有延时发送
	sync.Mutex
}

//waitForCache
const waitForCacheDuration = 1000 * time.Microsecond

//WriteFrameDataToQueue 写入发送队列
//TODO：返回累积太多未发送数据的错误
//TODO: 高链接下锁的性能影响不大，对链接较少的后段服务器，影响较大，待测试后优化。
func (s *SessionTCP) WriteFrameDataToQueue(f *FrameSlice) error {
	var err error
	s.wBuf.Lock()
	defer s.wBuf.Unlock()
	if s.wBuf.w == -1 {
		s.wBuf.buf = util.BytesPoolGet()
		s.wBuf.w = f.WriteToBytes(s.wBuf.buf)
	} else {
		if (s.wBuf.w + f.GetFrameLength()) < util.BytesPoolLenght {
			n := f.WriteToBytes(s.wBuf.buf[s.wBuf.w:])
			s.wBuf.w += n
		} else {
			ns := &WriteBuf{
				session: s,
				buf:     s.wBuf.buf,
				w:       s.wBuf.w,
			}
			s.dispatcher.JobQueue <- ns
			s.wBuf.buf = util.BytesPoolGet()
			s.wBuf.w = f.WriteToBytes(s.wBuf.buf)
		}
	}
	if s.wBuf.ready {
		s.wBuf.ready = false
		time.AfterFunc(waitForCacheDuration, s.waitToSend)
	}
	return err
}

//waitToSend 等待适当的数据再发送
func (s *SessionTCP) waitToSend() {
	s.wBuf.Lock()
	defer s.wBuf.Unlock()
	ns := &WriteBuf{
		session: s,
		buf:     s.wBuf.buf,
		w:       s.wBuf.w,
	}
	s.dispatcher.JobQueue <- ns
	s.wBuf.w = -1
	s.wBuf.buf = nil
	s.wBuf.ready = true
}

//WorkFunc 发送
func (wb *WriteBuf) WorkFunc() {
	defer util.BytesPoolPut(wb.buf)
	if wb.w >= FrameHeadLength {
		if err := wb.session.Conn.SetWriteDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
			wb.session.Close()
			return
		}
		if _, err := wb.session.Conn.Write(wb.buf[:wb.w]); err != nil {
			wb.session.Close()
			return
		}
	}
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

//readInt64 读Int64
func (s *SessionTCP) readInt64() (int64, error) {
	b := make([]byte, 8)
	if err := s.Conn.SetWriteDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
		return 0, err
	}
	if _, err := s.Conn.Read(b); err != nil {
		return 0, err
	}
	i := int64(b[0]) | int64(b[1])<<8 | int64(b[2])<<16 | int64(b[3])<<24 | int64(b[4])<<32 | int64(b[5])<<40 | int64(b[6])<<48 | int64(b[7])<<56
	return i, nil
}
