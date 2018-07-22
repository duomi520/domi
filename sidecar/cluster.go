package sidecar

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
)

//定义状态
const (
	StateDie uint32 = 1 + iota
	StateWork
	StatePause
)

//AskAppoint 指定请求
func (c *cluster) AskAppoint(target int, fs *transport.FrameSlice) error {
	var err error
	s := atomic.LoadUint32(&c.sessionsState[target])
	m := (*member)(atomic.LoadPointer(&c.sessions[target]))
	if m != nil && s == StateWork {
		err = m.WriteFrameDataPromptly(fs)
	} else {
		err = fmt.Errorf("AskAppoint|target %d ： m==nil or state = %d", target, s)
	}
	return err
}

//AskOne 请求某一个
func (c *cluster) AskOne(channel uint16, fs *transport.FrameSlice) error {
	var err error
	v := (*[]uint16)(atomic.LoadPointer(&c.channels[channel]))
	if v != nil {
		id := int((*v)[0])
		s := atomic.LoadUint32(&c.sessionsState[id])
		m := (*member)(atomic.LoadPointer(&c.sessions[id]))
		if m != nil && s == StateWork {
			err = m.WriteFrameDataPromptly(fs)
		} else {
		}
		//TODO 均衡负载及熔断
	} else {
		fmt.Println(c.channels[:6], c.sessionsState[:6], c.sessions[:6])
		err = fmt.Errorf("Ask|channels no find %d", channel)
	}
	return err
}

//AskAll 请求所有
func (c *cluster) AskAll(channel uint16, fs *transport.FrameSlice) error {
	var err error
	v := (*[]uint16)(atomic.LoadPointer(&c.channels[channel]))
	l := len(*v)
	for i := 0; i < l; i++ {
		id := int((*v)[i])
		s := atomic.LoadUint32(&c.sessionsState[id])
		m := (*member)(atomic.LoadPointer(&c.sessions[id]))
		if m != nil && s == StateWork {
			err = m.WriteFrameDataPromptly(fs)
		}
	}
	return err
}

type idSession struct {
	id int
	ss transport.Session
}

type member struct {
	transport.Session //会话
}

type cluster struct {
	sessions      [1024]unsafe.Pointer  //*member	原子操作
	sessionsState [1024]uint32          //状态		原子操作
	channels      [65536]unsafe.Pointer //*[]uint16	原子操作

	state     uint32 //状态
	machineID uint16

	joinSessionChan   chan idSession //加入会话信号
	PutStateChan      chan uint32
	ChangeChannelChan chan [3]uint16 //id,channel,0 put,1 del
	stopChan          chan struct{}
	readyChan         chan struct{}

	*Peer
	closeOnce sync.Once
}

