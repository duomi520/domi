package sidecar

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/concurrency"
	"github.com/duomi520/domi/util"
)

//SystemCenterStartupTime 时间戳启动计算时间零点
const SystemCenterStartupTime int64 = 1527811200000000000 //2018-6-1 00:00:00 UTC

//systemServerLock 服务ID分配锁
const systemServerLock string = "/systemServerLock/"

type etcd struct {
	Client  *clientv3.Client
	leaseID clientv3.LeaseID

	NodePrefix, StatePrefix, ChannelPrefix          string
	NodeWatchChan, StateWatchChan, ChannelWatchChan clientv3.WatchChan
	distributerChan

	Endpoints   []string
	initAddress []Info
	stopChan    chan struct{}
	closeOnce   sync.Once
}

//GetInitAddress 读
func (e *etcd) GetInitAddress() []Info {
	return e.initAddress
}

//RegisterServer 注册服务到etcd
func (e *etcd) RegisterServer(info Info, endpoints interface{}) (int64, int, error) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints.([]string),
		DialTimeout: 3 * time.Second,
	})
	if err != nil {
		return -1, -1, errors.New("registerServer|连接到ETCD失败: " + err.Error())
	}
	e.initAddress = append(e.initAddress, info)
	var bSuccess bool
	defer func() {
		if !bSuccess {
			client.Close()
		}
	}()
	//分布式锁
	s, err := concurrency.NewSession(client)
	if err != nil {
		return -1, -1, errors.New("registerServer|NewSession分布式锁失败: " + err.Error())
	}
	defer s.Close()
	m := concurrency.NewMutex(s, systemServerLock)
	if err := m.Lock(context.TODO()); err != nil {
		return -1, -1, errors.New("registerServer|ETCD分布式锁失败: " + err.Error())
	}
	defer m.Unlock(context.TODO())
	//取得机器id
	e.initAddress[0].MachineID, err = e.getMachineID(client)
	if err != nil {
		return -1, -1, errors.New("registerServer|取得机器id失败: " + err.Error())
	}
	//注册机器id
	e.Client = client
	resp, err := e.Client.Grant(context.TODO(), 3)
	if err != nil {
		return -1, -1, errors.New("registerServer|grant失败: " + err.Error())
	}
	e.leaseID = resp.ID
	e.initAddress[0].ID = int64(e.leaseID)
	//PUT 值
	key := []byte(e.NodePrefix + "aa")
	util.CopyUint16(key[len(e.NodePrefix):], uint16(e.initAddress[0].MachineID))
	value, err := json.Marshal(e.initAddress[0])
	if err != nil {
		return -1, -1, errors.New("registerServer|json编码失败: " + err.Error())
	}
	_, err = e.Client.Put(context.TODO(), string(key), string(value), clientv3.WithLease(e.leaseID))
	if err != nil {
		return -1, -1, errors.New("registerServer|注册机器id失败: " + err.Error())
	}
	//健康检查
	_, err = client.KeepAlive(context.TODO(), e.leaseID)
	if err != nil {
		return -1, -1, errors.New("registerServer|保持健康检查失败: " + err.Error())
	}
	bSuccess = true
	e.NodeDeleteChan = make(chan []byte)
	e.StateChangeChan = make(chan []byte)
	e.ChannelPutChan = make(chan []byte)
	e.ChannelDeleteChan = make(chan []byte)
	e.NodeWatchChan = e.Client.Watch(context.TODO(), e.NodePrefix, clientv3.WithPrefix())
	e.StateWatchChan = e.Client.Watch(context.TODO(), e.StatePrefix, clientv3.WithPrefix())
	e.ChannelWatchChan = e.Client.Watch(context.TODO(), e.ChannelPrefix, clientv3.WithPrefix())
	go e.run()
	return e.initAddress[0].ID, e.initAddress[0].MachineID, nil
}

//DisconDistributer 释放
func (e *etcd) DisconDistributer() error {
	if _, err := e.Client.Revoke(context.TODO(), e.leaseID); err != nil {
		return err
	}
	e.closeOnce.Do(func() {
		e.Client.Close()
		close(e.stopChan)
	})
	return nil
}

func (e *etcd) run() {
	for {
		select {
		case nw := <-e.NodeWatchChan:
			for _, ev := range nw.Events {
				switch ev.Type {
				case clientv3.EventTypeDelete:
					e.NodeDeleteChan <- ev.Kv.Key
				}
			}
		case sw := <-e.StateWatchChan:
			for _, ev := range sw.Events {
				switch ev.Type {
				case clientv3.EventTypePut:
					e.StateChangeChan <- ev.Kv.Value
				}
			}
		case cw := <-e.ChannelWatchChan:
			for _, ev := range cw.Events {
				switch ev.Type {
				case clientv3.EventTypePut:
					e.ChannelPutChan <- ev.Kv.Value
				case clientv3.EventTypeDelete:
					e.ChannelDeleteChan <- ev.Kv.Key

				}
			}
		case <-e.stopChan:
			return
		}
	}
}

//getMachineID 分配机器id
func (e *etcd) getMachineID(cli *clientv3.Client) (int, error) {
	l := len(e.NodePrefix)
	//设置3秒超时
	ctx, cancel := context.WithTimeout(context.TODO(), 3*time.Second)
	defer cancel()
	resp, err := cli.Get(ctx, e.NodePrefix, clientv3.WithPrefix())
	if err != nil {
		return -1, err
	}
	var queue []int
	for _, ev := range resp.Kvs {
		address := &Info{}
		if err := json.Unmarshal([]byte(ev.Value), address); err != nil {
			return -1, errors.New("getAllMachineAddress|json解码错误：" + err.Error())
		}
		e.initAddress = append(e.initAddress, *address)
		id := int(util.BytesToUint16(ev.Key[l:]))
		if id > -1 && id < util.MaxWorkNumber {
			queue = append(queue, id)
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

//PutKey p
func (e *etcd) PutKey(ctx context.Context, key, value string) error {
	_, err := e.Client.Put(ctx, key, value, clientv3.WithLease(e.leaseID))
	return err
}

//DeleteKey d
func (e *etcd) DeleteKey(ctx context.Context, key string) error {
	_, err := e.Client.Delete(ctx, key, clientv3.WithPrefix())
	return err
}

//GetKey g
func (e *etcd) GetKey(ctx context.Context, key string) ([][]byte, error) {
	resp, err := e.Client.Get(ctx, key, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	v := make([][]byte, len(resp.Kvs))
	i := 0
	for _, ev := range resp.Kvs {
		v[i] = ev.Value
		i++
	}
	return v, nil
}
