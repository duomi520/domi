# 运行CHAT例子

## 启动 etcd

```bash
etcd
```

## 运行 room

```bash
cd examples\chat\simpleRoom
go build room.go
room
```

## 运行 gateway

```bash
cd examples\chat\gateway
go build gateway.go
gateway
```

## 浏览器测试（不支持IE）

可以打开多个窗口进行测试

```bash
http://127.0.0.1:8080/

```

通过http，或ctrl+c结束服务。

```bash
curl http://127.0.0.1:7081/ID号/exit
curl http://127.0.0.1:7082/ID号/exit

```