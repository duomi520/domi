# DOMI

[![MIT licensed][1]][2]

[1]: https://img.shields.io/badge/license-MIT-blue.svg
[2]: LICENSE

DOMI是个简单的开源的网络库，借助etcd来实现一个小规模的集群。

## 第三方库依赖

* github.com/coreos/etcd/clientv3

## 架构图

![png](/framework.png)

## 集群

### 服务注册与服务发现

使用etcd来实现服务注册与服务发现。

### 负载均衡

采用轮询方式进行负载。

### 故障处理及恢复

TODO

### 熔断

支持将服务暂停及启动。

### 规模可扩展

支持根据性能的需求增减服务节点，集群最多支持1024个服务节点。

### 服务节点关闭

* ctrl+c，或者 kill 指定的服务节点，可以强制将相关的服务节点推出集群，服务节点会等待8秒后强制退出。

* 支持通过http关闭服务节点。

## 快速开始

### 调用

```golang
package main
import (
    "fmt"
    "github.com/duomi520/domi"
)
//频道
const (
    ChannelMsg uint16 = 50 + iota
    ChannelRpl
)

func main() {
    //新建管理协程
    app := domi.NewMaster()
    //app.Stop服务退出函数，"client v1.0.0" 服务名，":7081" http端口号，":9501" tcp端口号，最后一个为 etcd 服务地址
    r := domi.NewNode(app.Ctx, app.Stop, "client V1.0.0", ":7081", ":9501", []string{"localhost:2379"})
    //运行r节点
    app.RunAssembly(r)
    //订阅频道ChannelRpl，关联到处理函数pong
    r.Subscribe(ChannelRpl, pong)
    //请求频道ChannelMsg服务，同时告知回复频道为ChannelRpl
    if err := r.Call(ChannelMsg, []byte("ping"), ChannelRpl); err != nil {
        fmt.Println(err)
    }
    //管理协程阻塞
    app.Guard()
}
//频道ChannelRpl的处理函数
func pong(ctx *domi.ContextMQ) {
    fmt.Println(string(ctx.Request))
}

```

### 无状态的服务

```golang
package main
import (
    "fmt"
    "github.com/duomi520/domi"
)
//频道
const (
    ChannelMsg uint16 = 50 + iota
    ChannelRpl
)
//无状态的服务
func main() {
    //新建管理协程
    app := domi.NewMaster()
    //app.Stop服务退出函数，"server v1.0.0" 服务名，":7080" http端口号，":9500" tcp端口号，最后一个为 etcd 服务地址
    r := domi.NewNode(app.Ctx, app.Stop, "server v1.0.0", ":7080", ":9500", []string{"localhost:2379"})
    //运行r节点
    app.RunAssembly(r)
    //订阅频道ChannelMsg，关联到处理函数ping
    r.Subscribe(ChannelMsg, ping)
    //管理协程阻塞
    app.Guard()
}
//频道ChannelMsg的处理函数
func ping(ctx *domi.ContextMQ) {
    fmt.Println(string(ctx.Request))
    //回复“pong”
    ctx.Reply([]byte("pong"))
}

```

## API样例

### 往频道发送请求

Notify 不回复请求，申请一服务处理。

```golang
func do() {
    ...
    //往频道ChannelMsg发送[]byte("Hellow")
    err := r.Notify(ChannelMsg, []byte("Hellow"))
    if err != nil {
        fmt.Println(err.Error())
    }
    ...
}
```

Call 请求，申请一服务处理，使用Call，服务需调用Reply。

```golang
func do() {
    ...
    //注册ChannelRpl的处理函数pong
    r.Subscribe(ChannelRpl, pong)
    //往频道ChannelMsg发送[]byte("ping"),在函数pong中处理服务回复的信息。
    if err := r.Call(ChannelMsg, []byte("ping"), ChannelRpl); err != nil {
        fmt.Println(err)
    }
    ...
}
func pong(ctx *domi.ContextMQ) {
    fmt.Println(string(ctx.Request))
}
```

```golang
func do() {
    ...
    cc := make(chan []byte)
    r.WatchChannel(ChannelRpl, cc)
    //往频道ChannelMsg发送[]byte("ping")。
    if err := r.Call(ChannelMsg, []byte("ping"), ChannelRpl); err != nil {
        fmt.Println(err)
    }
    //阻塞，等收到回复的信息后继续处理。
    data:=<-cc
    ...
}
```

