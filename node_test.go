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
	n1.Call(56, n2.sidecar.FullName, []byte("Hellow"), 59, testReply)
	time.Sleep(500 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(500 * time.Millisecond)
}

func testRequest(ctx *ContextMQ) {
	fmt.Println(ctx.N.sidecar.FullName, " testRequest:", string(ctx.Request))
	ctx.Reply([]byte("Hi"))
}

func testReply(ctx *ContextMQ) {
	fmt.Println(ctx.N.sidecar.FullName, " testReply:", string(ctx.Request))
}

//发布者-订阅者（pubsub publisher-subscriber）1 TO N
func Test_PubSub1(t *testing.T) {
	ctxExitFunc, n1, n2, n3, n4 := test4Node()
	go n1.RunChannel()
	localNode = n1
	var channel uint16 = 60
	time.Sleep(500 * time.Millisecond)
	n2.Subscribe(channel, n1.sidecar.FullName, 65, testReply)
	n3.Subscribe(channel, n1.sidecar.FullName, 66, testReply)
	n4.Subscribe(channel, n1.sidecar.FullName, 67, testReply)
	time.Sleep(500 * time.Millisecond)
	n1.Publish(channel, []byte("Broadcast"))
	time.Sleep(500 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(500 * time.Millisecond)
}

func Test_PubSub2(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node()
	go n1.RunChannel()
	localNode = n1
	var channel uint16 = 61
	time.Sleep(500 * time.Millisecond)
	n2.Subscribe(channel, n1.sidecar.FullName, 62, testReply)
	n2.Subscribe(channel, n1.sidecar.FullName, 63, testReply)
	n2.Subscribe(channel, n1.sidecar.FullName, 64, testReply)
	time.Sleep(500 * time.Millisecond)
	n1.Publish(channel, []byte("Broadcast1"))
	time.Sleep(500 * time.Millisecond)
	n2.Unsubscribe(62, channel, n1.sidecar.FullName)
	n2.Unsubscribe(64, channel, n1.sidecar.FullName)
	time.Sleep(500 * time.Millisecond)
	n1.Publish(channel, []byte("Broadcast2"))
	time.Sleep(500 * time.Millisecond)
	n2.Unsubscribe(63, channel, n1.sidecar.FullName)
	ctxExitFunc()
	time.Sleep(500 * time.Millisecond)
}

//管道（pipeline） ventilator  worker  sink  1 TO 1
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
	fmt.Println(ctx.N.sidecar.FullName, " testWorker:", string(ctx.Request)+","+ctx.N.sidecar.FullName)
	d := make([]byte, len(ctx.Request))
	copy(d, ctx.Request)
	d = append(d, []byte(","+ctx.N.sidecar.FullName)...)
	ctx.Next(d)
}
func testSink(ctx *ContextMQ) {
	fmt.Println(ctx.N.sidecar.FullName, " testSink:", string(ctx.Request)+","+ctx.N.sidecar.FullName)
}

//总线（bus） TODO  N VS N
func Test_Bus(t *testing.T) {
}
