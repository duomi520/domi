package main

import (
	"fmt"

	"github.com/duomi520/domi"
)

//定义
const (
	ChannelMsg uint16 = 50
	ChannelRpl
)

func main() {
	app := domi.NewMaster()
	r := domi.NewNode(app.Ctx, "1/server/", ":7080", ":9500", []string{"localhost:2379"})
	app.RunAssembly(r)
	r.SerialProcess(ChannelMsg, ping)
	app.Guard()
}
func ping(ctx *domi.ContextMQ) {
	fmt.Println(string(ctx.Request))
	ctx.Reply([]byte("pong"))
}
