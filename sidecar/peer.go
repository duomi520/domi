package sidecar

import (
	"context"

	"github.com/duomi520/domi/util"
)

//MaxWorkNumber 最大工作机器数
const MaxWorkNumber int = 1024 //最大1024

//Distributer 分布式键值对数据存储接口
type Distributer interface {
	RegisterServer(Info, interface{}) (int64, int, error)
	GetInitAddress() []Info
	DisconDistributer() error
	PutKey(context.Context, string, string) error
	DeleteKey(context.Context, string) error
	GetKey(context.Context, string) ([][]byte, error)
}

//Info 地址信息
type Info struct {
	Name      string //服务名
	Address   string //地址
	HTTPPort  string //http端口
	TCPPort   string //tcp端口
	ID        int64
	MachineID int //机器id
}

//Peer 子
type Peer struct {
	Info
	Distributer
	distributerChan
}

type distributerChan struct {
	StateChan   chan stateMsg
	NodeChan    chan nodeMsg
	ChannelChan chan channelMsg
}

//newPeer 新增
func newPeer(name, HTTPPort, TCPPort string, operation interface{}) (*Peer, error) {
	var err error
	p := &Peer{}
	p.Name = name
	p.HTTPPort = HTTPPort
	p.TCPPort = TCPPort
	p.Address, err = util.GetLocalAddress()
	if err != nil {
		return nil, err
	}
	etcd := &etcd{
		NodePrefix:    "machine/",
		StatePrefix:   "state/",
		ChannelPrefix: "channel/",
		stopChan:      make(chan struct{}),
	}
	p.Distributer = etcd
	p.ID, p.MachineID, err = p.RegisterServer(p.Info, operation)
	if err != nil {
		return nil, err
	}
	p.NodeChan = etcd.NodeChan
	p.StateChan = etcd.StateChan
	p.ChannelChan = etcd.ChannelChan
	return p, nil
}
