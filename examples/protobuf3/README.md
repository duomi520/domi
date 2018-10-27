# 运行protobuf3例子

## 编译proto文件

```bash
cd examples\protobuf3\pb
protoc --go_out=. string_service.proto
```

## 启动 etcd

```bash
etcd
```

## 运行 server

```bash
cd examples\protobuf3\server
go build main.go
main
```

## 运行 client

```bash
cd examples\protobuf3\client
go build main.go
main abcde
```