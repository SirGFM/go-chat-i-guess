package go_chat_i_guess

import (
    "io"
    "log"
    "time"
    "sync/atomic"
)

// Conn is a generic interface for sending and receiving messages.
type Conn interface {
    io.Closer

    // Recv blocks until a new message was received.
    Recv() (string, error)

    // SendStr send `msg`, previously formatted by the caller.
    //
    // Note that the server may send an empty message to check if this
    // connection is still active.
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

    // logger used by the user to report events. If this is nil, no message
    // shall be logged!
    logger *log.Logger

    // Whether debug messages should be logged.
    debugLog bool
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
            if u.logger != nil {
                u.logger.Printf("[ERROR] go_chat_i_guess/user: Failed to receive the message.\n\tuser: \"%s\"\n\terror: %+v",
                        u.name, err)
            }

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
        if u.debugLog && u.logger != nil {
            u.logger.Printf("[DEBUG] go_chat_i_guess/user: Closing connection...\n\tuser: \"%s\"",
                    u.name)
        }

        u.conn.Close()
    }

    return nil
}

// RunAndWait handle requests send from the remote client in a blocking
// manner, until the connection gets closed.
//
// This is useful when the server (HTTP, TCP etc) that is running the Chat
// Server already spawns a new goroutine for each received connection. In
// this scenario, instead of calling `newUserBg()` and spawning yet another
// goroutine, it's possible to call `newUser()` followed by `RunAndWait()`.
//
// The calling `user` will be closed when this function returns.
func (u *user) RunAndWait() {
    defer u.Close()

    u.run()
}

// newUserBg create a new user named `name`, connected to `channel` and
// receiving and sending messages to `conn`.
//
// `newUser()` executes a new goroutine to handle messages received by the
// user, forwarding those message to the channel. To stop this goroutine
// and clean up its resources, call `c.Close()`.
//
// If `channel` or `conn` is nil, then this function will panic!
func newUserBg(name string, channel ChatChannel, conn Conn,
        logger *log.Logger, debugLog bool) *user {

    if channel == nil {
        panic("go_chat_i_guess/user newUserBg: nil channel")
    } else if conn == nil {
        panic("go_chat_i_guess/user newUserBg: nil conn")
    }

    u := newUser(name, channel, conn, logger, debugLog)

    go u.run()

    return u
}

// newUser create a new user named `name`, connected to `channel` and
// receiving and sending messages to `conn`.
//
// If `channel` or `conn` is nil, then this function will panic!
func newUser(name string, channel ChatChannel, conn Conn,
        logger *log.Logger, debugLog bool) *user {

    return &user {
        name: name,
        last: time.Now(),
        channel: channel,
        conn: conn,
        running: 1,
        logger: logger,
        debugLog: debugLog,
    }
}
