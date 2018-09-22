package domi

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"
)

var testEndpoints = []string{"localhost:2379"}

func test2Node() (context.CancelFunc, *Node, *Node) {
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	n1 := NewNode(ctx, nil, "1/server/", ":7081", ":9521", testEndpoints)
	go n1.Run()
	n1.WaitInit()
	n2 := NewNode(ctx, nil, "2/server/", ":7082", ":9522", testEndpoints)
	go n2.Run()
	n2.WaitInit()
	return ctxExitFunc, n1, n2
}
func test4Node() (context.CancelFunc, *Node, *Node, *Node, *Node) {
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	n1 := NewNode(ctx, nil, "1/server/", ":7081", ":9521", testEndpoints)
	go n1.Run()
	n1.WaitInit()
	n2 := NewNode(ctx, nil, "2/server/", ":7082", ":9522", testEndpoints)
	go n2.Run()
	n2.WaitInit()
	n3 := NewNode(ctx, nil, "3/server/", ":7083", ":9523", testEndpoints)
	go n3.Run()
	n3.WaitInit()
	n4 := NewNode(ctx, nil, "4/server/", ":7084", ":9524", testEndpoints)
	go n4.Run()
	n4.WaitInit()
	return ctxExitFunc, n1, n2, n3, n4
}

//请求-响应（request-reply）1 VS N
func Test_ReqRep1(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node()
	n2.Subscribe(56, testRequest)
	n1.Subscribe(57, testReply)
	time.Sleep(500 * time.Millisecond)
	err := n1.Call(56, []byte("Hellow"), 57)
	if err != nil {
		t.Error(err.Error())
	}
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(1000 * time.Millisecond)
}

func Test_ReqRep2(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node()
	n2.Subscribe(56, testRequest)
	n1.Subscribe(57, testReply)
	time.Sleep(500 * time.Millisecond)
	err := n1.Call(56, []byte("Hellow"), 57)
	if err != nil {
		t.Error(err.Error())
	}
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(1000 * time.Millisecond)
}

func testRequest(ctx *ContextMQ) {
	fmt.Println(ctx.sidecar.MachineID, " testRequest:", string(ctx.Request))
	ctx.Reply([]byte("Hi"))
}

func testReply(ctx *ContextMQ) {
	fmt.Println(ctx.sidecar.MachineID, " testReply:", string(ctx.Request))
}

//管道（pipeline） ventilator  worker  sink  1 TO 1 TO 1

func Test_Pipeline1(t *testing.T) {
	ctxExitFunc, n1, n2, n3, n4 := test4Node()
	n2.Subscribe(71, testWorker)
	n3.Subscribe(72, testWorker)
	n4.Subscribe(73, testSink)
	time.Sleep(1000 * time.Millisecond)
	n1.Ventilator([]uint16{71, 72, 73}, []byte("Pipeline "+n1.sidecar.Name))
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(1500 * time.Millisecond)

}
func testWorker(ctx *ContextMQ) {
	fmt.Println(ctx.sidecar.Name, " testWorker:", string(ctx.Request)+","+ctx.sidecar.Name)
	d := make([]byte, len(ctx.Request))
	copy(d, ctx.Request)
	d = append(d, []byte(","+ctx.sidecar.Name)...)
	ctx.Next(d)
}

func testSink(ctx *ContextMQ) {
	fmt.Println(ctx.sidecar.Name, " testSink:", string(ctx.Request)+","+ctx.sidecar.Name)
}

//发布者-订阅者（pubsub publisher-subscriber）1 TO N
func Test_PubSub1(t *testing.T) {
	ctxExitFunc, n1, n2, n3, n4 := test4Node()
	time.Sleep(1000 * time.Millisecond)
	var channel uint16 = 60
	n1.Subscribe(channel, testReply)
	n2.Subscribe(channel, testReply)
	n3.Subscribe(channel, testReply)
	n4.Subscribe(channel, testReply)
	time.Sleep(150 * time.Millisecond)
	n1.Publish(channel, []byte("Broadcast"))
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(1500 * time.Millisecond)
}

func Test_PubSub2(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node()
	var channel uint16 = 61
	time.Sleep(1000 * time.Millisecond)
	n1.Subscribe(channel, testReply)
	n2.Subscribe(channel, testReply)
	time.Sleep(150 * time.Millisecond)
	n1.Publish(channel, []byte("Broadcast1"))
	time.Sleep(150 * time.Millisecond)
	n2.Unsubscribe(channel)
	time.Sleep(150 * time.Millisecond)
	n1.Publish(channel, []byte("Broadcast2"))
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(1500 * time.Millisecond)
}

//总线（bus）   N VS N
func Test_Bus1(t *testing.T) {
	ctxExitFunc, n1, n2, n3, n4 := test4Node()
	time.Sleep(1000 * time.Millisecond)
	var bus uint16 = 70
	n1.Subscribe(bus, testReply)
	n2.Subscribe(bus, testReply)
	n3.Subscribe(bus, testReply)
	n4.Subscribe(bus, testReply)
	time.Sleep(150 * time.Millisecond)
	n1.Publish(bus, []byte("Broadcast1"))
	n2.Publish(bus, []byte("Broadcast2"))
	time.Sleep(100 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(1500 * time.Millisecond)
}

func Test_Bus2(t *testing.T) {
	ctxExitFunc, n1, n2, n3, n4 := test4Node()
	time.Sleep(1000 * time.Millisecond)
	var bus uint16 = 70
	n1.Subscribe(bus, testReply)
	n2.Subscribe(bus, testReply)
	n3.Subscribe(bus, testReply)
	n4.Subscribe(bus, testReply)
	time.Sleep(150 * time.Millisecond)
	n1.Publish(bus, []byte("speech1"))
	time.Sleep(150 * time.Millisecond)
	n1.Unsubscribe(bus)
	time.Sleep(150 * time.Millisecond)
	n3.Publish(bus, []byte("speech2"))
	time.Sleep(150 * time.Millisecond)
	n3.Unsubscribe(bus)
	time.Sleep(150 * time.Millisecond)
	n2.Publish(bus, []byte("speech3"))
	time.Sleep(150 * time.Millisecond)
	n4.Unsubscribe(bus)
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(1500 * time.Millisecond)
}

func Test_WatchChannel(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node()
	cc := make(chan []byte)
	n2.WatchChannel(80, cc)
	time.Sleep(500 * time.Millisecond)
	err := n1.Notify(80, []byte("Hellow"))
	if err != nil {
		t.Fatal(err.Error())
	}
	data := <-cc
	if !bytes.Equal([]byte("Hellow"), data) {
		t.Error("ERR:", data)
	}
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(1500 * time.Millisecond)
}
