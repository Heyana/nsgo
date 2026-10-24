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
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/internal/driver"
)

// Option configures Start.
type Option func(*App)

// Size sets the surface size; 390 by 844 points by default.
func Size(width, height driver.Points) Option {
	return func(a *App) { a.size = driver.Size{Width: width, Height: height} }
}

// Storage starts the app with entries in its storage, as if kept from an
// earlier run; it starts empty otherwise.
func Storage(entries map[string]string) Option {
	return func(a *App) { a.store = core.NewStore(seed(entries)) }
}

// Environment starts the app in env, as a platform reports it before the
// app mounts: screens have the system header where env has HeaderHeight.
func Environment(env driver.Environment) Option {
	return func(a *App) { a.env = &env }
}

// seed is a storage backend that loads entries and keeps nothing.
type seed map[string]string

func (s seed) Load() (map[string]string, error) { return s, nil }
func (seed) Write(driver.StorageChange)         {}

// App is an app running headless. Its methods run on the test's
// goroutine; the app's goroutines reach it through Scope.Post, which App
// runs when it settles.
type App struct {
	t    testing.TB
	size driver.Size
	// Clipboard is the clipboard's text; Announced what screen readers
	// were asked to say.
	Clipboard string
	Announced []string
	// AccessibilityFocus is the view the app last moved the screen
	// reader's focus to.
	AccessibilityFocus *View
	// Keymap is the keyboard shortcuts the app binds; Title its title.
	Keymap []string
	Title  string
	// Menu is the application menu the app set; Window the window
	// commands it sent, oldest first.
	Menu   []driver.MenuEntry
	Window []driver.WindowCommand
	// ImageSizes are the pixel sizes of the images the model's platform
	// loads, by URI, which answer Scope.ImageSize; others fail to load.
	ImageSizes map[string]driver.Size
	tree       *core.Tree
	root       *core.Scope
	store      *core.Store
	Clock      *core.ManualClock

	// StatusBar is the status bar the app last set.
	StatusBar driver.StatusBarConfig

	mu    sync.Mutex
	posts []func()
	// frameWanted is set while the graph waits for a display frame.
	frameWanted bool

	views  map[driver.NodeID]*View
	anim   *driver.Animator
	start  time.Time
	alerts []driver.AlertConfig
	rootID driver.NodeID
	edits  map[driver.NodeID]uint32
	env    *driver.Environment
}

// View is the model of one native view, as the last batch left it.
type View struct {
	ID       driver.NodeID
	Kind     driver.ViewKind
	Parent   *View
	Children []*View
	Frame    driver.Bounds
	Style    driver.ViewStyle
	Props    driver.ViewProps
	Events   driver.EventMask
	Text     string // a text view's text or a field's value
	Input    driver.InputConfig
	Content  driver.Size  // a scroll view's content
	Offset   driver.Point // and its offset
	// Applied counts the ItemScrollTo and ItemScrollBy items the scroll
	// view applied, which its reports carry; Sticky are its sticky
	// cells.
	Applied uint32
	Sticky  []driver.StickyCell
	// Scroll is a scroll view's configuration, and ZoomScale its zoom
	// (0 until it zooms).
	Scroll    driver.ScrollConfig
	ZoomScale float32
	Image     driver.ImageConfig
	Web       driver.WebConfig
	// WebCommands are what the app asked a web view to do, oldest first.
	WebCommands []WebCommand
	Video       driver.VideoConfig
	// Seeks are the positions the app asked a video to jump to, oldest
	// first.
	Seeks []time.Duration
	// Component and NativeProps are a native view's component and props;
	// NativeCommands what Ref.Command sent it, oldest first.
	Component      string
	NativeProps    map[string]any
	NativeCommands []NativeCommand
	// Animated are the values of the animated properties bound to the
	// view, which the model's animator steps with the clock.
	Animated driver.AnimatedValues
	Focused  bool
	// Selection is a field's selection, as the app last set it or Select
	// moved it, in UTF-16 units.
	Selection driver.TextRange
	// Composing is the text an input method composes in a field (Compose);
	// held is a value the app set meanwhile, applied when it ends.
	Composing string
	held      *driver.MountItem
	// Screen is a screen's system header, sent where the environment
	// has HeaderHeight; Modal a modal's config.
	Screen driver.ScreenConfig
	Modal  driver.ModalConfig
}

// clamp keeps a scroll view's offset within its content and insets, as
// platforms do.
func (v *View) clamp(p driver.Point) driver.Point {
	in := v.Scroll.Behavior.ContentInset
	p.X = min(max(p.X, -in.Left), max(v.Content.Width-v.Frame.Size.Width, 0)+in.Right)
	p.Y = min(max(p.Y, -in.Top), max(v.Content.Height-v.Frame.Size.Height, 0)+in.Bottom)
	return p
}

