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
    "sync"
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
    lock sync.Mutex
}

func (r *room) run() {
    for {
        msg := <-r.newMsg

        txt := []byte(fmt.Sprintf("%+v - %s: %s", msg.t, msg.from, msg.msg))
        log.Printf("@%s - %s", r.name, string(txt))

        // Manually iterate over the list, since it may change live
        r.lock.Lock()
        count := len(r.users)
        r.lock.Unlock()
        for i := 0; i < count; i++ {
            r.lock.Lock()
            p := &(r.users[i])
            r.lock.Unlock()
            if p.name == msg.from {
                continue
            }

            // TODO Lock before sending the message
            err := wsutil.WriteServerMessage(p.conn, ws.OpText, txt)
            if err != nil {
                log.Printf("err: %+v", err)

                r.lock.Lock()
                copy(r.users[i+1:], r.users[i:])
                r.users = r.users[:len(r.users)-1]
                r.lock.Unlock()

                // Decrease both the index and the count because of the 'i++'
                i--
                count--
                continue
            }
            p.last = msg.t
        }

        r.log = append(r.log, msg)
    }
}

type runningServer struct {
    httpServer *http.Server
    rooms map[string]*room
    roomsLock sync.Mutex
}

func (s *runningServer) connChat(w http.ResponseWriter, req *http.Request, channel string, username string) {
    // Try to update the connection to a websocket connection
    conn, _, _, err := ws.UpgradeHTTP(req, w)
    if err != nil {
        log.Printf("Failed to upgrade to websocket: %+v", err)
    }

    // Add the participant to the room (creating it as necessary)
    s.roomsLock.Lock()
    chatRoom, ok := s.rooms[channel]
    s.roomsLock.Unlock()
    if !ok {
        chatRoom = &room {
            newMsg: make(chan message, 1),
            name: channel,
        }
        s.roomsLock.Lock()
        s.rooms[channel] = chatRoom
        s.roomsLock.Unlock()
        go chatRoom.run()
    }

    p := participant {
        conn: conn,
        name: username,
        send: chatRoom.newMsg,
    }
    chatRoom.lock.Lock()
    chatRoom.users = append(chatRoom.users, p)
    chatRoom.lock.Unlock()
    go p.run()

    msg := fmt.Sprintf("%s joined %s", username, channel)
    log.Printf(msg)
    chatRoom.newMsg <- newMessage(msg, "")
}

func (s *runningServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
    var channel string
    var username string

    // Normalize and strip the URL from its leading prefix (and slash)
    resUrl := path.Clean(req.URL.EscapedPath())
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
            log.Printf("Failed to normalize the URL: %+v", err)
            return
        }
        urlPath = append(urlPath, cleanPath)
    }

    if len(urlPath) == 3 && urlPath[0] == "chat" {
        channel = urlPath[1]
        username = urlPath[2]

        s.connChat(w, req, channel, username)
    } else {
        log.Printf("Regular URL: %+v", urlPath)
    }
}

// Halts the server, if still running
func (s *runningServer) Close() {
    if s.httpServer != nil {
        s.httpServer.Close()
        s.httpServer = nil
    }
}

func main() {
    log.SetFlags(log.Lshortfile | log.Ldate | log.Ltime)

    var srv runningServer
    srv.httpServer = &http.Server {
        Addr: "0.0.0.0:8888",
      Handler: &srv,
    }
    srv.rooms = make(map[string]*room)

    intHndlr := make(chan os.Signal, 1)
    signal.Notify(intHndlr, os.Interrupt)

    go func() {
        log.Printf("Waiting...")
        srv.httpServer.ListenAndServe()
    } ()

    <-intHndlr
    log.Printf("Exiting...")
    srv.Close()
}
