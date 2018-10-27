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
	n := domi.NewNode(app.Ctx, app.Stop, "server v1.0.0", ":7080", ":9500", []string{"localhost:2379"})
	app.RunAssembly(n)
	s := domi.NewSerial(n)
	s.InitRingBuffer(16777216)
	s.Subscribe(ChannelMsg, ping)
	app.RunAssembly(s)
	app.Guard()
}
func ping(ctx *domi.ContextMQ) {
	if err := ctx.Reply([]byte("pong")); err != nil {
		fmt.Println(err.Error())
		os.Exit(2)
	}
}
