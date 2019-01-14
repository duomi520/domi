package main

import (
	"github.com/duomi520/domi"
	"log"
)

//定义
const (
	ChannelMsg uint16 = 50 + iota
	ChannelRpl
)

func main() {
	app := domi.NewMaster()
	n := &domi.Node{
		Ctx:       app.Ctx,
		ExitFunc:  app.Stop,
		Name:      "server v1.0.0",
		HTTPPort:  ":7080",
		TCPPort:   ":9500",
		Endpoints: []string{"localhost:2379"},
	}
	app.RunAssembly(n)
	s := &domi.Serial{
		Node:           n,
		RingBufferSize: 268435456, //2^30
	}
	app.RunAssembly(s)
	s.Subscribe(ChannelMsg, ping)
	app.Guard()
}
func ping(ctx *domi.ContextMQ) {
	ctx.Reply([]byte("pong"), errFunc)
}
func errFunc(err error) {
	log.Fatalln(err.Error())
}
