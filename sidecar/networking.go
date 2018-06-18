package sidecar

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
)

//Contact 通讯联络
type Contact struct {
	ctx         context.Context
	ComputerMap *sync.Map                       //key:版本/服务名/机器id  value:  session
	ClientMap   map[string]*transport.ClientTCP //key:url  value:  ClientTCP

	tcpServer  *transport.ServerTCP
	httpServer *http.Server

	closeOnce sync.Once
	*transport.Handler
}

//newContact 新建
func newContact(ctx context.Context, p *Peer) (*Contact, error) {
	c := &Contact{
		ctx:         ctx,
		ComputerMap: &sync.Map{},
		ClientMap:   make(map[string]*transport.ClientTCP, 1024),
		Handler:     transport.NewHandler(),
	}
	//tcp支持
	c.tcpServer = transport.NewServerTCP(ctx, p.TCPPort, c.Handler, p.SnowFlakeID)
	if c.tcpServer == nil {
		return nil, errors.New("NewContact|NewServerTCP失败:" + p.TCPPort)
	}
	c.HandleFunc(transport.FrameTypeNodeName, c.addSessionTCP)

	//http支持
	c.httpServer = &http.Server{
		Addr:           p.HTTPPort,
		Handler:        http.DefaultServeMux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	pre := fmt.Sprintf("/%d", p.leaseID)
	http.HandleFunc(pre+"/ping", c.echo)
	http.HandleFunc(pre+"/exit", c.exit)
	return c, nil
}

//releaseContact 释放
func (c *Contact) releaseContact() {
	c.ComputerMap = nil
	c.ClientMap = nil
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
func (c *Contact) addSessionTCP(s transport.Session) {
	ft := s.GetFrameSlice()
	c.ComputerMap.Store(string(ft.GetData()), s)
	c.ComputerMap.Store(getNodeID(string(ft.GetData())), s)
}

//getNodeID
func getNodeID(s string) int64 {
	i := strings.LastIndex(s, "/")
	d, err := strconv.Atoi(s[i+1:])
	if err != nil {
		return 0
	}
	return int64(d)
}
