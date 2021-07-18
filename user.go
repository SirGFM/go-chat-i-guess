package go_chat_i_guess

import (
    "io"
    "time"
    "sync/atomic"
)

// Conn is a generic interface for sending and receiving messages.
type Conn interface {
    io.Closer

    // Recv blocks until a new message was received.
    Recv() (string, error)

    // SendStr send `msg`, previously formatted by the caller.
    SendStr(msg string) error
}

// user represent a user connected to a channel.
type user struct {
    // The user's name.
    name string

    // last time this user was sent a message from the server.
    last time.Time

    // The channel to which this user is connected.
    channel ChatChannel

    // The connection to the user's remote endpoint.
    conn Conn

    // Whether the user is currently running.
    running uint32
}

// isRunning check if the user is still running.
func (u *user) isRunning() bool {
    return atomic.LoadUint32(&u.running) == 1
}

// run wait for new messages from the user and forward them to the channel.
func (u *user) run() {
    for u.isRunning() {
        msg, err := u.conn.Recv()
        if err != nil {
            u.Close()
            return
        }

        u.channel.NewBroadcast(msg, u.name)
    }
}

// GetName return the user's name.
func (u *user) GetName() string {
    return u.name
}

// SendStr a new, formatted, message to the user.
func (u *user) SendStr(msg string) error {
    return u.conn.SendStr(msg)
}

// Close the user's connection and any other resource, like its handling
// goroutine.
//
// This can safely be called multiple times (and from multiple goroutines),
// as it will only run on the first call.
func (u *user) Close() error {
    if atomic.CompareAndSwapUint32(&u.running, 1, 0) {
        u.conn.Close()
    }

    return nil
}

// newUser create a new user named `name`, connected to `channel` and
// receiving and sending messages to `conn`.
//
// `newUser()` executes a new goroutine to handle messages received by the
// user, forwarding those message to the channel. To stop this goroutine
// and clean up its resources, call `c.Close()`.
func newUser(name string, channel ChatChannel, conn Conn) *user {
    u := &user {
        name: name,
        last: time.Now(),
        channel: channel,
        conn: conn,
        running: 1,
    }

    go u.run()

    return u
}
