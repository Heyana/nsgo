// Package nsgotest runs NSGo apps headless in Go tests, as net/http/httptest
// runs handlers: no window, approximate text, a manual clock, and every
// batch applied to a model of the native views that tests query.
//
//	func TestAdd(t *testing.T) {
//		app := nsgotest.Start(t, Root)
//		app.Type("id:draft", "Milk")
//		app.Press("role:button Add")
//		app.Find("text:Milk")
//	}
//
// Selectors pick visible views: "id:<TestID>", "text:<substring>",
// "label:<Label>", "role:<role>" and "role:<role> <label or text>".
// " >> " chains them, each part looking inside the views the last
// matched, and "nth=<i>" picks one match. A selector that matches no view
// or more than one fails the test.
package nsgotest

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oneo-me/nsgo/core"
)

// Option configures Start.
type Option func(*App)

// Size sets the surface size; 390 by 844 points by default.
func Size(width, height core.Points) Option {
	return func(a *App) { a.size = core.Size{Width: width, Height: height} }
}

// App is an app running headless. Its methods run on the test's
// goroutine; the app's goroutines reach it through Scope.Post, which App
// runs when it settles.
type App struct {
	t    testing.TB
	size core.Size
	// Clipboard is the clipboard's text; Announced what screen readers
	// were asked to say.
	Clipboard string
	Announced []string
	// Keymap is the keyboard shortcuts the app binds; Title its title.
	Keymap []string
	Title  string
	// Menu is the application menu the app set; Window the window
	// commands it sent, oldest first.
	Menu   []core.MenuEntry
	Window []core.WindowCommand
	tree   *core.Tree
	root   *core.Scope
	Clock  *core.ManualClock

	mu    sync.Mutex
	posts []func()

	views  map[core.NodeID]*View
	alerts []core.AlertConfig
	rootID core.NodeID
	edits  map[core.NodeID]uint32
}

// View is the model of one native view, as the last batch left it.
type View struct {
	ID       core.NodeID
	Kind     core.ViewKind
	Parent   *View
	Children []*View
	Frame    core.Bounds
	Style    core.ViewStyle
	Props    core.ViewProps
	Events   core.EventMask
	Text     string // a text view's text or a field's value
	Input    core.InputConfig
	Content  core.Size  // a scroll view's content
	Offset   core.Point // and its offset
	Image    core.ImageConfig
	Focused  bool
}

// Start runs app and settles it; the test's cleanup disposes it.
func Start(t testing.TB, app func(cx *core.Scope) core.Node, opts ...Option) *App {
	t.Helper()
	a := &App{
		t:     t,
		size:  core.Size{Width: 390, Height: 844},
		Clock: core.NewManualClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		views: map[core.NodeID]*View{},
		edits: map[core.NodeID]uint32{},
	}
	for _, o := range opts {
		o(a)
	}
	a.root = core.NewRoot(core.RootOptions{
		Post: func(fn func()) {
			a.mu.Lock()
			a.posts = append(a.posts, fn)
			a.mu.Unlock()
		},
		Clock:  a.Clock,
		Report: func(err error) { t.Errorf("nsgotest: the app failed: %v", err) },
	})
	t.Cleanup(a.root.Dispose)
	a.tree = core.NewTree(a.root, core.ApproxTextEngine{})
	a.tree.Resize(a.size)
	a.tree.Mount(app)
	a.Settle()
	return a
}

// Settle runs what the app's goroutines posted and applies the commits
// that follow, until nothing is left.
func (a *App) Settle() {
	for {
		a.mu.Lock()
		posts := a.posts
		a.posts = nil
		a.mu.Unlock()
		for _, fn := range posts {
			fn()
		}
		b := a.tree.Commit()
		if b != nil {
			a.apply(b)
		}
		if b == nil && len(posts) == 0 {
			return
		}
	}
}

// Advance moves the clock by d, runs the timers that come due and
// settles.
func (a *App) Advance(d time.Duration) {
	a.Clock.Advance(d)
	a.Settle()
}

// WaitFor settles until selector matches a view, failing the test after
// timeout of real time. Use it for work on other goroutines.
func (a *App) WaitFor(selector string, timeout time.Duration) *View {
	a.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		a.Settle()
		if vs := a.Query(selector); len(vs) == 1 {
			return vs[0]
		}
		if time.Now().After(deadline) {
			a.t.Fatalf("nsgotest: no view matches %q after %v\n%s", selector, timeout, a.Dump())
		}
		time.Sleep(time.Millisecond)
	}
}

