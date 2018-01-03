package transport

import (
	"bytes"
	"context"
	"fmt"
	"github.com/duomi520/domi/util"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

var testPingFuncNum int32
var testPongFuncNum int32

func Test_tcpServer(t *testing.T) {
	ctx := context.TODO()
	h := NewHandler()
	sfID := util.NewSnowFlakeID(1, time.Now().UnixNano())
	dispatcher := util.NewDispatcher(ctx, "TCP", 256)
	go dispatcher.Run()
	s := NewServerTCP(ctx, ":4568", h, sfID, dispatcher)
	go s.Run()
	c, err := NewClientTCP(ctx, "127.0.0.1:4568", h)
	if err != nil {
		t.Error(err)
	}
	go c.Run()
	time.Sleep(150 * time.Millisecond)
	c.Close()
	time.Sleep(150 * time.Millisecond)
	s.Close()
	time.Sleep(150 * time.Millisecond)
}

func Test_tcpServerPingPong(t *testing.T) {
	ctx := context.TODO()
	h := NewHandler()
	sfID := util.NewSnowFlakeID(1, time.Now().UnixNano())
	dispatcher := util.NewDispatcher(ctx, "TCP", 256)
	go dispatcher.Run()
	s := NewServerTCP(ctx, ":4569", h, sfID, dispatcher)
	go s.Run()
	h.HandleFunc(55, testPingFunc1)
	h.HandleFunc(56, testPingFunc2)
	c, err := NewClientTCP(ctx, "127.0.0.1:4569", h)
	h.HandleFunc(FrameTypePong, testPongFunc)
	if err != nil {
		t.Error(err)
	}
	go c.Run()
	time.Sleep(50 * time.Millisecond)
	for i := 0; i < 1500; i++ {
		data := "ping" + strconv.Itoa(i)
		f := NewFrameSlice(55, []byte(data), nil)
		if err := c.Csession.WriteFrameDataPromptly(f); err != nil {
			t.Error(err)
		}
	}
	for i := 1500; i < 3000; i++ {
		data := "ping" + strconv.Itoa(i)
		f := NewFrameSlice(56, []byte(data), nil)
		if err := c.SendToQueue(f); err != nil {
			t.Error(err)
		}
	}
	time.Sleep(150 * time.Millisecond)
	c.Close()
	time.Sleep(150 * time.Millisecond)
	s.Close()
	time.Sleep(150 * time.Millisecond)
	t.Log("数值：", atomic.LoadInt32(&testPingFuncNum), atomic.LoadInt32(&testPongFuncNum))
	if testPingFuncNum != testPongFuncNum {
		t.Error("PingFuncNum、PongFuncNum不相等。")
	}
	if testPingFuncNum != 3000 {
		t.Error("Func失效。")
	}
}

func testPingFunc1(s Session) {
	num := atomic.LoadInt32(&testPingFuncNum)
	if !bytes.EqualFold([]byte("ping"+strconv.Itoa(int(num))), s.GetFrameSlice().GetData()) {
		fmt.Println("testPingFunc1不相等。", s.GetFrameSlice().GetData())
	}
	if err := s.WriteFrameDataPromptly(FramePong); err != nil {
		fmt.Println(err.Error())
	}
	atomic.AddInt32(&testPingFuncNum, 1)
}
func testPingFunc2(s Session) {
	num := atomic.LoadInt32(&testPingFuncNum)
	if !bytes.EqualFold([]byte("ping"+strconv.Itoa(int(num))), s.GetFrameSlice().GetData()) {
		fmt.Println("testPingFunc2不相等。", s.GetFrameSlice().GetData())
	}
	if err := s.WriteFrameDataToQueue(FramePong); err != nil {
		fmt.Println(err.Error())
	}
	atomic.AddInt32(&testPingFuncNum, 1)
}
func testPongFunc(s Session) {
	if !bytes.EqualFold([]byte("pong"), s.GetFrameSlice().GetData()) {
		fmt.Println("testPongFunc不相等。", s.GetFrameSlice().GetData())
	}
	atomic.AddInt32(&testPongFuncNum, 1)
}
