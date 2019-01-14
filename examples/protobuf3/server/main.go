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

func main() {
	app := domi.NewMaster()
	r := &domi.Node{
		Ctx:       app.Ctx,
		ExitFunc:  app.Stop,
		Name:      "server V1.0.0",
		HTTPPort:  ":7080",
		TCPPort:   ":9500",
		Endpoints: []string{"localhost:2379"},
	}
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
		ctx.Reply([]byte(strings.ToUpper(m.Data)), func(err error) {
			log.Println(err.Error())
		})
	}
}
