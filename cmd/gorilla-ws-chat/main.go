package main

import (
    "fmt"
    gows "github.com/gorilla/websocket"
    "log"
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
    conn *gows.Conn
    name string
    last time.Time
    send chan message
}

func (p *participant) onClose() {
    closeTxt := fmt.Sprintf("%s is exiting...", p.name)
    log.Printf("Closing connection with %s", p.name)
    p.send <- newMessage(closeTxt, "")
}

func (p *participant) run() {
    defer p.conn.Close()

    for {
        typ, txt, err := p.conn.ReadMessage()
        if err != nil {
            log.Printf("err: %+v", err)
            p.onClose()
            return
        }

        switch typ {
        case gows.CloseMessage:
            p.onClose()
            return
        case gows.TextMessage:
            break
        default:
            continue
        }

        p.send <- newMessage(string(txt), p.name)
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

            err := p.conn.WriteMessage(gows.TextMessage, txt)
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
    upgrader gows.Upgrader
}

// ServeHTTP is called by Go's http package whenever a new HTTP request arrives
func (s *runningServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
    // Normalize and strip the URL from its leading prefix (and slash)
    resUrl := path.Clean(req.URL.EscapedPath())
    if len(resUrl) > 0 && resUrl[0] == '/' {
        // NOTE: The first character must not be a '/' because of the split
        resUrl = resUrl[1:]
    } else if len(resUrl) == 1 && resUrl[0] == '.' {
        // Clean converts an empty path into a single "."
        resUrl = ""
    }

    // As part of the normalization, unescape each component individually
    var urlPath []string
    for _, p := range strings.Split(resUrl, "/") {
        cleanPath, err := url.PathUnescape(p)
        if err != nil {
            log.Printf("err: %+v", err)
            return
        }
        urlPath = append(urlPath, cleanPath)
    }

    if len(urlPath) >= 2 {
        user := urlPath[len(urlPath)-1]
        roomName := strings.Join(urlPath[:len(urlPath)-1], "|")

        s.roomsLock.Lock()
        chatRoom, ok := s.rooms[roomName]
        s.roomsLock.Unlock()
        if !ok {
            chatRoom = &room {
                newMsg: make(chan message, 1),
                name: roomName,
            }
            s.roomsLock.Lock()
            s.rooms[roomName] = chatRoom
            s.roomsLock.Unlock()
            go chatRoom.run()
        }

        conn, err := s.upgrader.Upgrade(w, req, nil)
        if err != nil {
            log.Printf("err: %+v", err)
            return
        }

        p := participant {
            conn: conn,
            name: user,
            send: chatRoom.newMsg,
        }
        chatRoom.lock.Lock()
        chatRoom.users = append(chatRoom.users, p)
        chatRoom.lock.Unlock()
        go p.run()

        s := fmt.Sprintf("%s joined %s", user, roomName)
        log.Printf(s)
        p.send <- newMessage(s, "")
    }
}

// Halts the `http.Server`, if still running
func (s *runningServer) Close() {
    if s.httpServer != nil {
        s.httpServer.Close()
        s.httpServer = nil
    }
}

func yes(r *http.Request) bool {
    return true
}

func main() {
    var srv runningServer

    log.SetFlags(log.Lshortfile | log.Ldate | log.Ltime)

    srv.httpServer = &http.Server {
        Addr: "0.0.0.0:8888",
        Handler: &srv,
    }
    srv.rooms = make(map[string]*room)
    srv.upgrader = gows.Upgrader {
        ReadBufferSize:  1024,
        WriteBufferSize: 1024,
        CheckOrigin: yes,
    }

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
