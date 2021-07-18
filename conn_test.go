package go_chat_i_guess

import (
    "sync/atomic"
    "time"
)

// A simple mock connection, used to test the chat server without an actual
// HTTP connection.
//
// Although `channel` and `user` may use the `Conn` API to use this
// connection, tests must access this structure directly to simulate
// interactions.
//
// To simulate a message arriving from the client's remote endpoint, push a
// message into `fromClient`:
//
//     c := mockConn()
//     /* Setup the channel and the user. */
//     c.fromClient <- "the message"
//
// On the other hand, to simulate a client receiving a message, pop a
// message from `fromServer`. Be sure to check that the channel isn't
// empty, to avoid causing tests to hang:
//
//     c := mockConn()
//     /* Setup the channel and the user. */
//     select {
//     case <-c.fromServer:
//         /* Got a message back from the server. */
//     default:
//         t.Error("Server did not respond.")
//     }
type mockConn struct {
    // fromClient simulates incoming messages (from the server's
    // perspectives) from the client's remote endpoint. Therefore, tests
    // must push directly to this channel.
    fromClient chan string

    // fromServer simulates outgoing messages (from the server's
    // perspectives) to the client's remote endpoint. Therefore, tests must
    // read directly to this channel
    fromServer chan string

    // stop signals, by getting closed, that the channel should get closed.
    stop chan struct{}

    // Whether the connection is currently running.
    running uint32
}

// isClosed check if the connection is closed.
func (mc *mockConn) isClosed() bool {
    return atomic.LoadUint32(&mc.running) == 0
}

// Close the connection.
//
// This can safely be called multiple times without any issue.
func (mc *mockConn) Close() error {
    if atomic.CompareAndSwapUint32(&mc.running, 1, 0) {
        close(mc.stop)
    }
    return nil
}

// Recv blocks until a new message was received.
func (mc *mockConn) Recv() (string, error) {
    var msg string

    select {
    case msg = <-mc.fromClient:
        return msg, nil
    case <-mc.stop:
        return msg, ConnEOF
    }
}

// SendStr send `msg`, previously formatted by the caller.
func (mc *mockConn) SendStr(msg string) error {
    if mc.isClosed() {
        return ConnEOF
    }

    mc.fromServer <- msg

    return nil
}

// TestSend send a message from the client to the server.
func (mc *mockConn) TestSend(msg string) error {
    if mc.isClosed() {
        return ConnEOF
    }

    mc.fromClient <- msg
    return nil
}

// TestRecv wait for `timeout` to receive a message from the server.
func (mc *mockConn) TestRecv(timeout time.Duration) (string, error) {
    select {
    case msg := <-mc.fromServer:
        return msg, nil
    case <-time.After(timeout):
        return "", TestTimeout
    case <-mc.stop:
        return "", ConnEOF
    }
}

// NewMockConn() create a dummy, mock connection that may be used in tests.
func NewMockConn() Conn {
    return &mockConn {
        fromClient: make(chan string),
        fromServer: make(chan string, 100),
        stop: make(chan struct{}),
        running: 1,
    }
}