// StickyShift is how far the sticky cell id of scroll view v moves from
// its frame at v's offset, as a platform moves it.
func (v *View) StickyShift(id driver.NodeID) driver.Points {
	for _, c := range v.Sticky {
		if c.Cell == id {
			if v.Scroll.Horizontal {
				return c.StickyShift(v.Offset.X)
			}
			return c.StickyShift(v.Offset.Y)
		}
	}
	return 0
}

// NativeCommand is a command a native view received.
type NativeCommand struct {
	Name string
	Args any
}

// WebCommand is a command a web view received, with its message or script.
type WebCommand struct {
	Command driver.WebCommand
	Value   string
}

// Start runs app and settles it; the test's cleanup disposes it.
func Start(t testing.TB, app func(cx *core.Scope) core.Node, opts ...Option) *App {
	t.Helper()
	a := &App{
		t:     t,
		size:  driver.Size{Width: 390, Height: 844},
		Clock: core.NewManualClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		views: map[driver.NodeID]*View{},
		edits: map[driver.NodeID]uint32{},
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
		// Frames run as the clock advances, as a platform's display does.
		RequestFrame: func() {
			a.mu.Lock()
			a.frameWanted = true
			a.mu.Unlock()
		},
		Storage: a.store,
	})
	t.Cleanup(a.root.Dispose)
	a.start = a.Clock.Now()
	a.anim = driver.NewAnimator(func(ev driver.Event) {
		a.mu.Lock()
		a.posts = append(a.posts, func() { a.tree.Dispatch(ev) })
		a.mu.Unlock()
	})
	a.tree = core.NewTree(a.root, core.ApproxTextEngine{})
	a.tree.Resize(a.size)
	if a.env != nil {
		a.tree.SetEnvironment(*a.env)
	}
	a.tree.Mount(app)
	a.Settle()
	return a
}

// Storage returns the app's storage, to read what it kept or change it as
// another of the app's windows would.
func (a *App) Storage() core.Storage { return a.root.Storage() }

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
		a.animate()
		a.mu.Lock()
		more := len(a.posts) > 0
		a.mu.Unlock()
		if b == nil && len(posts) == 0 && !more {
			return
		}
	}
}

// Advance moves the clock by d, runs the timers and display frames that
// come due and settles. A frame runs every FrameInterval while the app
// waits for one, as a display's frames do.
func (a *App) Advance(d time.Duration) {
	for d > 0 {
		step := min(d, driver.FrameInterval)
		a.Clock.Advance(step)
		d -= step
		a.runFrame()
	}
	a.Settle()
}

// runFrame runs the display frame the app waits for, if it waits for one.
func (a *App) runFrame() {
	a.mu.Lock()
	wanted := a.frameWanted
	a.frameWanted = false
	a.mu.Unlock()
	if wanted {
		a.root.RunFrame()
	}
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
func (a *App) SetEnvironment(env driver.Environment) {
	a.tree.SetEnvironment(env)
	a.Settle()
}

// Resize sets the surface size and settles.
func (a *App) Resize(width, height driver.Points) {
	a.tree.Resize(driver.Size{Width: width, Height: height})
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

// Query returns the visible views selector matches, in tree order, then
// in the bar above the keyboard while a field with an
// InputAccessoryView has focus.
func (a *App) Query(selector string) []*View {
	a.t.Helper()
	scope := []*View{a.views[a.rootID]}
	if scope[0] == nil {
		return nil
	}
	for _, v := range a.views {
		if bar := a.views[v.Input.Accessory]; v.Focused && bar != nil && v.Input.Accessory != 0 {
			scope = append(scope, bar)
		}
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
		return v.Kind == driver.KindText && strings.Contains(v.Text, arg)
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
		if v.Kind == driver.KindText {
			b.WriteString(v.Text)
		}
		for _, c := range v.Children {
			walk(c)
		}
	}
	walk(v)
	return b.String()
}

var roles = map[driver.Role]string{
	driver.RoleButton: "button", driver.RoleLink: "link", driver.RoleHeader: "header", driver.RoleImage: "image",
	driver.RoleCheckbox: "checkbox", driver.RoleSwitch: "switch", driver.RoleRadio: "radio", driver.RoleTab: "tab",
	driver.RoleText: "text", driver.RoleTextField: "textbox", driver.RoleAdjustable: "adjustable",
	driver.RoleProgressBar: "progressbar",
}

func roleName(r driver.Role) string { return roles[r] }

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
		if p.Events.Has(driver.EventPress) || p.Events.Has(driver.EventPressDown) {
			a.press(p, driver.EventPress)
			return
		}
	}
	a.t.Fatalf("nsgotest: nothing pressable at %q", selector)
}

