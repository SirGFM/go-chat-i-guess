# Go chat, I guess?

A generic, connection-agnostic chat server.

The server support multiple channels/chat rooms, to which users may
connect. Both the channel and users must be uniquely identified by their
name, although the calling application is responsible by authenticating if
those are valid values.

# Prerequisites

* Go 1.15 (this package hasn't been made in a module, so more recent versions probably won't work)

# Documentation

The documentation may be generated using `godoc`:

```bash
go get golang.org/x/tools/cmd/godoc
godoc .
```

# Example chat

To build a simple WebSocket-based example chat:

```bash
go get github.com/gorilla/websocket
go build ./cmd/chat-server
./chat-server
```

Then, simply open http://localhost:8888 in a browser.
