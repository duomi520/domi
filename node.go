package domi

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"

	"github.com/duomi520/domi/sidecar"
	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
)

//2^24  16777216  16M	TODO 考虑换,牺牲cpu，省内存。
const ringUint32Shift uint64 = 16777216 - 1 //求余 设X对Y求余，Y等于2^N，公式为：X & (2^N - 1)

//Node 节点
type Node struct {
	sidecar        *sidecar.Sidecar
	userFunc       [16777216]interface{} //key:uint32  	value:func(*ContextMQ)
	serverFunc     [65536]interface{}    //key:uint16  	value:func(*ContextMQ)
	cursor         uint64
	channelNodeMap map[uint16]*util.IntList //key:uint16  	value:*util.IntList
	channelUserMap map[uint16]*util.IntList //key:uint16  	value:*IntList
}

//NewNode 新建
func NewNode(ctx context.Context, name, HTTPPort, TCPPort string, endpoints []string) *Node {
	n := &Node{
		cursor:         0,
		channelNodeMap: make(map[uint16]*util.IntList),
		channelUserMap: make(map[uint16]*util.IntList),
	}
	n.sidecar = sidecar.NewSidecar(ctx, name, HTTPPort, TCPPort, endpoints)
	n.sidecar.HandleFunc(transport.FrameTypeReply, n.requestReply)
	n.sidecar.HandleFunc(transport.FrameTypeJoinChannel, n.joinChannel)
	n.sidecar.HandleFunc(transport.FrameTypeLeaveChannel, n.leaveChannel)
	n.sidecar.HandleFunc(transport.FrameTypeBusGetChannels, n.busGetChannels)
	return n
}

//Close g
func (n *Node) Close() {
}

//Run 运行
func (n *Node) Run() {
	n.sidecar.Run()
}

//nextID 下一个		TODO debug
func (n *Node) nextID() uint32 {
	new := atomic.AddUint64(&n.cursor, 1) & ringUint32Shift
	if n.userFunc[new] != nil {
		return n.nextID()
	}
	return uint32(new) - 1
}

//requestReply
func (n *Node) requestReply(s transport.Session) error {
	ex := s.GetFrameSlice().GetExtend()
	id := util.BytesToUint32(ex)
	v := n.userFunc[id]
	if v == nil {
		n.sidecar.Logger.Error("requestReply|userMap找不到:", id)
		return nil
	}
	f := v.(func(*ContextMQ))
	ctx := getCtx(s, n)
	ctx.Node = n
	f(ctx)
	n.userFunc[id] = nil
	return nil
}
func (n *Node) joinChannel(s transport.Session) error {
	data := s.GetFrameSlice().GetData()
	node := int(util.BytesToUint32(data[:4]))
	channel := util.BytesToUint16(data[4:])
	c, ok := n.channelNodeMap[channel]
	if !ok {
		c = util.NewIntList()
		n.channelNodeMap[channel] = c
	}
	c.Add(node)
	return nil
}
func (n *Node) leaveChannel(s transport.Session) error {
	data := s.GetFrameSlice().GetData()
	node := int(util.BytesToUint32(data[:4]))
	channel := util.BytesToUint16(data[4:])
	c, ok := n.channelNodeMap[channel]
	if !ok {
		return nil
	}
	if c.Remove(node) == 0 {
		delete(n.channelNodeMap, channel)
	}
	return nil
}
func (n *Node) busGetChannels(s transport.Session) error {
	data := s.GetFrameSlice().GetData()
	channel := util.BytesToUint16(data[:2])
	mids, err := n.sidecar.GetChannels(channel)
	if err != nil {
		n.sidecar.Logger.Error("busGetChannels|:", err.Error())
		return nil
	}
	nodes, ok := n.channelNodeMap[channel]
	if ok {
		nodes.Fill(mids)
	}
	return nil
}

//HandleC 封装
func (n *Node) HandleC(ft uint16, f func(*ContextMQ)) {
	n.serverFunc[ft] = f
	n.sidecar.HandleFunc(ft, n.handleWrapper)
}

func (n *Node) handleWrapper(s transport.Session) error {
	ft := s.GetFrameSlice().GetFrameType()
	v := n.serverFunc[ft]
	if v == nil {
		n.sidecar.Logger.Error("handleWrapper|serverFunc找不到:", ft)
		return nil
	}
	f := v.(func(*ContextMQ))
	ctx := getCtx(s, n)
	ctx.Node = n
	f(ctx)
	return nil
}

//Call 请求 request-reply	每个请求都带有回应id
func (n *Node) Call(ft uint16, target string, data []byte, f func(*ContextMQ)) error {
	fu := n.nextID()
	ex := make([]byte, 4)
	util.CopyUint32(ex, fu)
	n.userFunc[fu] = f
	fs := transport.NewFrameSlice(ft, data, ex)
	return n.sidecar.AskGroup(target, fs)
}

//Publish 发布 publisher-subscriber
func (n *Node) Publish(channel uint16, data []byte) {
	c, ok := n.channelNodeMap[channel]
	if ok {
		fs := transport.NewFrameSlice(channel, data, nil)
		c.Range(func(v int) {
			n.sidecar.AskAppoint(v, fs)
		})
	}
}

