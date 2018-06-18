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
