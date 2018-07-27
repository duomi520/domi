package sidecar

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/duomi520/domi/transport"
)

//需先启动 etcd

func Test_newSidecar(t *testing.T) {
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	sc1 := NewSidecar(ctx, nil, "1/server", ":7081", ":9521", testEndpoints)
	go sc1.Run()
	sc1.WaitInit()
	sc2 := NewSidecar(ctx, nil, "2/server", ":7082", ":9522", testEndpoints)
	go sc2.Run()
	sc2.WaitInit()
	sc3 := NewSidecar(ctx, nil, "3/server", ":7083", ":9523", testEndpoints)
	go sc3.Run()
	sc3.WaitInit()
	time.Sleep(250 * time.Millisecond)
	if sc1.state != 2 || sc2.state != 2 || sc3.state != 2 {
		t.Fatal("失败:", sc1.state, sc2.state, sc3.state)
	}
	ctxExitFunc()
	time.Sleep(1500 * time.Millisecond)
	t.Log(sc1.state, sc1.sessionsState[:5], sc1.sessions[:5])
	t.Log(sc2.state, sc2.sessionsState[:5], sc2.sessions[:5])
	t.Log(sc3.state, sc3.sessionsState[:5], sc3.sessions[:5])
	if sc1.Child.GetChildCount() != 0 || sc2.Child.GetChildCount() != 0 || sc3.Child.GetChildCount() != 0 {
		t.Fatal("失败:", sc1.Child.GetChildCount(), sc2.Child.GetChildCount(), sc3.Child.GetChildCount())
	}
}

func Test_runSidecar3(t *testing.T) {
	ctx1, ctxExitFunc1 := context.WithCancel(context.Background())
	ctx2, ctxExitFunc2 := context.WithCancel(context.Background())
	ctx3, ctxExitFunc3 := context.WithCancel(context.Background())
	sc1 := NewSidecar(ctx1, nil, "1/server", ":7081", ":9521", testEndpoints)
	go sc1.Run()
	sc1.WaitInit()
	sc2 := NewSidecar(ctx2, nil, "2/server", ":7082", ":9522", testEndpoints)
	go sc2.Run()
	sc2.WaitInit()
	sc3 := NewSidecar(ctx3, nil, "3/server", ":7083", ":9523", testEndpoints)
	go sc3.Run()
	sc3.WaitInit()
	time.Sleep(150 * time.Millisecond)
	if sc1.state != 2 || sc2.state != 2 || sc3.state != 2 {
		t.Fatal("失败:", sc1.state, sc2.state, sc3.state)
	}
	ctxExitFunc1()
	time.Sleep(600 * time.Millisecond)
	ctxExitFunc2()
	time.Sleep(600 * time.Millisecond)
	ctxExitFunc3()
	time.Sleep(600 * time.Millisecond)
	if sc1.Child.GetChildCount() != 0 || sc2.Child.GetChildCount() != 0 || sc3.Child.GetChildCount() != 0 {
		t.Fatal("失败:", sc1.Child.GetChildCount(), sc2.Child.GetChildCount(), sc3.Child.GetChildCount())
	}
}

func Test_runSidecar4(t *testing.T) {
	buf := [8]byte{8, 0, 0, 0, 0, 0, 55, 0}
	fs := transport.DecodeByBytes(buf[:8])
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	sc1 := NewSidecar(ctx, nil, "1/server", ":7081", ":9521", testEndpoints)
	go sc1.Run()
	sc1.WaitInit()
	sc2 := NewSidecar(ctx, nil, "2/server", ":7082", ":9522", testEndpoints)
	go sc2.Run()
	sc2.WaitInit()
	sc3 := NewSidecar(ctx, nil, "3/server", ":7083", ":9523", testEndpoints)
	go sc3.Run()
	sc3.WaitInit()
	sc4 := NewSidecar(ctx, nil, "4/server", ":7084", ":9524", testEndpoints)
	go sc4.Run()
	sc4.WaitInit()
	time.Sleep(150 * time.Millisecond)
	if sc1.state != 2 || sc2.state != 2 || sc3.state != 2 || sc4.state != 2 {
		t.Fatal("失败:", sc1.state, sc2.state, sc3.state, sc4.state)
	}
	sc1.SetChannel(sc1.machineID, 55, 3)
	sc1.HandleFunc(55, testPing)
	sc2.SetChannel(sc2.machineID, 55, 3)
	sc2.HandleFunc(55, testPing)
	sc3.SetChannel(sc3.machineID, 55, 3)
	sc3.HandleFunc(55, testPing)
	sc4.SetChannel(sc4.machineID, 55, 3)
	sc4.HandleFunc(55, testPing)
	time.Sleep(600 * time.Millisecond)
	for i := 0; i < 10; i++ {
		err := sc1.AskOne(55, fs)
		if err != nil {
			t.Fatal(err)
		}
		cc, _ := getCursorAndLenght((*bucket)(sc1.channels[55]).cursorAndLenght)
		fmt.Println(cc)
	}
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(1200 * time.Millisecond)
	if sc1.Child.GetChildCount() != 0 || sc2.Child.GetChildCount() != 0 || sc3.Child.GetChildCount() != 0 || sc4.Child.GetChildCount() != 0 {
		t.Fatal("失败:", sc1.Child.GetChildCount(), sc2.Child.GetChildCount(), sc3.Child.GetChildCount(), sc4.Child.GetChildCount())
	}
}

func Test_ask1(t *testing.T) {
	ctx1, ctxExitFunc1 := context.WithCancel(context.Background())
	ctx2, ctxExitFunc2 := context.WithCancel(context.Background())
	sc1 := NewSidecar(ctx1, nil, "1/server", ":7081", ":9521", testEndpoints)
	go sc1.Run()
	sc1.WaitInit()
	sc2 := NewSidecar(ctx2, nil, "2/server", ":7082", ":9522", testEndpoints)
	sc2.HandleFunc(transport.FrameTypePing, testPing)
	go sc2.Run()
	sc2.WaitInit()
	sc2.SetChannel(sc2.machineID, transport.FrameTypePing, 3)
	time.Sleep(150 * time.Millisecond)
	err := sc1.AskOne(transport.FrameTypePing, transport.FramePing)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc1()
	time.Sleep(200 * time.Millisecond)
	ctxExitFunc2()
	time.Sleep(1200 * time.Millisecond)
}
func testPing(s transport.Session) error {
	fmt.Println(s.GetFrameSlice(), string(s.GetFrameSlice().GetData()))
	return nil
}

func Test_conver(t *testing.T) {
	var c, l uint32
	c = 11
	l = 22
	d := setCursorAndLenght(c, l)
	d++
	tc, tl := getCursorAndLenght(d)
	if tc != 12 || tl != 22 || setCursorAndLenght(0, 0) != 0 {
		t.Fatal("失败:", c, l, d, tc, tl, setCursorAndLenght(0, 0))
	}
}
