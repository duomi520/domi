package util

import (
	"sync"
	"sync/atomic"
	"time"
)

//LimiterConfigure 限流器配置
type LimiterConfigure struct {
	LimitRate int64 //限流器速率，每秒处理的令牌数
	LimitSize int64 //限流器大小，存放令牌的最大值
}

//Limiter 限流器
//每隔一段时间加入一批令牌，达到上限后，不再增加。
//Wait(n int64),申请n个令牌，取不到足够数量时阻塞。
type Limiter struct {
	*LimiterConfigure
	Snippet time.Duration //加入的时间间隔

	padding   [8]uint64
	tokens    int64         //令牌
	stopChan  chan struct{} //退出信号
	closeOnce sync.Once
}

//Wait 阻塞等待
func (l *Limiter) Wait(n int64) {
	for atomic.AddInt64(&l.tokens, -n) < 0 {
		atomic.AddInt64(&l.tokens, n)
		time.Sleep(l.Snippet)
	}
}

func (l *Limiter) initLimiter() {
	if l.Snippet == 0 {
		l.Snippet = 1000 * time.Millisecond //默认1秒
	}
	if l.stopChan == nil {
		l.stopChan = make(chan struct{})
	}
}

//Run 运行
func (l *Limiter) Run() {
	l.initLimiter()
	l.tokens = l.LimitSize
	limiterSnippet := time.NewTicker(l.Snippet)
	defer limiterSnippet.Stop()
	for {
		select {
		case <-limiterSnippet.C:
			//竟态下，准确度低，影响不大。
			new := atomic.AddInt64(&l.tokens, l.LimitRate)
			if new > l.LimitSize {
				atomic.StoreInt64(&l.tokens, l.LimitSize)
			}
		case <-l.stopChan:
			return
		}
	}
}

//Close 关闭
func (l *Limiter) Close() {
	l.closeOnce.Do(func() {
		close(l.stopChan)
	})
}
