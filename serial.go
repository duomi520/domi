﻿package domi

import (
	"errors"
	"runtime/debug"
	"sync"
	"time"

	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
)

var globalContextMQHandler [65536]func(*ContextMQ)

//Serial 串行处理
//一个协程处理一个serial,以避免锁的问题，同时减少协程切换，提高cpu利用率。
//按时间轮来分配cpu，不适用于cpu密集计算或长IO场景。
type Serial struct {
	SnippetDuration time.Duration //定时调用的时间间隔
	channelMap      map[uint16]uint16
	bags            []*bag

	RingBufferSize uint64 //RingBuffer缓存大小
	util.RingBuffer

	*Node

	unsubscribeChan chan []uint16
	stopChan        chan struct{} //退出信号
	closeOnce       sync.Once
}

//Close 关闭
func (s *Serial) Close() {
	s.closeOnce.Do(func() {
		close(s.stopChan)
	})
}

//Init 初始化
func (s *Serial) Init() {
	if s.SnippetDuration == 0 {
		s.SnippetDuration = 10 * time.Microsecond
	}
	if s.RingBufferSize == 0 {
		s.RingBufferSize = 2097152 //默认2^21
	}
	s.InitRingBuffer(s.RingBufferSize)
	s.channelMap = make(map[uint16]uint16, 256)
	s.bags = make([]*bag, 0, 64)
	s.unsubscribeChan = make(chan []uint16, 64)
	s.stopChan = make(chan struct{})
	s.SetState(util.StatePause)
}

//WaitInit 准备好
func (s *Serial) WaitInit() {}

//Run 定时工作
func (s *Serial) Run() {
	snippet := time.NewTicker(s.SnippetDuration)
	s.SetState(util.StateWork)
	for {
		select {
		case <-snippet.C:
			s.assignmentTask()
		case u := <-s.unsubscribeChan:
			l := len(s.bags)
			if l > 0 {
				for i := 0; i < l; i++ {
					if util.Uint16Equal(u, s.bags[i].channels) {
						copy(s.bags[i:l-1], s.bags[i+1:])
						s.bags = s.bags[:l-1]
						break
					}
				}
			}
			for _, v := range u {
				delete(s.channelMap, v)
			}
		case <-s.Node.sidecar.Ctx.Done():
			s.Close()
		case <-s.stopChan:
			//停止输入
			s.SetState(util.StateDie)
			snippet.Stop()
			for k := range s.channelMap {
				s.Unsubscribe(k)
			}
			time.Sleep(5 * s.SnippetDuration)
			s.assignmentTask()
			//5分钟后强制释放，如果部分Handler时间超过5分钟，最后释放时会产生异常。
			time.AfterFunc(5*time.Minute, func() {
				defer func() {
					if r := recover(); r != nil {
						s.Logger.Error("Run|释放时异常：", r, string(debug.Stack()))
					}
				}()
				s.channelMap = nil
				s.ReleaseRingBuffer()
				s.bags = nil
				close(s.unsubscribeChan)
			})
			return
		}
	}
}

func (s *Serial) assignmentTask() {
	c := &ContextMQ{}
	c.Node = s.Node
	data, available := s.ReadFromRingBuffer()
	for available != 0 {
		fs := transport.DecodeByBytes(data)
		c.Request = fs.GetData()
		c.ex = fs.GetExtend()
		globalContextMQHandler[fs.GetFrameType()](c)
		s.SetAvailableCursor(available)
		data, available = s.ReadFromRingBuffer()
	}
	cs := &ContextMQs{}
	cs.Node = s.Node
	for i := 0; i < len(s.bags); i++ {
		cs.Channels = s.bags[i].channels
		count := 0
		for s.bags[i].buf[count] != nil {
			count++
			if count == s.bags[i].channelsSize {
				cs.Requests = s.bags[i].buf[:s.bags[i].channelsSize]
				s.bags[i].fs(cs)
				s.bags[i].buf = s.bags[i].buf[count:]
				if len(s.bags[i].buf) == 0 {
					s.bags[i].buf = make([][]byte, s.bags[i].channelsSize)
				}
				count = 0
			}
		}
	}
}

