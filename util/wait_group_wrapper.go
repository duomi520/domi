package util

import (
	"sync"
)

//WaitGroupWrapper 封装
type WaitGroupWrapper struct {
	sync.WaitGroup
}

//Wrap 封装
func (w *WaitGroupWrapper) Wrap(cb func()) {
	w.Add(1)
	go func() {
		cb()
		w.Done()
	}()
}
