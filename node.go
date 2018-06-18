package domi

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/duomi520/domi/sidecar"
	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
)

//BroadcastNews 消息
type BroadcastNews struct {
	Channel uint16 //频道
	Data    []byte
}

//PubSubNews 消息
type PubSubNews struct {
	Node    int64  //节点
	User    uint16 //订阅者
	Channel uint16 //频道
}

//Node 节点
type Node struct {
	sidecar       *sidecar.Sidecar
	joinChan      chan PubSubNews
	broadcastChan chan BroadcastNews
	leaveChan     chan PubSubNews
	stopChan      chan struct{}      //退出信号
	workFuncMap   [65536]interface{} //key:uint16  	value:func(*ContextMQ)
}

//NewNode 新建
func NewNode(ctx context.Context, name, HTTPPort, TCPPort string, endpoints []string) *Node {
	n := &Node{
		joinChan:      make(chan PubSubNews, 64),
		broadcastChan: make(chan BroadcastNews, 64),
		leaveChan:     make(chan PubSubNews, 64),
		stopChan:      make(chan struct{}),
	}
	n.sidecar = sidecar.NewSidecar(ctx, name, HTTPPort, TCPPort, endpoints)
	n.sidecar.HandleFunc(transport.FrameTypeReply, n.requestReply)
	n.sidecar.HandleFunc(transport.FrameTypeJoinChannel, n.joinChannel)
	n.sidecar.HandleFunc(transport.FrameTypeLeaveChannel, n.leaveChannel)
	return n
}

//requestReply
func (n *Node) requestReply(s transport.Session) {
	ex := s.GetFrameSlice().GetExtend()
	id := util.BytesToUint16(ex)
	v := n.workFuncMap[id]
	if v == nil {
		n.sidecar.Logger.Error("requestReply|userMap:", id)
		return
	}
	f := v.(func(*ContextMQ))
	ctx := getCtx(s)
	ctx.N = n
	f(ctx)
	n.workFuncMap[id] = nil
}
func (n *Node) joinChannel(s transport.Session) {
	data := s.GetFrameSlice().GetData()
	var d PubSubNews
	d.Node = util.BytesToInt64(data[:8])
	d.User = util.BytesToUint16(data[8:10])
	d.Channel = util.BytesToUint16(data[10:])
	n.joinChan <- d
}
func (n *Node) leaveChannel(s transport.Session) {
	data := s.GetFrameSlice().GetData()
	var d PubSubNews
	d.Node = util.BytesToInt64(data[:8])
	d.User = util.BytesToUint16(data[8:10])
	d.Channel = util.BytesToUint16(data[10:])
	n.leaveChan <- d
}

//HandleC 封装
func (n *Node) HandleC(ft uint16, f func(*ContextMQ)) {
	n.workFuncMap[ft] = f
	n.sidecar.HandleFunc(ft, n.handleWrapper)
}

func (n *Node) handleWrapper(s transport.Session) {
	ft := s.GetFrameSlice().GetFrameType()
	v := n.workFuncMap[ft]
	if v == nil {
		n.sidecar.Logger.Error("handleWrapper|WorkFuncMap找不到:", ft)
		return
	}
	f := v.(func(*ContextMQ))
	ctx := getCtx(s)
	ctx.N = n
	f(ctx)
}

//Tell 单向告诉，不回复
func (n *Node) Tell(ft uint16, target string, data []byte) error {
	return n.sidecar.Send(ft, target, data, nil)
}

//Call 请求 request-reply	每个请求都带有回应地址
func (n *Node) Call(ft uint16, target string, data []byte, fu uint16, f func(*ContextMQ)) error {
	ex := util.Uint16ToBytes(fu)
	n.workFuncMap[fu] = f
	return n.sidecar.Send(ft, target, data, ex)
}

//Run 运行
func (n *Node) Run() {
	n.sidecar.Run()
}

//RunChannel 运行 publisher-subscriber
func (n *Node) RunChannel() {
	//Group 组
	type Group struct {
		Users []uint16 //订阅者
		Buf   []byte   //字节
	}
	//Channel 频道
	type Channel struct {
		group map[int64]*Group
	}
	channel := make(map[uint16]*Channel)
	//TODO 优化，改读写锁
	for {
		select {
		case jc := <-n.joinChan:
			c, ok := channel[jc.Channel]
			if !ok {
				c = &Channel{
					group: make(map[int64]*Group),
				}
				channel[jc.Channel] = c
			}
			g, ok := c.group[jc.Node]
			if !ok {
				g = &Group{}
				g.Users = make([]uint16, 0)
				g.Buf = make([]byte, 0)
				c.group[jc.Node] = g
			}
			g.Users = append(g.Users, jc.User)
			g.Buf = append(g.Buf, util.Uint16ToBytes(jc.User)...)
		//	fmt.Println("jjj:",g, jc)
		case lc := <-n.leaveChan:
			c, ok := channel[lc.Channel]
			if !ok {
				n.sidecar.Logger.Error("RunChannel|leaveChan找不到Channel：", lc.Channel)
				continue
			}
			g, ok := c.group[lc.Node]
			if !ok {
				n.sidecar.Logger.Error("RunChannel|leaveChan找不到Node：", lc.Node)
				continue
			}
			l := len(g.Users)
			if l == 0 {
				n.sidecar.Logger.Error("RunChannel|leaveChan找不到User：", lc.User)
				continue
			}
			for i := 0; i < l; i++ {
				if g.Users[i] == lc.User {
					copy(g.Users[i:l-1], g.Users[i+1:])
					g.Users = g.Users[:l-1]
					copy(g.Buf[i+i:l+l-2], g.Buf[i+i+2:])
					g.Buf = g.Buf[:l+l-2]
					break
				}
			}
		//	fmt.Println("zzz:", g, lc)
		case bc := <-n.broadcastChan:
			c, ok := channel[bc.Channel]
			if !ok {
				n.sidecar.Logger.Error("RunChannel|broadcastChan找不到Channel：", bc.Channel)
				continue
			}
			for nid, g := range c.group {
				n.sidecar.Send(bc.Channel, nid, bc.Data, g.Buf)
			}
		case <-n.stopChan:
			return
		}
	}
}

