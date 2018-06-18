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
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	h := NewHandler()
	sfID := util.NewSnowFlakeID(1, time.Now().UnixNano())
	s := NewServerTCP(ctx, ":4568", h, sfID)
	go s.Run()
	c, err := NewClientTCP(context.TODO(), "127.0.0.1:4568", h)
	if err != nil {
		t.Error(err)
	}
	go c.Run()
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(150 * time.Millisecond)
	c.Close()
	time.Sleep(150 * time.Millisecond)
}

func Test_tcpServerPingPong(t *testing.T) {
	loop1 := 5000
	loop2 := loop1 * 2
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	h := NewHandler()
	sfID := util.NewSnowFlakeID(1, time.Now().UnixNano())
	s := NewServerTCP(ctx, ":4569", h, sfID)
	go s.Run()
	h.HandleFunc(55, testPingFunc55)
	h.HandleFunc(56, testPingFunc56)
	c, err := NewClientTCP(context.TODO(), "127.0.0.1:4569", h)
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
		c.Send(f)
	}
	time.Sleep(150 * time.Millisecond)
	c.Close()
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(150 * time.Millisecond)
	t.Log("数值：", atomic.LoadInt32(&testPingFuncNum), atomic.LoadInt32(&testPongFuncNum))
	if testPingFuncNum != testPongFuncNum {
		t.Error("PingFuncNum、PongFuncNum不相等。")
	}
	if testPingFuncNum != int32(loop2) {
		t.Error("Func失效。")
	}
}

func testPingFunc55(s Session) {
	num := atomic.LoadInt32(&testPingFuncNum)
	if !bytes.EqualFold([]byte("ping"+strconv.Itoa(int(num))), s.GetFrameSlice().GetData()) {
		fmt.Println("testPingFunc55不相等。", s.GetFrameSlice().GetData())
	}
	if err := s.WriteFrameDataPromptly(FramePong); err != nil {
		fmt.Println(err.Error())
	}
	atomic.AddInt32(&testPingFuncNum, 1)
}
func testPingFunc56(s Session) {
	num := atomic.LoadInt32(&testPingFuncNum)
	if !bytes.EqualFold([]byte("ping"+strconv.Itoa(int(num))), s.GetFrameSlice().GetData()) {
		fmt.Println("testPingFunc56不相等。", s.GetFrameSlice().GetData())
	}
	if err := s.WriteFrameDataToCache(FramePong); err != nil {
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
