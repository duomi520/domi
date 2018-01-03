// +build linux

package util

import (
	"syscall"
)

//Memory  读取Linux内存信息
func Memory() (MemoryStat, error) {
	ret := MemoryStat{}
	sysInfo := new(syscall.Sysinfo_t)
	err := syscall.Sysinfo(sysInfo)
	if err == nil {
		ret.Total = sysInfo.Totalram
		ret.Free = sysInfo.Freeram
		ret.Used = ret.Total - ret.Free
		ret.UsedPercent = float64(ret.Total-ret.Free) / float64(ret.Total) * 100.0
	}
	return ret, err
}
