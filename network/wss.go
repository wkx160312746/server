package network

import (
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/ratel-online/core/log"
	"github.com/ratel-online/core/protocol"
)

type Websocket struct {
	addr string
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func NewWebsocketServer(addr string) Websocket {
	return Websocket{addr: addr}
}

func (w Websocket) Serve() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", serveWs)
	mux.HandleFunc("/healthz", healthz)
	log.Infof("Websocket server listener on %s\n", w.addr)
	return http.ListenAndServe(w.addr, mux)
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func serveWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error(err)
		return
	}
	err = handle(protocol.NewWebsocketReadWriteCloser(conn))
	if err != nil {
		log.Error(err)
	}
}
