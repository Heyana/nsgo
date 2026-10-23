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

// Storage starts the app with entries in its storage, as if kept from an
// earlier run; it starts empty otherwise.
func Storage(entries map[string]string) Option {
	return func(a *App) { a.store = core.NewStore(seed(entries)) }
}

// Environment starts the app in env, as a platform reports it before the
// app mounts: screens have the system header where env has HeaderHeight.
func Environment(env core.Environment) Option {
	return func(a *App) { a.env = &env }
}

// seed is a storage backend that loads entries and keeps nothing.
type seed map[string]string

func (s seed) Load() (map[string]string, error) { return s, nil }
func (seed) Write(core.StorageChange)           {}

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
	// AccessibilityFocus is the view the app last moved the screen
	// reader's focus to.
	AccessibilityFocus *View
	// Keymap is the keyboard shortcuts the app binds; Title its title.
	Keymap []string
	Title  string
	// Menu is the application menu the app set; Window the window
	// commands it sent, oldest first.
	Menu   []core.MenuEntry
	Window []core.WindowCommand
	// ImageSizes are the pixel sizes of the images the model's platform
	// loads, by URI, which answer Scope.ImageSize; others fail to load.
	ImageSizes map[string]core.Size
	tree       *core.Tree
	root       *core.Scope
	store      *core.Store
	Clock      *core.ManualClock

	// StatusBar is the status bar the app last set.
	StatusBar core.StatusBarConfig

	mu    sync.Mutex
	posts []func()
	// frameWanted is set while the graph waits for a display frame.
	frameWanted bool

	views  map[core.NodeID]*View
	anim   *core.Animator
	start  time.Time
	alerts []core.AlertConfig
	rootID core.NodeID
	edits  map[core.NodeID]uint32
	env    *core.Environment
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
	// Applied counts the ItemScrollTo and ItemScrollBy items the scroll
	// view applied, which its reports carry; Sticky are its sticky
	// cells.
	Applied uint32
	Sticky  []core.StickyCell
	// Scroll is a scroll view's configuration, and ZoomScale its zoom
	// (0 until it zooms).
	Scroll    core.ScrollConfig
	ZoomScale float32
	Image     core.ImageConfig
	Web       core.WebConfig
	// WebCommands are what the app asked a web view to do, oldest first.
	WebCommands []WebCommand
	Video       core.VideoConfig
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
	Animated core.AnimatedValues
	Focused  bool
	// Selection is a field's selection, as the app last set it or Select
	// moved it, in UTF-16 units.
	Selection core.TextRange
	// Composing is the text an input method composes in a field (Compose);
	// held is a value the app set meanwhile, applied when it ends.
	Composing string
	held      *core.MountItem
	// Screen is a screen's system header, sent where the environment
	// has HeaderHeight; Modal a modal's config.
	Screen core.ScreenConfig
	Modal  core.ModalConfig
}

// clamp keeps a scroll view's offset within its content and insets, as
// platforms do.
func (v *View) clamp(p core.Point) core.Point {
	in := v.Scroll.Behavior.ContentInset
	p.X = min(max(p.X, -in.Left), max(v.Content.Width-v.Frame.Size.Width, 0)+in.Right)
	p.Y = min(max(p.Y, -in.Top), max(v.Content.Height-v.Frame.Size.Height, 0)+in.Bottom)
	return p
}

