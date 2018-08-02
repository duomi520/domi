package domi

import (
	"errors"
	"github.com/duomi520/domi/transport"
	"sync/atomic"
	"time"
)

//Serial 串行处理
type Serial struct {
	flag int32 //标志
	n    *Node
	ring *RingBuffer
}

//NewSerial 新建
func NewSerial(n *Node) *Serial {
	s := &Serial{n: n}
	s.ring = NewRingBuffer(12) //2^12 4096
	return s
}

//WorkFunc 任务
func (s *Serial) WorkFunc() {
	defer atomic.StoreInt32(&s.flag, 0)
	v := s.ring.ReadRingBuffer()
	for v != nil {
		sw := v.(sessionWrapper)
		sw.fun(sw.cmq)
		v = s.ring.ReadRingBuffer()
	}
}

//Close 关闭
func (s *Serial) Close() {
	s.ring = nil
	s.n = nil
	s = nil
}

type sessionWrapper struct {
	fun func(*ContextMQ)
	cmq *ContextMQ
}

//SerialProcess 多频道订阅，用单一协程处理多个Process，避免多线程下竟态问题,以单线程的方式写代码。
func (s *Serial) SerialProcess(channel uint16, f func(*ContextMQ)) {
	s.n.sidecar.SetChannel(uint16(s.n.sidecar.MachineID), channel, 3)
	pw := processWrapper{
		serial: s,
		n:      s.n,
		f:      f,
	}
	s.n.sidecar.HandleFunc(channel, pw.serialProcessWrapper)
}
func (pw processWrapper) serialProcessWrapper(s transport.Session) error {
	fs := s.GetFrameSlice()
	c := &ContextMQ{
		Node:     pw.n,
		response: s.WriteFrameDataPromptly,
	}
	c.Request = append(c.Request, fs.GetData()...)
	c.ex = append(c.ex, fs.GetExtend()...)
	sw := sessionWrapper{fun: pw.f,
		cmq: c}
	err := pw.serial.ring.WriteToRingBuffer(sw)
	if err != nil {
		pw.serial.n.Logger.Error("serialProcessWrapper|", err.Error())
	}
	if atomic.LoadInt32(&pw.serial.flag) == 0 {
		pw.serial.n.sidecar.Dispatcher.JobQueue <- pw.serial
	}
	return nil
}

//RingBuffer 环形数组  多生产者，单消费者。
type RingBuffer struct {
	ringBufferShift int64 //求余 设X对Y求余，Y等于2^N，公式为：X & (2^N - 1)
	ringBufferSize  int64 //2^12 4096
	ring            []interface{}
	writeCursor     int64
	availableCursor int64 //已提交位置
}

//NewRingBuffer 新建
func NewRingBuffer(n int) *RingBuffer {
	r := &RingBuffer{}
	var a int64 = 1
	for i := 0; i < n; i++ {
		a = a * 2
	}
	r.ringBufferShift = a - 1
	r.ringBufferSize = a
	r.ring = make([]interface{}, a)
	return r
}

//WriteToRingBuffer  写入环形数组
func (r *RingBuffer) WriteToRingBuffer(data interface{}) error {
	//申请空间
	num := 0
loop:
	if (atomic.LoadInt64(&r.writeCursor) + 1 - atomic.LoadInt64(&r.availableCursor)) >= r.ringBufferSize {
		time.Sleep(5 * time.Microsecond)
		num++
		if num < 10 {
			goto loop
		}
		return errors.New("WriteToRingBuffer|环形数组已满。")
	}
	end := atomic.AddInt64(&r.writeCursor, 1) - 1
	//写入ring
	cut := end & r.ringBufferShift
	r.ring[cut] = data
	return nil
}

//ReadRingBuffer 从ring读取切片
func (r *RingBuffer) ReadRingBuffer() interface{} {
	read := atomic.LoadInt64(&r.availableCursor)
	if read < atomic.LoadInt64(&r.writeCursor) {
		cut := read & r.ringBufferShift
		data := r.ring[cut]
		r.ring[cut] = nil
		atomic.AddInt64(&r.availableCursor, 1)
		return data
	}
	return nil
}
