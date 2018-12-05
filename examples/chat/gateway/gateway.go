package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/duomi520/domi"
	"github.com/gorilla/websocket"
)

//定义
const (
	ChannelRoom uint16 = 50 + iota
	ChannelMsg
	ChannelJoin
	ChannelLeave
)

var homeTemplate = template.Must(template.ParseFiles("home.html"))
var node *domi.Node
var gate *gateway

func main() {
	app := domi.NewMaster()
	gate = &gateway{
		Ctx:     app.Ctx,
		connMap: &sync.Map{},
	}
	var ctx context.Context
	ctx, gate.Cancel = context.WithCancel(context.Background())
	app.RunAssembly(gate)
	node = &domi.Node{
		Ctx:       ctx,
		ExitFunc:  app.Stop,
		Name:      "gate V1.0.1",
		HTTPPort:  ":7081",
		TCPPort:   ":9501",
		Endpoints: []string{"localhost:2379"},
	}
	app.RunAssembly(node)
	node.Subscribe(ChannelRoom, gate.rev)
	defer node.Unsubscribe(ChannelRoom)
	httpServer := &http.Server{
		Addr:           ":8080",
		Handler:        http.DefaultServeMux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "favicon.ico")
	})
	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(w, r)
	})
	go func() {
		err := httpServer.ListenAndServe()
		if err != nil {
			log.Fatal("ListenAndServe: ", err)
		}
	}()

	app.Guard()
	httpServer.Shutdown(context.Background())
}
func serveHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Not found", 404)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	homeTemplate.Execute(w, r.Host)
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  2048,
	WriteBufferSize: 2048,
}

func serveWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	defer conn.Close()
	log.Println(conn.RemoteAddr(), "start")
	defer log.Println(conn.RemoteAddr(), "exit")
	gate.Add(1)
	defer gate.Done()
	if err != nil {
		log.Println("Upgrade错误：", err)
		return
	}
	gate.connMap.Store(conn.RemoteAddr(), conn)
	defer gate.connMap.Delete(conn.RemoteAddr())
	node.Notify(ChannelJoin, nil)
	defer node.Notify(ChannelLeave, nil)
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				log.Println("IsUnexpectedCloseError:", err.Error())
			}
			return
		}
		err = node.Notify(ChannelMsg, message)
		if err != nil {
			log.Println("err:", err)
		}
	}

}

type gateway struct {
	Ctx     context.Context
	Cancel  context.CancelFunc
	connMap *sync.Map
	sync.WaitGroup
}

//Init 初始化
func (g *gateway) Init() {}

//WaitInit 准备
func (g *gateway) WaitInit() {}

//Run 运行
func (g *gateway) Run() {
	<-g.Ctx.Done()
	//外部用户都退出后才能正常关闭
	gate.Wait()
	g.Cancel()
}

func (g *gateway) rev(ctx *domi.ContextMQ) {
	g.connMap.Range(func(k, v interface{}) bool {
		v.(*websocket.Conn).WriteMessage(websocket.BinaryMessage, ctx.Request)
		return true
	})
}
