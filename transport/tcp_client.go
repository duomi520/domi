package transport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime/debug"
	"strings"
	"time"

	"github.com/duomi520/domi/util"
)

//ClientTCP 客户端
type ClientTCP struct {
	Ctx     context.Context
	conn    *net.TCPConn
	URL     string
	limiter *util.Limiter //限流器
	handler *Handler

	Logger   *util.Logger
	Csession *SessionTCP //会话
}

//NewClientTCP 新建
func NewClientTCP(ctx context.Context, url string, h *Handler, sd *util.Dispatcher, limiter *util.Limiter, cbc *util.CircuitBreakerConfigure) (*ClientTCP, error) {
	logger, _ := util.NewLogger(util.ErrorLevel, "")
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
		Ctx:     ctx,
		conn:    conn,
		URL:     url,
		limiter: limiter,
		handler: h,
		Logger:  logger,
	}
	c.Csession = NewSessionTCP(conn, c.handler, cbc)
	c.Csession.dispatcher = sd
	//设置IO超时
	if err := conn.SetWriteDeadline(time.Now().Add(DefaultDeadlineDuration)); err != nil {
		c.Csession.Close()
		return nil, errors.New("NewClientTCP|写入ProtocolMagicNumber超时:" + err.Error())
	}
	pmn := make([]byte, 4)
	util.CopyUint32(pmn, ProtocolMagicNumber)
	if _, err := conn.Write(pmn); err != nil {
		c.Csession.Close()
		return nil, errors.New("NewClientTCP|写入ProtocolMagicNumber失败:" + err.Error())
	}
	c.Logger.SetMark("ClientTCP")
	return c, nil
}

//Heartbeat 写入心跳包
func (c *ClientTCP) Heartbeat() error {
	if err := c.Csession.WriteFrameDataPromptly(FrameHeartbeatS); err != nil {
		if !strings.Contains(err.Error(), "use of closed network connection") {
			c.Logger.Error("Heartbeat|写入心跳包失败:", err)
		}
		return err
	}
	return nil
}

//Init 初始化
func (c *ClientTCP) Init() {}

//WaitInit 准备好
func (c *ClientTCP) WaitInit() {}

//Run 运行
func (c *ClientTCP) Run() {
	defer func() {
		if r := recover(); r != nil {
			c.Logger.Error("Run|defer错误：", r, string(debug.Stack()))
		}
		c.Csession.Close()
		c.Logger.Debug("Run|ClientTCP关闭。")
	}()
	c.Logger.Debug(fmt.Sprintf("Run|连接到服务器%s。", c.URL))
	for {
		n, err := c.Csession.ioRead()
		if err != nil {
			if err != io.EOF && !strings.Contains(err.Error(), "use of closed network connection") {
				c.Logger.Error("Run|ioRead错误：", err.Error())
			}
			return
		}
		//限流器阻塞
		if c.limiter != nil {
			c.limiter.Wait(int64(n))
		}
		//处理数据
		for {
			ft := c.Csession.getFrameType()
			if err := c.handler.route(ft, c.Csession); err != nil {
				//读完缓存
				if ft == FrameTypeNil {
					break
				}
				//收到退出指令
				if ft == FrameTypeExit {
					return
				}
				c.Logger.Error("Run|错误：", err.Error())
			}
			c.Csession.r += int(util.BytesToUint32(c.Csession.rBuf[c.Csession.r : c.Csession.r+4]))
		}
	}
}
