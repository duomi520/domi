package util

import (
	"testing"
)

func Test_mem(t *testing.T) {
	v, _ := Memory()
	t.Logf("全部: %v, 可用:%v, 百分比:%f%%\n", v.Total, v.Available, v.UsedPercent)
	t.Log(v)
}
