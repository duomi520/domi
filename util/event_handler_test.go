package util

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func Test_EventHandler(t *testing.T) {
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	testEventHandler := NewEventHandler(ctx, testTask, testDecode)
	go testEventHandler.Run()
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(150 * time.Millisecond)
}

var sum int64

func Test_WriteToRingBuffer(t *testing.T) {
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	testEventHandler := NewEventHandler(ctx, testTask, testDecode)
	go testEventHandler.Run()
	time.Sleep(150 * time.Millisecond)
	var wg sync.WaitGroup
	for p := 0; p < 12; p++ {
		temp := p
		wg.Add(1)
		go func(n int, ww *sync.WaitGroup) {
			fp := []byte{28, 0, 0, 0, 16, 0, 3, 0, 112, 105, 110, 103, 8, 0, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0}
			for i := 0; i < 1000000; i++ {
				if i%100 == 0 {
					time.Sleep(100 * time.Microsecond) //1秒写1M次
				}
				fp[20] = byte(i % 1000)
				atomic.AddInt64(&sum, 1)
				if err := testEventHandler.Ring.WriteToRingBuffer(fp); err != nil {
					fmt.Println(err.Error(), i, n)
				}
			}
			ww.Done()
		}(temp, &wg)
	}
	wg.Wait()
	if testEventHandler.Ring.writeCursor != 12*28*1000000 || testEventHandler.Ring.availableCursor != 12*28*1000000 {
		t.Error("失败", sum*28, testEventHandler.Ring.writeCursor, testEventHandler.Ring.availableCursor)
	} else {
		t.Log(sum*28, testEventHandler.Ring.writeCursor, testEventHandler.Ring.availableCursor)
	}
	if testWrong > 0 {
		t.Error("失败", testWrong)
	}
	ctxExitFunc()
	time.Sleep(150 * time.Millisecond)
}

var testWrong int64

func testTask(pj ProcessorJob) {
	fp := []byte{20, 0, 0, 0, 8, 0, 3, 0, 112, 105, 110, 103, 8, 0, 0, 0, 0, 0, 0, 0}
	for _, v := range pj.Data {
		if !bytes.EqualFold(fp, v) {
			atomic.AddInt64(&testWrong, 1)
			//	fmt.Println(wrong,  v)
		}
	}
}

func testDecode(b []byte) (int64, []byte) {
	length := len(b)
	newLength := length - 8
	buf := b[:newLength]
	id := BytesToInt64(b[newLength:])
	CopyInt64(buf[:4], int64(newLength))
	u16 := BytesToUint16(buf[4:6])
	CopyUint16(buf[4:6], u16-8)
	return id, buf
}
