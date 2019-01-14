package util

//定义状态
const (
	StateDie uint32 = 1 + iota
	StateWork
	StatePause
	StateCircuitBreakerClosed
	StateCircuitBreakerOpen
	StateCircuitBreakerHalfOpen
)
