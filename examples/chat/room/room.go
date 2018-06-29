package main

import (
	"github.com/duomi520/domi"
	"github.com/duomi520/domi/util"
)

func main() {
	a := util.NewApplication()
	room := domi.NewNode(a.Ctx, "1/room/", ":7082", ":9522", []string{"localhost:2379"})
	a.RunAssembly(room)
	a.Run()
}
