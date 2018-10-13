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

var clientNwg sync.WaitGroup
var n [num]*domi.Node
var cr [num]uint16

const num int = 100

func main() {
	app := domi.NewMaster()
	app.Logger.SetLevel(util.ErrorLevel)
	//单节点
	node := domi.NewNode(app.Ctx, app.Stop, "client V1.0.0", ":7081", ":9501", []string{"localhost:2379"})
	app.RunAssembly(node)
	s := domi.NewSerial(node)
	s.Subscribe(ChannelRpl, pong)
	app.RunAssembly(s)
	loop := 20000
	gNum := 1000
	clientNwg.Add(loop * gNum)
	start := time.Now()
	for j := 0; j < gNum; j++ {
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
	qps := float64(loop*gNum) / end.Sub(start).Seconds()
	fmt.Printf("1个Node运行%d个协程Call:%6.0f\n", gNum, qps)
	//num个节点
	for i := 0; i < num; i++ {
		var hp, tp string
		cr[i] = uint16(i)
		if i < 10 {
			hp = fmt.Sprintf(":500%d", i)
			tp = fmt.Sprintf(":600%d", i)
		} else {
			if i < num {
				hp = fmt.Sprintf(":50%d", i)
				tp = fmt.Sprintf(":60%d", i)
			} else {
				hp = fmt.Sprintf(":5%d", i)
				tp = fmt.Sprintf(":6%d", i)
			}
		}
		n[i] = domi.NewNode(app.Ctx, app.Stop, "client V1.0.0", hp, tp, []string{"localhost:2379"})
		app.RunAssembly(n[i])
		n[i].Subscribe(cr[i], pong)
	}
	loop = 10000
	clientNwg.Add(loop * num)
	start = time.Now()
	for j := 0; j < num; j++ {
		go func(k int) {
			for i := 0; i < loop; i++ {
				if err := n[k].Call(ChannelMsg, []byte("ping"), cr[k]); err != nil {
					fmt.Println(err)
					os.Exit(2)
				}
			}
		}(j)
	}
	clientNwg.Wait()
	end = time.Now()
	qps = float64(loop*num) / end.Sub(start).Seconds()
	fmt.Printf("%d个Node运行Call:%6.0f\n", num, qps)
	time.AfterFunc(time.Second, app.Stop)
	app.Guard()
}

func pong(ctx *domi.ContextMQ) {
	clientNwg.Done()
	//fmt.Println(string(ctx.Request))
}
