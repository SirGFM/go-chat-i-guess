package go_chat_i_guess

import (
    "encoding/hex"
    "io"
    "hash/crc32"
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

// getUID generate a unique identifier for the message.
//
// This should only be used for debugging purposes, as performance isn't
// the primary concern of this function.
func (m *message) getUID() string {
    hasher := crc32.NewIEEE()

    date, _ := m.Date.MarshalBinary()
    hasher.Write(date)
    hasher.Write([]byte(m.From))
    hasher.Write([]byte(m.To))
    hasher.Write([]byte(m.Message))

    uid := hasher.Sum(nil)
    return hex.EncodeToString(uid)
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

    // idle reports that the channel has been idle for too long, and should
    // check its users.
    idle *time.Ticker

    // stop signals, by getting closed, that the channel should get closed.
    stop chan struct{}

    // logger used by the channel to report events. If this is nil, no
    // message shall be logged!
    logger *log.Logger

    // Whether debug messages should be logged.
    debugLog bool
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
    if c.debugLog && c.logger != nil {
        c.logger.Printf("[DEBUG] go_chat_i_guess/channel: Sending message...\n\tchannel: \"%s\"\n\tdate: \"%+v\"\n\tfrom: \"%s\"\n\tto: \"%s\"\n\tmessage: \"%s\"\n\tuid: \"%s\"",
                c.name, packet.Date, packet.From, packet.To,
                packet.Message, packet.getUID())
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

// RemoveUserUnsafe remove the user `username` from this channel, assuming
// that access to the map is properly synchronized.
func (c *channel) RemoveUserUnsafe(username string) {
    c.users[username].Close()
    delete(c.users, username)

    c.NewSystemBroadcast(username + " exited " + c.name + "...")
}

// Remove the user `username` from this channel.
func (c *channel) RemoveUser(username string) error {
    var err error = InvalidUser

    c.lockUsers.Lock()
    for k := range c.users {
        if k == username {
            c.RemoveUserUnsafe(k)
            err = nil
            break
        }
    }
    c.lockUsers.Unlock()

    if err == nil && c.debugLog && c.logger != nil {
        c.logger.Printf("[DEBUG] go_chat_i_guess/channel: Removing user...\n\tchannel: \"%s\"\n\tuser: \"%s\"",
                c.name, username)
    } else if err != nil && c.logger != nil {
        c.logger.Printf("[ERROR] go_chat_i_guess/channel: Couldn't remove the user.\n\tchannel: \"%s\"\n\tuser: \"%s\"",
                c.name, username)
    }

    return err
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
            if c.debugLog && c.logger != nil {
                c.logger.Printf("[DEBUG] go_chat_i_guess/channel: Connection to user was closed.\n\tchannel: \"%s\"\n\tusername: \"%s\"",
                        c.name, username)
            }
        } else if err != nil {
            if c.logger != nil {
                c.logger.Printf("[ERROR] go_chat_i_guess/channel: Couldn't send a message to the user.\n\tchannel: \"%s\"\n\tusername: \"%s\"\n\terror: %+v",
                        c.name, username, err)
            }
        }

        c.RemoveUserUnsafe(username)
    }
}

// run the channel, broadcasting every message received to every other user.
//
// When `newChannel()` is called, `c.run()` is executed in a new goroutine.
// `c.Close()` should be called to stop the goroutine and clean up its
// resources. Regardless, if every user disconnects and the channel is
// left idle for long enough (more specifically, for `defIdleTimeout`),
// this goroutine will automatically stop.
func (c *channel) run() {
    for {
        select {
        case <-c.stop:
            // The channel should be closed when it receives a `c.stop`,
            // but `c.Close()` may safelly be called multiple times.
            c.Close()
            return
        case <-c.idle.C:
            c.checkConnections()
        case msg := <-c.recv:
            c.handleMessage(msg)
        }

        // Reset the idle timeout on any active on the channel. If this
        // activity was caused by a timeout, this will simply delay the
        // timeout by a bit.
        c.idle.Reset(c.idleTimeout)
    }
}

// handleMessage encode the received message and broadcast it to every
// connected user.
func (c *channel) handleMessage(msg *message) {
    // uid is only used for debug printing.
    var uid string

    if c.debugLog && c.logger != nil {
        uid = msg.getUID()

        c.logger.Printf("[DEBUG] go_chat_i_guess/channel: Message received.\n\tchannel: \"%s\"\n\tdate: \"%+v\"\n\tfrom: \"%s\"\n\tto: \"%s\"\n\tmessage: \"%s\"\n\tuid: \"%s\"",
                c.name, msg.Date, msg.From, msg.To, msg.Message, uid)
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
            if c.debugLog && c.logger != nil {
                c.logger.Printf("[DEBUG] go_chat_i_guess/channel: Message was filtered out!\n\tuid: \"%s\"",
                        uid)
            }

            return
        }
    }

    // Broadcast the message to every user. Alternatively, if the
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

    c.lockUsers.Unlock()
}

// checkConnections send a dummy message to every connect user to check if
// they are still active, and to remove inactive users.
func (c *channel) checkConnections() {
    if c.debugLog && c.logger != nil {
        c.logger.Printf("[DEBUG] go_chat_i_guess/channel: Idle timeout; checking connectivity...\n\tchannel: \"%s\"",
                c.name)
    }

    c.lockUsers.Lock()
    for k := range c.users {
        u := c.users[k]
        c.messageUserUsafe(u, "")
    }

    // If there's no user after a timeout, simply close the channel.
    if len(c.users) == 0 {
        if c.logger != nil {
            c.logger.Printf("[INFO] go_chat_i_guess/channel: Closing inactive channel...\n\tchannel: \"%s\"",
                    c.name)
        }

        c.Close()
    }
    c.lockUsers.Unlock()
}

