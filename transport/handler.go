package transport

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"
	"unsafe"
)

//ProtocolMagicNumber 协议头
const ProtocolMagicNumber uint32 = 2299

//DefaultHeartbeatDuration 心跳包间距
const DefaultHeartbeatDuration time.Duration = time.Second * 5

//DefaultDeadlineDuration IO超时
const DefaultDeadlineDuration time.Duration = time.Second * 10

//DefaultWaitCloseDuration 等待关闭时间
const DefaultWaitCloseDuration time.Duration = time.Second * 2

//Handler 帧处理器函数handler
type Handler struct {
	frameWorker [65536]unsafe.Pointer
	errorWorker [65536]unsafe.Pointer
}

var errFrameTypeNil = errors.New("读完缓存。")
var errFrameTypeExit = errors.New("收到退出信息。")

//NewHandler 新建
func NewHandler() *Handler {
	h := &Handler{}
	h.HandleFunc(FrameTypeNil, func(Session) error {
		return errFrameTypeNil
	})
	h.HandleFunc(FrameTypeHeartbeatC, func(s Session) error {
		return nil
	})
	h.HandleFunc(FrameTypeHeartbeatS, func(s Session) error {
		s.WriteFrameDataPromptly(FrameHeartbeatC)
		return nil
	})
	h.HandleFunc(FrameTypeExit, func(Session) error {
		return errFrameTypeExit
	})
	return h
}

//HandleFunc 添加处理器
func (h *Handler) HandleFunc(u16 uint16, f func(Session) error) {
	atomic.StorePointer(&h.frameWorker[u16], unsafe.Pointer(&f))
}

//ErrorFunc 错误处理器
func (h *Handler) ErrorFunc(u16 uint16, f func(int, error)) {
	atomic.StorePointer(&h.errorWorker[u16], unsafe.Pointer(&f))
}

//route 帧处理器函数路由
func (h *Handler) route(ft uint16, s Session) error {
	f := atomic.LoadPointer(&h.frameWorker[ft])
	if f != nil {
		t := (*(*func(Session) error)(f))
		return t(s)
	}
	return fmt.Errorf("route|处理器函数为nil。%d", int(ft))
}

//ErrorRoute 错误路由
func (h *Handler) ErrorRoute(ft uint16, status int, err error) {
	if ft < FrameTypeNil {
		f := atomic.LoadPointer(&h.errorWorker[ft])
		if f != nil {
			t := *((*func(int, error))(f))
			t(status, err)
		}
	}
}

//protocolMagic 判断协议头是否有效
func protocolMagic(u uint32) bool {
	return u == ProtocolMagicNumber
}

//Session 会话接口
type Session interface {
	GetFrameSlice() FrameSlice //帧指向的空间将在下次io读取时被覆盖。
	WriteFrameDataPromptly(FrameSlice) error
	WriteFrameDataToCache(FrameSlice, uint16)
	Close()
}
