package util

import (
	"errors"
	"sync"
	"time"
)

//定义snowflake的参数
const (
	MaxWorkNumber      = 1023 //最大1023
	MaxSequenceNumber  = 4000 //理论最大4095
	WorkLeftShift      = uint(12)
	TimestampLeftShift = uint(22)
)

//定义错误
var (
	ErrFailureAllocID           = errors.New("util.IdWorker.GetId|工作机器id耗尽。")
	ErrMachineTimeUnSynchronize = errors.New("util.SnowFlakeId.NextId|机器时钟后跳，timeGen()生成时间戳，早于SnowFlakeId记录的时间戳")
)

//
// 工作组中心分配工作机器id
//

//IDWorker 工作组用于分配工作机器id
type IDWorker struct {
	SystemCenterStartupTime int64 //2015-12-31 00:00:00 +0800 CST  ，时间戳启动计算时间零点
	queue                   []int
	queueMap                []int
	consumption             int //消费
	production              int //生产
	mutex                   sync.Mutex
}

//NewIDWorker 初始化
func NewIDWorker(st int64) *IDWorker {
	w := &IDWorker{
		SystemCenterStartupTime: st,
		queue:       make([]int, MaxWorkNumber),
		queueMap:    make([]int, MaxWorkNumber),
		production:  0,
		consumption: 0,
	}
	for i := 0; i < MaxWorkNumber; i++ {
		w.queue[i] = i
		w.queueMap[i] = i
	}
	return w
}

//GetID 取id
func (w *IDWorker) GetID() (int, error) {
	n := 0
	var err error
	w.mutex.Lock()
	if w.queue[w.consumption] == -1 {
		n = -1
		err = ErrFailureAllocID
	} else {
		n = w.queue[w.consumption]
		w.queue[w.consumption] = -1
		w.consumption++
		if w.consumption >= MaxWorkNumber {
			w.consumption = 0
		}
		w.queueMap[n] = -1
		err = nil
	}
	w.mutex.Unlock()
	return n, err
}

//
//当工作机器与工作组中心的时间不同步时，释放后再利用的workID与之前释放的workID的snowflake id会重复，逻辑上产生bug。
//

//PutID 还id
func (w *IDWorker) PutID(n int) {
	if n < 0 || n >= MaxWorkNumber {
		return
	}
	w.mutex.Lock()
	if w.queueMap[n] != -1 || w.queue[w.production] != -1 {
		w.mutex.Unlock()
		return
	}
	w.queue[w.production] = n
	w.queueMap[n] = w.production
	w.production++
	if w.production >= MaxWorkNumber {
		w.production = 0
	}
	w.mutex.Unlock()
}

//
//各个工作站生成Twitter-Snowflake算法的ID
//

//SnowFlakeID 工作站
type SnowFlakeID struct {
	systemCenterStartupTime int64 //时间戳启动计算时间零点
	lastTimestamp           int64 //41bit的时间戳，仅支持69.7年
	workID                  int64 //10bit的工作机器id
	sequence                int64 //12bit的序列号
	mutex                   sync.Mutex
}

//NewSnowFlakeID 工作组
func NewSnowFlakeID(id int64, startupTime int64) *SnowFlakeID {
	if id < 0 || id >= MaxWorkNumber || startupTime < 0 {
		return nil
	}
	s := &SnowFlakeID{
		systemCenterStartupTime: startupTime / int64(time.Millisecond),
		lastTimestamp:           timeGen(),
		workID:                  id << WorkLeftShift,
		sequence:                0,
	}
	return s
}

//NextID 取得 snowflake id.
func (s *SnowFlakeID) NextID() (int64, error) {
	timestamp := timeGen()
	s.mutex.Lock()
	if timestamp < s.lastTimestamp {
		s.mutex.Unlock()
		return 0, ErrMachineTimeUnSynchronize
	}
	if timestamp == s.lastTimestamp {
		s.sequence = s.sequence + 1
		if s.sequence > MaxSequenceNumber {
			time.Sleep(time.Millisecond) //效率不高，貌似影响不大
			timestamp++
			s.sequence = 0
		}
	} else {
		s.sequence = 0
	}
	s.lastTimestamp = timestamp
	id := ((timestamp - s.systemCenterStartupTime) << TimestampLeftShift) | s.workID | s.sequence
	s.mutex.Unlock()
	return id, nil
}

//timeGen 取得time.Now() unix 毫秒.
func timeGen() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

//GetWorkID 取得工作机器id
func GetWorkID(id int64) int64 {
	temp := id >> WorkLeftShift
	return temp & 1023 //1111111111   10bit
}
