package main

import (
	"context"
	"fmt"
	"time"

	"github.com/duomi520/domi"
)

//定义
const (
	ChannelRoom uint16 = 50 + iota
	ChannelMsg
	ChannelJoin
	ChannelLeave
)

//有状态的服务
func main() {
	app := domi.NewMaster()
	//控制关闭顺序，先关闭room，再关闭node。
	r := &room{
		Ctx:       app.Ctx,
		recChan:   make(chan []byte, 254),
		joinChan:  make(chan []byte, 64),
		leaveChan: make(chan []byte, 64),
		count:     0,
	}
	var ctx context.Context
	ctx, r.Cancel = context.WithCancel(context.Background())
	app.RunAssembly(r)
	n := domi.NewNode(ctx, app.Stop, "room V1.0.1", ":7082", ":9522", []string{"localhost:2379"})
	app.RunAssembly(n)
	r.N = n
	//注册频道
	n.WatchChannel(ChannelMsg, r.recChan)
	n.WatchChannel(ChannelJoin, r.joinChan)
	n.WatchChannel(ChannelLeave, r.leaveChan)
	app.Guard()
}

type room struct {
	Ctx       context.Context
	Cancel    context.CancelFunc
	N         *domi.Node
	recChan   chan []byte
	joinChan  chan []byte
	leaveChan chan []byte
	count     int32 //用户数
}

//Run 运行
func (r *room) Run() {
	for {
		select {
		case data := <-r.recChan:
			fmt.Println(string(data))
			r.N.Publish(ChannelRoom, data)
		case <-r.joinChan:
			r.count++
		case <-r.leaveChan:
			r.count--
		case <-r.Ctx.Done():
			if r.count == 0 {
				r.Cancel()
				close(r.recChan)
				close(r.joinChan)
				close(r.leaveChan)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

//WaitInit 准备好
func (r *room) WaitInit() {}
