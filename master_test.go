package domi

import (
	"testing"
	"time"
)

type testMaster struct {
}

func (p *testMaster) Run() {}

func Test_RunAssembly(t *testing.T) {
	d := NewMaster()
	app := &testMaster{}
	d.SetChildCount(1)
	d.RunAssembly(app)
	go d.Run()
	time.Sleep(5 * time.Second)
	d.Stop()
	d.SetChildCount(0)
	time.Sleep(1 * time.Second)
}