package main

import (
	"context"
	"fmt"
	"net"
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

func main() {
	fmt.Println("本地机器的逻辑CPU个数:", runtime.NumCPU())
	testN(100)
	testN(500)
	testN(1000)
	testN(2500)
	//
	clientN(100)
	clientN(500)
	clientN(1000)
	clientN(2500)
	clientN(4000)
}
func testN(num int) {
	sessions := make([]*transport.SessionTCP, num)
	var wg sync.WaitGroup
	for c := 0; c < num; c++ {
		sessions[c] = dial()
		if sessions[c] == nil {
			fmt.Println("+连接服务端失败:", c)
			os.Exit(1)
		}
	}
	fmt.Printf("测试 ")
	time.Sleep(2000 * time.Millisecond)
	start := time.Now()
	for j := 0; j < 500; j++ {
		k := j
		go func(jj int, ss []*transport.SessionTCP) {
			wg.Add(1)
			buf := make([]byte, 12)
			for l := 0; l < 5000; l++ {
				index := l % num
				if err := ss[index].WriteFrameDataPromptly(transport.FramePing); err != nil {
					fmt.Println("+SendData：", jj, l, index, err)
					os.Exit(1)
				}
				if n, err := ss[index].Conn.Read(buf); err != nil {
					fmt.Println("+ioRead", jj, l, index, n, err)
					os.Exit(1)
				}
			}
			wg.Done()
		}(k, sessions)
	}
	wg.Wait()
	end := time.Now()
	qps := 2500000.0 / end.Sub(start).Seconds()
	fmt.Printf("%d个连接500协程qps:%6.0f\n", num, qps)
	for c := 0; c < num; c++ {
		sessions[c].WriteFrameDataPromptly(transport.FrameExit)
		sessions[c].Conn.Close()
	}
}
func dial() *transport.SessionTCP {
	tcpAddr, err := net.ResolveTCPAddr("tcp4", "127.0.0.1:4567")
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		fmt.Println("连接服务端失败:", err.Error())
		conn.Close()
		return nil
	}
	session := transport.NewSessionTCP(conn)
	b := util.Uint32ToBytes(transport.ProtocolMagicNumber)
	if _, err = conn.Write(b); err != nil {
		fmt.Println("ProtocolMagicNumber:", err)
		conn.Close()
		return nil
	}
	return session
}

var clientNwg sync.WaitGroup

func clientN(num int) {
	loop := 25000000
	//	f, _ := os.Create("profile.mem")
	//	defer f.Close()
	var err error
	h := transport.NewHandler()
	h.HandleFunc(transport.FrameTypePong, pongFunc)
	cs := make([]*transport.ClientTCP, num)
	for i := 0; i < num; i++ {
		k := i
		cs[i], err = transport.NewClientTCP(context.TODO(), "127.0.0.1:4567", h)
		cs[i].Logger.SetLevel(util.ErrorLevel)
		if err != nil {
			fmt.Println("连接服务端失败:", err.Error())
			os.Exit(1)
		}
		go cs[k].Run()
		time.Sleep(1 * time.Millisecond) //防止连接太快，被操作系统拒绝。
	}
	time.Sleep(2000 * time.Millisecond)
	fmt.Printf("测试 ")
	clientNwg.Add(loop)
	start := time.Now()
	for i := 0; i < loop; i++ {
		index := i % num
		cs[index].Send(transport.FramePing)
		//cs[index].Csession.WriteFrameDataPromptly(transport.FramePing)
	}
	clientNwg.Wait()
	end := time.Now()
	qps := float64(loop) / end.Sub(start).Seconds()
	fmt.Printf("%d个连接及协程qps:%6.0f\n", num, qps)
	//	pprof.WriteHeapProfile(f)
	time.Sleep(time.Second)
	for i := 0; i < num; i++ {
		cs[i].Close()
	}
	cs = nil
}

func pongFunc(s transport.Session) {
	//	fmt.Println(string(s.GetFrame().GetData()))
	clientNwg.Done()
}
