package util

import (
	"strings"
	"testing"
)

//converTestString
var converTestString = strings.Repeat("a", 1024)

//converTest1
func converTest1() {
	b := []byte(converTestString)
	_ = string(b)
}

//converTest2
func converTest2() {
	b := StringToBytes(converTestString)
	_ = BytesToString(b)
}

//Benchmark_Test1
func Benchmark_Test1(b *testing.B) {
	for i := 0; i < b.N; i++ {
		converTest1()
	}
}

//Benchmark_Test2
func Benchmark_Test2(b *testing.B) {
	for i := 0; i < b.N; i++ {
		converTest2()
	}
}
