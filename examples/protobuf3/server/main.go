package main

import (
	"github.com/duomi520/domi"
	"github.com/duomi520/domi/examples/protobuf3/pb"
	proto "github.com/golang/protobuf/proto"
	"log"
	"strings"
)

//定义
const (
	ChannelMsg uint16 = 50 + iota
	ChannelRpl
)

//无状态的服务
func main() {
	app := domi.NewMaster()
	r := domi.NewNode(app.Ctx, app.Stop, "server v1.0.0", ":7080", ":9500", []string{"localhost:2379"})
	app.RunAssembly(r)
	r.Subscribe(ChannelMsg, reply)
	app.Guard()
}
func reply(ctx *domi.ContextMQ) {
	m := &pb.Msg{}
	if err := proto.Unmarshal(ctx.Request, m); err != nil {
		log.Fatalln(err.Error())
	} else {
		log.Println(m.Data)
		ctx.Reply([]byte(strings.ToUpper(m.Data)))
	}
}
