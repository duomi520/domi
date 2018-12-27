package sidecar

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"unsafe"

	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
)

//Specify 指定请求
func (c *cluster) Specify(id, channel uint16, fs transport.FrameSlice, reject uint16) {
	s := atomic.LoadUint32(&c.sessionsState[id])
	m := (*member)(atomic.LoadPointer(&c.sessions[id]))
	if m != nil && s == util.StateWork {
		m.WriteFrameDataToCache(fs, reject)
		return
	}
	c.handler.ErrorRoute(reject, 922, errors.New("Specify|请求失败！"))
}

//AskOne 请求某一个
func (c *cluster) AskOne(channel uint16, fs transport.FrameSlice, reject uint16) {
	b := (*bucket)(atomic.LoadPointer(&c.channels[channel]))
	if b != nil {
		for {
			var count uint32
			id, l := b.next()
			s := atomic.LoadUint32(&c.sessionsState[id])
			m := (*member)(atomic.LoadPointer(&c.sessions[id]))
			if m != nil && s == util.StateWork {
				m.WriteFrameDataToCache(fs, reject)
				return
			}
			count++
			if count > l {
				c.handler.ErrorRoute(reject, 923, fmt.Errorf("AskOne|bucket.sets未发现 %d,id=%d,l=%d", channel, id, l))
				return
			}
		}
	}
	c.handler.ErrorRoute(reject, 924, fmt.Errorf("AskOne|bucket 未发现频道 %d", channel))

}

//AskAll 请求所有
func (c *cluster) AskAll(channel uint16, fs transport.FrameSlice, reject uint16) {
	b := (*bucket)(atomic.LoadPointer(&c.channels[channel]))
	if b != nil {
		l := len(b.sets)
		for i := 0; i < l; i++ {
			id := b.sets[i]
			s := atomic.LoadUint32(&c.sessionsState[id])
			m := (*member)(atomic.LoadPointer(&c.sessions[id]))
			if m != nil && s == util.StateWork {
				m.WriteFrameDataToCache(fs, reject)
			}
		}
		return
	}
	c.handler.ErrorRoute(reject, 925, fmt.Errorf("AskAll|bucket 未发现频道 %d", channel))
}

type member struct {
	transport.Session //会话
}

type cluster struct {
	handler *transport.Handler

	sessions      [1024]unsafe.Pointer  //*member	原子操作
	sessionsState [1024]uint32          //状态		原子操作
	channels      [65536]unsafe.Pointer //*bucket	原子操作

	machineID uint16

	state     uint32 //状态
	readyChan chan struct{}

	*Peer
	Logger *util.Logger
}

func newCluster(h *transport.Handler, name, HTTPPort, TCPPort string, operation interface{}, logger *util.Logger) (*cluster, error) {
	var err error
	c := &cluster{
		handler:   h,
		readyChan: make(chan struct{}),
		Logger:    logger,
	}
	c.Peer, err = newPeer(name, HTTPPort, TCPPort, operation)
	if err != nil {
		return nil, err
	}
	c.machineID = uint16(c.MachineID)
	return c, nil
}

