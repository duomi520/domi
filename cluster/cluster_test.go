package cluster

import (
	"context"
	"github.com/duomi520/domi/util"
	"net/http"
	"strings"
	"testing"
	"time"
)

//需先启动 consul agent -dev -data-dir=.
func Test_RegisterServer(t *testing.T) {
	var err error
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	p := NewPeer(ctx, "serverNode", ":7080", 9520)
	p.Version = 3
	if err = p.RegisterServer(); err != nil {
		t.Fatal(err)
	}
	s := &http.Server{
		Addr:           ":7080",
		Handler:        http.DefaultServeMux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	go func() {
		if err := s.ListenAndServe(); err != nil {
			t.Log(err)
		}
	}()
	go func() {
		time.Sleep(1 * time.Second)
		ctxExitFunc()
		s.Shutdown(context.TODO())
		time.Sleep(150 * time.Millisecond)
	}()
	p.Run()
}

func Test_GetServer(t *testing.T) {
	var err error
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	p2 := NewPeer(ctx, "serverNode1", ":7090", 9522)
	p2.iWeight = 82
	if err = p2.RegisterServer(); err != nil {
		t.Fatal(err)
	}
	p3 := NewPeer(ctx, "serverNode1", ":7090", 9523)
	p3.iWeight = 63
	if err = p3.RegisterServer(); err != nil {
		t.Fatal(err)
	}
	p4 := NewPeer(ctx, "serverNode1", ":7090", 9524)
	p4.iWeight = 84
	if err = p4.RegisterServer(); err != nil {
		t.Fatal(err)
	}
	s := &http.Server{
		Addr:           ":7090",
		Handler:        http.DefaultServeMux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	go func() {
		if err := s.ListenAndServe(); err != nil {
			t.Log(err)
		}
	}()
	go func() {
		time.Sleep(200 * time.Millisecond)
		k, err := p2.ChoseServer("serverNode1", ":7090")
		if err != nil {
			t.Error(err)
		}
		local, _ := util.GetLocalAddress()
		if !strings.EqualFold(k, local+":9523") {
			t.Error("选择：", k, local+":9523")
		}
		time.Sleep(1 * time.Second)
		ctxExitFunc()
		s.Shutdown(context.TODO())
	}()
	go p2.Run()
	go p3.Run()
	go p4.Run()
	time.Sleep(3 * time.Second)
}
