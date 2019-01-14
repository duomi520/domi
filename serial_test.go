package domi

import (
	"testing"
	"time"
)

func Test_Serial1(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node(510)
	s := &Serial{
		Node: n2,
	}
	s.Init()
	s.Subscribe(1091, testReply)
	s.Subscribe(1092, testReply)
	s.Subscribe(1093, testRequest)
	s.Subscribe(1094, testRequest)
	go s.Run()
	n1.Subscribe(1193, testReply)
	n1.Subscribe(1194, testReply)
	time.Sleep(1000 * time.Millisecond)
	n1.Notify(1091, []byte("1091"), testError)
	n1.Notify(1092, []byte("1092"), testError)
	n1.Call(1093, []byte("1093"), 1193, testError)
	n1.Call(1094, []byte("1094"), 1194, testError)
	time.Sleep(50 * time.Millisecond)
	ctxExitFunc()
	s.Close()
	time.Sleep(50 * time.Millisecond)
	testTableVerification(t, []string{
		"1 testReply:1091",
		"1 testReply:1092",
		"1 testRequest:1093",
		"1 testRequest:1094",
		"0 testReply:Hi",
		"0 testReply:Hi",
	})
}

func Test_Serial2(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node(520)
	s := &Serial{
		Node: n2,
	}
	s.Init()
	s.SubscribeRace([]uint16{1106, 1107, 1108}, testReply)
	go s.Run()
	time.Sleep(1000 * time.Millisecond)
	n1.Notify(1106, []byte("s1106"), testError)
	n1.Notify(1107, []byte("s1107"), testError)
	n1.Notify(1108, []byte("s1108"), testError)
	time.Sleep(50 * time.Millisecond)
	ctxExitFunc()
	s.Close()
	time.Sleep(50 * time.Millisecond)
	testTableVerification(t, []string{
		"1 testReply:s1106",
		"1 testReply:s1107",
		"1 testReply:s1108",
	})
}

func Test_Serial3(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node(530)
	s := &Serial{
		Node: n2,
	}
	s.Init()
	s.SubscribeAll([]uint16{1201, 1202, 1203}, testRequestAll)
	go s.Run()
	time.Sleep(1000 * time.Millisecond)
	n1.Notify(1201, []byte("s1201"), testError)
	n1.Notify(1202, []byte("s1202"), testError)
	n1.Notify(1203, []byte("s1203"), testError)
	time.Sleep(50 * time.Millisecond)
	n1.Notify(1201, []byte("s1204"), testError)
	n1.Notify(1202, []byte("s1205"), testError)
	time.Sleep(50 * time.Millisecond)
	n1.Notify(1201, []byte("s1207"), testError)
	n1.Notify(1202, []byte("s1208"), testError)
	n1.Notify(1203, []byte("s1209"), testError)
	time.Sleep(50 * time.Millisecond)
	ctxExitFunc()
	s.Close()
	time.Sleep(50 * time.Millisecond)
	testTableVerification(t, []string{
		"testRequests: s1201",
		"testRequests: s1202",
		"testRequests: s1203",
		"testRequests: s1204",
		"testRequests: s1205",
		"testRequests: s1209",
	})
}

func testRequestAll(ctx *ContextMQs) {
	for _, data := range ctx.Requests {
		testNodeTable = append(testNodeTable, "testRequests: "+string(data))
	}
}

func Test_Serial4(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node(540)
	s := &Serial{
		Node: n2,
	}
	s.Init()
	s.SubscribeAll([]uint16{1301, 1302, 1303}, testRequestAll)
	go s.Run()
	time.Sleep(1000 * time.Millisecond)
	n1.Notify(1301, []byte("s1301"), testError)
	n1.Notify(1302, []byte("s1302"), testError)
	n1.Notify(1303, []byte("s1303"), testError)
	time.Sleep(50 * time.Millisecond)
	s.UnsubscribeGroup([]uint16{1301, 1302, 1303})
	time.Sleep(500 * time.Millisecond)
	n1.Notify(1301, []byte("s1307"), testError)
	n1.Notify(1302, []byte("s1308"), testError)
	n1.Notify(1303, []byte("s1309"), testError)
	time.Sleep(50 * time.Millisecond)
	if len(s.bags) != 0 {
		t.Fatal("退订失败。")
	}
	ctxExitFunc()
	s.Close()
	time.Sleep(50 * time.Millisecond)
	testTableVerification(t, []string{
		"testRequests: s1301",
		"testRequests: s1302",
		"testRequests: s1303",
	})
}

func Test_RejectFunc1(t *testing.T) {
	ctxExitFunc, n1, _ := test2Node(550)
	s := &Serial{
		Node: n1,
	}
	s.Init()
	go s.Run()
	time.Sleep(1000 * time.Millisecond)
	s.Notify(1402, []byte("s1401"), func(err error) {
		if err == nil {
			t.Fatal(err.Error())
		}
	})
	time.Sleep(50 * time.Millisecond)
	ctxExitFunc()
	s.Close()
	time.Sleep(50 * time.Millisecond)
}