func newCluster(name, HTTPPort, TCPPort string, operation interface{}) (*cluster, error) {
	var err error
	c := &cluster{
		joinSessionChan:   make(chan idSession, 64),
		PutStateChan:      make(chan uint32, 64),
		ChangeChannelChan: make(chan [3]uint16, 1024),
		stopChan:          make(chan struct{}),
		readyChan:         make(chan struct{}),
	}
	c.Peer, err = newPeer(name, HTTPPort, TCPPort, operation)
	if err != nil {
		return nil, err
	}
	c.machineID = uint16(c.MachineID)
	return c, nil
}
func (c *cluster) Ready() {
	<-c.readyChan
}
func (c *cluster) Run() {
	machineIDAndState := make([]byte, 6)
	util.CopyUint16(machineIDAndState[:2], c.machineID)
	stateKey := []byte("state/xx")
	util.CopyUint16(stateKey[len(stateKey)-2:], c.machineID)
	machineIDAndChanne := make([]byte, 4)
	channelKey := []byte("channel/xx/xx")
	lenChannelKey := len(channelKey)
	err := c.initStateAndChannels()
	close(c.readyChan)
	if err != nil {
		return
	}
	for {
		select {
		//put状态
		case state := <-c.PutStateChan:
			c.state = state
			util.CopyUint32(machineIDAndState[2:], c.state)
			c.PutKey(context.TODO(), string(stateKey), string(machineIDAndState))
			if c.state == StateDie {
				c.stopCluster()
			}
		//状态修改
		case scc := <-c.StateChangeChan:
			id := util.BytesToUint16(scc[:2])
			state := util.BytesToUint32(scc[2:])
			atomic.StoreUint32(&c.sessionsState[id], state)
		//加入节点
		case ssc := <-c.joinSessionChan:
			m := &member{}
			m.Session = ssc.ss
			atomic.StorePointer(&c.sessions[ssc.id], unsafe.Pointer(m))
		//节点下线
		case ndc := <-c.NodeDeleteChan:
			id := getNodeID(ndc)
			m := atomic.LoadPointer(&c.sessions[id])
			if m != nil {
				atomic.StorePointer(&c.sessions[id], nil)
				atomic.StoreUint32(&c.sessionsState[id], 0)
			}
		//改变频道
		case pcc := <-c.ChangeChannelChan:
			util.CopyUint16(machineIDAndChanne[:2], pcc[0])
			util.CopyUint16(machineIDAndChanne[2:], pcc[1])
			copy(channelKey[lenChannelKey-5:lenChannelKey-3], machineIDAndChanne[2:])
			copy(channelKey[lenChannelKey-2:], machineIDAndChanne[:2])
			if pcc[2] == 0 {
				err = c.PutKey(context.TODO(), string(channelKey), string(machineIDAndChanne))
				if err != nil {
					fmt.Println("ERR:", err.Error())
				}
			} else {
				err = c.DeleteKey(context.TODO(), string(channelKey))
				if err != nil {
					fmt.Println("ERR:", err.Error())
				}
			}
		//频道修改
		case cpc := <-c.ChannelPutChan:
			id := util.BytesToUint16(cpc[:2])
			channel := util.BytesToUint16(cpc[2:])
			v := (*[]uint16)(atomic.LoadPointer(&c.channels[channel]))
			var channels []uint16
			if v == nil {
				channels = make([]uint16, 1)
				channels[0] = id
			} else {
				l := len(*v)
				channels = make([]uint16, l+1)
				copy(channels[:l], (*v))
				channels[l] = id
			}
			atomic.StorePointer(&c.channels[channel], unsafe.Pointer(&channels))
		//频道删除
		case cdc := <-c.ChannelDeleteChan:
			channel, id := getChannelID(cdc)
			v := (*[]uint16)(atomic.LoadPointer(&c.channels[channel]))
			if v != nil {
				l := len(*v)
				channels := make([]uint16, l)
				for i := 0; i < l; i++ {
					if (*v)[i] == id {
						if i < l-1 {
							copy(channels[i:l-1], (*v)[i+1:])
						}
						channels = channels[:l-1]
						break
					}
					channels[i] = (*v)[i]
				}
				atomic.StorePointer(&c.channels[channel], unsafe.Pointer(&channels))
			}
		//关闭
		case <-c.stopChan:
			for id := range c.sessions {
				m := (*member)(atomic.LoadPointer(&c.sessions[id]))
				if m != nil {
					m.Close()
					atomic.StorePointer(&c.sessions[id], nil)
				}
			}
			return
		}
	}
}

//TODO 一致性问题
func (c *cluster) initStateAndChannels() error {
	s, err := c.GetKey(context.TODO(), "state/")
	if err != nil {
		return err
	}
	for i := 0; i < len(s); i++ {
		id := util.BytesToUint16(s[i][:2])
		c.sessionsState[id] = util.BytesToUint32(s[i][2:])
	}
	cl, err := c.GetKey(context.TODO(), "channel/")
	if err != nil {
		return err
	}
	temp := make(map[uint16][]uint16, 128)
	for i := 0; i < len(cl); i++ {
		id := util.BytesToUint16(cl[i][:2])
		channel := util.BytesToUint16(cl[i][2:])
		v, ok := temp[channel]
		if ok {
			v = append(v, id)
			temp[channel] = v
		} else {
			tt := make([]uint16, 1)
			tt[0] = id
			temp[channel] = tt
		}
	}
	for key, value := range temp {
		c.channels[key] = unsafe.Pointer(&value)
	}
	temp = nil
	return nil
}

func (c *cluster) stopCluster() {
	c.closeOnce.Do(func() {
		close(c.stopChan)
	})
}

/*
编码
"machine/xx"	xx 2字节机器id
"state/xx"		xx 2字节机器id					值： 2字节机器id、4字节状态
"channel/xx/xx",xx/xx 2字节频道/2字节机器id		值： 2字节机器id、2字节频道
*/

//getNodeID 转换
func getNodeID(b []byte) int {
	return int(util.BytesToUint16(b[len(b)-2:]))
}

//getChannelID 转换
func getChannelID(b []byte) (int, uint16) {
	l := len(b)
	return int(util.BytesToUint16(b[l-5 : l-3])), util.BytesToUint16(b[l-2:])
}