// SetEnvironment reports env as a platform would and settles.
func (a *App) SetEnvironment(env core.Environment) {
	a.tree.SetEnvironment(env)
	a.Settle()
}

// Resize sets the surface size and settles.
func (a *App) Resize(width, height core.Points) {
	a.tree.Resize(core.Size{Width: width, Height: height})
	a.Settle()
}

// Find returns the one visible view selector matches.
func (a *App) Find(selector string) *View {
	a.t.Helper()
	vs := a.Query(selector)
	switch len(vs) {
	case 0:
		a.t.Fatalf("nsgotest: no view matches %q\n%s", selector, a.Dump())
	case 1:
		return vs[0]
	default:
		a.t.Fatalf("nsgotest: %d views match %q\n%s", len(vs), selector, a.Dump())
	}
	return nil
}

// Exists reports whether a visible view matches selector.
func (a *App) Exists(selector string) bool { return len(a.Query(selector)) > 0 }

// Query returns the visible views selector matches, in tree order.
func (a *App) Query(selector string) []*View {
	a.t.Helper()
	scope := []*View{a.views[a.rootID]}
	if scope[0] == nil {
		return nil
	}
	for i, part := range strings.Split(selector, " >> ") {
		if n, ok := strings.CutPrefix(part, "nth="); ok {
			var at int
			if _, err := fmt.Sscan(n, &at); err != nil {
				a.t.Fatalf("nsgotest: bad %q in %q", part, selector)
			}
			if at < 0 || at >= len(scope) {
				return nil
			}
			scope = scope[at : at+1]
			continue
		}
		kind, arg, ok := strings.Cut(part, ":")
		if !ok {
			a.t.Fatalf("nsgotest: %q in %q has no kind; use id:, text:, label: or role:", part, selector)
		}
		var out []*View
		seen := map[*View]bool{}
		for _, root := range scope {
			walk(root, i > 0, func(v *View) {
				if !seen[v] && matches(v, kind, arg) {
					seen[v] = true
					out = append(out, v)
				}
			})
		}
		scope = out
	}
	return scope
}

func matches(v *View, kind, arg string) bool {
	switch kind {
	case "id":
		return v.Props.TestID == arg
	case "text":
		return v.Kind == core.KindText && strings.Contains(v.Text, arg)
	case "label":
		return v.Props.Label == arg
	case "role":
		role, name, _ := strings.Cut(arg, " ")
		if roleName(v.Props.Role) != role {
			return false
		}
		return name == "" || v.Props.Label == name || strings.Contains(v.textWithin(), name)
	}
	return false
}

func (v *View) textWithin() string {
	var b strings.Builder
	var walk func(*View)
	walk = func(v *View) {
		if v.Kind == core.KindText {
			b.WriteString(v.Text)
		}
		for _, c := range v.Children {
			walk(c)
		}
	}
	walk(v)
	return b.String()
}

var roles = map[core.Role]string{
	core.RoleButton: "button", core.RoleLink: "link", core.RoleHeader: "header", core.RoleImage: "image",
	core.RoleCheckbox: "checkbox", core.RoleSwitch: "switch", core.RoleRadio: "radio", core.RoleTab: "tab",
	core.RoleText: "text", core.RoleTextField: "textbox",
}

func roleName(r core.Role) string { return roles[r] }

// walk calls fn for v (unless only below) and its visible descendants.
func walk(v *View, below bool, fn func(*View)) {
	if v.Style.Hidden {
		return
	}
	if !below {
		fn(v)
	}
	for _, c := range v.Children {
		walk(c, false, fn)
	}
}

// Press presses the view selector matches, or the pressable view around
// it, as a tap on it would.
func (a *App) Press(selector string) {
	a.t.Helper()
	v := a.Find(selector)
	for p := v; p != nil; p = p.Parent {
		if p.Events.Has(core.EventPress) || p.Events.Has(core.EventPressDown) {
			a.press(p, core.EventPress)
			return
		}
	}
	a.t.Fatalf("nsgotest: nothing pressable at %q", selector)
}

