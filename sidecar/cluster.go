package sidecar

import (
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/duomi520/domi/transport"
)

//AskAppoint 指定请求
func (c *cluster) AskAppoint(target int, fs *transport.FrameSlice) error {
	var err error
	s := atomic.LoadInt64(&c.sessionsState[target])
	if s != StateWork {
		return fmt.Errorf("AskAppoint|member is %d,%d!=StateWork", target, s)
	}
	m := (*member)(atomic.LoadPointer(&c.sessions[target]))
	if m != nil {
		err = m.WriteFrameDataPromptly(fs)
	} else {
		err = fmt.Errorf("AskAppoint|sessions[%d] is nil", target)
	}
	return err
}

//AskGroup 请求
func (c *cluster) AskGroup(target string, fs *transport.FrameSlice) error {
	var err error
	v, ok := c.groupMap.Load(target)
	if ok {
		list := v.([]int)
		s := atomic.LoadInt64(&c.sessionsState[list[0]])
		m := (*member)(atomic.LoadPointer(&c.sessions[list[0]]))
		if m != nil && s == StateWork {
			err = m.WriteFrameDataPromptly(fs)
		} else {
		}
		//TODO 均衡负载及熔断
	} else {
		err = fmt.Errorf("AskGroup|groupMap no find %s", target)
	}
	return err
}

//ChangeOccupy 关闭
func (c *cluster) ChangeOccupy(id, i int) {
	var data [2]int
	data[0] = id
	data[1] = i
	c.occupyChan <- data
}

type memberMsg struct {
	index int
	name  string
	state int64
	ss    transport.Session
}

type member struct {
	name              string
	transport.Session //会话
}

type cluster struct {
	sessions      [1024]unsafe.Pointer //member	原子操作
	sessionsState [1024]int64          //状态		原子操作
	occupy        [1024]int            //占用数
	occupySum     int                  //占用数合计
	groupMap      *sync.Map            //key:string, value:[]int	原子操作

	storeSessionChan chan memberMsg //加入会话信号
	occupyChan       chan [2]int    //占用数信号

	*Peer
	closeOnce sync.Once
	stopChan  chan struct{}
}

func newCluster(name, HTTPPort, TCPPort string, operation interface{}) (*cluster, error) {
	var err error
	c := &cluster{
		occupySum:        0,
		groupMap:         &sync.Map{},
		storeSessionChan: make(chan memberMsg, 1024),
		occupyChan:       make(chan [2]int, 1024),
		stopChan:         make(chan struct{}),
	}
	c.Peer, err = newPeer(name, HTTPPort, TCPPort, operation)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (c *cluster) Run() {
	temp := false
	for {
		select {
		//加入会话
		case ssc := <-c.storeSessionChan:
			m := &member{name: ssc.name}
			m.Session = ssc.ss
			atomic.StorePointer(&c.sessions[ssc.index], unsafe.Pointer(m))
			c.occupy[ssc.index] = 0
			atomic.StoreInt64(&c.sessionsState[ssc.index], ssc.state)
			c.joinGroup(ssc.name, ssc.index)
		//改变状态
		case epc := <-c.eventPutChan:
			id, name, state := getIDAndStateByString(epc)
			s := atomic.LoadInt64(&c.sessionsState[id])
			if s != StateDie {
				c.sessionsState[id] = state
				switch state {
				case StateDie:
					c.leaverGroup(name, id)
					c.checkDialClose(id)
				case StateWork:
					if temp {
						c.joinGroup(name, id)
					}
					temp = true
				case StatePause:
					c.leaverGroup(name, id)
				default:
				}
			}
		//机器下线
		case edc := <-c.eventDeleteChan:
			id := getNodeIDByName(string(edc))
			if c.occupy[id] != 0 {
				c.occupySum = c.occupySum - c.occupy[id]
				c.occupy[id] = 0
			}
			p := atomic.LoadPointer(&c.sessions[id])
			if p != nil {
				key := (*member)(p).name
				c.leaverGroup(key, id)
				atomic.StorePointer(&c.sessions[id], nil)
				atomic.StoreInt64(&c.sessionsState[id], StateDie)
			}
		//占用数修改
		case occ := <-c.occupyChan:
			c.occupy[occ[0]] += occ[1]
			c.occupySum += occ[1]
			if c.occupySum == 0 {
				c.checkDialClose(occ[0])
			}
		//关闭
		case <-c.stopChan:
			for id := range c.sessions {
				m := (*member)(atomic.LoadPointer(&c.sessions[id]))
				if m != nil {
					atomic.StorePointer(&c.sessions[id], nil)
					m.Close()
				}
			}
			return
		}
	}
}

func (c *cluster) leaverGroup(name string, id int) {
	if v, ok := c.groupMap.Load(name); ok {
		current := v.([]int)
		l := len(current)
		newList := make([]int, l)
		for i := 0; i < l; i++ {
			if current[i] == id {
				copy(newList[i:l-1], current[i+1:l])
				newList = newList[:l-1]
				break
			}
			newList[i] = current[i]
		}
		if len(newList) == 0 {
			c.groupMap.Delete(name)
		} else {
			c.groupMap.Store(name, newList)
		}
	}
}
func (c *cluster) joinGroup(name string, id int) {
	//fmt.Println("ddd", name, id, c.occupySum, c.occupy[:5], c.sessionsState[:5], c.sessions[:5])
	if v, ok := c.groupMap.Load(name); ok {
		current := v.([]int)
		current = append(current, id)
		c.groupMap.Store(name, current)

	} else {
		l := make([]int, 1, 64)
		l[0] = id
		c.groupMap.Store(name, l)
	}
}

func (c *cluster) checkDialClose(id int) {
	m := (*member)(atomic.LoadPointer(&c.sessions[id]))
	if m != nil && c.occupy[id] == 0 && c.sessionsState[id] == StateDie {
		atomic.StorePointer(&c.sessions[id], nil)
		m.Close()
	}
	//fmt.Println("ddd", c.occupySum, c.occupy[:5], c.sessionsState[:5], c.sessions[:5])
	if c.occupySum == 0 && c.sessionsState[c.MachineID] == StateDie {
		c.stopCluster()
	}
}
func (c *cluster) stopCluster() {
	c.closeOnce.Do(func() {
		close(c.stopChan)
	})
}
