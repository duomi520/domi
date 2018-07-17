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
	sc1 := NewSidecar(ctx, "1/server", ":7081", ":9521", testEndpoints)
	go sc1.Run()
	sc2 := NewSidecar(ctx, "2/server", ":7082", ":9522", testEndpoints)
	go sc2.Run()
	sc3 := NewSidecar(ctx, "3/server", ":7083", ":9523", testEndpoints)
	go sc3.Run()
	time.Sleep(150 * time.Millisecond)
	if sc1.State != 2 || sc2.State != 2 || sc3.State != 2 {
		t.Fatal("失败:", sc1.State, sc2.State, sc3.State)
	}
	ctxExitFunc()
	time.Sleep(1500 * time.Millisecond)
	t.Log(sc1.occupySum, sc1.occupy[:5], sc1.sessionsState[:5], sc1.sessions[:5])
	t.Log(sc2.occupySum, sc2.occupy[:5], sc2.sessionsState[:5], sc2.sessions[:5])
	t.Log(sc3.occupySum, sc3.occupy[:5], sc3.sessionsState[:5], sc3.sessions[:5])
	if sc1.Child.GetChildCount() != 0 || sc2.Child.GetChildCount() != 0 || sc3.Child.GetChildCount() != 0 {
		t.Fatal("失败:", sc1.Child.GetChildCount(), sc2.Child.GetChildCount(), sc3.Child.GetChildCount())
	}
}

func Test_runSidecar3(t *testing.T) {
	ctx1, ctxExitFunc1 := context.WithCancel(context.Background())
	ctx2, ctxExitFunc2 := context.WithCancel(context.Background())
	ctx3, ctxExitFunc3 := context.WithCancel(context.Background())
	sc1 := NewSidecar(ctx1, "1/server", ":7081", ":9521", testEndpoints)
	go sc1.Run()
	sc2 := NewSidecar(ctx2, "2/server", ":7082", ":9522", testEndpoints)
	go sc2.Run()
	sc3 := NewSidecar(ctx3, "3/server", ":7083", ":9523", testEndpoints)
	go sc3.Run()
	time.Sleep(150 * time.Millisecond)
	if sc1.State != 2 || sc2.State != 2 || sc3.State != 2 {
		t.Fatal("失败:", sc1.State, sc2.State, sc3.State)
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

func Test_ask1(t *testing.T) {
	ctx1, ctxExitFunc1 := context.WithCancel(context.Background())
	ctx2, ctxExitFunc2 := context.WithCancel(context.Background())
	sc1 := NewSidecar(ctx1, "1/server", ":7081", ":9521", testEndpoints)
	go sc1.Run()
	sc2 := NewSidecar(ctx2, "2/server", ":7082", ":9522", testEndpoints)
	sc2.HandleFunc(transport.FrameTypePing, testPing)
	go sc2.Run()
	time.Sleep(150 * time.Millisecond)
	sc1.AskAppoint(sc2.MachineID, transport.FramePing)
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc1()
	time.Sleep(600 * time.Millisecond)
	ctxExitFunc2()
	time.Sleep(1200 * time.Millisecond)
}
func testPing(s transport.Session) error {
	fmt.Println(s.GetFrameSlice(), string(s.GetFrameSlice().GetData()))
	return nil
}
