package main

import (
	"github.com/duomi520/domi"
	"github.com/duomi520/domi/util"
)

func main() {
	a := util.NewApplication()
	gate := domi.NewNode(a.Ctx, "1/gate/", ":7081", ":9521", []string{"localhost:2379"})
	/*	gto := server.NodeOptions{
			Version:          1,
			HTTPPort:         ":8080",
			TCPPort:          ":8888",
			WebsocketPattern: "/ws",
			Name:             "gate",
		}
	*/
	//	gate = server.NewGateway(ms.Ctx, gto)
	//注册一事件FrameTypeJoin，处理函数joinGroup
	//	gate.Handler.HandleFunc(chat.FrameTypeJoin, joinGroup)
	a.RunAssembly(gate)
	a.Run()
}

/*
//joinGroup 加入组
func joinGroup(s transport.Session) {
	//申请服务
	zoneID, _ := gate.CallServer(s.GetID(), "room", ":8090")
	//回复用户
	data := make([]byte, 16)
	copy(data[:8], util.Int64ToBytes(zoneID))
	copy(data[8:], util.Int64ToBytes(666))
	f := transport.NewFrameSlice(chat.FrameTypeJoin, data, nil)
	s.WriteFrameDataPromptly(f)
	//通知服务节点
	ro := make([]byte, 16)
	copy(ro[:8], util.Int64ToBytes(s.GetID()))
	copy(ro[8:], util.Int64ToBytes(zoneID))
	nf := transport.NewFrameSlice(chat.FrameTypeJoin, ro, data)
	if v, ok := gate.SessionMap.Load(zoneID); ok {
		v.(*server.UserConnect).Session.WriteFrameDataPromptly(nf)
	}
*/
