package main

import (
    gochat "github.com/SirGFM/go-chat-i-guess"
    gows "github.com/gorilla/websocket"
    "net/http"
)

type gwsConn struct {
    conn *gows.Conn
}

// Close the connection.
func (c *gwsConn) Close() error {
    if c.conn != nil {
        c.conn.Close()
        c.conn = nil
    }

    return nil
}

// Recv blocks until a new message was received.
func (c *gwsConn) Recv() (string, error) {
    for c.conn != nil {
        typ, txt, err := c.conn.ReadMessage()
        if err != nil {
            c.Close()
            return "", gochat.ConnEOF
        }

        switch typ {
        case gows.CloseMessage:
            c.Close()
            return "", gochat.ConnEOF
        case gows.TextMessage:
            return string(txt), nil
        default:
            continue
        }
    }

    return "", gochat.ConnEOF
}

// SendStr send `msg`, previously formatted by the caller.
func (c *gwsConn) SendStr(msg string) error {
    if c.conn == nil {
        return gochat.ConnEOF
    }

    return c.conn.WriteMessage(gows.TextMessage, []byte(msg))
}

// Upgrade a HTTP connection to a Chat Connection.
func newConn(w http.ResponseWriter, req *http.Request) (gochat.Conn, error) {
    conn, err := upgrader.Upgrade(w, req, nil)
    if err != nil {
        return nil, err
    }

    c := &gwsConn {
        conn: conn,
    }
    return c, nil
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
