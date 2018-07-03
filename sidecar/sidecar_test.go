package sidecar

import (
	"context"
	"testing"
	"time"
)

func Test_newSidecar(t *testing.T) {
	ctx, ctxExitFunc := context.WithCancel(context.Background())
	sc1 := NewSidecar(ctx, "1/server/", ":7081", ":9521", testEndpoints)
	go sc1.Run()
	sc2 := NewSidecar(ctx, "2/server/", ":7082", ":9522", testEndpoints)
	go sc2.Run()
	sc3 := NewSidecar(ctx, "3/server/", ":7083", ":9523", testEndpoints)
	go sc3.Run()
	time.Sleep(1 * time.Second)
	ctxExitFunc()
	time.Sleep(1 * time.Second)
}

func Test_runSidecar2(t *testing.T) {
	ctx1, ctxExitFunc1 := context.WithCancel(context.Background())
	ctx2, ctxExitFunc2 := context.WithCancel(context.Background())
	sc1 := NewSidecar(ctx1, "1/server/", ":7081", ":9521", testEndpoints)
	go sc1.Run()
	sc2 := NewSidecar(ctx2, "2/server/", ":7082", ":9522", testEndpoints)
	go sc2.Run()
	time.Sleep(3 * time.Second)
	ctxExitFunc1()
	time.Sleep(3 * time.Second)
	ctxExitFunc2()
	time.Sleep(3 * time.Second)
}
func Test_runSidecar3(t *testing.T) {
	ctx1, ctxExitFunc1 := context.WithCancel(context.Background())
	ctx2, ctxExitFunc2 := context.WithCancel(context.Background())
	ctx3, ctxExitFunc3 := context.WithCancel(context.Background())
	sc1 := NewSidecar(ctx1, "1/server/", ":7081", ":9521", testEndpoints)
	go sc1.Run()
	sc2 := NewSidecar(ctx2, "2/server/", ":7082", ":9522", testEndpoints)
	go sc2.Run()
	sc3 := NewSidecar(ctx3, "3/server/", ":7083", ":9523", testEndpoints)
	go sc3.Run()
	time.Sleep(3 * time.Second)
	ctxExitFunc1()
	time.Sleep(3 * time.Second)
	ctxExitFunc2()
	time.Sleep(3 * time.Second)
	ctxExitFunc3()
	time.Sleep(3 * time.Second)
}
