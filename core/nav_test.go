package core

import (
	"slices"
	"testing"
)

// phone is an environment with a system header and a status bar 40
// points tall.
var phone = Environment{OS: OSIOS, SafeArea: Edges{Top: 40, Bottom: 20}, HeaderHeight: 44, LargeHeaderHeight: 96}

func newPhone(t *testing.T, env Environment, app func(cx *Scope) Node) *host {
	h := newHost(t, Size{300, 600}, nil)
	h.tree.SetEnvironment(env)
	h.tree.Mount(app)
	return h
}

func screenItems(b *MountBatch) map[NodeID]ScreenConfig {
	out := map[NodeID]ScreenConfig{}
	for _, it := range b.Items {
		if it.Kind == ItemScreen {
			out[it.ID] = *it.Screen
		}
	}
	return out
}

func TestScreenSystemHeader(t *testing.T) {
	var routes *Signal[[]string]
	var insets Edges
	var popped []string
	h := newPhone(t, phone, func(cx *Scope) Node {
		routes = NewSignalFunc(cx, []string{"Home", "Detail"}, nil)
		return Stack{Style: Style{FlexGrow: 1}, Children: []Node{
			Each(routes.Get, func(r string) string { return r }, func(cx *Scope, r func() string, _ func() int) Node {
				name := r()
				s := Screen{Title: name, TestID: name, HeaderBackTitle: "Back to " + name,
					Header: View{TestID: "drawn-" + name},
					OnDismiss: func() {
						popped = append(popped, name)
						routes.Update(func(rs []string) []string { return rs[:len(rs)-1] })
					},
					Children: []Node{Func(func(cx *Scope) Node {
						if name == "Detail" {
							cx.Effect(func() { insets = Insets(cx) })
						}
						return View{TestID: "content-" + name, Style: Style{Height: Pt(10)}}
					})}}
				if name == "Detail" {
					s.HeaderBackTitle = ""
					s.HeaderRight = View{TestID: "action", Style: Style{Width: Pt(30), Height: Pt(20)}}
					s.HeaderStyle = Style{Background: RGB(0x0a141e), Color: RGB(0xffffff), TintColor: RGB(0x0000ff)}
				}
				return s
			}),
		}}
	})
	b := h.commit()
	for _, id := range []string{"drawn-Home", "drawn-Detail"} {
		for _, n := range h.tree.nodes {
			if n != nil && !n.deleted && n.props.TestID == id {
				t.Errorf("%s mounted: the system header replaces Header", id)
			}
		}
	}
	detail, action := h.find("Detail"), h.find("action")
	c, ok := screenItems(b)[detail.id]
	want := ScreenConfig{Title: "Detail", CanGoBack: true, BackTitle: "Back to Home",
		Background: RGB(0x0a141e), TitleColor: RGB(0xffffff), TintColor: RGB(0x0000ff),
		Right: action.parent.id, HeaderHeight: 84}
	if !ok || c != want {
		t.Errorf("detail's header = %+v, want %+v", c, want)
	}
	if home := screenItems(b)[h.find("Home").id]; home.CanGoBack || home.HeaderHeight != 84 {
		t.Errorf("home's header = %+v", home)
	}
	if got := h.find("content-Detail").frame.Origin.Y; got != 84 {
		t.Errorf("content at y %v, want below the bar and the status bar", got)
	}
	if insets.Top != 0 || insets.Bottom != 20 {
		t.Errorf("insets = %v, want the bottom only", insets)
	}
	if action.frame.Size != (Size{30, 20}) || slices.Contains(detail.nativeChildren(), action) {
		t.Errorf("header view = %v, want its own size outside the screen's children", action.frame)
	}
	for i, it := range b.Items {
		if it.Kind == ItemScreen && it.ID == detail.id {
			if !slices.ContainsFunc(b.Items[:i], func(x MountItem) bool { return x.Kind == ItemCreate && x.ID == action.id }) {
				t.Error("the header view is created after the screen naming it")
			}
		}
	}

	// The back button of a screen not on top does nothing.
	h.tree.Dispatch(Event{Kind: EventBack, ID: h.find("Home").id})
	h.tree.Dispatch(Event{Kind: EventBack, ID: detail.id})
	h.commit()
	if !slices.Equal(popped, []string{"Detail"}) {
		t.Errorf("popped = %q, want only the top screen", popped)
	}
}

