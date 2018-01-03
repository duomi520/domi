package main

import (
	"fmt"
	"os"
	"time"

	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
)

func main() {
	m := util.NewMicroService()
	h := transport.NewHandler()
	sfID := util.NewSnowFlakeID(1, time.Now().UnixNano())
	dispatcher := util.NewDispatcher(m.Ctx, "TCP", 256)
	go dispatcher.Run()
	s := transport.NewServerTCP(m.Ctx, ":4567", h, sfID, dispatcher)
	h.HandleFunc(transport.FrameTypePing, ping)
	if s == nil {
		fmt.Println("启动tcp服务失败。")
		os.Exit(1)
	}
	s.Logger.SetLevel(util.InfoLevel)
	m.RunAssembly(s)
	m.Run()
}
func ping(s transport.Session) {
	if err := s.WriteFrameDataToQueue(transport.FramePong); err != nil {
		fmt.Println("ping:", err.Error())
	}
	//s.WriteFrameDataPromptly(transport.FramePong)
}
