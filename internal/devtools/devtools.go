// Package devtools is what nsgo dev -devtools and -automation read from a running app and send it: core registers each surface's tree here while Enabled, and nsgo's dev client serves them to the DevTools page and hands them to the automation's Agent, over one channel. Its types are what the page receives as JSON.
package devtools

import (
	"slices"
	"sync"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// OverlayTestID is the test ID of DevTools' overlay (core.UpdateHighlights), whose views are DevTools' own, not the app's: the automation's Agent leaves them out of what it lists and finds, as the page's Elements does.
const OverlayTestID = "nsgo-update-highlights"

// Enabled has core record what DevTools show, and register its trees: nsgo's dev client sets it under nsgo dev -devtools, -automation and -trace, before any graph starts. While it is unset, core records nothing and pays one check where it would.
var Enabled bool

// MountOverlay, set by nsgo's dev client in nsgodev builds under -devtools, -highlight or -layout, returns the core.Node (core.UpdateHighlights) to mount on each listed surface s as its tree mounts, an optional mounted callback run right after it mounts, and an optional cleanup run when s's root scope disposes, all on s's graph goroutine. Leaving it nil in release builds keeps DevTools' overlay unlinked from core.
var MountOverlay func(s Surface) (node any, mounted func(), cleanup func())

// ScriptSite, which tsx sets while Enabled, is the line of TSX its JS runs now, through the bundles' source maps; ok is false while none runs. A site that falls in tsx's Go code takes it, so DevTools name the app's TSX rather than the Go that ran it.
var ScriptSite func() (file string, line int, ok bool)

// Agent is nsgo dev -automation's agent, package nsgotest's, which keeps a model of each surface as a test keeps one of the app it opened, and answers the automation's requests with them. The builds with the nsgoautomation tag link it.
type Agent interface {
	// Registered hears each surface on its graph goroutine before its first batch. listed reports whether it shows views or holds app-level services alone.
	Registered(s Surface, listed bool)
	// Unregistered hears that s went, as its window closed, on its graph goroutine.
	Unregistered(s Surface)
	// Do answers a request of package internal/automation's protocol, JSON in and out, from any goroutine.
	Do(request []byte) []byte
}

// Automation is the agent while nsgo dev -automation drives the app: nsgotest sets it as the process starts, and the dev client clears it in an app the tool gave no token. Register and Unregister hand it every surface, the ones DevTools pages leave out too.
var Automation Agent

// Follower hears what a surface it follows commits and what the surface's platform reports, on the surface's graph goroutine.
type Follower interface {
	// Batch hears each batch the surface sends its platform; it may keep the batch and must not block.
	Batch(b *driver.MountBatch)
	// Event hears each event the surface's platform reports; it must not block.
	Event(ev driver.Event)
}

// Surface is one surface's tree, as DevTools read and drive it. Every method but ID, Graph and Post, whose answers never change, runs on the surface's graph goroutine: call it through Post.
type Surface interface {
	// ID numbers the surface within its graph, 0 for the main window's.
	ID() int
	// Graph numbers the surface's graph within the process, from 1: a process that runs several graphs, as a host embedding several apps does, has surfaces of the same ID.
	Graph() int
	// Post runs fn on the surface's graph goroutine, from any goroutine.
	Post(fn func())
	// Elements returns the surface's views as a tree, from its root view.
	Elements() Node
	// Owners returns the graph's owners as a tree, from its root, with the values and computations each holds.
	Owners() Owner
	// Inspect outlines view id on the surface, or nothing for 0.
	Inspect(id uint32)
	// Highlight turns the outlines of what each commit changed on or off.
	Highlight(on bool)
	// Layout shows the layout audit over the surface or hides it.
	Layout(on bool)
	// WatchLayout reports to shown whether the layout audit shows, now and each time that changes, until the next call; a surface without DevTools' overlay reports false once.
	WatchLayout(shown func(on bool))
	// Pick has the next press on the surface choose a view rather than reach the app, reporting each view the pointer is over to hover and the one pressed to picked; off ends it.
	Pick(on bool, hover, picked func(id uint32))
	// Record reports what the user does on the surface as test steps, until off.
	Record(on bool, step func(Step))
	// Set writes value, as text, to the signal numbered signal: strings, booleans and numbers only.
	Set(signal uint64, value string) error
	// Hot returns the computations that ran most since the last call, at most n.
	Hot(n int) []Computation
	// Follow has f hear the surface's batches and events from now on, nil none.
	Follow(f Follower)
}

// Node is a view and the views in it.
type Node struct {
	ID uint32 `json:"id"`
	// Kind is the view's kind: View, Text, Input, Scroll, Image and the others.
	Kind string `json:"kind"`
	// Component is the nearest component around the view, as its Go type (ui.Button), and Site the line of the function that built it, as /path/to/file.go:line, where there is one.
	Component string `json:"component,omitempty"`
	Site      string `json:"site,omitempty"`
	TestID    string `json:"testID,omitempty"`
	Role      string `json:"role,omitempty"`
	Label     string `json:"label,omitempty"`
	Text      string `json:"text,omitempty"`
	// Clipped is whether Text is the first of a longer text, ending in an ellipsis.
	Clipped  bool       `json:"clipped,omitempty"`
	Selector string     `json:"selector,omitempty"`
	Bounds   [4]float32 `json:"bounds"`
	Hidden   bool       `json:"hidden,omitempty"`
	// Classes are the view's Tailwind classes, and Box its margin, border and padding as laid out.
	Classes  string `json:"classes,omitempty"`
	Box      Box    `json:"box"`
	Style    []Prop `json:"style,omitempty"`
	Children []Node `json:"children,omitempty"`
}

// Box is a view's margin, border and padding as laid out, each top, right, bottom and left.
type Box struct {
	Margin  [4]float32 `json:"margin"`
	Border  [4]float32 `json:"border"`
	Padding [4]float32 `json:"padding"`
}

// Prop is a style property a view sets, by its Go name.
type Prop struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Owner is an owner of the graph: a component's scope, a computation's, a region's or the root, with the values and computations it holds and the owners under it.
type Owner struct {
	Name         string        `json:"name"`
	Site         string        `json:"site,omitempty"`
	Values       []Value       `json:"values,omitempty"`
	Computations []Computation `json:"computations,omitempty"`
	Children     []Owner       `json:"children,omitempty"`
}

// Value is a signal: where it was made, its Go type and its value as text, and whether Set takes it.
type Value struct {
	ID       uint64 `json:"id"`
	Site     string `json:"site,omitempty"`
	Type     string `json:"type"`
	Text     string `json:"text"`
	Editable bool   `json:"editable,omitempty"`
	// Readers are the computations that read it now: none for a value no binding shows.
	Readers int `json:"readers"`
}

// Computation is a memo, a binding or an effect: where it was made and how often it ran.
type Computation struct {
	Kind string `json:"kind"`
	Site string `json:"site,omitempty"`
	Runs int    `json:"runs"`
	// Took is the time its runs took, in nanoseconds, not counting what their writes ran next.
	Took int64 `json:"took,omitempty"`
	// Value is a memo's value as text, and Readers the computations that read it.
	Value   string `json:"value,omitempty"`
	Readers int    `json:"readers,omitempty"`
}

// Step is what the user did, as a test step: Press, Type or Clear on the view Selector names, Type with Text, or ScrollTo with the Offset a scroll view came to.
type Step struct {
	Action   string     `json:"action"`
	Selector string     `json:"selector"`
	Text     string     `json:"text,omitempty"`
	Offset   [2]float32 `json:"offset,omitempty"`
}

// registered is a surface core registered, and whether DevTools pages list it.
type registered struct {
	s      Surface
	listed bool
}

var (
	mu       sync.Mutex
	surfaces []registered
)

// Register adds s to the surfaces DevTools pages list, where listed, and hands it to the automation's agent, listed or not: the first tree of an app that opens windows shows no view of its own, but holds the app's alerts and main menu.
func Register(s Surface, listed bool) {
	mu.Lock()
	surfaces = append(surfaces, registered{s, listed})
	mu.Unlock()
	if a := Automation; a != nil {
		a.Registered(s, listed)
	}
}

// Unregister removes s.
func Unregister(s Surface) {
	mu.Lock()
	at := slices.IndexFunc(surfaces, func(r registered) bool { return r.s == s })
	if at >= 0 {
		surfaces = slices.Delete(surfaces, at, at+1)
	}
	mu.Unlock()
	if a := Automation; a != nil && at >= 0 {
		a.Unregistered(s)
	}
}

// Surfaces returns the surfaces DevTools pages list, the main window's first.
func Surfaces() []Surface {
	mu.Lock()
	defer mu.Unlock()
	var out []Surface
	for _, r := range surfaces {
		if r.listed {
			out = append(out, r.s)
		}
	}
	return out
}
