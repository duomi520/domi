package server

import (
	"context"
	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
	"sync"
	"time"
)

//Zone 区域
type Zone struct {
	GroupMap *sync.Map
	*Node
	*util.EventHandler
}

//NewZone 新建
func NewZone(ctx context.Context, o NodeOptions) *Zone {
	logger, _ := util.NewLogger(util.DebugLevel, "")
	logger.SetMark("Zone")
	z := &Zone{GroupMap: &sync.Map{}}
	z.Node = NewNode(ctx, o)
	z.Next = z.writeTOEventHandler
	if len(z.options.TCPPort) > 0 {
		z.dispatcher = util.NewDispatcher(z.sequence.Ctx[2], "Zone", 256)
	}
	z.Logger = logger
	z.EventHandler = util.NewEventHandler(z.sequence.Ctx[1], z.task, decode)
	return z
}

//Run 运行
func (z *Zone) Run() {
	if z.dispatcher != nil {
		z.sequence.WG[2].Wrap(z.dispatcher.Run)
	}
	z.sequence.WG[1].Wrap(z.EventHandler.Run)
	z.Node.Run()
	z.GroupMap = nil
}

func (z *Zone) task(pj util.ProcessorJob) {
	if g, ok := z.GroupMap.Load(pj.ID); ok {
		for _, v := range pj.Data {
			fs := transport.DecodeByBytes(v)
			if do, ok := g.(*Group).recvWorker[fs.GetFrameType()]; ok {
				if do != nil {
					do(fs)
				}
			}
		}
	}
}
func decode(b []byte) (int64, []byte) {
	length := len(b)
	newLength := length - 8
	buf := b[:newLength]
	id := util.BytesToInt64(b[newLength:])
	copy(buf[:4], util.Int64ToBytes(int64(newLength)))
	u16 := util.BytesToUint16(buf[4:6])
	copy(buf[4:6], util.Uint16ToBytes(u16-8))
	return id, buf
}
func (z *Zone) writeTOEventHandler(se transport.Session) error {
	time.Sleep(time.Microsecond) //--!没明白  TODO:优化一次拷完。
	buf := se.GetFrameSlice().EncodedTOBytes()
	err := z.EventHandler.Ring.WriteToRingBuffer(buf)
	return err
}
