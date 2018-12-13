package domi

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var testEndpoints = []string{"localhost:2379"}

var testNodeTable []string

func testTableVerification(t *testing.T, s []string) {
	defer func() { testNodeTable = nil }()
	if len(testNodeTable) != len(s) {
		t.Fatal(testNodeTable, s)
	}
	for i, v := range s {
		if !strings.EqualFold(testNodeTable[i], v) {
			t.Fatal(s, v)
		}
	}
}

func testTableVerificationDisorder(t *testing.T, s []string) {
	defer func() { testNodeTable = nil }()
	if len(s) != len(testNodeTable) {
		for _, v := range testNodeTable {
			t.Error(v)
		}
		return
	}
	for _, v := range testNodeTable {
		j := 0
	L:
		for j < len(s) {
			if strings.EqualFold(v, s[j]) {
				if j != (len(s) - 1) {
					copy(s[j:], s[j+1:])
				}
				s = s[:len(s)-1]
				break L
			}
			j++
		}
	}
	if len(s) > 0 {
		for _, v := range testNodeTable {
			t.Error(v)
		}
	}
}

func test2Node(port int) (context.CancelFunc, *Node, *Node) {
	p1 := strconv.Itoa(port)
	p2 := strconv.Itoa(port + 1)
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	n1 := &Node{
		Ctx:       ctx,
		Name:      "1/server/",
		HTTPPort:  ":7" + p1,
		TCPPort:   ":9" + p1,
		Endpoints: testEndpoints,
	}
	n1.Init()
	go n1.Run()
	n1.WaitInit()
	n2 := &Node{
		Ctx:       ctx,
		Name:      "2/server/",
		HTTPPort:  ":7" + p2,
		TCPPort:   ":9" + p2,
		Endpoints: testEndpoints,
	}
	n2.Init()
	go n2.Run()
	n2.WaitInit()
	time.Sleep(250 * time.Millisecond)
	return ctxExitFunc, n1, n2
}
func test4Node(port int) (context.CancelFunc, *Node, *Node, *Node, *Node) {
	p1 := strconv.Itoa(port)
	p2 := strconv.Itoa(port + 1)
	p3 := strconv.Itoa(port + 2)
	p4 := strconv.Itoa(port + 3)
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	n1 := &Node{
		Ctx:       ctx,
		Name:      "1/server/",
		HTTPPort:  ":7" + p1,
		TCPPort:   ":9" + p1,
		Endpoints: testEndpoints,
	}
	n1.Init()
	go n1.Run()
	n1.WaitInit()
	n2 := &Node{
		Ctx:       ctx,
		Name:      "2/server/",
		HTTPPort:  ":7" + p2,
		TCPPort:   ":9" + p2,
		Endpoints: testEndpoints,
	}
	n2.Init()
	go n2.Run()
	n2.WaitInit()
	n3 := &Node{
		Ctx:       ctx,
		Name:      "3/server/",
		HTTPPort:  ":7" + p3,
		TCPPort:   ":9" + p3,
		Endpoints: testEndpoints,
	}
	n3.Init()
	go n3.Run()
	n3.WaitInit()
	n4 := &Node{
		Ctx:       ctx,
		Name:      "4/server/",
		HTTPPort:  ":7" + p4,
		TCPPort:   ":9" + p4,
		Endpoints: testEndpoints,
	}
	n4.Init()
	go n4.Run()
	n4.WaitInit()
	time.Sleep(250 * time.Millisecond)
	return ctxExitFunc, n1, n2, n3, n4
}

