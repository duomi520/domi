package util

import (
	"errors"
	"net"
)

//ErrLocalAddressNotfound 定义错误
var ErrLocalAddressNotfound = errors.New("util.GetLocalAddr|找不到本地网络地址。")

//MemoryStat 内存信息
type MemoryStat struct {
	Total       uint64  `json:"total"`
	Available   uint64  `json:"available"`
	Used        uint64  `json:"used"`
	UsedPercent float64 `json:"usedPercent"`
	Free        uint64  `json:"free"`
}

//GetLocalAddress 读取本机地址
func GetLocalAddress() (string, error) {
	address, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, addr := range address {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", ErrLocalAddressNotfound
}
