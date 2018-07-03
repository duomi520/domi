package sidecar

import (
	"testing"
	"time"
)

//需先启动 etcd

var testEndpoints = []string{"localhost:2379"}

func Test_newPeer(t *testing.T) {
	p0, err := newPeer("0/server/", ":7080", ":9520", testEndpoints)
	if err != nil {
		t.Fatal(err)
	}
	p1, err := newPeer("1/server/", ":7080", ":9521", testEndpoints)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := newPeer("2/server/", ":7080", ":9522", testEndpoints)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(15 * time.Second)
	_, address, _ := p0.getAllMachineAddress()
	if len(address) != 3 {
		t.Fatal(address)
	}
	time.Sleep(1 * time.Second)
	p0.releasePeer()
	p1.releasePeer()
	p2.releasePeer()
}

//设置api版本  ./set ETCDCTL_API=3
//读取所有key  ./etcdctl get --from-key ''
/*
machine/0
{"Name":"0/server/","FullName":"0/server/0","URL":"192.168.3.102:9520","HTTPPort":":7080","TCPPort":":9520"}
machine/1
{"Name":"1/server/","FullName":"1/server/1","URL":"192.168.3.102:9521","HTTPPort":":7080","TCPPort":":9521"}
machine/2
{"Name":"2/server/","FullName":"2/server/2","URL":"192.168.3.102:9522","HTTPPort":":7080","TCPPort":":9522"}
*/
