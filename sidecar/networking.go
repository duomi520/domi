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
	ctx context.Context

	dispatcher *util.Dispatcher
	tcpServer  *transport.ServerTCP
	httpServer *http.Server

	closeOnce sync.Once
	*transport.Handler
	*cluster
}

//newContact 新建
func newContact(ctx context.Context, name, HTTPPort, TCPPort string, operation interface{}) (*Contact, error) {
	c := &Contact{
		ctx:     ctx,
		Handler: transport.NewHandler(),
	}
	var err error
	//监视
	c.cluster, err = newCluster(name, HTTPPort, TCPPort, operation)
	if err != nil {
		return nil, err
	}
	c.dispatcher = util.NewDispatcher("TCP", 256)
	//tcp支持
	c.tcpServer = transport.NewServerTCP(ctx, TCPPort, c.Handler, c.dispatcher)
	if c.tcpServer == nil {
		return nil, errors.New("newContact|NewServerTCP失败:" + TCPPort)
	}
	c.tcpServer.Logger.SetMark(fmt.Sprintf("%d", c.MachineID))
	c.HandleFunc(transport.FrameTypeNodeName, c.addSessionTCP)
	//http支持
	c.httpServer = &http.Server{
		Addr:           HTTPPort,
		Handler:        http.DefaultServeMux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	pre := fmt.Sprintf("/%d", c.ID)
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
	fmt.Fprintln(w, "exit")
}

//addSessionTCP 用户连接时
func (c *Contact) addSessionTCP(s transport.Session) error {
	ft := s.GetFrameSlice()
	id := int(util.BytesToInt64(ft.GetData()[:8]))
	state := util.BytesToInt64(ft.GetData()[8:16])
	group := string(ft.GetData()[16:])
	if c.MachineID != id {
		c.storeSessionChan <- memberMsg{id, group, state, s}
	}
	return nil
}
