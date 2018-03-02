package server

import (
	"context"
	"github.com/duomi520/domi/cluster"
	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
	"net/http"
	"strconv"
	"sync"
	"time"
)

//Node 服务节点
type Node struct {
	Ctx         context.Context
	snowFlakeID *util.SnowFlakeID
	dispatcher  *util.Dispatcher
	options     NodeOptions
	SessionMap  *sync.Map
	tcpServer   *transport.ServerTCP
	httpServer  *http.Server
	Peer        *cluster.Peer
	Logger      *util.Logger
	*transport.Handler
	sequence *util.WaitSequence //按顺序调度
}

//NodeOptions 配置
type NodeOptions struct {
	Version          int
	HTTPPort         string
	TCPPort          string //不为空时，默认启用。
	WebsocketPattern string //不为空时，默认启用。
	Name             string
}

//NewNode 新建
func NewNode(ctx context.Context, o NodeOptions) *Node {
	var err error
	logger, _ := util.NewLogger(util.DebugLevel, "")
	logger.SetMark("Node")
	n := &Node{
		Ctx:        ctx,
		options:    o,
		SessionMap: &sync.Map{},
		Logger:     logger,
		Handler:    transport.NewHandler(),
		sequence:   util.NewWaitSequence(ctx, 5),
	}
	port, err := strconv.Atoi(o.TCPPort[1:])
	if err != nil {
		logger.Fatal("NewNode|TCPPort不为整数。" + err.Error())
		return nil
	}
	n.Peer = cluster.NewPeer(n.sequence.Ctx[0], o.Name, o.HTTPPort, port)
	n.Peer.Version = o.Version
	if err = n.Peer.RegisterServer(); err != nil {
		logger.Fatal("NewNode|注册失败：", err.Error())
	}
	n.snowFlakeID = util.NewSnowFlakeID(int64(n.Peer.ID), time.Now().UnixNano())
	//tcp支持
	if len(n.options.TCPPort) > 0 {
		n.dispatcher = util.NewDispatcher(n.sequence.Ctx[2], "Node", 256)
		n.tcpServer = transport.NewServerTCP(n.sequence.Ctx[0], n.options.TCPPort, n.Handler, n.snowFlakeID, n.dispatcher)
		if n.tcpServer == nil {
			logger.Fatal("NewNode|NewServerTCP失败。", n.options.TCPPort, n.dispatcher)
		}
		n.tcpServer.OnNewSessionTCP = n.addSessionTCP
		n.tcpServer.OnCloseSessionTCP = n.removeSessionTCP
	}
	//http支持
	n.httpServer = &http.Server{
		Addr:           o.HTTPPort,
		Handler:        http.DefaultServeMux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	return n
}

//Run 运行
func (n *Node) Run() {
	n.Logger.Info("Run|启动……")
	if n.tcpServer != nil {
		n.sequence.WG[0].Wrap(n.tcpServer.Run)
		go func() {
			<-n.sequence.Ctx[0].Done()
			n.tcpServer.Close()
		}()
	}
	go func() {
		n.Logger.Info("Run|HTTP监听端口", n.options.HTTPPort)
		if err := n.httpServer.ListenAndServe(); err != nil {
			n.Logger.Info("run|", err.Error())
		}
	}()
	n.sequence.WG[0].Wrap(n.Peer.Run)
	n.sequence.Wait()
	n.httpServer.Shutdown(context.TODO())
	n.Logger.Info("Run|关闭。")
	//	n.SessionMap = nil
}

//Close 关闭
func (n *Node) Close() {

}

//addSessionTCP 用户连接时
func (n *Node) addSessionTCP(s transport.Session) {
	id := s.GetID()
	n.SessionMap.Store(id, s)
}

//removeSessionTCP 用户连接关闭时
func (n *Node) removeSessionTCP(s transport.Session) {
	id := s.GetID()
	n.SessionMap.Delete(id)
}
