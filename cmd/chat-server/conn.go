package main

import (
    gows "github.com/gorilla/websocket"
    gochat "github.com/SirGFM/go-chat-i-guess"
    gochat_ws "github.com/SirGFM/go-chat-i-guess/gorilla-ws-conn"
    "net/http"
    "time"
)

// How long a remote connection may stay idle.
const timeout = time.Minute

// Upgrade a HTTP connection to a Chat Connection.
func newConn(w http.ResponseWriter, req *http.Request) (gochat.Conn, error) {
    return gochat_ws.NewConn(upgrader, timeout, w, req)
}

var upgrader gows.Upgrader

func ignoreOrigin(r *http.Request) bool {
    return true
}

func setUpgrader(args Args) {
    upgrader = gows.Upgrader {
        ReadBufferSize:  args.ReadSize,
        WriteBufferSize: args.WriteSize,
    }
    if args.IgnoreOrigin {
        upgrader.CheckOrigin = ignoreOrigin
    }
}