//Subscribe 订阅 返回订阅者ID  publisher-subscriber
func (n *Node) Subscribe(channel uint16, target int, f func(*ContextMQ)) (uint32, error) {
	n.sidecar.ChangeOccupy(target, 1)
	user := n.nextID()
	n.userFunc[user] = f
	c, ok := n.channelUserMap[channel]
	if !ok {
		c = util.NewIntList()
		n.channelUserMap[channel] = c
		c.Add(int(user))
		data := make([]byte, 6)
		nid := n.sidecar.SnowFlakeID.GetWorkID()
		util.CopyUint32(data[0:4], uint32(nid))
		util.CopyUint16(data[4:], channel)
		n.sidecar.HandleFunc(channel, n.callWorkFunc)
		fs := transport.NewFrameSlice(transport.FrameTypeJoinChannel, data, nil)
		return user, n.sidecar.AskAppoint(target, fs)
	}
	c.Add(int(user))
	return user, nil
}

//callWorkFunc
func (n *Node) callWorkFunc(s transport.Session) error {
	ctx := getCtx(s, n)
	ctx.Node = n
	channel := s.GetFrameSlice().GetFrameType()
	users := n.channelUserMap[channel]
	users.Range(func(v int) {
		f := n.userFunc[uint32(v)]
		if f == nil {
			n.sidecar.Logger.Error("callWorkFunc|userFunc找不到:", v)
			return
		}
		ff := f.(func(*ContextMQ))
		ff(ctx)
	})
	return nil
}

//Unsubscribe 退订 publisher-subscriber
func (n *Node) Unsubscribe(user uint32, channel uint16, target int) error {
	n.sidecar.ChangeOccupy(target, -1)
	n.userFunc[user] = nil
	c, ok := n.channelUserMap[channel]
	if ok {
		if c.Remove(int(user)) == 0 {
			delete(n.channelUserMap, channel)
			data := make([]byte, 6)
			nid := n.sidecar.SnowFlakeID.GetWorkID()
			util.CopyUint32(data[0:4], uint32(nid))
			util.CopyUint16(data[4:], channel)
			fs := transport.NewFrameSlice(transport.FrameTypeLeaveChannel, data, nil)
			return n.sidecar.AskAppoint(target, fs)
		}
	}
	return errors.New("Unsubscribe|channelUserMap:找不到。")
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
	fs := transport.NewFrameSlice(pipe, data, vj)
	return n.sidecar.AskGroup(target[0], fs)
}

//JoinBus 加入总线频道 返回订阅者ID  bus	TODO 同步阻塞及超时问题
func (n *Node) JoinBus(channel uint16, f func(*ContextMQ)) (uint32, error) {
	c, ok := n.channelUserMap[channel]
	if !ok {
		n.sidecar.ChangeOccupy(n.sidecar.MachineID, 1)
		err := n.sidecar.JoinChannel(channel)
		if err != nil {
			return 0, err
		}
		mids, err := n.sidecar.GetChannels(channel)
		if err != nil {
			return 0, err
		}
		n.channelNodeMap[channel] = util.NewIntList()
		data := make([]byte, 2)
		util.CopyUint16(data[:], channel)
		fs := transport.NewFrameSlice(transport.FrameTypeBusGetChannels, data, nil)
		for _, v := range mids {
			err = n.sidecar.AskAppoint(v, fs)
			if err != nil {
				n.sidecar.Logger.Error("JoinBus|错误。", err.Error(), ",", mids, n.sidecar.MachineID, v)
			}
		}
		n.sidecar.HandleFunc(channel, n.callWorkFunc)
		c = util.NewIntList()
		n.channelUserMap[channel] = c
	}
	user := n.nextID()
	n.userFunc[user] = f
	c.Add(int(user))
	return user, nil
}

//LeaveBus 离开总线频道	 bus	TODO 同步阻塞及超时问题
func (n *Node) LeaveBus(channel uint16, user uint32) error {
	c, ok := n.channelUserMap[channel]
	if ok {
		if c.Remove(int(user)) == 0 {
			n.sidecar.ChangeOccupy(n.sidecar.MachineID, -1)
			err := n.sidecar.LeaveChannel(channel)
			if err != nil {
				return err
			}
			data := make([]byte, 2)
			util.CopyUint16(data[:2], channel)
			fs := transport.NewFrameSlice(transport.FrameTypeBusGetChannels, data, nil)
			nodes, _ := n.channelNodeMap[channel]
			nodes.Range(func(v int) {
				n.sidecar.AskAppoint(v, fs)
			})
			delete(n.channelUserMap, channel)
			delete(n.channelNodeMap, channel)
		}
		n.userFunc[user] = nil
	}
	return nil
}

//ContextMQ 上下文
type ContextMQ struct {
	*Node
	Request []byte
	session transport.Session
}

//getCtx 取得上下文	TODO 解决slice cap问题
func getCtx(s transport.Session, n *Node) *ContextMQ {
	c := &ContextMQ{
		Request: s.GetFrameSlice().GetData(),
		session: s,
	}
	c.Node = n
	return c
}

//Reply 回复 request-reply
func (c *ContextMQ) Reply(data []byte) {
	fs := transport.NewFrameSlice(transport.FrameTypeReply, data, c.session.GetFrameSlice().GetExtend())
	c.session.WriteFrameDataPromptly(fs)
}

//Next 下一个 pipeline	TODO优化 自编码
func (c *ContextMQ) Next(data []byte) error {
	pipeline := &PipelineTarget{}
	ex := c.session.GetFrameSlice().GetExtend()
	if err := json.Unmarshal(ex, pipeline); err != nil {
		return errors.New("Next|json解码错误：" + err.Error() + "    " + string(ex))
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
	fs := transport.NewFrameSlice(c.session.GetFrameSlice().GetFrameType(), data, vj)
	return c.sidecar.AskGroup(pipeline.Target[0], fs)
}
