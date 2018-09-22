package domi

import (
	"fmt"
	"testing"
	"time"
)

func Test_Serial1(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node()
	s := NewSerial(n2)
	s.Subscribe(1091, testReply)
	s.Subscribe(1092, testReply)
	s.Subscribe(1093, testRequest)
	s.Subscribe(1094, testRequest)
	go s.Run()
	n1.Subscribe(1193, testReply)
	n1.Subscribe(1194, testReply)
	time.Sleep(500 * time.Millisecond)
	n1.Notify(1091, []byte("1091"))
	n1.Notify(1092, []byte("1092"))
	n1.Call(1093, []byte("1093"), 1193)
	n1.Call(1094, []byte("1094"), 1194)
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	s.Close()
	time.Sleep(150 * time.Millisecond)
}

func Test_Serial2(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node()
	s := NewSerial(n2)
	s.SubscribeRace([]uint16{1106, 1107, 1108}, testReply)
	go s.Run()
	time.Sleep(500 * time.Millisecond)
	n1.Notify(1106, []byte("s1106"))
	n1.Notify(1107, []byte("s1107"))
	n1.Notify(1108, []byte("s1108"))
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	s.Close()
	time.Sleep(150 * time.Millisecond)
}

func Test_Serial3(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node()
	s := NewSerial(n2)
	s.SubscribeAll([]uint16{1201, 1202, 1203}, testRequestAll)
	go s.Run()
	time.Sleep(500 * time.Millisecond)
	n1.Notify(1201, []byte("s1201"))
	n1.Notify(1202, []byte("s1202"))
	n1.Notify(1203, []byte("s1203"))
	time.Sleep(150 * time.Millisecond)
	n1.Notify(1201, []byte("s1204"))
	n1.Notify(1202, []byte("s1205"))
	time.Sleep(150 * time.Millisecond)
	n1.Notify(1201, []byte("s1207"))
	n1.Notify(1202, []byte("s1208"))
	n1.Notify(1203, []byte("s1209"))
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	s.Close()
	time.Sleep(150 * time.Millisecond)
}

func testRequestAll(ctx *ContextMQs) {
	for _, data := range ctx.Requests {
		fmt.Println(ctx.sidecar.MachineID, " testRequests:", string(data))
	}
}

func Test_Serial4(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node()
	s := NewSerial(n2)
	s.SubscribeAll([]uint16{1301, 1302, 1303}, testRequestAll)
	go s.Run()
	time.Sleep(500 * time.Millisecond)
	n1.Notify(1301, []byte("s1301"))
	n1.Notify(1302, []byte("s1302"))
	n1.Notify(1303, []byte("s1303"))
	time.Sleep(150 * time.Millisecond)
	s.UnsubscribeGroup([]uint16{1301, 1302, 1303})
	time.Sleep(150 * time.Millisecond)
	n1.Notify(1301, []byte("s1307"))
	n1.Notify(1302, []byte("s1308"))
	n1.Notify(1303, []byte("s1309"))
	time.Sleep(150 * time.Millisecond)
	if len(s.bags) != 0 {
		t.Fatal("退订失败。")
	}
	ctxExitFunc()
	s.Close()
	time.Sleep(150 * time.Millisecond)
}
