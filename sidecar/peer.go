package sidecar

import (
	"context"
	"encoding/json"
	"github.com/duomi520/domi/util"
)

//定义状态
const (
	StateDie int64 = 1 + iota
	StateWork
	StatePause
)

//Distributer 分布式键值对数据存储接口
type Distributer interface {
	RegisterServer(Info, interface{}) (chan []byte, chan []byte, int64, int, error)
	GetInitAddress() []Info
	DisconDistributer() error
	PutKeyToDistributer(Info) error
	PutKey(context.Context, string, string) error
	DeleteKey(context.Context, string) error
	GetKey(context.Context, string) ([][]byte, error)
}

//getNodeIDByName 转换
func getNodeIDByName(s string) int {
	b := util.StringToBytes(s)
	i16 := util.BytesToUint16(b[8:])
	return int(i16)
}

//Info 地址信息
type Info struct {
	Name      string //服务名
	Address   string //地址
	HTTPPort  string //http端口
	TCPPort   string //tcp端口
	ID        int64
	MachineID int   //机器id
	State     int64 //状态
}

//Peer 子
type Peer struct {
	Info
	Distributer
	eventPutChan, eventDeleteChan chan []byte
}

//newPeer 新增
func newPeer(name, HTTPPort, TCPPort string, operation interface{}) (*Peer, error) {
	var err error
	p := &Peer{}
	p.Name = name
	p.HTTPPort = HTTPPort
	p.TCPPort = TCPPort
	p.State = StateWork
	p.Address, err = util.GetLocalAddress()
	if err != nil {
		return nil, err
	}
	p.Distributer = &etcd{}
	p.eventPutChan, p.eventDeleteChan, p.ID, p.MachineID, err = p.RegisterServer(p.Info, operation)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func getIDAndStateByString(b []byte) (int, string, int64) {
	i := &Info{}
	if err := json.Unmarshal(b, i); err != nil {
		return 0, "", -1
	}
	return i.MachineID, i.Name, i.State
}
