package util

import (
	"testing"
	"time"
)

type testApplication struct {
}

func (p *testApplication) Run() {}

func Test_RunAssembly(t *testing.T) {
	a := NewApplication()
	app := &testApplication{}
	a.childCount = 1
	a.RunAssembly(app)
	go a.Run()
	time.Sleep(10 * time.Second)
	a.Stop()
	a.childCount = 0
	time.Sleep(1 * time.Second)
}
