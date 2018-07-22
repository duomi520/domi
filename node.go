package domi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/duomi520/domi/sidecar"
	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
)

//TypeTell 不回复
const TypeTell uint16 = 65535

//Node 节点
type Node struct {
	sidecar    *sidecar.Sidecar
	serverFunc [65536]interface{} //key:uint16  	value:func(*ContextMQ)
	Logger     *util.Logger
}

//NewNode 新建
func NewNode(ctx context.Context, name, HTTPPort, TCPPort string, endpoints []string) *Node {
	n := &Node{}
	n.sidecar = sidecar.NewSidecar(ctx, name, HTTPPort, TCPPort, endpoints)
	n.Logger = n.sidecar.Logger
	n.Logger.SetLevel(util.ErrorLevel)
	return n
}

//Close g
func (n *Node) Close() {
}

//Run 运行
func (n *Node) Run() {
	n.sidecar.Run()
}

//Ready 准备好
func (n *Node) Ready() {
	n.sidecar.Ready()
	time.Sleep(10 * time.Millisecond)
}

//SerialProcess 单频道处理
func (n *Node) SerialProcess(channel uint16, f func(*ContextMQ)) {
	var idAndChannel [3]uint16
	idAndChannel[0] = uint16(n.sidecar.MachineID)
	idAndChannel[1] = channel
	idAndChannel[2] = 0
	n.sidecar.ChangeChannelChan <- idAndChannel
	n.serverFunc[channel] = f
	n.sidecar.HandleFunc(channel, n.handleWrapper)
}

func (n *Node) handleWrapper(s transport.Session) error {
	ft := s.GetFrameSlice().GetFrameType()
	v := n.serverFunc[ft]
	if v == nil {
		n.Logger.Warn("handleWrapper|serverFunc找不到:", ft)
		return nil
	}
	f := v.(func(*ContextMQ))
	ctx := getCtx(s, n)
	f(ctx)
	return nil
}

//Call 请求 request-reply	每个请求都带有回应id
func (n *Node) Call(channel uint16, data []byte, reply uint16) error {
	var ex []byte
	if reply != TypeTell {
		ex = make([]byte, 2)
		util.CopyUint16(ex, reply)
	}
	fs := transport.NewFrameSlice(channel, data, ex)
	return n.sidecar.AskOne(channel, fs)
}

//PipelineChannel 数据 TODO 优化json，自己编码
type PipelineChannel struct {
	Channel []uint16
}

//Ventilator 开始 pipeline
func (n *Node) Ventilator(channel []uint16, data []byte) error {
	if len(channel) < 2 {
		return errors.New("Ventilator|channel小于2")
	}
	var p PipelineChannel
	p.Channel = channel[1:]
	vj, err := json.Marshal(p)
	if err != nil {
		return errors.New("Ventilator|json编码失败: " + err.Error())
	}
	fs := transport.NewFrameSlice(channel[0], data, vj)
	return n.sidecar.AskOne(channel[0], fs)
}

//Publish 发布 publisher-subscriber
func (n *Node) Publish(channel uint16, data []byte) error {
	fs := transport.NewFrameSlice(channel, data, nil)
	return n.sidecar.AskAll(channel, fs)
}

//Unsubscribe 退订 publisher-subscriber
func (n *Node) Unsubscribe(channel uint16) {
	var idAndChannel [3]uint16
	idAndChannel[0] = uint16(n.sidecar.MachineID)
	idAndChannel[1] = channel
	idAndChannel[2] = 1
	n.sidecar.ChangeChannelChan <- idAndChannel
	n.serverFunc[channel] = nil
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
func (c *ContextMQ) Reply(data []byte) error {
	ex := c.session.GetFrameSlice().GetExtend()
	if ex == nil {
		return fmt.Errorf("Reply|该请求无回复处理函数。")
	}
	fs := transport.NewFrameSlice(util.BytesToUint16(ex), data, nil)
	return c.session.WriteFrameDataPromptly(fs)
}

//Next 下一个 pipeline	TODO优化 自编码
func (c *ContextMQ) Next(data []byte) error {
	pipeline := &PipelineChannel{}
	ex := c.session.GetFrameSlice().GetExtend()
	if err := json.Unmarshal(ex, pipeline); err != nil {
		return errors.New("Next|json解码错误：" + err.Error() + "    " + string(ex))
	}
	if len(pipeline.Channel) < 1 {
		return errors.New("Next|target小于1")
	}
	var p PipelineChannel
	p.Channel = pipeline.Channel[1:]
	vj, err := json.Marshal(p)
	if err != nil {
		return errors.New("Next|json编码失败: " + err.Error())
	}
	fs := transport.NewFrameSlice(pipeline.Channel[0], data, vj)
	return c.sidecar.AskOne(pipeline.Channel[0], fs)
}
