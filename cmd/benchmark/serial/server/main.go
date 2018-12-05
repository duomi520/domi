package main

import (
	"fmt"
	"os"

	"github.com/duomi520/domi"
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
		RingBufferSize: 67108864, //2^26
	}
	app.RunAssembly(s)
	s.Subscribe(ChannelMsg, ping)
	app.Guard()
}
func ping(ctx *domi.ContextMQ) {
	if err := ctx.Reply([]byte("pong")); err != nil {
		fmt.Println(err.Error())
		os.Exit(2)
	}
}
