# 运行聊天例子

## 启动 consul

```bash
consul agent -dev -config-dir=.
```

## 运行 房间服务

```bash
cd examples\chat\room
go build room.go
room
```

## 运行 网关服务

```bash
cd examples\chat\gateway
go build gateway.go
gateway
```

## 运行 go客户端测试

```bash
cd examples\chat\client
go build client.go
client 消息内容
```

## 浏览器测试

```bash
http://127.0.0.1:8080/home.html

```