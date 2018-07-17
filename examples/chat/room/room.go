package main

import (
	"github.com/duomi520/domi"
)

//定义
const (
	FrameTypeRoom uint16 = 50 + iota
	FrameTypeMsg
)

func main() {
	app := domi.NewMaster()
	r := domi.NewNode(app.Ctx, "1/room/", ":7082", ":9522", []string{"localhost:2379"})
	r.HandleC(FrameTypeMsg, rec)
	app.RunAssembly(r)
	app.Run()
}
func rec(ctx *domi.ContextMQ) {
	ctx.Publish(FrameTypeRoom, ctx.Request)
}
