package transport

import (
	"context"
	"net/http"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/duomi520/domi/util"
	"github.com/gorilla/websocket"
)

//ServerWebsocket Websocket服务
type ServerWebsocket struct {
	Ctx               context.Context
	sfID              *util.SnowFlakeID
	handler           *Handler
	OnNewSessionTCP   func(Session)
	OnCloseSessionTCP func(Session)
	stopChan          chan struct{} //退出信号
	closeOnce         sync.Once
	Logger            *util.Logger
	closeFlag         int32 //关闭状态
	util.WaitGroupWrapper
}

//NewServerWebsocket 新建
func NewServerWebsocket(ctx context.Context, h *Handler, sfID *util.SnowFlakeID) *ServerWebsocket {
	logger, _ := util.NewLogger(util.DebugLevel, "")
	logger.SetMark("ServerWebsocket")
	if sfID == nil {
		logger.Error("NewServerWebsocket|Context缺少SnowFlakeID")
		return nil
	}
	if h == nil {
		logger.Error("NewServerWebsocket|Handler不为nil")
		return nil
	}
	s := &ServerWebsocket{
		Ctx:       ctx,
		sfID:      sfID,
		handler:   h,
		stopChan:  make(chan struct{}),
		Logger:    logger,
		closeFlag: 0,
	}
	return s
}

//Close 关闭监听
func (s *ServerWebsocket) Close() {
	s.closeOnce.Do(func() {
		s.stopChan <- struct{}{}
	})
}

//Run 运行
func (s *ServerWebsocket) Run() {
	s.Logger.Debug("Run|已初始化连接，等待客户端连接……")
	select {
	case <-s.Ctx.Done():
	case <-s.stopChan:
	}
	if v := s.Ctx.Value(util.KeyExitFlag); v != nil {
		atomic.StoreInt32(&s.closeFlag, *(v.(*int32)))
	}
	s.Logger.Debug("Run|等待子协程关闭。", s.closeFlag)
	s.Wait()
	s.Logger.Info("Run|关闭。")
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  2048,
	WriteBufferSize: 2048,
}

//Accept 接受连接
func (s *ServerWebsocket) Accept(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r := recover(); r != nil {
			s.Logger.Error("Accept|defer错误：", r, string(debug.Stack()))
		}
	}()
	conn, err := upgrader.Upgrade(w, r, nil)
	defer conn.Close()
	if err != nil {
		s.Logger.Error("Accept|Upgrade错误：", err)
		return
	}
	if atomic.LoadInt32(&s.closeFlag) == 2 {
		s.Logger.Error("Accept|服务已关闭。")
		return
	}
	id, err := s.sfID.NextID()
	if err != nil {
		s.Logger.Error("Accept|生成SnowFlakeID错误:", err)
		return
	}
	sws := NewSessionWebsocket(conn)
	sws.ID = id
	u32, err := sws.readUint32()
	if err != nil {
		s.Logger.Error("Accept|读取头部失败:", err)
		return
	}
	if !protocolMagic(u32) {
		s.Logger.Warn("Accept|警告: 无效的协议头。")
		return
	}
	s.Add(1)
	if s.OnNewSessionTCP != nil {
		s.OnNewSessionTCP(sws)
	}
	s.Logger.Debug("Accept|用户接入:", sws.ID, s.closeFlag)
	for {
		if atomic.LoadInt32(&s.closeFlag) == 1 {
			goto end
		}
		//设置IO超时
		conn.SetReadDeadline(time.Now().Add(DefaultDeadlineDuration))
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				s.Logger.Error("Accept|IsUnexpectedCloseError:", err.Error())
			}
			goto end
		}
		sws.currentFrameSlice = DecodeByBytes(message)
		switch sws.currentFrameSlice.GetFrameType() {
		case FrameTypeHeartbeat:
			if err = sws.WriteFrameDataPromptly(FrameHeartbeat); err != nil {
				goto end
			}
			continue
		case FrameTypeExit:
			goto end
		}
		//		s.Logger.Debug(string(message), "|", sws.GetFrameSlice(), "|", sws.GetFrameSlice().GetData())
		if err = s.handler.Route(sws); err != nil {
			goto end
		}
	}
end:
	s.Done()
	if s.OnCloseSessionTCP != nil {
		s.OnCloseSessionTCP(sws)
	}
	s.Logger.Debug("Accept|用户断开:", sws.ID)
}

//SessionWebsocket 会话
type SessionWebsocket struct {
	ID                int64
	conn              *websocket.Conn
	currentFrameSlice *FrameSlice //当前帧
}

//NewSessionWebsocket 新建
func NewSessionWebsocket(conn *websocket.Conn) *SessionWebsocket {
	s := &SessionWebsocket{
		conn: conn,
	}
	return s
}

//GetID 取得ID
func (s *SessionWebsocket) GetID() int64 { return s.ID }

//GetFrameSlice 取得当前帧
func (s *SessionWebsocket) GetFrameSlice() *FrameSlice { return s.currentFrameSlice }

//WriteFrameDataPromptly 立即发送数据
func (s *SessionWebsocket) WriteFrameDataPromptly(f *FrameSlice) error {
	w, err := s.conn.NextWriter(websocket.BinaryMessage)
	if err == nil {
		if f.GetFrameLength() >= FrameHeadLength {
			buf := util.BytesPoolGet()
			f.WriteToBytes(buf)
			_, err = w.Write(buf[:f.GetFrameLength()])
			util.BytesPoolPut(buf)
		}
	}
	return err
}

//WriteFrameDataToQueue 同WriteFrameDataPromptly
func (s *SessionWebsocket) WriteFrameDataToQueue(f *FrameSlice) error {
	return s.WriteFrameDataPromptly(f)
}

//readUint32 读uint32
func (s *SessionWebsocket) readUint32() (uint32, error) {
	_, message, err := s.conn.ReadMessage()
	if err != nil {
		return 0, err
	}
	var u uint32
	if len(message) == 4 {
		u = uint32(message[0]) | uint32(message[1])<<8 | uint32(message[2])<<16 | uint32(message[3])<<24
	}
	return u, err
}
