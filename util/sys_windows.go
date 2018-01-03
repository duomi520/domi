// +build windows

package util

// 参考
// "github.com/shirou/gopsutil/mem"

import (
	"errors"
	"syscall"
	"unsafe"
)

//ErrWindowsMemoryFail 定义错误
var ErrWindowsMemoryFail = errors.New("util.Memory|读取Windows内存信息失败。")

var (
	procGlobalMemoryStatusEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")
)

type memoryStatusEx struct {
	cbSize                  uint32
	dwMemoryLoad            uint32
	ullTotalPhys            uint64
	ullAvailPhys            uint64
	ullTotalPageFile        uint64
	ullAvailPageFile        uint64
	ullTotalVirtual         uint64
	ullAvailVirtual         uint64
	ullAvailExtendedVirtual uint64
}

//Memory 读取Windows内存信息
func Memory() (MemoryStat, error) {
	var memInfo memoryStatusEx
	memInfo.cbSize = uint32(unsafe.Sizeof(memInfo))
	mem, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&memInfo)))
	if mem == 0 {
		return MemoryStat{}, ErrWindowsMemoryFail
	}
	ret := MemoryStat{
		Total:       memInfo.ullTotalPhys,
		Available:   memInfo.ullAvailPhys,
		Used:        memInfo.ullTotalPhys - memInfo.ullAvailPhys,
		UsedPercent: float64(memInfo.dwMemoryLoad),
	}
	return ret, nil

}
