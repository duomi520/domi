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
	ctx                           context.Context
	dispatcher                    *util.Dispatcher //调度
	limiter                       *util.Limiter    //限流器
	tcpAddress                    *net.TCPAddr
	tcpListener                   *net.TCPListener
	tcpPost                       string //端口号
	handler                       *Handler
	Logger                        *util.Logger
	*util.CircuitBreakerConfigure //熔断器配置
	util.WaitGroupWrapper
}

//NewServerTCP 新建
func NewServerTCP(ctx context.Context, post string, h *Handler, sd *util.Dispatcher, limiter *util.Limiter, cbc *util.CircuitBreakerConfigure) *ServerTCP {
	logger, _ := util.NewLogger(util.ErrorLevel, "")
	logger.SetMark("ServerTCP")
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
		dispatcher:  sd,
		tcpAddress:  tcpAddress,
		tcpListener: listener,
		tcpPost:     post,
		limiter:     limiter,
		handler:     h,
		Logger:      logger,
	}
	s.CircuitBreakerConfigure = cbc
	return s
}

//Init 初始化
func (s *ServerTCP) Init() {}

//WaitInit 准备好
func (s *ServerTCP) WaitInit() {}

//Run 运行
func (s *ServerTCP) Run() {
	go func() {
		var closeOnce sync.Once
		<-s.ctx.Done()
		closeOnce.Do(func() {
			if err := s.tcpListener.Close(); err != nil {
				s.Logger.Error("tcpServer|TCP监听端口关闭失败:", err)
			} else {
				s.Logger.Debug("tcpServer|TCP监听端口关闭。")
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
		if err = conn.SetNoDelay(false); err != nil {
			s.Logger.Error("tcpServer|设定操作系统是否应该延迟数据包传递失败:" + err.Error())
		}
		go tcpReceive(s, conn)
	}
	s.Logger.Debug("tcpServer|等待子协程关闭……")
	s.Wait()
	s.Logger.Debug("tcpServer|ServerTCP关闭。")
}

//tcpReceive 接收
func tcpReceive(s *ServerTCP, conn *net.TCPConn) {
	defer func() {
		if r := recover(); r != nil {
			s.Logger.Error("tcpReceive|defer错误：", r, string(debug.Stack()))
		}
	}()
	session := NewSessionTCP(conn, s.handler, s.CircuitBreakerConfigure)
	session.dispatcher = s.dispatcher
	s.Add(1)
	defer func() {
		session.Close()
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
	err = s.ioLoop(session)
	if err != nil && err != io.EOF {
		if !strings.Contains(err.Error(), "wsarecv: An existing connection was forcibly closed by the remote host.") {
			s.Logger.Error("tcpReceive|错误断开：", session.Conn.RemoteAddr(), " err:", err)
			return
		}
	}
}

//ioLoop 接收
func (s *ServerTCP) ioLoop(session *SessionTCP) error {
	defer func() {
		if r := recover(); r != nil {
			s.Logger.Error("ioLoop|defer错误：", r, string(debug.Stack()))
		}
	}()
	for {
		n, err := session.ioRead()
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				return nil
			}
			return err
		}
		//限流器阻塞
		if s.limiter != nil {
			s.limiter.Wait(int64(n))
		}
		//处理数据
		for {
			ft := session.getFrameType()
			if err := s.handler.route(ft, session); err != nil {
				//读完缓存
				if ft == FrameTypeNil {
					break
				}
				//收到退出指令
				if ft == FrameTypeExit {
					return nil
				}
				s.Logger.Error("ioLoop|:", err.Error())
			}
			session.r += int(util.BytesToUint32(session.rBuf[session.r : session.r+4]))
		}
	}
}
