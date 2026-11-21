package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/nsgo-dev/nsgo/internal/automation"
)

// nsgo dev -automation: the dev server relays the requests of nsgo mcp, or any tool holding the session file's token, to the running app, whose agent answers them (package nsgotest), and answers status, logs and reload itself. The requests and answers travel over the channel the app keeps with the server for DevTools (devtools.go), so the app never listens:
//
//	POST /_nsgo/automation       a tool's request; loopback only, no Origin, Authorization: Bearer <session token>
//	GET  /_nsgo/events?devtools  with the run's token, the app's DevTools stream, which also carries the requests as automation events; one app at a time
//	POST /_nsgo/devtools/out     with the run's token, the app's DevTools messages, whose automation messages are the answers and reach no DevTools page
//
// The run's token, which every dev server endpoint but this one takes (devServer.handle), reaches the app through NSGO_DEV_TOKEN, which it takes out of its environment; the tools' token is only in the session file, which only its user reads.

// automationHub is nsgo dev -automation's relay between tools and the app.
type automationHub struct {
	srv         *devServer
	bridgeToken string
	target      string
	// session is the session file's path.
	session string

	// calls lets one request reach the app at a time, as the agent answers them.
	calls sync.Mutex

	mu sync.Mutex
	// app takes the connected app's requests, nil without one (connect); gone is closed as it goes, and changed as an app connects or goes.
	app     chan []byte
	gone    chan struct{}
	changed chan struct{}
	// run counts the apps that connected.
	run      int
	next     uint64
	pending  map[uint64]chan automation.Answer
	building bool
	// logs are the app's latest lines, logLimit at most.
	logs []string
}

// The hub's limits: how long a request waits for an app to connect, for its answer, and how many of the app's lines it keeps.
const (
	appWait   = 30 * time.Second
	answerFor = 2 * time.Minute
	logLimit  = 2000
)

// newAutomationHub starts relaying on srv's server for target.
func newAutomationHub(srv *devServer, target string) (*automationHub, error) {
	bridge, err := newToken()
	if err != nil {
		return nil, err
	}
	h := &automationHub{srv: srv, bridgeToken: bridge, target: target, changed: make(chan struct{}), pending: map[uint64]chan automation.Answer{}}
	srv.mux.HandleFunc("/_nsgo/automation", h.bridge)
	srv.mu.Lock()
	srv.hub = h
	srv.mu.Unlock()
	return h, nil
}

// notify wakes what waits for an app. The caller holds h.mu.
func (h *automationHub) notify() {
	close(h.changed)
	h.changed = make(chan struct{})
}

// setBuilding records whether the app builds, for status.
func (h *automationHub) setBuilding(on bool) {
	h.mu.Lock()
	h.building = on
	h.mu.Unlock()
}

// log keeps one of the app's lines.
func (h *automationHub) log(line string) {
	h.mu.Lock()
	h.logs = append(h.logs, line)
	if len(h.logs) > logLimit {
		h.logs = append(h.logs[:0], h.logs[len(h.logs)-logLimit:]...)
	}
	h.mu.Unlock()
}

// bridge answers a tool's request: from this machine, outside any web page, with the session's token.
func (h *automationHub) bridge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST a request", http.StatusMethodNotAllowed)
		return
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() || r.Header.Get("Origin") != "" {
		http.Error(w, "the automation answers this machine's tools only", http.StatusForbidden)
		return
	}
	if !tokenIs(r.Header.Get("Authorization"), "Bearer "+h.bridgeToken) {
		http.Error(w, "the session file's token is needed", http.StatusUnauthorized)
		return
	}
	var req automation.Request
	var answer automation.Answer
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		answer = automation.Reply(0, nil, automation.Errorf(automation.InvalidRequest, "%v", err))
	} else {
		answer = h.answer(req)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(answer)
}

// answer answers req: the tool's own methods here, the app's through the app.
func (h *automationHub) answer(req automation.Request) automation.Answer {
	switch req.Method {
	case automation.Status:
		return automation.Reply(req.ID, h.status(), nil)
	case automation.ListAppTools:
		if !h.calls.TryLock() {
			return automation.Reply(req.ID, nil, automation.Errorf(automation.Timeout, "the app is answering another request"))
		}
		defer h.calls.Unlock()
		a := h.relay(req, 0, 2*automation.AppToolListTimeout)
		if a.Error != nil && a.Error.Code == automation.NoApp {
			return automation.Reply(req.ID, []automation.AppTool{}, nil)
		}
		return a
	case automation.Logs:
		var p automation.LogsParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				return automation.Reply(req.ID, nil, automation.Errorf(automation.InvalidRequest, "%v", err))
			}
		}
		n := p.Lines
		if n <= 0 {
			n = 200
		}
		h.mu.Lock()
		lines := append([]string{}, h.logs[max(len(h.logs)-n, 0):]...)
		h.mu.Unlock()
		return automation.Reply(req.ID, automation.LogsResult{Lines: lines}, nil)
	case automation.Reload:
		h.srv.reload()
		return automation.Reply(req.ID, h.status(), nil)
	}
	return h.call(req, appWait)
}

