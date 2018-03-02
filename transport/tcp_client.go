package transport

import (
	"context"
	"errors"
	"io"
	"net"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/duomi520/domi/util"
)

//ErrClientTCPClose 客户端已关闭
var ErrClientTCPClose = errors.New("Send|客户端已关闭。")

//ClientTCP 客户端
type ClientTCP struct {
	Ctx  context.Context
	conn *net.TCPConn
	URL  string

	SendChan chan *FrameSlice

	handler           *Handler
	OnCloseSessionTCP func(Session)

	stopChan  chan struct{} //退出信号
	closeOnce sync.Once
	Logger    *util.Logger
	util.WaitGroupWrapper
	Csession *SessionTCP //会话
}

//NewClientTCP 新建
func NewClientTCP(ctx context.Context, url string, h *Handler) (*ClientTCP, error) {
	logger, _ := util.NewLogger(util.DebugLevel, "")
	logger.SetLevel(util.DebugLevel)
	if h == nil {
		return nil, errors.New("NewClientTCP|Handler不为nil。")
	}
	tcpAddr, err := net.ResolveTCPAddr("tcp4", url)
	if err != nil {
		return nil, errors.New("NewClientTCP|tcpAddr失败:" + err.Error())
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return nil, errors.New("NewClientTCP|连接服务端失败:" + err.Error())
	}
	if conn.SetNoDelay(false) != nil {
		return nil, errors.New("NewClientTCP|设定操作系统是否应该延迟数据包传递失败:" + err.Error())
	}
	c := &ClientTCP{
		Ctx:      ctx,
		conn:     conn,
		URL:      url,
		handler:  h,
		stopChan: make(chan struct{}),
		Logger:   logger,
	}
	c.Csession = NewSessionTCP(conn)
	//设置IO超时
	if err := conn.SetWriteDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
		c.releaseByError()
		return nil, errors.New("NewClientTCP|写入ProtocolMagicNumber超时:" + err.Error())
	}
	if _, err := conn.Write(util.Uint32ToBytes(ProtocolMagicNumber)); err != nil {
		c.releaseByError()
		return nil, errors.New("NewClientTCP|写入ProtocolMagicNumber失败:" + err.Error())
	}
	if err := conn.SetWriteDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
		c.releaseByError()
		return nil, errors.New("NewClientTCP|读取client.ID超时:" + err.Error())
	}
	var id int64
	if id, err = c.Csession.readInt64(); err != nil {
		c.releaseByError()
		return nil, errors.New("NewClientTCP|读取client.ID失败:" + err.Error())
	}
	c.Csession.ID = id
	c.Logger.SetMark("ClientTCP." + strconv.Itoa(int(id)))
	return c, nil
}

//Close 关闭
func (c *ClientTCP) Close() {
	c.closeOnce.Do(func() {
		close(c.stopChan)
	})
}

//releaseByError 释放
func (c *ClientTCP) releaseByError() {
	c.closeOnce.Do(func() {
		c.conn.Close()
		close(c.stopChan)
		c.Csession.Release()
	})
}

//Run 运行
func (c *ClientTCP) Run() {
	c.Logger.Info("Run|连接到服务器")
	c.Wrap(c.sendLoop)
	c.Wrap(c.receiveLoop)
	c.Wait()
	if c.OnCloseSessionTCP != nil {
		c.OnCloseSessionTCP(c.Csession)
	}
	c.Logger.Info("Run|ClientTCP关闭。")
	c.Csession.Release()
}

//Send 发送
func (c *ClientTCP) Send(f *FrameSlice) error {
	select {
	case <-c.stopChan:
		return ErrClientTCPClose
	default:
		c.SendChan <- f
		return nil
	}
}

//sendLoop 发送
func (c *ClientTCP) sendLoop() {
	heartbeat := time.NewTicker(DefaultHeartbeatDuration)
	send := time.NewTicker(3000 * time.Microsecond)
	c.SendChan = make(chan *FrameSlice, 1024)
	defer func() {
		if r := recover(); r != nil {
			c.Logger.Error("sendLoop|defer错误：", r, string(debug.Stack()))
		}
		heartbeat.Stop()
		send.Stop()
		c.Close()
	}()
	for {
		select {
		case <-heartbeat.C:
			if err := c.Csession.WriteFrameDataPromptly(FrameHeartbeat); err != nil {
				c.Logger.Error("sendLoop|写入心跳包失败:", err)
				return
			}
		case f := <-c.SendChan:
			if f != nil {
				if (c.Csession.wSlot.availableCursor + int32(f.GetFrameLength())) < int32(util.BytesPoolLenght) {
					n := f.WriteToBytes(c.Csession.wSlot.buf[c.Csession.wSlot.availableCursor:])
					c.Csession.wSlot.availableCursor += int32(n)
				} else {
					if err := c.conn.SetWriteDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
						c.Logger.Error("sendLoop|写入数据超时:", err)
						return
					}
					if _, err := c.conn.Write(c.Csession.wSlot.buf[:c.Csession.wSlot.availableCursor]); err != nil {
						c.Logger.Error("sendLoop|写入数据失败:", err)
						return
					}
					c.Csession.wSlot.availableCursor = int32(f.WriteToBytes(c.Csession.wSlot.buf))
				}
			}
		case <-send.C:
			if c.Csession.wSlot.availableCursor >= int32(FrameHeadLength) {
				if err := c.conn.SetWriteDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
					c.Logger.Error("sendLoop|写入数据超时:", err)
					return
				}
				if _, err := c.conn.Write(c.Csession.wSlot.buf[:c.Csession.wSlot.availableCursor]); err != nil {
					c.Logger.Error("sendLoop|写入数据失败:", err)
					return
				}
				c.Csession.wSlot.availableCursor = 0
			}
		case <-c.Ctx.Done():
			c.Close()
		case <-c.stopChan:
			//通知服务端关闭
			c.Csession.WriteFrameDataPromptly(FrameExit)
			return
		}
	}
}

//receiveLoop 接受
func (c *ClientTCP) receiveLoop() {
	defer func() {
		if r := recover(); r != nil {
			c.Logger.Error("receiveLoop|defer错误：", r, string(debug.Stack()))
		}
		c.Close()
	}()
	for {
		var err error
		if err = c.Csession.ioRead(); err != nil {
			if err != nil && err != io.EOF && !strings.Contains(err.Error(), "use of closed network connection") {
				c.Logger.Error("receiveLoop|ioRead错误：", err.Error())
			}
			break
		}
		ft := c.Csession.getFrameType()
		for ft > FrameTypeNil {
			l := int(util.BytesToUint32(c.Csession.rBuf[c.Csession.r : c.Csession.r+4]))
			if ft > FrameTypeExit {
				if err = c.handler.Route(c.Csession); err != nil {
					if err != nil && err != io.EOF && !strings.Contains(err.Error(), "use of closed network connection") {
						c.Logger.Error("receiveLoop|Route错误：", err.Error())
					}
					break
				}
			} else {
				if ft == FrameTypeExit {
					return
				}
			}
			c.Csession.r += l
			ft = c.Csession.getFrameType()
		}
	}
}
