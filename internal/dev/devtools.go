//go:build nsgodev

package dev

import (
	"bufio"
	"bytes"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

// runDevCommand runs c on its surface's graph (or across every surface for shared overlay modes) and sends the page what it answers.
func runDevCommand(c devCommand, out *devOut) {
	ctrl := devOverlays.Load()
	switch c.Op {
	case "surfaces":
		var ids []int
		if ctrl != nil {
			ids = ctrl.surfaceIDs(nil)
		} else {
			for _, s := range devtools.Surfaces() {
				ids = append(ids, s.ID())
			}
		}
		out.send(devMessage{Type: "surfaces", Data: ids})
		return
	case "highlight":
		if ctrl != nil {
			ctrl.setHighlight(c.On)
			return
		}
	case "layout":
		if ctrl != nil {
			ctrl.setLayout(c.On, out)
			return
		}
	case "watch-layout":
		if ctrl != nil {
			ctrl.watchLayout(out, c.Surface)
			return
		}
	case "pick":
		if ctrl != nil {
			ctrl.setPick(c.On, out)
			return
		}
	}
	var s devtools.Surface
	if ctrl != nil {
		s = ctrl.findSurface(c.Surface)
	} else {
		for _, x := range devtools.Surfaces() {
			if x.ID() == c.Surface {
				s = x
			}
		}
	}
	if s == nil {
		out.send(devMessage{Type: "error", Surface: c.Surface, Error: "no surface " + strconv.Itoa(c.Surface)})
		return
	}
	if c.Op == "inspect" && ctrl != nil {
		ctrl.inspect(s, c.ID)
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
		case "causes":
			if cs, ok := s.(devtools.CausalSurface); ok {
				reply(devMessage{Type: "causes", Data: cs.Causes(causalLimit(c.N))})
			}
		case "triggers":
			if cs, ok := s.(devtools.CausalSurface); ok {
				reply(devMessage{Type: "triggers", Data: cs.Triggers(causalLimit(c.N))})
			}
		case "disposals":
			if cs, ok := s.(devtools.CausalSurface); ok {
				reply(devMessage{Type: "disposals", Data: cs.Disposals(causalLimit(c.N))})
			}
		}
	})
}

