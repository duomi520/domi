package domi

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/duomi520/domi/util"
)

const (
	exitWaitTime = 8 //等待8秒
)

//Master 管理
type Master struct {
	Ctx         context.Context
	ctxExitFunc context.CancelFunc
	util.Child
	stopChan   chan struct{}  //等待子模块全部关闭后退出
	signalChan chan os.Signal //立即退出信号
	closeOnce  sync.Once
	Logger     *util.Logger
}

//NewMaster 新建管理协程，协调各个工作协程，及监听关闭信号。
func NewMaster() *Master {
	m := &Master{
		stopChan:   make(chan struct{}),
		signalChan: make(chan os.Signal, 1),
	}
	m.Logger, _ = util.NewLogger(util.InfoLevel, "")
	m.Logger.SetMark("Master")
	//暴露出关闭函数给子模块
	m.Ctx, m.ctxExitFunc = context.WithCancel(context.Background())
	//监听系统关闭信号
	signal.Notify(m.signalChan, os.Interrupt, os.Kill, syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM)
	return m
}

//Guard 看守,阻塞main函数。
func (m *Master) Guard() {
	m.Logger.Info("Run|程序开始运行")
	go func() {
		//等待exitWaitTime秒后，强制关闭。
		<-m.signalChan
		m.Logger.Info("Run|强制关闭中……")
		m.ctxExitFunc()
		i := exitWaitTime
		for m.GetChildCount() > 0 && i > 0 {
			time.Sleep(1 * time.Second)
			m.Logger.Info("Run| ", i, "秒等待关闭的模块数：", m.GetChildCount())
			i--
		}
		if i == 0 {
			m.Logger.Info("Run|强制退出。")
		} else {
			m.Logger.Info("Run|正常退出。")
		}
		signal.Stop(m.signalChan)
		os.Exit(1)
	}()
	//等待子模块全部关闭后，再关闭。
	<-m.stopChan
	m.Logger.Info("Run|正常关闭中……")
	m.ctxExitFunc()
	m.Wait()
	m.Logger.Info("Run|正常退出。")
	signal.Stop(m.signalChan)
}

//Stop 通知管理协程退出。
func (m *Master) Stop() {
	m.closeOnce.Do(func() {
		close(m.stopChan)
	})
}