// press sends what a tap on v sends: presses outside to the overlays
// that listen and do not hold v, then down, up and the press itself.
func (a *App) press(v *View, kind core.EventKind) {
	at := v.surfaceFrame()
	center := core.Point{X: at.Origin.X + at.Size.Width/2, Y: at.Origin.Y + at.Size.Height/2}
	var outside []core.NodeID
	walk(a.views[a.rootID], false, func(o *View) {
		if o.Events.Has(core.EventPressOutside) && !v.within(o) {
			outside = append(outside, o.ID)
		}
	})
	for _, id := range outside {
		a.dispatch(core.Event{Kind: core.EventPressOutside, ID: id, Point: center})
	}
	for _, k := range []core.EventKind{core.EventPressDown, core.EventPressUp, kind} {
		if v.Events.Has(k) {
			a.dispatch(core.Event{Kind: k, ID: v.ID})
		}
	}
}

// PressDown starts a press on the view selector matches, or the pressable
// view around it, and holds it until PressUp.
func (a *App) PressDown(selector string) { a.t.Helper(); a.half(selector, core.EventPressDown) }

// PressUp ends a press PressDown started.
func (a *App) PressUp(selector string) { a.t.Helper(); a.half(selector, core.EventPressUp) }

func (a *App) half(selector string, kind core.EventKind) {
	a.t.Helper()
	for p := a.Find(selector); p != nil; p = p.Parent {
		if p.Events.Has(kind) {
			a.dispatch(core.Event{Kind: kind, ID: p.ID})
			return
		}
	}
	a.t.Fatalf("nsgotest: nothing at %q reports %v", selector, kind)
}

// Hover moves the pointer onto (or, with on false, off) the view selector
// matches.
func (a *App) Hover(selector string, on bool) {
	a.t.Helper()
	kind := core.EventHoverIn
	if !on {
		kind = core.EventHoverOut
	}
	a.half(selector, kind)
}

// LongPress long-presses the view selector matches, or the pressable view
// around it.
func (a *App) LongPress(selector string) {
	a.t.Helper()
	v := a.Find(selector)
	for p := v; p != nil; p = p.Parent {
		if p.Events.Has(core.EventLongPress) {
			a.press(p, core.EventLongPress)
			return
		}
	}
	a.t.Fatalf("nsgotest: nothing long-pressable at %q", selector)
}

// Alerts returns the alerts shown and not answered yet, oldest first.
func (a *App) Alerts() []core.AlertConfig { return a.alerts }

// Answer chooses the button at index (-1: dismiss) of the oldest alert
// shown.
func (a *App) Answer(index int) {
	a.t.Helper()
	if len(a.alerts) == 0 {
		a.t.Fatal("nsgotest: no alert to answer")
	}
	id := a.alerts[0].ID
	a.alerts = a.alerts[1:]
	a.dispatch(core.Event{Kind: core.EventAlert, Count: id, Index: index})
}

// RightClick sends a secondary click to the view selector matches, or the
// one around it that listens.
func (a *App) RightClick(selector string) { a.t.Helper(); a.half(selector, core.EventContextMenu) }

// Key presses a keyboard shortcut, such as "mod+s", if the app binds it.
func (a *App) Key(key string) {
	a.t.Helper()
	k := core.NormalizeKey(key)
	for _, b := range a.Keymap {
		if b == k {
			a.dispatch(core.Event{Kind: core.EventKey, Text: k})
			return
		}
	}
	a.t.Fatalf("nsgotest: no binding for %q in %v", key, a.Keymap)
}

// Pan drags the view selector matches, or the one around it with a pan,
// through each translation in turn, then lets go.
func (a *App) Pan(selector string, translations ...core.Point) {
	a.t.Helper()
	v := a.gestureView(selector, core.EventPan)
	a.gesture(v, core.EventPan, len(translations), func(ev *core.Event, i int) {
		ev.Point = translations[i]
	})
}

// Pinch pinches the view selector matches, or the one around it with a
// pinch, through each scale in turn.
func (a *App) Pinch(selector string, scales ...float32) {
	a.t.Helper()
	v := a.gestureView(selector, core.EventPinch)
	a.gesture(v, core.EventPinch, len(scales), func(ev *core.Event, i int) { ev.Value = scales[i] })
}

// Rotate rotates the view selector matches, or the one around it with a
// rotation, through each angle in radians in turn.
func (a *App) Rotate(selector string, angles ...float32) {
	a.t.Helper()
	v := a.gestureView(selector, core.EventRotation)
	a.gesture(v, core.EventRotation, len(angles), func(ev *core.Event, i int) { ev.Value = angles[i] })
}