//请求-响应（request-reply）1 VS N
func Test_ReqRep1(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node(100)
	n2.Subscribe(56, testRequest)
	n1.Subscribe(57, testReply)
	time.Sleep(500 * time.Millisecond)
	err := n1.Call(56, []byte("Hellow"), 57)
	if err != nil {
		t.Error(err.Error())
	}
	time.Sleep(50 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(50 * time.Millisecond)
	testTableVerification(t, []string{
		"1 testRequest:Hellow",
		"0 testReply:Hi",
	})
}

func Test_ReqRep2(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node(110)
	n2.Subscribe(56, testRequest)
	n1.Subscribe(57, testReply)
	time.Sleep(500 * time.Millisecond)
	err := n1.Call(56, []byte("Hellow"), 57)
	if err != nil {
		t.Error(err.Error())
	}
	time.Sleep(50 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(50 * time.Millisecond)
	testTableVerification(t, []string{
		"1 testRequest:Hellow",
		"0 testReply:Hi",
	})
}

func testRequest(ctx *ContextMQ) {
	text := strconv.Itoa(ctx.sidecar.MachineID) + " testRequest:" + string(ctx.Request)
	ctx.Reply([]byte("Hi"))
	testNodeTable = append(testNodeTable, text)
}

func testReply(ctx *ContextMQ) {
	text := strconv.Itoa(ctx.sidecar.MachineID) + " testReply:" + string(ctx.Request)
	testNodeTable = append(testNodeTable, text)
}

//管道（pipeline） ventilator  worker  sink  1 TO 1 TO 1

var pipelineWg sync.WaitGroup

func Test_Pipeline1(t *testing.T) {
	pipelineWg.Add(1)
	ctxExitFunc, n1, n2, n3, n4 := test4Node(120)
	n2.Subscribe(71, testWorker)
	n3.Subscribe(72, testWorker)
	n4.Subscribe(73, testSink)
	time.Sleep(500 * time.Millisecond)
	n1.Ventilator([]uint16{71, 72, 73}, []byte("Pipeline "+n1.sidecar.Name))
	time.Sleep(50 * time.Millisecond)
	ctxExitFunc()
	pipelineWg.Wait()
	testTableVerification(t, []string{
		"2/server/ testWorker:Pipeline 1/server/",
		"3/server/ testWorker:Pipeline 1/server/->2/server/",
		"4/server/ testSink:Pipeline 1/server/->2/server/->3/server/",
	})
}
func testWorker(ctx *ContextMQ) {
	text := ctx.sidecar.Name + " testWorker:" + string(ctx.Request)
	testNodeTable = append(testNodeTable, text)
	d := make([]byte, len(ctx.Request))
	copy(d, ctx.Request)
	d = append(d, []byte("->"+ctx.sidecar.Name)...)
	ctx.Next(d)
}

func testSink(ctx *ContextMQ) {
	text := ctx.sidecar.Name + " testSink:" + string(ctx.Request)
	testNodeTable = append(testNodeTable, text)
	pipelineWg.Done()
}

//发布者-订阅者（pubsub publisher-subscriber）1 TO N
func Test_PubSub1(t *testing.T) {
	ctxExitFunc, n1, n2, n3, n4 := test4Node(130)
	var channel uint16 = 60
	n1.Subscribe(channel, testReply)
	n2.Subscribe(channel, testReply)
	n3.Subscribe(channel, testReply)
	n4.Subscribe(channel, testReply)
	time.Sleep(1000 * time.Millisecond)
	n1.Publish(channel, []byte("Broadcast"))
	time.Sleep(50 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(50 * time.Millisecond)
	testTableVerificationDisorder(t, []string{
		"0 testReply:Broadcast",
		"1 testReply:Broadcast",
		"2 testReply:Broadcast",
		"3 testReply:Broadcast",
	})
}

func Test_PubSub2(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node(140)
	var channel uint16 = 61
	n1.Subscribe(channel, testReply)
	n2.Subscribe(channel, testReply)
	time.Sleep(1000 * time.Millisecond)
	n1.Publish(channel, []byte("Broadcast1"))
	time.Sleep(50 * time.Millisecond)
	n2.Unsubscribe(channel)
	time.Sleep(1000 * time.Millisecond)
	n1.Publish(channel, []byte("Broadcast2"))
	time.Sleep(50 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(50 * time.Millisecond)
	testTableVerificationDisorder(t, []string{
		"1 testReply:Broadcast1",
		"0 testReply:Broadcast1",
		"0 testReply:Broadcast2",
	})
}

//总线（bus）   N VS N
func Test_Bus1(t *testing.T) {
	ctxExitFunc, n1, n2, n3, n4 := test4Node(150)
	var bus uint16 = 80
	n1.Subscribe(bus, testReply)
	n2.Subscribe(bus, testReply)
	n3.Subscribe(bus, testReply)
	n4.Subscribe(bus, testReply)
	time.Sleep(1000 * time.Millisecond)
	n1.Publish(bus, []byte("Broadcast1"))
	n2.Publish(bus, []byte("Broadcast2"))
	time.Sleep(50 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(50 * time.Millisecond)
	testTableVerificationDisorder(t, []string{
		"0 testReply:Broadcast1",
		"1 testReply:Broadcast1",
		"2 testReply:Broadcast1",
		"3 testReply:Broadcast1",
		"0 testReply:Broadcast2",
		"1 testReply:Broadcast2",
		"2 testReply:Broadcast2",
		"3 testReply:Broadcast2",
	})
}

func Test_Bus2(t *testing.T) {
	ctxExitFunc, n1, n2, n3, n4 := test4Node(160)
	var bus uint16 = 81
	n1.Subscribe(bus, testReply)
	n2.Subscribe(bus, testReply)
	n3.Subscribe(bus, testReply)
	n4.Subscribe(bus, testReply)
	time.Sleep(1000 * time.Millisecond)
	n1.Publish(bus, []byte("speech1"))
	time.Sleep(50 * time.Millisecond)
	n1.Unsubscribe(bus)
	time.Sleep(1000 * time.Millisecond)
	n3.Publish(bus, []byte("speech2"))
	time.Sleep(50 * time.Millisecond)
	n3.Unsubscribe(bus)
	time.Sleep(1000 * time.Millisecond)
	n2.Publish(bus, []byte("speech3"))
	time.Sleep(50 * time.Millisecond)
	n4.Unsubscribe(bus)
	time.Sleep(1000 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(50 * time.Millisecond)
	testTableVerificationDisorder(t, []string{
		"0 testReply:speech1",
		"1 testReply:speech1",
		"2 testReply:speech1",
		"3 testReply:speech1",
		"1 testReply:speech2",
		"2 testReply:speech2",
		"3 testReply:speech2",
		"1 testReply:speech3",
		"3 testReply:speech3",
	})
}

func Test_WatchChannel(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node(170)
	cc := make(chan []byte)
	n2.WatchChannel(90, cc)
	time.Sleep(500 * time.Millisecond)
	err := n1.Notify(90, []byte("Hellow"))
	if err != nil {
		t.Fatal(err.Error())
	}
	data := <-cc
	if !bytes.Equal([]byte("Hellow"), data) {
		t.Error("ERR:", data)
	}
	time.Sleep(50 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(50 * time.Millisecond)
}
