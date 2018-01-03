package server

import (
	"context"
	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
	"net/http"
	"sync"
)

//Gateway 网关
type Gateway struct {
	URLMap         *sync.Map
	addUserChan    chan nodeArgs
	userLeaveChan  chan nodeArgs
	addNodeChan    chan nodeArgs
	removeNodeChan chan nodeArgs
	websocket      *transport.ServerWebsocket
	*Node
}

//NewGateway 新建
func NewGateway(ctx context.Context, o NodeOptions) *Gateway {
	logger, _ := util.NewLogger(util.DebugLevel, "")
	logger.SetMark("Gateway")
	g := &Gateway{
		URLMap:         &sync.Map{},
		addUserChan:    make(chan nodeArgs, 100),
		userLeaveChan:  make(chan nodeArgs, 100),
		addNodeChan:    make(chan nodeArgs, 100),
		removeNodeChan: make(chan nodeArgs, 100),
	}
	g.Node = NewNode(ctx, o)
	g.Logger = logger
	g.HandleFunc(transport.FrameTypeGateToNextZoneFunc, g.toNextZoneFunc)
	g.HandleFunc(transport.FrameTypeGateToUserFunc, g.toUserFunc)
	//启动WebSocket支持
	if len(g.Node.options.WebsocketPattern) > 0 {
		g.websocket = transport.NewServerWebsocket(g.sequence.Ctx[0], g.Handler, g.Node.snowFlakeID)
		g.websocket.OnNewSessionTCP = g.addSessionTCP
		g.websocket.OnCloseSessionTCP = g.removeSessionTCP
		http.HandleFunc(g.Node.options.WebsocketPattern, func(w http.ResponseWriter, r *http.Request) {
			g.websocket.Accept(w, r)
		})
		http.Handle("/", http.FileServer(http.Dir("./")))
	}
	//
	if len(g.options.TCPPort) > 0 {
		g.dispatcher = util.NewDispatcher(g.sequence.Ctx[2], "Gateway", 256)
		g.tcpServer.OnNewSessionTCP = g.addSessionTCP
		g.tcpServer.OnCloseSessionTCP = g.removeSessionTCP
	}
	//
	return g
}

//Run 运行
func (g *Gateway) Run() {
	if g.dispatcher != nil {
		g.sequence.WG[2].Wrap(g.dispatcher.Run)
	}
	if len(g.Node.options.WebsocketPattern) > 0 {
		g.sequence.WG[0].Wrap(g.websocket.Run)
	}
	g.sequence.WG[3].Wrap(g.governance)
	g.Node.Run()
	//  n.UrlMap =nil
}

//toNextZoneFunc 路由到服务节点
//输入： 扩展 8-byte 目标服务节点id，8-byte 组id，2-byte frameType
//输出： 扩展 8-byte 用户端id  ，8-byte 组id，
func (g *Gateway) toNextZoneFunc(s transport.Session) {
	id := s.GetID()
	f := s.GetFrameSlice()
	ex := f.GetExtend()
	if len(ex) != 18 {
		g.Logger.Error("toServerFunc|扩展的长度不对。")
		return
	}
	oid := util.BytesToInt64(ex[:8])
	f.SetFrameTypeByBytes(ex[16:18])
	if v, ok := g.SessionMap.Load(oid); ok {
		copy(ex[:8], util.Int64ToBytes(id))
		f = f.SetExtend(ex[:16])
		if err := v.(*UserConnect).Session.WriteFrameDataPromptly(f); err != nil { //TODO SendToQueue
			g.Logger.Error("toServerFunc|路由转发失败：", err.Error())
		}
	} else {
		g.Logger.Error("toServerFunc|匹配不成功。", oid, id)
	}
}

//toUserFunc 路由给用户
//输入：扩展  N个 8-byte ... N个用户端id，8-byte 组id，2-byte frameType
//输出：无扩展
func (g *Gateway) toUserFunc(s transport.Session) {
	f := s.GetFrameSlice()
	ex := f.GetExtend()
	n := len(ex) - 10
	f.SetFrameTypeByBytes(ex[n+8:])
	f = f.SetExtend(nil)
	for i := 0; i < n; i = i + 8 {
		id := util.BytesToInt64(ex[i : i+8])
		if v, ok := g.SessionMap.Load(id); ok {
			if err := v.(*UserConnect).Session.WriteFrameDataPromptly(f); err != nil { //TODO WriteFrameDataToQueue
				g.Logger.Error("toUserFunc|路由转发失败：", err.Error())
			}
		} else {
			g.Logger.Info("toUserFunc|匹配不成功。", id)
			extend := make([]byte, 16)
			copy(extend[:8], ex[i:i+8])
			copy(extend[8:], ex[n:n+8])
			nf := transport.NewFrameSlice(transport.FrameTypeUserLeave, util.Int64ToBytes(s.GetID()), extend)
			if err := s.WriteFrameDataPromptly(nf); err != nil {
				g.Logger.Error("toUserFunc|通知服务节点失败。")
			}
		}

	}

}

//UserConnect 用户申请的服务
type UserConnect struct {
	Session transport.Session
	nodes   []int64
}

//nodeArgs 服务节点的参数
type nodeArgs struct {
	serverName string
	userID     int64
	nodeID     int64
	tcp        *transport.ClientTCP
	url        string
}

