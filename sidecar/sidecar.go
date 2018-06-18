package sidecar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/coreos/etcd/clientv3"
	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
)

//Sidecar 边车
type Sidecar struct {
	ctx context.Context
	*Peer
	*Contact
	util.Child
	Logger *util.Logger
}

//NewSidecar 新建
func NewSidecar(ctx context.Context, name, HTTPPort, TCPPort string, endpoints []string) *Sidecar {
	logger, _ := util.NewLogger(util.DebugLevel, "")
	s := &Sidecar{
		ctx:    ctx,
		Logger: logger,
	}
	var err error
	s.Peer, err = newPeer(name, HTTPPort, TCPPort, endpoints)
	if err != nil {
		logger.Fatal("NewMonitor|Peer失败：", err.Error())
		return nil
	}
	s.Contact, err = newContact(ctx, s.Peer)
	if err != nil {
		logger.Fatal("NewMonitor|Contact失败：", err.Error())
		return nil
	}
	logger.SetMark(fmt.Sprintf("Sidecar.%d", s.leaseID))
	return s
}

//getSession d
func (s *Sidecar) getSession(target interface{}) (transport.Session, error) {
	v, ok := s.ComputerMap.Load(target)
	if !ok {
		return nil, errors.New("getSession|ComputerMap找不到。")
	}
	return v.(transport.Session), nil
}

//Send 发送
func (s *Sidecar) Send(ft uint16, target interface{}, data, ex []byte) error {
	fs := transport.NewFrameSlice(ft, data, ex)
	ss, err := s.getSession(target)
	if err != nil {
		return err
	}
	if err := ss.WriteFrameDataPromptly(fs); err != nil {
		return err
	}
	return nil
}

//Run 运行
func (s *Sidecar) Run() {
	s.Logger.Info("Run|启动……")
	//启动http
	go func() {
		s.Logger.Info("Run|HTTP监听端口", s.HTTPPort)
		if err := s.httpServer.ListenAndServe(); err != nil {
			s.Logger.Debug("Run|", err.Error())
		}
	}()
	//启动tcp
	s.Logger.Info("Run|TCP监听端口", s.TCPPort)
	s.RunAssembly(s.tcpServer)
	//与其它服务器建立连接
	if allMachine, err := s.getAllMachineAddress(); err != nil {
		s.Logger.Error("Run|", err.Error())
	} else {
		for _, url := range allMachine {
			cli, err := transport.NewClientTCP(context.TODO(), url, s.Handler)
			if err != nil {
				s.Logger.Error("Run|错误：" + err.Error())
			}
			s.ClientMap[url] = cli
			go cli.Run() //TODO 优化,仅需发心跳就可以。
			fs := transport.NewFrameSlice(transport.FrameTypeNodeName, []byte(s.FullName), nil)
			go cli.Csession.WriteFrameDataPromptly(fs)
		}
	}
	//监视
	machineWatch := s.client.Watch(context.TODO(), "machine/", clientv3.WithPrefix())
	for {
		select {
		case mw := <-machineWatch:
			s.machineGuard(mw)
		case <-s.ctx.Done():
			s.Logger.Info("Run|等待子模块关闭……")
			for _, cli := range s.ClientMap {
				cli.Close() //TODO 完善关闭机制
			}
			s.Wait()
			s.httpServer.Shutdown(context.TODO())
			s.Logger.Info("Run|关闭。")
			if err := s.releasePeer(); err != nil {
				s.Logger.Error(err)
			}
			s.releaseContact()
			return
		}
	}
}

//machineGuard 维护
func (s *Sidecar) machineGuard(wc clientv3.WatchResponse) {
	for _, ev := range wc.Events {
		switch ev.Type {
		case clientv3.EventTypePut:
			info := &AddressInfo{}
			if err := json.Unmarshal([]byte(ev.Kv.Value), info); err != nil {
				s.Logger.Error("guard|json解码错误：" + err.Error())
			}
			cli, err := transport.NewClientTCP(context.TODO(), info.URL, s.Handler)
			if err != nil {
				s.Logger.Error("guard|NewClientTCP错误：" + err.Error())
			}
			go cli.Run() //TODO 优化
			s.ClientMap[info.URL] = cli
			fs := transport.NewFrameSlice(transport.FrameTypeNodeName, []byte(s.FullName), nil)
			go cli.Csession.WriteFrameDataPromptly(fs)
			s.Logger.Debug(ev.Type, ev.Kv.Key, ev.Kv.Value, info)
		case clientv3.EventTypeDelete:
			info := &AddressInfo{}
			if err := json.Unmarshal([]byte(ev.Kv.Value), info); err != nil {
				s.Logger.Error("guard|json解码错误：" + err.Error())
			}
			delete(s.ClientMap, info.URL)
			s.ComputerMap.Delete(info.FullName)
			s.ComputerMap.Delete(getNodeID(info.FullName))
			s.ClientMap[info.URL].Close() //TODO 完善关闭机制
			s.Logger.Debug(ev.Type, ev.Kv.Key, ev.Kv.Value, info)
		}
	}
}
