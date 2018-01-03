package util

import (
	"testing"
)

func Test_Logger(t *testing.T) {
	l, _ := NewLogger(DebugLevel, "")
	l.SetMark("cccc")
	l.SetMark("dddd")
	//	l.Debug("eeee|test")
}
