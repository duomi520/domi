package main

import (
	"github.com/duomi520/domi"
	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
	"log"
)

var pongB = [12]byte{12, 0, 0, 0, 0, 0, 225, 0, 112, 111, 110, 103}
var fsPong = transport.DecodeByBytes(pongB[:])

func main() {
	cbc := util.NewCircuitBreakerConfigure()
	a := domi.NewMaster()
	h := transport.NewHandler()
	sd := util.NewDispatcher(256)
	go sd.Run()
	s := transport.NewServerTCP(a.Ctx, ":4567", h, sd, nil, &cbc)
	h.HandleFunc(224, ping)
	if s == nil {
		log.Fatalln("启动tcp服务失败。")
	}
	s.Logger.SetLevel(util.InfoLevel)
	a.RunAssembly(s)
	a.Guard()
	sd.Close()
}
func ping(s transport.Session) error {
	if err := s.WriteFrameDataToCache(fsPong, errorFunc); err != nil {
		log.Fatalln("ping:", err.Error())
	}
	return nil
	//	if err := s.WriteFrameDataPromptly(transport.FramePong); err != nil {
	//		fmt.Println("ping:", err.Error())
	//	}
}
func errorFunc(err error) {
	log.Fatalln("errorFunc:", err.Error())
}
