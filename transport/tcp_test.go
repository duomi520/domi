package transport

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/duomi520/domi/util"
)

var testPingFuncNum int32
var testPongFuncNum int32

func Test_tcpServer(t *testing.T) {
	cbc := util.NewCircuitBreakerConfigure()
	sd := util.NewDispatcher(64)
	go sd.Run()
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	h := NewHandler()
	s := NewServerTCP(ctx, ":4567", h, sd, nil, &cbc)
	go s.Run()
	c, err := NewClientTCP(context.TODO(), "127.0.0.1:4567", h, sd, nil, &cbc)
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
	cbc := util.NewCircuitBreakerConfigure()
	sd := util.NewDispatcher(32)
	go sd.Run()
	loop1 := 50000 //50000
	loop2 := loop1 * 2
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	h := NewHandler()
	s := NewServerTCP(ctx, ":4568", h, sd, nil, &cbc)
	go s.Run()
	h.HandleFunc(55, testPingFunc55)
	h.HandleFunc(56, testPingFunc56)
	c, err := NewClientTCP(context.TODO(), "127.0.0.1:4568", h, sd, nil, &cbc)
	h.HandleFunc(57, testPongFunc)
	if err != nil {
		t.Error(err)
	}
	go c.Run()
	time.Sleep(50 * time.Millisecond)
	for i := 0; i < loop1; i++ {
		data := "ping" + strconv.Itoa(i)
		f := NewFrameSlice(55, []byte(data), nil)
		if err := c.Csession.WriteFrameDataPromptly(f); err != nil {
			t.Fatal(err.Error())
		}
	}
	for i := loop1; i < loop2; i++ {
		data := "ping" + strconv.Itoa(i)
		f := NewFrameSlice(56, []byte(data), nil)
		if err := c.Csession.WriteFrameDataToCache(f, nil); err != nil {
			t.Fatal(err.Error())
		}
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
	fp := [12]byte{12, 0, 0, 0, 0, 0, 57, 0, 112, 111, 110, 103}
	num := atomic.LoadInt32(&testPingFuncNum)
	if !bytes.EqualFold([]byte("ping"+strconv.Itoa(int(num))), s.GetFrameSlice().GetData()) {
		fmt.Println("testPingFunc55不相等。", s.GetFrameSlice().GetData())
	}
	if err := s.WriteFrameDataPromptly(DecodeByBytes(fp[:])); err != nil {
		fmt.Println(err.Error())
	}
	atomic.AddInt32(&testPingFuncNum, 1)
	return nil
}
func testPingFunc56(s Session) error {
	fp := [12]byte{12, 0, 0, 0, 0, 0, 57, 0, 112, 111, 110, 103}
	atomic.LoadInt32(&testPingFuncNum)
	if err := s.WriteFrameDataToCache(DecodeByBytes(fp[:]), nil); err != nil {
		fmt.Println(err.Error())
	}
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

func Test_limiter(t *testing.T) {
	cbc := util.NewCircuitBreakerConfigure()
	var testLimiterWG sync.WaitGroup
	var count int
	buf := make([]byte, 10000)
	util.CopyUint32(buf[0:4], 10000)
	util.CopyUint16(buf[6:8], 56)
	fs := DecodeByBytes(buf)
	limiter := &util.Limiter{}
	limiter.LimiterConfigure = &util.LimiterConfigure{LimitRate: 20000, LimitSize: 50000}
	go limiter.Run()
	sd := util.NewDispatcher(32)
	defer sd.Close()
	go sd.Run()
	h := NewHandler()
	h.HandleFunc(56, func(s Session) error {
		log.Println(count)
		testLimiterWG.Done()
		count++
		return nil
	})
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	s := NewServerTCP(ctx, ":4569", h, sd, limiter, &cbc)
	go s.Run()
	c, err := NewClientTCP(context.TODO(), "127.0.0.1:4569", h, sd, nil, &cbc)
	if err != nil {
		t.Fatal(err)
	}
	go c.Run()
	time.Sleep(150 * time.Millisecond)
	for i := 0; i < 20; i++ {
		testLimiterWG.Add(1)
		if err := c.Csession.WriteFrameDataPromptly(fs); err != nil {
			t.Fatal(err)
		}
	}
	testLimiterWG.Wait()
	ctxExitFunc()
	time.Sleep(150 * time.Millisecond)
	c.Csession.Close()
	time.Sleep(150 * time.Millisecond)
}

func Test_circuitBreaker(t *testing.T) {
	cbc := util.NewCircuitBreakerConfigure()
	cbc.RequestVolumeThreshold = 5
	sd := util.NewDispatcher(64)
	go sd.Run()
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	h := NewHandler()
	h.HandleFunc(65, func(Session) error {
		return nil
	})
	s := NewServerTCP(ctx, ":4570", h, sd, nil, &cbc)
	go s.Run()
	c, err := NewClientTCP(context.TODO(), "127.0.0.1:4570", h, sd, nil, &cbc)
	if err != nil {
		t.Error(err)
	}
	go c.Run()
	time.Sleep(150 * time.Millisecond)
	fs := NewFrameSlice(65, nil, nil)
	for i := 0; i < 8; i++ {
		time.Sleep(time.Millisecond)
		c.Csession.circuitBreaker.ErrorRecord()
		if err := c.Csession.WriteFrameDataToCache(fs, nil); err != nil {
			t.Log(err)
		}
		t.Log(c.Csession.circuitBreaker)
	}
	ctxExitFunc()
	time.Sleep(150 * time.Millisecond)
	c.Csession.Close()
	time.Sleep(150 * time.Millisecond)
	sd.Close()
	time.Sleep(150 * time.Millisecond)
}
