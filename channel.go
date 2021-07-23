package go_chat_i_guess

import (
    "io"
    "log"
    "time"
    "sync"
    "sync/atomic"
)

// For how long a given channel should be allowed
const defIdleTimeout = time.Minute * 5

// message represent a message received by the server, alongside its
// metadata.
type message struct {
    // Date when the message was received by the server.
    Date time.Time

    // Message received by the server.
    Message string

    // From whom the message was sent. Empty for automatic/system messages.
    From string

    // To whom the message will be sent. Empty for broadcasts and
    // omitted when encoded into JSON.
    To string `json:-`
}

// Encode the message into a string that may be sent to users.
func (m *message) Encode() string {
    t := m.Date.Format("2006-01-02 - 15:04:05 (-0700)")
    u := ""
    if len(m.From) > 0 {
        u = m.From + ": "
    }
    return t + " > " + u + m.Message
}

// MessageEncoder encodes a given message into the string that will be
// sent by the channel.
type MessageEncoder interface {
    // Encode the message described in the parameter into the string that
    // will be sent to the channel.
    //
    // Returning the empty string will cancel sending the message, which
    // may be useful for command processing. It may also be used to reply
    // directly, and exclusively, to the sender after processing the
    // message.
    //
    // `from` is set internally by the `ChatChannel`, based on the `Conn`
    // that received this message and forwarded it to the server. Using it
    // to determine whether the requesting user is allowed to do some
    // operation should be safe, as long as users are properly
    // authenticated before generating their connection token.
    Encode(channel ChatChannel, date time.Time, msg, from, to string) string
}

// A chat channel, to which users may connect to.
type channel struct {
    // name of this channel.
    name string

    // encoder optionally encodes/process messages. If not defined,
    // `message.Encode()` is used instead.
    encoder MessageEncoder

    // recv messages sent from a remote client.
    recv chan *message

    // log every message received by this channel.
    log []*message

    // idleTimeout after which this channel is automatically closed, if no
    // user connected to it.
    idleTimeout time.Duration

    // Collection of users currently active in this chat room.
    users map[string]*user

    // lock fields that could be accessed concurrently.
    lockUsers sync.Mutex

    // Whether the channel is currently running.
    running uint32

    // stop signals, by getting closed, that the channel should get closed.
    stop chan struct{}
}

// newMessage queue a new message, setting its `Date` to the current time
// and setting the other fields according to the arguments.
func (c *channel) newMessage(msg, from, to string) {
    packet := &message {
        Date: time.Now(),
        Message: msg,
        From: from,
        To: to,
    }

    c.recv <- packet
}

// NewBroadcast queue a new broadcast message from a specific sender,
// setting its `Date` to the current time and setting the other fields
// according to the arguments.
func (c *channel) NewBroadcast(msg, from string) {
    c.newMessage(msg, from, "")
}

// NewSystemBroadcast queue a new system message (i.e., a message without
// a sender), setting its `Date` to the current time and setting
// `Message` to `msg`.
func (c *channel) NewSystemBroadcast(msg string) {
    c.newMessage(msg, "", "")
}

// NewSystemWhisper queue a new system message (i.e., a message without a
// sender) to a specific receiver, setting its `Date` to the current time
// and setting `Message` to `msg`.
func (c *channel) NewSystemWhisper(msg, to string) {
    c.newMessage(msg, "", to)
}

// Name retrieve the channel's name.
func (c *channel) Name() string {
    return c.name
}

// GetUsers retrieve the list of connected users in this channel. If
// `list` is supplied, the users are appended to the that list, so be
// sure to empty it before calling this function.
func (c *channel) GetUsers(list []string) []string {
    c.lockUsers.Lock()
    for k := range c.users {
        list = append(list, k)
    }
    c.lockUsers.Unlock()

    return list
}

// IsClosed check if the channel is closed.
//
// The channel reports itself as being closed as soon as `c.Close()` was
// first called, regardless of whether or not it's finished running.
func (c *channel) IsClosed() bool {
    return atomic.LoadUint32(&c.running) == 0
}

