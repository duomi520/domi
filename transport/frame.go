package transport

import (
	"github.com/duomi520/domi/util"
)

/*
帧（frame）
  [x][x][x][x]    [x][x]   [x][x]    [x][x][x][x]    [x][x][x]...
|   (uint32)   ||(uint16)||(uint16)|| (binary)... || (binary)...
|    4-byte    || 2-byte || 2-byte ||   N-byte    ||   N-byte
--------------------------------------------------------------...
     length     extendSize frameType    data           extend
*/

//FrameHeadLength 帧头部大小
const FrameHeadLength int = 8

//定义frameType
const (
	FrameTypeNil uint16 = iota
	FrameTypeHeartbeat
	FrameTypeExit
	FrameTypePing
	FrameTypePong
	FrameType5
	FrameType6
	FrameType7
	FrameTypeGateToNextZoneFunc
	FrameTypeGateToUserFunc
	FrameTypeUserLeave
	FrameType11
	FrameType12
	FrameType13
	FrameType14
	FrameType15
)

//定义
var (
	FrameNil       *FrameSlice
	FrameHeartbeat *FrameSlice
	FrameExit      *FrameSlice
	FramePing      *FrameSlice
	FramePong      *FrameSlice
)

//初始化
func init() {
	buf := [48]byte{8, 0, 0, 0, 0, 0, 0, 0, 8, 0, 0, 0, 0, 0, 1, 0, 8, 0, 0, 0, 0, 0, 2, 0, 12, 0, 0, 0, 0, 0, 3, 0, 112, 105, 110, 103, 12, 0, 0, 0, 0, 0, 4, 0, 112, 111, 110, 103}
	FrameNil = DecodeByBytes(buf[0:8])
	FrameHeartbeat = DecodeByBytes(buf[8:16])
	FrameExit = DecodeByBytes(buf[16:24])
	FramePing = DecodeByBytes(buf[24:36])
	FramePong = DecodeByBytes(buf[36:48])
}

//FrameSlice 帧切片
type FrameSlice struct {
	data   []byte //8-byte 头 和 N-byte 数据
	extend []byte
	base   []byte //原始切片
}

//NewFrameSlice 新建 拷贝到新的切片
func NewFrameSlice(ft uint16, d, e []byte) *FrameSlice {
	tmp := make([]byte, 8, FrameHeadLength+len(d)+len(e))
	copy(tmp[0:4], util.Uint32ToBytes(uint32(FrameHeadLength+len(d)+len(e))))
	copy(tmp[4:6], util.Uint16ToBytes(uint16(len(e))))
	copy(tmp[6:8], util.Uint16ToBytes(ft))
	tmp = append(tmp, d...)
	f := &FrameSlice{
		data:   tmp,
		extend: e,
		base:   append(tmp, e...),
	}
	return f
}

//DecodeByBytes 解码 引用原地址
func DecodeByBytes(b []byte) *FrameSlice {
	if len(b) < FrameHeadLength {
		return nil
	}
	f := &FrameSlice{}
	le := int(util.BytesToUint16(b[4:6]))
	f.data = b[:len(b)-le]
	f.extend = b[len(b)-le:]
	f.base = b
	return f
}

//EncodedTOBytes 编码
func (f *FrameSlice) EncodedTOBytes() []byte {
	return f.base
}

//GetFrameType 读取类型
func (f *FrameSlice) GetFrameType() uint16 { return util.BytesToUint16(f.data[6:8]) }

//SetFrameType 设置类型
func (f *FrameSlice) SetFrameType(ft uint16) { copy(f.data[6:8], util.Uint16ToBytes(ft)) }

//SetFrameTypeByBytes 设置类型
func (f *FrameSlice) SetFrameTypeByBytes(b []byte) {
	if len(b) == 2 {
		copy(f.data[6:8], b)
	}
}

//GetFrameLength 读取长度
func (f *FrameSlice) GetFrameLength() int { return len(f.base) }

//GetData 读取数据
func (f *FrameSlice) GetData() []byte { return f.data[FrameHeadLength:] }

//GetExtend 读取扩展
func (f *FrameSlice) GetExtend() []byte { return f.extend }

//WriteToBytes 写入目标切片
func (f *FrameSlice) WriteToBytes(b []byte) int {
	if len(f.base) > len(b) {
		return 0
	}
	copy(b[:len(f.base)], f.base)
	return len(f.base)
}

//Release 释放
func (f *FrameSlice) Release() {
	f.data = nil
	f.extend = nil
	f.base = nil
}

//SetExtend 设置扩展
func (f *FrameSlice) SetExtend(ex []byte) *FrameSlice {
	if len(ex) <= int(util.BytesToUint16(f.base[4:6])) {
		copy(f.base[0:4], util.Uint32ToBytes(uint32(len(f.data)+len(ex))))
		copy(f.base[4:6], util.Uint16ToBytes(uint16(len(ex))))
		f.base = append(f.base[:len(f.data)], ex...)
		f.extend = f.base[len(f.data):]
		return f
	}
	length := len(f.base) - len(f.extend) + len(ex)
	base := make([]byte, length)
	copy(base, f.base[:len(f.base)-len(f.extend)])
	copy(base[length-len(ex):], ex)
	copy(base[0:4], util.Uint32ToBytes(uint32(len(f.data)+len(ex))))
	copy(base[4:6], util.Uint16ToBytes(uint16(len(ex))))
	return DecodeByBytes(base)
}