Publish 发布，通知所有订阅频道的节点。

```golang
func do() {
    ...
    //往频道ChannelMsg广播[]byte("Hellow")
    if err := r.Publish(ChannelMsg, []byte("Hellow")); err != nil {
        fmt.Println(err.Error())
    }
    ...
}
```

Ventilator 开始，pipeline模式，数据在不同服务之间传递，后续服务需调用Next，以将服务传递给下一个服务，最后一个服务不可调用Next。

```golang
func do() {
    ...
    //后续服务通过注册频道Channel1、 Channel2、 Channel3来完成处理。
    if err :=r.Ventilator([]uint16{Channel1, Channel2, Channel3}, []byte("Pipeline")); err != nil {
        fmt.Println(err.Error())
    }
    ...
}
```

### 订阅频道

Subscribe 订阅频道，共用tcp读协程，不可有长时间的阻塞或IO。

```golang
func do() {
    ...
    //注册ChannelRpl的处理函数，回复[]byte("pong")
    r.Subscribe(ChannelMsg, func(c *domi.ContextMQ) {
        c.Reply([]byte("pong"))
        })
    ...
}
```

WatchChannel 监听频道，将读取到数据存入chan。

```golang
func do() {
    ...
    cc := make(chan []byte,128)
    r.WatchChannel(ChannelMsg, cc)
    ...
}
```

Unsubscribe 退订频道。

```golang
func do() {
    ...
    //退订频道ChannelMsg
    r.Unsubscribe(ChannelMsg)
    ...
}
```

### 后处理

Reply 回复，与Call配套，回复请求。

```golang
func ping(c *domi.ContextMQ) {
    ...
    //回复“pong”
    if err :=c.Reply([]byte("pong")); err != nil {
         fmt.Println(err.Error())
    }
    ...
}
```

Next 下一个，pipeline模式，发布使用Ventilator，后续服务用Next，最后一个服务不得使用Next。

```golang
func do(c *domi.ContextMQ) {
    ...
    if err :=c.Next(data); err != nil {
         fmt.Println(err.Error())
    }
    ...
}
```

## 串行模式

单一协程处理，可以以单线程的方式写代码，以避免使用锁，同时减少协程切换，提高cpu利用率。适应于频繁IO及同步操作，按时间轮来分配cpu，不适用于cpu密集计算或长IO场景。

启动

```golang
func do() {
    ...
    app := domi.NewMaster()
    n := domi.NewNode(app.Ctx, app.Stop, "client V1.0.0", ":7081", ":9501", []string{"localhost:2379"})
    app.RunAssembly(n)
    //继承自Node
    s := domi.NewSerial(n)
    app.RunAssembly(s)
    app.Guard()
    ...
}
```

关闭

```golang
func do() {
    ...
    s.Close()
    ...
}
```

### 专属API

SubscribeRace 订阅频道,某一频道收到信息后，执行处理函数，需在串行模式运行前执行（线程不安全）。

```golang
func do() {
    ...
    s.SubscribeRace([]uint16{1201, 1202, 1203}, func(c *domi.ContextMQ) {doSomething})
    ...
    app.RunAssembly(s)
    ...
}
```

SubscribeAll 订阅频道,全部频道都收到信息后，执行处理函数，需在串行模式运行前执行（线程不安全）。

```golang
func do() {
    ...
    s.SubscribeAll([]uint16{1201, 1202, 1203}, func(c *domi.ContextMQ) {doSomething})
    ...
    app.RunAssembly(s)
    ...
}
```

UnsubscribeGroup 退订频道。

```golang
func do() {
    ...
    s.UnsubscribeGroup([]uint16{1201, 1202, 1203})
    ...
}
```

## 版本

* 在etcd版本3.3.4上完成的测试。
* go版本需要1.9以上。

## 备注

* 仅支持64位。
* 网关需另行开发，不能直接连接客户端。
* 频道65500后的值为库保留，所有节点的频道值要一致，为了减少同步负担，频道编号直接硬编码到程序，频道一般采用递增方式，建议单独存一文件。

## 联系

联系邮箱: s_w_wang@163.com