//Init 初始化
func (c *cluster) Init() {
}
func (c *cluster) WaitInit() {
	<-c.readyChan
}
func (c *cluster) Run() {
	//xx machineID xxxx State xx 流转控制
	stateValue := make([]byte, 8)
	util.CopyUint16(stateValue[:2], c.machineID)
	stateKey := []byte("state/xx")
	util.CopyUint16(stateKey[len(stateKey)-2:len(stateKey)], c.machineID)
	//Value: xx machineID xx channel xx 流转控制
	channelValue := make([]byte, 4)
	//Key:	xx 2字节频道 /xx 2字节机器id
	channelKey := []byte("channel/xx/xx")
	lenChannelKey := len(channelKey)
	err := c.initStateAndChannels()
	close(c.readyChan)
	if err != nil {
		c.Logger.Fatal("Run|", err.Error())
		return
	}
	for {
		select {
		//频道
		case cc := <-c.ChannelChan:
			switch cc.operation {
			case 1: //频道修改
				v := (*bucket)(atomic.LoadPointer(&c.channels[cc.channel]))
				nb := newBucket()
				if v == nil {
					nb.sets = append(nb.sets, cc.id)
					nb.cursorAndLenght = setCursorAndLenght(0, 1)
				} else {
					nb.add(v.sets, cc.id)
				}
				atomic.StorePointer(&c.channels[cc.channel], unsafe.Pointer(nb))
			case 2: //频道删除
				v := (*bucket)(atomic.LoadPointer(&c.channels[cc.channel]))
				if v != nil {
					nb := newBucket()
					nb.remove(v.sets, cc.id)
					if nb.cursorAndLenght != 0 {
						atomic.StorePointer(&c.channels[cc.channel], unsafe.Pointer(nb))
					} else {
						atomic.StorePointer(&c.channels[cc.channel], nil)
					}
				}
			case 3: //PUT
				util.CopyUint16(channelValue[:2], cc.id)
				util.CopyUint16(channelValue[2:4], cc.channel)
				copy(channelKey[lenChannelKey-5:lenChannelKey-3], channelValue[2:4])
				copy(channelKey[lenChannelKey-2:], channelValue[:2])
				if err = c.PutKey(context.TODO(), string(channelKey), string(channelValue)); err != nil {
					c.Logger.Error("Run|", err.Error())
				}

			case 4: //Delete
				util.CopyUint16(channelValue[:2], cc.id)
				util.CopyUint16(channelValue[2:4], cc.channel)
				copy(channelKey[lenChannelKey-5:lenChannelKey-3], channelValue[2:4])
				copy(channelKey[lenChannelKey-2:], channelValue[:2])
				if err = c.DeleteKey(context.TODO(), string(channelKey)); err != nil {
					c.Logger.Error("Run|", err.Error())
				}
			}

		//状态
		case sc := <-c.StateChan:
			switch sc.operation {
			case 1:
				atomic.StoreUint32(&c.sessionsState[sc.id], sc.state)
			case 3: //PUT
				if c.state != util.StateDie {
					c.state = sc.state
					util.CopyUint32(stateValue[2:6], c.state)
					c.PutKey(context.TODO(), string(stateKey), string(stateValue))
					if sc.state == util.StateDie {
						//关闭
						//fmt.Println(c.MachineID, c.sessions[:4], c.sessionsState[:4], c.state)
						for id := range c.sessions {
							m := (*member)(atomic.LoadPointer(&c.sessions[id]))
							if m != nil {
								m.Close()
								atomic.StorePointer(&c.sessions[id], nil)
							}
						}
						c.Logger.Info("Run|cluster关闭。")
						return
					}
				}
			}
		//节点
		case nc := <-c.NodeChan:
			switch nc.operation {
			case 2: //下线
				m := atomic.LoadPointer(&c.sessions[nc.id])
				if m != nil {
					atomic.StorePointer(&c.sessions[nc.id], nil)
				}
				atomic.StoreUint32(&c.sessionsState[nc.id], 0)
			case 3: //连接
				m := &member{}
				m.Session = nc.ss
				atomic.StorePointer(&c.sessions[nc.id], unsafe.Pointer(m))
			}
		}
	}
}

//TODO 优化
func (c *cluster) initStateAndChannels() error {
	s, err := c.GetKey(context.TODO(), "state/")
	if err != nil {
		return err
	}
	for i := 0; i < len(s); i++ {
		id := util.BytesToUint16(s[i][:2])
		c.sessionsState[id] = util.BytesToUint32(s[i][2:6])
	}
	cl, err := c.GetKey(context.TODO(), "channel/")
	if err != nil {
		return err
	}
	temp := make(map[uint16]*bucket, 128)
	for i := 0; i < len(cl); i++ {
		id := util.BytesToUint16(cl[i][:2])
		channel := util.BytesToUint16(cl[i][2:])
		v, ok := temp[channel]
		if ok {
			v.sets = append(v.sets, id)
			v.cursorAndLenght = setCursorAndLenght(0, uint32(len(v.sets)))
		} else {
			nb := newBucket()
			nb.sets = append(nb.sets, id)
			nb.cursorAndLenght = setCursorAndLenght(0, 1)
			temp[channel] = nb
		}
	}
	for {
		select {
		case <-c.StateChan:
			return errors.New("initStateAndChannels|StateChan 检测到不同步。")
		default:
			goto end1
		}
	}
end1:
	for {
		select {
		case <-c.ChannelChan:
			return errors.New("initStateAndChannels|ChannelChan 检测到不同步。")
		default:
			goto end2
		}
	}
end2:
	for key, value := range temp {
		c.channels[key] = unsafe.Pointer(value)
	}
	temp = nil
	return nil
}

