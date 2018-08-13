package main

import (
	"fmt"

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
	r := domi.NewNode(app.Ctx, app.Stop, "server v1.0.0", ":7080", ":9500", []string{"localhost:2379"})
	app.RunAssembly(r)
	r.Subscribe(ChannelMsg, ping)
	app.Guard()
}
func ping(ctx *domi.ContextMQ) {
	fmt.Println(string(ctx.Request))
	ctx.Reply([]byte("pong"))
}
