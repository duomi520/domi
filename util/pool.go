package util

import (
	"bytes"
	"sync"
)

//BytesPoolLenght 长度
const BytesPoolLenght int = 16384

//bufferPool bytesPool 池
var bufferPool sync.Pool
var bytesPool sync.Pool

func init() {
	bufferPool.New = func() interface{} {
		return &bytes.Buffer{}
	}
	bytesPool.New = func() interface{} {
		b := make([]byte, BytesPoolLenght)
		return b[:]
	}
}

//BufferPoolGet 取一个
func BufferPoolGet() *bytes.Buffer {
	return bufferPool.Get().(*bytes.Buffer)
}

//BufferPoolPut 还一个
func BufferPoolPut(b *bytes.Buffer) {
	b.Reset()
	bufferPool.Put(b)
}

//BytesPoolGet 取一个
func BytesPoolGet() []byte {
	b := bytesPool.Get().([]byte)
	return b[:]
}

//BytesPoolPut 还一个
func BytesPoolPut(b []byte) {
	bytesPool.Put(b)
}
