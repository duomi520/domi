package main

import (
	"github.com/duomi520/domi"
)

//定义
const (
	ChannelRoom uint16 = 50 + iota
	ChannelMsg
)

func main() {
	app := domi.NewMaster()
	r := domi.NewNode(app.Ctx, "1/room/", ":7082", ":9522", []string{"localhost:2379"})
	app.RunAssembly(r)
	r.SerialProcess(ChannelMsg, rec)
	app.Guard()
}
func rec(ctx *domi.ContextMQ) {
	ctx.Publish(ChannelRoom, ctx.Request)
}