// StickyShift is how far the sticky cell id of scroll view v moves from
// its frame at v's offset, as a platform moves it.
func (v *View) StickyShift(id core.NodeID) core.Points {
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
	Command core.WebCommand
	Value   string
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
	a.anim = core.NewAnimator(func(ev core.Event) {
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
		step := min(d, core.FrameInterval)
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
	core.RoleText: "text", core.RoleTextField: "textbox", core.RoleAdjustable: "adjustable",
	core.RoleProgressBar: "progressbar",
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

// AccessibilityAction performs the accessibility action name on the view
// selector matches, as a screen reader would: an action the view offers,
// or ActionActivate on a pressable view without it, which presses it.
func (a *App) AccessibilityAction(selector, name string) {
	a.t.Helper()
	v := a.Find(selector)
	switch {
	case v.Props.HasAction(name) && v.Events.Has(core.EventAccessibilityAction):
		a.dispatch(core.Event{Kind: core.EventAccessibilityAction, ID: v.ID, Text: name})
	case name == core.ActionActivate && v.Events.Has(core.EventPress):
		a.dispatch(core.Event{Kind: core.EventPress, ID: v.ID})
	default:
		a.t.Fatalf("nsgotest: %q offers no %q action", selector, name)
	}
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
		if kind == core.EventPan {
			// A platform moves bound values before it reports the move.
			a.anim.Pan(v.ID, ev.Point)
		}
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

// HeaderBack presses the back button in the system header of the screen
// selector matches, as iOS and Android draw it.
func (a *App) HeaderBack(selector string) {
	a.t.Helper()
	a.dispatch(core.Event{Kind: core.EventBack, ID: a.Find(selector).ID})
}

// SwipeClose swipes away the modal around what selector matches, as an
// iOS sheet is: the platform closes it and asks the app to remove it
// ("swipe").
func (a *App) SwipeClose(selector string) {
	a.t.Helper()
	a.dispatch(core.Event{Kind: core.EventRequestClose, ID: a.modal(selector).ID, Text: "swipe", Finished: true})
}

// PresentModal reports that the platform presents the modal around what
// selector matches at size, with insets covered, as an iOS sheet does.
func (a *App) PresentModal(selector string, size core.Size, insets core.Edges) {
	a.t.Helper()
	a.dispatch(core.Event{Kind: core.EventModalSize, ID: a.modal(selector).ID, Size: size, Insets: insets})
}

// modal returns the modal selector matches or holds what it matches.
func (a *App) modal(selector string) *View {
	a.t.Helper()
	v := a.Find(selector)
	for m := v; m != nil; m = m.Parent {
		if m.Kind == core.KindModal {
			return m
		}
	}
	a.t.Fatalf("nsgotest: %q is in no modal", selector)
	return nil
}

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

// Type replaces the text of the field selector matches, as a user's edit,
// cut to its MaxLength as the platforms cut what the user enters. The
// caret goes to the end.
func (a *App) Type(selector, text string) {
	a.t.Helper()
	v := a.Find(selector)
	if v.Kind != core.KindInput {
		a.t.Fatalf("nsgotest: %q is not a text field", selector)
	}
	if v.Input.Readonly || v.Input.Disabled {
		a.t.Fatalf("nsgotest: %q does not take text", selector)
	}
	a.edit(v, core.ClipUTF16(text, v.Input.MaxLength))
}

func (a *App) edit(v *View, text string) {
	a.edits[v.ID]++
	v.Text = text
	a.dispatch(core.Event{Kind: core.EventChangeText, ID: v.ID, Text: text, Count: a.edits[v.ID]})
	end := core.UTF16Len(text)
	a.moveSelection(v, core.TextRange{Start: end, End: end})
}

// Select moves the selection of the field selector matches, as the user
// would, in UTF-16 units.
func (a *App) Select(selector string, start, end int) {
	a.t.Helper()
	v := a.Find(selector)
	if v.Kind != core.KindInput {
		a.t.Fatalf("nsgotest: %q is not a text field", selector)
	}
	a.moveSelection(v, core.TextRange{Start: start, End: end})
}

func (a *App) moveSelection(v *View, r core.TextRange) {
	if r == v.Selection {
		return
	}
	v.Selection = r
	if v.Events.Has(core.EventSelectionChange) {
		a.dispatch(core.Event{Kind: core.EventSelectionChange, ID: v.ID, Index: r.Start, Count: uint32(r.End)})
	}
}

// KeyPress types key in the field selector matches, as a key the user
// presses: its text, "Backspace" or "Enter". It reports the key, then
// edits the end of the text as the key would; Enter goes as Submit.
func (a *App) KeyPress(selector, key string) {
	a.t.Helper()
	v := a.Find(selector)
	if v.Events.Has(core.EventKeyPress) {
		a.dispatch(core.Event{Kind: core.EventKeyPress, ID: v.ID, Text: key})
	}
	switch key {
	case "Enter":
		a.Submit(selector)
	case "Backspace":
		if r := []rune(v.Text); len(r) > 0 {
			a.edit(v, string(r[:len(r)-1]))
		}
	default:
		if add, ok := core.LimitEdit(v.Text, 0, key, v.Input.MaxLength); ok {
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
		if v.Events.Has(core.EventCompositionChange) {
			a.dispatch(core.Event{Kind: core.EventCompositionChange, ID: v.ID, Text: text})
		}
		return
	}
	v.Composing = ""
	if v.Events.Has(core.EventCompositionChange) {
		a.dispatch(core.Event{Kind: core.EventCompositionChange, ID: v.ID, Finished: true})
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
	if v.Kind != core.KindInput {
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
	case core.SubmitBehaviorNewline:
		if add, ok := core.LimitEdit(v.Text, 0, "\n", v.Input.MaxLength); ok && !v.Input.Readonly {
			a.edit(v, v.Text+add)
		}
		return
	case core.SubmitBehaviorBlurAndSubmit:
		a.dispatch(core.Event{Kind: core.EventSubmitEditing, ID: v.ID, Text: v.Text})
		a.setFocus(v, false)
		a.Settle()
		return
	}
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

// Play reports that the video selector matches opened and loaded
// info, as a platform does: it starts loading, then loads.
func (a *App) Play(selector string, info core.VideoInfo) {
	a.t.Helper()
	v := a.Find(selector)
	for _, k := range []core.EventKind{core.EventLoadStart, core.EventLoad} {
		if v.Events.Has(k) {
			a.dispatch(core.Event{Kind: k, ID: v.ID, Duration: info.Duration, Size: info.Size})
		}
	}
}

// Progress reports that the video selector matches played to position:
// its progress, then its end when position reaches duration.
func (a *App) Progress(selector string, position, duration time.Duration) {
	a.t.Helper()
	v := a.Find(selector)
	if v.Events.Has(core.EventProgress) {
		a.dispatch(core.Event{Kind: core.EventProgress, ID: v.ID, Time: position, Duration: duration})
	}
	if position >= duration && v.Events.Has(core.EventEnd) {
		a.dispatch(core.Event{Kind: core.EventEnd, ID: v.ID})
	}
}

// Navigate reports that the web view selector matches loaded the page
// nav describes, as a platform does: it starts loading, then loads.
func (a *App) Navigate(selector string, nav core.WebNavigation) {
	a.t.Helper()
	v := a.Find(selector)
	for _, loading := range []bool{true, false} {
		nav.Loading = loading
		kind := core.EventLoad
		if loading {
			kind = core.EventLoadStart
		}
		for _, k := range []core.EventKind{kind, core.EventNavigationStateChange} {
			if v.Events.Has(k) {
				n := nav
				a.dispatch(core.Event{Kind: k, ID: v.ID, Navigation: &n})
			}
		}
	}
}

// PostMessage reports that the page in the web view selector matches
// posted data.
func (a *App) PostMessage(selector, data string) {
	a.t.Helper()
	v := a.Find(selector)
	if v.Events.Has(core.EventMessage) {
		a.dispatch(core.Event{Kind: core.EventMessage, ID: v.ID, Text: data})
	}
}

// Scroll scrolls the view selector matches to offset.
func (a *App) Scroll(selector string, offset core.Point) {
	a.t.Helper()
	v := a.Find(selector)
	v.Offset = offset
	a.anim.Scroll(v.ID, offset)
	a.dispatch(core.Event{Kind: core.EventScroll, ID: v.ID, Point: offset, Count: v.Applied})
}

// DragScroll drags the scroll view selector matches to offset and lets
// go at velocity (points per second along its axis, positive as the
// offset grows), as a platform does: the drag's events, then the glide or
// the snap its ScrollBehavior decides, clamped to its content and insets.
func (a *App) DragScroll(selector string, offset core.Point, velocity float32) {
	a.t.Helper()
	v := a.Find(selector)
	if v.Scroll.Behavior.NoScroll {
		return
	}
	a.scrollEvent(v, core.EventScrollDragStart)
	a.Scroll(selector, offset)
	a.scrollEvent(v, core.EventScrollDragEnd)
	b := &v.Scroll.Behavior
	main := func(p core.Point) core.Points {
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
	to := core.Projected(at, ms, b.Deceleration())
	if snap, ok := b.SnapTarget(at, to, ms, viewport, content); ok {
		to = snap
	} else if ms == 0 {
		return
	}
	to = min(max(to, lo), max(content-viewport, 0)+hi)
	if to == at {
		return
	}
	a.scrollEvent(v, core.EventMomentumStart)
	end := offset
	if v.Scroll.Horizontal {
		end.X = to
	} else {
		end.Y = to
	}
	a.Scroll(selector, end)
	a.scrollEvent(v, core.EventMomentumEnd)
}

// PullToRefresh pulls the refreshable scroll view selector matches down
// past its top, as a user does to refresh it.
func (a *App) PullToRefresh(selector string) {
	a.t.Helper()
	v := a.Find(selector)
	if !v.Scroll.Refreshable || v.Scroll.Refreshing || v.Scroll.Horizontal || v.Scroll.Behavior.NoScroll {
		return
	}
	a.scrollEvent(v, core.EventScrollDragStart)
	a.dispatch(core.Event{Kind: core.EventRefresh, ID: v.ID, Point: v.Offset, Count: v.Applied})
	a.scrollEvent(v, core.EventScrollDragEnd)
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
	a.dispatch(core.Event{Kind: core.EventScroll, ID: v.ID, Point: v.Offset, Value: v.ZoomScale, Count: v.Applied})
}

func (a *App) scrollEvent(v *View, kind core.EventKind) {
	if v.Events.Has(kind) {
		a.dispatch(core.Event{Kind: kind, ID: v.ID, Point: v.Offset, Value: v.ZoomScale, Count: v.Applied})
	}
}

// DropFiles drops files on the view selector matches, as a desktop or
// the web does: the drag enters, then the files drop.
func (a *App) DropFiles(selector string, uris ...string) {
	a.t.Helper()
	v := a.Find(selector)
	if v.Events.Has(core.EventFileDragEnter) {
		a.dispatch(core.Event{Kind: core.EventFileDragEnter, ID: v.ID})
	}
	a.dispatch(core.Event{Kind: core.EventFileDrop, ID: v.ID, URIs: uris})
}

// Emit reports that the component of the native view selector matches
// emitted name with data.
func (a *App) Emit(selector, name string, data any) {
	a.t.Helper()
	v := a.Find(selector)
	a.dispatch(core.Event{Kind: core.EventNative, ID: v.ID, Text: name, Data: data})
}

// animate steps the animator to the clock and records the values it
// moved.
func (a *App) animate() {
	if a.anim.Running() {
		a.anim.Step(a.Clock.Now().Sub(a.start))
	}
	a.anim.Changed(func(id core.NodeID, vals core.AnimatedValues) {
		if v := a.views[id]; v != nil {
			v.Animated = vals
		}
	})
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
			a.anim.Forget(it.ID)
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
		case core.ItemAccessibilityLabel:
			v.Props.Label = it.Value
		case core.ItemAccessibilityFocus:
			a.AccessibilityFocus = v
		case core.ItemEvents:
			v.Events = it.Events
		case core.ItemText:
			v.Text = it.Text.Text
		case core.ItemInput:
			v.Input = *it.Input
		case core.ItemInputValue:
			if v.Composing != "" {
				held := it
				v.held = &held
			} else if a.edits[it.ID] <= it.Count {
				v.Text = it.Value
			}
		case core.ItemSelection:
			n := core.UTF16Len(v.Text)
			r := it.Selection
			v.Selection = core.TextRange{Start: min(max(r.Start, 0), n), End: min(max(r.End, 0), n)}
		case core.ItemScrollContent:
			v.Content = it.Size
		case core.ItemScrollConfig:
			if !v.Scroll.Zooms() && it.Scroll.Zooms() || it.Scroll.ZoomScale != v.Scroll.ZoomScale {
				v.ZoomScale = it.Scroll.ZoomScale
			}
			v.Scroll = *it.Scroll
		case core.ItemZoomTo:
			v.ZoomScale = it.Zoom.Scale
			id, at, z := it.ID, v.Offset, v.ZoomScale
			a.mu.Lock()
			a.posts = append(a.posts, func() {
				a.tree.Dispatch(core.Event{Kind: core.EventScroll, ID: id, Point: at, Value: z})
			})
			a.mu.Unlock()
		case core.ItemImage:
			v.Image = *it.Image
		case core.ItemWeb:
			v.Web = *it.Web
		case core.ItemWebCommand:
			v.WebCommands = append(v.WebCommands, WebCommand{it.WebCommand, it.Value})
		case core.ItemVideo:
			v.Video = *it.Video
		case core.ItemVideoSeek:
			v.Seeks = append(v.Seeks, it.Time)
		case core.ItemNative:
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
		case core.ItemNativeCommand:
			v.NativeCommands = append(v.NativeCommands, NativeCommand{it.Value, it.Data})
		case core.ItemAnimation:
			a.anim.Apply(it.Animation)
		case core.ItemFocus:
			for _, o := range a.views {
				if o != v {
					a.setFocus(o, false)
				}
			}
			a.setFocus(v, true)
		case core.ItemBlur:
			a.setFocus(v, false)
		case core.ItemScrollTo, core.ItemScrollBy:
			v.Applied++
			if it.Kind == core.ItemScrollBy {
				v.Offset.X += it.Point.X
				v.Offset.Y += it.Point.Y
			} else {
				v.Offset = v.clamp(it.Point)
			}
			// A platform reports where a scroll lands.
			id, at, n := it.ID, v.Offset, v.Applied
			a.mu.Lock()
			a.posts = append(a.posts, func() {
				a.tree.Dispatch(core.Event{Kind: core.EventScroll, ID: id, Point: at, Count: n})
			})
			a.mu.Unlock()
		case core.ItemStickyCells:
			v.Sticky = it.Sticky
		case core.ItemSetClipboard:
			a.Clipboard = it.Value
		case core.ItemReadClipboard:
			id, text := it.Count, a.Clipboard
			a.mu.Lock()
			a.posts = append(a.posts, func() {
				a.tree.Dispatch(core.Event{Kind: core.EventClipboard, Count: id, Text: text})
			})
			a.mu.Unlock()
		case core.ItemImageInfo:
			ev := core.Event{Kind: core.EventImageInfo, Count: it.Count}
			if size, ok := a.ImageSizes[it.Image.Source.URI]; ok {
				ev.Size = size
			} else {
				ev.Text = "nsgotest: no image at " + it.Image.Source.URI
			}
			a.mu.Lock()
			a.posts = append(a.posts, func() { a.tree.Dispatch(ev) })
			a.mu.Unlock()
		case core.ItemKeymap:
			a.Keymap = it.Keys
		case core.ItemTitle:
			a.Title = it.Value
		case core.ItemStatusBar:
			a.StatusBar = *it.StatusBar
		case core.ItemScreen:
			v.Screen = *it.Screen
		case core.ItemModal:
			v.Modal = *it.Modal
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
	kind := core.EventBlur
	if on {
		kind = core.EventFocus
	}
	a.mu.Lock()
	a.posts = append(a.posts, func() { a.tree.Dispatch(core.Event{Kind: kind, ID: v.ID}) })
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
	case core.KindCanvas:
		return "canvas"
	case core.KindWeb:
		return "web"
	case core.KindVideo:
		return "video"
	}
	return "?"
}
