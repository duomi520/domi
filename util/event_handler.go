package util

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"time"
)

//Event 事件
type Event interface {
}

//ErrRingBufferOverflow 环形数组已满。
var ErrRingBufferOverflow = errors.New("util.WriteToRingBuffer|环形数组已满。")

//ErrRingBufferClose 环形数组已关闭
var ErrRingBufferClose = errors.New("util.WriteToRingBuffer|环形数组已关闭")

//RingBuffer 环形数组
type RingBuffer struct {
	eventHandler    *EventHandler
	stopFlag        int32 //退出标志
	ringBufferShift int64 //求余 设X对Y求余，Y等于2^N，公式为：X & (2^N - 1)
	ringBufferSize  int64 //2^26 67108864 64M,2^27 134217728 128M,2^28 268435456 256M
	ring            []byte
	pad1            [56]byte //pad*:凑够64字节CPU缓存行
	writeCursor     int64
	pad2            [56]byte
	availableCursor int64 //已提交位置

}

//getAvailableCursor 读取已提交位置
func (r *RingBuffer) getAvailableCursor() int64 {
	return atomic.LoadInt64(&r.availableCursor)
}

//WriteToRingBuffer  写入环形数组
func (r *RingBuffer) WriteToRingBuffer(data []byte) error {
	if atomic.LoadInt32(&r.stopFlag) != 0 {
		return ErrRingBufferClose
	}
	l := int64(len(data))
	//申请空间
	if (atomic.LoadInt64(&r.writeCursor) + l - r.eventHandler.getLastAvailableCursor()) >= r.ringBufferSize {
		return ErrRingBufferOverflow
	}
	end := atomic.AddInt64(&r.writeCursor, l)
	start := end - l
	//写入ring
	cut := end & r.ringBufferShift
	if cut >= l {
		copy(r.ring[cut-l:cut], data)
	} else {
		copy(r.ring[:cut], data[l-cut:])
		copy(r.ring[r.ringBufferSize+cut-l:], data[:l-cut])
	}
	//自旋等待提交
	for !atomic.CompareAndSwapInt64(&r.availableCursor, start, end) {
	}
	//通知
	select {
	case r.eventHandler.noticeChan <- struct{}{}:
	default:
	}
	return nil

}

//getBytes 从ring读取切片
func (r *RingBuffer) getBytes(start, end int64) []byte {
	s := start & r.ringBufferShift
	e := end & r.ringBufferShift
	if e >= s {
		return r.ring[s:e]
	}
	l := end - start
	buf := make([]byte, l)
	copy(buf[:l-e], r.ring[s:])
	copy(buf[l-e:], r.ring[:e])
	return buf
}

//EventHandler 消费者
type EventHandler struct {
	ctx context.Context

	Ring *RingBuffer

	noticeChan chan struct{} //信号

	processors     []*Processor
	processorQueue chan chan Event //事件队列
	processorTask  func(ProcessorJob)
	bytesDecode    func([]byte) (int64, []byte)

	waitForWriteDuration time.Duration

	logger *Logger
	WaitGroupWrapper
}

//NewEventHandler 初始化
func NewEventHandler(ctx context.Context, task func(ProcessorJob), decode func([]byte) (int64, []byte)) *EventHandler {
	logger, _ := NewLogger(DebugLevel, "")
	logger.SetMark("EventHandler")
	if task == nil {
		logger.Fatal("NewEventHandler|缺少func。")
	}
	if decode == nil {
		logger.Fatal("NewEventHandler|缺少func。")
	}
	e := &EventHandler{
		ctx:        ctx,
		Ring:       &RingBuffer{ringBufferShift: 67108864 - 1, ringBufferSize: 67108864, ring: make([]byte, 67108864)},
		noticeChan: make(chan struct{}),

		processors:     make([]*Processor, runtime.NumCPU()),
		processorQueue: make(chan chan Event, runtime.NumCPU()),
		processorTask:  task,
		bytesDecode:    decode,

		waitForWriteDuration: 5 * time.Microsecond,

		logger: logger,
	}
	e.Ring.eventHandler = e
	for i := 0; i < runtime.NumCPU(); i++ {
		e.processors[i] = &Processor{eventHandler: e, eventChan: make(chan Event)}
		e.Wrap(e.processors[i].run)
		e.processorQueue <- e.processors[i].eventChan
	}
	return e
}

//Run 运行
func (e *EventHandler) Run() {
	e.logger.Info("Run|启动……")
	var readCursor int64 //readCursor:EventHandler读取位置
	processorJobMap := make(map[int64]ProcessorJob, 4096)
	for {
		select {
		case <-e.noticeChan:
			//读 ringbuffer 一个消费者模式
			starTime := time.Now()
			for {
				if e.Ring.getAvailableCursor() <= readCursor {
					break
				}
				if time.Now().Sub(starTime) > e.waitForWriteDuration {
					break
				}
				oldCursor := readCursor
				length := int64(BytesToUint32(e.Ring.getBytes(oldCursor, oldCursor+4)))
				readCursor += length
				id, buf := e.bytesDecode(e.Ring.getBytes(oldCursor, readCursor))
				if pj, ok := processorJobMap[id]; ok {
					pj.Data = append(pj.Data, buf)
				} else {
					npj := ProcessorJob{
						ID:         id,
						lastCursor: readCursor,
						Data:       make([][]byte, 1, 100), //TODO 优化
					}
					npj.Data[0] = buf
					processorJobMap[id] = npj
				}
			}
			//推 数据及函数 以实现单协程处理。
			for key := range processorJobMap {
				w := <-e.processorQueue
				w <- processorJobMap[key]
				delete(processorJobMap, key)
			}
		case <-e.ctx.Done():
			atomic.AddInt32(&e.Ring.stopFlag, 1)
			e.logger.Info("Run|等待子协程关闭。")
			for _, v := range e.processors {
				close(v.eventChan)
			}
			e.Wait()
			e.logger.Info("Run|关闭。")
			close(e.processorQueue)
			e.processors = nil
			processorJobMap = nil
			return
		}
	}
}

//
func (e *EventHandler) getLastAvailableCursor() int64 {
	var last int64
	for _, v := range e.processors {
		temp := atomic.LoadInt64(&v.availableCursor)
		if last < temp || last == 0 {
			last = temp
		}
	}
	return last
}

//Processor 处理器 数据与函数同时提供，通过调度，避免产生竞争。
type Processor struct {
	eventHandler    *EventHandler
	do              func(ProcessorJob) //不能有IO及协程切换，最好纯计算，IO写在work_queue处理。
	eventChan       chan Event
	pad             [56]byte //pad*:凑够64字节CPU缓存行
	availableCursor int64    //提交点
}

//run 运行
func (p *Processor) run() {
	for {
		j, ok := <-p.eventChan
		if !ok {
			break
		}
		pj := j.(ProcessorJob)
		p.eventHandler.processorTask(pj)
		//处理完毕后提交
		atomic.StoreInt64(&p.availableCursor, pj.lastCursor)
		p.eventHandler.processorQueue <- p.eventChan
	}
}

//ProcessorJob 处理器任务
type ProcessorJob struct {
	ID         int64
	Data       [][]byte
	pad        [56]byte //pad*:凑够64字节CPU缓存行
	lastCursor int64    //供Ring写入时查是否越界
}
