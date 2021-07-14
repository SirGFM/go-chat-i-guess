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
}

// GetDefaultServerConf retrieve a fully initialized `ServerConf`, with all
// fields set to some default, and non-zero, value.
func GetDefaultServerConf() ServerConf {
    return ServerConf {
        ReadBuf: 1024,
        WriteBuf: 1024,
        TokenDeadline: defTokenDeadline,
        TokenCleanupDelay: defTokenCleanupDelay,
    }
}

// The chat server.
type server struct {
    // The server configurations.
    conf ServerConf

    // Every currently active token. The token itself is used as the map's key.
    tokens map[string]*accessToken

    // Synchronizes access to tokens.
    tokenMutex sync.Mutex

    // Whether the chat server is currently running.
    running bool
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
}

// Clean up every resource used by the chat server.
func (s *server) Close() error {
    s.running = false

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
        }
    }

    token.Stop()
}

// NewServerConf create a new chat server, as specified by `conf`.
//
// When a new chat server starts, a clean up goroutine is spawned to check
// and release expired resources periodically. This goroutine is stopped,
// and every resource is released, when the ChatServer gets `Close()`d.
func NewServerConf(conf ServerConf) ChatServer {
    s := &server {
        conf: conf,
        tokens: make(map[string]*accessToken),
        running: true,
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
    conf := ServerConf {
        ReadBuf: readBuf,
        WriteBuf: writeBuf,
        TokenDeadline: tokenDeadline,
        TokenCleanupDelay: tokenCleanupDelay,
    }

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
