package sidecar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/coreos/etcd/clientv3"
	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
)

//Sidecar 边车
type Sidecar struct {
	ctx    context.Context
	Logger *util.Logger
	*Contact
	util.Child
}

//NewSidecar 新建
func NewSidecar(ctx context.Context, name, HTTPPort, TCPPort string, endpoints []string) *Sidecar {
	logger, _ := util.NewLogger(util.DebugLevel, "")
	s := &Sidecar{
		ctx:    ctx,
		Logger: logger,
	}
	var err error
	s.Contact, err = newContact(ctx, name, HTTPPort, TCPPort, endpoints)
	if err != nil {
		logger.Fatal("NewSidecar|Contact失败：", err.Error())
		return nil
	}
	logger.SetMark("Sidecar")
	return s
}

//Send 发送
func (s *Sidecar) Send(target interface{}, fs *transport.FrameSlice) error {
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
			s.RunAssembly(cli) //TODO 优化,仅需发心跳就可以。
			fs := transport.NewFrameSlice(transport.FrameTypeNodeName, []byte(s.FullName), nil)
			go cli.Csession.WriteFrameDataPromptly(fs)
		}
	}
	atomic.StoreInt64(&s.ServiceState, ServiceStateRun)
	for {
		select {
		case mw := <-s.NodeWatchChan:
			s.machineGuard(mw)
		case <-s.ctx.Done():
			atomic.StoreInt64(&s.ServiceState, ServiceStateStop)
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
			atomic.StoreInt64(&s.ServiceState, ServiceStateDie)
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
			s.RunAssembly(cli) //TODO 优化
			fs := transport.NewFrameSlice(transport.FrameTypeNodeName, []byte(s.FullName), nil)
			go cli.Csession.WriteFrameDataPromptly(fs)
			s.addNode(*info, cli)
			//s.Logger.Debug(ev.Type, ev.Kv.Key, ev.Kv.Value, info)
		case clientv3.EventTypeDelete:
			info, ok := s.AddressInfoMap[string(ev.Kv.Value)]
			if ok {
				s.ClientMap[info.URL].Close() //TODO 完善关闭机制
				s.removeNode(info, ev.Kv.Value)
			}
			//s.Logger.Debug(ev.Type, ev.Kv.Key, ev.Kv.Value)
		}
	}
}

//JoinChannel 加入总线频道  TODO:加分布锁
func (s *Sidecar) JoinChannel(channel uint16) error {
	mid := s.SnowFlakeID.GetWorkID()
	mKey := fmt.Sprintf("channel/%d/%d", channel, mid)
	mValue := fmt.Sprintf("%d", mid)
	_, err := s.Client.Put(context.TODO(), mKey, mValue, clientv3.WithLease(s.leaseID))
	if err != nil {
		return errors.New("joinChannel|Put失败: " + err.Error())
	}
	return nil
}

//LeaveChannel 离开总线频道	TODO:加分布锁
func (s *Sidecar) LeaveChannel(channel uint16) error {
	mid := s.SnowFlakeID.GetWorkID()
	mKey := fmt.Sprintf("channel/%d/%d", channel, mid)
	_, err := s.Client.Delete(context.TODO(), mKey)
	if err != nil {
		return errors.New("leaveChannel|Delete失败: " + err.Error())
	}
	return nil
}

//GetChannels 取
func (s *Sidecar) GetChannels(channel uint16) ([]int, error) {
	mKey := fmt.Sprintf("channel/%d/", channel)
	resp, err := s.Client.Get(context.TODO(), mKey, clientv3.WithPrefix())
	if err != nil {
		return nil, errors.New("joinChannel|Get失败: " + err.Error())
	}
	mids := make([]int, 0, len(resp.Kvs))
	for _, ev := range resp.Kvs {
		i, err := strconv.Atoi(string(ev.Value))
		if err != nil {
			continue
		}
		mids = append(mids, i)
	}
	return mids, nil
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
