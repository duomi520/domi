package util

import (
	"fmt"
	"testing"
	"time"
)

func Test_Dispatcher(t *testing.T) {
	d := NewDispatcher("d1", 10)
	go d.Run()
	for i := 0; i < 10; i++ {
		var td testDoJob1
		td.i = i
		d.JobQueue <- td
	}
	time.Sleep(150 * time.Millisecond)
	d.Close()
	time.Sleep(150 * time.Millisecond)
}

type testDoJob1 struct {
	i int
}

func (td testDoJob1) WorkFunc() {
	fmt.Println(td.i)
}
