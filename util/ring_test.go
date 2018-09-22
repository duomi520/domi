package util

import (
	"bytes"
	"testing"
)

var testText1 = []byte("a---z")
var testText2 = []byte{10, 0, 0, 0, 97, 45, 45, 45, 45, 122}

func Test_ring_overflow(t *testing.T) {
	r := &RingBuffer{}
	r.InitRingBuffer(128)
	defer r.ReleaseRingBuffer()
	for i := 0; i < 30; i++ {
		if err := r.WriteToRingBuffer(testText1); err != nil {
			if i != 25 {
				t.Error("溢出测试失败，不等于25.", i)
			}
			return
		}
	}
}
func Test_ring(t *testing.T) {
	r := &RingBuffer{}
	r.InitRingBuffer(128)
	for i := 0; i < 10; i++ {
		if err := r.WriteToRingBuffer(testText2); err != nil {
			t.Error(err.Error())
		}
	}
	t.Log(r)
	text, l := r.ReadFromRingBuffer()
	r.SetAvailableCursor(l)
	if l != 10 && !bytes.Equal(text, testText2) {
		t.Error(text, l)
	}
	t.Log(r)
	r.ReleaseRingBuffer()
}

func Benchmark_WriteToRingBuffer(b *testing.B) {
	r := &RingBuffer{}
	r.InitRingBuffer(67108864)
	for i := 0; i < b.N; i++ {
		r.WriteToRingBuffer(testText2)
		r.SetAvailableCursor(10)
	}
	r.ReleaseRingBuffer()
}
