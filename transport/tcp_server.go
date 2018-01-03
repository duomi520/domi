package transport

import (
	"context"
	"io"
	"net"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/duomi520/domi/util"
)

//ServerTCP TCP服务
type ServerTCP struct {
	Ctx               context.Context
	sfID              *util.SnowFlakeID
	dispatcher        *util.Dispatcher
	tcpAddress        *net.TCPAddr
	tcpListener       *net.TCPListener
	tcpPost           string
	handler           *Handler
	OnNewSessionTCP   func(Session)
	OnCloseSessionTCP func(Session)

	stopChan  chan struct{} //退出信号
	closeOnce sync.Once
	Logger    *util.Logger
	closeFlag int32 //关闭状态
	util.WaitGroupWrapper
}

//NewServerTCP 新建
func NewServerTCP(ctx context.Context, post string, h *Handler, sfID *util.SnowFlakeID, d *util.Dispatcher) *ServerTCP {
	logger, _ := util.NewLogger(util.DebugLevel, "")
	logger.SetMark("ServerTCP")
	if sfID == nil {
		logger.Error("NewServerTCP|SnowFlakeID为nil")
		return nil
	}
	if h == nil {
		logger.Error("NewServerTCP|Handler不为nil")
		return nil
	}
	tcpAddress, err := net.ResolveTCPAddr("tcp4", post)
	listener, err := net.ListenTCP("tcp", tcpAddress)
	if err != nil {
		logger.Fatal("NewTCPServer|监听端口失败:", err)
	}
	s := &ServerTCP{
		Ctx:         ctx,
		sfID:        sfID,
		dispatcher:  d,
		tcpAddress:  tcpAddress,
		tcpListener: listener,
		tcpPost:     post,
		handler:     h,
		stopChan:    make(chan struct{}),
		Logger:      logger,
		closeFlag:   0,
	}
	go func() {
		select {
		case <-s.Ctx.Done():
		case <-s.stopChan:
		}
		err := s.tcpListener.Close()
		if v := ctx.Value(util.KeyExitFlag); v != nil {
			atomic.StoreInt32(&s.closeFlag, *(v.(*int32)))
		}
		s.Logger.Info("NewTCPServer|监听关闭。")
		if err != nil {
			s.Logger.Error("NewTCPServer|监听关闭失败:", err)
		}
	}()
	return s
}

//Close 关闭监听
func (s *ServerTCP) Close() {
	s.closeOnce.Do(func() {
		close(s.stopChan)
	})
}

//Run 运行
func (s *ServerTCP) Run() {
	s.tcpServer()
}

//tcpServer 服务监听
func (s *ServerTCP) tcpServer() {
	s.Logger.Info("tcpServer|TCP监听端口", s.tcpPost)
	s.Logger.Info("tcpServer|已初始化连接，等待客户端连接……")
	for {
		conn, err := s.tcpListener.AcceptTCP()
		if err != nil {
			if nErr, ok := err.(net.Error); ok && nErr.Temporary() {
				s.Logger.Warn("tcpServer|Temporary error when accepting new connections:", err)
				runtime.Gosched()
				continue
			}
			if !strings.Contains(err.Error(), "use of closed network connection") {
				s.Logger.Warn("tcpServer|Permanent error when accepting new connections:", err)
			}
			break
		}
		id, err := s.sfID.NextID()
		if err != nil {
			s.Logger.Error("tcpServer|生成SnowFlakeID错误:", err)
			continue
		}
		if err = conn.SetNoDelay(false); err != nil {
			s.Logger.Error("tcpServer|设定操作系统是否应该延迟数据包传递失败:" + err.Error())
		}
		session := NewSessionTCP(conn)
		session.ID = id
		session.dispatcher = s.dispatcher
		go func(ts *SessionTCP) {
			s.Add(1)
			defer s.Done()
			defer ts.Close()
			defer ts.Conn.Close()
			var pm uint32
			var err error
			if pm, err = ts.readUint32(); err != nil {
				s.Logger.Error("tcpServer|读取协议头失败:", err)
				return
			}
			if !protocolMagic(pm) {
				s.Logger.Warn("tcpServer|警告: 无效的协议头。")
				return
			}
			if _, err = ts.Conn.Write(util.Int64ToBytes(ts.ID)); err != nil {
				s.Logger.Error("tcpServer|写入ID失败:" + err.Error())
				return
			}
			if s.OnNewSessionTCP != nil {
				s.OnNewSessionTCP(ts)
			}
			s.receiveLoop(ts)
			if s.OnCloseSessionTCP != nil {
				s.OnCloseSessionTCP(ts)
			}
		}(session)
	}
	s.Logger.Debug("tcpServer|等待子协程关闭。")
	s.Wait()
	s.Logger.Info("tcpServer|关闭。")
}

//receiveLoop 接收
//
func (s *ServerTCP) receiveLoop(se *SessionTCP) {
	defer func() {
		if r := recover(); r != nil {
			s.Logger.Error("receiveLoop|defer错误：", r, string(debug.Stack()))
		}
	}()
	var err error
	for {
		if atomic.LoadInt32(&s.closeFlag) == 1 {
			goto end
		}
		if err = se.ioRead(); err != nil {
			goto end
		}
		//	fmt.Println("dd", se.rBuf[0:26], se.r, se.w)
		se.readFrameData()
		for i := 0; i < len(se.frameSlices); i++ {
			se.currentFrameSlice = i
			switch se.frameSlices[i].GetFrameType() {
			case FrameTypeHeartbeat:
				if err = se.WriteFrameDataPromptly(FrameHeartbeat); err != nil {
					goto end
				}
				continue
			case FrameTypeExit:
				goto end
			}
			if err = s.handler.Route(se); err != nil {
				goto end
			}
		}
	}
end:
	if err == io.EOF {
		err = nil
	}
	if err != nil {
		s.Logger.Error("receiveLoop|错误：", se.Conn.RemoteAddr(), " err:", err)
	}
	se.WriteFrameDataPromptly(FrameExit) //通知客户端关闭
	s.Logger.Debug("receiveLoop|连接断开：", se.Conn.RemoteAddr(), " err:", err)
}
