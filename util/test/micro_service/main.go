package main

import (
	"github.com/duomi520/domi/util"
	"time"
)

func main() {
	m := util.NewMicroService()
	m.AddChildCount(1)
	go m.Run()
	time.Sleep(10 * time.Second)
	m.Stop()
	time.Sleep(5 * time.Second)
}