// waitClient wait until any client connects to the channel and sends their
// first message. If this doesn't happen in `c.idleTimeout`, this function
// fails and return with an error.
//
// `waitClient()` should only be called when the channel has no clients
// connected and from the channel's main goroutine. This way, this is
// guaranteed to return on the first client connection and message, which
// doesn't need to be broadcast.
func (c *channel) waitClient() error {
    timeout := time.NewTimer(c.idleTimeout)

    select {
    case msg := <-c.recv:
        // Since a message was received before the idle timeout,
        // clear the timer before exiting.
        if !timeout.Stop() {
            <-timeout.C
        }

        // Re-send this message, so the connecting user may see it.
        c.recv <- msg
        return nil
    case <-timeout.C:
        return IdleChannel
    case <-c.stop:
        return ChannelClosed
    }
}

// messageUser send `msgStr` to the specified user `u`.
//
// If the channel fails to send the message to the user, the user gets
// removed from the channel. Therefore, the users container must have
// been properly synchronized before calling this.
func (c *channel) messageUserUsafe(u *user, msgStr string) {
    err := u.SendStr(msgStr)
    if err != nil {
        username := u.GetName()
        if err == ConnEOF {
            c.NewSystemBroadcast(username + " exited.")
        } else if err != nil {
            log.Printf("Couldn't send a message to %s on %s: %+v",
                    username, c.name, err)
        }
        delete(c.users, username)
    }
}

// run the channel, broadcasting every message received to every other user.
//
// When `newChannel()` is called, `c.run()` is executed in a new goroutine.
// `c.Close()` should be called to stop the goroutine and clean up its
// resources. Regardless, if every client disconnects and the channel is
// left idle for long enough (more specifically, for `defIdleTimeout`),
// this goroutine will automatically stop.
func (c *channel) run() {
    err := c.waitClient()
    if err != nil {
        c.Close()
        return
    }

    for {
        var msg *message

        select {
        case <-c.stop:
            // The channel should be closed when it receives a `c.stop`,
            // but `c.Close()` may safelly be called multiple times.
            c.Close()
            return
        case msg = <-c.recv:
            break
        }

        if len(msg.To) == 0 {
            c.log = append(c.log, msg)
        }

        var msgStr string
        if c.encoder == nil {
            msgStr = msg.Encode()
        } else {
            // Encode the received message using the application supplied
            // encoder. The application may cancel forwarding this message,
            // by returning the empty string.
            msgStr = c.encoder.Encode(c, msg.Date, msg.Message, msg.From,
                    msg.To)
            if len(msgStr) == 0 {
                continue
            }
        }

        // Broadcast the message to every client. Alternatively, if the
        // message was directed to a specific user, send them the message
        // and skip everything else.
        c.lockUsers.Lock()

        if len(msg.To) > 0 {
            u := c.users[msg.To]
            c.messageUserUsafe(u, msgStr)
        } else {
            for k := range c.users {
                u := c.users[k]
                c.messageUserUsafe(u, msgStr)
            }
        }
        numUsers := len(c.users)

        c.lockUsers.Unlock()

        // If every client disconnected, wait for another connection.
        if numUsers == 0 {
            err = c.waitClient()
            if err != nil {
                c.Close()
                return
            }
        }
    }
}

// ConnectClient add a new client to the channel.
//
// It's entirely up to the caller to initialize the connection used by
// this client, for example upgrading a HTTP request to a WebSocket
// connection.
//
// The `channel` do properly synchronize this function, so it may be
// called by different goroutines concurrently.
func (c *channel) ConnectClient(username string, conn Conn) error {
    u := newUserBg(username, c, conn)

    c.lockUsers.Lock()
    defer c.lockUsers.Unlock()
    if _, ok := c.users[username]; ok {
        return UserAlreadyConnected
    }

    c.users[username] = u
    c.NewSystemBroadcast(username + " entered " + c.name +"!")

    return nil
}

