package main

import (
	"fmt"
	"os"

	"github.com/duomi520/domi"
	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
)

func main() {
	a := domi.NewMaster()
	h := transport.NewHandler()
	sd := util.NewDispatcher("TCP", 256)
	go sd.Run()
	s := transport.NewServerTCP(a.Ctx, ":4567", h, sd)
	h.HandleFunc(transport.FrameTypePing, ping)
	if s == nil {
		fmt.Println("启动tcp服务失败。")
		os.Exit(1)
	}
	s.Logger.SetLevel(util.InfoLevel)
	a.RunAssembly(s)
	a.Run()
	sd.Close()
}
func ping(s transport.Session) error {
	if err := s.WriteFrameDataToCache(transport.FramePong); err != nil {
		fmt.Println("ping:", err.Error())
	}
	return nil
	//	if err := s.WriteFrameDataPromptly(transport.FramePong); err != nil {
	//		fmt.Println("ping:", err.Error())
	//	}
}