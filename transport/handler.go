package transport

import (
	"errors"
	"fmt"
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

var errFrameTypeNil = errors.New("读完缓存。")
var errFrameTypeExit = errors.New("收到退出信息。")

//NewHandler 新建
func NewHandler() *Handler {
	h := &Handler{}
	h.HandleFunc(FrameTypeNil, func(Session) error {
		return errFrameTypeNil
	})
	h.HandleFunc(FrameTypeExit, func(Session) error {
		return errFrameTypeExit
	})
	return h
}

//HandleFunc 添加处理器 线程不安全。
//处理函数避免阻塞。
func (h *Handler) HandleFunc(u16 uint16, f func(Session) error) {
	h.frameWorker[u16] = f
}

//route 帧处理器函数路由
func (h *Handler) route(ft uint16, s Session) error {
	if h.frameWorker[ft] != nil {
		return h.frameWorker[ft].(func(Session) error)(s)
	}
	return fmt.Errorf("Route|处理器函数为nil。%d", int(ft))
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
