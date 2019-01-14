package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
	//	"runtime/pprof"
	"sync"
	"time"
)

var ping = []byte("ping")
var pong = []byte("pong")
var pingB = [12]byte{12, 0, 0, 0, 0, 0, 224, 0, 112, 105, 110, 103}
var fsPing = transport.DecodeByBytes(pingB[:])

func main() {
	fmt.Println("本地机器的逻辑CPU个数:", runtime.NumCPU())
	clientN(100)
	clientN(500)
	clientN(1000)
	clientN(2500)
	clientN(5000)
	//	clientN(10000)
}

var clientNwg sync.WaitGroup

func clientN(num int) {
	cbc := util.NewCircuitBreakerConfigure()
	sd := util.NewDispatcher(256)
	go sd.Run()
	defer sd.Close()
	loop := 50000000 //500000000
	//	f, _ := os.Create("profile.mem")
	//	defer f.Close()
	var err error
	h := transport.NewHandler()
	h.HandleFunc(225, pongFunc)
	cs := make([]*transport.ClientTCP, num)
	for i := 0; i < num; i++ {
		k := i
		cs[i], err = transport.NewClientTCP(context.TODO(), "127.0.0.1:4567", h, sd, nil, &cbc)
		if err != nil {
			fmt.Println("连接服务端失败:", err.Error())
			os.Exit(1)
		}
		cs[i].Logger.SetLevel(util.ErrorLevel)
		go cs[k].Run()
	}
	time.Sleep(500 * time.Millisecond)
	fmt.Printf("测试 ")
	clientNwg.Add(loop)
	start := time.Now()
	for i := 0; i < loop; i++ {
		index := i % num
		if err := cs[index].Csession.WriteFrameDataToCache(fsPing, errorFunc); err != nil {
			log.Fatalln("err:", err.Error())
		}
		//cs[index].Csession.WriteFrameDataPromptly(transport.FramePing)
	}
	clientNwg.Wait()
	end := time.Now()
	qps := float64(loop) / end.Sub(start).Seconds()
	fmt.Printf("%d个连接及协程:%6.0f\n", num, qps)
	//	pprof.WriteHeapProfile(f)
	for i := 0; i < num; i++ {
		cs[i].Csession.Close()
	}
	cs = nil
	time.Sleep(1 * time.Second)
}

func pongFunc(s transport.Session) error {
	//	fmt.Println(string(s.GetFrame().GetData()))
	clientNwg.Done()
	return nil
}
func errorFunc(err error) {
	log.Fatalln("errorFunc:", err.Error())
}
