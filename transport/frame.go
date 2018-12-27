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
	FrameTypeNil uint16 = 65500 + iota
	FrameTypeHeartbeatC
	FrameTypeHeartbeatS
	FrameTypeExit
	FrameTypePing
	FrameTypePong
	FrameType6
	FrameType7
	FrameType8
	FrameType9
	FrameTypeNodeName
	FrameSerialRejectFunc
)

//定义
var (
	FrameNil        FrameSlice
	FrameHeartbeatC FrameSlice
	FrameHeartbeatS FrameSlice
	FrameExit       FrameSlice
	FramePing       FrameSlice
	FramePong       FrameSlice
)

//初始化
func init() {
	buf := [56]byte{8, 0, 0, 0, 0, 0, 220, 255, 8, 0, 0, 0, 0, 0, 221, 255, 8, 0, 0, 0, 0, 0, 222, 255, 8, 0, 0, 0, 0, 0, 223, 255, 12, 0, 0, 0, 0, 0, 224, 255, 112, 105, 110, 103, 12, 0, 0, 0, 0, 0, 225, 255, 112, 111, 110, 103}
	FrameNil = DecodeByBytes(buf[:8])
	FrameHeartbeatC = DecodeByBytes(buf[8:16])
	FrameHeartbeatS = DecodeByBytes(buf[16:24])
	FrameExit = DecodeByBytes(buf[24:32])
	FramePing = DecodeByBytes(buf[32:44])
	FramePong = DecodeByBytes(buf[44:])
}

//FrameSlice 帧切片
type FrameSlice struct {
	splitLength int    //data和extend的分割位置
	base        []byte //原始切片
}

//NewFrameSlice 新建帧切片 拷贝到新的切片
func NewFrameSlice(ft uint16, d, ex []byte) FrameSlice {
	l := FrameHeadLength + len(d) + len(ex)
	dl := FrameHeadLength + len(d)
	tmp := make([]byte, l)
	util.CopyUint32(tmp[0:4], uint32(l))
	util.CopyUint16(tmp[4:6], uint16(len(ex)))
	util.CopyUint16(tmp[6:8], ft)
	copy(tmp[FrameHeadLength:dl], d)
	copy(tmp[dl:], ex)
	f := FrameSlice{
		splitLength: dl,
		base:        tmp,
	}
	return f
}

//DecodeByBytes 解码 引用原地址
func DecodeByBytes(b []byte) FrameSlice {
	f := FrameSlice{}
	f.splitLength = len(b) - int(util.BytesToUint16(b[4:6]))
	f.base = b
	return f
}

//GetFrameLength 读取长度
func (f FrameSlice) GetFrameLength() int { return len(f.base) }

//GetData 读取数据
func (f FrameSlice) GetData() []byte { return f.base[FrameHeadLength:f.splitLength] }

//GetExtend 读取扩展
func (f FrameSlice) GetExtend() []byte { return f.base[f.splitLength:] }

//GetAll 读取所有
func (f FrameSlice) GetAll() []byte { return f.base }

//GetFrameType 读取
func (f FrameSlice) GetFrameType() uint16 { return util.BytesToUint16(f.base[6:8]) }

//WriteToBytes 写入目标切片
func (f FrameSlice) WriteToBytes(b []byte) int {
	if len(f.base) > len(b) {
		return 0
	}
	copy(b[:len(f.base)], f.base)
	return len(f.base)
}
