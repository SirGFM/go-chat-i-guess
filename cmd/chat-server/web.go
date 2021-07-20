package main

import (
    "fmt"
    gochat "github.com/SirGFM/go-chat-i-guess"
    "io"
    "log"
    "net/http"
    "net/url"
    "path"
    "strings"
)

type server struct {
    // The server's HTTP server
    httpServer *http.Server
    // The chat server
    chat gochat.ChatServer
}


// ServeHTTP is called by Go's http package whenever a new HTTP request arrives
func (s *server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
    uri := cleanURL(req.URL)
    log.Printf("%s - %s - %s", req.RemoteAddr, req.Method, uri)

    if uri == "chat_page" || uri == "" {
        serveChatPage(w)
    } else {
        parts := strings.Split(uri, "/")
        if len(parts) == 2 && parts[0] == "new_channel" {
            err := s.chat.CreateChannel(parts[1])
            if err == nil {
                w.Header().Set("Content-Type", "text/plain")
                w.WriteHeader(http.StatusNoContent)
                log.Printf("%s - %s - %s [OK]", req.RemoteAddr, req.Method, uri)
            } else {
                httpTextReply(http.StatusInternalServerError, fmt.Sprintf("Couldn't create the channel: %+v", err), w)
                log.Printf("%s - %s - %s [500]", req.RemoteAddr, req.Method, uri)
            }
        } else if len(parts) == 3 && parts[0] == "new_token" {
            channel := parts[1]
            username := parts[2]

            tk, err := s.chat.RequestToken(username, channel)
            if err == nil {
                httpTextReply(http.StatusOK, tk, w)
                log.Printf("%s - %s - %s [OK]", req.RemoteAddr, req.Method, uri)
            } else {
                httpTextReply(http.StatusInternalServerError, fmt.Sprintf("Couldn't create the token: %+v", err), w)
                log.Printf("%s - %s - %s [500]", req.RemoteAddr, req.Method, uri)
            }
        } else if len(parts) == 1 && parts[0] == "chat" {
            // '/chat' expects the token to be sent in a 'X-ChatToken' cookie
            tk := ""
            for _, c := range req.Cookies() {
                if c.Name == "X-ChatToken" {
                    tk = c.Value
                    break
                }
            }
            if len(tk) == 0 {
                httpTextReply(http.StatusInternalServerError, "Couldn't find the supplied token", w)
                log.Printf("%s - %s - %s [500]", req.RemoteAddr, req.Method, uri)
                return
            }

            // Upgrade to websocket
            conn, err := newConn(w, req)
            if err != nil {
                httpTextReply(http.StatusInternalServerError, fmt.Sprintf("Couldn't upgrade the connection: %+v", err), w)
                log.Printf("%s - %s - %s [500]", req.RemoteAddr, req.Method, uri)
                return
            }

            // On success, the upgraded request will be handled by the chat server
            err = s.chat.Connect(tk, conn)
            if err != nil {
                // Can't do HTTP anymore as the connection was upgraded to a websocket
                conn.Close()
                log.Printf("%s - %s - %s - Couldn't connect to the chat room", req.RemoteAddr, req.Method, uri)
            }
        } else {
            httpTextReply(http.StatusNotFound, "404 - Nothing to see here...", w)
            log.Printf("%s - %s - %s [404]", req.RemoteAddr, req.Method, uri)
        }
    }
}

// cleanURL so everything is properly escaped/encoded and so it may be split into each of its components.
//
// Use `url.Unescape` to retrieve the unescaped path, if so desired.
func cleanURL(uri *url.URL) string {
    // Normalize and strip the URL from its leading prefix (and slash)
    resUrl := path.Clean(uri.EscapedPath())
    if len(resUrl) > 0 && resUrl[0] == '/' {
        resUrl = resUrl[1:]
    } else if len(resUrl) == 1 && resUrl[0] == '.' {
        // Clean converts an empty path into a single "."
        resUrl = ""
    }

    return resUrl
}

// httpTextReply send a simple HTTP response as a plain text.
func httpTextReply(status int, msg string, w http.ResponseWriter) {
    w.Header().Set("Content-Type", "text/plain")
    w.WriteHeader(status)

    for data := []byte(msg); len(data) > 0; {
        n, err := w.Write(data)
        if err != nil {
            log.Printf("Failed to send %d: %+v", err, status)
            return
        }
        data = data[n:]
    }
}

// Close the running web server and clean up resourcers
func (s *server) Close() error {
    if s.httpServer != nil {
        s.httpServer.Close()
        s.httpServer = nil
    }

    return nil
}

// runWeb server into a goroutine
func runWeb(args Args) io.Closer {
    var srv server

    srv.httpServer = &http.Server {
        Addr: fmt.Sprintf("%s:%d", args.IP, args.Port),
        Handler: &srv,
    }
    srv.chat = gochat.NewServerConf(gochat.GetDefaultServerConf())
    setUpgrader(args)

    go func() {
        log.Printf("Waiting...")
        srv.httpServer.ListenAndServe()
    } ()

    return &srv
}
