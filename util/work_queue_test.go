package util

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func Test_Dispatcher(t *testing.T) {
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	d := NewDispatcher(ctx, "d1", 10)
	go d.Run()
	for i := 0; i < 10; i++ {
		var td testDoJob1
		td.i = i
		d.PutJob(td)
	}
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(150 * time.Millisecond)
}

type testDoJob1 struct {
	i int
}

func (td testDoJob1) WorkFunc() {
	fmt.Println(td.i)
}

func Test_Dispatcher_Check(t *testing.T) {
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	d := NewDispatcher(ctx, "d2", 30)
	d.DispatcherCheckDuration = 200 * time.Millisecond
	go d.Run()
	var td testDoJob2
	for i := 0; i < 10; i++ {
		d.PutJob(td)
	}
	time.Sleep(500 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(150 * time.Millisecond)
}

type testDoJob2 struct {
}

func (td testDoJob2) WorkFunc() {
	time.Sleep(250 * time.Millisecond)
}
