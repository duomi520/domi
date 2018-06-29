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

//BytesToUint32 切片转uint32  little_endian
func BytesToUint32(b []byte) uint32 {
	if len(b) == 4 {
		return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
	}
	return 0
}

//CopyUint32 将uint32加入切片
func CopyUint32(dst []byte, n uint32) {
	var c [4]byte
	c[0] = byte(n)
	c[1] = byte(n >> 8)
	c[2] = byte(n >> 16)
	c[3] = byte(n >> 24)
	copy(dst[:], c[:])
}

//BytesToUint16 切片转uint16  little_endian
func BytesToUint16(b []byte) uint16 {
	if len(b) == 2 {
		return uint16(b[0]) | uint16(b[1])<<8
	}
	return 0
}

//CopyUint16 将uint16加入切片
func CopyUint16(dst []byte, n uint16) {
	var c [2]byte
	c[0] = byte(n)
	c[1] = byte(n >> 8)
	copy(dst[:], c[:])
}

//BytesToInt64 切片转int64
func BytesToInt64(b []byte) int64 {
	if len(b) == 8 {
		return int64(b[0]) | int64(b[1])<<8 | int64(b[2])<<16 | int64(b[3])<<24 | int64(b[4])<<32 | int64(b[5])<<40 | int64(b[6])<<48 | int64(b[7])<<56
	}
	return 0
}

//CopyInt64 将int64加入切片
func CopyInt64(dst []byte, n int64) {
	var c [8]byte
	c[0] = byte(n)
	c[1] = byte(n >> 8)
	c[2] = byte(n >> 16)
	c[3] = byte(n >> 24)
	c[4] = byte(n >> 32)
	c[5] = byte(n >> 40)
	c[6] = byte(n >> 48)
	c[7] = byte(n >> 56)
	copy(dst[:], c[:])
}
