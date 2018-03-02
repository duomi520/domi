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

func Test_gateway(t *testing.T) {
	var err error
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	//服务端
	nto := NodeOptions{
		Version:  3,
		HTTPPort: ":8090",
		TCPPort:  ":4700",
		Name:     "Node",
	}
	s := NewNode(ctx, nto)
	s.HandleFunc(38, testRecBytes38)
	go s.Run()
	time.Sleep(150 * time.Millisecond)
	//网关
	gto := NodeOptions{
		Version:  3,
		HTTPPort: ":8080",
		TCPPort:  ":4500",
		Name:     "Gate",
	}
	g := NewGateway(ctx, gto)
	go g.Run()
	time.Sleep(150 * time.Millisecond)
	//客户端
	h := transport.NewHandler()
	h.HandleFunc(39, testRecReply39)
	var c1, c2 *transport.ClientTCP
	if c1, err = transport.NewClientTCP(ctx, "127.0.0.1:4500", h); err != nil {
		t.Fatal(err)
	}
	if c2, err = transport.NewClientTCP(ctx, "127.0.0.1:4500", h); err != nil {
		t.Fatal(err)
	}
	go c1.Run()
	go c2.Run()
	time.Sleep(150 * time.Millisecond)
	//网关连接服务端
	ids1, _ := g.CallServer(c1.Csession.ID, "Node", ":8090")
	if ids1 == 0 {
		t.Fatal("ids1为0")
	}
	ex := make([]byte, 0, 16)
	ex = util.AppendInt64(ex, ids1)
	ex = util.AppendInt64(ex, ids1)
	for i := 0; i < 3; i++ {
		err := SendToZonePromptly(c1.Csession, 38, []byte("route"), ex)
		if err != nil {
			t.Error(err)
		}
	}
	time.Sleep(150 * time.Millisecond)
	//
	ids2, _ := g.CallServer(c2.Csession.ID, "Node", ":8090")
	if ids2 == 0 {
		t.Fatal("ids2为0")
	}
	if ids1 != ids2 {
		t.Error("申请不一致", ids1, ids2)
	}
	//
	time.Sleep(150 * time.Millisecond)
	g.LeaveServer(c1.Csession.ID, ids1)
	c2.Close()
	time.Sleep(150 * time.Millisecond)
	u1, _ := g.SessionMap.Load(c1.Csession.ID)
	u2, _ := g.SessionMap.Load(c2.Csession.ID)
	if u1.(*UserConnect).nodes[0] != 0 {
		t.Error("u1未清理")
	}
	if u2 != nil {
		t.Error("u2未清理")
	}
	if u, _ := g.URLMap.Load("192.168.3.102:4700"); u != nil {
		t.Error("url未清理")
	}
	ctxExitFunc()
	time.Sleep(150 * time.Millisecond)
}

func testRecBytes38(s transport.Session) {
	f := s.GetFrameSlice()
	fmt.Println("38:", f, string(f.GetData()))
	SendToUserPromptly(s, 39, []byte("7777"), f.GetExtend()[:8])
}

func testRecReply39(s transport.Session) {
	f := s.GetFrameSlice()
	fmt.Println("39:", f, string(f.GetData()))
}
