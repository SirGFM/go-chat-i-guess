package main

import (
    "encoding/binary"
    crand "crypto/rand"
    "fmt"
    "github.com/gobwas/ws"
    "github.com/gobwas/ws/wsutil"
    "log"
    mrand "math/rand"
    "net"
    "net/url"
    "os"
    "os/signal"
    "sync"
    "time"
)

func seedIt() {
    var buf [8]byte

    crand.Read(buf[:])
    s, _ := binary.Varint(buf[:])
    mrand.Seed(s)
}

func main() {
    var buf [1]wsutil.Message

    log.SetFlags(log.Lshortfile | log.Ldate | log.Ltime)
    seedIt()

    if len(os.Args) != 3 {
        log.Fatalf("Usage: %s channel username", os.Args[0])
    }
    channel := os.Args[1]
    username := os.Args[2]

    uri, err := url.ParseRequestURI(fmt.Sprintf("ws://localhost:8888/chat/%s/%s", channel, username))
    if err != nil {
        log.Fatalf("Couldn't parse the URL: %+v", err)
    }

    conn, err := net.Dial("tcp", "localhost:8888")
    if err != nil {
        log.Fatalf("Couldn't connect: %+v", err)
    }

    var m sync.Mutex

    onClose := func() {
        if conn != nil {
            m.Lock()
            err := wsutil.WriteClientMessage(conn, ws.OpClose, nil)
            if err != nil {
                log.Printf("Couldn't send close: %+v", err)
            }
            m.Unlock()
            tmp := conn
            conn = nil
            time.Sleep(time.Millisecond)

            tmp.Close()
        }
    }
    defer onClose()

    _, _, err = ws.DefaultDialer.Upgrade(conn, uri)
    if err != nil {
        log.Fatalf("Failed to upgrade: %+v", err)
    }

    intHndlr := make(chan os.Signal, 1)
    signal.Notify(intHndlr, os.Interrupt)

    go func() {
        <-intHndlr
        log.Printf("Exiting...")
        onClose()
    } ()

    go func() {
        for {
            // Generate a number between 1 and 128 and
            // then convert it to 125ms to 16s
            n := (mrand.Uint32() & 0x7f) + 1
            t := time.Millisecond * time.Duration(n * 125)
            time.Sleep(t)

            s := fmt.Sprintf("%s waited %d to say something", channel, t)
            m.Lock()
            err = wsutil.WriteClientMessage(conn, ws.OpText, []byte(s))
            m.Unlock()
            if err != nil {
                log.Fatalf("Couldn't send message: %+v", err)
                return
            }
        }
    } ()

    log.Printf("Waiting...")
    for conn != nil {
        buf, err := wsutil.ReadServerMessage(conn, buf[:])
        if err != nil {
            log.Printf("Couldn't read: %+v", err)
        }

        for i := range buf {
            data := &(buf[i])
            switch data.OpCode {
            case ws.OpClose:
                log.Fatal("Server closed the connections")
                return
            case ws.OpPing:
                m.Lock()
                err = wsutil.WriteClientMessage(conn, ws.OpPong, data.Payload)
                m.Unlock()
                if err != nil {
                    log.Fatalf("Couldn't pong: %+v", err)
                    return
                }
            }
        }
    }
}
