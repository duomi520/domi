package transport

import (
	"errors"
	"time"
)

//ProtocolMagicNumber 协议头
const ProtocolMagicNumber uint32 = 2299

//DefaultHeartbeatDuration 心跳包间距
const DefaultHeartbeatDuration time.Duration = time.Second * 5

//DefaultDeadlineDuration IO超时
const DefaultDeadlineDuration time.Duration = time.Second * 10

//Handler 帧处理器函数handler
type Handler struct {
	recvWorker map[uint16]func(Session)
	Next       func(Session) error
}

//NewHandler 新建
func NewHandler() *Handler {
	h := &Handler{}
	h.recvWorker = make(map[uint16]func(Session))
	return h
}

//HandleFunc 添加处理器 线程不安全，需在初始化时设置完毕，服务启动后不能再添加。
func (h *Handler) HandleFunc(u16 uint16, f func(Session)) {
	h.recvWorker[u16] = f
}

//Route 帧处理器函数路由
func (h *Handler) Route(s Session) error {
	fs := s.GetFrameSlice()
	if v, ok := h.recvWorker[fs.GetFrameType()]; ok {
		if v != nil {
			v(s)
		} else {
			return errors.New("Route|处理器函数为nil。")
		}
	} else {
		if h.Next != nil {
			err := h.Next(s)
			return err
		}
		return errors.New("Route|" + string(fs.GetFrameType()) + "无相应的frameType处理器函数")
	}
	return nil
}

//Session 会话接口
type Session interface {
	GetID() int64
	GetFrameSlice() *FrameSlice //帧指向的空间将在下次io读取时被覆盖。
	WriteFrameDataPromptly(*FrameSlice) error
	WriteFrameDataToQueue(*FrameSlice) error
}

//protocolMagic 判断协议头是否有效
func protocolMagic(u uint32) bool {
	return u == ProtocolMagicNumber
}