func (a *App) gestureView(selector string, kind core.EventKind) *View {
	a.t.Helper()
	for p := a.Find(selector); p != nil; p = p.Parent {
		if p.Events.Has(kind) {
			return p
		}
	}
	a.t.Fatalf("nsgotest: no gesture view at %q", selector)
	return nil
}

// gesture sends a gesture of n steps: began with the first, changed with
// the rest and ended with the last.
func (a *App) gesture(v *View, kind core.EventKind, n int, step func(*core.Event, int)) {
	if n == 0 {
		return
	}
	send := func(phase core.GesturePhase, i int) {
		ev := core.Event{Kind: kind, ID: v.ID, Phase: phase}
		step(&ev, i)
		a.dispatch(ev)
	}
	send(core.GestureBegan, 0)
	for i := 1; i < n; i++ {
		send(core.GestureChanged, i)
	}
	send(core.GestureEnded, n-1)
}

// ChooseMenu chooses the application menu item at path, its labels from
// the menu bar down, such as "File", "New".
func (a *App) ChooseMenu(path ...string) {
	a.t.Helper()
	items := a.Menu
	for i, label := range path {
		var found *core.MenuEntry
		for k := range items {
			if items[k].Label == label {
				found = &items[k]
			}
		}
		if found == nil || found.Disabled {
			a.t.Fatalf("nsgotest: no menu item %q", path[:i+1])
		}
		if i == len(path)-1 {
			if found.ID == 0 {
				a.t.Fatalf("nsgotest: menu item %q does nothing", path)
			}
			a.dispatch(core.Event{Kind: core.EventMenu, Count: found.ID})
			return
		}
		items = found.Items
	}
}

// Back presses Escape or the back button.
func (a *App) Back() { a.dispatch(core.Event{Kind: core.EventBack, ID: a.rootID}) }

func (v *View) surfaceFrame() core.Bounds {
	b := v.Frame
	for p := v.Parent; p != nil; p = p.Parent {
		b.Origin.X += p.Frame.Origin.X
		b.Origin.Y += p.Frame.Origin.Y
	}
	return b
}

func (v *View) within(o *View) bool {
	for p := v; p != nil; p = p.Parent {
		if p == o {
			return true
		}
	}
	return false
}

// Type replaces the text of the field selector matches, as a user's edit.
func (a *App) Type(selector, text string) {
	a.t.Helper()
	v := a.Find(selector)
	if v.Kind != core.KindInput {
		a.t.Fatalf("nsgotest: %q is not a text field", selector)
	}
	if v.Input.Readonly || v.Input.Disabled {
		a.t.Fatalf("nsgotest: %q does not take text", selector)
	}
	a.edits[v.ID]++
	v.Text = text
	a.dispatch(core.Event{Kind: core.EventChangeText, ID: v.ID, Text: text, Count: a.edits[v.ID]})
}

// Submit presses Return in the field selector matches.
func (a *App) Submit(selector string) {
	a.t.Helper()
	v := a.Find(selector)
	a.dispatch(core.Event{Kind: core.EventSubmitEditing, ID: v.ID, Text: v.Text})
}

// Load reports that the image selector matches loaded at pixels.
func (a *App) Load(selector string, pixels core.Size) {
	a.t.Helper()
	v := a.Find(selector)
	for _, k := range []core.EventKind{core.EventLoadStart, core.EventLoad, core.EventLoadEnd} {
		if v.Events.Has(k) {
			a.dispatch(core.Event{Kind: k, ID: v.ID, Size: pixels})
		}
	}
}

// Scroll scrolls the view selector matches to offset.
func (a *App) Scroll(selector string, offset core.Point) {
	a.t.Helper()
	v := a.Find(selector)
	v.Offset = offset
	a.dispatch(core.Event{Kind: core.EventScroll, ID: v.ID, Point: offset})
}

func (a *App) dispatch(ev core.Event) {
	a.tree.Dispatch(ev)
	a.Settle()
}

