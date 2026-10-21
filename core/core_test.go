package core

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"
)

// host runs a tree headless: posts queue until drain, text is approximate.
type host struct {
	t     *testing.T
	root  *Scope
	tree  *Tree
	mu    sync.Mutex
	posts []func()
	clock *ManualClock
	errs  []error
}

func newHost(t *testing.T, size Size, app func(cx *Scope) Node) *host {
	h := &host{t: t, clock: NewManualClock(time.Unix(0, 0))}
	h.root = NewRoot(RootOptions{
		Post: func(fn func()) {
			h.mu.Lock()
			h.posts = append(h.posts, fn)
			h.mu.Unlock()
		},
		Clock:  h.clock,
		Report: func(err error) { h.errs = append(h.errs, err) },
	})
	h.tree = NewTree(h.root, ApproxTextEngine{})
	h.tree.Resize(size)
	if app != nil {
		h.tree.Mount(app)
	}
	return h
}

func (h *host) drain() {
	for {
		h.mu.Lock()
		posts := h.posts
		h.posts = nil
		h.mu.Unlock()
		if len(posts) == 0 {
			return
		}
		for _, fn := range posts {
			fn()
		}
	}
}

func (h *host) waitPost() {
	for {
		h.mu.Lock()
		n := len(h.posts)
		h.mu.Unlock()
		if n > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

func (h *host) commit() *MountBatch {
	h.t.Helper()
	b := h.tree.Commit()
	if b == nil {
		return &MountBatch{}
	}
	return b
}

func (h *host) find(testID string) *node {
	for _, n := range h.tree.nodes {
		if n != nil && !n.deleted && n.props.TestID == testID {
			return n
		}
	}
	h.t.Fatalf("no node %q", testID)
	return nil
}

func kinds(b *MountBatch) map[ItemKind]int {
	out := map[ItemKind]int{}
	for _, it := range b.Items {
		out[it.Kind]++
	}
	return out
}

func TestSignalsMemosAndEffects(t *testing.T) {
	cx := NewRoot(RootOptions{})
	count := NewSignal(cx, 1)
	doubled := NewMemo(cx, func() int { return count.Get() * 2 })
	parity := NewMemo(cx, func() bool { return doubled.Get()%4 == 0 })
	var seen []int
	var parities []bool
	stop := cx.Effect(func() { seen = append(seen, doubled.Get()) })
	cx.Effect(func() { parities = append(parities, parity.Get()) })
	count.Set(2)
	count.Set(2)
	cx.Batch(func() {
		count.Set(3)
		count.Set(4)
	})
	stop()
	count.Set(5)
	if want := []int{2, 4, 8}; !slices.Equal(seen, want) {
		t.Errorf("effect runs = %v, want %v", seen, want)
	}
	// 2 → false, 4 → true, 8 → true (unchanged: not run), 10 → false
	if want := []bool{false, true, false}; !slices.Equal(parities, want) {
		t.Errorf("parity runs = %v, want %v", parities, want)
	}
}

func TestSignalFuncNotifiesSlices(t *testing.T) {
	cx := NewRoot(RootOptions{})
	list := NewSignalFunc(cx, []int{1}, nil)
	runs := 0
	cx.Effect(func() { list.Get(); runs++ })
	list.Update(func(s []int) []int { return append(s, 2) })
	if runs != 2 {
		t.Fatalf("runs = %d, want 2", runs)
	}
}

func TestOwnersDisposeAndCleanUp(t *testing.T) {
	cx := NewRoot(RootOptions{})
	on := NewSignal(cx, true)
	n := NewSignal(cx, 0)
	var log []string
	cx.Effect(func() {
		if !on.Get() {
			return
		}
		cx.Effect(func() { log = append(log, "inner "+strconv.Itoa(n.Get())) })
		cx.Cleanup(func() { log = append(log, "cleanup") })
	})
	n.Set(1)
	on.Set(false)
	n.Set(2) // the inner effect is gone
	want := []string{"inner 0", "inner 1", "cleanup"}
	if !slices.Equal(log, want) {
		t.Fatalf("log = %q, want %q", log, want)
	}
}

func TestEffectRerunsOnOwnWrite(t *testing.T) {
	cx := NewRoot(RootOptions{})
	n := NewSignal(cx, 0)
	cx.Effect(func() {
		if v := n.Get(); v < 3 {
			n.Set(v + 1)
		}
	})
	if n.GetUntracked() != 3 {
		t.Fatalf("n = %d, want 3", n.GetUntracked())
	}
}

var themeKey = NewKey("theme", "light")

func TestContextAndPost(t *testing.T) {
	var seen []string
	h := newHost(t, Size{100, 100}, func(cx *Scope) Node {
		themeKey.Provide(cx, "dark")
		return Func(func(cx *Scope) Node {
			seen = append(seen, themeKey.Use(cx))
			label := NewSignal(cx, "loading")
			go cx.Post(func() { label.Set("done") })
			return Text{ContentFunc: label.Get, TestID: "label"}
		})
	})
	if !slices.Equal(seen, []string{"dark"}) {
		t.Fatalf("context = %q", seen)
	}
	h.commit()
	h.waitPost()
	h.drain()
	if got := h.find("label").text.content; got != "done" {
		t.Fatalf("label = %q", got)
	}
	if themeKey.Use(h.root) != "light" {
		t.Fatal("a key provided below leaked to the root")
	}
}

func TestAfterFuncRunsOnClock(t *testing.T) {
	h := newHost(t, Size{100, 100}, func(cx *Scope) Node {
		n := NewSignal(cx, 0)
		cx.AfterFunc(time.Second, func() { n.Set(1) })
		return Text{ContentFunc: func() string { return strconv.Itoa(n.Get()) }, TestID: "n"}
	})
	h.clock.Advance(999 * time.Millisecond)
	h.drain()
	if h.find("n").text.content != "0" {
		t.Fatal("fired early")
	}
	h.clock.Advance(time.Millisecond)
	h.drain()
	if h.find("n").text.content != "1" {
		t.Fatal("did not fire")
	}
}

func frames(b *MountBatch) map[NodeID]Bounds {
	out := map[NodeID]Bounds{}
	for _, it := range b.Items {
		if it.Kind == ItemFrame {
			out[it.ID] = it.Frame
		}
	}
	return out
}

func TestColumnStretchesAndPads(t *testing.T) {
	h := newHost(t, Size{200, 300}, func(cx *Scope) Node {
		return View{Style: Style{Padding: All(10), Gap: 5, FlexGrow: 1}, TestID: "root", Children: []Node{
			Text{Content: "hello", Style: Style{FontSize: 10}, TestID: "text"},
			Input{Style: Style{Height: Pt(40)}, TestID: "input"},
		}}
	})
	h.commit()
	cases := map[string]Bounds{
		"root":  {Size: Size{200, 300}},
		"text":  {Point{10, 10}, Size{180, 12}},
		"input": {Point{10, 27}, Size{180, 40}},
	}
	for id, want := range cases {
		if got := h.find(id).frame; got != want {
			t.Errorf("%s = %v, want %v", id, got, want)
		}
	}
}

func TestRowGrowsShrinksAndCenters(t *testing.T) {
	h := newHost(t, Size{100, 50}, func(cx *Scope) Node {
		return View{Style: Style{Direction: Row, Height: Pt(50), AlignItems: AlignCenter}, Children: []Node{
			View{Style: Style{Width: Pt(30), Height: Pt(20)}, TestID: "fixed"},
			View{Style: Style{FlexGrow: 1, Height: Pt(10)}, TestID: "grow"},
			View{Style: Style{Width: Pt(90), FlexShrink: 1, Height: Pt(10), MinWidth: Pt(40)}, TestID: "shrink"},
		}}
	})
	h.commit()
	cases := map[string]Bounds{
		"fixed":  {Point{0, 15}, Size{30, 20}},
		"grow":   {Point{30, 20}, Size{0, 10}},
		"shrink": {Point{30, 20}, Size{70, 10}},
	}
	for id, want := range cases {
		if got := h.find(id).frame; got != want {
			t.Errorf("%s = %v, want %v", id, got, want)
		}
	}
}

func TestWrapAbsoluteAndPercent(t *testing.T) {
	h := newHost(t, Size{100, 100}, func(cx *Scope) Node {
		return View{Style: Style{Direction: Row, Wrap: Wrap, Gap: 10, Height: Pct(100)}, Children: []Node{
			View{Style: Style{Width: Pct(50), Height: Pt(10)}, TestID: "a"},
			View{Style: Style{Width: Pct(50), Height: Pt(10)}, TestID: "b"},
			View{Style: Style{Position: Absolute, Right: Pt(5), Bottom: Pt(5), Width: Pt(20), Height: Pt(20)}, TestID: "abs"},
		}}
	})
	h.commit()
	cases := map[string]Bounds{
		"a":   {Point{0, 0}, Size{50, 10}},
		"b":   {Point{0, 20}, Size{50, 10}},
		"abs": {Point{75, 75}, Size{20, 20}},
	}
	for id, want := range cases {
		if got := h.find(id).frame; got != want {
			t.Errorf("%s = %v, want %v", id, got, want)
		}
	}
}

func TestTextInheritsAndWraps(t *testing.T) {
	h := newHost(t, Size{30, 100}, func(cx *Scope) Node {
		return View{Style: Style{FontSize: 10, Color: RGB(0xff0000)}, Children: []Node{
			Text{Content: "aaaa bbbb", TestID: "text"},
		}}
	})
	h.commit()
	n := h.find("text")
	if n.text.style.Font.Size != 10 || n.text.style.Color != RGB(0xff0000) {
		t.Errorf("style = %+v, want inherited size and color", n.text.style)
	}
	if got, want := n.frame.Size, (Size{30, 24}); got != want {
		t.Errorf("size = %v, want %v (stretched, two lines)", got, want)
	}
}

func TestCommitSendsOnlyChanges(t *testing.T) {
	var count *Signal[int]
	h := newHost(t, Size{100, 100}, func(cx *Scope) Node {
		count = NewSignal(cx, 0)
		return View{Children: []Node{
			Text{ContentFunc: func() string { return strconv.Itoa(count.Get()) }, TestID: "n"},
			View{OnPress: func() { count.Update(func(n int) int { return n + 1 }) }, Style: Style{Height: Pt(30)}, TestID: "button"},
		}}
	})
	first := h.commit()
	if k := kinds(first); k[ItemCreate] != 4 || k[ItemInsert] != 3 {
		t.Fatalf("first batch kinds = %v", k)
	}
	if h.tree.Commit() != nil {
		t.Fatal("an unchanged tree commits again")
	}
	button := h.find("button")
	if got := h.tree.HitTest(Point{50, 40}); got != button.id {
		t.Fatalf("hit test = %d, want %d", got, button.id)
	}
	h.tree.Dispatch(Event{Kind: EventPress, ID: button.id})
	b := h.commit()
	if k := kinds(b); k[ItemText] != 1 || len(b.Items) != 1 {
		t.Errorf("batch after press = %v, want one text item", k)
	}
}

func TestShowAndEachMountAndUnmount(t *testing.T) {
	var open *Signal[bool]
	var items *Signal[[]string]
	h := newHost(t, Size{100, 200}, func(cx *Scope) Node {
		open = NewSignal(cx, true)
		items = NewSignalFunc(cx, []string{"a", "b", "c"}, nil)
		return View{TestID: "list", Children: []Node{
			Show{When: open.Get, Then: func(cx *Scope) Node { return Text{Content: "open", TestID: "open"} }},
			Each(items.Get, func(s string) string { return s }, func(cx *Scope, item func() string, index func() int) Node {
				return Text{ContentFunc: func() string { return fmt.Sprint(index(), item()) }, TestID: "row-" + item()}
			}),
		}}
	})
	h.commit()
	list := h.find("list")
	order := func() (out []string) {
		for _, c := range list.sent.children {
			out = append(out, c.text.content)
		}
		return out
	}
	if got := order(); !slices.Equal(got, []string{"open", "0a", "1b", "2c"}) {
		t.Fatalf("children = %q", got)
	}

	open.Set(false)
	b := h.commit()
	if k := kinds(b); k[ItemDelete] != 1 {
		t.Errorf("closing kinds = %v, want one delete", k)
	}

	c := h.find("row-c")
	items.Set([]string{"c", "a", "d"})
	b = h.commit()
	k := kinds(b)
	if k[ItemDelete] != 1 || k[ItemCreate] != 1 {
		t.Errorf("reorder kinds = %v, want b deleted and d created", k)
	}
	if h.find("row-c") != c {
		t.Error("row c was rebuilt instead of kept")
	}
	if got := order(); !slices.Equal(got, []string{"0c", "1a", "2d"}) {
		t.Errorf("children = %q", got)
	}
	// c moved before a: one insert for c, one for the new d.
	if k[ItemInsert] != 2 {
		t.Errorf("inserts = %d, want 2", k[ItemInsert])
	}
}

func TestCatchShowsFallbackAndResets(t *testing.T) {
	var fail *Signal[bool]
	var reset func()
	h := newHost(t, Size{100, 100}, func(cx *Scope) Node {
		fail = NewSignal(cx, false)
		return Catch{
			Try: func(cx *Scope) Node {
				return Dynamic(func(cx *Scope) Node {
					if fail.Get() {
						panic(errors.New("boom"))
					}
					return Text{Content: "fine", TestID: "fine"}
				})
			},
			Fallback: func(cx *Scope, err error, r func()) Node {
				reset = r
				return Text{Content: err.Error()[:len("panic: boom")], TestID: "fallback"}
			},
		}
	})
	h.commit()
	fail.Set(true)
	h.commit()
	if got := h.find("fallback").text.content; got != "panic: boom" {
		t.Fatalf("fallback = %q", got)
	}
	fail.Set(false)
	reset()
	h.commit()
	h.find("fine")
	if len(h.errs) != 0 {
		t.Fatalf("reported %v", h.errs)
	}
}

func TestInputEditIsNotMountedBack(t *testing.T) {
	var name *Signal[string]
	h := newHost(t, Size{100, 100}, func(cx *Scope) Node {
		name = NewSignal(cx, "")
		upper := func(s string) {
			if len(s) <= 3 {
				name.Set(s)
			}
		}
		return Input{Value: name.Get, OnChange: upper, TestID: "input"}
	})
	h.commit()
	in := h.find("input")

	h.tree.Dispatch(Event{Kind: EventChangeText, ID: in.id, Text: "Go", Count: 1})
	if name.Get() != "Go" {
		t.Fatalf("signal = %q", name.Get())
	}
	if b := h.tree.Commit(); b != nil {
		t.Fatalf("edit mounted back: %+v", b.Items)
	}

	// The app refuses a fourth character: the field goes back.
	h.tree.Dispatch(Event{Kind: EventChangeText, ID: in.id, Text: "Gooo", Count: 2})
	b := h.commit()
	if len(b.Items) != 1 || b.Items[0].Kind != ItemInputValue || b.Items[0].Value != "Go" || b.Items[0].Count != 2 {
		t.Fatalf("items = %+v, want the value Go after edit 2", b.Items)
	}
}

func TestScrollContentSize(t *testing.T) {
	h := newHost(t, Size{100, 100}, func(cx *Scope) Node {
		return Scroll{Style: Style{FlexGrow: 1}, TestID: "scroll", Children: []Node{
			View{Style: Style{Height: Pt(80)}},
			View{Style: Style{Height: Pt(80)}},
		}}
	})
	h.commit()
	s := h.find("scroll")
	if got, want := s.scroll.content, (Size{100, 160}); got != want {
		t.Fatalf("content = %v, want %v", got, want)
	}
}

func TestIncreasingRun(t *testing.T) {
	got := increasingRun([]int{2, 0, -1, 1, 3})
	want := []bool{false, true, false, true, true}
	if !slices.Equal(got, want) {
		t.Fatalf("run = %v, want %v", got, want)
	}
}

func TestStateStyles(t *testing.T) {
	var disabled *Signal[bool]
	h := newHost(t, Size{100, 100}, func(cx *Scope) Node {
		disabled = NewSignal(cx, false)
		return View{
			Style: Style{Background: RGB(0x0000ff),
				Hover:    &Style{Background: RGB(0x00ff00)},
				Active:   &Style{Background: RGB(0xff0000)},
				Disabled: &Style{Opacity: Alpha(0.5)}},
			OnPress:  func() {},
			Disabled: disabled.Get,
			TestID:   "button",
		}
	})
	h.commit()
	n := h.find("button")
	if m := n.events(); !m.Has(EventPressDown) || !m.Has(EventHoverIn) {
		t.Fatalf("mask = %b, want press down and hover reported", m)
	}
	bg := func() Color { return n.style.Background }
	h.tree.Dispatch(Event{Kind: EventHoverIn, ID: n.id})
	if bg() != RGB(0x00ff00) {
		t.Errorf("hovered = %x", bg())
	}
	h.tree.Dispatch(Event{Kind: EventPressDown, ID: n.id})
	if bg() != RGB(0xff0000) {
		t.Errorf("pressed = %x, want active over hover", bg())
	}
	h.tree.Dispatch(Event{Kind: EventPressUp, ID: n.id})
	h.tree.Dispatch(Event{Kind: EventHoverOut, ID: n.id})
	if bg() != RGB(0x0000ff) {
		t.Errorf("released = %x", bg())
	}
	disabled.Set(true)
	h.tree.Dispatch(Event{Kind: EventPressDown, ID: n.id})
	if bg() != RGB(0x0000ff) || n.style.Opacity.Value() != 0.5 {
		t.Errorf("disabled: background %x, opacity %v", bg(), n.style.Opacity.Value())
	}
	b := h.commit()
	if k := kinds(b); k[ItemViewStyle] != 1 {
		t.Errorf("kinds = %v, want one view style", k)
	}
}