func TestScreenHeaderOptions(t *testing.T) {
	var insets Edges
	h := newPhone(t, phone, func(cx *Scope) Node {
		return View{Style: Style{FlexGrow: 1}, Children: []Node{
			View{Style: Style{Height: Pt(100)}},
			Stack{Style: Style{FlexGrow: 1}, Children: []Node{
				Screen{Title: "Large", TestID: "large", HeaderLargeTitle: true, Children: []Node{View{TestID: "large-content"}}},
			}},
			Stack{Style: Style{Height: Pt(100)}, Children: []Node{
				Screen{TestID: "clear", HeaderTransparent: true, Children: []Node{Func(func(cx *Scope) Node {
					cx.Effect(func() { insets = Insets(cx) })
					return View{TestID: "clear-content"}
				})}},
			}},
			Stack{Style: Style{Height: Pt(100)}, Children: []Node{
				Screen{TestID: "bare", HeaderHidden: true, Children: []Node{View{TestID: "bare-content"}}},
			}},
		}}
	})
	b := h.commit()
	items := screenItems(b)
	// Below the status bar, a stack's header covers only its bar.
	if got := items[h.find("large").id]; !got.LargeTitle || got.HeaderHeight != 96 {
		t.Errorf("large = %+v", got)
	}
	if got := h.find("large-content").frame.Origin.Y; got != 96 {
		t.Errorf("large title content at %v", got)
	}
	if got := items[h.find("clear").id].HeaderHeight; got != 44 || h.find("clear-content").frame.Origin.Y != 0 {
		t.Errorf("transparent header %v, content at %v, want the content under it", got, h.find("clear-content").frame.Origin.Y)
	}
	if insets.Top != 44 {
		t.Errorf("insets under a transparent header = %v, want its height on top", insets)
	}
	if got := items[h.find("bare").id]; !got.HeaderHidden || got.HeaderHeight != 0 || h.find("bare-content").frame.Origin.Y != 0 {
		t.Errorf("hidden header = %+v", got)
	}
}

func TestScreenHeaderWithoutNativeHeader(t *testing.T) {
	h := newHost(t, Size{300, 600}, func(cx *Scope) Node {
		return Stack{Style: Style{FlexGrow: 1}, Children: []Node{
			Screen{Title: "Home", Header: View{TestID: "drawn", Style: Style{Height: Pt(50)}},
				HeaderRight: View{TestID: "action"}, Children: []Node{View{TestID: "content"}}},
		}}
	})
	b := h.commit()
	if kinds(b)[ItemScreen] != 0 {
		t.Error("ItemScreen sent without header heights")
	}
	if got := h.find("content").frame.Origin.Y; got != 50 {
		t.Errorf("content at %v, want below the drawn header", got)
	}
	for _, n := range h.tree.nodes {
		if n != nil && n.props.TestID == "action" {
			t.Error("a header view mounted without a system header")
		}
	}
}

func statusItems(b *MountBatch) (out []StatusBarConfig) {
	for _, it := range b.Items {
		if it.Kind == ItemStatusBar {
			out = append(out, *it.StatusBar)
		}
	}
	return out
}

func TestStatusBarLayers(t *testing.T) {
	var routes *Signal[[]string]
	var open, light *Signal[bool]
	var root *Scope
	h := newHost(t, Size{300, 600}, func(cx *Scope) Node {
		root = cx
		routes = NewSignalFunc(cx, []string{"home"}, nil)
		open, light = NewSignal(cx, false), NewSignal(cx, true)
		return View{Style: Style{FlexGrow: 1}, Children: []Node{
			Stack{Style: Style{FlexGrow: 1}, Children: []Node{
				Each(routes.Get, func(r string) string { return r }, func(cx *Scope, r func() string, _ func() int) Node {
					s := Screen{TestID: r(), OnDismiss: func() { routes.Set([]string{"home"}) }}
					if r() == "photo" {
						s.StatusBar = StatusBarSetting{Hidden: ToggleOn}
					}
					return s
				}),
			}},
			Show{When: open.Get, Then: func(*Scope) Node {
				return Modal{StatusBarFunc: func() StatusBarSetting {
					if light.Get() {
						return StatusBarSetting{Foreground: StatusBarLight, Animated: ToggleOn}
					}
					return StatusBarSetting{Foreground: StatusBarDark}
				}}
			}},
		}}
	})
	if got := statusItems(h.commit()); len(got) != 0 {
		t.Fatalf("status bar = %v before anything set it", got)
	}
	steps := []struct {
		do   func()
		want []StatusBarConfig
	}{
		{func() { root.SetStatusBar(StatusBarSetting{Foreground: StatusBarDark}) }, []StatusBarConfig{{Style: StatusBarDark}}},
		{func() { routes.Set([]string{"home", "photo"}) }, []StatusBarConfig{{Style: StatusBarDark, Hidden: true}}},
		{func() { open.Set(true) }, []StatusBarConfig{{Style: StatusBarLight, Hidden: true, Animated: true}}},
		{func() { light.Set(false) }, []StatusBarConfig{{Style: StatusBarDark, Hidden: true}}},
		{func() { light.Set(true); light.Set(false) }, nil},
		{func() { open.Set(false) }, nil},
		{func() { h.tree.Dispatch(Event{Kind: EventBack}) }, []StatusBarConfig{{Style: StatusBarDark}}},
	}
	for i, s := range steps {
		s.do()
		if got := statusItems(h.commit()); !slices.Equal(got, s.want) {
			t.Errorf("step %d: status bar = %v, want %v", i, got, s.want)
		}
	}
}

