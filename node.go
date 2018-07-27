package domi

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unsafe"

	"github.com/duomi520/domi/sidecar"
	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
)

//Node 节点
type Node struct {
	sidecar *sidecar.Sidecar
	Logger  *util.Logger
}

//NewNode 新建，cancel为服务的关闭函数。
func NewNode(ctx context.Context, cancel func(), name, HTTPPort, TCPPort string, endpoints []string) *Node {
	n := &Node{}
	n.sidecar = sidecar.NewSidecar(ctx, cancel, name, HTTPPort, TCPPort, endpoints)
	n.Logger = n.sidecar.Logger
	n.Logger.SetLevel(util.ErrorLevel)
	return n
}

//Run 运行
func (n *Node) Run() {
	n.sidecar.Run()
}

//WaitInit 阻塞，等待Run初始化完成
func (n *Node) WaitInit() {
	n.sidecar.WaitInit()
	time.Sleep(10 * time.Millisecond)
}

//Pause 使服务暂停
func (n *Node) Pause() {
	n.sidecar.SetState(sidecar.StatePause)
}

//Work 使服务工作
func (n *Node) Work() {
	n.sidecar.SetState(sidecar.StateWork)
}

//IsWorking 是否工作状态
func (n *Node) IsWorking() bool {
	return n.sidecar.GetState() == sidecar.StateWork
}

type channelWrapper struct {
	cc chan []byte
}
type processWrapper struct {
	n *Node
	f func(*ContextMQ)
}

//WatchChannel 监听频道 将读取到数据存入chan
func (n Node) WatchChannel(channel uint16, cc chan []byte) {
	cs := channelWrapper{
		cc: cc,
	}
	n.sidecar.SetChannel(uint16(n.sidecar.MachineID), channel, 3)
	n.sidecar.HandleFunc(channel, cs.watchChannelWrapper)
}

func (wcs channelWrapper) watchChannelWrapper(s transport.Session) error {
	fd := s.GetFrameSlice().GetData()
	data := make([]byte, len(fd))
	copy(data, fd)
	wcs.cc <- data
	return nil
}

//SimpleProcess 订阅频道，Process共用tcp读协程，不可有长时间的阻塞或IO。
func (n *Node) SimpleProcess(channel uint16, f func(*ContextMQ)) {
	n.sidecar.SetChannel(uint16(n.sidecar.MachineID), channel, 3)
	pw := processWrapper{
		n: n,
		f: f,
	}
	n.sidecar.HandleFunc(channel, pw.processWrapper)
}

func (pw processWrapper) processWrapper(s transport.Session) error {
	ctx := getCtx(s, pw.n)
	pw.f(ctx)
	return nil
}

//SerialProcess 多频道订阅，用单一协程处理多个Process，避免多线程下竟态问题,以单线程的方式写代码。
func (n *Node) SerialProcess(channels []uint16, f []func(*ContextMQ)) {
	//TODO
}

//Unsubscribe 退订频道
func (n *Node) Unsubscribe(channel uint16) {
	n.sidecar.SetChannel(uint16(n.sidecar.MachineID), channel, 4)
}

//Call 请求	request-reply模式 ，源使用Call，目标需调用Reply
func (n *Node) Call(channel uint16, data []byte, reply uint16) error {
	ex := make([]byte, 2)
	util.CopyUint16(ex, reply)
	fs := transport.NewFrameSlice(channel, data, ex)
	return n.sidecar.AskOne(channel, fs)
}

//Tell 不回复请求
func (n *Node) Tell(channel uint16, data []byte) error {
	fs := transport.NewFrameSlice(channel, data, nil)
	return n.sidecar.AskOne(channel, fs)
}

//Ventilator 开始 pipeline模式，数据在不同服务之间传递，后续服务需调用Next，最后一个服务不可调用Next。
func (n *Node) Ventilator(channel []uint16, data []byte) error {
	if len(channel) < 2 {
		return errors.New("Ventilator|channel小于2")
	}
	l := len(channel)
	vj := make([]byte, 2*l)
	for i := 1; i < l; i++ {
		util.CopyUint16(vj[2*i:2*i+2], channel[i])
	}
	fs := transport.NewFrameSlice(channel[0], data, vj[2:])
	return n.sidecar.AskOne(channel[0], fs)
}

//Publish 发布 publisher-subscriber及bus模式，通知所有订阅频道的channel
func (n *Node) Publish(channel uint16, data []byte) error {
	fs := transport.NewFrameSlice(channel, data, nil)
	return n.sidecar.AskAll(channel, fs)
}

//ContextMQ 上下文
type ContextMQ struct {
	*Node
	Request []byte
	session transport.Session
}

//getCtx 取得上下文
func getCtx(s transport.Session, n *Node) *ContextMQ {
	c := &ContextMQ{
		Request: s.GetFrameSlice().GetData(),
		session: s,
	}
	//修改slice 的cap
	r := (*[3]uintptr)(unsafe.Pointer(&c.Request))
	r[2] = r[1]
	c.Node = n
	return c
}

//Reply 回复 request-reply模式 ，源使用Call，目标需调用Reply
func (c *ContextMQ) Reply(data []byte) error {
	ex := c.session.GetFrameSlice().GetExtend()
	if ex == nil {
		return fmt.Errorf("Reply|该请求无回复处理函数。")
	}
	fs := transport.NewFrameSlice(util.BytesToUint16(ex), data, nil)
	return c.session.WriteFrameDataPromptly(fs)
}

//Next 下一个 pipeline模式 发布使用Ventilator，后续服务用Next，最后一个服务不得使用Next
func (c *ContextMQ) Next(data []byte) error {
	ex := c.session.GetFrameSlice().GetExtend()
	if len(ex) < 2 {
		return errors.New("Next|target小于1")
	}
	channel := util.BytesToUint16(ex[:2])
	fs := transport.NewFrameSlice(channel, data, ex[2:])
	return c.sidecar.AskOne(channel, fs)
}
