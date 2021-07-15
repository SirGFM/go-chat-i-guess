package go_chat_i_guess

import (
    crand "crypto/rand"
    "encoding/hex"
    "io"
    "time"
    "sync"
)

// For how long a given token should exist before being used or expiring.
const defTokenDeadline = time.Second * 30

// Delay between executions of the token cleanup routine.
const defTokenCleanupDelay = time.Minute * 5

// Delay between executions of the channel cleanup routine.
const defChannelCleanupDelay = time.Minute * 30

// Ephemeral access token received from an authenticated.
type accessToken struct {
    // The username for whom the token was generated.
    username string

    // The channel that this token gives access to.
    channel string

    // Expiration time for this token.
    deadline time.Time
}

// ServerConf define various parameters that may be used to configure
// the server.
type ServerConf struct {
    // Size for the read buffer on new connections.
    ReadBuf int

    // Size for the write buffer on new connections.
    WriteBuf int

    // For how long a given token should exist before being used or expiring.
    TokenDeadline time.Duration

    // Delay between executions of the token cleanup routine.
    TokenCleanupDelay time.Duration

    // Delay between executions of the channel cleanup routine.
    ChannelCleanupDelay time.Duration
}

// GetDefaultServerConf retrieve a fully initialized `ServerConf`, with all
// fields set to some default, and non-zero, value.
func GetDefaultServerConf() ServerConf {
    return ServerConf {
        ReadBuf: 1024,
        WriteBuf: 1024,
        TokenDeadline: defTokenDeadline,
        TokenCleanupDelay: defTokenCleanupDelay,
        ChannelCleanupDelay: defChannelCleanupDelay,
    }
}

// The chat server.
type server struct {
    // The server configurations.
    conf ServerConf

    // Collection of channels currently active in this server.
    channels map[string]ChatChannel

    // Synchronizes access to `channels`.
    chanMutex sync.Mutex

    // Every currently active token. The token itself is used as the map's key.
    tokens map[string]*accessToken

    // Synchronizes access to tokens.
    tokenMutex sync.Mutex

    // Whether the chat server is currently running.
    running bool

    // stop receives a new message when the server should get closed.
    stop chan bool
}

// The public interfacer of the chat server.
type ChatServer interface {
    io.Closer

    // GetConf retrieve a copy of the server's configuration. As such,
    // changing it won't cause any change to the configurations of the
    // running server.
    GetConf() ServerConf

    // RequestToken generate a token temporarily associating the user identified
    // by `username` may connect to a `channel`.
    //
    // This token should be requested from an authenticated and secure channel.
    // Then, the returned token may be sent in a 'Connect()' to identify the
    // user and the desired channel.
    //
    // RequestToken should only fail if it somehow fails to generate a token.
    RequestToken(username, channel string) (string, error)

    // CreateChannel create and start the channel with the given `name`.
    //
    // Channels are uniquely identified by their names. Also, the chat
    // server automatically removes a closed channel, regardless whether
    // it was manually closed or whether it timed out.
    CreateChannel(name string) error

    // GetChannel retrieve the channel named `name`.
    GetChannel(name string) (ChatChannel, error)

    //Connect(w http.ResponseWriter, req *http.Request, channel string
}

// Clean up every resource used by the chat server.
func (s *server) Close() error {
    if s.running {
        s.running = false
        s.stop <- true
    }

    return nil
}

// GetConf retrieve a copy of the server's configuration. As such,
// changing it won't cause any change to the configurations of the
// running server.
func (s *server) GetConf() ServerConf {
    return s.conf
}

// RequestToken generate a token temporarily associating the user identified
// by `username` may connect to a `channel`.
//
// See `ChatServer.RequestToken` for a more complete description.
//
// The generated token is generated from a cryptographically secure source and
// encoded as a hexadecimal string.
func (s *server) RequestToken(username, channel string) (string, error) {
    var randToken [32]byte

    _, err := crand.Read(randToken[:])
    if err != nil {
        return "", err
    }

    token := hex.EncodeToString(randToken[:])
    value := &accessToken {
        username: username,
        channel: channel,
        deadline: time.Now().Add(s.conf.TokenDeadline),
    }

    s.tokenMutex.Lock()
    s.tokens[token] = value
    s.tokenMutex.Unlock()

    return token, nil
}

