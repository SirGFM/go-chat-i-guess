package go_chat_i_guess

import (
    "io"
    "time"
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

// A chat channel, to which users may connect to.
type channel struct {
    // name of this channel.
    name string

    // recv messages sent from a remote client.
    recv chan *message

    // log every message received by this channel.
    log []*message

    // idleTimeout after which this channel is automatically closed, if no
    // user connected to it.
    idleTimeout time.Duration

    // Whether the channel is currently running.
    running uint32

    // stop receives a new message when the channel should get closed.
    stop chan bool
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

// newSystemBroadcast queue a new system message (i.e., a message without
// a sender), setting its `Date` to the current time and setting
// `Message` to `msg`.
func (c *channel) newSystemBroadcast(msg string) {
    c.newMessage(msg, "", "")
}

// newSystemBroadcast queue a new system message (i.e., a message without
// a sender) to a specific receiver, setting its `Date` to the current time
// and setting `Message` to `msg`.
func (c *channel) newSystemWhisper(msg, to string) {
    c.newMessage(msg, "", to)
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

        // Also, since this was the first message, simply log it.
        c.log = append(c.log, msg)
        return nil
    case <-timeout.C:
        return IdleChannel
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

        c.log = append(c.log, msg)

        // TODO Broadcast the message to every client.

        // TODO If a client was removed, check whether this was the last
        // client and, in that case, wait for another connection.
    }
}

// Close the channel, remove every client and stop the goroutine.
func (c *channel) Close() error {
    // Atomically check if `c.running` is 1 and set it to 0. If this
    // returns true, the swap happened and thus this is the first time
    // that `c.Close()` was called.
    if atomic.CompareAndSwapUint32(&c.running, 1, 0) {
        c.stop <- true

        // TODO Clean any other resources.
    }

    return nil
}

// The public interface for a chat channel.
type ChatChannel interface {
    io.Closer

    // NewBroadcast queue a new broadcast message from a specific sender,
    // setting its `Date` to the current time and setting the message's
    // `Message` and sender (its `From`) as `msg` and `from`, respectively.
    NewBroadcast(msg, from string)

    // TODO Add a function to add new clients.
}

// newChannel create a new ChatChannel named `name`.
//
// `newChannel()` executes a new goroutine to handle messages received by
// the channel. To stop this goroutine and clean up its resources, call
// `c.Close()`.
//
// Regardless, if every client disconnects and the channel is left idle for
// long enough (more specifically, for `defIdleTimeout`), this goroutine
// will automatically stop.
func newChannel(name string) ChatChannel {
    c := &channel {
        name: name,
        recv: make(chan *message),
        idleTimeout: defIdleTimeout,
        running: 1,
        stop: make(chan bool),
    }

    go c.run()

    return c
}
