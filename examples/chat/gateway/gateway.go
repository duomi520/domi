package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/duomi520/domi"
	"github.com/gorilla/websocket"
)

//定义
const (
	ChannelRoom uint16 = 50 + iota
	ChannelMsg
)

var homeTemplate = template.Must(template.ParseFiles("home.html"))

var gate *domi.Node

func main() {
	app := domi.NewMaster()
	gate = domi.NewNode(app.Ctx, "1/gate/", ":7081", ":9521", []string{"localhost:2379"})
	app.RunAssembly(gate)
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
	revChan = make(chan []byte, 1024)
	conns = make([]*websocket.Conn, 0, 1024)
	go run()
	gate.SerialProcess(ChannelRoom, reply)
	defer gate.Unsubscribe(ChannelRoom)
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

type wb struct {
	conn *websocket.Conn
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
	if err != nil {
		log.Println("Upgrade错误：", err)
		return
	}
	conns = append(conns, conn)
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				log.Println("IsUnexpectedCloseError:", err.Error())
			}
			return
		}
		err = gate.Call(ChannelMsg, message, domi.TypeTell)
		if err != nil {
			log.Println("err:", err)
		}
	}

}

func reply(ctx *domi.ContextMQ) {
	revChan <- ctx.Request
}

var revChan chan []byte

//初步演示，暂不考虑竟态及退出
var conns []*websocket.Conn

func run() {
	for {
		select {
		case data := <-revChan:
			for _, v := range conns {
				v.WriteMessage(websocket.BinaryMessage, data)
			}
		}
	}
}
