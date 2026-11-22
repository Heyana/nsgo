//go:build nsgodev

package dev

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

// devDevtools is "1" in nsgo dev -devtools's builds, set with -ldflags -X.
var devDevtools = ""

// devCommand is what the DevTools page asks the app: an op on a surface, with what it needs.
type devCommand struct {
	Op      string `json:"op"`
	Surface int    `json:"surface"`
	ID      uint32 `json:"id"`
	On      bool   `json:"on"`
	Signal  uint64 `json:"signal"`
	Value   string `json:"value"`
	N       int    `json:"n"`
}

// devMessage is what the app tells the DevTools page. Type comes first: the server tells the automation's answers by it (cmd/nsgo's automationMessage).
type devMessage struct {
	Type    string                 `json:"type"`
	Surface int                    `json:"surface"`
	ID      uint32                 `json:"id,omitempty"`
	Error   string                 `json:"error,omitempty"`
	Data    any                    `json:"data,omitempty"`
	Commit  *devCommit             `json:"commit,omitempty"`
	Step    *devtools.Step         `json:"step,omitempty"`
	Hot     []devtools.Computation `json:"hot,omitempty"`
}

// maxUnapplied bounds the batches waiting for the platform that DevTools remember: a platform that applies none, as Android's Java runtime reports none, keeps no more.
const maxUnapplied = 1000

// devCommit is a commit as the profiler shows it.
type devCommit struct {
	N      int            `json:"n"`
	Items  int            `json:"items"`
	Millis float64        `json:"ms"`
	Causes *driver.Causes `json:"causes,omitempty"`
}

// startDevTools has core record what DevTools show, sends the page each commit, and follows the page's commands from the server's events. It runs once, before any graph starts (Start).
func startDevTools(server string) {
	out := openDevChannel(server)
	driver.TraceCauses = true
	commits := 0
	var mu sync.Mutex
	// sent are the batches the platform has not applied yet, by commit number, so its time joins the commit's row.
	sent := map[*driver.MountBatch]int{}
	prev := engine.Trace
	engine.Trace = func(b *driver.MountBatch, took time.Duration) {
		if prev != nil {
			prev(b, took)
		}
		mu.Lock()
		commits++
		n := commits
		if len(sent) < maxUnapplied {
			sent[b] = n
		}
		mu.Unlock()
		out.send(devMessage{Type: "commit", Commit: &devCommit{N: n, Items: len(b.Items), Millis: float64(took.Microseconds()) / 1000, Causes: b.Causes}})
	}
	driver.Applied = func(b *driver.MountBatch, took time.Duration) {
		mu.Lock()
		n, ok := sent[b]
		delete(sent, b)
		mu.Unlock()
		if ok {
			out.send(devMessage{Type: "applied", ID: uint32(n), Data: float64(took.Microseconds()) / 1000})
		}
	}
}

// devChannel is the app's end of the channel DevTools and nsgo dev -automation share, nil until either starts it.
var devChannel *devOut

// openDevChannel has core record what DevTools show and register its trees, follows the server's events for DevTools' commands and the automation's requests, and returns where the app posts its messages; it starts them once, for whichever of DevTools and the automation starts first, before any graph starts (Start).
func openDevChannel(server string) *devOut {
	if devChannel == nil {
		devtools.Enabled = true
		devChannel = &devOut{server: server, wake: make(chan struct{}, 1), hurry: make(chan struct{}, 1)}
		go devChannel.run()
		go listenDevTools(server, devChannel)
	}
	return devChannel
}

// listenDevTools follows the server's events as the app's DevTools stream, ?devtools, for DevTools' commands under -devtools and the automation's requests under -automation, reconnecting while it runs: -automation alone takes no page's commands. A stream the server refuses, as a second app's while one answers the automation, is tried again a second later.
func listenDevTools(server string, out *devOut) {
	for {
		if resp, err := devtools.Fetch("GET", server+"/_nsgo/events?devtools", "", nil); err == nil {
			sc := bufio.NewScanner(resp.Body)
			sc.Buffer(make([]byte, 1<<16), 16<<20)
			for resp.StatusCode == 200 && sc.Scan() {
				data, ok := strings.CutPrefix(sc.Text(), "data: ")
				if !ok {
					continue
				}
				switch kind, raw, _ := strings.Cut(data, " "); kind {
				case "devtools":
					var c devCommand
					if devDevtools == "1" && json.Unmarshal([]byte(raw), &c) == nil {
						runDevCommand(c, out)
					}
				case "automation":
					// The agent may wait for the app to settle: DevTools' commands go on meanwhile.
					if a := devtools.Automation; a != nil {
						go func() { out.sendNow(devMessage{Type: "automation", Data: json.RawMessage(a.Do([]byte(raw)))}) }()
					}
				}
			}
			resp.Body.Close()
		}
		time.Sleep(time.Second)
	}
}

