package sidecar

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
)

//Sidecar 边车
type Sidecar struct {
	ctx         context.Context
	Logger      *util.Logger
	SnowFlakeID *util.SnowFlakeID

	*Contact
	util.Child
}

//NewSidecar 新建
func NewSidecar(ctx context.Context, name, HTTPPort, TCPPort string, operation interface{}) *Sidecar {
	logger, _ := util.NewLogger(util.DebugLevel, "")
	s := &Sidecar{
		ctx:    ctx,
		Logger: logger,
	}
	var err error
	s.Contact, err = newContact(ctx, name, HTTPPort, TCPPort, operation)
	if err != nil {
		logger.Fatal("NewSidecar|Contact失败：", err.Error())
		return nil
	}
	s.SnowFlakeID = util.NewSnowFlakeID(int64(s.MachineID), SystemCenterStartupTime)
	logger.SetMark(fmt.Sprintf("Sidecar.%d", s.MachineID))
	return s
}

//Run 运行
func (s *Sidecar) Run() {
	s.Logger.Info(fmt.Sprintf("Run|%d启动……", s.ID))
	//启动心跳
	heartbeatSlice := make([]*transport.ClientTCP, len(s.GetInitAddress()))
	heartbeat := time.NewTicker(transport.DefaultHeartbeatDuration)
	defer heartbeat.Stop()
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
	s.dialNode(heartbeatSlice)
	s.RunAssembly(s.cluster)
	for {
		select {
		case <-heartbeat.C:
			i := 0
			l := len(heartbeatSlice)
			for i < l {
				if err := heartbeatSlice[i].Heartbeat(); err != nil {
					copy(heartbeatSlice[i:l-1], heartbeatSlice[i+1:])
					heartbeatSlice = heartbeatSlice[:l-1]
					l--
					i--
				}
				i++
			}
		case <-s.ctx.Done():
			s.Logger.Info("Run|等待子模块关闭……")
			s.State = StateDie
			if err := s.PutKeyToDistributer(s.Info); err != nil {
				s.Logger.Error("Run|:", err.Error())
			}
			s.Wait()
			s.httpServer.Shutdown(context.TODO())
			s.dispatcher.Close()
			s.Logger.Info("Run|Sidecar关闭。")
			if err := s.DisconDistributer(); err != nil {
				s.Logger.Error(err)
			}
			return
		}

	}
}

//getURLTCP 地址转换
func (s *Sidecar) getURLTCP(u Info) string {
	if strings.EqualFold(u.Address, s.Address) {
		return "127.0.0.1" + u.TCPPort
	}
	return u.Address + u.TCPPort
}

//dialNode 与其它服务器建立连接
func (s *Sidecar) dialNode(heartbeatSlice []*transport.ClientTCP) {
	for i, node := range s.GetInitAddress() {
		cli, err := transport.NewClientTCP(s.ctx, s.getURLTCP(node), s.Handler, s.dispatcher)
		if err != nil {
			s.Logger.Error("Run|错误：" + err.Error())
		}
		cli.Logger.SetMark(fmt.Sprintf("%d", s.MachineID))
		s.RunAssembly(cli)
		data := make([]byte, 16+len(s.Name))
		util.CopyInt64(data[:8], int64(s.MachineID))
		util.CopyInt64(data[8:16], s.State)
		copy(data[16:], util.StringToBytes(s.Name))
		fs := transport.NewFrameSlice(transport.FrameTypeNodeName, data, nil)
		err = cli.Csession.WriteFrameDataToCache(fs)
		if err != nil {
			s.Logger.Error("Run|错误：" + err.Error())
		} else {
			s.storeSessionChan <- memberMsg{node.MachineID, node.Name, node.State, cli.Csession} //node.State的状态可能不准 TODO
			heartbeatSlice[i] = cli
		}
	}
}

//JoinChannel 加入总线频道  TODO:加分布锁
func (s *Sidecar) JoinChannel(channel uint16) error {
	mid := s.SnowFlakeID.GetWorkID()
	mKey := fmt.Sprintf("channel/%d/%d", channel, mid)
	mValue := fmt.Sprintf("%d", mid)
	err := s.PutKey(context.TODO(), mKey, mValue)
	return err
}

//LeaveChannel 离开总线频道	TODO:加分布锁
func (s *Sidecar) LeaveChannel(channel uint16) error {
	mid := s.SnowFlakeID.GetWorkID()
	mKey := fmt.Sprintf("channel/%d/%d", channel, mid)
	err := s.DeleteKey(context.TODO(), mKey)
	return err
}

//GetChannels 取
func (s *Sidecar) GetChannels(channel uint16) ([]int, error) {
	mKey := fmt.Sprintf("channel/%d/", channel)
	resp, err := s.GetKey(context.TODO(), mKey)
	if err != nil {
		return nil, err
	}
	mids := make([]int, len(resp))
	for n, v := range resp {
		channel, err := strconv.Atoi(string(v))
		if err != nil {
			continue
		}
		mids[n] = channel
	}
	return mids, nil
}