func causalLimit(n int) int {
	if n <= 0 {
		return 100
	}
	return n
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

// devOverlays is the process-wide DevTools overlay controller, active under -highlight, -layout or -devtools.
var devOverlays atomic.Pointer[overlayController]

// startDevOverlay installs the shared DevTools overlay controller so every surface (main window, secondary windows, panels, overlays, scenes and embedded views) mounts its own overlay wired to it.
func startDevOverlay(highlight bool, layout core.LayoutAudit) *overlayController {
	c := newOverlayController(highlight, layout)
	if devDevtools == "1" && devChannel != nil {
		c.layoutOut = devChannel
	}
	devOverlays.Store(c)
	devtools.MountOverlay = c.mount
	return c
}

// overlayController shares DevTools' overlay state (highlight updates, Pick and the layout audit) across every surface of the process, including secondary windows, panels and separate embedded-view graphs.
type overlayController struct {
	mu             sync.Mutex
	surfaces       []devtools.Surface
	devIDs         map[devtools.Surface]int
	highlight      bool
	layoutSpec     core.LayoutAudit
	layoutShown    bool
	picking        bool
	pickOut        *devOut
	layoutOut      *devOut
	hoverSurface   devtools.Surface
	inspectSurface devtools.Surface
}

func newOverlayController(highlight bool, layout core.LayoutAudit) *overlayController {
	return &overlayController{
		devIDs:      map[devtools.Surface]int{},
		highlight:   highlight,
		layoutSpec:  layout,
		layoutShown: layout.Bounds || layout.Grid > 0,
	}
}

// mount returns the core.UpdateHighlights node to mount on listed surface s, a mounted callback that wires s into c, and a cleanup that unregisters s when its surface closes.
func (c *overlayController) mount(s devtools.Surface) (node any, mounted func(), cleanup func()) {
	c.mu.Lock()
	highlight := c.highlight
	layout := c.layoutSpec
	layoutShown := c.layoutShown
	c.mu.Unlock()

	opts := core.UpdateHighlights{
		NoUpdates:    !highlight,
		Layout:       layout,
		LayoutHidden: !layoutShown,
	}

	mounted = func() {
		c.mu.Lock()
		if !slices.Contains(c.surfaces, s) {
			c.surfaces = append(c.surfaces, s)
		}
		sid := c.surfaceIDLocked(s)
		curHighlight := c.highlight
		curLayoutShown := c.layoutShown
		picking := c.picking
		pickOut := c.pickOut
		c.mu.Unlock()

		s.Highlight(curHighlight)
		s.Layout(curLayoutShown)
		s.WatchLayout(func(on bool) { c.onSurfaceLayout(s, on) })
		if picking {
			c.startPickOnSurface(s, sid, pickOut)
		}
		c.notifySurfaces(nil)
	}

	cleanup = func() {
		c.mu.Lock()
		c.surfaces = slices.DeleteFunc(c.surfaces, func(x devtools.Surface) bool { return x == s })
		delete(c.devIDs, s)
		if c.hoverSurface == s {
			c.hoverSurface = nil
		}
		if c.inspectSurface == s {
			c.inspectSurface = nil
		}
		c.mu.Unlock()
		s.WatchLayout(nil)
		c.notifySurfaces(s)
	}

	return opts, mounted, cleanup
}

// surfaceIDLocked returns a stable unique DevTools ID for s while it lives. Single-graph surfaces keep s.ID(); another graph's surface (such as a second nsgo.NewView) whose s.ID() is already taken gets the lowest unused positive ID.
func (c *overlayController) surfaceIDLocked(s devtools.Surface) int {
	if id, ok := c.devIDs[s]; ok {
		return id
	}
	used := func(id int) bool {
		for _, v := range c.devIDs {
			if v == id {
				return true
			}
		}
		return false
	}
	if want := s.ID(); !used(want) {
		c.devIDs[s] = want
		return want
	}
	for id := 1; ; id++ {
		if !used(id) {
			c.devIDs[s] = id
			return id
		}
	}
}

// surfaceID returns s's DevTools surface ID.
func (c *overlayController) surfaceID(s devtools.Surface) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.surfaceIDLocked(s)
}

// surfaceIDs returns the DevTools IDs of all listed surfaces, excluding exclude.
func (c *overlayController) surfaceIDs(exclude devtools.Surface) []int {
	registered := devtools.Surfaces()
	c.mu.Lock()
	defer c.mu.Unlock()
	seen := map[devtools.Surface]bool{}
	var ids []int
	for _, s := range registered {
		if s == exclude || seen[s] {
			continue
		}
		seen[s] = true
		ids = append(ids, c.surfaceIDLocked(s))
	}
	for _, s := range c.surfaces {
		if s == exclude || seen[s] {
			continue
		}
		seen[s] = true
		ids = append(ids, c.surfaceIDLocked(s))
	}
	return ids
}

// findSurface returns the live surface whose DevTools ID is id, or nil.
func (c *overlayController) findSurface(id int) devtools.Surface {
	registered := devtools.Surfaces()
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range registered {
		if c.surfaceIDLocked(s) == id {
			return s
		}
	}
	for _, s := range c.surfaces {
		if c.surfaceIDLocked(s) == id {
			return s
		}
	}
	return nil
}

// notifySurfaces pushes the updated surface list to connected DevTools pages.
func (c *overlayController) notifySurfaces(exclude devtools.Surface) {
	if devDevtools != "1" || devChannel == nil {
		return
	}
	devChannel.send(devMessage{Type: "surfaces", Data: c.surfaceIDs(exclude)})
}

func (c *overlayController) setHighlight(on bool) {
	c.mu.Lock()
	c.highlight = on
	targets := slices.Clone(c.surfaces)
	c.mu.Unlock()
	for _, x := range targets {
		s := x
		s.Post(func() { s.Highlight(on) })
	}
}

