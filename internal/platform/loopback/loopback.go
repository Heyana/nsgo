// Package loopback serves the redirects of the auth sessions desktops open in the default browser (driver.ItemAuthSession), on a loopback address as RFC 8252 section 7.3 asks: a listener on 127.0.0.1 at a port the system picks, taking one callback. It reads the browser's GET itself rather than through net/http, which an app links only by asking for it (docs/architecture/layers.md, What an app links).
package loopback

import (
	"bufio"
	"html"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Sessions are a surface's loopback auth sessions. Its methods run on any goroutine.
type Sessions struct {
	mu sync.Mutex
	m  map[uint32]*session
}

type session struct {
	listener net.Listener
	redirect string
}

// Has reports whether session id is one of s's.
func (s *Sessions) Has(id uint32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.m[id]
	return ok
}

// Handle carries out r for session id: a start listens and reports the redirect, an open has open show the URL in the default browser and answer, on any goroutine, whether it did, and a cancel stops listening. page is the message the browser's tab shows once the callback arrives; dispatch hears the session's events.
func (s *Sessions) Handle(id uint32, r driver.AuthRequest, open func(url string, answer func(opened bool)), page string, dispatch func(driver.Event)) {
	switch r.Command {
	case driver.AuthStart:
		s.start(id, r.Path, page, dispatch)
	case driver.AuthOpen:
		if s.Has(id) {
			open(r.URL, func(opened bool) {
				if !opened {
					s.end(id, driver.AuthEnd(id, driver.AuthFailed, "no browser opened the authorization URL"), dispatch)
				}
			})
		}
	case driver.AuthCancel:
		s.end(id, driver.Event{}, nil)
	}
}

// Close stops every session's listener, as the surface closes.
func (s *Sessions) Close() {
	s.mu.Lock()
	ids := make([]uint32, 0, len(s.m))
	for id := range s.m {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		s.end(id, driver.Event{}, nil)
	}
}

func (s *Sessions) start(id uint32, path, page string, dispatch func(driver.Event)) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		dispatch(driver.AuthEnd(id, driver.AuthFailed, "listening on loopback: "+err.Error()))
		return
	}
	base := &url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(l.Addr().(*net.TCPAddr).Port), Path: "/"}
	// Path is a decoded path, as serve reads it. Escape it and resolve dot
	// segments before reporting the redirect, so browser navigation and the
	// listener agree on the callback's path.
	redirect := base.ResolveReference(&url.URL{Path: path})
	path = redirect.Path
	ss := &session{listener: l, redirect: redirect.String()}
	body := `<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>` + html.EscapeString(page) +
		`</title><body style="font:16px system-ui,sans-serif;margin:3em;text-align:center">` + html.EscapeString(page) + `</body>`
	s.mu.Lock()
	if s.m == nil {
		s.m = map[uint32]*session{}
	}
	s.m[id] = ss
	s.mu.Unlock()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return // closed
			}
			// A browser may open connections it sends nothing on, so each is read apart.
			go func() {
				query, ok := serve(conn, path, body)
				if !ok {
					return
				}
				callback := ss.redirect
				if query != "" {
					callback += "?" + query
				}
				s.end(id, driver.Event{Kind: driver.EventAuthSession, Count: id, Data: &driver.AuthEvent{Phase: driver.AuthEnded, Callback: callback}}, dispatch)
			}()
		}
	}()
	dispatch(driver.Event{Kind: driver.EventAuthSession, Count: id, Data: &driver.AuthEvent{Phase: driver.AuthRedirect, Redirect: ss.redirect}})
}

// serve answers the one request conn carries, and closes it: a GET of path gets body and reports its query, anything else a 404.
func serve(conn net.Conn, path, body string) (query string, ok bool) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	r := bufio.NewReaderSize(conn, 8<<10)
	line, err := r.ReadSlice('\n')
	if err != nil {
		return "", false
	}
	// Headers end with an empty line; what they say does not matter.
	for n := 0; ; n++ {
		h, err := r.ReadSlice('\n')
		if err != nil || n > 100 {
			return "", false
		}
		if len(strings.TrimRight(string(h), "\r\n")) == 0 {
			break
		}
	}
	method, rest, _ := strings.Cut(strings.TrimRight(string(line), "\r\n"), " ")
	target, _, _ := strings.Cut(rest, " ")
	u, err := url.ParseRequestURI(target)
	ok = err == nil && method == "GET" && u.Path == path
	status, content := "404 Not Found", "Not found"
	if ok {
		status, content = "200 OK", body
	}
	_, _ = conn.Write([]byte("HTTP/1.1 " + status + "\r\nContent-Type: text/html; charset=utf-8\r\nCache-Control: no-store\r\nReferrer-Policy: no-referrer\r\nConnection: close\r\nContent-Length: " +
		strconv.Itoa(len(content)) + "\r\n\r\n" + content))
	if !ok {
		return "", false
	}
	return u.RawQuery, true
}

// end stops session id's listener and dispatches ev, once, unless the session was already gone; a nil dispatch reports nothing.
func (s *Sessions) end(id uint32, ev driver.Event, dispatch func(driver.Event)) {
	s.mu.Lock()
	ss := s.m[id]
	delete(s.m, id)
	s.mu.Unlock()
	if ss == nil {
		return
	}
	_ = ss.listener.Close()
	if dispatch != nil {
		dispatch(ev)
	}
}
