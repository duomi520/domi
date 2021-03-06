package sidecar

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
)

//Sidecar 边车
type Sidecar struct {
	Ctx      context.Context
	exitFunc func()

	dispatcher              *util.Dispatcher
	limiter                 *util.Limiter
	circuitBreakerConfigure *util.CircuitBreakerConfigure

	tcpServer  *transport.ServerTCP
	httpServer *http.Server

	*cluster

	readyChan chan struct{}

	doOnce sync.Once

	Logger *util.Logger
	*transport.Handler
	util.Child
}

//NewSidecar 新建
func NewSidecar(ctx context.Context, cancel func(), name, HTTPPort, TCPPort string, operation interface{}, lc *util.LimiterConfigure, cc *util.CircuitBreakerConfigure) *Sidecar {
	logger, _ := util.NewLogger(util.DebugLevel, "")
	s := &Sidecar{
		Ctx:       ctx,
		exitFunc:  cancel,
		Handler:   transport.NewHandler(),
		readyChan: make(chan struct{}),
		Logger:    logger,
	}
	var err error
	//监视
	s.cluster, err = newCluster(name, HTTPPort, TCPPort, operation, logger)
	if err != nil {
		s.Logger.Fatal(err)
	}
	s.dispatcher = util.NewDispatcher(256)
	//限流器
	if lc != nil && lc.LimitRate > 0 && lc.LimitSize > 0 {
		if transport.BytesPoolLenght >= int(lc.LimitSize) {
			s.Logger.Fatal("NewSidecar|限流器的的大小小于读取缓存。")
		}
		s.limiter = &util.Limiter{}
		s.limiter.LimiterConfigure = lc
	}
	//熔断器
	if cc == nil {
		temp := util.NewCircuitBreakerConfigure()
		s.circuitBreakerConfigure = &temp
	} else {
		s.circuitBreakerConfigure = cc
	}
	//tcp支持
	s.tcpServer = transport.NewServerTCP(ctx, TCPPort, s.Handler, s.dispatcher, s.limiter, s.circuitBreakerConfigure)
	if s.tcpServer == nil {
		s.Logger.Error("NewSidecar|NewServerTCP失败:" + TCPPort)
		return nil
	}
	s.tcpServer.Logger.SetMark(fmt.Sprintf("%d", s.MachineID))
	s.HandleFunc(transport.FrameTypeNodeName, s.addSessionTCP)
	//http支持
	s.httpServer = &http.Server{
		Addr:           HTTPPort,
		Handler:        http.DefaultServeMux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	pre := fmt.Sprintf("/%d", s.ID)
	http.HandleFunc(pre+"/ping", s.echo)
	http.HandleFunc(pre+"/exit", s.exit)
	s.Logger.SetMark(fmt.Sprintf("Sidecar.%d", s.MachineID))
	return s
}

//Init 初始化
func (s *Sidecar) Init() {

}

//WaitInit 准备好
func (s *Sidecar) WaitInit() {
	<-s.readyChan
}

//Run 运行
func (s *Sidecar) Run() {
	s.Logger.Info(fmt.Sprintf("Run|%d 启动……", s.ID))
	//启动心跳
	heartbeatSlice := make([]*transport.ClientTCP, len(s.GetInitAddress()))
	heartbeat := time.NewTicker(transport.DefaultHeartbeatDuration)
	defer heartbeat.Stop()
	//启动限流器
	if s.limiter != nil {
		go s.limiter.Run()
	}
	//启动http
	go func() {
		s.Logger.Info("Run|HTTP监听端口", s.HTTPPort)
		if err := s.httpServer.ListenAndServe(); err != nil {
			s.Logger.Debug("Run|", err.Error())
		}
	}()
	//启动tcp
	go s.dispatcher.Run()
	s.Logger.Info("Run|TCP监听端口", s.TCPPort)
	s.RunAssembly(s.tcpServer)
	//与其它服务器建立连接
	s.dialNode(heartbeatSlice)
	s.RunAssembly(s.cluster)
	s.SetState(util.StateWork)
	close(s.readyChan)
	for {
		select {
		case <-heartbeat.C:
			i := 0
			l := len(heartbeatSlice)
			for i < l {
				if err := heartbeatSlice[i].Heartbeat(); err != nil {
					copy(heartbeatSlice[i:l-1], heartbeatSlice[i+1:])
					heartbeatSlice = heartbeatSlice[:l-1]
					l--
					i--
				}
				i++
			}
		case <-s.Ctx.Done():
			s.Logger.Info("Run|等待子模块关闭……")
			s.SetState(util.StateDie)
			s.Wait()
			s.httpServer.Shutdown(context.TODO())
			s.dispatcher.Close()
			if s.limiter != nil {
				s.limiter.Close()
			}
			s.Logger.Info("Run|Sidecar关闭。")
			if err := s.DisconDistributer(); err != nil {
				s.Logger.Error(err)
			}
			return
		}
	}
}

//getURLTCP 地址转换
func (s *Sidecar) getURLTCP(u Info) string {
	if strings.EqualFold(u.Address, s.Address) {
		return "127.0.0.1" + u.TCPPort
	}
	return u.Address + u.TCPPort
}

//dialNode 与其它服务器建立连接
func (s *Sidecar) dialNode(heartbeatSlice []*transport.ClientTCP) {
	data := make([]byte, 2)
	fs := transport.NewFrameSlice(transport.FrameTypeNodeName, data, nil)
	for i, node := range s.GetInitAddress() {
		cli, err := transport.NewClientTCP(s.Ctx, s.getURLTCP(node), s.Handler, s.dispatcher, s.limiter, s.circuitBreakerConfigure)
		if err != nil {
			s.Logger.Error("Run|错误：" + err.Error())
			continue
		}
		cli.Logger.SetMark(fmt.Sprintf("%d", s.MachineID))
		s.RunAssembly(cli)
		util.CopyUint16(fs.GetData(), s.machineID)
		err = cli.Csession.WriteFrameDataPromptly(fs)
		if err != nil {
			s.Logger.Error("Run|错误：" + err.Error())
			continue
		} else {
			s.NodeChan <- nodeMsg{id: uint16(node.MachineID), ss: cli.Csession, operation: 3}
			heartbeatSlice[i] = cli
		}
	}
}

//echo Ping 回复 pong
func (s *Sidecar) echo(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "pong")
}

//exit 退出  TODO 安全 加认证
func (s *Sidecar) exit(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "exit")
	s.doOnce.Do(func() {
		s.exitFunc()
	})
}

//addSessionTCP 用户连接时
func (s *Sidecar) addSessionTCP(se transport.Session) error {
	ft := se.GetFrameSlice()
	id := util.BytesToUint16(ft.GetData())
	if s.machineID != id {
		s.NodeChan <- nodeMsg{id: id, ss: se.(*transport.SessionTCP), operation: 3}
	}
	return nil
}
