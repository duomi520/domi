package transport

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/duomi520/domi/util"
)

//ClientTCP 客户端
type ClientTCP struct {
	Ctx  context.Context
	conn *net.TCPConn
	URL  string

	sendChan chan *FrameSlice

	handler           *Handler
	OnCloseSessionTCP func(Session)

	closeFlag int32         //关闭状态  1:关闭
	stopChan  chan struct{} //退出信号
	closeOnce sync.Once
	Logger    *util.Logger
	util.WaitGroupWrapper
	Csession *SessionTCP
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
		Ctx:       ctx,
		conn:      conn,
		URL:       url,
		handler:   h,
		closeFlag: 0,
		stopChan:  make(chan struct{}),
		Logger:    logger,
	}
	c.Csession = NewSessionTCP(conn)
	//设置IO超时
	if err := conn.SetWriteDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
		return nil, errors.New("NewClientTCP|写入ProtocolMagicNumber超时:" + err.Error())
	}
	if _, err := conn.Write(util.Uint32ToBytes(ProtocolMagicNumber)); err != nil {
		return nil, errors.New("NewClientTCP|写入ProtocolMagicNumber失败:" + err.Error())
	}
	if err := conn.SetWriteDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
		return nil, errors.New("NewClientTCP|读取client.ID超时:" + err.Error())
	}
	var id int64
	if id, err = c.Csession.readInt64(); err != nil {
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
	c.Csession.Close()
}

//sendLoop 发送
func (c *ClientTCP) sendLoop() {
	heartbeat := time.NewTicker(DefaultHeartbeatDuration)
	send := time.NewTicker(3000 * time.Microsecond)
	c.sendChan = make(chan *FrameSlice, 1024)
	c.Csession.wBuf.buf = util.BytesPoolGet()
	c.Csession.wBuf.w = 0
	for {
		select {
		case <-heartbeat.C:
			if err := c.Csession.WriteFrameDataPromptly(FrameHeartbeat); err != nil {
				c.Logger.Error("sendLoop|写入心跳包失败。")
				goto end
			}
		case f := <-c.sendChan:
			if (c.Csession.wBuf.w + f.GetFrameLength()) < util.BytesPoolLenght {
				n := f.WriteToBytes(c.Csession.wBuf.buf[c.Csession.wBuf.w:])
				c.Csession.wBuf.w += n
			} else {
				if err := c.conn.SetWriteDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
					c.Logger.Error("sendLoop|写入数据超时。")
					goto end
				}
				if _, err := c.conn.Write(c.Csession.wBuf.buf[:c.Csession.wBuf.w]); err != nil {
					c.Logger.Error("sendLoop|写入数据失败。")
					goto end
				}
				c.Csession.wBuf.w = f.WriteToBytes(c.Csession.wBuf.buf)
			}
		case <-send.C:
			if c.Csession.wBuf.w >= FrameHeadLength {
				if err := c.conn.SetWriteDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
					c.Logger.Error("sendLoop|写入数据超时。")
					goto end
				}
				if _, err := c.conn.Write(c.Csession.wBuf.buf[:c.Csession.wBuf.w]); err != nil {
					c.Logger.Error("sendLoop|写入数据失败。")
					goto end
				}
				c.Csession.wBuf.w = 0
			}
		case <-c.Ctx.Done():
			c.Close()
		case <-c.stopChan:
			goto end
		}
	}
end:
	atomic.StoreInt32(&c.closeFlag, 1)
	c.Csession.WriteFrameDataPromptly(FrameExit) //通知服务端关闭
	c.Logger.Debug("sendLoop|关闭。")
	c.conn.Close()
	heartbeat.Stop()
	send.Stop()
	util.BytesPoolPut(c.Csession.wBuf.buf)
	//	close(c.sendChan)  TODO 合理释放
}

//SendToQueue 发送到队列
func (c *ClientTCP) SendToQueue(f *FrameSlice) error {
	var err error
	if atomic.LoadInt32(&c.closeFlag) > 0 {
		return errors.New("Send|连接已关闭。")
	}
	c.sendChan <- f
	return err
}

//receiveLoop 接受
func (c *ClientTCP) receiveLoop() {
	var err error
	for {
		if atomic.LoadInt32(&c.closeFlag) > 0 {
			goto end
		}
		if err = c.Csession.ioRead(); err != nil {
			goto end
		}
		c.Csession.readFrameData()
		for i := 0; i < len(c.Csession.frameSlices); i++ {
			c.Csession.currentFrameSlice = i
			switch c.Csession.frameSlices[i].GetFrameType() {
			case FrameTypeHeartbeat:
				continue
			case FrameTypeExit:
				goto end
			}
			c.handler.Route(c.Csession)
		}
	}
end:
	c.Close()
	if err != nil && err != io.EOF {
		if !strings.Contains(err.Error(), "use of closed network connection") {
			c.Logger.Error("receiveLoop|错误：", err.Error())
		} else {
			err = nil
		}
	}
	c.Logger.Debug("receiveLoop|关闭", err, c.Csession.currentFrameSlice)
}
