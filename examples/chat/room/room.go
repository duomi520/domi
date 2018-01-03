package main

import (
	"github.com/duomi520/domi/examples/chat"
	"github.com/duomi520/domi/server"
	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
)

var mygroup *server.Group
var zone *server.Zone

func main() {
	ms := util.NewMicroService()
	nto := server.NodeOptions{
		Version:  1,
		HTTPPort: ":8090",
		TCPPort:  ":9999",
		Name:     "room",
	}
	//新建一节点服务
	zone = server.NewZone(ms.Ctx, nto)
	//新建一组，并挂在节点服务下
	mygroup = server.NewGroup(666, zone)
	//注册一事件FrameTypeJoin，处理函数join
	mygroup.HandleFunc(chat.FrameTypeJoin, join)
	//注册一事件FrameTypeMessage，处理函数recv
	mygroup.HandleFunc(chat.FrameTypeMessage, recv)
	ms.RunAssembly(zone)
	//启动服务
	ms.Run()
}
func join(fs *transport.FrameSlice) {
	data := fs.GetData()
	//加入新成员
	mygroup.AddUser(util.BytesToInt64(data[8:16]), util.BytesToInt64(data[:8]))
}
func recv(fs *transport.FrameSlice) {
	//广播到组里所有成员
	mygroup.BroadcastToGateway(chat.FrameTypeMessage, fs)
}
