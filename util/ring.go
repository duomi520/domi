package util

import (
	"errors"
	"runtime"
	"sync/atomic"
)

//定义状态
const (
	StateDie uint32 = 1 + iota
	StateWork
	StatePause
)

//ErrRingBufferOverflow 环形数组已满。
var ErrRingBufferOverflow = errors.New("util.ErrRingBufferOverflow|环形数组已满。")

//ErrRingBufferClose 环形数组已关闭。
var ErrRingBufferClose = errors.New("util.ErrRingBufferClose|环形数组已关闭。")

//ErrWriteToRingBufferData 写入环形数组的长度小于4。
var ErrWriteToRingBufferData = errors.New("util.ErrWriteToRingBufferData|写入环形数组的长度小于4。")

//RingBuffer 环形数组
//多生产者，单消费者模式。
type RingBuffer struct {
	ringBufferShift uint64 //求余 设X对Y求余，Y等于2^N，公式为：X & (2^N - 1)
	ringBufferSize  uint64 //2^26 67108864 64M
	state           uint32
	ring            []byte
	submitRing      []byte    //写入后提交位置。
	_padding1       [8]uint64 //凑够64字节CPU缓存行
	askCursor       uint64    //申请写入位置
	_padding2       [8]uint64
	availableCursor uint64 //已消费位置
}

//InitRingBuffer  初始化环形数组
func (r *RingBuffer) InitRingBuffer(n uint64) {
	r.ringBufferSize = minQuantity(n)
	r.ringBufferShift = r.ringBufferSize - 1
	r.ring = make([]byte, r.ringBufferSize)
	r.submitRing = make([]byte, r.ringBufferSize)
}

//ReleaseRingBuffer 释放
func (r *RingBuffer) ReleaseRingBuffer() {
	r.SetState(StateDie)
	r.ring = nil
	r.submitRing = nil
}

//WriteToRingBuffer  多生产者,写入环形数组
func (r *RingBuffer) WriteToRingBuffer(data []byte) error {
	if !r.HasWork() {
		return ErrRingBufferClose
	}
	if len(data) < 4 {
		return ErrWriteToRingBufferData
	}
	l := uint64(len(data))
	var end uint64
	//申请空间
	for {
		ask := atomic.LoadUint64(&r.askCursor)
		end = ask + l
		if (end - atomic.LoadUint64(&r.availableCursor)) >= r.ringBufferSize {
			return ErrRingBufferOverflow
		}
		if atomic.CompareAndSwapUint64(&r.askCursor, ask, end) {
			break
		}
		runtime.Gosched()
	}
	//写入Ring
	cut := end & r.ringBufferShift
	if cut >= l {
		copy(r.ring[cut-l:cut], data)
	} else {
		copy(r.ring[:cut], data[l-cut:])
		copy(r.ring[r.ringBufferSize+cut-l:], data[:l-cut])
	}
	//提交标识位
	r.submitRing[cut] = 1
	return nil
}

//ReadFromRingBuffer 单消费者模式 读出环形数组 前4个字节必须为长度。
func (r *RingBuffer) ReadFromRingBuffer() ([]byte, uint64) {
	available := atomic.LoadUint64(&r.availableCursor)
	if atomic.LoadUint64(&r.askCursor) > available {
		end := available + uint64(BytesToUint32(r.getBytes(available, available+4)))
		if r.submitRing[end&r.ringBufferShift] == 1 {
			return r.getBytes(available, end), end
		}
	}
	return nil, 0
}

//SetAvailableCursor 单消费者模式，提交已消费位置
func (r *RingBuffer) SetAvailableCursor(val uint64) {
	r.submitRing[val&r.ringBufferShift] = 0
	atomic.StoreUint64(&r.availableCursor, val)
}

//HasWork 是否工作
func (r *RingBuffer) HasWork() bool {
	return atomic.LoadUint32(&r.state) == StateWork
}

//SetState 设置状态
func (r *RingBuffer) SetState(s uint32) {
	atomic.StoreUint32(&r.state, s)
}

//getBytes 从ring读取切片
func (r *RingBuffer) getBytes(start, end uint64) []byte {
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

//minQuantity 到最近的2的倍数
func minQuantity(v uint64) uint64 {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v |= v >> 32
	v++
	return v
}