func (c *overlayController) setLayout(on bool, out *devOut) {
	c.mu.Lock()
	c.layoutShown = on
	if out != nil {
		c.layoutOut = out
	}
	notify := c.layoutOut
	targets := slices.Clone(c.surfaces)
	c.mu.Unlock()
	for _, x := range targets {
		s := x
		s.Post(func() { s.Layout(on) })
	}
	if notify != nil {
		notify.send(devMessage{Type: "layout", Surface: -1, Data: on})
	}
}

func (c *overlayController) watchLayout(out *devOut, surface int) {
	c.mu.Lock()
	c.layoutOut = out
	shown := c.layoutShown
	c.mu.Unlock()
	if out != nil {
		out.send(devMessage{Type: "layout", Surface: surface, Data: shown})
	}
}

// onSurfaceLayout propagates a layout audit toggle (such as Mod+Alt+Shift+L in any surface) to every other surface and to DevTools.
func (c *overlayController) onSurfaceLayout(from devtools.Surface, on bool) {
	c.mu.Lock()
	if on == c.layoutShown {
		c.mu.Unlock()
		return
	}
	c.layoutShown = on
	others := make([]devtools.Surface, 0, len(c.surfaces))
	for _, x := range c.surfaces {
		if x != from {
			others = append(others, x)
		}
	}
	out := c.layoutOut
	c.mu.Unlock()
	for _, x := range others {
		s := x
		s.Post(func() { s.Layout(on) })
	}
	if out != nil {
		out.send(devMessage{Type: "layout", Surface: -1, Data: on})
	}
}

func (c *overlayController) setPick(on bool, out *devOut) {
	c.mu.Lock()
	c.picking = on
	if on {
		c.pickOut = out
	} else {
		c.hoverSurface = nil
	}
	type target struct {
		s  devtools.Surface
		id int
	}
	targets := make([]target, len(c.surfaces))
	for i, s := range c.surfaces {
		targets[i] = target{s: s, id: c.surfaceIDLocked(s)}
	}
	c.mu.Unlock()
	for _, t := range targets {
		t := t
		if on {
			t.s.Post(func() { c.startPickOnSurface(t.s, t.id, out) })
		} else {
			t.s.Post(func() { t.s.Pick(false, nil, nil) })
		}
	}
}

func (c *overlayController) startPickOnSurface(s devtools.Surface, sid int, out *devOut) {
	s.Pick(true,
		func(id uint32) {
			c.mu.Lock()
			prev := c.hoverSurface
			if prev != s {
				c.hoverSurface = s
			} else {
				prev = nil
			}
			c.mu.Unlock()
			if prev != nil {
				prev.Post(func() { prev.Inspect(0) })
			}
			if out != nil {
				out.send(devMessage{Type: "hover", Surface: sid, ID: id})
			}
		},
		func(id uint32) {
			c.mu.Lock()
			c.picking = false
			c.hoverSurface = nil
			c.inspectSurface = s
			others := make([]devtools.Surface, 0, len(c.surfaces))
			for _, x := range c.surfaces {
				if x != s {
					others = append(others, x)
				}
			}
			c.mu.Unlock()
			for _, x := range others {
				other := x
				other.Post(func() { other.Pick(false, nil, nil) })
			}
			if out != nil {
				out.send(devMessage{Type: "picked", Surface: sid, ID: id})
			}
		},
	)
}

func (c *overlayController) inspect(target devtools.Surface, id uint32) {
	c.mu.Lock()
	var prev devtools.Surface
	if id != 0 {
		if c.inspectSurface != nil && c.inspectSurface != target {
			prev = c.inspectSurface
		}
		c.inspectSurface = target
	} else if c.inspectSurface == target {
		c.inspectSurface = nil
	}
	c.mu.Unlock()
	if prev != nil {
		prev.Post(func() { prev.Inspect(0) })
	}
	target.Post(func() { target.Inspect(id) })
}
