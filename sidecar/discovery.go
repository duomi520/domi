package sidecar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/concurrency"
	"github.com/duomi520/domi/util"
)

//SystemCenterStartupTime 时间戳启动计算时间零点
const SystemCenterStartupTime int64 = 1527811200000000000 //2018-6-1 00:00:00 UTC

//systemServerLock 服务ID分配锁
const systemServerLock string = "systemServerLock"

//AddressInfo 地址信息
type AddressInfo struct {
	Name     string //服务， 格式：版本/服务名
	FullName string //完整服务， 格式：版本/服务名/机器id
	URL      string //地址
	HTTPPort string //http端口
	TCPPort  string //tcp端口
}

//Peer 子
type Peer struct {
	Client  *clientv3.Client
	leaseID clientv3.LeaseID

	SnowFlakeID   *util.SnowFlakeID
	NodeWatchChan clientv3.WatchChan

	AddressInfo
	Endpoints []string
}

//newPeer 新增
func newPeer(name, HTTPPort, TCPPort string, endpoints []string) (*Peer, error) {
	var err error
	p := &Peer{}
	p.Name = name
	p.HTTPPort = HTTPPort
	p.TCPPort = TCPPort
	p.URL, err = util.GetLocalAddress()
	if err != nil {
		return nil, err
	}
	p.URL = p.URL + TCPPort
	id, err := p.registerServer(endpoints)
	if err != nil {
		return nil, err
	}
	p.SnowFlakeID = util.NewSnowFlakeID(id, SystemCenterStartupTime)
	return p, nil
}

//releasePeer 释放
func (p *Peer) releasePeer() error {
	if _, err := p.Client.Revoke(context.TODO(), p.leaseID); err != nil {
		return err
	}
	p.Client.Close()
	return nil
}

//registerServer 注册服务到etcd
func (p *Peer) registerServer(endpoints []string) (int64, error) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 3 * time.Second,
	})
	if err != nil {
		return 0, errors.New("registerServer|连接到etcd失败: " + err.Error())
	}
	var bSuccess bool
	defer func() {
		if !bSuccess {
			client.Close()
		}
	}()
	//分布式锁
	s, err := concurrency.NewSession(client)
	if err != nil {
		return 0, errors.New("registerServer|分布式锁失败: " + err.Error())
	}
	defer s.Close()
	m := concurrency.NewMutex(s, systemServerLock)
	if err := m.Lock(context.TODO()); err != nil {
		return 0, errors.New("registerServer|分布式锁失败: " + err.Error())
	}
	defer m.Unlock(context.TODO())
	//取得机器id
	id, err := p.getMachineID(client)
	if err != nil {
		return 0, errors.New("registerServer|取得机器id失败: " + err.Error())
	}
	//注册机器id
	p.FullName = fmt.Sprintf("%s%d", p.Name, id)
	key := fmt.Sprintf("machine/%d", id)
	value, err := json.Marshal(p.AddressInfo)
	if err != nil {
		return 0, errors.New("registerServer|json编码失败: " + err.Error())
	}
	resp, err := client.Grant(context.TODO(), 3)
	if err != nil {
		return 0, errors.New("registerServer|grant失败: " + err.Error())
	}
	_, err = client.Put(context.TODO(), key, string(value), clientv3.WithLease(resp.ID))
	if err != nil {
		return 0, errors.New("registerServer|注册机器id失败: " + err.Error())
	}
	//健康检查
	_, err = client.KeepAlive(context.TODO(), resp.ID)
	if err != nil {
		return 0, errors.New("registerServer|保持健康检查失败: " + err.Error())
	}
	p.Client = client
	p.leaseID = resp.ID
	bSuccess = true
	return int64(id), nil
}

//getMachineID 分配机器id
func (p *Peer) getMachineID(cli *clientv3.Client) (int, error) {
	//设置3秒超时
	ctx, cancel := context.WithTimeout(context.TODO(), 3*time.Second)
	defer cancel()
	resp, err := cli.Get(ctx, "machine/", clientv3.WithPrefix())
	if err != nil {
		return -1, err
	}
	var queue []int
	for _, ev := range resp.Kvs {
		if id, err := strconv.Atoi(string(ev.Key[8:])); err == nil {
			if id > -1 && id < util.MaxWorkNumber {
				queue = append(queue, id)
			}
		}
	}
	if len(queue) == 0 {
		return 0, nil
	}
	n := -1
	sort.Ints(queue)
	for i := range queue {
		if i < queue[i] {
			n = i
			break
		}
	}
	if n == -1 && len(queue) < util.MaxWorkNumber {
		n = len(queue)
	}
	if n == -1 {
		return n, errors.New("getMachineID|工作机器id已分配完")
	}
	return n, nil
}

//getAllMachineAddress 取得所有机器地址
func (p *Peer) getAllMachineAddress() ([]string, []string, error) {
	//分布式锁
	s, err := concurrency.NewSession(p.Client)
	if err != nil {
		return nil, nil, errors.New("getAllMachineAddress|分布式锁失败: " + err.Error())
	}
	defer s.Close()
	m := concurrency.NewMutex(s, systemServerLock)
	if err := m.Lock(context.TODO()); err != nil {
		return nil, nil, errors.New("getAllMachineAddress|分布式锁失败: " + err.Error())
	}
	defer m.Unlock(context.TODO())
	//设置3秒超时
	ctx, cancel := context.WithTimeout(context.TODO(), 3*time.Second)
	defer cancel()
	resp, err := p.Client.Get(ctx, "machine/", clientv3.WithPrefix())
	if err != nil {
		return nil, nil, err
	}
	var addr, keys []string
	for _, ev := range resp.Kvs {
		info := &AddressInfo{}
		if err := json.Unmarshal([]byte(ev.Value), info); err != nil {
			return nil, nil, errors.New("getAllMachineAddress|json解码错误：" + err.Error())
		}
		addr = append(addr, info.URL)
		keys = append(keys, info.FullName)
	}
	//监视
	p.NodeWatchChan = p.Client.Watch(context.TODO(), "machine/", clientv3.WithPrefix())
	return keys, addr, nil
}
