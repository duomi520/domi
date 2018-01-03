package cluster

import (
	"context"
	"errors"
	"fmt"
	"github.com/duomi520/domi/util"
	consulapi "github.com/hashicorp/consul/api"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

//SystemCenterStartupTime 时间戳启动计算时间零点
const SystemCenterStartupTime int64 = 1514764800000000000 //2018-1-1 00:00:00 UTC

//systemServerLock 服务ID分配锁
const systemServerLock string = "systemServerLock"

//DefaultCheckVersionDuration 定时监测版本
var DefaultCheckVersionDuration = time.Second * 1

//Peer 子
type Peer struct {
	Ctx         context.Context
	Client      *consulapi.Client
	Version     int    //版本
	ID          int    //SnowFlake服务ID
	iWeight     int32  //权重
	ServiceName string //名称
	Address     string //地址
	ServicePort int    //服务端口
	Tags        []string
	httpPort    string        //http端口
	stopChan    chan struct{} //退出信号
	closeOnce   sync.Once
	*util.Logger
}

//NewPeer 新增
func NewPeer(ctx context.Context, name, hp string, port int) *Peer {
	var err error
	p := &Peer{
		Ctx:         ctx,
		Version:     0,
		iWeight:     0,
		ServiceName: name,
		ServicePort: port,
		httpPort:    hp,
		stopChan:    make(chan struct{}),
	}
	p.Logger, _ = util.NewLogger(util.DebugLevel, "")
	p.Address, err = util.GetLocalAddress()
	if err != nil {
		p.Logger.Fatal(err)
	}
	p.Logger.Info("NewPeer|", name, "新增。")
	return p
}

//healthyCheck 健康检测
func (p *Peer) healthyCheck(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "consul healthy check!")
}

//echo Ping 回复 pong
func (p *Peer) echo(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "pong")
}

//getVersion 取得版本
func (p *Peer) getVersion(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, p.Version)
}

//getWeight 取得权重
func (p *Peer) getWeight(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, atomic.LoadInt32(&p.iWeight))
}

//exit 退出  TODO 安全 加认证
func (p *Peer) exit(w http.ResponseWriter, r *http.Request) {
	p.Close()
	fmt.Fprintln(w, "exiting")
}

//initHTTPHandleFunc 预设置相关http服务
func initHTTPHandleFunc(p *Peer) {
	http.HandleFunc("/"+strconv.Itoa(p.ID)+"/health", p.healthyCheck)
	http.HandleFunc("/"+strconv.Itoa(p.ID)+"/ping", p.echo)
	http.HandleFunc("/"+strconv.Itoa(p.ID)+"/version", p.getVersion)
	http.HandleFunc("/"+strconv.Itoa(p.ID)+"/weight", p.getWeight)
	http.HandleFunc("/"+strconv.Itoa(p.ID)+"/exit", p.exit)
}

//RegisterServer 注册服务到consul
func (p *Peer) RegisterServer() error {
	client, err := consulapi.NewClient(consulapi.DefaultConfig())
	if err != nil {
		p.Client = nil
		return errors.New("RegisterServer|连接到consul失败: " + err.Error())
	}
	//比较版本
loop:
	kv := client.KV()
	snversion := p.ServiceName + "Version"
	pair, _, err := kv.Get(snversion, nil)
	if err != nil {
		p.Client = nil
		return err
	}
	if pair == nil {
		kvp := &consulapi.KVPair{Key: snversion, Value: []byte(strconv.Itoa(p.Version))}
		_, err = kv.Put(kvp, nil)
		if err != nil {
			p.Client = nil
			return err
		}
	} else {
		v, _ := strconv.Atoi(string(pair.Value))
		if p.Version < v {
			p.Client = nil
			return errors.New("RegisterServer|版本太低。")
		}
		if p.Version > v {
			pair.Value = []byte(strconv.Itoa(p.Version))
			b, _, err := kv.CAS(pair, nil)
			if err != nil {
				p.Client = nil
				return err
			}
			if !b {
				goto loop
			}
		}
	}
	//注册服务id
	p.Client = client
	id, err := p.registerID()
	if err != nil {
		p.Client = nil
		return errors.New("RegisterServer|注册服务id失败: " + err.Error())
	}
	p.ID = id
	p.Logger.SetMark("Peer." + p.ServiceName + "." + strconv.Itoa(id))
	//注册http服务
	initHTTPHandleFunc(p)
	//注册服务
	registration := new(consulapi.AgentServiceRegistration)
	registration.ID = strconv.Itoa(p.ID)
	registration.Name = p.ServiceName
	registration.Port = p.ServicePort
	registration.Tags = p.Tags
	registration.Address = p.Address
	registration.Check = &consulapi.AgentServiceCheck{
		HTTP:                           fmt.Sprintf("http://%s%s/%d/health", p.Address, p.httpPort, p.ID),
		Timeout:                        "3s",
		Interval:                       "5s",
		DeregisterCriticalServiceAfter: "30s", //check失败后30秒删除本服务，实际运行中大约80秒才注销服务
	}
	err = client.Agent().ServiceRegister(registration)
	if err != nil {
		p.Client = nil
		return errors.New("RegisterServer|注册服务失败: " + err.Error())
	}
	return nil
}

