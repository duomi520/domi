package main

import (
	"log"
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
	r := &domi.Node{
		Ctx:       app.Ctx,
		ExitFunc:  app.Stop,
		Name:      "client V1.0.0",
		HTTPPort:  ":7081",
		TCPPort:   ":9501",
		Endpoints: []string{"localhost:2379"},
	}
	app.RunAssembly(r)
	r.Subscribe(ChannelRpl, pong)
	r.RejectFunc(100, func(status int, err error) {
		log.Fatalln(status, err.Error())
	})
	r.Call(ChannelMsg, []byte("ping"), ChannelRpl, 100)
	app.Guard()
}
func pong(ctx *domi.ContextMQ) {
	log.Println(string(ctx.Request))
	os.Exit(0)
}
