package domi

import (
	"errors"
	"runtime/debug"
	"sync"
	"time"

	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
)

//Serial 串行处理
//一个协程处理一个serial,以避免锁的问题，同时减少协程切换，提高cpu利用率。
//按时间轮来分配cpu，不适用于cpu密集计算或长IO场景。
type Serial struct {
	SnippetDuration  time.Duration //时间间隔
	contextMQHandler []interface{}
	channelMap       map[uint16]uint16
	util.RingBuffer
	RingBufferSize uint64 //缓存大小

	bags []*bag
	*Node

	unsubscribeChan chan []uint16

	stopChan  chan struct{} //退出信号
	closeOnce sync.Once
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
		s.RingBufferSize = 1048576 //默认2^20
	}
	s.InitRingBuffer(s.RingBufferSize)
	s.contextMQHandler = make([]interface{}, 65536)
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
		case <-s.Node.sidecar.Ctx.Done():
			s.Close()
		case <-s.stopChan:
			//停止输入
			s.SetState(util.StateWork)
			snippet.Stop()
			for k := range s.channelMap {
				s.sidecar.HandleFunc(k, nil)
			}
			time.Sleep(5 * s.SnippetDuration)
			s.assignmentTask()
			//5分钟后强制释放，如果部分Handler时间超过5分钟，最后释放时会产生异常。
			time.AfterFunc(5*time.Minute, func() {
				s.contextMQHandler = nil
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
		s.contextMQHandler[fs.GetFrameType()].(func(*ContextMQ))(c)
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

//Subscribe 订阅频道，需在serial.run()运行前执行（线程不安全）。
func (s *Serial) Subscribe(channel uint16, f func(*ContextMQ)) error {
	if s.HasWork() {
		return errors.New("Subscribe|Serial已运行,需在serial.run()运行前执行。")
	}
	n := s.Node
	s.contextMQHandler[channel] = s.abnormal(f)
	n.sidecar.SetChannel(uint16(n.sidecar.MachineID), channel, 3)
	n.sidecar.HandleFunc(channel, s.serialProcessWrapper)
	s.channelMap[channel] = channel
	return nil
}

//SubscribeRace 订阅频道,某一频道收到信息后，执行f，需在serial.run()运行前执行（线程不安全）。
func (s *Serial) SubscribeRace(channels []uint16, f func(*ContextMQ)) error {
	if s.HasWork() {
		return errors.New("SubscribeRace|Serial已运行,需在serial.run()运行前执行。")
	}
	n := s.Node
	for _, a := range channels {
		s.contextMQHandler[a] = s.abnormal(f)
		n.sidecar.SetChannel(uint16(n.sidecar.MachineID), a, 3)
		n.sidecar.HandleFunc(a, s.serialProcessWrapper)
		s.channelMap[a] = a
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
	fs           func(*ContextMQs)
	channels     []uint16
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

//SubscribeAll 订阅频道,全部频道都收到信息后，执行f，需在serial.run()运行前执行（线程不安全）。
func (s *Serial) SubscribeAll(channels []uint16, fs func(*ContextMQs)) error {
	if s.HasWork() {
		return errors.New("SubscribeAll|Serial已运行,需在serial.run()运行前执行。")
	}
	n := s.Node
	b := &bag{
		channelsSize: len(channels),
		fs:           fs,
		buf:          make([][]byte, len(channels)),
		channels:     channels,
	}
	s.bags = append(s.bags, b)
	ba := bagAndIndex{}
	ba.bag = b
	for i, a := range channels {
		ba.index = i
		s.contextMQHandler[a] = s.abnormal(ba.serialProcessWrapper)
		n.sidecar.SetChannel(uint16(n.sidecar.MachineID), a, 3)
		n.sidecar.HandleFunc(a, s.serialProcessWrapper)
		s.channelMap[a] = a
	}
	return nil
}

//UnsubscribeGroup 退订频道
func (s *Serial) UnsubscribeGroup(channels []uint16) error {
	for _, a := range channels {
		s.sidecar.HandleFunc(a, nil)
		s.sidecar.SetChannel(uint16(s.sidecar.MachineID), a, 4)
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
