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
	frameWorker [65536]interface{}
}

//NewHandler 新建
func NewHandler() *Handler {
	h := &Handler{}
	return h
}

//HandleFunc 添加处理器 线程不安全。
//处理函数避免阻塞。
func (h *Handler) HandleFunc(u16 uint16, f func(Session)) {
	h.frameWorker[u16] = f
}

//Route 帧处理器函数路由
func (h *Handler) Route(s Session) error {
	ft := s.GetFrameSlice().GetFrameType()
	if h.frameWorker[ft] != nil {
		h.frameWorker[ft].(func(Session))(s)
	} else {
		return errors.New("Route|处理器函数为nil。")
	}
	return nil
}

//Session 会话接口
type Session interface {
	GetID() int64
	GetFrameSlice() *FrameSlice //帧指向的空间将在下次io读取时被覆盖。
	WriteFrameDataPromptly(*FrameSlice) error
	WriteFrameDataToCache(*FrameSlice) error
}

//protocolMagic 判断协议头是否有效
func protocolMagic(u uint32) bool {
	return u == ProtocolMagicNumber
}