// ConnectUser add a new user to the channel.
//
// It's entirely up to the caller to initialize the connection used by
// this user, for example upgrading a HTTP request to a WebSocket
// connection.
//
// The `channel` do properly synchronize this function, so it may be
// called by different goroutines concurrently.
//
// If `conn` is nil, then this function will panic!
func (c *channel) ConnectUser(username string, conn Conn) error {
    if conn == nil {
        panic("go_chat_i_guess/channel ConnectUser: nil conn")
    }

    u := newUserBg(username, c, conn, c.logger, c.debugLog)

    c.lockUsers.Lock()
    defer c.lockUsers.Unlock()
    if _, ok := c.users[username]; ok {
        if c.logger != nil {
            c.logger.Printf("[ERROR] go_chat_i_guess/channel: User tried to connect more than once to a channel.\n\tchannel: \"%s\"\n\tuser: \"%s\"",
                    c.name, username)
        }
        return UserAlreadyConnected
    }

    c.users[username] = u
    c.NewSystemBroadcast(username + " entered " + c.name +"!")

    return nil
}

// ConnectUser add a new user to the channel and blocks until the
// user closes the connection to the server.
//
// The `channel` does properly synchronize this function, so it may be
// called by different goroutines concurrently.
//
// On error, `conn` is left unchanged and must be closed by the caller.
//
// Differently from `ConnectUser`, this function handles messages
// from the remote client in the calling goroutine. This may be
// advantageous if the external server already spawns a new goroutine
// to handle each new connection.
//
// If `conn` is nil, then this function will panic!
func (c *channel) ConnectUserAndWait(username string, conn Conn) error {
    if conn == nil {
        panic("go_chat_i_guess/channel ConnectUserAndWait: nil conn")
    }

    u := newUser(username, c, conn, c.logger, c.debugLog)

    c.lockUsers.Lock()
    if _, ok := c.users[username]; ok {
        c.lockUsers.Unlock()

        if c.logger != nil {
            c.logger.Printf("[ERROR] go_chat_i_guess/channel: User tried to connect more than once to a channel.\n\tchannel: \"%s\"\n\tuser: \"%s\"",
                    c.name, username)
        }
        return UserAlreadyConnected
    }
    c.users[username] = u
    c.lockUsers.Unlock()

    c.NewSystemBroadcast(username + " entered " + c.name +"!")
    u.RunAndWait()

    return nil
}

// Close the channel, remove every user and stop the goroutine.
func (c *channel) Close() error {
    // Atomically check if `c.running` is 1 and set it to 0. If this
    // returns true, the swap happened and thus this is the first time
    // that `c.Close()` was called.
    if atomic.CompareAndSwapUint32(&c.running, 1, 0) {
        if c.debugLog && c.logger != nil {
            c.logger.Printf("[DEBUG] go_chat_i_guess/channel: Closing channel...\n\tchannel: \"%s\"",
                    c.name)
        }
        close(c.stop)

        c.lockUsers.Lock()
        for k := range c.users {
            c.RemoveUserUnsafe(k)
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

    // Remove the user `username` from this channel.
    RemoveUser(username string) error

    // ConnectUser add a new user to the channel.
    //
    // It's entirely up to the caller to initialize the connection used by
    // this user, for example upgrading a HTTP request to a WebSocket
    // connection.
    //
    // The `channel` does properly synchronize this function, so it may be
    // called by different goroutines concurrently.
    //
    // On error, `conn` is left unchanged and must be closed by the caller.
    //
    // If `conn` is nil, then this function will panic!
    ConnectUser(username string, conn Conn) error

    // ConnectUser add a new user to the channel and blocks until the
    // user closes the connection to the server.
    //
    // The `channel` does properly synchronize this function, so it may be
    // called by different goroutines concurrently.
    //
    // On error, `conn` is left unchanged and must be closed by the caller.
    //
    // Differently from `ConnectUser`, this function handles messages
    // from the remote client in the calling goroutine. This may be
    // advantageous if the external server already spawns a new goroutine
    // to handle each new connection.
    //
    // If `conn` is nil, then this function will panic!
    ConnectUserAndWait(username string, conn Conn) error
}

// newChannel create a new ChatChannel named `name`.
//
// An `Encoder` may optionally be supplied on `conf` to process and encode
// messages received by the channel.
//
// `newChannel()` executes a new goroutine to handle messages received by
// the channel. To stop this goroutine and clean up its resources, call
// `c.Close()`.
//
// Regardless, if every user disconnects and the channel is left idle for
// long enough (more specifically, for `defIdleTimeout`), this goroutine
// will automatically stop.
func newChannel(name string, conf ServerConf) ChatChannel {
    c := &channel {
        name: name,
        encoder: conf.Encoder,
        recv: make(chan *message, 8),
        idleTimeout: conf.ChannelIdleTimeout,
        users: make(map[string]*user),
        running: 1,
        idle: time.NewTicker(conf.ChannelIdleTimeout),
        stop: make(chan struct{}),
        logger: conf.Logger,
        debugLog: conf.DebugLog,
    }

    go c.run()

    return c
}