/*
编码
key: "machine/xx"	xx 2字节机器id
key: "state/xx"		xx 2字节机器id						值： 2字节机器id、4字节状态
key: "channel/xx/xx",xx/xx 2字节频道/2字节机器id		值： 2字节机器id、2字节频道
*/

type stateMsg struct {
	id        uint16
	state     uint32
	operation uint16
}

func bytesTOStateMsg(b []byte) stateMsg {
	var sm stateMsg
	sm.id = util.BytesToUint16(b[:2])
	sm.state = util.BytesToUint32(b[2:6])
	return sm
}

//SetState 设置状态
func (c *cluster) SetState(s uint32) {
	var sm stateMsg
	sm.state = s
	sm.operation = 3
	c.StateChan <- sm
}

//GetState 读取状态
func (c *cluster) GetState() uint32 {
	return c.state
}

type nodeMsg struct {
	id        uint16
	ss        transport.Session
	operation uint16
}

func getNodeID(b []byte) uint16 {
	return util.BytesToUint16(b[len(b)-2:])
}

type channelMsg struct {
	id, channel, operation uint16
}

func keyTOChannelMsg(b []byte) channelMsg {
	var cm channelMsg
	l := len(b)
	cm.channel = util.BytesToUint16(b[l-5 : l-3])
	cm.id = util.BytesToUint16(b[l-2 : l])
	return cm
}

func valueTOChannelMsg(b []byte) channelMsg {
	var cm channelMsg
	cm.id = util.BytesToUint16(b[:2])
	cm.channel = util.BytesToUint16(b[2:4])
	return cm
}

//SetChannel 设置频道
func (c *cluster) SetChannel(id, channel, operation uint16) {
	var cm channelMsg
	cm.id = id
	cm.channel = channel
	cm.operation = operation
	c.ChannelChan <- cm
}

type bucket struct {
	cursorAndLenght uint64
	sets            []uint16
}

func setCursorAndLenght(cursor, lenght uint32) uint64 {
	return (uint64(lenght) << 32) | uint64(cursor)
}

func getCursorAndLenght(d uint64) (uint32, uint32) {
	return uint32(d), uint32(uint64(d) >> 32)
}

func newBucket() *bucket {
	nb := &bucket{
		cursorAndLenght: 0,
		sets:            make([]uint16, 0, 64),
	}
	return nb
}

func (b *bucket) add(base []uint16, id uint16) {
	b.sets = append(b.sets, base...)
	b.sets = append(b.sets, id)
	b.cursorAndLenght = setCursorAndLenght(0, uint32(len(b.sets)))
}

//TODO 优化性能
func (b *bucket) remove(base []uint16, id uint16) {
	l := len(base)
	b.sets = append(b.sets, base...)
	for i := 0; i < l; i++ {
		if base[i] == id {
			if i < l-1 {
				copy(b.sets[i:l-1], b.sets[i+1:])
			}
			b.sets = b.sets[:l-1]
			break
		}
	}
	b.cursorAndLenght = setCursorAndLenght(0, uint32(len(b.sets)))
}

func (b *bucket) next() (uint16, uint32) {
	d := atomic.AddUint64(&b.cursorAndLenght, 1)
	cursor, lenght := getCursorAndLenght(d)
	//很可能0被多次分配，不影响。
	if cursor >= lenght {
		cursor = 0
		d = setCursorAndLenght(cursor, lenght)
		atomic.StoreUint64(&b.cursorAndLenght, d)
	}
	return b.sets[cursor], lenght
}