//registerID 注册服务id
func (p *Peer) registerID() (int, error) {
	// Make a session
	session := p.Client.Session()
	s, _, err := session.Create(nil, nil)
	if err != nil {
		return 0, err
	}
	defer session.Destroy(s, nil)
	// Acquire the key
	kv := p.Client.KV()
	value := []byte("Lock")
	KVp := &consulapi.KVPair{Key: systemServerLock, Value: value, Session: s}
	num := 50
loop:
	if work, _, err := kv.Acquire(KVp, nil); err != nil {
		return 0, err
	} else if !work {
		num--
		time.Sleep(200 * time.Millisecond)
		if num < 0 {
			return 0, errors.New("systemServerLock锁失败")
		}
		goto loop
	}
	id, err1 := p.getID()
	// Release
	if work, _, err := kv.Release(KVp, nil); err != nil {
		return 0, err
	} else if !work {
		return 0, errors.New("systemServerLock锁释放失败")
	}
	return id, err1
}

//getId 取id
func (p *Peer) getID() (int, error) {
	catalog := p.Client.Catalog()
	all, _, err := catalog.Services(nil)
	if err != nil {
		return 0, err
	}
	var queue []int
	for key := range all {
		services, _, err := catalog.Service(key, "", nil)
		if err != nil {
			p.Logger.Error("getID|", err.Error())
			continue
		}
		for ix := range services {
			if id, err := strconv.Atoi(services[ix].ServiceID); err == nil {
				if id > -1 && id < util.MaxWorkNumber {
					queue = append(queue, id)
				}
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
		return n, errors.New("工作机器id已分配完")
	}
	p.Logger.Debug("已用ID列表:", queue, "分配：", n)
	return n, nil
}

//Run 运行
func (p *Peer) Run() {
	check := time.NewTicker(DefaultCheckVersionDuration)
	p.Logger.Info("Run|启动……")
	snversion := p.ServiceName + "Version"
	for {
		select {
		case <-check.C:
			//发现新版本通知服务停止
			kv := p.Client.KV()
			pair, _, err := kv.Get(snversion, nil)
			if err != nil || pair == nil {
				p.Logger.Info("Run|未发现版本值。", err.Error())
				p.Close()
				//调高权重，不再接入新请求
				atomic.StoreInt32(&p.iWeight, 101)
				continue
			}
			v, _ := strconv.Atoi(string(pair.Value))
			if p.Version < v {
				p.Logger.Info("Run|发现新版本。")
				p.Close()
				//调高权重，不再接入新请求
				atomic.StoreInt32(&p.iWeight, 101)
				continue
			}
			//根据内存占有率修改权重
			mem, _ := util.Memory()
			atomic.StoreInt32(&p.iWeight, (int32)(mem.UsedPercent))
			//p.Logger.Debug("Run|内存及权重", mem, p.iWeight)
		case <-p.stopChan:
			goto end
		case <-p.Ctx.Done():
			//p.Logger.Debug("Run|收到系统关闭消息。")
			p.Close()
		}
	}
end:
	if p.Ctx.Value(util.KeyCtxStopFunc) != nil {
		p.Ctx.Value(util.KeyCtxStopFunc).(func())()
	}
	p.Logger.Info("Run|关闭。")
	check.Stop()
}

//Close 关闭
func (p *Peer) Close() {
	p.closeOnce.Do(func() {
		close(p.stopChan)
	})
}

//ChoseServer 通过名称选择一服务器
func (p *Peer) ChoseServer(name, httpPort string) (string, error) {
	catalog := p.Client.Catalog()
	services, _, err := catalog.Service(name, "", nil)
	if err != nil {
		return "", err
	}
	if len(services) == 0 {
		return "", errors.New("ChoseServer|找不到合适的服务。")
	}
	var best, index int
	for i := 0; i < len(services); i++ {
		address := fmt.Sprintf("http://%s%s/%s/weight", services[i].ServiceAddress, httpPort, services[i].ServiceID)
		res, err := http.Get(address)
		if err != nil {
			continue
		}
		robots, err := ioutil.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			continue
		}
		var w int
		if w, err = strconv.Atoi(string(robots[0 : len(robots)-1])); err != nil {
			continue
		}
		if best > w || best == 0 {
			best = w
			index = i
		}
	}
	if best > 90 {
		return "", errors.New("ChoseServer|集群负荷过大。")
	}
	return fmt.Sprintf("%s:%d", services[index].ServiceAddress, services[index].ServicePort), nil
}
