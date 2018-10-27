package main

import (
	"log"
	"os"

	"github.com/duomi520/domi"
	"github.com/duomi520/domi/examples/protobuf3/pb"
	proto "github.com/golang/protobuf/proto"
)

//定义
const (
	ChannelMsg uint16 = 50 + iota
	ChannelRpl
)

func main() {
	app := domi.NewMaster()
	r := domi.NewNode(app.Ctx, app.Stop, "client V1.0.0", ":7081", ":9501", []string{"localhost:2379"})
	app.RunAssembly(r)
	r.Subscribe(ChannelRpl, request)
	if len(os.Args) > 1 {
		m := &pb.Msg{}
		m.Data = os.Args[1]
		out, err := proto.Marshal(m)
		if err != nil {
			log.Fatalln(err.Error())
		}
		if err := r.Call(ChannelMsg, out, ChannelRpl); err != nil {
			log.Println(err.Error())
		}
		app.Guard()
	}
}
func request(ctx *domi.ContextMQ) {
	log.Println(string(ctx.Request))
	os.Exit(0)
}
