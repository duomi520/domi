package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"text/template"
	"time"

	"github.com/duomi520/domi/transport"
	"github.com/duomi520/domi/util"
)

var homeTemplate = template.Must(template.ParseFiles("home.html"))
var server *transport.ServerWebsocket
var showFrameType uint16 = 55

func testShowFunc(s transport.Session) {
	fs := s.GetFrameSlice()
	fmt.Println(s.GetID(), string(fs.GetData()))
	s.WriteFrameDataPromptly(fs)
}

func main() {
	tc := transport.NewHandler()
	sfID := util.NewSnowFlakeID(55, time.Now().UnixNano())
	server = transport.NewServerWebsocket(context.TODO(), tc, sfID)
	tc.HandleFunc(showFrameType, testShowFunc)
	go server.Run()
	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(w, r)
	})
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)
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

func serveWs(w http.ResponseWriter, r *http.Request) {
	server.Accept(w, r)
}
