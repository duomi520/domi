package sidecar

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
)

//Contact 通讯联络
type Contact struct {
	ctx        context.Context
	tcpServer  *transport.ServerTCP
	httpServer *http.Server
	closeOnce  sync.Once

	*transport.Handler
	*Peer
}

//newContact 新建
func newContact(ctx context.Context, name, HTTPPort, TCPPort string, endpoints []string) (*Contact, error) {
	c := &Contact{
		ctx:     ctx,
		Handler: transport.NewHandler(),
	}
	var err error
	c.Peer, err = newPeer(name, HTTPPort, TCPPort, endpoints)
	if err != nil {
		return nil, errors.New("newContact|Peer失败：" + err.Error())
	}
	//tcp支持
	c.tcpServer = transport.NewServerTCP(ctx, TCPPort, c.Handler, c.SnowFlakeID)
	if c.tcpServer == nil {
		return nil, errors.New("newContact|NewServerTCP失败:" + TCPPort)
	}
	c.HandleFunc(transport.FrameTypeNodeName, c.addSessionTCP)

	//http支持
	c.httpServer = &http.Server{
		Addr:           HTTPPort,
		Handler:        http.DefaultServeMux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	pre := fmt.Sprintf("/%d", c.leaseID)
	http.HandleFunc(pre+"/ping", c.echo)
	http.HandleFunc(pre+"/exit", c.exit)
	return c, nil
}

//echo Ping 回复 pong
func (c *Contact) echo(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "pong")
}

//exit 退出  TODO 安全 加认证
func (c *Contact) exit(w http.ResponseWriter, r *http.Request) {
	c.closeOnce.Do(func() {
		if c.ctx.Value(util.KeyCtxStopFunc) != nil {
			c.ctx.Value(util.KeyCtxStopFunc).(func())()
		}
	})
	fmt.Fprintln(w, "exiting")
}

//addSessionTCP 用户连接时
func (c *Contact) addSessionTCP(s transport.Session) error {
	ft := s.GetFrameSlice()
	c.NodeMap.Store(string(ft.GetData()), s)
	c.NodeMap.Store(getNodeID(string(ft.GetData())), s)
	return nil
}