//NodeInfo 服务节点信息
type NodeInfo struct {
	url   string
	TCP   *transport.ClientTCP
	users map[int64]bool
}

func (g *Gateway) governance() {
	nodeMap := make(map[int64]*NodeInfo)
	for {
		select {
		//用户新申请服务
		case au := <-g.addUserChan:
			nodeMap[au.nodeID].users[au.userID] = true
			if v, ok := g.SessionMap.Load(au.userID); ok {
				v.(*UserConnect).nodes = append(v.(*UserConnect).nodes, au.nodeID)
			}
		//用户离开服务节点
		case ul := <-g.userLeaveChan:
			if ul.nodeID == 0 {
				if v, ok := g.SessionMap.Load(ul.userID); ok {
					for _, nid := range v.(*UserConnect).nodes {
						if nid != 0 {
							delete(nodeMap[nid].users, ul.userID)
							if len(nodeMap[nid].users) == 0 {
								nodeMap[nid].TCP.Close()
							}
						}
					}
				}
				g.SessionMap.Delete(ul.userID)
			} else {
				delete(nodeMap[ul.nodeID].users, ul.userID)
				if v, ok := g.SessionMap.Load(ul.userID); ok {
					for index, nid := range v.(*UserConnect).nodes {
						if nid == ul.nodeID {
							v.(*UserConnect).nodes[index] = 0
						}
					}
				}
				if len(nodeMap[ul.nodeID].users) == 0 {
					nodeMap[ul.nodeID].TCP.Close()
				}
			}
		//新增加一服务节点
		case an := <-g.addNodeChan:
			nni := &NodeInfo{users: make(map[int64]bool)}
			nni.TCP = an.tcp
			nni.url = an.url
			nni.users[an.userID] = true
			nodeMap[an.nodeID] = nni
			if v, ok := g.SessionMap.Load(an.userID); ok {
				v.(*UserConnect).nodes = append(v.(*UserConnect).nodes, an.nodeID)
			}
		//服务节点连接关闭
		case rn := <-g.removeNodeChan:
			g.SessionMap.Delete(rn.nodeID)
			g.URLMap.Delete(nodeMap[rn.nodeID].url)
			nodeMap[rn.nodeID].users = nil
			nodeMap[rn.nodeID].TCP = nil
			delete(nodeMap, rn.nodeID)
		case <-g.Ctx.Done():
			goto end
		}
	}
end:
	nodeMap = nil
}

//addSessionTCP 用户连接时
func (g *Gateway) addSessionTCP(s transport.Session) {
	g.SessionMap.Store(s.GetID(), &UserConnect{Session: s, nodes: make([]int64, 0, 8)})
}

//removeSessionTCP 用户连接关闭时
func (g *Gateway) removeSessionTCP(s transport.Session) {
	g.userLeaveChan <- nodeArgs{userID: s.GetID()}
}

//LeaveServer 离开服务节点
func (g *Gateway) LeaveServer(user, nodeID int64) {
	g.userLeaveChan <- nodeArgs{userID: user, nodeID: nodeID}
}

//removeNode 节点服务连接关闭时
func (g *Gateway) removeNode(s transport.Session) {
	g.removeNodeChan <- nodeArgs{nodeID: s.GetID()}
}

//CallServer 申请服务 返回服务节点ID
func (g *Gateway) CallServer(user int64, serverName, httpPort string) (int64, error) {
	url, err := g.Peer.ChoseServer(serverName, httpPort)
	if err != nil {
		g.Logger.Error("CallServer|加入服务失败:", err)
		return 0, err
	}
	if v, ok := g.URLMap.Load(url); ok {
		g.addUserChan <- nodeArgs{userID: user, nodeID: v.(int64)}
		return v.(int64), nil
	}
	client, err := transport.NewClientTCP(g.sequence.Ctx[2], url, g.Handler)
	if err != nil {
		g.Logger.Error("CallServer|加入服务失败:", err)
		return 0, err
	}
	client.OnCloseSessionTCP = g.removeNode
	g.SessionMap.Store(client.Csession.ID, &UserConnect{Session: client.Csession})
	g.URLMap.Store(url, client.Csession.ID)
	g.addNodeChan <- nodeArgs{userID: user, nodeID: client.Csession.ID, tcp: client, url: url}
	g.sequence.WG[2].Wrap(client.Run)
	return client.Csession.ID, nil
}

//--------------------------------------------------------------------------

//SendToZonePromptly golang客户端发送到服务节点
//参数 ex: 8-byte 目标服务节点id,8-byte 组id
func SendToZonePromptly(s transport.Session, ft uint16, data []byte, ex []byte) error {
	extend := make([]byte, 18)
	copy(extend[:16], ex)
	copy(extend[16:], util.Uint16ToBytes(ft))
	f := transport.NewFrameSlice(transport.FrameTypeGateToNextZoneFunc, data, extend)
	return s.WriteFrameDataPromptly(f)
}

//SendToUserPromptly 服务节点发送到客户端
//参数 ex: 8-byte 目标用户端id, 8-byte 组id
func SendToUserPromptly(s transport.Session, ft uint16, data []byte, ex []byte) error {
	extend := make([]byte, 18) //TODO 优化
	copy(extend[:16], ex)
	copy(extend[16:], util.Uint16ToBytes(ft))
	f := transport.NewFrameSlice(transport.FrameTypeGateToUserFunc, data, extend)
	return s.WriteFrameDataPromptly(f)
}
