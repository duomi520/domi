package main

import (
	"fmt"
	"time"

	"github.com/duomi520/domi"
)

//定义
const (
	FrameTypeMsg uint16 = 50
)

func main() {
	app := domi.NewMaster()
	r := domi.NewNode(app.Ctx, "1/client/", ":7081", ":9501", []string{"localhost:2379"})
	app.RunAssembly(r)
	time.Sleep(time.Second)
	if err := r.Call(FrameTypeMsg, "1/server/", []byte("ping"), pong); err != nil {
		fmt.Println(err)
	}
	app.Run()
}
func pong(ctx *domi.ContextMQ) {
	fmt.Println(string(ctx.Request))
}
