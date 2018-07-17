package main

import (
	"fmt"

	"github.com/duomi520/domi"
)

//定义
const (
	FrameTypeMsg uint16 = 50
)

func main() {
	app := domi.NewMaster()
	r := domi.NewNode(app.Ctx, "1/server/", ":7080", ":9500", []string{"localhost:2379"})
	r.HandleC(FrameTypeMsg, ping)
	app.RunAssembly(r)
	app.Run()
}
func ping(ctx *domi.ContextMQ) {
	fmt.Println(string(ctx.Request))
	ctx.Reply([]byte("pong"))
}
