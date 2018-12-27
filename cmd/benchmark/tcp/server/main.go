package main

import (
	"github.com/duomi520/domi"
	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
	"log"
)

func main() {
	a := domi.NewMaster()
	h := transport.NewHandler()
	sd := util.NewDispatcher(256)
	go sd.Run()
	s := transport.NewServerTCP(a.Ctx, ":4567", h, sd, nil)
	h.HandleFunc(transport.FrameTypePing, ping)
	h.ErrorFunc(77, func(status int, err error) {
		if err != nil {
			log.Fatalln("ping:", err.Error())
		}
	})
	if s == nil {
		log.Fatalln("启动tcp服务失败。")
	}
	s.Logger.SetLevel(util.InfoLevel)
	a.RunAssembly(s)
	a.Guard()
	sd.Close()
}
func ping(s transport.Session) error {
	s.WriteFrameDataToCache(transport.FramePong, 77)
	return nil
	//	if err := s.WriteFrameDataPromptly(transport.FramePong); err != nil {
	//		fmt.Println("ping:", err.Error())
	//	}
}