// press sends what a tap on v's center sends, one click with no
// modifiers: presses outside to the overlays that listen and do not hold
// v, then down, up and the press itself.
func (a *App) press(v *View, kind driver.EventKind) {
	local, center := v.center()
	var outside []driver.NodeID
	walk(a.views[a.rootID], false, func(o *View) {
		if o.Events.Has(driver.EventPressOutside) && !v.within(o) {
			outside = append(outside, o.ID)
		}
	})
	for _, id := range outside {
		a.dispatch(driver.Event{Kind: driver.EventPressOutside, ID: id, Point: center})
	}
	for _, k := range []driver.EventKind{driver.EventPressDown, driver.EventPressUp, kind} {
		if v.Events.Has(k) {
			a.dispatch(driver.Event{Kind: k, ID: v.ID, Point: local, Page: center, Count: 1})
		}
	}
}

// center returns the middle of v in its own and in surface points.
func (v *View) center() (local, page driver.Point) {
	at := v.surfaceFrame()
	local = driver.Point{X: at.Size.Width / 2, Y: at.Size.Height / 2}
	return local, driver.Point{X: at.Origin.X + local.X, Y: at.Origin.Y + local.Y}
}

// PressDown starts a press on the view selector matches, or the pressable
// view around it, and holds it until PressUp.
func (a *App) PressDown(selector string) { a.t.Helper(); a.half(selector, driver.EventPressDown) }

// PressUp ends a press PressDown started.
func (a *App) PressUp(selector string) { a.t.Helper(); a.half(selector, driver.EventPressUp) }

func (a *App) half(selector string, kind driver.EventKind) {
	a.t.Helper()
	for p := a.Find(selector); p != nil; p = p.Parent {
		if p.Events.Has(kind) {
			ev := driver.Event{Kind: kind, ID: p.ID}
			if kind != driver.EventHoverIn && kind != driver.EventHoverOut {
				ev.Point, ev.Page = p.center()
				if kind != driver.EventContextMenu {
					ev.Count = 1
				}
			}
			a.dispatch(ev)
			return
		}
	}
	a.t.Fatalf("nsgotest: nothing at %q reports %v", selector, kind)
}

// Hover moves the pointer onto (or, with on false, off) the view selector
// matches.
func (a *App) Hover(selector string, on bool) {
	a.t.Helper()
	kind := driver.EventHoverIn
	if !on {
		kind = driver.EventHoverOut
	}
	a.half(selector, kind)
}

// LongPress long-presses the view selector matches, or the pressable view
// around it.
func (a *App) LongPress(selector string) {
	a.t.Helper()
	v := a.Find(selector)
	for p := v; p != nil; p = p.Parent {
		if p.Events.Has(driver.EventLongPress) {
			a.press(p, driver.EventLongPress)
			return
		}
	}
	a.t.Fatalf("nsgotest: nothing long-pressable at %q", selector)
}

// AccessibilityAction performs the accessibility action name on the view
// selector matches, as a screen reader would: an action the view offers,
// or ActionActivate on a pressable view without it, which presses it.
func (a *App) AccessibilityAction(selector, name string) {
	a.t.Helper()
	v := a.Find(selector)
	switch {
	case v.Props.HasAction(name) && v.Events.Has(driver.EventAccessibilityAction):
		a.dispatch(driver.Event{Kind: driver.EventAccessibilityAction, ID: v.ID, Text: name})
	case name == driver.ActionActivate && v.Events.Has(driver.EventPress):
		a.dispatch(driver.Event{Kind: driver.EventPress, ID: v.ID})
	default:
		a.t.Fatalf("nsgotest: %q offers no %q action", selector, name)
	}
}

// Alerts returns the alerts shown and not answered yet, oldest first.
func (a *App) Alerts() []driver.AlertConfig { return a.alerts }

// Answer chooses the button at index (-1: dismiss) of the oldest alert
// shown.
func (a *App) Answer(index int) {
	a.t.Helper()
	if len(a.alerts) == 0 {
		a.t.Fatal("nsgotest: no alert to answer")
	}
	id := a.alerts[0].ID
	a.alerts = a.alerts[1:]
	a.dispatch(driver.Event{Kind: driver.EventAlert, Count: id, Index: index})
}

// RightClick sends a secondary click to the view selector matches, or the
// one around it that listens.
func (a *App) RightClick(selector string) { a.t.Helper(); a.half(selector, driver.EventContextMenu) }

// Key presses a keyboard shortcut, such as "mod+s", if the app binds it.
func (a *App) Key(key string) {
	a.t.Helper()
	k := driver.NormalizeKey(key)
	for _, b := range a.Keymap {
		if b == k {
			a.dispatch(driver.Event{Kind: driver.EventKey, Text: k})
			return
		}
	}
	a.t.Fatalf("nsgotest: no binding for %q in %v", key, a.Keymap)
}

// Pan drags the view selector matches, or the one around it with a pan,
// through each translation in turn, then lets go.
func (a *App) Pan(selector string, translations ...driver.Point) {
	a.t.Helper()
	v := a.gestureView(selector, driver.EventPan)
	a.gesture(v, driver.EventPan, len(translations), func(ev *driver.Event, i int) {
		ev.Point = translations[i]
	})
}

