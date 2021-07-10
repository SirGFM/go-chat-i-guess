# Go chat, I guess?

A simple websocket-based chat server written for the heck of it.

# Quick guide

To build everything:

```bash
go get github.com/gorilla/websocket
go get github.com/gobwas/ws
go build ./cmd/gobwas-ws-raw-chat
go build ./cmd/gobwas-ws-req-chat
go build ./cmd/gorilla-ws-chat
go build ./cmd/pinger
```

Then, execute one of `gobwas-ws-raw-chat`, `gobwas-ws-req-chat` or `gorilla-ws-chat`. The server will be listening on port 8888 and accept connections on any interface/IP address.

To launch a couple of pinger, run:

```bash
i=0; while [ $i -lt 1000 ]; do ./pinger asd user${i} & i=$((i + 1)); done
```

This will most likely increase CPU usage by quite a lot! Be sure to have `killall` installed, so you may run the following to kill every pinger:

```bash
killall pinger
```
