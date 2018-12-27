package transport

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/duomi520/domi/util"
)

var testPingFuncNum int32
var testPongFuncNum int32

func Test_tcpServer(t *testing.T) {
	sd := util.NewDispatcher(64)
	go sd.Run()
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	h := NewHandler()
	s := NewServerTCP(ctx, ":4568", h, sd, nil)
	go s.Run()
	c, err := NewClientTCP(context.TODO(), "127.0.0.1:4568", h, sd, nil)
	if err != nil {
		t.Error(err)
	}
	go c.Run()
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(150 * time.Millisecond)
	c.Csession.Close()
	time.Sleep(150 * time.Millisecond)
	sd.Close()
	time.Sleep(150 * time.Millisecond)
}

func Test_tcpServerPingPong(t *testing.T) {
	sd := util.NewDispatcher(32)
	go sd.Run()
	loop1 := 50000 //50000
	loop2 := loop1 * 2
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	h := NewHandler()
	s := NewServerTCP(ctx, ":4569", h, sd, nil)
	go s.Run()
	h.HandleFunc(55, testPingFunc55)
	h.HandleFunc(56, testPingFunc56)
	c, err := NewClientTCP(context.TODO(), "127.0.0.1:4569", h, sd, nil)
	h.HandleFunc(FrameTypePong, testPongFunc)
	if err != nil {
		t.Error(err)
	}
	go c.Run()
	time.Sleep(50 * time.Millisecond)
	for i := 0; i < loop1; i++ {
		data := "ping" + strconv.Itoa(i)
		f := NewFrameSlice(55, []byte(data), nil)
		if err := c.Csession.WriteFrameDataPromptly(f); err != nil {
			t.Error(err)
		}
	}
	for i := loop1; i < loop2; i++ {
		data := "ping" + strconv.Itoa(i)
		f := NewFrameSlice(56, []byte(data), nil)
		c.Csession.WriteFrameDataToCache(f, 0)
	}
	time.Sleep(1500 * time.Millisecond)
	c.Csession.Close()
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(150 * time.Millisecond)
	sd.Close()
	time.Sleep(150 * time.Millisecond)
	t.Log("数值：", atomic.LoadInt32(&testPingFuncNum), atomic.LoadInt32(&testPongFuncNum))
	if testPingFuncNum != testPongFuncNum {
		t.Error("PingFuncNum、PongFuncNum不相等。")
	}
	if testPingFuncNum != int32(loop2) {
		t.Error("Func失效。")
	}
}

func testPingFunc55(s Session) error {
	num := atomic.LoadInt32(&testPingFuncNum)
	if !bytes.EqualFold([]byte("ping"+strconv.Itoa(int(num))), s.GetFrameSlice().GetData()) {
		fmt.Println("testPingFunc55不相等。", s.GetFrameSlice().GetData())
	}
	if err := s.WriteFrameDataPromptly(FramePong); err != nil {
		fmt.Println(err.Error())
	}
	atomic.AddInt32(&testPingFuncNum, 1)
	return nil
}
func testPingFunc56(s Session) error {
	atomic.LoadInt32(&testPingFuncNum)
	s.WriteFrameDataToCache(FramePong, 0)
	atomic.AddInt32(&testPingFuncNum, 1)
	return nil
}
func testPongFunc(s Session) error {
	if !bytes.EqualFold([]byte("pong"), s.GetFrameSlice().GetData()) {
		fmt.Println("testPongFunc不相等。", s.GetFrameSlice().GetData())
	}
	atomic.AddInt32(&testPongFuncNum, 1)
	return nil
}

func Test_tcpReject(t *testing.T) {
	sd := util.NewDispatcher(32)
	go sd.Run()
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	h := NewHandler()
	h.HandleFunc(65, func(s Session) error {
		return nil
	})
	h.ErrorFunc(66, func(fi int, fe error) {
		if fe == nil {
			t.Fatal(fi, fe.Error())
		}
	})
	s := NewServerTCP(ctx, ":4570", h, sd, nil)
	go s.Run()
	c, err := NewClientTCP(context.TODO(), "127.0.0.1:4570", h, sd, nil)
	if err != nil {
		t.Error(err)
	}
	go c.Run()
	time.Sleep(50 * time.Millisecond)
	f := NewFrameSlice(65, []byte("reject"), nil)
	c.Csession.token = 5000
	c.Csession.WriteFrameDataToCache(f, 66)
	time.Sleep(150 * time.Millisecond)
	c.Csession.Close()
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(150 * time.Millisecond)
	sd.Close()
	time.Sleep(150 * time.Millisecond)
}
