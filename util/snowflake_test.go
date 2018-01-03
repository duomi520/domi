package util

import (
	"testing"
	"time"
)

func Test_IDWorker(t *testing.T) {
	idWorkerTests := []int{2, 3, 8, 10, -1, MaxWorkNumber + 1, -10} // Test table
	idWorkerVerify := []int{2, 3, 8, 10}                            // Verify table
	w := NewIDWorker(time.Date(2017, time.January, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	for i := 0; i < MaxWorkNumber; i++ {
		f, err := w.GetID()
		if f != i || err != nil {
			t.Error(f, err, w)
		}
	}
	g0, err := w.GetID()
	if err == nil {
		t.Error(g0, err, w)
	}
	for _, l := range idWorkerTests {
		w.PutID(l)
	}
	for _, l := range idWorkerVerify {
		g, err := w.GetID()
		if l != g || err != nil {
			t.Error(g, err, w)
		}

	}
}
func Test_NextID(t *testing.T) {
	var v [30000]int64
	s1 := NewSnowFlakeID(1, time.Date(2017, time.January, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	for i := 0; i < 10000; i++ {
		v[i], _ = s1.NextID()
	}
	time.Sleep(time.Millisecond)
	s2 := NewSnowFlakeID(1, time.Date(2017, time.January, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	for i := 10000; i < 20000; i++ {
		v[i], _ = s2.NextID()
	}
	s3 := NewSnowFlakeID(3, time.Date(2017, time.January, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	for i := 20000; i < 30000; i++ {
		v[i], _ = s3.NextID()
	}
	//验证
	for i := 0; i < (30000 - 1); i++ {
		if v[i] >= v[i+1] {
			t.Error("失败：i:", i, "v[i]:", v[i], "v[i+1]:", v[i+1])
			t.FailNow()
		}
	}
}

func Test_GetWorkID(t *testing.T) {
	s1 := NewSnowFlakeID(1, time.Date(2017, time.January, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	s2 := NewSnowFlakeID(555, time.Date(2017, time.January, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	s3 := NewSnowFlakeID(1022, time.Date(2017, time.January, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	n1, _ := s1.NextID()
	n2, _ := s2.NextID()
	n3, _ := s3.NextID()
	if GetWorkID(n1) != 1 {
		t.Error("失败:", n1, GetWorkID(n1))
	}
	if GetWorkID(n2) != 555 {
		t.Error("失败:", n2, GetWorkID(n2))
	}
	if GetWorkID(n3) != 1022 {
		t.Error("失败:", n3, GetWorkID(n3))
	}

}
