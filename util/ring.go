package util

import (
	"errors"
	"sync/atomic"
)

//ErrRingBufferOverflow 环形数组已满。
var ErrRingBufferOverflow = errors.New("util.WriteToRingBuffer|环形数组已满。")

//RingBuffer 环形数组
//多生产者，单消费者模式。
type RingBuffer struct {
	ringBufferShift int64 //求余 设X对Y求余，Y等于2^N，公式为：X & (2^N - 1)
	ringBufferSize  int64 //2^26 67108864 64M
	ring            []byte
	_padding1       [8]uint64 //凑够64字节CPU缓存行
	askCursor       int64     //申请写入位置
	_padding2       [8]uint64
	submitCursor    int64 //写入后提交位置
	_padding3       [8]uint64
	availableCursor int64 //已消费位置
}

//InitRingBuffer  初始化环形数组
func (r *RingBuffer) InitRingBuffer(n int) {
	r.ringBufferShift = int64(n - 1)
	r.ringBufferSize = int64(n)
	r.ring = make([]byte, n)
}

//ReleaseRingBuffer 释放
func (r *RingBuffer) ReleaseRingBuffer() {
	r.ring = nil
}

//WriteToRingBuffer  多生产者,写入环形数组
func (r *RingBuffer) WriteToRingBuffer(data []byte) error {
	l := int64(len(data))
	//申请空间
	if (atomic.LoadInt64(&r.askCursor) + l - atomic.LoadInt64(&r.availableCursor)) >= r.ringBufferSize {
		return ErrRingBufferOverflow
	}
	end := atomic.AddInt64(&r.askCursor, l)
	start := end - l
	//写入Ring
	cut := end & r.ringBufferShift
	if cut >= l {
		copy(r.ring[cut-l:cut], data)
	} else {
		copy(r.ring[:cut], data[l-cut:])
		copy(r.ring[r.ringBufferSize+cut-l:], data[:l-cut])
	}
	//自旋等待提交
	for !atomic.CompareAndSwapInt64(&r.submitCursor, start, end) {
	}
	return nil
}

//ReadFromRingBuffer 单消费者模式 读出环形数组 前4个字节必须为长度
func (r *RingBuffer) ReadFromRingBuffer() ([]byte, int64) {
	available := atomic.LoadInt64(&r.availableCursor)
	end := available + int64(BytesToUint32(r.getBytes(available, available+4)))
	return r.getBytes(available, end), end
}

//HasData 单消费者模式，是否有数据
func (r *RingBuffer) HasData() bool {
	return atomic.LoadInt64(&r.submitCursor) > atomic.LoadInt64(&r.availableCursor)
}

//SetAvailableCursor 单消费者模式，提交已消费位置
func (r *RingBuffer) SetAvailableCursor(val int64) {
	atomic.StoreInt64(&r.availableCursor, val)
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