func (a *App) apply(b *core.MountBatch) {
	a.rootID = b.Root
	for _, it := range b.Items {
		v := a.views[it.ID]
		switch it.Kind {
		case core.ItemCreate:
			a.views[it.ID] = &View{ID: it.ID, Kind: it.View, Style: core.ViewStyle{Opacity: 1}}
		case core.ItemDelete:
			if v != nil {
				v.detach()
			}
			delete(a.views, it.ID)
			delete(a.edits, it.ID)
		case core.ItemRemove:
			v.detach()
		case core.ItemInsert:
			v.detach()
			p := a.views[it.Parent]
			at := len(p.Children)
			for i, c := range p.Children {
				if c.ID == it.Before {
					at = i
				}
			}
			p.Children = append(p.Children[:at], append([]*View{v}, p.Children[at:]...)...)
			v.Parent = p
		case core.ItemFrame:
			v.Frame = it.Frame
		case core.ItemViewStyle:
			v.Style = *it.ViewStyle
		case core.ItemViewProps:
			v.Props = *it.ViewProps
		case core.ItemEvents:
			v.Events = it.Events
		case core.ItemText:
			v.Text = it.Text.Text
		case core.ItemInput:
			v.Input = *it.Input
		case core.ItemInputValue:
			if a.edits[it.ID] <= it.Count {
				v.Text = it.Value
			}
		case core.ItemScrollContent:
			v.Content = it.Size
		case core.ItemImage:
			v.Image = *it.Image
		case core.ItemFocus:
			for _, o := range a.views {
				o.Focused = false
			}
			v.Focused = true
		case core.ItemBlur:
			v.Focused = false
		case core.ItemScrollTo:
			v.Offset = it.Point
			// A platform reports where a scroll lands.
			a.mu.Lock()
			a.posts = append(a.posts, func() {
				a.tree.Dispatch(core.Event{Kind: core.EventScroll, ID: it.ID, Point: it.Point})
			})
			a.mu.Unlock()
		case core.ItemSetClipboard:
			a.Clipboard = it.Value
		case core.ItemReadClipboard:
			id, text := it.Count, a.Clipboard
			a.mu.Lock()
			a.posts = append(a.posts, func() {
				a.tree.Dispatch(core.Event{Kind: core.EventClipboard, Count: id, Text: text})
			})
			a.mu.Unlock()
		case core.ItemKeymap:
			a.Keymap = it.Keys
		case core.ItemTitle:
			a.Title = it.Value
		case core.ItemMenu:
			a.Menu = it.Menu
		case core.ItemWindow:
			a.Window = append(a.Window, *it.Window)
		case core.ItemAnnounce:
			a.Announced = append(a.Announced, it.Value)
		case core.ItemAlert:
			a.alerts = append(a.alerts, *it.Alert)
		case core.ItemDismissKeyboard:
			for _, o := range a.views {
				o.Focused = false
			}
		}
	}
}

func (v *View) detach() {
	if p := v.Parent; p != nil {
		for i, c := range p.Children {
			if c == v {
				p.Children = append(p.Children[:i], p.Children[i+1:]...)
				break
			}
		}
		v.Parent = nil
	}
}

// Dump returns the visible views as an indented outline, for failure
// messages and golden files.
func (a *App) Dump() string {
	var b strings.Builder
	var walk func(*View, int)
	walk = func(v *View, depth int) {
		if v.Style.Hidden {
			return
		}
		f := v.Frame
		fmt.Fprintf(&b, "%s%s %g,%g %gx%g", strings.Repeat("  ", depth), kindName(v.Kind),
			f.Origin.X, f.Origin.Y, f.Size.Width, f.Size.Height)
		if r := roleName(v.Props.Role); r != "" && r != "text" && r != "textbox" {
			fmt.Fprintf(&b, " role=%s", r)
		}
		if v.Props.TestID != "" {
			fmt.Fprintf(&b, " id=%s", v.Props.TestID)
		}
		if v.Kind == core.KindText || v.Kind == core.KindInput {
			fmt.Fprintf(&b, " %q", v.Text)
		}
		b.WriteByte('\n')
		for _, c := range v.Children {
			walk(c, depth+1)
		}
	}
	if r := a.views[a.rootID]; r != nil {
		walk(r, 0)
	}
	return b.String()
}

func kindName(k core.ViewKind) string {
	switch k {
	case core.KindView:
		return "view"
	case core.KindText:
		return "text"
	case core.KindInput:
		return "input"
	case core.KindScroll:
		return "scroll"
	case core.KindImage:
		return "image"
	case core.KindModal:
		return "modal"
	}
	return "?"
}