func TestModalSheetSizeAndSwipe(t *testing.T) {
	var reasons []string
	var insets Edges
	h := newPhone(t, phone, func(cx *Scope) Node {
		return View{Style: Style{FlexGrow: 1}, Children: []Node{
			Modal{TestID: "under"},
			Modal{TestID: "sheet", Presentation: ModalPageSheet, SupportedOrientations: OrientationPortrait,
				OnRequestClose: func(r string) { reasons = append(reasons, r) },
				Children: []Node{Func(func(cx *Scope) Node {
					cx.Effect(func() { insets = Insets(cx) })
					return View{TestID: "body", Style: Style{FlexGrow: 1}}
				})}},
		}}
	})
	b := h.commit()
	sheet := h.find("sheet")
	for _, it := range b.Items {
		if it.Kind == ItemModal && it.ID == sheet.id &&
			(it.Modal.Presentation != ModalPageSheet || it.Modal.Orientations != OrientationPortrait) {
			t.Errorf("modal config = %+v", *it.Modal)
		}
	}
	h.tree.Dispatch(Event{Kind: EventModalSize, ID: sheet.id, Size: Size{300, 540}, Insets: Edges{Bottom: 20}})
	h.commit()
	if sheet.frame != (Bounds{Size: Size{300, 540}}) || h.find("body").frame.Size.Height != 540 {
		t.Errorf("sheet = %v, want the size the platform reported", sheet.frame)
	}
	if insets != (Edges{Bottom: 20}) {
		t.Errorf("insets = %v, want the sheet's", insets)
	}
	// Swiped away, the sheet asks to close and stays hidden; back goes to
	// the modal under it.
	h.tree.Dispatch(Event{Kind: EventRequestClose, ID: sheet.id, Text: "swipe", Finished: true})
	h.commit()
	if !sheet.hidden || sheet.viewStyle().Hidden != true {
		t.Error("a swiped sheet is not hidden")
	}
	h.tree.Dispatch(Event{Kind: EventBack})
	if !slices.Equal(reasons, []string{"swipe"}) {
		t.Errorf("reasons = %q, want back to skip the swiped sheet", reasons)
	}
}

func TestAndroidModalStartsBelowTheStatusBar(t *testing.T) {
	var below, under Edges
	env := Environment{OS: OSAndroid, SafeArea: Edges{Top: 24, Bottom: 16}}
	h := newPhone(t, env, func(cx *Scope) Node {
		probe := func(dst *Edges) Node {
			return Func(func(cx *Scope) Node { cx.Effect(func() { *dst = Insets(cx) }); return nil })
		}
		return View{Children: []Node{
			Modal{TestID: "below", Children: []Node{probe(&below)}},
			Modal{TestID: "under", StatusBarTranslucent: true, Children: []Node{probe(&under)}},
		}}
	})
	h.commit()
	if got := h.find("below").frame; got != (Bounds{Point{0, 24}, Size{300, 576}}) || below != (Edges{Bottom: 16}) {
		t.Errorf("modal = %v with insets %v, want below the status bar", got, below)
	}
	if got := h.find("under").frame; got != (Bounds{Size: Size{300, 600}}) || under != env.SafeArea {
		t.Errorf("translucent modal = %v with insets %v, want under the status bar", got, under)
	}
}
