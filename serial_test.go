package domi

import (
	"fmt"
	"testing"
	"time"
)

func Test_NewSerial(t *testing.T) {
	Ring := NewRingBuffer(12)
	t.Log(Ring.ringBufferShift, Ring.ringBufferSize)
}
func Test_Serial1(t *testing.T) {
	ctxExitFunc, n1, n2 := test2Node()
	s := NewSerial(n2)
	s.ring = NewRingBuffer(3)
	s.SerialProcess(91, testReply)
	s.SerialProcess(92, testReply)
	s.SerialProcess(93, testReply)
	s.SerialProcess(94, testReply)
	s.SerialProcess(95, testReply)
	time.Sleep(500 * time.Millisecond)
	n1.Notify(91, []byte("91"))
	n1.Notify(92, []byte("92"))
	n1.Notify(93, []byte("93"))
	n1.Notify(94, []byte("94"))
	for i := 0; i < 20; i++ {
		n1.Notify(95, []byte(fmt.Sprintf("<---%d", i)))
	}
	time.Sleep(150 * time.Millisecond)
	ctxExitFunc()
	s.Close()
	time.Sleep(1500 * time.Millisecond)
}
