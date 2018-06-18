package util

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

//Key 通过Ctx暴露出来的函数
type Key int

const (
	//KeyCtxStopFunc Application停止函数
	KeyCtxStopFunc Key = iota + 101
)
const (
	exitWaitTime = 8 //等待8秒
)

//Application 应用
type Application struct {
	Ctx         context.Context
	ctxExitFunc context.CancelFunc
	Child
	stopChan   chan struct{}  //等待子模块全部关闭后退出
	signalChan chan os.Signal //立即退出信号
	closeOnce  sync.Once
	Logger     *Logger
}

//NewApplication 新建
func NewApplication() *Application {
	m := &Application{
		stopChan:   make(chan struct{}),
		signalChan: make(chan os.Signal, 1),
	}
	m.Logger, _ = NewLogger(DebugLevel, "")
	m.Logger.SetMark("Application")
	var ctx context.Context
	ctx, m.ctxExitFunc = context.WithCancel(context.Background())
	//暴露出关闭函数给子模块
	m.Ctx = context.WithValue(ctx, KeyCtxStopFunc, m.Stop)
	//监听系统关闭信号
	signal.Notify(m.signalChan, os.Interrupt, os.Kill, syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM)
	return m
}

//Run 运行
func (m *Application) Run() {
	m.Logger.Info("Run|程序开始运行")
	select {
	//等待exitWaitTime秒后，强制关闭。
	case <-m.signalChan:
		m.Logger.Info("Run|强制关闭,等待子模块关闭……")
		m.ctxExitFunc()
		i := exitWaitTime
		for m.GetChildCount() > 0 && i > 0 {
			time.Sleep(1 * time.Second)
			m.Logger.Info("Run|", i, "秒等待关闭的模块数：", m.GetChildCount())
			i--
		}
		goto end
	//等待子模块全部关闭后，再关闭。
	case <-m.stopChan:
		m.Logger.Info("Run|正常关闭,等待子模块关闭……")
		m.ctxExitFunc()
		m.Wait()
		goto end
	}
end:
	m.Logger.Info("Run|退出。")
	signal.Stop(m.signalChan)
	close(m.signalChan)
	m.Stop()
}

//Stop 停止服务
func (m *Application) Stop() {
	m.closeOnce.Do(func() {
		close(m.stopChan)
	})
}

//Assembly 组件
type Assembly interface {
	Run()
}

//Child 子模块
type Child struct {
	childCount int32 //运行中的子模块数
	sync.WaitGroup
}

//RunAssembly 运行子模块
func (c *Child) RunAssembly(a Assembly) {
	atomic.AddInt32(&c.childCount, 1)
	c.Add(1)
	go func() {
		a.Run()
		atomic.AddInt32(&c.childCount, -1)
		c.Done()
	}()
}

//GetChildCount 取得子模块数
func (c *Child) GetChildCount() int32 {
	return atomic.LoadInt32(&c.childCount)
}