//serialProcessWrapper
func (s *Serial) serialProcessWrapper(se transport.Session) error {
	return s.WriteToRingBuffer(se.GetFrameSlice().GetAll())
}

//abnormal 异常拦截
func (s *Serial) abnormal(f func(*ContextMQ)) func(*ContextMQ) {
	return func(c *ContextMQ) {
		defer func() {
			if r := recover(); r != nil {
				s.Logger.Error("abnormal|异常拦截：", r, string(debug.Stack()))
			}
		}()
		f(c)
	}
}

//abnormals 异常拦截
func (s *Serial) abnormals(f func(*ContextMQs)) func(*ContextMQs) {
	return func(c *ContextMQs) {
		defer func() {
			if r := recover(); r != nil {
				s.Logger.Error("abnormal|异常拦截：", r, string(debug.Stack()))
			}
		}()
		f(c)
	}
}

//Subscribe 订阅频道，需在serial运行前执行（线程不安全）。
func (s *Serial) Subscribe(channel uint16, f func(*ContextMQ)) error {
	if s.HasWork() {
		return errors.New("Subscribe|Serial已运行,需在serial运行前执行。")
	}
	globalContextMQHandler[channel] = s.abnormal(f)
	s.Node.sidecar.HandleFunc(channel, s.serialProcessWrapper)
	s.channelMap[channel] = channel
	s.Node.sidecar.SetChannel(uint16(s.Node.sidecar.MachineID), channel, 3)
	return nil
}

//SubscribeRace 订阅一组频道,某一频道收到信息后，执行f，需在serial运行前执行（线程不安全）。
func (s *Serial) SubscribeRace(channels []uint16, f func(*ContextMQ)) error {
	if s.HasWork() {
		return errors.New("SubscribeRace|Serial已运行,需在serial运行前执行。")
	}
	for _, a := range channels {
		if err := s.Subscribe(a, f); err != nil {
			return err
		}
	}
	return nil
}

//ContextMQs 上下文
type ContextMQs struct {
	*Node
	Channels []uint16
	Requests [][]byte
}

type bag struct {
	channelsSize int
	channels     []uint16
	fs           func(*ContextMQs)
	buf          [][]byte
}

type bagAndIndex struct {
	*bag
	index int
}

//serialProcessWrapper
func (bi bagAndIndex) serialProcessWrapper(c *ContextMQ) {
	data := make([]byte, len(c.Request))
	copy(data, c.Request)
	i := bi.index
	b := bi.bag
	for {
		if i >= len(b.buf) {
			b.buf = append(b.buf, make([][]byte, b.channelsSize)...)
		}
		if b.buf[i] == nil {
			b.buf[i] = data
			return
		}
		i = i + b.channelsSize
	}
}

//SubscribeAll 订阅一组频道,全部频道都收到信息后，执行f，需在serial运行前执行（线程不安全）。
func (s *Serial) SubscribeAll(channels []uint16, fs func(*ContextMQs)) error {
	if s.HasWork() {
		return errors.New("SubscribeAll|Serial已运行,需在serial运行前执行。")
	}
	b := &bag{
		channelsSize: len(channels),
		channels:     channels,
		fs:           s.abnormals(fs),
		buf:          make([][]byte, len(channels)),
	}
	s.bags = append(s.bags, b)
	ba := bagAndIndex{}
	ba.bag = b
	for i, a := range channels {
		ba.index = i
		if err := s.Subscribe(a, ba.serialProcessWrapper); err != nil {
			return err
		}
	}
	return nil
}

//UnsubscribeGroup 退订频道
func (s *Serial) UnsubscribeGroup(channels []uint16) error {
	for _, a := range channels {
		s.Unsubscribe(a)
	}
	if !s.HasWork() {
		return errors.New("UnsubscribeGroup|Serial未运行。")
	}
	select {
	case s.unsubscribeChan <- channels:
		return nil
	case <-time.After(64 * s.SnippetDuration):
		return errors.New("UnsubscribeGroup|退订频道超时。")
	}
}
