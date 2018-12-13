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
	if err := s.Subscribe(ChannelMsg, ping); err != nil {
		log.Fatalln(err.Error())
	}
	app.Guard()
}
func ping(ctx *domi.ContextMQ) {
	if err := ctx.Reply([]byte("pong")); err != nil {
		log.Fatalln(err.Error())
	}
}
