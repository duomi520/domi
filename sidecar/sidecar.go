package sidecar

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

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
	logger, _ := util.NewLogger(util.ErrorLevel, "")
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
	logger.SetMark(fmt.Sprintf("Sidecar.%d", GetNodeID(s.FullName)))
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
	s.Logger.Info(fmt.Sprintf("Run|%d启动……", s.leaseID))
	heartbeat := time.NewTicker(transport.DefaultHeartbeatDuration)
	defer heartbeat.Stop()
	heartbeatMap := make(map[int64]*transport.ClientTCP, 1024) //key:机器id  value:  ClientTCP
	//启动http
	go func() {
		s.Logger.Info("Run|HTTP监听端口", s.HTTPPort)
		if err := s.httpServer.ListenAndServe(); err != nil {
			s.Logger.Debug("Run|", err.Error())
		}
	}()
	//启动tcp
	go s.dispatcher.Run()
	s.Logger.Info("Run|TCP监听端口", s.TCPPort)
	s.RunAssembly(s.tcpServer)
	//与其它服务器建立连接
	if mkey, allMachine, err := s.getAllMachineAddress(); err != nil {
		s.Logger.Error("Run|", err.Error())
	} else {
		for index, url := range allMachine {
			cli, err := transport.NewClientTCP(s.ctx, url, s.Handler, s.dispatcher)
			if err != nil {
				s.Logger.Error("Run|错误：" + err.Error())
			}
			Nodeid := GetNodeID(mkey[index])
			cli.Logger.SetMark(strconv.FormatInt(GetNodeID(s.FullName), 10))
			heartbeatMap[Nodeid] = cli
			s.tcpMap.Store(Nodeid, cli.Csession)
			s.RunAssembly(cli)
			fs := transport.NewFrameSlice(transport.FrameTypeNodeName, []byte(s.FullName), nil)
			cli.Csession.WriteFrameDataToCache(fs)
		}
	}
	for {
		select {
		case <-heartbeat.C:
			for key, v := range heartbeatMap {
				if err := v.Heartbeat(); err != nil {
					delete(heartbeatMap, key)
				}
			}
		case mw := <-s.NodeWatchChan:
			for _, ev := range mw.Events {
				switch ev.Type {
				case clientv3.EventTypeDelete:
					nodeiID := GetNodeID(string(ev.Kv.Key))
					v, ok := s.tcpMap.Load(nodeiID)
					if ok {
						ss := v.(transport.Session)
						ss.SetState(transport.StateStop)
						s.tcpMap.Delete(nodeiID)
					}
					//	s.Logger.Debug(ev.Type, string(ev.Kv.Key), ev.Kv.Value)
				}
			}
		case <-s.ctx.Done():
			s.Logger.Info("Run|等待子模块关闭……")
			//通知关闭
			s.tcpMap.Range(func(k, v interface{}) bool {
				v.(transport.Session).SyncState(transport.StateStop, transport.StateStop)
				return true
			})
			s.Wait()
			s.httpServer.Shutdown(context.TODO())
			s.dispatcher.Close()
			s.Logger.Info("Run|Sidecar关闭。")
			if err := s.releasePeer(); err != nil {
				s.Logger.Error(err)
			}
			return
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

//GetNodeID 转换
func GetNodeID(s string) int64 {
	i := strings.LastIndex(s, "/")
	d, err := strconv.Atoi(s[i+1:])
	if err != nil {
		return 0
	}
	return int64(d)
}
