package util

import (
	"context"
	"sync"
)

//WaitGroupWrapper 封装
type WaitGroupWrapper struct {
	sync.WaitGroup
}

//Wrap 封装
func (w *WaitGroupWrapper) Wrap(cb func()) {
	w.Add(1)
	go func() {
		cb()
		w.Done()
	}()
}

//WaitSequence 顺序调度
type WaitSequence struct {
	Ctx      []context.Context
	exitFunc []context.CancelFunc
	WG       []WaitGroupWrapper
}

//NewWaitSequence 新建
func NewWaitSequence(ctx context.Context, num int) *WaitSequence {
	if num < 1 {
		return nil
	}
	wgs := &WaitSequence{}
	wgs.Ctx = make([]context.Context, num)
	wgs.exitFunc = make([]context.CancelFunc, num)
	wgs.WG = make([]WaitGroupWrapper, num)
	wgs.Ctx[0] = ctx
	for i := 1; i < num; i++ {
		wgs.Ctx[i], wgs.exitFunc[i] = context.WithCancel(context.Background())
	}
	return wgs
}

//Wait 等待按顺序关闭
func (w *WaitSequence) Wait() {
	w.WG[0].Wait()
	for i := 1; i < len(w.WG); i++ {
		w.exitFunc[i]()
		w.WG[i].Wait()
	}
}
