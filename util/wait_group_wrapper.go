package util

import (
	"sync"
	"sync/atomic"
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

//Key 通过Ctx暴露出来的函数
type Key int

const (
	//KeyCtxStopFunc Application停止函数
	KeyCtxStopFunc Key = iota + 101
)

//Runnable 组件
type Runnable interface {
	Run()
}

//Child 子模块
type Child struct {
	childCount int32 //运行中的子模块数
	sync.WaitGroup
}

//RunAssembly 运行子模块
func (c *Child) RunAssembly(a Runnable) {
	atomic.AddInt32(&c.childCount, 1)
	c.Add(1)
	go func() {
		a.Run()
		c.Done()
		atomic.AddInt32(&c.childCount, -1)
	}()
}

//GetChildCount 取得子模块数
func (c *Child) GetChildCount() int32 {
	return atomic.LoadInt32(&c.childCount)
}

//SetChildCount 设置子模块数
func (c *Child) SetChildCount(i int32) {
	atomic.StoreInt32(&c.childCount, i)
}