// ConnectClient add a new client to the channel and blocks until the
// client closes the connection to the server.
//
// The `channel` does properly synchronize this function, so it may be
// called by different goroutines concurrently.
//
// On error, `conn` is left unchanged and must be closed by the caller.
//
// Differently from `ConnectClient`, this function handles messages
// from the remote client in the calling goroutine. This may be
// advantageous if the external server already spawns a new goroutine
// to handle each new connection.
func (c *channel) ConnectClientAndWait(username string, conn Conn) error {
    u := newUser(username, c, conn)

    c.lockUsers.Lock()
    if _, ok := c.users[username]; ok {
        c.lockUsers.Unlock()
        return UserAlreadyConnected
    }
    c.users[username] = u
    c.lockUsers.Unlock()

    c.NewSystemBroadcast(username + " entered " + c.name +"!")
    u.RunAndWait()

    return nil
}

// Close the channel, remove every client and stop the goroutine.
func (c *channel) Close() error {
    // Atomically check if `c.running` is 1 and set it to 0. If this
    // returns true, the swap happened and thus this is the first time
    // that `c.Close()` was called.
    if atomic.CompareAndSwapUint32(&c.running, 1, 0) {
        close(c.stop)

        c.lockUsers.Lock()
        for k := range c.users {
            c.users[k].Close()
            delete(c.users, k)
        }
        c.lockUsers.Unlock()
    }

    return nil
}

// The public interface for a chat channel.
type ChatChannel interface {
    io.Closer

    // Name retrieve the channel's name.
    Name() string

    // GetUsers retrieve the list of connected users in this channel. If
    // `list` is supplied, the users are appended to the that list, so be
    // sure to empty it before calling this function.
    GetUsers(list []string) []string

    // NewBroadcast queue a new broadcast message from a specific sender,
    // setting its `Date` to the current time and setting the message's
    // `Message` and sender (its `From`) as `msg` and `from`, respectively.
    NewBroadcast(msg, from string)

    // NewSystemBroadcast queue a new system message (i.e., a message
    // without a sender), setting its `Date` to the current time and
    // setting `Message` to `msg`.
    NewSystemBroadcast(msg string)

    // NewSystemWhisper queue a new system message (i.e., a message without
    // a sender) to a specific receiver, setting its `Date` to the current
    // time and setting `Message` to `msg`.
    NewSystemWhisper(msg, to string)

    // IsClosed check if the channel is closed.
    IsClosed() bool

    // ConnectClient add a new client to the channel.
    //
    // It's entirely up to the caller to initialize the connection used by
    // this client, for example upgrading a HTTP request to a WebSocket
    // connection.
    //
    // The `channel` does properly synchronize this function, so it may be
    // called by different goroutines concurrently.
    //
    // On error, `conn` is left unchanged and must be closed by the caller.
    ConnectClient(username string, conn Conn) error

    // ConnectClient add a new client to the channel and blocks until the
    // client closes the connection to the server.
    //
    // The `channel` does properly synchronize this function, so it may be
    // called by different goroutines concurrently.
    //
    // On error, `conn` is left unchanged and must be closed by the caller.
    //
    // Differently from `ConnectClient`, this function handles messages
    // from the remote client in the calling goroutine. This may be
    // advantageous if the external server already spawns a new goroutine
    // to handle each new connection.
    ConnectClientAndWait(username string, conn Conn) error
}

// newChannel create a new ChatChannel named `name`.
//
// `encoder` may optionally be supplied to process and encode messages
// received by the channel.
//
// `newChannel()` executes a new goroutine to handle messages received by
// the channel. To stop this goroutine and clean up its resources, call
// `c.Close()`.
//
// Regardless, if every client disconnects and the channel is left idle for
// long enough (more specifically, for `defIdleTimeout`), this goroutine
// will automatically stop.
func newChannel(name string, encoder MessageEncoder) ChatChannel {
    c := &channel {
        name: name,
        encoder: encoder,
        recv: make(chan *message, 8),
        idleTimeout: defIdleTimeout,
        users: make(map[string]*user),
        running: 1,
        stop: make(chan struct{}),
    }

    go c.run()

    return c
}
