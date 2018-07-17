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
	n2.HandleC(56, testRequest)
	time.Sleep(500 * time.Millisecond)
	err := n1.Call(56, n2.sidecar.Name, []byte("Hellow"), testReply)
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

//发布者-订阅者（pubsub publisher-subscriber）1 TO N
func Test_PubSub1(t *testing.T) {
	ctxExitFunc, n1, n2, n3, n4 := test4Node()
	var channel uint16 = 60
	time.Sleep(1000 * time.Millisecond)
	u1, err := n1.Subscribe(channel, n1.sidecar.MachineID, testReply)
	if err != nil {
		t.Error("s1", err)
	}
	u2, err := n2.Subscribe(channel, n1.sidecar.MachineID, testReply)
	if err != nil {
		t.Error("s2", err)
	}
	u3, err := n3.Subscribe(channel, n1.sidecar.MachineID, testReply)
	if err != nil {
		t.Error("s3", err)
	}
	u4, err := n4.Subscribe(channel, n1.sidecar.MachineID, testReply)
	if err != nil {
		t.Error("s4", err)
	}
	time.Sleep(500 * time.Millisecond)
	n1.Publish(channel, []byte("Broadcast"))
	time.Sleep(500 * time.Millisecond)
	if err := n1.Unsubscribe(u1, channel, n1.sidecar.MachineID); err != nil {
		t.Error("u1", err)
	}
	if err := n2.Unsubscribe(u2, channel, n1.sidecar.MachineID); err != nil {
		t.Error("u2", err)
	}
	if err := n3.Unsubscribe(u3, channel, n1.sidecar.MachineID); err != nil {
		t.Error("u3", err)
	}
	if err := n4.Unsubscribe(u4, channel, n1.sidecar.MachineID); err != nil {
		t.Error("u4", err)
	}
	ctxExitFunc()
	time.Sleep(2000 * time.Millisecond)
}

func Test_PubSub2(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node()
	var channel uint16 = 61
	time.Sleep(1000 * time.Millisecond)
	u1, _ := n2.Subscribe(channel, n1.sidecar.MachineID, testReply)
	u2, _ := n2.Subscribe(channel, n1.sidecar.MachineID, testReply)
	u3, _ := n2.Subscribe(channel, n1.sidecar.MachineID, testReply)
	time.Sleep(500 * time.Millisecond)
	n1.Publish(channel, []byte("Broadcast1"))
	time.Sleep(500 * time.Millisecond)
	n2.Unsubscribe(u1, channel, n1.sidecar.MachineID)
	n2.Unsubscribe(u3, channel, n1.sidecar.MachineID)
	time.Sleep(500 * time.Millisecond)
	n1.Publish(channel, []byte("Broadcast2"))
	time.Sleep(500 * time.Millisecond)
	n2.Unsubscribe(u2, channel, n1.sidecar.MachineID)
	ctxExitFunc()
	time.Sleep(2000 * time.Millisecond)
}

//管道（pipeline） ventilator  worker  sink  1 TO 1 TO 1

func Test_Pipeline1(t *testing.T) {
	ctxExitFunc, n1, n2, n3, n4 := test4Node()
	var pipe uint16 = 70
	n2.HandleC(pipe, testWorker)
	n3.HandleC(pipe, testWorker)
	n4.HandleC(pipe, testSink)
	time.Sleep(1000 * time.Millisecond)
	n1.Ventilator(pipe, []string{n2.sidecar.Name, n3.sidecar.Name, n4.sidecar.Name}, []byte("Pipeline "+n1.sidecar.Name))
	time.Sleep(500 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(2000 * time.Millisecond)

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

//总线（bus） TODO  N VS N
func Test_Bus1(t *testing.T) {
	ctxExitFunc, n1, n2, n3, n4 := test4Node()
	time.Sleep(1000 * time.Millisecond)
	var bus uint16 = 70
	u1, err := n1.JoinBus(bus, testReply)
	if err != nil {
		t.Error(err)
	}
	u2, err := n2.JoinBus(bus, testReply)
	if err != nil {
		t.Error(err)
	}
	u3, err := n3.JoinBus(bus, testReply)
	if err != nil {
		t.Error(err)
	}
	u4, err := n4.JoinBus(bus, testReply)
	if err != nil {
		t.Error(err)
	}
	time.Sleep(500 * time.Millisecond)
	t.Log(n1.channelNodeMap[70], n2.channelNodeMap[70], n3.channelNodeMap[70], n4.channelNodeMap[70])
	if err := n1.LeaveBus(bus, u1); err != nil {
		t.Error(err)
	}
	if err := n2.LeaveBus(bus, u2); err != nil {
		t.Error(err)
	}
	if err := n3.LeaveBus(bus, u3); err != nil {
		t.Error(err)
	}
	if err := n4.LeaveBus(bus, u4); err != nil {
		t.Error(err)
	}
	time.Sleep(500 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(2000 * time.Millisecond)
}
func Test_Bus2(t *testing.T) {
	ctxExitFunc, n1, n2, n3, n4 := test4Node()
	time.Sleep(1000 * time.Millisecond)
	var bus uint16 = 70
	u1, err := n1.JoinBus(bus, testReply)
	if err != nil {
		t.Error(err)
	}
	u2, err := n2.JoinBus(bus, testReply)
	if err != nil {
		t.Error(err)
	}
	u3, err := n3.JoinBus(bus, testReply)
	if err != nil {
		t.Error(err)
	}
	u4, err := n4.JoinBus(bus, testReply)
	if err != nil {
		t.Error(err)
	}
	time.Sleep(500 * time.Millisecond)
	n1.Publish(bus, []byte("speech1"))
	time.Sleep(500 * time.Millisecond)
	if err := n1.LeaveBus(bus, u1); err != nil {
		t.Error(err)
	}
	if err := n2.LeaveBus(bus, u2); err != nil {
		t.Error(err)
	}
	time.Sleep(500 * time.Millisecond)
	n3.Publish(bus, []byte("speech2"))
	time.Sleep(500 * time.Millisecond)
	if err := n3.LeaveBus(bus, u3); err != nil {
		t.Error(err)
	}
	n2.Publish(bus, []byte("speech3"))
	time.Sleep(500 * time.Millisecond)
	if err := n4.LeaveBus(bus, u4); err != nil {
		t.Error(err)
	}
	time.Sleep(500 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(2000 * time.Millisecond)
}
