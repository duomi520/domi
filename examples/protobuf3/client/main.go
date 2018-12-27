package main

import (
	"log"
	"os"

	"github.com/duomi520/domi"
	"github.com/duomi520/domi/examples/protobuf3/pb"
	proto "github.com/golang/protobuf/proto"
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
		Name:      "client V1.0.0",
		HTTPPort:  ":7081",
		TCPPort:   ":9501",
		Endpoints: []string{"localhost:2379"},
	}
	app.RunAssembly(r)
	r.Subscribe(ChannelRpl, request)
	if len(os.Args) > 1 {
		m := &pb.Msg{}
		m.Data = os.Args[1]
		out, err := proto.Marshal(m)
		if err != nil {
			log.Fatalln(err.Error())
		}
		r.RejectFunc(100, func(status int, err error) {
			log.Fatalln(status, err.Error())
		})
		r.Call(ChannelMsg, out, ChannelRpl, 100)
		app.Guard()
	}
}
func request(ctx *domi.ContextMQ) {
	log.Println(string(ctx.Request))
	os.Exit(0)
}
