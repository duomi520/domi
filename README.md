# DOMI

[![MIT licensed][1]][2]

[1]: https://img.shields.io/badge/license-MIT-blue.svg
[2]: LICENSE

DOMI是个简单的开源的有状态服务网络库，借助etcd来实现一个小规模的集群。
应用场景为分组进行的实时互动，如聊天室，简单游戏。不支持复杂的事务及服务流转。

## 第三方库依赖

* github.com/coreos/etcd/clientv3

## 架构图

![png](/framework.png)

## 集群

### 服务注册与服务发现

使用etcd来实现服务注册与服务发现。

### 负载均衡

待实施。

### 故障处理及恢复

待实施。

### 规模可扩展

可以根据性能的需求增减服务节点，集群最多支持1023个服务节点。

### 服务节点关闭

* ctrl+c，或者 kill 指定的服务节点，可以强制将相关的服务节点推出集群，服务节点会等待8秒后强制退出。

* 可以通过http关闭服务节点，停止新用户接入，等待所有用户连接关闭后自动退出。

## 样例

### 注册事件

```golang

```

## 封包格式

[x][x][x][x] | [x][x] | [x][x] | [x]... | [x]...
---|---|---|---|---
(uint32) | (uint16) | (uint16) | (binary) | (binary)
4-byte | 2-byte | 2-byte | N-byte | N-byte
length | extendSize | frameType | data | extend

包头固定长度为8个字节，前4字节为包的长度，5-6字节为路由的长度，7-8字节为事件，data为包的内容，extend为路由的内容。

事件65500后的值为库保留。

封包的处理非按接受到的顺序，需求对顺序敏感的，需自行再处理。

## 版本

* 在etcd版本3.3.4上完成的测试。
* go版本需要1.9以上。

## 备注

## 联系

联系邮箱: s_w_wang@163.com
