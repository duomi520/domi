package main

import (
	"log"

	"github.com/duomi520/domi"
)

//定义
const (
	ChannelMsg uint16 = 50 + iota
	ChannelRpl
)

//无状态的服务
func main() {
	app := domi.NewMaster()
	r := &domi.Node{
		Ctx:       app.Ctx,
		ExitFunc:  app.Stop,
		Name:      "server v1.0.0",
		HTTPPort:  ":7080",
		TCPPort:   ":9500",
		Endpoints: []string{"localhost:2379"},
	}
	app.RunAssembly(r)
	r.Subscribe(ChannelMsg, ping)
	app.Guard()
}
func ping(ctx *domi.ContextMQ) {
	log.Println(string(ctx.Request))
	ctx.Node.RejectFunc(100, func(status int, err error) {
		log.Println(status, err.Error())
	})
	ctx.Reply([]byte("pong"), 100)
}
