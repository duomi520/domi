package sidecar

import (
	"context"
	"fmt"
	"github.com/duomi520/domi/transport"
	"strconv"
	"testing"
	"time"
)

//需先启动 etcd
func test4Sidecar(port int) (*Sidecar, *Sidecar, *Sidecar, *Sidecar) {
	p1 := strconv.Itoa(port)
	p2 := strconv.Itoa(port + 1)
	p3 := strconv.Itoa(port + 2)
	p4 := strconv.Itoa(port + 3)
	ctx1, ctxExitFunc1 := context.WithCancel(context.Background())
	ctx2, ctxExitFunc2 := context.WithCancel(context.Background())
	ctx3, ctxExitFunc3 := context.WithCancel(context.Background())
	ctx4, ctxExitFunc4 := context.WithCancel(context.Background())
	sc1 := NewSidecar(ctx1, ctxExitFunc1, "1/server", ":7"+p1, ":9"+p1, testEndpoints, nil, nil)
	go sc1.Run()
	sc1.WaitInit()
	sc2 := NewSidecar(ctx2, ctxExitFunc2, "2/server", ":7"+p2, ":9"+p2, testEndpoints, nil, nil)
	go sc2.Run()
	sc2.WaitInit()
	sc3 := NewSidecar(ctx3, ctxExitFunc3, "3/server", ":7"+p3, ":9"+p3, testEndpoints, nil, nil)
	go sc3.Run()
	sc3.WaitInit()
	sc4 := NewSidecar(ctx4, ctxExitFunc4, "4/server", ":7"+p4, ":9"+p4, testEndpoints, nil, nil)
	go sc4.Run()
	sc4.WaitInit()
	return sc1, sc2, sc3, sc4
}
func Test_newSidecar(t *testing.T) {
	sc1, sc2, sc3, _ := test4Sidecar(100)
	time.Sleep(150 * time.Millisecond)
	if sc1.state != 2 || sc2.state != 2 || sc3.state != 2 {
		t.Fatal("失败:", sc1.state, sc2.state, sc3.state)
	}
	sc1.exitFunc()
	sc2.exitFunc()
	sc3.exitFunc()
	time.Sleep(600 * time.Millisecond)
	t.Log(sc1.state, sc1.sessions[:5])
	t.Log(sc2.state, sc2.sessions[:5])
	t.Log(sc3.state, sc3.sessions[:5])
	if sc1.Child.GetChildCount() != 0 || sc2.Child.GetChildCount() != 0 || sc3.Child.GetChildCount() != 0 {
		t.Fatal("失败:", sc1.Child.GetChildCount(), sc2.Child.GetChildCount(), sc3.Child.GetChildCount())
	}
}

func Test_runSidecar3(t *testing.T) {
	sc1, sc2, sc3, _ := test4Sidecar(110)
	time.Sleep(150 * time.Millisecond)
	if sc1.state != 2 || sc2.state != 2 || sc3.state != 2 {
		t.Fatal("失败:", sc1.state, sc2.state, sc3.state)
	}
	sc1.exitFunc()
	time.Sleep(600 * time.Millisecond)
	sc2.exitFunc()
	time.Sleep(600 * time.Millisecond)
	sc3.exitFunc()
	time.Sleep(600 * time.Millisecond)
	if sc1.Child.GetChildCount() != 0 || sc2.Child.GetChildCount() != 0 || sc3.Child.GetChildCount() != 0 {
		t.Fatal("失败:", sc1.Child.GetChildCount(), sc2.Child.GetChildCount(), sc3.Child.GetChildCount())
	}
}

func Test_runSidecar4(t *testing.T) {
	buf := [8]byte{8, 0, 0, 0, 0, 0, 55, 0}
	fs := transport.DecodeByBytes(buf[:8])
	sc1, sc2, sc3, sc4 := test4Sidecar(120)
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
		sc1.AskOne(55, fs, func(err error) {
			t.Fatal(err)
		})
		cc, _ := getCursorAndLenght((*bucket)(sc1.channels[55]).cursorAndLenght)
		fmt.Println(cc)
	}
	time.Sleep(150 * time.Millisecond)
	sc1.exitFunc()
	sc2.exitFunc()
	sc3.exitFunc()
	sc4.exitFunc()
	time.Sleep(600 * time.Millisecond)
	if sc1.Child.GetChildCount() != 0 || sc2.Child.GetChildCount() != 0 || sc3.Child.GetChildCount() != 0 || sc4.Child.GetChildCount() != 0 {
		t.Fatal("失败:", sc1.Child.GetChildCount(), sc2.Child.GetChildCount(), sc3.Child.GetChildCount(), sc4.Child.GetChildCount())
	}
}

func Test_ask1(t *testing.T) {
	sc1, sc2, _, _ := test4Sidecar(130)
	sc2.SetChannel(sc2.machineID, transport.FrameTypePing, 3)
	sc2.HandleFunc(transport.FrameTypePing, testPing)
	time.Sleep(350 * time.Millisecond)
	sc1.AskOne(transport.FrameTypePing, transport.FramePing, func(err error) {
		t.Fatal(err)
	})
	time.Sleep(150 * time.Millisecond)
	sc1.exitFunc()
	time.Sleep(150 * time.Millisecond)
	sc2.exitFunc()
	time.Sleep(600 * time.Millisecond)
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
