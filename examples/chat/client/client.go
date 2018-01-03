package main

import (
	"fmt"
	"github.com/duomi520/domi/examples/chat"
	"github.com/duomi520/domi/server"
	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
	"os"
)

var arg string
var extend []byte
var synchronizationChan chan struct{} //同步信号

func main() {
	if len(os.Args) != 2 {
		fmt.Println("参数无效，需运行如：client 消息")
		os.Exit(1)
	}
	arg = os.Args[1]
	synchronizationChan = make(chan struct{})
	ms := util.NewMicroService()
	talk := transport.NewHandler()
	//注册一事件FrameTypeJoin，处理函数join
	talk.HandleFunc(chat.FrameTypeJoin, join)
	//注册一事件FrameTypeMessage，处理函数recvMessage
	talk.HandleFunc(chat.FrameTypeMessage, recvMessage)
	c, err := transport.NewClientTCP(ms.Ctx, "127.0.0.1:8888", talk)
	if err != nil {
		fmt.Println("连接服务端失败:", err.Error())
		os.Exit(1)
	}
	ms.RunAssembly(c)
	go func() {
		f := transport.NewFrameSlice(chat.FrameTypeJoin, nil, nil)
		c.Csession.WriteFrameDataPromptly(f)
		<-synchronizationChan
		for i := 0; i < 1; i++ {
			server.SendToZonePromptly(c.Csession, chat.FrameTypeMessage, []byte(arg), extend) //发送到节点服务器
		}
	}()
	ms.Run()
}
func join(s transport.Session) {
	//extend: 8-byte 目标服务id,8-byte 组id,2-byte frameType
	extend = make([]byte, 18)
	data := s.GetFrameSlice().GetData()
	copy(extend[:16], data[0:16])
	synchronizationChan <- struct{}{}
}
func recvMessage(s transport.Session) {
	fmt.Println(string(s.GetFrameSlice().GetData()))
}
