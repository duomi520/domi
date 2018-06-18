package transport

import (
	"context"
	"io"
	"net"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/duomi520/domi/util"
)

//ServerTCP TCP服务
type ServerTCP struct {
	ctx               context.Context
	sfID              *util.SnowFlakeID
	dispatcher        *util.Dispatcher
	tcpAddress        *net.TCPAddr
	tcpListener       *net.TCPListener
	tcpPost           string
	handler           *Handler
	OnNewSessionTCP   func(Session)
	OnCloseSessionTCP func(Session)
	Logger            *util.Logger
	util.WaitGroupWrapper
}

//NewServerTCP 新建
func NewServerTCP(ctx context.Context, post string, h *Handler, sfID *util.SnowFlakeID) *ServerTCP {
	logger, _ := util.NewLogger(util.ErrorLevel, "")
	logger.SetMark("ServerTCP")
	if sfID == nil {
		logger.Fatal("NewServerTCP|SnowFlakeID为nil")
		return nil
	}
	if h == nil {
		logger.Fatal("NewServerTCP|Handler不为nil")
		return nil
	}

	tcpAddress, err := net.ResolveTCPAddr("tcp4", post)
	listener, err := net.ListenTCP("tcp", tcpAddress)
	if err != nil {
		logger.Fatal("NewTCPServer|监听端口失败:", err)
	}
	s := &ServerTCP{
		ctx:         ctx,
		sfID:        sfID,
		tcpAddress:  tcpAddress,
		tcpListener: listener,
		tcpPost:     post,
		handler:     h,
		Logger:      logger,
	}
	s.dispatcher = util.NewDispatcher("TCPSend", 256)
	return s
}

//Run 运行
func (s *ServerTCP) Run() {
	go s.dispatcher.Run()
	go func() {
		var closeOnce sync.Once
		<-s.ctx.Done()
		closeOnce.Do(func() {
			if err := s.tcpListener.Close(); err != nil {
				s.Logger.Error("Close|TCP监听端口关闭失败:", err)
			} else {
				s.Logger.Debug("Close|TCP监听端口关闭。")
			}
		})
	}()
	s.Logger.Debug("tcpServer|TCP监听端口", s.tcpPost)
	s.Logger.Debug("tcpServer|已初始化连接，等待客户端连接……")
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
		go tcpReceive(s, id, conn)
	}
	s.Logger.Debug("tcpServer|等待子协程关闭……")
	s.Wait()
	s.dispatcher.Close()
	s.Logger.Debug("tcpServer|关闭。")
}

//tcpReceive 接收
func tcpReceive(s *ServerTCP, id int64, conn *net.TCPConn) {
	defer func() {
		if r := recover(); r != nil {
			s.Logger.Error("tcpReceive|defer错误：", r, string(debug.Stack()))
		}
	}()
	session := NewSessionTCP(conn)
	session.ID = id
	session.dispatcher = s.dispatcher
	s.Add(1)
	defer func() {
		session.Wait()
		session.Conn.Close()
		session.Release()
		s.Done()
	}()
	var pm uint32
	var err error
	if pm, err = session.readUint32(); err != nil {
		s.Logger.Error("tcpReceive|读取协议头失败:", err)
		return
	}
	if !protocolMagic(pm) {
		s.Logger.Warn("tcpReceive|警告: 无效的协议头。")
		return
	}
	if s.OnNewSessionTCP != nil {
		s.OnNewSessionTCP(session)
	}
	err = s.ioLoop(session)
	if s.OnCloseSessionTCP != nil {
		s.OnCloseSessionTCP(session)
	}
	if err == io.EOF {
		err = nil
	}
	if err != nil {
		if !strings.Contains(err.Error(), "wsarecv: An existing connection was forcibly closed by the remote host.") {
			s.Logger.Error("tcpReceive|错误断开：", session.Conn.RemoteAddr(), " err:", err)
			return
		}
	}
	s.Logger.Debug("tcpReceive|连接断开：", session.Conn.RemoteAddr(), " err:", err)
}

//ioLoop 接收
func (s *ServerTCP) ioLoop(session *SessionTCP) error {
	defer func() {
		if r := recover(); r != nil {
			s.Logger.Error("ioLoop|defer错误：", r, string(debug.Stack()))
		}
	}()
	for {
		if err := session.ioRead(); err != nil {
			return err
		}
		ft := session.getFrameType()
		for ft != FrameTypeNil {
			if ft == FrameTypeExit {
				return nil
			}
			if ft == FrameTypeHeartbeat {
			} else {
				if err := s.handler.Route(session); err != nil {
					return err
				}
			}
			session.r += int(util.BytesToUint32(session.rBuf[session.r : session.r+4]))
			ft = session.getFrameType()
		}
	}
}
