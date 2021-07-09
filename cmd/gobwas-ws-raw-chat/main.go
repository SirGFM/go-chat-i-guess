package main

import (
    "fmt"
    "github.com/gobwas/ws"
    "github.com/gobwas/ws/wsutil"
    "log"
    "net"
    "net/http"
    "net/url"
    "os"
    "os/signal"
    "path"
    "strings"
    "time"
)

type message struct {
    t time.Time
    msg string
    from string
}

func newMessage(msg string, from string) message {
    return message {
        t: time.Now(),
        msg: msg,
        from: from,
    }
}

type participant struct {
    conn net.Conn
    name string
    last time.Time
    send chan message
}

func (p *participant) run() {
    var buf [1]wsutil.Message
    defer p.conn.Close()

    for {
        buf, err := wsutil.ReadClientMessage(p.conn, buf[:])
        if err != nil {
            log.Printf("Couldn't read: %+v", err)
            return
        }

        for i := range buf {
            data := &(buf[i])
            switch data.OpCode {
            case ws.OpClose:
                log.Printf("Server closed the connection to %s", p.name)
                return
            case ws.OpPing:
                // TODO Lock before ponging
                err = wsutil.WriteServerMessage(p.conn, ws.OpPong, data.Payload)
                if err != nil {
                    log.Printf("Couldn't pong: %+v", err)
                    return
                }
            case ws.OpPong:
                // Do nothing
                continue
            case ws.OpText:
                // Queue the message
                p.send <- newMessage(string(data.Payload), p.name)
            default:
                log.Printf("Ignoring message of type: %+v", data.OpCode)
            }
        }
    }
}

type room struct {
    log []message
    users []participant
    newMsg chan message
    name string
}

func (r *room) run() {
    for {
        msg := <-r.newMsg

        txt := []byte(fmt.Sprintf("%+v - %s: %s", msg.t, msg.from, msg.msg))
        log.Printf("@%s - %s", r.name, string(txt))
        for i := range r.users {
            p := &(r.users[i])
            if p.name == msg.from {
                continue
            }

            // TODO Lock before sending the message
            err := wsutil.WriteServerMessage(p.conn, ws.OpText, txt)
            if err != nil {
                log.Printf("Couldn't send: %+v", err)
                return
            }
            p.last = msg.t
        }

        r.log = append(r.log, msg)
    }
}

type runningServer struct {
    conn net.Listener
    rooms map[string]*room
}

func (s *runningServer) handle() {
    var channel string
    var username string

    wsUpgrader := ws.Upgrader {
        OnRequest: func (uri []byte) error {
            // Normalize and strip the URL from its leading prefix (and slash)
            resUrl := path.Clean(string(uri))
            if len(resUrl) > 0 && resUrl[0] == '/' {
                // NOTE: The first character must not be a '/' because of the split
                resUrl = resUrl[1:]
            } else if len(resUrl) == 1 && resUrl[0] == '.' {
                // Clean converts an empty path into a single ""
                resUrl = ""
            }

            // As part of the normalization, unescape each component individually
            var urlPath []string
            for _, p := range strings.Split(resUrl, "/") {
                cleanPath, err := url.PathUnescape(p)
                if err != nil {
                    log.Printf("err: %+v", err)
                    return ws.RejectConnectionError(
                        ws.RejectionStatus(http.StatusNotFound),
                        ws.RejectionReason(fmt.Sprintf("handshake error: invalid URI '%s'", string(uri))),
                    )
                }
                urlPath = append(urlPath, cleanPath)
            }

            if len(urlPath) >= 2 {
                username = urlPath[len(urlPath)-1]
                channel = strings.Join(urlPath[:len(urlPath)-1], "|")

                return nil
            } else {
                return ws.RejectConnectionError(
                    ws.RejectionStatus(http.StatusNotFound),
                    ws.RejectionReason("handshake error: invalid need at least two parts"),
                )
            }
        },
    }

    for {
        conn, err := s.conn.Accept()
        if err != nil {
            log.Fatalf("Failed to accept: %+v", err)
        }

        // Try to update the connection to a websocket connection
        _, err = wsUpgrader.Upgrade(conn)
        if err != nil {
            log.Printf("Not a websocket! %+v", err)
            conn.Close()
            continue
        }

        // Add the participant to the room (creating it as necessary)
        chatRoom, ok := s.rooms[channel]
        if !ok {
            chatRoom = &room {
                newMsg: make(chan message, 1),
                name: channel,
            }
            s.rooms[channel] = chatRoom
            go chatRoom.run()
        }

        p := participant {
            conn: conn,
            name: username,
            send: chatRoom.newMsg,
        }
        chatRoom.users = append(chatRoom.users, p)
        go p.run()

        msg := fmt.Sprintf("%s joined %s", username, channel)
        log.Printf(msg)
        chatRoom.newMsg <- newMessage(msg, "")
    }
}

// Halts the server, if still running
func (s *runningServer) Close() {
    if s.conn != nil {
        s.conn.Close()
        s.conn = nil
    }
}

func main() {
    log.SetFlags(log.Lshortfile | log.Ldate | log.Ltime)

    ln, err := net.Listen("tcp", "0.0.0.0:8888")
    if err != nil {
        log.Fatalf("Failed to listen: %+v", err)
    }

    srv := runningServer {
        conn: ln,
        rooms: make(map[string]*room),
    }

    intHndlr := make(chan os.Signal, 1)
    signal.Notify(intHndlr, os.Interrupt)

    go func() {
        <-intHndlr
        log.Printf("Exiting...")
        srv.Close()
    } ()

    log.Printf("Waiting...")
    srv.handle()
}