// Drag puts a finger down at the middle of the view selector matches,
// moves it there plus each offset in turn, 16 ms apart, and lifts it, as
// the platform reports touches: OnTouch and the touch responder hear it.
func (a *App) Drag(selector string, offsets ...driver.Point) {
	a.t.Helper()
	_, at := a.Find(selector).center()
	finger := func(phase driver.GesturePhase, p driver.Point, step int) {
		a.dispatch(driver.Event{Kind: driver.EventTouch, Phase: phase, Time: time.Duration(step) * 16 * time.Millisecond,
			Touches: []driver.Touch{{PagePosition: p, Changed: true}}})
	}
	finger(driver.GestureBegan, at, 0)
	p := at
	for i, d := range offsets {
		p = driver.Point{X: at.X + d.X, Y: at.Y + d.Y}
		finger(driver.GestureChanged, p, i+1)
	}
	finger(driver.GestureEnded, p, len(offsets)+1)
}

// Pinch pinches the view selector matches, or the one around it with a
// pinch, through each scale in turn.
func (a *App) Pinch(selector string, scales ...float32) {
	a.t.Helper()
	v := a.gestureView(selector, driver.EventPinch)
	a.gesture(v, driver.EventPinch, len(scales), func(ev *driver.Event, i int) { ev.Value = scales[i] })
}

// Rotate rotates the view selector matches, or the one around it with a
// rotation, through each angle in radians in turn.
func (a *App) Rotate(selector string, angles ...float32) {
	a.t.Helper()
	v := a.gestureView(selector, driver.EventRotation)
	a.gesture(v, driver.EventRotation, len(angles), func(ev *driver.Event, i int) { ev.Value = angles[i] })
}

func (a *App) gestureView(selector string, kind driver.EventKind) *View {
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
func (a *App) gesture(v *View, kind driver.EventKind, n int, step func(*driver.Event, int)) {
	if n == 0 {
		return
	}
	send := func(phase driver.GesturePhase, i int) {
		ev := driver.Event{Kind: kind, ID: v.ID, Phase: phase}
		step(&ev, i)
		// A platform moves bound values before it reports the move.
		a.anim.Gesture(&ev)
		a.dispatch(ev)
	}
	send(driver.GestureBegan, 0)
	for i := 1; i < n; i++ {
		send(driver.GestureChanged, i)
	}
	send(driver.GestureEnded, n-1)
}

// ChooseMenu chooses the application menu item at path, its labels from
// the menu bar down, such as "File", "New".
func (a *App) ChooseMenu(path ...string) {
	a.t.Helper()
	items := a.Menu
	for i, label := range path {
		var found *driver.MenuEntry
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
			a.dispatch(driver.Event{Kind: driver.EventMenu, Count: found.ID})
			return
		}
		items = found.Items
	}
}

// Back presses Escape or the back button.
func (a *App) Back() { a.dispatch(driver.Event{Kind: driver.EventBack, ID: a.rootID}) }

// HeaderBack presses the back button in the system header of the screen
// selector matches, as iOS and Android draw it.
func (a *App) HeaderBack(selector string) {
	a.t.Helper()
	a.dispatch(driver.Event{Kind: driver.EventBack, ID: a.Find(selector).ID})
}

// SwipeClose swipes away the modal around what selector matches, as an
// iOS sheet is: the platform closes it and asks the app to remove it
// ("swipe").
func (a *App) SwipeClose(selector string) {
	a.t.Helper()
	a.dispatch(driver.Event{Kind: driver.EventRequestClose, ID: a.modal(selector).ID, Text: "swipe", Finished: true})
}

// PresentModal reports that the platform presents the modal around what
// selector matches at size, with insets covered, as an iOS sheet does.
func (a *App) PresentModal(selector string, size driver.Size, insets driver.Edges) {
	a.t.Helper()
	a.dispatch(driver.Event{Kind: driver.EventModalSize, ID: a.modal(selector).ID, Size: size, Insets: insets})
}

// modal returns the modal selector matches or holds what it matches.
func (a *App) modal(selector string) *View {
	a.t.Helper()
	v := a.Find(selector)
	for m := v; m != nil; m = m.Parent {
		if m.Kind == driver.KindModal {
			return m
		}
	}
	a.t.Fatalf("nsgotest: %q is in no modal", selector)
	return nil
}