// runDevCommand runs c on its surface's graph and sends the page what it answers.
func runDevCommand(c devCommand, out *devOut) {
	if c.Op == "surfaces" {
		var ids []int
		for _, s := range devtools.Surfaces() {
			ids = append(ids, s.ID())
		}
		out.send(devMessage{Type: "surfaces", Data: ids})
		return
	}
	var s devtools.Surface
	for _, x := range devtools.Surfaces() {
		if x.ID() == c.Surface {
			s = x
		}
	}
	if s == nil {
		out.send(devMessage{Type: "error", Surface: c.Surface, Error: "no surface " + strconv.Itoa(c.Surface)})
		return
	}
	reply := func(m devMessage) {
		m.Surface = c.Surface
		out.send(m)
	}
	s.Post(func() {
		switch c.Op {
		case "elements":
			reply(devMessage{Type: "elements", Data: s.Elements()})
		case "owners":
			reply(devMessage{Type: "owners", Data: s.Owners()})
		case "inspect":
			s.Inspect(c.ID)
		case "highlight":
			s.Highlight(c.On)
		case "layout":
			s.Layout(c.On)
		case "watch-layout":
			s.WatchLayout(func(on bool) { reply(devMessage{Type: "layout", Data: on}) })
		case "pick":
			s.Pick(c.On, func(id uint32) { reply(devMessage{Type: "hover", ID: id}) }, func(id uint32) { reply(devMessage{Type: "picked", ID: id}) })
		case "record":
			s.Record(c.On, func(step devtools.Step) { reply(devMessage{Type: "step", Step: &step}) })
		case "set":
			m := devMessage{Type: "set"}
			if err := s.Set(c.Signal, c.Value); err != nil {
				m.Error = err.Error()
			}
			reply(m)
		case "hot":
			reply(devMessage{Type: "hot", Hot: s.Hot(max(c.N, 1))})
		}
	})
}

// devOut sends the page and the automation messages: one goroutine posts what gathered while the last post was on its way, so the graph never waits for the network.
type devOut struct {
	server      string
	mu          sync.Mutex
	pending     bytes.Buffer
	wake, hurry chan struct{}
}

func (o *devOut) send(m devMessage) { o.queue(m, false) }

// sendNow sends m without waiting for a quarter of a second's commits: the automation waits for its answer.
func (o *devOut) sendNow(m devMessage) { o.queue(m, true) }

func (o *devOut) queue(m devMessage, now bool) {
	data, err := json.Marshal(m)
	if err != nil {
		return
	}
	o.mu.Lock()
	o.pending.Write(data)
	o.pending.WriteByte('\n')
	o.mu.Unlock()
	select {
	case o.wake <- struct{}{}:
	default:
	}
	if now {
		select {
		case o.hurry <- struct{}{}:
		default:
		}
	}
}

func (o *devOut) run() {
	for range o.wake {
		// Commits come every frame: a quarter of a second's go in one post, unless an answer hurries it.
		wait := time.NewTimer(250 * time.Millisecond)
		select {
		case <-wait.C:
		case <-o.hurry:
			wait.Stop()
		}
		o.mu.Lock()
		body := bytes.Clone(o.pending.Bytes())
		o.pending.Reset()
		o.mu.Unlock()
		if len(body) == 0 {
			continue
		}
		if resp, err := devtools.Fetch("POST", o.server+"/_nsgo/devtools/out", "application/x-ndjson", bytes.NewReader(body)); err == nil {
			resp.Body.Close()
		}
	}
}

// withDevOverlay mounts DevTools' overlay over app, outlining commits from the start under -highlight and showing the layout audit as -layout says.
func withDevOverlay(app engine.App, highlight bool, layout core.LayoutAudit) engine.App {
	return func(cx *core.Scope) core.Node {
		return core.Group{core.Func(app), core.UpdateHighlights{NoUpdates: !highlight, Layout: layout}}
	}
}
