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
	//KeyCtxStopFunc MicroService停止函数
	KeyCtxStopFunc Key = iota + 101
	//KeyExitFlag 立即强制退出 为 1 ，正常退出为 2
	KeyExitFlag
)
const (
	exitWaitTime = 8 //等待8秒
)

//MicroService 微服务
type MicroService struct {
	Ctx         context.Context
	ctxExitFunc context.CancelFunc
	ExitFlag    int32
	StopChan    chan struct{}  //等待子模块全部关闭后退出
	signalChan  chan os.Signal //立即退出信号
	closeOnce   sync.Once
	Logger      *Logger
	childCount  int32 //运行中的子模块数
}

//NewMicroService 新建
func NewMicroService() *MicroService {
	m := &MicroService{
		StopChan:   make(chan struct{}),
		signalChan: make(chan os.Signal, 1),
		childCount: 0,
	}
	m.Logger, _ = NewLogger(DebugLevel, "")
	m.Logger.SetMark("MicroService")
	var ctx context.Context
	ctx, m.ctxExitFunc = context.WithCancel(context.Background())
	ctx = context.WithValue(ctx, KeyExitFlag, &m.ExitFlag)
	//暴露出关闭函数给子模块
	m.Ctx = context.WithValue(ctx, KeyCtxStopFunc, m.Stop)
	//监听系统关闭信号
	signal.Notify(m.signalChan, os.Interrupt, os.Kill, syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM)
	return m
}

//Run 运行
func (m *MicroService) Run() {
	m.Logger.Info("Run|服务开始运行")
	select {
	//等待几秒后，强制关闭。
	case <-m.signalChan:
		m.ExitFlag = 1
		m.ctxExitFunc()
		m.Logger.Info("Run|Kill,等待子模块关闭……")
		i := exitWaitTime
		for {
			time.Sleep(1 * time.Second)
			m.Logger.Info("Run|", i, "s等待关闭的模块数：", m.getChildCount())
			i--
			if m.getChildCount() == 0 || i < 1 {
				m.Stop()
				goto end
			}
		}
	//等待子模块全部关闭后，再关闭。
	case <-m.StopChan:
		m.ExitFlag = 2
		m.ctxExitFunc()
		m.Logger.Info("Run|Stop,等待子模块关闭……")
		for {
			time.Sleep(1 * time.Second)
			//	m.Logger.Info("Run|等待关闭的模块数：", m.getChildCount())
			if m.getChildCount() == 0 {
				goto end
			}
		}
	}

end:
	m.Logger.Info("Run|所有子模块已关闭。")
	signal.Stop(m.signalChan)
	close(m.signalChan)
}

//Stop 停止服务
func (m *MicroService) Stop() {
	m.closeOnce.Do(func() {
		close(m.StopChan)
	})
}

//RunAssembly 运行子模块
func (m *MicroService) RunAssembly(a Assembly) {
	atomic.AddInt32(&m.childCount, 1)
	go func() {
		a.Run()
		atomic.AddInt32(&m.childCount, -1)
	}()
}

//Assembly 组件
type Assembly interface {
	Run()
	Close()
}

//getChildCount取得子模块数
func (m *MicroService) getChildCount() int32 {
	return atomic.LoadInt32(&m.childCount)
}

//AddChildCount 修改子模块数
func (m *MicroService) AddChildCount(delta int32) {
	atomic.AddInt32(&m.childCount, delta)
}