func (v *View) surfaceFrame() driver.Bounds {
	b := v.Frame
	for p := v.Parent; p != nil; p = p.Parent {
		b.Origin.X += p.Frame.Origin.X - p.Offset.X
		b.Origin.Y += p.Frame.Origin.Y - p.Offset.Y
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

// Type replaces the text of the field selector matches, as a user's edit,
// cut to its MaxLength as the platforms cut what the user enters. The
// caret goes to the end.
func (a *App) Type(selector, text string) {
	a.t.Helper()
	v := a.Find(selector)
	if v.Kind != driver.KindInput {
		a.t.Fatalf("nsgotest: %q is not a text field", selector)
	}
	if v.Input.Readonly || v.Input.Disabled {
		a.t.Fatalf("nsgotest: %q does not take text", selector)
	}
	a.edit(v, driver.ClipUTF16(text, v.Input.MaxLength))
}

func (a *App) edit(v *View, text string) {
	a.edits[v.ID]++
	v.Text = text
	a.dispatch(driver.Event{Kind: driver.EventChangeText, ID: v.ID, Text: text, Count: a.edits[v.ID]})
	end := driver.UTF16Len(text)
	a.moveSelection(v, driver.TextRange{Start: end, End: end})
}

// Paste pastes into the field selector matches as the user would: the
// field hears it with OnPaste, then, unless its PasteBehavior is
// PasteReport, the text goes in at its end.
func (a *App) Paste(selector string, p driver.PasteEvent) {
	a.t.Helper()
	v := a.Find(selector)
	if v.Kind != driver.KindInput || v.Input.Readonly || v.Input.Disabled {
		a.t.Fatalf("nsgotest: %q takes no paste", selector)
	}
	if v.Events.Has(driver.EventPaste) {
		a.dispatch(driver.Event{Kind: driver.EventPaste, ID: v.ID, Data: &p})
	}
	if p.Text != "" && v.Input.PasteBehavior == driver.PasteInsert {
		a.edit(v, driver.ClipUTF16(v.Text+p.Text, v.Input.MaxLength))
	}
}

// Select moves the selection of the field selector matches, as the user
// would, in UTF-16 units.
func (a *App) Select(selector string, start, end int) {
	a.t.Helper()
	v := a.Find(selector)
	if v.Kind != driver.KindInput {
		a.t.Fatalf("nsgotest: %q is not a text field", selector)
	}
	a.moveSelection(v, driver.TextRange{Start: start, End: end})
}

func (a *App) moveSelection(v *View, r driver.TextRange) {
	if r == v.Selection {
		return
	}
	v.Selection = r
	if v.Events.Has(driver.EventSelectionChange) {
		a.dispatch(driver.Event{Kind: driver.EventSelectionChange, ID: v.ID, Index: r.Start, Count: uint32(r.End)})
	}
}

// KeyPress types key in the field selector matches, as a key the user
// presses: its text, "Backspace" or "Enter". It reports the key, then
// edits the end of the text as the key would; Enter goes as Submit.
func (a *App) KeyPress(selector, key string) {
	a.t.Helper()
	v := a.Find(selector)
	if v.Events.Has(driver.EventKeyPress) {
		a.dispatch(driver.Event{Kind: driver.EventKeyPress, ID: v.ID, Text: key})
	}
	switch key {
	case "Enter":
		a.Submit(selector)
	case "Backspace":
		if r := []rune(v.Text); len(r) > 0 {
			a.edit(v, string(r[:len(r)-1]))
		}
	default:
		if add, ok := driver.LimitEdit(v.Text, 0, key, v.Input.MaxLength); ok {
			a.edit(v, v.Text+add)
		}
	}
}

// Compose has an input method compose text in the field selector
// matches; "" ends the composition, after the Type of what it commits.
// While it lasts the field holds the values the app sets, as platforms do.
func (a *App) Compose(selector, text string) {
	a.t.Helper()
	v := a.Find(selector)
	if text != "" {
		v.Composing = text
		if v.Events.Has(driver.EventCompositionChange) {
			a.dispatch(driver.Event{Kind: driver.EventCompositionChange, ID: v.ID, Text: text})
		}
		return
	}
	v.Composing = ""
	if v.Events.Has(driver.EventCompositionChange) {
		a.dispatch(driver.Event{Kind: driver.EventCompositionChange, ID: v.ID, Finished: true})
	}
	if it := v.held; it != nil {
		v.held = nil
		if a.edits[v.ID] <= it.Count {
			v.Text = it.Value
		}
	}
}

// Focus focuses the text field selector matches, as a tap on it would.
func (a *App) Focus(selector string) {
	a.t.Helper()
	v := a.Find(selector)
	if v.Kind != driver.KindInput {
		a.t.Fatalf("nsgotest: %q is not a text field", selector)
	}
	for _, o := range a.views {
		if o != v {
			a.setFocus(o, false)
		}
	}
	a.setFocus(v, true)
	a.Settle()
}

// Submit presses Return in the field selector matches, as its
// SubmitBehavior says: a line break at the end of a multiline field's
// text, else OnSubmit, and the focus leaves it unless it keeps it.
func (a *App) Submit(selector string) {
	a.t.Helper()
	v := a.Find(selector)
	switch v.Input.Submit() {
	case driver.SubmitBehaviorNewline:
		if add, ok := driver.LimitEdit(v.Text, 0, "\n", v.Input.MaxLength); ok && !v.Input.Readonly {
			a.edit(v, v.Text+add)
		}
		return
	case driver.SubmitBehaviorBlurAndSubmit:
		a.dispatch(driver.Event{Kind: driver.EventSubmitEditing, ID: v.ID, Text: v.Text})
		a.setFocus(v, false)
		a.Settle()
		return
	}
	a.dispatch(driver.Event{Kind: driver.EventSubmitEditing, ID: v.ID, Text: v.Text})
}

// Load reports that the image selector matches loaded at pixels.
func (a *App) Load(selector string, pixels driver.Size) {
	a.t.Helper()
	v := a.Find(selector)
	for _, k := range []driver.EventKind{driver.EventLoadStart, driver.EventLoad, driver.EventLoadEnd} {
		if v.Events.Has(k) {
			a.dispatch(driver.Event{Kind: k, ID: v.ID, Size: pixels})
		}
	}
}

// Play reports that the video selector matches opened and loaded
// info, as a platform does: it starts loading, then loads.
func (a *App) Play(selector string, info core.VideoInfo) {
	a.t.Helper()
	v := a.Find(selector)
	for _, k := range []driver.EventKind{driver.EventLoadStart, driver.EventLoad} {
		if v.Events.Has(k) {
			a.dispatch(driver.Event{Kind: k, ID: v.ID, Duration: info.Duration, Size: info.Size})
		}
	}
}

// Progress reports that the video selector matches played to position:
// its progress, then its end when position reaches duration.
func (a *App) Progress(selector string, position, duration time.Duration) {
	a.t.Helper()
	v := a.Find(selector)
	if v.Events.Has(driver.EventProgress) {
		a.dispatch(driver.Event{Kind: driver.EventProgress, ID: v.ID, Time: position, Duration: duration})
	}
	if position >= duration && v.Events.Has(driver.EventEnd) {
		a.dispatch(driver.Event{Kind: driver.EventEnd, ID: v.ID})
	}
}

// Navigate reports that the web view selector matches loaded the page
// nav describes, as a platform does: it starts loading, then loads.
func (a *App) Navigate(selector string, nav driver.WebNavigation) {
	a.t.Helper()
	v := a.Find(selector)
	for _, loading := range []bool{true, false} {
		nav.Loading = loading
		kind := driver.EventLoad
		if loading {
			kind = driver.EventLoadStart
		}
		for _, k := range []driver.EventKind{kind, driver.EventNavigationStateChange} {
			if v.Events.Has(k) {
				n := nav
				a.dispatch(driver.Event{Kind: k, ID: v.ID, Navigation: &n})
			}
		}
	}
}

// PostMessage reports that the page in the web view selector matches
// posted data.
func (a *App) PostMessage(selector, data string) {
	a.t.Helper()
	v := a.Find(selector)
	if v.Events.Has(driver.EventMessage) {
		a.dispatch(driver.Event{Kind: driver.EventMessage, ID: v.ID, Text: data})
	}
}

// Scroll scrolls the view selector matches to offset.
func (a *App) Scroll(selector string, offset driver.Point) {
	a.t.Helper()
	v := a.Find(selector)
	v.Offset = offset
	a.anim.Scroll(v.ID, offset)
	a.dispatch(driver.Event{Kind: driver.EventScroll, ID: v.ID, Point: offset, Count: v.Applied})
}

// DragScroll drags the scroll view selector matches to offset and lets
// go at velocity (points per second along its axis, positive as the
// offset grows), as a platform does: the drag's events, then the glide or
// the snap its ScrollBehavior decides, clamped to its content and insets.
func (a *App) DragScroll(selector string, offset driver.Point, velocity float32) {
	a.t.Helper()
	v := a.Find(selector)
	if v.Scroll.Behavior.NoScroll {
		return
	}
	a.scrollEvent(v, driver.EventScrollDragStart)
	a.Scroll(selector, offset)
	a.scrollEvent(v, driver.EventScrollDragEnd)
	b := &v.Scroll.Behavior
	main := func(p driver.Point) driver.Points {
		if v.Scroll.Horizontal {
			return p.X
		}
		return p.Y
	}
	viewport, content := v.Frame.Size.Height, v.Content.Height
	lo, hi := -b.ContentInset.Top, b.ContentInset.Bottom
	if v.Scroll.Horizontal {
		viewport, content = v.Frame.Size.Width, v.Content.Width
		lo, hi = -b.ContentInset.Left, b.ContentInset.Right
	}
	at, ms := main(offset), velocity/1000
	to := driver.Projected(at, ms, b.Deceleration())
	if snap, ok := b.SnapTarget(at, to, ms, viewport, content); ok {
		to = snap
	} else if ms == 0 {
		return
	}
	to = min(max(to, lo), max(content-viewport, 0)+hi)
	if to == at {
		return
	}
	a.scrollEvent(v, driver.EventMomentumStart)
	end := offset
	if v.Scroll.Horizontal {
		end.X = to
	} else {
		end.Y = to
	}
	a.Scroll(selector, end)
	a.scrollEvent(v, driver.EventMomentumEnd)
}

// PullToRefresh pulls the refreshable scroll view selector matches down
// past its top, as a user does to refresh it.
func (a *App) PullToRefresh(selector string) {
	a.t.Helper()
	v := a.Find(selector)
	if !v.Scroll.Refreshable || v.Scroll.Refreshing || v.Scroll.Horizontal || v.Scroll.Behavior.NoScroll {
		return
	}
	a.scrollEvent(v, driver.EventScrollDragStart)
	a.dispatch(driver.Event{Kind: driver.EventRefresh, ID: v.ID, Point: v.Offset, Count: v.Applied})
	a.scrollEvent(v, driver.EventScrollDragEnd)
}

// PinchZoom zooms the zoomable scroll view selector matches to scale, as a
// user's pinch does, within its limits.
func (a *App) PinchZoom(selector string, scale float32) {
	a.t.Helper()
	v := a.Find(selector)
	c := &v.Scroll
	if !c.Zooms() || c.NoPinchZoom {
		return
	}
	v.ZoomScale = min(max(scale, c.MinZoomScale), c.MaxZoomScale)
	a.dispatch(driver.Event{Kind: driver.EventScroll, ID: v.ID, Point: v.Offset, Value: v.ZoomScale, Count: v.Applied})
}

func (a *App) scrollEvent(v *View, kind driver.EventKind) {
	if v.Events.Has(kind) {
		a.dispatch(driver.Event{Kind: kind, ID: v.ID, Point: v.Offset, Value: v.ZoomScale, Count: v.Applied})
	}
}

// DropFiles drops files on the view selector matches, as a desktop or
// the web does: the drag enters, then the files drop.
func (a *App) DropFiles(selector string, uris ...string) {
	a.t.Helper()
	v := a.Find(selector)
	if v.Events.Has(driver.EventFileDragEnter) {
		a.dispatch(driver.Event{Kind: driver.EventFileDragEnter, ID: v.ID})
	}
	a.dispatch(driver.Event{Kind: driver.EventFileDrop, ID: v.ID, URIs: uris})
}

// Emit reports that the component of the native view selector matches
// emitted name with data.
func (a *App) Emit(selector, name string, data any) {
	a.t.Helper()
	v := a.Find(selector)
	a.dispatch(driver.Event{Kind: driver.EventNative, ID: v.ID, Text: name, Data: data})
}

// animate steps the animator to the clock and records the values it
// moved.
func (a *App) animate() {
	if a.anim.Running() {
		a.anim.Step(a.Clock.Now().Sub(a.start))
	}
	a.anim.Changed(func(id driver.NodeID, vals driver.AnimatedValues) {
		if v := a.views[id]; v != nil {
			v.Animated = vals
		}
	})
}

func (a *App) dispatch(ev driver.Event) {
	a.tree.Dispatch(ev)
	a.Settle()
}

func (a *App) apply(b *driver.MountBatch) {
	a.rootID = b.Root
	for _, it := range b.Items {
		v := a.views[it.ID]
		switch it.Kind {
		case driver.ItemCreate:
			a.views[it.ID] = &View{ID: it.ID, Kind: it.View, Style: driver.ViewStyle{Opacity: 1}}
		case driver.ItemDelete:
			if v != nil {
				v.detach()
			}
			a.anim.Forget(it.ID)
			delete(a.views, it.ID)
			delete(a.edits, it.ID)
		case driver.ItemRemove:
			v.detach()
		case driver.ItemInsert:
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
		case driver.ItemFrame:
			v.Frame = it.Frame
		case driver.ItemViewStyle:
			v.Style = *it.ViewStyle
		case driver.ItemViewProps:
			v.Props = *it.ViewProps
		case driver.ItemAccessibilityLabel:
			v.Props.Label = it.Value
		case driver.ItemAccessibilityFocus:
			a.AccessibilityFocus = v
		case driver.ItemEvents:
			v.Events = it.Events
		case driver.ItemText:
			v.Text = it.Text.Text
		case driver.ItemInput:
			v.Input = *it.Input
		case driver.ItemInputValue:
			if v.Composing != "" {
				held := it
				v.held = &held
			} else if a.edits[it.ID] <= it.Count {
				v.Text = it.Value
			}
		case driver.ItemSelection:
			n := driver.UTF16Len(v.Text)
			r := it.Selection
			v.Selection = driver.TextRange{Start: min(max(r.Start, 0), n), End: min(max(r.End, 0), n)}
		case driver.ItemScrollContent:
			v.Content = it.Size
		case driver.ItemScrollConfig:
			if !v.Scroll.Zooms() && it.Scroll.Zooms() || it.Scroll.ZoomScale != v.Scroll.ZoomScale {
				v.ZoomScale = it.Scroll.ZoomScale
			}
			v.Scroll = *it.Scroll
		case driver.ItemZoomTo:
			v.ZoomScale = it.Zoom.Scale
			id, at, z := it.ID, v.Offset, v.ZoomScale
			a.mu.Lock()
			a.posts = append(a.posts, func() {
				a.tree.Dispatch(driver.Event{Kind: driver.EventScroll, ID: id, Point: at, Value: z})
			})
			a.mu.Unlock()
		case driver.ItemImage:
			v.Image = *it.Image
		case driver.ItemWeb:
			v.Web = *it.Web
		case driver.ItemWebCommand:
			v.WebCommands = append(v.WebCommands, WebCommand{it.WebCommand, it.Value})
		case driver.ItemVideo:
			v.Video = *it.Video
		case driver.ItemVideoSeek:
			v.Seeks = append(v.Seeks, it.Time)
		case driver.ItemNative:
			v.Component = it.Native.Component
			if v.NativeProps == nil {
				v.NativeProps = map[string]any{}
			}
			for k, p := range it.Native.Props {
				if p == nil {
					delete(v.NativeProps, k)
				} else {
					v.NativeProps[k] = p
				}
			}
		case driver.ItemNativeCommand:
			v.NativeCommands = append(v.NativeCommands, NativeCommand{it.Value, it.Data})
		case driver.ItemAnimation:
			a.anim.Apply(it.Animation)
		case driver.ItemFocus:
			for _, o := range a.views {
				if o != v {
					a.setFocus(o, false)
				}
			}
			a.setFocus(v, true)
		case driver.ItemBlur:
			a.setFocus(v, false)
		case driver.ItemScrollTo, driver.ItemScrollBy:
			v.Applied++
			if it.Kind == driver.ItemScrollBy {
				v.Offset.X += it.Point.X
				v.Offset.Y += it.Point.Y
			} else {
				v.Offset = v.clamp(it.Point)
			}
			// A platform reports where a scroll lands.
			id, at, n := it.ID, v.Offset, v.Applied
			a.mu.Lock()
			a.posts = append(a.posts, func() {
				a.tree.Dispatch(driver.Event{Kind: driver.EventScroll, ID: id, Point: at, Count: n})
			})
			a.mu.Unlock()
		case driver.ItemStickyCells:
			v.Sticky = it.Sticky
		case driver.ItemSetClipboard:
			a.Clipboard = it.Value
		case driver.ItemReadClipboard:
			id, text := it.Count, a.Clipboard
			a.mu.Lock()
			a.posts = append(a.posts, func() {
				a.tree.Dispatch(driver.Event{Kind: driver.EventClipboard, Count: id, Text: text})
			})
			a.mu.Unlock()
		case driver.ItemImageInfo:
			ev := driver.Event{Kind: driver.EventImageInfo, Count: it.Count}
			if size, ok := a.ImageSizes[it.Image.Source.URI]; ok {
				ev.Size = size
			} else {
				ev.Text = "nsgotest: no image at " + it.Image.Source.URI
			}
			a.mu.Lock()
			a.posts = append(a.posts, func() { a.tree.Dispatch(ev) })
			a.mu.Unlock()
		case driver.ItemKeymap:
			a.Keymap = a.Keymap[:0]
			for _, b := range it.Keys {
				if !slices.Contains(a.Keymap, b.Key) {
					a.Keymap = append(a.Keymap, b.Key)
				}
			}
		case driver.ItemTitle:
			a.Title = it.Value
		case driver.ItemStatusBar:
			a.StatusBar = *it.StatusBar
		case driver.ItemScreen:
			v.Screen = *it.Screen
		case driver.ItemModal:
			v.Modal = *it.Modal
		case driver.ItemMenu:
			a.Menu = it.Menu
		case driver.ItemWindow:
			a.Window = append(a.Window, *it.Window)
		case driver.ItemAnnounce:
			a.Announced = append(a.Announced, it.Value)
		case driver.ItemAlert:
			a.alerts = append(a.alerts, *it.Alert)
		case driver.ItemDismissKeyboard:
			for _, o := range a.views {
				a.setFocus(o, false)
			}
		}
	}
}

// setFocus moves focus to or from v and, as a platform does, reports the
// change to the app.
func (a *App) setFocus(v *View, on bool) {
	if v.Focused == on {
		return
	}
	v.Focused = on
	kind := driver.EventBlur
	if on {
		kind = driver.EventFocus
	}
	a.mu.Lock()
	a.posts = append(a.posts, func() { a.tree.Dispatch(driver.Event{Kind: kind, ID: v.ID}) })
	a.mu.Unlock()
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
		if v.Kind == driver.KindText || v.Kind == driver.KindInput {
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

func kindName(k driver.ViewKind) string {
	switch k {
	case driver.KindView:
		return "view"
	case driver.KindText:
		return "text"
	case driver.KindInput:
		return "input"
	case driver.KindScroll:
		return "scroll"
	case driver.KindImage:
		return "image"
	case driver.KindModal:
		return "modal"
	case driver.KindCanvas:
		return "canvas"
	case driver.KindWeb:
		return "web"
	case driver.KindVideo:
		return "video"
	}
	return "?"
}
