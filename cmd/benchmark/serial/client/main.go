package main

import (
	"fmt"
	"log"
	"strings"
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
	node := &domi.Node{
		Ctx:       app.Ctx,
		ExitFunc:  app.Stop,
		Name:      "client v1.0.0",
		HTTPPort:  ":7081",
		TCPPort:   ":9501",
		Endpoints: []string{"localhost:2379"},
	}
	app.RunAssembly(node)
	s := &domi.Serial{
		Node:           node,
		RingBufferSize: 67108864, //2^26
	}
	app.RunAssembly(s)
	s.Subscribe(ChannelRpl, pong)
	lose := 0
	s.RejectFunc(55, func(status int, err error) {
		if strings.Contains(err.Error(), "ErrConnClose|SessionTCP已关闭") {
			log.Fatalln(status, err.Error())
		}
		lose++
		clientNwg.Done()
		if lose > 1000 {
			log.Fatalln("lose太多。")
		}
	})
	s.RejectFunc(56, func(status int, err error) {
		log.Fatalln(err.Error())
	})
	loop := 2000 //20000
	gNum := 1000 //1000
	clientNwg.Add(loop * gNum)
	start := time.Now()
	for j := 0; j < gNum; j++ {
		go func() {
			for i := 0; i < loop; i++ {
				s.Call(ChannelMsg, []byte("ping"), ChannelRpl, 55)
			}
		}()
	}
	clientNwg.Wait()
	end := time.Now()
	qps := float64(loop*gNum) / end.Sub(start).Seconds()
	fmt.Printf("1个Node运行%d个协程Call:%6.0f   lose:%d\n", gNum, qps, lose)
	//num个节点
	num := 100 //1000
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
		n[i] = &domi.Node{
			Ctx:       app.Ctx,
			ExitFunc:  app.Stop,
			Name:      "client v1.0.0",
			HTTPPort:  hp,
			TCPPort:   tp,
			Endpoints: []string{"localhost:2379"},
		}
		app.RunAssembly(n[i])
		n[i].Subscribe(cr[i], pong)
	}
	loop = 1000 //10000
	clientNwg.Add(loop * num)
	start = time.Now()
	for j := 0; j < num; j++ {
		go func(k int) {
			for i := 0; i < loop; i++ {
				n[k].Call(ChannelMsg, []byte("ping"), cr[k], 56)
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
}
