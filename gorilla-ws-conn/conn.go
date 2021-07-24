// Package gorilla_ws_conn implements the Conn interface from
// https://github.com/SirGFM/go-chat-i-guess  over a WebSocket connection
// from https://github.com/gorilla/websocket.
package gorilla_ws_conn

import (
    gochat "github.com/SirGFM/go-chat-i-guess"
    gows "github.com/gorilla/websocket"
    "log"
    "net/http"
    "sync"
    "sync/atomic"
    "time"
)

// defaultPing is sent on ping messages as the application data.
const defaultPing = "go_chat_i_guess says hi"

// module is the string used when logging messages from this package.
const module = "go-chat-i-guess/gorilla-ws-conn"

// gwsConn wrap a gorilla/ws connection into a gochat.Conn.
type gwsConn struct {
    // The gorilla WebSocket connection.
    conn *gows.Conn

    // How long the connection waits until sending a ping back to the
    // remote endpoint.
    timeout time.Duration

    // ticker generates a message on a channel if `timeout` elapsed without
    // receiving any message.
    ticker *time.Ticker

    // timeoutCount counts the number of consecutive timeouts that happened.
    timeoutCount uint32

    // sendMutex synchronizes write operations on `conn`.
    sendMutex sync.Mutex

    // Whether the connection is currently active.
    active uint32

    // stop signals, by getting closed, that the connection should get
    // closed.
    stop chan struct{}
}

// isRunning check if the connection is still active.
func (c *gwsConn) isActive() bool {
    return atomic.LoadUint32(&c.active) == 1
}

// Close the connection.
func (c *gwsConn) Close() error {
    if atomic.CompareAndSwapUint32(&c.active, 1, 0) {
        c.sendMutex.Lock()
        c.conn.Close()
        c.conn = nil
        c.sendMutex.Unlock()

        c.ticker.Stop()
        close(c.stop)
    }

    return nil
}

// resetTimeout reset the last timeout.
//
// This must be called whenever this connections receives any message from
// its remote endpoint.
func (c *gwsConn) resetTimeout() {
    atomic.StoreUint32(&c.timeoutCount, 0)
    c.ticker.Reset(c.timeout)
}

// Recv blocks until a new message was received.
func (c *gwsConn) Recv() (string, error) {
    for c.conn != nil {
        typ, txt, err := c.conn.ReadMessage()
        if err != nil {
            c.Close()
            return "", gochat.ConnEOF
        }

        c.resetTimeout()

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

// send the message, properly synchronizing the connection.
func (c *gwsConn) send(mType int, data []byte) error {
    var err error

    c.sendMutex.Lock()
    if c.conn != nil {
        err = c.conn.WriteMessage(mType, data)
    } else {
        err = gochat.ConnEOF
    }
    c.sendMutex.Unlock()
    return err
}

// SendStr send `msg`, previously formatted by the caller.
func (c *gwsConn) SendStr(msg string) error {
    mType := gows.TextMessage

    if c.conn == nil {
        return gochat.ConnEOF
    } else if len(msg) == 0 {
        // In case of empty message, just change it into a pong, to check
        // if the remote endpoint is alive.
        mType = gows.PongMessage
    }

    return c.send(mType, []byte(msg))
}

// detectTimeout wait some time checking if the connection timed out.
//
// After two consecutive timeouts, the connection is automatically closed.
func (c *gwsConn) detectTimeout() {
    for c.isActive() {
        select {
        case <-c.ticker.C:
            if atomic.CompareAndSwapUint32(&c.timeoutCount, 0, 1) {
                // Try to ping the remote endpoint and see if there's any
                // response.
                err := c.send(gows.PingMessage, []byte(defaultPing))
                if err != nil {
                    log.Printf("%s: Couldn't ping on timeout: %+v", module, err)
                    c.Close()
                }
            } else {
                // This is the second time that this connection timed out,
                // so just close it.
                c.Close()
            }
        case <-c.stop:
            /* Do nothing and simply exit */
        }
    }
}

// ping handle received ping messages.
//
// The WebSocket protocol defines that the receiver must respond with a
// pong with the same `appData` as received.
//
// Instead of using the default ping handler, it's important to use a
// custom one to guarantee that this write isn't concurrent to other
// messages.
//
// Also, since this implies on activity on the channel, the connection's
// timeout is reset.
func (c *gwsConn) ping(appData string) error {
    c.resetTimeout()

    return c.send(gows.PongMessage, []byte(appData))
}

// pong handle received pong messages.
//
// The WebSocket protocol defines two kinds of pong messages: unrequested
// pongs, which may be ignored, and pongs in response to pings. For the
// later, `appData` should be the same that was sent by this connection. In
// both cases, this message is used to reset the time without messages.
func (c *gwsConn) pong(appData string) error {
    c.resetTimeout()
    return nil
}

// Upgrade a HTTP connection to a Chat Connection.
//
// The supplied `upgrader` is used to upgrade the HTTP request into a
// WebSocket connection. Other than that, this connection's times out if it
// doesn't receive any message from its remote endpoint in `timeout`. Upon
// timing out, the connection will first try to ping the remote end point,
// but it will close if there's no response in a timely manner.
//
// Gorilla/ws's documentation specifies that if `SetReadDeadline` is set
// and a read times out, the websocket becomes corrupt. To work around
// that, `NewConn` spawns a goroutine to manually detect timeouts.
func NewConn(upgrader gows.Upgrader, timeout time.Duration,
        w http.ResponseWriter, req *http.Request) (gochat.Conn, error) {

    conn, err := upgrader.Upgrade(w, req, nil)
    if err != nil {
        return nil, err
    }

    c := &gwsConn {
        conn: conn,
        timeout: timeout,
        ticker: time.NewTicker(timeout),
        timeoutCount: 0,
        active: 1,
        stop: make(chan struct{}),
    }
    conn.SetPingHandler(c.ping)
    conn.SetPongHandler(c.pong)
    go c.detectTimeout()

    return c, nil
}
