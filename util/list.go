package util

import (
	"sort"
	"sync"
	"time"
)

//TODO 优化 红黑树 或 去除

//IntList 加锁正整数切片
type IntList struct {
	sync.RWMutex
	timestamp int64
	cursor    int
	list      []int
}

//NewIntList 新建
func NewIntList() *IntList {
	return &IntList{timestamp: 0, cursor: 0, list: make([]int, 0, 64)}
}

//Add +
func (i *IntList) Add(a int) {
	i.Lock()
	i.list = append(i.list, a)
	sort.Ints(i.list)
	i.Unlock()
}

//Remove -
func (i *IntList) Remove(a int) int {
	i.Lock()
	defer i.Unlock()
	l := len(i.list)
	if l == 0 {
		i.cursor = 0
		return l
	}
	index := sort.SearchInts(i.list, a)
	if i.list[index] != a {
		return l
	}
	copy(i.list[index:l-1], i.list[index+1:])
	i.list = i.list[:l-1]
	if index <= i.cursor && i.cursor > 0 {
		i.cursor--
	}
	return l - 1
}

//Fill 充填
func (i *IntList) Fill(a []int) {
	i.Lock()
	t := time.Now().UnixNano()
	if t > i.timestamp {
		i.timestamp = t
		i.cursor = 0
		i.list = a
	}
	i.Unlock()
}

//Range 遍历
func (i *IntList) Range(f func(v int)) {
	i.RLock()
	for _, value := range i.list {
		f(value)
	}
	i.RUnlock()
}

//Len 长度
func (i *IntList) Len() int {
	i.RLock()
	l := len(i.list)
	i.RUnlock()
	return l
}

//Next 下一个
func (i *IntList) Next() int {
	i.Lock()
	i.cursor++
	if i.cursor == len(i.list) {
		i.cursor = 0
	}
	v := i.list[i.cursor]
	i.Unlock()
	return v
}