// status is how the build and the app stand.
func (h *automationHub) status() automation.StatusResult {
	h.srv.mu.Lock()
	last := h.srv.last
	h.srv.mu.Unlock()
	var failure string
	if rest, ok := strings.CutPrefix(last, "error "); ok {
		failure, _ = strconv.Unquote(rest)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return automation.StatusResult{Target: h.target, Building: h.building, BuildError: failure, App: h.app != nil, Run: h.run}
}

// call relays req to the app, waiting up to wait for one to connect, and returns its answer.
func (h *automationHub) call(req automation.Request, wait time.Duration) automation.Answer {
	h.calls.Lock()
	defer h.calls.Unlock()
	return h.relay(req, wait, answerFor)
}

// relay sends a request while the caller holds h.calls.
func (h *automationHub) relay(req automation.Request, wait, limit time.Duration) automation.Answer {
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	h.mu.Lock()
	for h.app == nil {
		changed := h.changed
		h.mu.Unlock()
		select {
		case <-changed:
		case <-deadline.C:
			return automation.Reply(req.ID, nil, automation.Errorf(automation.NoApp, "no app connected within %v: status says whether it builds", wait))
		}
		h.mu.Lock()
	}
	app, gone := h.app, h.gone
	h.next++
	id := h.next
	answered := make(chan automation.Answer, 1)
	h.pending[id] = answered
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.pending, id)
		h.mu.Unlock()
	}()
	asked := req
	asked.ID = id
	data, _ := json.Marshal(asked)
	timeout := time.NewTimer(limit)
	defer timeout.Stop()
	select {
	case app <- data:
	case <-timeout.C:
		return automation.Reply(req.ID, nil, automation.Errorf(automation.Timeout, "the app did not take the request within %v", limit))
	case <-gone:
		return automation.Reply(req.ID, nil, automation.Errorf(automation.NoApp, "the app went away"))
	}
	select {
	case a := <-answered:
		a.ID = req.ID
		return a
	case <-gone:
		return automation.Reply(req.ID, nil, automation.Errorf(automation.NoApp, "the app went away before it answered"))
	case <-timeout.C:
		return automation.Reply(req.ID, nil, automation.Errorf(automation.Timeout, "the app did not answer within %v", limit))
	}
}

// connect makes an app's DevTools stream the one the tools' requests reach, returning the requests and what ends it as the stream ends, or the HTTP status that refuses it: a second app's while one is connected.
func (h *automationHub) connect() (requests chan []byte, done func(), status int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.app != nil {
		return nil, nil, http.StatusConflict
	}
	app, gone := make(chan []byte), make(chan struct{})
	h.app, h.gone = app, gone
	h.run++
	h.notify()
	return app, func() {
		h.mu.Lock()
		h.app, h.gone = nil, nil
		close(gone)
		h.notify()
		h.mu.Unlock()
	}, 0
}

// answered takes the app's answer to a request, as an automation message on /_nsgo/devtools/out.
func (h *automationHub) answered(data []byte) {
	var a automation.Answer
	if json.Unmarshal(data, &a) != nil {
		return
	}
	h.mu.Lock()
	answered := h.pending[a.ID]
	h.mu.Unlock()
	if answered != nil {
		// A second answer to one request is dropped.
		select {
		case answered <- a:
		default:
		}
	}
}

// lost fails the request waiting for the app's answer, as the post carrying it could not be read.
func (h *automationHub) lost(why string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, answered := range h.pending {
		select {
		case answered <- automation.Reply(id, nil, automation.Errorf(automation.Failed, "%s", why)):
		default:
		}
	}
}

// automationSession is what the session file says: where a tool reaches nsgo dev -automation for the app in its directory, and with which token.
type automationSession struct {
	URL    string `yaml:"url"`
	Token  string `yaml:"token"`
	PID    int    `yaml:"pid"`
	Target string `yaml:"target"`
}

// sessionPath is the session file of the app in dir.
func sessionPath(dir string) string { return filepath.Join(dir, ".nsgo", "automation.yaml") }

// readSession reads the session file of the app in dir.
func readSession(dir string) (*automationSession, error) {
	data, err := os.ReadFile(sessionPath(dir))
	if err != nil {
		return nil, err
	}
	var s automationSession
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", sessionPath(dir), err)
	}
	return &s, nil
}

// sessionLive reports whether the nsgo dev -automation s names runs: its process does, and its server answers status with the session's token, which a process that took a crashed one's ID never does.
func sessionLive(s *automationSession) bool {
	if !processAlive(s.PID) {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	body, _ := json.Marshal(automation.Request{ID: 1, Method: automation.Status})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL+"/_nsgo/automation", bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+s.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var a automation.Answer
	return resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&a) == nil && a.Error == nil
}

// errSessionTaken is writeSession's answer while another nsgo dev -automation runs for the app.
var errSessionTaken = errors.New("another nsgo dev -automation runs for this app")

// writeSession writes the session file of the app in dir, readable by its user alone, unless another live nsgo dev -automation wrote it.
func (h *automationHub) writeSession(dir string) error {
	if s, err := readSession(dir); err == nil && s.PID != os.Getpid() && sessionLive(s) {
		return fmt.Errorf("%w (process %d): stop it, or use its session through nsgo mcp", errSessionTaken, s.PID)
	}
	data, err := yaml.Marshal(automationSession{URL: h.srv.url(), Token: h.bridgeToken, PID: os.Getpid(), Target: h.target})
	if err != nil {
		return err
	}
	path := sessionPath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".automation-*.yaml")
	if err != nil {
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	_, werr := tmp.Write(data)
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(tmp.Name())
		return werr
	}
	h.session = path
	return os.Rename(tmp.Name(), path)
}

// removeSession removes the session file this hub wrote, if it is still its own.
func (h *automationHub) removeSession() {
	if h.session == "" {
		return
	}
	if s, err := readSession(filepath.Dir(filepath.Dir(h.session))); err == nil && s.PID == os.Getpid() {
		os.Remove(h.session)
	}
}
