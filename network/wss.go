package network

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ratel-online/core/log"
	"github.com/ratel-online/core/protocol"
)

const (
	websocketPongWait   = 70 * time.Second
	websocketPingPeriod = 25 * time.Second
	websocketWriteWait  = 10 * time.Second
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
	_ = conn.SetReadDeadline(time.Now().Add(websocketPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(websocketPongWait))
	})

	pingDone := make(chan struct{})
	defer close(pingDone)
	go func() {
		ticker := time.NewTicker(websocketPingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				deadline := time.Now().Add(websocketWriteWait)
				if err := conn.WriteControl(websocket.PingMessage, nil, deadline); err != nil {
					return
				}
			case <-pingDone:
				return
			}
		}
	}()
	err = handle(protocol.NewWebsocketReadWriteCloser(conn))
	if err != nil {
		log.Error(err)
	}
}
