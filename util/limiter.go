package util

import (
	"sync"
	"sync/atomic"
	"time"
)

//Limiter 限流器
//每隔一段时间加入一批令牌，达到上限后，不再增加。
//Wait(n int64),申请n个令牌，取不到足够数量时阻塞。
type Limiter struct {
	Rate        int64         //每个时间间隔处理的速率
	BucketLimit int64         //桶的大小
	Snippet     time.Duration //加入的时间间隔

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

//Run 运行
func (l *Limiter) Run() {
	if l.Snippet == 0 {
		l.Snippet = time.Second
	}
	if l.stopChan == nil {
		l.stopChan = make(chan struct{})
	}
	l.tokens = l.BucketLimit
	limiterSnippet := time.NewTicker(l.Snippet)
	defer limiterSnippet.Stop()
	for {
		select {
		case <-limiterSnippet.C:
			//准确度低，但速度快。
			new := atomic.AddInt64(&l.tokens, l.Rate)
			if new > l.BucketLimit {
				atomic.StoreInt64(&l.tokens, l.BucketLimit)
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
