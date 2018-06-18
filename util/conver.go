package util

import (
	"unsafe"
)

//DIGITS 数字串
const DIGITS string = "0123456789"

//Uint32ToString Uint32转String
func Uint32ToString(d uint32) string {
	if d == 0 {
		return "0"
	}
	var b [10]byte
	n := 0
	for j := 9; j >= 0 && d > 0; j-- {
		b[j] = DIGITS[d%10]
		d /= 10
		n++
	}
	return string(b[10-n:])
}

//Uint64ToString Uint64转String
func Uint64ToString(d uint64) string {
	if d == 0 {
		return "0"
	}
	var b [20]byte
	n := 0
	for j := 19; j >= 0 && d > 0; j-- {
		b[j] = DIGITS[d%10]
		d /= 10
		n++
	}
	return string(b[20-n:])
}

//StringToBytes String转Bytes
func StringToBytes(s string) []byte {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&h))
}

//BytesToString Bytes转String
func BytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

//Uint32ToBytes uint32转切片 little_endian
func Uint32ToBytes(u uint32) []byte {
	b := make([]byte, 4)
	b[0] = byte(u)
	b[1] = byte(u >> 8)
	b[2] = byte(u >> 16)
	b[3] = byte(u >> 24)
	return b
}

//Uint16ToBytes uint16转切片 little_endian
func Uint16ToBytes(u uint16) []byte {
	b := make([]byte, 2)
	b[0] = byte(u)
	b[1] = byte(u >> 8)
	return b
}

//BytesToUint16 切片转uint16  little_endian
func BytesToUint16(b []byte) uint16 {
	if len(b) == 2 {
		return uint16(b[0]) | uint16(b[1])<<8
	}
	return 0
}

//BytesToUint32 切片转uint32  little_endian
func BytesToUint32(b []byte) uint32 {
	if len(b) == 4 {
		return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
	}
	return 0
}

//AppendInt64 将int64加入切片
func AppendInt64(dst []byte, n int64) []byte {
	m := len(dst)
	m8 := m + 8
	var c [8]byte
	c[0] = byte(n)
	c[1] = byte(n >> 8)
	c[2] = byte(n >> 16)
	c[3] = byte(n >> 24)
	c[4] = byte(n >> 32)
	c[5] = byte(n >> 40)
	c[6] = byte(n >> 48)
	c[7] = byte(n >> 56)
	if m8 > cap(dst) {
		newSlice := make([]byte, m8*2)
		copy(newSlice, dst)
		dst = newSlice
	}
	dst = dst[0:m8]
	copy(dst[m:m8], c[:])
	return dst
}

//BytesToInt64 切片转int64
func BytesToInt64(b []byte) int64 {
	if len(b) == 8 {
		return int64(b[0]) | int64(b[1])<<8 | int64(b[2])<<16 | int64(b[3])<<24 | int64(b[4])<<32 | int64(b[5])<<40 | int64(b[6])<<48 | int64(b[7])<<56
	}
	return 0
}

//Int64ToBytes int64转切片
func Int64ToBytes(n int64) []byte {
	var c [8]byte
	c[0] = byte(n)
	c[1] = byte(n >> 8)
	c[2] = byte(n >> 16)
	c[3] = byte(n >> 24)
	c[4] = byte(n >> 32)
	c[5] = byte(n >> 40)
	c[6] = byte(n >> 48)
	c[7] = byte(n >> 56)
	return c[:]
}
