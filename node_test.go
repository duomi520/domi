package domi

import (
	"context"
	"fmt"
	"testing"
	"time"
)

var testEndpoints = []string{"localhost:2379"}

func test2Node() (context.CancelFunc, *Node, *Node) {
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	n1 := NewNode(ctx, "1/server/", ":7081", ":9521", testEndpoints)
	go n1.Run()
	n2 := NewNode(ctx, "2/server/", ":7082", ":9522", testEndpoints)
	go n2.Run()
	return ctxExitFunc, n1, n2
}
func test4Node() (context.CancelFunc, *Node, *Node, *Node, *Node) {
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	n1 := NewNode(ctx, "1/server/", ":7081", ":9521", testEndpoints)
	go n1.Run()
	n2 := NewNode(ctx, "2/server/", ":7082", ":9522", testEndpoints)
	go n2.Run()
	n3 := NewNode(ctx, "3/server/", ":7083", ":9523", testEndpoints)
	go n3.Run()
	n4 := NewNode(ctx, "4/server/", ":7084", ":9524", testEndpoints)
	go n4.Run()
	return ctxExitFunc, n1, n2, n3, n4
}

//单向告知		1 TO 1
func Test_Tell1(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node()
	n2.HandleC(50, testTell)
	time.Sleep(500 * time.Millisecond)
	n1.Tell(50, n2.sidecar.FullName, []byte("Hellow 1"))
	n1.Tell(50, n2.sidecar.FullName, []byte("Hellow 2"))
	n1.Tell(50, n2.sidecar.FullName, []byte("Hellow 3"))
	n1.Tell(50, n2.sidecar.FullName, []byte("Hellow 4"))
	time.Sleep(500 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(500 * time.Millisecond)
}
func testTell(ctx *ContextMQ) {
	fmt.Println("testTell:", string(ctx.Request))
}

//请求-响应（request-reply）1 VS N
func Test_ReqRep(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node()
	n2.HandleC(56, testRequest)
	localNode = n2
	time.Sleep(500 * time.Millisecond)
	n1.Call(56, n2.sidecar.FullName, []byte("Hellow"), testReply)
	time.Sleep(500 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(500 * time.Millisecond)
}

func testRequest(ctx *ContextMQ) {
	fmt.Println(ctx.sidecar.FullName, " testRequest:", string(ctx.Request))
	ctx.Reply([]byte("Hi"))
}

func testReply(ctx *ContextMQ) {
	fmt.Println(ctx.sidecar.FullName, " testReply:", string(ctx.Request))
}

//发布者-订阅者（pubsub publisher-subscriber）1 TO N
func Test_PubSub1(t *testing.T) {
	ctxExitFunc, n1, n2, n3, n4 := test4Node()
	localNode = n1
	var channel uint16 = 60
	time.Sleep(500 * time.Millisecond)
	n1.Subscribe(channel, n1.sidecar.FullName, testReply)
	n2.Subscribe(channel, n1.sidecar.FullName, testReply)
	n3.Subscribe(channel, n1.sidecar.FullName, testReply)
	n4.Subscribe(channel, n1.sidecar.FullName, testReply)
	time.Sleep(500 * time.Millisecond)
	n1.Publish(channel, []byte("Broadcast"))
	time.Sleep(500 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(500 * time.Millisecond)
}

func Test_PubSub2(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node()
	localNode = n1
	var channel uint16 = 61
	time.Sleep(500 * time.Millisecond)
	u1, _ := n2.Subscribe(channel, n1.sidecar.FullName, testReply)
	u2, _ := n2.Subscribe(channel, n1.sidecar.FullName, testReply)
	u3, _ := n2.Subscribe(channel, n1.sidecar.FullName, testReply)
	time.Sleep(500 * time.Millisecond)
	n1.Publish(channel, []byte("Broadcast1"))
	time.Sleep(500 * time.Millisecond)
	n2.Unsubscribe(u1, channel, n1.sidecar.FullName)
	n2.Unsubscribe(u3, channel, n1.sidecar.FullName)
	time.Sleep(500 * time.Millisecond)
	n1.Publish(channel, []byte("Broadcast2"))
	time.Sleep(500 * time.Millisecond)
	n2.Unsubscribe(u2, channel, n1.sidecar.FullName)
	ctxExitFunc()
	time.Sleep(500 * time.Millisecond)
}

//管道（pipeline） ventilator  worker  sink  1 TO 1 TO 1
func Test_Pipeline1(t *testing.T) {
	ctxExitFunc, n1, n2, n3, n4 := test4Node()
	var pipe uint16 = 70
	n2.HandleC(pipe, testWorker)
	n3.HandleC(pipe, testWorker)
	n4.HandleC(pipe, testSink)
	time.Sleep(500 * time.Millisecond)
	n1.Ventilator(pipe, []string{n2.sidecar.FullName, n3.sidecar.FullName, n4.sidecar.FullName}, []byte("Pipeline "+n1.sidecar.FullName))
	time.Sleep(500 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(500 * time.Millisecond)

}
func testWorker(ctx *ContextMQ) {
	fmt.Println(ctx.sidecar.FullName, " testWorker:", string(ctx.Request)+","+ctx.sidecar.FullName)
	d := make([]byte, len(ctx.Request))
	copy(d, ctx.Request)
	d = append(d, []byte(","+ctx.sidecar.FullName)...)
	ctx.Next(d)
}
func testSink(ctx *ContextMQ) {
	fmt.Println(ctx.sidecar.FullName, " testSink:", string(ctx.Request)+","+ctx.sidecar.FullName)
}

//总线（bus） TODO  N VS N
func Test_Bus1(t *testing.T) {
	ctxExitFunc, n1, n2, n3, n4 := test4Node()
	time.Sleep(500 * time.Millisecond)
	var bus uint16 = 70
	u1, _ := n1.JoinBus(bus, testReply)
	u2, _ := n2.JoinBus(bus, testReply)
	u3, _ := n3.JoinBus(bus, testReply)
	u4, _ := n4.JoinBus(bus, testReply)
	time.Sleep(500 * time.Millisecond)
	fmt.Println("dd", n1.channelNodeMap[70].list, n2.channelNodeMap[70].list, n3.channelNodeMap[70].list, n4.channelNodeMap[70].list)
	n1.LeaveBus(bus, u1)
	n2.LeaveBus(bus, u2)
	n3.LeaveBus(bus, u3)
	n4.LeaveBus(bus, u4)
	time.Sleep(500 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(500 * time.Millisecond)
}
func Test_Bus2(t *testing.T) {
	ctxExitFunc, n1, n2, n3, n4 := test4Node()
	time.Sleep(500 * time.Millisecond)
	var bus uint16 = 70
	u1, _ := n1.JoinBus(bus, testReply)
	u2, _ := n2.JoinBus(bus, testReply)
	u3, _ := n3.JoinBus(bus, testReply)
	u4, _ := n4.JoinBus(bus, testReply)
	time.Sleep(500 * time.Millisecond)
	n1.Publish(bus, []byte("speech1"))
	time.Sleep(500 * time.Millisecond)
	n1.LeaveBus(bus, u1)
	n2.LeaveBus(bus, u2)
	time.Sleep(500 * time.Millisecond)
	n3.Publish(bus, []byte("speech2"))
	time.Sleep(500 * time.Millisecond)
	n3.LeaveBus(bus, u3)
	n2.Publish(bus, []byte("speech3"))
	time.Sleep(500 * time.Millisecond)
	n4.LeaveBus(bus, u4)
	time.Sleep(500 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(500 * time.Millisecond)
}
