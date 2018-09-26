package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/duomi520/domi"
	"github.com/duomi520/domi/util"
)

//定义
const (
	ChannelMsg uint16 = 50 + iota
	ChannelRpl
)

func main() {
	app := domi.NewMaster()
	app.Logger.SetLevel(util.ErrorLevel)
	n := domi.NewNode(app.Ctx, app.Stop, "client V1.0.0", ":7081", ":9501", []string{"localhost:2379"})
	app.RunAssembly(n)
	s := domi.NewSerial(n)
	s.Subscribe(ChannelRpl, pong)
	app.RunAssembly(s)
	clientN(s, 2000)
	time.AfterFunc(time.Second, app.Stop)
	app.Guard()
}

var clientNwg sync.WaitGroup

func clientN(s *domi.Serial, num int) {
	loop := 20000
	clientNwg.Add(loop * num)
	start := time.Now()
	for j := 0; j < num; j++ {
		go func() {
			for i := 0; i < loop; i++ {
				if err := s.Call(ChannelMsg, []byte("ping"), ChannelRpl); err != nil {
					fmt.Println(err)
					os.Exit(2)
				}

			}
		}()
	}
	clientNwg.Wait()
	end := time.Now()
	qps := float64(loop*num) / end.Sub(start).Seconds()
	fmt.Printf("%d个协程:%6.0f\n", num, qps)
}
func pong(ctx *domi.ContextMQ) {
	clientNwg.Done()
	//fmt.Println(string(ctx.Request))
}
