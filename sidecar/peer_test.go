package sidecar

import (
	"testing"
	"time"
)

//需先启动 etcd
//设置api版本  ./set ETCDCTL_API=3
//读取所有key  ./etcdctl get --from-key ''
/*
machine/
{"Name":"0/server","Address":"192.168.1.5","HTTPPort":":7080","TCPPort":":9520","ID":7587831686899188231,"MachineID":0}
machine/╔
{"Name":"1/server","Address":"192.168.1.5","HTTPPort":":7080","TCPPort":":9521","ID":7587831686899188240,"MachineID":1}
machine/╗
{"Name":"2/server","Address":"192.168.1.5","HTTPPort":":7080","TCPPort":":9522","ID":7587831686899188249,"MachineID":2}
*/

var testEndpoints = []string{"localhost:2379"}

func Test_newPeer(t *testing.T) {
	p0, err := newPeer("0/server", ":7080", ":9520", testEndpoints)
	if err != nil {
		t.Fatal(err)
	}
	p1, err := newPeer("1/server", ":7080", ":9521", testEndpoints)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := newPeer("2/server", ":7080", ":9522", testEndpoints)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Second)
	if len(p0.GetInitAddress()) != 1 || len(p1.GetInitAddress()) != 2 || len(p2.GetInitAddress()) != 3 {
		t.Fatal(p0.GetInitAddress(), p1.GetInitAddress(), p2.GetInitAddress())
	}
	time.Sleep(1 * time.Second)
	p0.DisconDistributer()
	p1.DisconDistributer()
	p2.DisconDistributer()
}