//Subscribe 订阅 返回订阅者ID  publisher-subscriber
func (n *Node) Subscribe(channel uint16, target string, user uint16, f func(*ContextMQ)) error {
	data := make([]byte, 0, 12)
	nid := n.sidecar.Peer.SnowFlakeID.GetWorkID()
	data = util.AppendInt64(data, nid)
	data = append(data, util.Uint16ToBytes(user)...)
	data = append(data, util.Uint16ToBytes(channel)...)
	n.workFuncMap[user] = f
	n.sidecar.HandleFunc(channel, n.callWorkFunc)
	err := n.sidecar.Send(transport.FrameTypeJoinChannel, target, data, nil)
	if err != nil {
		return err
	}
	return nil
}

//callWorkFunc
func (n *Node) callWorkFunc(s transport.Session) {
	ctx := getCtx(s)
	ctx.N = n
	ex := s.GetFrameSlice().GetExtend()
	num := len(ex) / 2
	for i := 0; i < num; i++ {
		id := util.BytesToUint16(ex[i+i : i+i+2])
		v := n.workFuncMap[id]
		if v == nil {
			n.sidecar.Logger.Error("callWorkFunc|userMap找不到:", id)
			continue
		}
		f := v.(func(*ContextMQ))
		ctx := getCtx(s)
		ctx.N = n
		f(ctx)
	}
}

//Unsubscribe 退订 publisher-subscriber
func (n *Node) Unsubscribe(user, channel uint16, target string) error {
	data := make([]byte, 0, 18)
	nid := n.sidecar.Peer.SnowFlakeID.GetWorkID()
	data = util.AppendInt64(data, nid)
	data = append(data, util.Uint16ToBytes(user)...)
	data = append(data, util.Uint16ToBytes(channel)...)
	n.workFuncMap[user] = nil
	return n.sidecar.Send(transport.FrameTypeLeaveChannel, target, data, nil)
}

//Publish 发布 publisher-subscriber
func (n *Node) Publish(channel uint16, data []byte) {
	news := BroadcastNews{
		Channel: channel,
		Data:    data,
	}
	n.broadcastChan <- news
}

//PipelineTarget 数据 TODO 优化json，自己编码
type PipelineTarget struct {
	Target []string
}

//Ventilator 开始 pipeline
func (n *Node) Ventilator(pipe uint16, target []string, data []byte) error {
	if len(target) < 2 {
		return errors.New("Ventilator|target小于2")
	}
	var p PipelineTarget
	p.Target = target[1:]
	vj, err := json.Marshal(p)
	if err != nil {
		return errors.New("Ventilator|json编码失败: " + err.Error())
	}
	return n.sidecar.Send(pipe, target[0], data, vj)
}

//ContextMQ 上下文
type ContextMQ struct {
	N         *Node
	FrameType uint16
	Request   []byte
	extend    []byte
	session   transport.Session
}

var localNode *Node //全局

//getCtx 取得上下文	TODO 解决slice cap问题
func getCtx(s transport.Session) *ContextMQ {
	c := &ContextMQ{
		N:         localNode,
		FrameType: s.GetFrameSlice().GetFrameType(),
		Request:   s.GetFrameSlice().GetData(),
		extend:    s.GetFrameSlice().GetExtend(),
		session:   s,
	}
	return c
}

//Reply 回复 request-reply
func (c *ContextMQ) Reply(data []byte) {
	fs := transport.NewFrameSlice(transport.FrameTypeReply, data, c.session.GetFrameSlice().GetExtend())
	c.session.WriteFrameDataPromptly(fs)
}

//Next 下一个 pipeline
func (c *ContextMQ) Next(data []byte) error {
	pipeline := &PipelineTarget{}
	if err := json.Unmarshal(c.extend, pipeline); err != nil {
		return errors.New("Next|json解码错误：" + err.Error() + "    " + string(c.extend))
	}
	if len(pipeline.Target) < 1 {
		return errors.New("Next|target小于1")
	}
	var p PipelineTarget
	p.Target = pipeline.Target[1:]
	vj, err := json.Marshal(p)
	if err != nil {
		return errors.New("Next|json编码失败: " + err.Error())
	}
	return c.N.sidecar.Send(c.FrameType, pipeline.Target[0], data, vj)
}