// CreateChannel create and start the channel with the given `name`.
//
// This shouldn't ever fail, unless there's already a channel with the
// requested name.
//
// See `ChatServer.CreateChannel` for a more complete description.
func (s *server) CreateChannel(name string) error {
    s.chanMutex.Lock()
    defer s.chanMutex.Unlock()

    if _, ok := s.channels[name]; ok {
        return DuplicatedChannel
    }

    s.channels[name] = newChannel(name)
    return nil
}

// GetChannel retrieve the channel named `name`.
func (s *server) GetChannel(name string) (ChatChannel, error) {
    s.chanMutex.Lock()
    defer s.chanMutex.Unlock()

    if c, ok := s.channels[name]; ok {
        return c, nil
    } else {
        return nil, InvalidChannel
    }
}

// getToken consume the given `token`, removing it from the server, and return
// the associated `username` and `channel`.
func (s *server) getToken(token string) (string, string, error) {
    s.tokenMutex.Lock()
    val, ok := s.tokens[token]
    if ok {
        delete(s.tokens, token)
    }
    s.tokenMutex.Unlock()

    if ok {
        return val.username, val.channel, nil
    } else {
        return "", "", InvalidToken
    }
}

// cleanup verify, periodically, whether any object should be removed.
func (s *server) cleanup() {
    token := time.NewTicker(s.conf.TokenCleanupDelay)
    channel := time.NewTicker(s.conf.ChannelCleanupDelay)

    for s.running {
        select {
        case <-token.C:
            // Clean up connection tokens
            s.tokenMutex.Lock()
            now := time.Now()
            for key, val := range s.tokens {
                if now.After(val.deadline) {
                    delete(s.tokens, key)
                }
            }
            s.tokenMutex.Unlock()
        case <-channel.C:
            // Clean up channels
            s.chanMutex.Lock()
            for key, val := range s.channels {
                if val.IsClosed() {
                    delete(s.channels, key)
                }
            }
            s.chanMutex.Unlock()
        case <-s.stop:
            // Do nothing and let cleanup exit
        }
    }

    token.Stop()
    channel.Stop()
}

// NewServerConf create a new chat server, as specified by `conf`.
//
// When a new chat server starts, a clean up goroutine is spawned to check
// and release expired resources periodically. This goroutine is stopped,
// and every resource is released, when the ChatServer gets `Close()`d.
func NewServerConf(conf ServerConf) ChatServer {
    s := &server {
        conf: conf,
        channels: make(map[string]ChatChannel),
        tokens: make(map[string]*accessToken),
        running: true,
        stop: make(chan bool, 1),
    }

    // Start the clean up goroutine for expired objects
    go s.cleanup()

    return s
}

// NewServerWithTimeout create a new chat server with the requested size for the
// `readBuf` and for the `writeBuf`. Additionally, the access `tokenDeadline`
// and `tokenCleanupDelay` may be configured.
//
// See `NewServerConf()` for more details.
func NewServerWithTimeout(readBuf, writeBuf int,
        tokenDeadline, tokenCleanupDelay time.Duration) ChatServer {
    conf := GetDefaultServerConf()
    conf.ReadBuf = readBuf
    conf.WriteBuf = writeBuf
    conf.TokenDeadline = tokenDeadline
    conf.TokenCleanupDelay = tokenCleanupDelay

    return NewServerConf(conf)
}

// NewServer create a new chat server with the requested size for the `readBuf`
// and for the `writeBuf`.
//
// See `NewServerConf()` for more details.
func NewServer(readBuf, writeBuf int) ChatServer {
    return NewServerWithTimeout(readBuf, writeBuf, defTokenDeadline,
            defTokenCleanupDelay)
}