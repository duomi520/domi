package main

import (
	"context"
	"log"
	"sync"
	"sync/atomic"

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
		Ctx:      app.Ctx,
		stopChan: make(chan struct{}),
		count:    0,
	}
	var ctx context.Context
	ctx, r.Cancel = context.WithCancel(context.Background())
	app.RunAssembly(r)
	n := &domi.Node{
		Ctx:       ctx,
		ExitFunc:  app.Stop,
		Name:      "room V1.0.1",
		HTTPPort:  ":7082",
		TCPPort:   ":9522",
		Endpoints: []string{"localhost:2379"},
	}
	app.RunAssembly(n)
	//注册频道
	n.Subscribe(ChannelMsg, r.rec)
	n.Subscribe(ChannelJoin, r.join)
	n.Subscribe(ChannelLeave, r.leave)
	app.Guard()
}

type room struct {
	Ctx       context.Context
	Cancel    context.CancelFunc
	stopChan  chan struct{}
	count     int32 //用户数
	closeOnce sync.Once
}

//Run 运行
func (r *room) Run() {
	<-r.Ctx.Done()
	if atomic.LoadInt32(&r.count) == 0 {
		r.stop()
	}
	<-r.stopChan
	r.Cancel()
}

//Init 初始化
func (r *room) Init() {}

//WaitInit 准备好
func (r *room) WaitInit() {}
func (r *room) stop() {
	r.closeOnce.Do(func() {
		close(r.stopChan)
	})
}
func (r *room) rec(ctx *domi.ContextMQ) {
	log.Println(string(ctx.Request))
	ctx.Publish(ChannelRoom, ctx.Request)
}
func (r *room) join(ctx *domi.ContextMQ) {
	atomic.AddInt32(&r.count, 1)
}
func (r *room) leave(ctx *domi.ContextMQ) {
	nc := atomic.AddInt32(&r.count, -1)
	//用户数为0时才能正常关闭
	if nc == 0 {
		select {
		case <-r.Ctx.Done():
			r.stop()
		default:
		}
	}
}
