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
	r := domi.NewNode(app.Ctx, "1/client/", ":7081", ":9501", []string{"localhost:2379"})
	app.RunAssembly(r)
	r.SerialProcess(ChannelRpl, pong)
	if err := r.Call(ChannelMsg, []byte("ping"), ChannelRpl); err != nil {
		fmt.Println(err)
	}
	app.Guard()
}
func pong(ctx *domi.ContextMQ) {
	fmt.Println(string(ctx.Request))
}
