package sidecar

import (
	"sync"
	"testing"
	"time"
)

func Test_group1(t *testing.T) {
	c := &cluster{
		occupySum:        0,
		groupMap:         &sync.Map{},
		storeSessionChan: make(chan memberMsg, 1024),
		occupyChan:       make(chan [2]int, 1024),
		stopChan:         make(chan struct{}),
	}
	c.Peer = &Peer{}
	c.Peer.eventPutChan = make(chan []byte)
	c.Peer.eventDeleteChan = make(chan []byte)
	go c.Run()
	c.storeSessionChan <- memberMsg{13, "s1", 0, nil}
	c.storeSessionChan <- memberMsg{14, "s1", 0, nil}
	c.storeSessionChan <- memberMsg{15, "s1", 0, nil}
	c.storeSessionChan <- memberMsg{17, "s1", 0, nil}
	c.storeSessionChan <- memberMsg{19, "s1", 0, nil}
	c.storeSessionChan <- memberMsg{21, "s2", 0, nil}
	c.storeSessionChan <- memberMsg{25, "s3", 0, nil}
	c.storeSessionChan <- memberMsg{27, "s3", 0, nil}
	c.storeSessionChan <- memberMsg{29, "s3", 0, nil}
	time.Sleep(100 * time.Millisecond)
	for i := 10; i < 30; i++ {
		c.leaverGroup("s1", i)
		c.leaverGroup("s2", i)
		c.leaverGroup("s3", i)
		t1, _ := c.groupMap.Load("s1")
		t2, _ := c.groupMap.Load("s2")
		t3, _ := c.groupMap.Load("s3")
		t.Log(i, t1, t2, t3)
	}
	t.Log(c.sessions[10:30])

}

func Test_group2(t *testing.T) {
	c := &cluster{
		occupySum:        0,
		groupMap:         &sync.Map{},
		storeSessionChan: make(chan memberMsg, 1024),
		occupyChan:       make(chan [2]int, 1024),
		stopChan:         make(chan struct{}),
	}
	c.Peer = &Peer{}
	c.Peer.eventPutChan = make(chan []byte)
	c.Peer.eventDeleteChan = make(chan []byte)
	go c.Run()
	c.storeSessionChan <- memberMsg{13, "s1", 0, nil}
	c.storeSessionChan <- memberMsg{14, "s1", 0, nil}
	c.storeSessionChan <- memberMsg{15, "s1", 0, nil}
	c.storeSessionChan <- memberMsg{17, "s1", 0, nil}
	c.storeSessionChan <- memberMsg{19, "s1", 0, nil}
	c.storeSessionChan <- memberMsg{21, "s1", 0, nil}
	c.storeSessionChan <- memberMsg{25, "s1", 0, nil}
	c.storeSessionChan <- memberMsg{27, "s1", 0, nil}
	c.storeSessionChan <- memberMsg{29, "s1", 0, nil}
	time.Sleep(100 * time.Millisecond)
	t1, _ := c.groupMap.Load("s1")
	t.Log("---", t1)
	c.leaverGroup("s1", 15)
	t1, _ = c.groupMap.Load("s1")
	t.Log("-15", t1)
	c.leaverGroup("s1", 27)
	t1, _ = c.groupMap.Load("s1")
	t.Log("-27", t1)
	c.leaverGroup("s1", 12)
	t1, _ = c.groupMap.Load("s1")
	t.Log("-12", t1)
	c.leaverGroup("s1", 29)
	t1, _ = c.groupMap.Load("s1")
	t.Log("-29", t1)
	c.leaverGroup("s1", 13)
	t1, _ = c.groupMap.Load("s1")
	t.Log("-13", t1)
}
