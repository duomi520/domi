package server

import (
	"context"
	"fmt"
	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
	"testing"
	"time"
)

//需先启动 consul agent -dev -data-dir=.

func Test_zone(t *testing.T) {
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	nto := NodeOptions{
		Version:  3,
		HTTPPort: ":8080",
		TCPPort:  ":4500",
		Name:     "Node1",
	}
	z := NewZone(ctx, nto)
	z.HandleFunc(48, testRecBytes48)
	go z.Run()
	//
	h := transport.NewHandler()
	c, err := transport.NewClientTCP(ctx, "127.0.0.1:4500", h)
	if err != nil {
		t.Fatal(err)
	}
	go c.Run()
	time.Sleep(150 * time.Millisecond)
	ff := transport.NewFrameSlice(48, []byte("node"), nil)
	for i := 0; i < 3; i++ {
		c.Send(ff)
	}
	//
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(150 * time.Millisecond)
}

func testRecBytes48(s transport.Session) {
	fs := s.GetFrameSlice()
	fmt.Println(fs, string(fs.GetData()))
}

var testZone *Zone
var testGroup *Group

func Test_Group(t *testing.T) {
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	nto := NodeOptions{
		Version:  3,
		HTTPPort: ":8080",
		TCPPort:  ":4500",
		Name:     "Node2",
	}
	testZone = NewZone(ctx, nto)
	go testZone.Run()
	//
	testGroup = NewGroup(333, testZone)
	testGroup.HandleFunc(57, testJoin57)
	testGroup.HandleFunc(58, testRecBytes58)
	testGroup.HandleFunc(59, testLeave59)
	var users [3][]byte
	users[0] = util.AppendInt64(util.Int64ToBytes(900), 333)
	users[1] = util.AppendInt64(util.Int64ToBytes(901), 333)
	users[2] = util.AppendInt64(util.Int64ToBytes(902), 333)
	//	users[2] = util.Int64ToBytes(902)
	//
	h := transport.NewHandler()
	h.HandleFunc(55, testRecReply55)
	c, err := transport.NewClientTCP(ctx, "127.0.0.1:4500", h)
	if err != nil {
		t.Fatal(err)
	}
	go c.Run()
	time.Sleep(100 * time.Millisecond)
	//加入
	c.Send(transport.NewFrameSlice(57, util.Int64ToBytes(c.Csession.ID), users[0]))
	c.Send(transport.NewFrameSlice(57, util.Int64ToBytes(c.Csession.ID), users[1]))
	c.Send(transport.NewFrameSlice(57, util.Int64ToBytes(c.Csession.ID), users[2]))
	//广播
	c.Send(transport.NewFrameSlice(58, []byte("58c0"), users[0]))
	c.Send(transport.NewFrameSlice(58, []byte("58c1"), users[1]))
	c.Send(transport.NewFrameSlice(58, []byte("58c2"), users[2]))
	//离开
	time.Sleep(100 * time.Millisecond)
	c.Send(transport.NewFrameSlice(59, util.Int64ToBytes(c.Csession.ID), users[2]))
	c.Send(transport.NewFrameSlice(59, util.Int64ToBytes(c.Csession.ID), users[1]))
	c.Send(transport.NewFrameSlice(59, util.Int64ToBytes(c.Csession.ID), users[0]))
	//
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	time.Sleep(150 * time.Millisecond)
}

func testRecReply55(s transport.Session) {
	fs := s.GetFrameSlice()
	fmt.Println("55:", string(fs.GetData()), fs)
}

func testJoin57(fs *transport.FrameSlice) {
	gateid := util.BytesToInt64(fs.GetData())
	uid := util.BytesToInt64(fs.GetExtend())
	testGroup.AddUser(gateid, uid)
	fmt.Println("57", testGroup, gateid, uid)
}

func testRecBytes58(fs *transport.FrameSlice) {
	nf := transport.NewFrameSlice(55, fs.GetData(), nil)
	for key := range testGroup.GateGroups {
		if se, ok := testZone.SessionMap.Load(key); ok {
			se.(transport.Session).WriteFrameDataPromptly(nf)
		}
	}
	fmt.Println("58", fs, string(fs.GetData()), testGroup.GateGroups)
}

func testLeave59(fs *transport.FrameSlice) {
	gateid := util.BytesToInt64(fs.GetData())
	uid := util.BytesToInt64(fs.GetExtend())
	testGroup.RemoveUser(gateid, uid)
	fmt.Println("59", testGroup, gateid, uid)
}
