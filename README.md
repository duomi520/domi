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
    app := domi.NewMaster()//新建管理协程
    //app.Stop服务退出函数，"client v1.0.0" 服务名，":7081" http端口号，":9501" tcp端口号，最后一个为 etcd 服务地址
    r := domi.NewNode(app.Ctx, app.Stop, "client V1.0.0", ":7081", ":9501", []string{"localhost:2379"})
    app.RunAssembly(r)//运行r节点
    r.SimpleProcess(ChannelRpl, pong)//订阅频道ChannelRpl，关联到处理函数pong
    //请求频道ChannelMsg服务，同时告知回复频道为ChannelRpl
    if err := r.Call(ChannelMsg, []byte("ping"), ChannelRpl); err != nil {
        fmt.Println(err)
    }
    app.Guard()//管理协程阻塞
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
    app := domi.NewMaster()//新建管理协程
    //app.Stop服务退出函数，"server v1.0.0" 服务名，":7080" http端口号，":9500" tcp端口号，最后一个为 etcd 服务地址
    r := domi.NewNode(app.Ctx, app.Stop, "server v1.0.0", ":7080", ":9500", []string{"localhost:2379"})
    app.RunAssembly(r)//运行r节点
    r.SimpleProcess(ChannelMsg, ping)//订阅频道ChannelMsg，关联到处理函数ping
    app.Guard()//管理协程阻塞
}
//频道ChannelMsg的处理函数
func ping(ctx *domi.ContextMQ) {
    fmt.Println(string(ctx.Request))
    ctx.Reply([]byte("pong"))//回复“pong”
}

```

## API样例

### 注册频道

```golang

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
