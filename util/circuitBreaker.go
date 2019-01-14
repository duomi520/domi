package util

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	rollingBucketTime       uint64 = 1048576 //滑动桶时间 约 1*Millisecond， 1048576  2^20	除1048576:>>20
	rollingBucketTimeDivide uint64 = 20
)

//CircuitBreakerConfigure 熔断器配置 实现快速失败并走备用方案
type CircuitBreakerConfigure struct {
	RollingBucketsNum      uint64 //熔断器设置统计窗口的桶数量 默认2，每个桶的时间间隔约1*Millisecond
	RequestVolumeThreshold uint64 //在统计窗口内达到此数量后的失败，进行熔断。默认20个
	SleepWindow            int64  //熔断后，尝试恢复时间。
}

//NewCircuitBreakerConfigure 新建
func NewCircuitBreakerConfigure() CircuitBreakerConfigure {
	return CircuitBreakerConfigure{
		RollingBucketsNum:      2,                              //设置统计窗口的桶数量 默认2
		RequestVolumeThreshold: 20,                             // 当在配置时间窗口内达到此数量后的失败，进行短路。默认20个
		SleepWindow:            int64(2000 * time.Millisecond), //熔断窗口时间，默认为2s
	}
}

//回路状态
//关闭状态：服务正常，并维护一个失败率统计，当失败率达到阀值时，转到开启状态
//开启状态：服务异常，一段时间之后，进入半开启状态
//半开启装态：这时熔断器只允许一个请求通过. 当该请求调用成功时, 熔断器恢复到关闭状态. 若该请求失败, 熔断器继续保持打开状态, 接下来的请求被禁止通过

//CircuitBreaker 回路
type CircuitBreaker struct {
	*CircuitBreakerConfigure
	state         uint32 //关闭、开启
	halfOpenToken uint32 //半开启
	HalfOpenFunc  func() error
	timestamp     int64 //熔断时间戳
	buckets       buckets
}

type buckets struct {
	sync.Mutex
	item []bucket
}
type bucket struct {
	stamp uint64 //桶时间戳
	count uint64 //计数
	total uint64 //合计
}

//NewCircuitBreaker 新加回路
func NewCircuitBreaker(configure *CircuitBreakerConfigure) *CircuitBreaker {
	cb := &CircuitBreaker{
		state:         StateCircuitBreakerClosed,
		halfOpenToken: 0,
	}
	cb.CircuitBreakerConfigure = configure
	if cb.RollingBucketsNum == 0 {
		cb.RollingBucketsNum = 1
	}
	if cb.RequestVolumeThreshold == 0 {
		cb.RequestVolumeThreshold = 20
	}
	if cb.SleepWindow < 1000 {
		cb.SleepWindow = int64(2000 * time.Millisecond)
	}
	cb.buckets.item = make([]bucket, 0, 64)
	return cb
}

//ErrorRecord 记载
func (cb *CircuitBreaker) ErrorRecord() {
	cb.buckets.Lock()
	defer cb.buckets.Unlock()
	t := uint64(time.Now().UnixNano()) >> rollingBucketTimeDivide
	size := len(cb.buckets.item)
	if size == 0 {
		cb.buckets.item = append(cb.buckets.item, bucket{stamp: t, count: 1, total: 1})
		if 1 >= cb.RequestVolumeThreshold {
			if atomic.CompareAndSwapUint32(&cb.state, StateCircuitBreakerClosed, StateCircuitBreakerOpen) {
				atomic.StoreInt64(&cb.timestamp, time.Now().UnixNano())
			}
		}
		return
	}
	size--
	if cb.buckets.item[size].stamp == t {
		cb.buckets.item[size].count++
		cb.buckets.item[size].total++
	} else {
		cb.buckets.item = append(cb.buckets.item, bucket{stamp: t, count: 1, total: 1})
		current := len(cb.buckets.item) - 1
		//		fmt.Println("zzz", current,  cb.buckets.item)
		i := current
		for i >= 0 {
			if (t - cb.buckets.item[i].stamp) < cb.RollingBucketsNum {
				cb.buckets.item[current].total += cb.buckets.item[i].count
			} else {
				//	fmt.Println("a", i, current, cb.buckets.item)
				copy(cb.buckets.item[:current-i], cb.buckets.item[i+1:current+1])
				cb.buckets.item = cb.buckets.item[:current-i]
				//	fmt.Println("b", i, current, cb.buckets.item)
				break
			}
			i--
		}
		//TODO 进一步测试
		_ = fmt.Println
	}
	if cb.buckets.item[len(cb.buckets.item)-1].total >= cb.RequestVolumeThreshold {
		if atomic.CompareAndSwapUint32(&cb.state, StateCircuitBreakerClosed, StateCircuitBreakerOpen) {
			atomic.StoreInt64(&cb.timestamp, time.Now().UnixNano())
		}
	}
}

//IsPass 是否通过
func (cb *CircuitBreaker) IsPass() bool {
	if atomic.LoadUint32(&cb.state) == StateCircuitBreakerClosed {
		return true
	}
	if (time.Now().UnixNano() - atomic.LoadInt64(&cb.timestamp)) > cb.SleepWindow {
		//取令牌
		if atomic.CompareAndSwapUint32(&cb.halfOpenToken, 0, StateCircuitBreakerHalfOpen) {
			atomic.StoreInt64(&cb.timestamp, time.Now().UnixNano())
			if err := cb.HalfOpenFunc(); err != nil {
				atomic.StoreUint32(&cb.halfOpenToken, 0)
			}
		}
	}
	return false
}

//SetCircuitBreakerPass 关闭状态：服务正常。
func (cb *CircuitBreaker) SetCircuitBreakerPass() {
	atomic.StoreUint32(&cb.state, StateCircuitBreakerClosed)
	atomic.StoreUint32(&cb.halfOpenToken, 0)
}
