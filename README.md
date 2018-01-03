# DOMI

[![MIT licensed][1]][2]

[1]: https://img.shields.io/badge/license-MIT-blue.svg
[2]: LICENSE

DOMI是个简单的开源的有状态服务网络库，借助consul来实现一个小规模的集群。
应用场景为分组进行的实时互动，如聊天室，简单游戏。不支持复杂的事务及服务流转。

## 第三方库依赖

* github.com/hashicorp/consul/api
* github.com/gorilla/websocket

## 架构图

![png](/framework.png)

## 集群

### 服务器Agent

每台服务器需要启动consul做为集群成员节点上守护进程。

Agent有client和server两种模式，一般在集群中设置3~5 Server，其它设置client。

* 启动第一台server

```bash
consul agent -server -bootstrap -data-dir=. -bind=198.198.110.10
```

* 启动client

```bash
consul agent -data-dir=.
```

* 将client加入到server 集群中

```bash
consul join 198.198.110.10
```

### 负载均衡

申请一个服务时，将在集群里找内存占有率最低的一个服务节点地址，发给申请者。当占有率都高于90%时，返回一个错误，拒绝新连接。

### 服务节点发现

* 服务节点启动时自动加入集群，由ServiceName来判断是否同一组服务，由Version来判断版本，当有新版本节点上线时，旧版本节点将停止新用户接入，等待所有连接关闭后自动退出。

* 服务节点健康由consul管理，当服务节点检测不到时，集群将自动剔除该服务节点。

### 规模可扩展

可以根据性能的需求增减服务节点，集群最多支持1023个服务节点。

### 服务节点关闭

* ctrl+c，或者 kill 指定的服务节点，可以强制将相关的服务节点推出集群，服务节点会等待8秒后强制退出。

* 可以通过http关闭服务节点，停止新用户接入，等待所有用户连接关闭后自动退出。

## 样例

### 注册事件

```golang
func main() {
    ms := util.NewMicroService()
    nto := server.NodeOptions{
        Version:  1,
        HTTPPort: ":8090",
        TCPPort:  ":9999",
        Name:     "room",
    }
    //新建一节点服务
    zone = server.NewZone(ms.Ctx, nto)
    //新建一组，并挂在节点服务下
    mygroup = server.NewGroup(666, zone)
    //注册一事件FrameTypeJoin，处理函数join
    mygroup.HandleFunc(chat.FrameTypeJoin, join)
    //注册一事件FrameTypeMessage，处理函数recv
    mygroup.HandleFunc(chat.FrameTypeMessage, recv)
    ms.RunAssembly(zone)
    //启动服务
    ms.Run()
}
func join(fs *transport.FrameSlice) {
    data := fs.GetData()
    //加入新成员
    mygroup.AddUser(util.BytesToInt64(data[8:16]), util.BytesToInt64(data[:8]))
}
func recv(fs *transport.FrameSlice) {
    //广播到组里所有成员
    mygroup.BroadcastToGateway(chat.FrameTypeMessage, fs)
}
```

## 封包格式

[x][x][x][x] | [x][x] | [x][x] | [x]... | [x]...
---|---|---|---|---
(uint32) | (uint16) | (uint16) | (binary) | (binary)
4-byte | 2-byte | 2-byte | N-byte | N-byte
length | extendSize | frameType | data | extend

包头固定长度为8个字节，前4字节为包的长度，5-6字节为路由的长度，7-8字节为事件，data为包的内容，extend为路由的内容。

事件前49的值为库保留，自定义事件从50后开始编号。

封包的处理非按接受到的顺序，需求对顺序敏感的，需自行再处理。

每个事件处理器有唯一ID（64位）用于路由匹配。无需路由时，extend为nil。

客户端收到的封包格式为：包头：7-8字节为事件值；extend为nil。

客户端发给网关的封包格式为：包头：7-8字节为事件值；extend为nil。

客户端发给节点的封包格式为：包头：7-8字节为事件值8；extend：1-8字节节点服务器id,8-16字节 组id,16-18字节事件值。（待优化,以节省带宽）

## 版本

* 在consul版本1.0.1上完成的测试。
* go版本需要1.9以上。

## 备注

* 节点服务的组处理函数，不要处理IO及锁，如有需要，需新建一协程处理，以防止RingBuffer阻塞。

## 联系

联系邮箱: s_w_wang@163.com
