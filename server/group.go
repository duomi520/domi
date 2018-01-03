package server

import (
	"errors"
	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
	"sort"
)

//Group 组
type Group struct {
	ID         int64
	parent     *Zone
	GateGroups map[int64]GateGroup
	recvWorker map[uint16]func(*transport.FrameSlice)
}

//GateGroup 网关组
type GateGroup struct {
	ID   int64   //网关ID
	list []int64 //用户ID
	buf  []byte  //用户ID转字节
}

//Len 方法返回集合中的元素个数
func (gg GateGroup) Len() int {
	return len(gg.list)
}

//Less 方法报告索引i的元素是否比索引j的元素小
func (gg GateGroup) Less(i, j int) bool {
	return gg.list[i] < gg.list[j]
}

//Swap 方法交换索引i和j的两个元素
func (gg GateGroup) Swap(i, j int) {
	gg.list[i], gg.list[j] = gg.list[j], gg.list[i]
	b := make([]byte, 8)
	copy(b, gg.buf[i*8:i*8+8])
	copy(gg.buf[i*8:i*8+8], gg.buf[j*8:j*8+8])
	copy(gg.buf[j*8:j*8+8], b)
}

//NewGroup 新建组
func NewGroup(id int64, zone *Zone) *Group {
	g := &Group{
		ID:         id,
		parent:     zone,
		GateGroups: make(map[int64]GateGroup),
		recvWorker: make(map[uint16]func(*transport.FrameSlice)),
	}
	g.recvWorker[transport.FrameTypeUserLeave] = g.leave
	zone.GroupMap.Store(id, g)
	return g
}

//Close 关闭
func (g *Group) Close(gateid, uid int64) {
	g.parent.GroupMap.Delete(gateid)
	g.GateGroups = nil
	g.recvWorker = nil
}

//HandleFunc 添加处理器 线程不安全，需在初始化时设置完毕，服务启动后不能再添加。
func (g *Group) HandleFunc(u16 uint16, f func(*transport.FrameSlice)) {
	g.recvWorker[u16] = f
}

func (g *Group) leave(fs *transport.FrameSlice) {
	ex := fs.GetExtend()
	data := fs.GetData()
	g.RemoveUser(util.BytesToInt64(data), util.BytesToInt64(ex[:8]))
}

//AddUser 用户加入组
func (g *Group) AddUser(gateid, uid int64) {
	if v, ok := g.GateGroups[gateid]; ok {
		index := sort.Search(len(v.list), func(i int) bool { return v.list[i] >= uid })
		if index == len(v.list) {
			v.list = append(v.list, uid)
			v.buf = util.AppendInt64(v.buf, uid)
			sort.Sort(v)
			g.GateGroups[gateid] = v
		}
	} else {
		ng := GateGroup{
			ID:   gateid,
			list: make([]int64, 1),
			buf:  make([]byte, 0, 1),
		}
		ng.list[0] = uid
		ng.buf = util.AppendInt64(ng.buf, uid)
		g.GateGroups[gateid] = ng
	}
}

//RemoveUser 用户离开组
func (g *Group) RemoveUser(gateid, uid int64) {
	if v, ok := g.GateGroups[gateid]; ok {
		index := sort.Search(len(v.list), func(i int) bool { return v.list[i] >= uid })
		//	fmt.Println("g", index, v.list)
		if v.list[index] == uid {
			if index == 0 && len(v.list) == 1 {
				delete(g.GateGroups, gateid)
			} else {
				if index < len(v.list) {
					v.list = append(v.list[:index], v.list[index+1:]...)
					v.buf = append(v.buf[:index*8], v.buf[index*8+8:]...)
					g.GateGroups[gateid] = v
				}
			}
		}
	}
}

//BroadcastToGateway 广播
//输出：Gate服务的 N个 8-byte ... N个用户端id ，扩展 2-byte frameType
func (g *Group) BroadcastToGateway(ft uint16, f *transport.FrameSlice) error {
	if f == nil {
		return errors.New("frame为空。")
	}
	data := f.GetData()
	nf := transport.NewFrameSlice(transport.FrameTypeGateToUserFunc, data, nil)
	for key, v := range g.GateGroups {
		ex := make([]byte, len(v.buf)+10)
		copy(ex[:len(v.buf)], v.buf)
		copy(ex[len(v.buf):len(v.buf)+8], util.Int64ToBytes(g.ID))
		copy(ex[len(v.buf)+8:], util.Uint16ToBytes(ft))
		nf = nf.SetExtend(ex)
		if se, ok := g.parent.SessionMap.Load(key); ok {
			se.(transport.Session).WriteFrameDataPromptly(nf) //TODO 改
		}
	}
	return nil
}
