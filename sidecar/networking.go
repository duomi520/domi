package sidecar

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
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
	tcpMap     *sync.Map //key: 机器id  value:  session

	httpServer *http.Server

	closeOnce sync.Once

	*transport.Handler
	*Peer
}

//newContact 新建
func newContact(ctx context.Context, name, HTTPPort, TCPPort string, endpoints []string) (*Contact, error) {
	c := &Contact{
		ctx:     ctx,
		tcpMap:  &sync.Map{},
		Handler: transport.NewHandler(),
	}
	var err error
	c.Peer, err = newPeer(name, HTTPPort, TCPPort, endpoints)
	if err != nil {
		return nil, errors.New("newContact|Peer失败：" + err.Error())
	}
	c.dispatcher = util.NewDispatcher("TCP", 256)
	//tcp支持
	c.tcpServer = transport.NewServerTCP(ctx, TCPPort, c.Handler, c.dispatcher)
	if c.tcpServer == nil {
		return nil, errors.New("newContact|NewServerTCP失败:" + TCPPort)
	}
	c.tcpServer.Logger.SetMark(strconv.FormatInt(GetNodeID(c.FullName), 10))
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
	fmt.Fprintln(w, "exit")
}

//
//
//

//addSessionTCP 用户连接时
func (c *Contact) addSessionTCP(s transport.Session) error {
	ft := s.GetFrameSlice()
	c.tcpMap.Store(GetNodeID(string(ft.GetData())), s)
	return nil
}

//getSession 取得会话 target必须int64 TODO
func (c *Contact) getSession(target interface{}) (transport.Session, error) {
	v, ok := c.tcpMap.Load(target)
	if !ok {
		return nil, fmt.Errorf("getSession|tcpMap:%d", target.(int64))
	}
	s := v.(transport.Session)
	if s.GetState() == transport.StateWork {
		return s, nil
	}
	return nil, errors.New("no work")
}
