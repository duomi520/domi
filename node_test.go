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

//请求-响应（request-reply）1 VS N
func Test_ReqRep1(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node()
	n2.SerialProcess(56, testRequest)
	n1.SerialProcess(57, testReply)
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
	n2.SerialProcess(71, testWorker)
	n3.SerialProcess(72, testWorker)
	n4.SerialProcess(73, testSink)
	time.Sleep(1000 * time.Millisecond)
	n1.Ventilator([]uint16{71, 72, 73}, []byte("Pipeline "+n1.sidecar.Name))
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(1000 * time.Millisecond)

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
	n1.SerialProcess(channel, testReply)
	n2.SerialProcess(channel, testReply)
	n3.SerialProcess(channel, testReply)
	n4.SerialProcess(channel, testReply)
	time.Sleep(150 * time.Millisecond)
	n1.Publish(channel, []byte("Broadcast"))
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(1000 * time.Millisecond)
}

func Test_PubSub2(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node()
	var channel uint16 = 61
	time.Sleep(1000 * time.Millisecond)
	n1.SerialProcess(channel, testReply)
	n2.SerialProcess(channel, testReply)
	time.Sleep(150 * time.Millisecond)
	n1.Publish(channel, []byte("Broadcast1"))
	time.Sleep(150 * time.Millisecond)
	n2.Unsubscribe(channel)
	time.Sleep(150 * time.Millisecond)
	n1.Publish(channel, []byte("Broadcast2"))
	time.Sleep(150 * time.Millisecond)
	n1.Unsubscribe(channel)
	ctxExitFunc()
	time.Sleep(1000 * time.Millisecond)
}

//总线（bus） TODO  N VS N
func Test_Bus1(t *testing.T) {
	ctxExitFunc, n1, n2, n3, n4 := test4Node()
	time.Sleep(1000 * time.Millisecond)
	var bus uint16 = 70
	n1.SerialProcess(bus, testReply)
	n2.SerialProcess(bus, testReply)
	n3.SerialProcess(bus, testReply)
	n4.SerialProcess(bus, testReply)
	time.Sleep(150 * time.Millisecond)
	n1.Publish(bus, []byte("Broadcast1"))
	n2.Publish(bus, []byte("Broadcast2"))
	time.Sleep(100 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(1000 * time.Millisecond)
}

func Test_Bus2(t *testing.T) {
	ctxExitFunc, n1, n2, n3, n4 := test4Node()
	time.Sleep(1000 * time.Millisecond)
	var bus uint16 = 70
	n1.SerialProcess(bus, testReply)
	n2.SerialProcess(bus, testReply)
	n3.SerialProcess(bus, testReply)
	n4.SerialProcess(bus, testReply)
	time.Sleep(150 * time.Millisecond)
	n1.Publish(bus, []byte("speech1"))
	time.Sleep(150 * time.Millisecond)
	n1.Unsubscribe(bus)
	time.Sleep(150 * time.Millisecond)
	n3.Publish(bus, []byte("speech2"))
	time.Sleep(150 * time.Millisecond)
	n3.Unsubscribe(bus)
	n2.Publish(bus, []byte("speech3"))
	time.Sleep(150 * time.Millisecond)
	n4.Unsubscribe(bus)
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(1000 * time.Millisecond)
}
