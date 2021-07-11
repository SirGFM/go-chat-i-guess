package main

import (
    "fmt"
    "io"
    "log"
    "net/http"
    "net/url"
    "path"
)

type server struct {
    // The server's HTTP server
    httpServer *http.Server
}

// ServeHTTP is called by Go's http package whenever a new HTTP request arrives
func (s *server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
    uri := cleanURL(req.URL)
    log.Printf("%s - %s - %s", req.RemoteAddr, req.Method, uri)

    if uri == "chat_page" || uri == "" {
        serveChatPage(w)
    } else {
        httpTextReply(http.StatusNotFound, "404 - Nothing to see here...", w)
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

    go func() {
        log.Printf("Waiting...")
        srv.httpServer.ListenAndServe()
    } ()

    return &srv
}
