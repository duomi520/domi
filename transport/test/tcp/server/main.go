package main

import (
	"fmt"
	"os"
	"time"

	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
)

func main() {
	a := util.NewApplication()
	h := transport.NewHandler()
	sfID := util.NewSnowFlakeID(1, time.Now().UnixNano())
	s := transport.NewServerTCP(a.Ctx, ":4567", h, sfID)
	h.HandleFunc(transport.FrameTypePing, ping)
	if s == nil {
		fmt.Println("启动tcp服务失败。")
		os.Exit(1)
	}
	s.Logger.SetLevel(util.InfoLevel)
	a.RunAssembly(s)
	a.Run()
}
func ping(s transport.Session) {
	if err := s.WriteFrameDataToCache(transport.FramePong); err != nil {
		fmt.Println("ping:", err.Error())
	}
	//	if err := s.WriteFrameDataPromptly(transport.FramePong); err != nil {
	//		fmt.Println("ping:", err.Error())
	//	}
}
