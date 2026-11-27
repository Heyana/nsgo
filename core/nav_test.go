package core

import (
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// phone is an environment with a system header and a status bar 40
// points tall.
var phone = Environment{OS: OSIOS, SafeArea: Edges{Top: 40, Bottom: 20}, HeaderHeight: 44, LargeHeaderHeight: 96}

func newPhone(t *testing.T, env Environment, app func(cx *Scope) Node) *host {
	h := newHost(t, Size{Width: 300, Height: 600}, nil)
	h.tree.SetEnvironment(env)
	h.tree.Mount(app)
	return h
}

func screenItems(b *driver.MountBatch) map[NodeID]driver.ScreenConfig {
	out := map[NodeID]driver.ScreenConfig{}
	for _, it := range b.Items {
		if it.Kind == driver.ItemScreen {
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
	// Header is made, as NSH makes it, but held as a header view the
	// system header leaves unused, never in the screen's content.
	for _, id := range []string{"drawn-Home", "drawn-Detail"} {
		for _, n := range h.tree.nodes {
			if n != nil && !n.deleted && n.props.TestID == id && (n.parent == nil || !n.parent.headerView) {
				t.Errorf("%s is in the screen: the system header replaces Header", id)
			}
		}
	}
	detail, action := h.find("Detail"), h.find("action")
	c, ok := screenItems(b)[detail.id]
	want := driver.ScreenConfig{Title: "Detail", CanGoBack: true, BackTitle: "Back to Home",
		Background: RGB(0x0a141e), TitleColor: RGB(0xffffff), TintColor: RGB(0x0000ff),
		Right: action.parent.id, HeaderHeight: 84, Header: h.find("drawn-Detail").parent.id}
	if !ok || !c.Equal(want) {
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
	if action.frame.Size != (Size{Width: 30, Height: 20}) || slices.Contains(detail.nativeChildren(), action) {
		t.Errorf("header view = %v, want its own size outside the screen's children", action.frame)
	}
	for i, it := range b.Items {
		if it.Kind == driver.ItemScreen && it.ID == detail.id {
			if !slices.ContainsFunc(b.Items[:i], func(x driver.MountItem) bool { return x.Kind == driver.ItemCreate && x.ID == action.id }) {
				t.Error("the header view is created after the screen naming it")
			}
		}
	}

	// The back button of a screen not on top does nothing.
	h.tree.Dispatch(driver.Event{Kind: driver.EventBack, ID: h.find("Home").id})
	h.tree.Dispatch(driver.Event{Kind: driver.EventBack, ID: detail.id})
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
				Screen{Title: "Large", TestID: "large", HeaderLargeTitle: true, Children: []Node{
					Scroll{TestID: "large-content", Style: Style{FlexGrow: 1}, Children: []Node{View{Style: Style{Height: Pt(900)}}}}}},
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
	// A large title lays the content under it, insetting the scroll
	// view at its top, which it collapses into.
	if got := h.find("large-content").frame.Origin.Y; got != 0 {
		t.Errorf("large title content at %v, want under the header", got)
	}
	var config *driver.ScrollConfig
	for _, it := range b.Items {
		if it.Kind == driver.ItemScrollConfig && it.ID == h.find("large-content").id {
			config = it.Scroll
		}
	}
	if config == nil || config.HeaderInset != 96 || config.Behavior.ContentInset.Top != 96 {
		t.Errorf("the scroll view under the large title has %+v", config)
	}
	if got := items[h.find("clear").id].HeaderHeight; got != 44 || h.find("clear-content").frame.Origin.Y != 0 {
		t.Errorf("transparent header %v, content at %v, want the content under it", got, h.find("clear-content").frame.Origin.Y)
	}
	if insets.Top != 44 {
		t.Errorf("insets under a transparent header = %v, want its height on top", insets)
	}
	if got := items[h.find("bare").id]; !got.HeaderHidden || got.HeaderHeight != 0 || h.find("bare-content").frame.Origin.Y != 0 {
		t.Errorf("hidden header = %+v", got)
	} // The platform's measure replaces the estimate.
	h.tree.Dispatch(driver.Event{Kind: driver.EventScreenHeader, ID: h.find("clear").id, Value: 50})
	// The clear screen is below the status bar: its header is its bar.
	b2 := h.commit()
	if got := screenItems(b2)[h.find("clear").id].HeaderHeight; got != 50 {
		t.Errorf("measured header %v, want 50", got)
	}
	if insets.Top != 50 {
		t.Errorf("insets after the measure %v", insets)
	}
}

func TestScreenHeaderWithoutNativeHeader(t *testing.T) {
	h := newHost(t, Size{Width: 300, Height: 600}, func(cx *Scope) Node {
		return Stack{Style: Style{FlexGrow: 1}, Children: []Node{
			Screen{Title: "Home", Header: View{TestID: "drawn", Style: Style{Height: Pt(50)}},
				HeaderRight: View{TestID: "action"}, Children: []Node{View{TestID: "content"}}},
		}}
	})
	b := h.commit()
	if kinds(b)[driver.ItemScreen] != 0 {
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

func statusItems(b *driver.MountBatch) (out []driver.StatusBarConfig) {
	for _, it := range b.Items {
		if it.Kind == driver.ItemStatusBar {
			out = append(out, *it.StatusBar)
		}
	}
	return out
}

func TestStatusBarLayers(t *testing.T) {
	var routes *Signal[[]string]
	var open, light *Signal[bool]
	var root *Scope
	h := newHost(t, Size{Width: 300, Height: 600}, func(cx *Scope) Node {
		root = cx
		routes = NewSignalFunc(cx, []string{"home"}, nil)
		open, light = NewSignal(cx, false), NewSignal(cx, true)
		return View{Style: Style{FlexGrow: 1}, Children: []Node{
			Stack{Style: Style{FlexGrow: 1}, Children: []Node{
				Each(routes.Get, func(r string) string { return r }, func(cx *Scope, r func() string, _ func() int) Node {
					s := Screen{TestID: r(), OnDismiss: func() { routes.Set([]string{"home"}) }}
					if r() == "photo" {
						s.StatusBar = StatusBarSetting{Hidden: ToggleOn, Immersive: ToggleOn}
					}
					return s
				}),
			}},
			Show{When: open.Get, Then: func(*Scope) Node {
				return Modal{StatusBarFunc: func() StatusBarSetting {
					if light.Get() {
						return StatusBarSetting{Foreground: StatusBarLight, Immersive: ToggleOff, Animated: ToggleOn}
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
		want []driver.StatusBarConfig
	}{
		{func() { root.SetStatusBar(StatusBarSetting{Foreground: StatusBarDark}) }, []driver.StatusBarConfig{{Style: StatusBarDark}}},
		{func() { routes.Set([]string{"home", "photo"}) }, []driver.StatusBarConfig{{Style: StatusBarDark, Hidden: true, Immersive: true}}},
		{func() { open.Set(true) }, []driver.StatusBarConfig{{Style: StatusBarLight, Hidden: true, Animated: true}}},
		{func() { light.Set(false) }, []driver.StatusBarConfig{{Style: StatusBarDark, Hidden: true, Immersive: true}}},
		{func() { light.Set(true); light.Set(false) }, nil},
		{func() { open.Set(false) }, nil},
		{func() { h.tree.Dispatch(driver.Event{Kind: driver.EventBack}) }, []driver.StatusBarConfig{{Style: StatusBarDark}}},
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
			Modal{TestID: "sheet", Presentation: ModalPageSheet, IOS: IOSModalProps{SupportedOrientations: OrientationPortrait},
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
		if it.Kind == driver.ItemModal && it.ID == sheet.id &&
			(it.Modal.Presentation != ModalPageSheet || it.Modal.Orientations != OrientationPortrait) {
			t.Errorf("modal config = %+v", *it.Modal)
		}
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventModalSize, ID: sheet.id, Size: Size{Width: 300, Height: 540}, Insets: Edges{Bottom: 20}})
	h.commit()
	if sheet.frame != (Bounds{Size: Size{Width: 300, Height: 540}}) || h.find("body").frame.Size.Height != 540 {
		t.Errorf("sheet = %v, want the size the platform reported", sheet.frame)
	}
	if insets != (Edges{Bottom: 20}) {
		t.Errorf("insets = %v, want the sheet's", insets)
	}
	// Swiped away, the sheet asks to close and stays hidden; back goes to
	// the modal under it.
	h.tree.Dispatch(driver.Event{Kind: driver.EventRequestClose, ID: sheet.id, Text: "swipe", Finished: true})
	h.commit()
	if !sheet.hidden || sheet.viewStyle().Hidden != true {
		t.Error("a swiped sheet is not hidden")
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventBack})
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
			Modal{TestID: "under", Android: AndroidModalProps{StatusBarTranslucent: true}, Children: []Node{probe(&under)}},
		}}
	})
	h.commit()
	if got := h.find("below").frame; got != (Bounds{Origin: Point{X: 0, Y: 24}, Size: Size{Width: 300, Height: 576}}) || below != (Edges{Bottom: 16}) {
		t.Errorf("modal = %v with insets %v, want below the status bar", got, below)
	}
	if got := h.find("under").frame; got != (Bounds{Size: Size{Width: 300, Height: 600}}) || under != env.SafeArea {
		t.Errorf("translucent modal = %v with insets %v, want under the status bar", got, under)
	}
}

// TestScreenInsetsFollowTheirPlace starts a screen's insets from what its place keeps clear of, not the surface's safe area: a part of the safe area something around it took (ProvideInsets) or a modal's.
func TestScreenInsetsFollowTheirPlace(t *testing.T) {
	var provided, modal Edges
	env := Environment{OS: OSAndroid, SafeArea: Edges{Top: 24, Bottom: 16}}
	h := newPhone(t, env, func(cx *Scope) Node {
		stack := func(dst *Edges) Node {
			return Stack{Style: Style{FlexGrow: 1}, Children: []Node{Screen{HeaderHidden: true, Children: []Node{
				Func(func(cx *Scope) Node { cx.Effect(func() { *dst = Insets(cx) }); return nil }),
			}}}}
		}
		return View{Style: Style{FlexGrow: 1}, Children: []Node{
			Func(func(cx *Scope) Node {
				outer := InsetsFunc(cx)
				ProvideInsets(cx, func() Edges { in := outer(); in.Bottom = 0; return in })
				return stack(&provided)
			}),
			Modal{Children: []Node{stack(&modal)}},
		}}
	})
	h.commit()
	if provided != (Edges{Top: 24}) {
		t.Errorf("screen under provided insets = %v, want its place's, without the bottom", provided)
	}
	if modal != (Edges{Bottom: 16}) {
		t.Errorf("screen in a modal below the status bar = %v, want the modal's", modal)
	}
}

func TestBackPreviewNamesTheScreenBackPops(t *testing.T) {
	var cx0 *Scope
	var open *Signal[bool]
	h := newPhone(t, phone, func(cx *Scope) Node {
		cx0 = cx
		open = NewSignal(cx, false)
		return View{Style: Style{FlexGrow: 1}, Children: []Node{
			Stack{Style: Style{FlexGrow: 1}, Children: []Node{
				Screen{TestID: "home"},
				Screen{TestID: "detail"},
			}},
			Show{When: open.Get, Then: func(*Scope) Node {
				return Overlay{OnCancel: func() {}, Children: []Node{View{}}}
			}},
		}}
	})
	h.commit()
	preview := func() NodeID {
		h.tree.Dispatch(driver.Event{Kind: driver.EventBackStarted})
		for _, it := range h.commit().Items {
			if it.Kind == driver.ItemBackPreview {
				return it.ID
			}
		}
		t.Fatal("no ItemBackPreview")
		return 0
	}
	if got := preview(); got != h.find("detail").id {
		t.Errorf("preview = %v, want the top screen", got)
	}
	if f := h.find("home").frame; f.Size.Width == 0 {
		t.Errorf("home's frame = %v while a back gesture may uncover it", f)
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventBackCancelled})
	h.commit()
	if f := h.find("home").frame; f.Size.Width != 0 {
		t.Errorf("home's frame = %v after the gesture was cancelled", f)
	}
	open.Set(true)
	if got := preview(); got != 0 {
		t.Errorf("preview = %v under an overlay hearing cancel, want none", got)
	}
	open.Set(false)
	cx0.OnBackPress(func() bool { return false })
	if got := preview(); got != 0 {
		t.Errorf("preview = %v with a back handler, want none", got)
	}
}

func TestScreenHeaderSystemItem(t *testing.T) {
	h := newPhone(t, phone, func(cx *Scope) Node {
		return Stack{Style: Style{FlexGrow: 1}, Children: []Node{
			Screen{Title: "Bare", TestID: "bare", IOS: IOSScreenProps{HeaderRightSystemItem: HeaderItemAdd}},
			Screen{Title: "Add", TestID: "add", IOS: IOSScreenProps{HeaderRightSystemItem: HeaderItemAdd},
				HeaderRight: View{TestID: "action", OnPress: func(PressEvent) {}}},
		}}
	})
	items := screenItems(h.commit())
	if c := items[h.find("add").id]; c.RightSystemItem != HeaderItemAdd || c.Right != h.find("action").parent.id {
		t.Errorf("with HeaderRight: item %v, right %v; want add over the action", c.RightSystemItem, c.Right)
	}
	if c := items[h.find("bare").id]; c.RightSystemItem != 0 {
		t.Errorf("without HeaderRight: item %v, want none, as nothing would take its press", c.RightSystemItem)
	}
}

func TestExitAppLeavesAsBackDoes(t *testing.T) {
	var scope *Scope
	h := newPhone(t, phone, func(cx *Scope) Node {
		scope = cx
		return View{}
	})
	h.commit()
	scope.ExitApp()
	if k := kinds(h.commit()); k[driver.ItemBackUnhandled] != 1 {
		t.Errorf("ExitApp sent %v, want one ItemBackUnhandled", k)
	}
}

func TestScreenHeaderClassAndAnimation(t *testing.T) {
	h := newPhone(t, phone, func(cx *Scope) Node {
		return Stack{Style: Style{FlexGrow: 1}, Children: []Node{
			Screen{Title: "Home", TestID: "Home", HeaderClass: "bg-white dark:bg-black text-red-500 tint-blue-500",
				HeaderStyle: Style{Color: RGB(0x00ff00)}, Animation: ScreenFade},
		}}
	})
	c := screenItems(h.commit())[h.find("Home").id]
	if c.Background != RGB(0xffffff) || c.TitleColor != RGB(0x00ff00) || c.TintColor != tailwindPalette["blue"][5] ||
		c.Animation != ScreenFade {
		t.Errorf("header %+v: the classes' colors under HeaderStyle, and the animation", c)
	}
	env := phone
	env.Dark = true
	h.tree.SetEnvironment(env)
	if c := screenItems(h.commit())[h.find("Home").id]; c.Background != RGB(0) {
		t.Errorf("dark header %#x", c.Background)
	}
}

// TestScreenHeaderSymbols sends the symbols over HeaderRight, and no right
// view while there are none, as the system would draw its glass for it.
func TestScreenHeaderSymbols(t *testing.T) {
	right := View{OnPress: func(PressEvent) {}}
	h := newPhone(t, phone, func(cx *Scope) Node {
		return Stack{Style: Style{FlexGrow: 1}, Children: []Node{
			Screen{Title: "Empty", TestID: "empty", HeaderRight: right,
				IOS: IOSScreenProps{HeaderRightSymbols: func() []HeaderSymbol { return nil }}},
			Screen{Title: "Task", TestID: "task", HeaderRight: right, IOS: IOSScreenProps{HeaderRightSymbols: func() []HeaderSymbol {
				return []HeaderSymbol{{Name: "play", Label: "Run"}, {Name: "ellipsis", Label: "More"}}
			}}},
		}}
	})
	items := screenItems(h.commit())
	if c := items[h.find("task").id]; c.RightSymbols != "play\tRun\nellipsis\tMore" || c.Right == 0 {
		t.Errorf("symbols %q, right %v", c.RightSymbols, c.Right)
	}
	if c := items[h.find("empty").id]; c.RightSymbols != "" || c.Right != 0 {
		t.Errorf("without symbols: %q, right %v", c.RightSymbols, c.Right)
	}
}
func TestAndroidLargeTitleHeader(t *testing.T) {
	env := Environment{OS: OSAndroid, SafeArea: Edges{Top: 24, Bottom: 16}, HeaderHeight: 64, LargeHeaderHeight: 152}
	h := newPhone(t, env, func(cx *Scope) Node {
		return Stack{Style: Style{FlexGrow: 1}, Children: []Node{
			Screen{Title: "Settings", TestID: "android-large", HeaderLargeTitle: true, Children: []Node{
				Scroll{TestID: "top-scroll", Style: Style{FlexGrow: 1}, Children: []Node{
					Scroll{TestID: "h-scroll", Horizontal: true, Style: Style{Height: Pt(40)}, Children: []Node{View{Style: Style{Width: Pt(500)}}}},
					View{Style: Style{Height: Pt(900)}},
				}},
				Scroll{TestID: "lower-scroll", Style: Style{Height: Pt(100)}, Children: []Node{View{Style: Style{Height: Pt(300)}}}},
			}},
		}}
	})
	b := h.commit()
	screen := h.find("android-large")
	got := screenItems(b)[screen.id]
	if !got.LargeTitle || got.HeaderHeight != 176 {
		t.Fatalf("android large header = %+v, want LargeTitle with HeaderHeight 176 (152 + 24)", got)
	}
	if y := h.find("top-scroll").frame.Origin.Y; y != 0 {
		t.Errorf("top-scroll Origin.Y = %v, want 0 under large header", y)
	}
	scrollConfigs := map[NodeID]*driver.ScrollConfig{}
	for _, it := range b.Items {
		if it.Kind == driver.ItemScrollConfig {
			scrollConfigs[it.ID] = it.Scroll
		}
	}
	topCfg := scrollConfigs[h.find("top-scroll").id]
	if topCfg == nil || topCfg.HeaderInset != 176 || topCfg.Behavior.ContentInset.Top != 176 {
		t.Errorf("top-scroll config = %+v, want HeaderInset and ContentInset.Top 176", topCfg)
	}
	if hCfg := scrollConfigs[h.find("h-scroll").id]; hCfg == nil || hCfg.HeaderInset != 0 {
		t.Errorf("horizontal scroll config = %+v, want HeaderInset 0", hCfg)
	}
	if lowCfg := scrollConfigs[h.find("lower-scroll").id]; lowCfg == nil || lowCfg.HeaderInset != 0 {
		t.Errorf("lower scroll config = %+v, want HeaderInset 0", lowCfg)
	}
	// Platform measuring the expanded 152 dp bar keeps the header and inset unchanged;
	// a taller measured bar (164 dp) updates both.
	h.tree.Dispatch(driver.Event{Kind: driver.EventScreenHeader, ID: screen.id, Value: 164})
	b2 := h.commit()
	if got := screenItems(b2)[screen.id]; got.HeaderHeight != 188 {
		t.Errorf("measured android large header = %v, want 188 (164 + 24)", got.HeaderHeight)
	}
}

func TestScreenHeaderFontsFollowStyles(t *testing.T) {
	var resize *Signal[Points]
	h := newPhone(t, phone, func(cx *Scope) Node {
		resize = NewSignal(cx, Points(24))
		return Stack{Style: Style{FlexGrow: 1}, Children: []Node{
			Screen{Title: "Inbox", TestID: "styled", HeaderLargeTitle: true,
				HeaderTitle:               Text{Content: "Search"},
				HeaderTitleStyle:          Style{FontFamily: "serif", FontSize: 18, FontWeight: FontWeightMedium},
				HeaderTitleStyleFunc:      func() Style { return Style{FontSize: resize.Get()} },
				HeaderLargeTitleStyle:     Style{FontFamily: "monospace", FontWeight: FontWeightSemibold},
				HeaderLargeTitleStyleFunc: func() Style { return Style{FontSize: resize.Get() + 8} },
				Android:                   AndroidScreenProps{HeaderRightWithLargeTitle: true}},
		}}
	})
	check := func(size Points) {
		t.Helper()
		c := screenItems(h.commit())[h.find("styled").id]
		if !c.LargeTitle || c.TitleView == 0 || !c.RightWithLargeTitle {
			t.Fatalf("custom title removed the large title or placement: %+v", c)
		}
		if !c.TitleFont.Equal(driver.Font{Family: "serif", Size: size, Weight: FontWeightMedium}) ||
			!c.LargeTitleFont.Equal(driver.Font{Family: "monospace", Size: size + 8, Weight: FontWeightSemibold}) {
			t.Fatalf("title fonts: compact %+v, large %+v", c.TitleFont, c.LargeTitleFont)
		}
	}
	check(24)
	resize.Set(30)
	check(30)
}

// TestModalLifecycleReports checks EventModalShown and EventModalDismissed, including dismissals reported after ItemDelete (holding the modal's ID out of reuse until EventModalDismissed arrives) and swipes reported before it.
func TestModalLifecycleReports(t *testing.T) {
	var open, extra *Signal[bool]
	var log []string
	h := newPhone(t, phone, func(cx *Scope) Node {
		open = NewSignal(cx, true)
		extra = NewSignal(cx, false)
		return View{Style: Style{FlexGrow: 1}, Children: []Node{
			Show{When: open.Get, Then: func(*Scope) Node {
				return Modal{
					TestID:      "m",
					OnShown:     func() { log = append(log, "shown") },
					OnDismissed: func() { log = append(log, "dismissed") },
				}
			}},
			Show{When: extra.Get, Then: func(*Scope) Node { return View{TestID: "other"} }},
		}}
	})
	h.commit()
	modalID := h.find("m").id

	// Duplicate EventModalShown fires OnShown only once.
	h.tree.Dispatch(driver.Event{Kind: driver.EventModalShown, ID: modalID})
	h.tree.Dispatch(driver.Event{Kind: driver.EventModalShown, ID: modalID})
	if !slices.Equal(log, []string{"shown"}) {
		t.Fatalf("after EventModalShown: log = %q, want [shown]", log)
	}

	// Removing the modal emits ItemDelete and holds modalID out of free until EventModalDismissed arrives, so a new view cannot reuse it meanwhile.
	open.Set(false)
	b := h.commit()
	if !slices.ContainsFunc(b.Items, func(it driver.MountItem) bool { return it.Kind == driver.ItemDelete && it.ID == modalID }) {
		t.Fatal("closing the modal did not emit ItemDelete")
	}
	extra.Set(true)
	h.commit()
	if got := h.find("other").id; got == modalID {
		t.Fatalf("new view reused dismissing modal ID %d before EventModalDismissed", modalID)
	}

	// Late EventModalDismissed reaches OnDismissed once and frees modalID.
	h.tree.Dispatch(driver.Event{Kind: driver.EventModalDismissed, ID: modalID})
	h.tree.Dispatch(driver.Event{Kind: driver.EventModalDismissed, ID: modalID})
	if !slices.Equal(log, []string{"shown", "dismissed"}) {
		t.Fatalf("after EventModalDismissed: log = %q, want [shown dismissed]", log)
	}

	// Swipe-away: EventModalDismissed before ItemDelete fires once and does not double-fire when ItemDelete follows.
	log = nil
	open.Set(true)
	h.commit()
	sheetID := h.find("m").id
	h.tree.Dispatch(driver.Event{Kind: driver.EventModalShown, ID: sheetID})
	h.tree.Dispatch(driver.Event{Kind: driver.EventRequestClose, ID: sheetID, Text: "swipe", Finished: true})
	h.tree.Dispatch(driver.Event{Kind: driver.EventModalDismissed, ID: sheetID})
	open.Set(false)
	h.commit()
	h.tree.Dispatch(driver.Event{Kind: driver.EventModalDismissed, ID: sheetID})
	if !slices.Equal(log, []string{"shown", "dismissed"}) {
		t.Fatalf("after swipe-away: log = %q, want [shown dismissed]", log)
	}
}

// TestModalDismissalFallback checks that a removed modal whose platform never reports it gone gets OnDismissed and frees its ID once its animation and dismissFallback have passed, that a report arriving after that changes nothing, and that a report within the bound stops the fallback.
func TestModalDismissalFallback(t *testing.T) {
	var open *Signal[bool]
	var log []string
	h := newPhone(t, phone, func(cx *Scope) Node {
		open = NewSignal(cx, true)
		return View{Style: Style{FlexGrow: 1}, Children: []Node{
			Show{When: open.Get, Then: func(*Scope) Node {
				return Modal{TestID: "m", Animation: ModalFade, OnDismissed: func() { log = append(log, "dismissed") }}
			}},
		}}
	})
	h.commit()
	modalID := h.find("m").id
	open.Set(false)
	h.commit()
	wait := ModalFade.Duration() + dismissFallback
	h.clock.Advance(wait - time.Millisecond)
	h.drain()
	if len(log) != 0 || h.tree.dismissing[modalID].owners == nil {
		t.Fatalf("before the fallback: log = %q, held = %v", log, h.tree.dismissing[modalID].owners != nil)
	}
	h.clock.Advance(time.Millisecond)
	h.drain()
	if !slices.Equal(log, []string{"dismissed"}) {
		t.Fatalf("after the fallback: log = %q, want [dismissed]", log)
	}
	if _, held := h.tree.dismissing[modalID]; held || !slices.Contains(h.tree.free, modalID) {
		t.Fatalf("the fallback kept modal ID %d: held = %v, free = %v", modalID, held, h.tree.free)
	}
	// The platform's report, coming late, is dropped once.
	h.tree.Dispatch(driver.Event{Kind: driver.EventModalDismissed, ID: modalID})
	if !slices.Equal(log, []string{"dismissed"}) || h.tree.lateDismissals[modalID] {
		t.Fatalf("after a late report: log = %q, tombstone = %v", log, h.tree.lateDismissals[modalID])
	}

	// A report within the bound stops the fallback: OnDismissed runs once.
	log = nil
	open.Set(true)
	h.commit()
	again := h.find("m").id
	open.Set(false)
	h.commit()
	h.tree.Dispatch(driver.Event{Kind: driver.EventModalDismissed, ID: again})
	h.clock.Advance(2 * wait)
	h.drain()
	if !slices.Equal(log, []string{"dismissed"}) || h.tree.lateDismissals[again] {
		t.Fatalf("report then fallback time: log = %q, tombstone = %v", log, h.tree.lateDismissals[again])
	}
}

// TestScreenInsetsEventOverridesDerived checks that EventScreenInsets replaces core's derived screen insets and updates readers reactively.
func TestScreenInsetsEventOverridesDerived(t *testing.T) {
	var insets Edges
	h := newPhone(t, phone, func(cx *Scope) Node {
		return Stack{Style: Style{FlexGrow: 1}, Children: []Node{
			Screen{Title: "Home", TestID: "home", HeaderTransparent: true, Children: []Node{
				Func(func(cx *Scope) Node {
					cx.Effect(func() { insets = Insets(cx) })
					return View{TestID: "body"}
				}),
			}},
		}}
	})
	h.commit()
	if want := (Edges{Top: 84, Bottom: 20}); insets != want {
		t.Fatalf("derived insets = %+v, want %+v", insets, want)
	}
	screenID := h.find("home").id
	want := Edges{Top: 92, Right: 12, Bottom: 34, Left: 12}
	h.tree.Dispatch(driver.Event{Kind: driver.EventScreenInsets, ID: screenID, Insets: want})
	h.commit()
	if insets != want {
		t.Fatalf("reported insets = %+v, want %+v", insets, want)
	}
}

func TestScreenHeaderTitleFillsRoom(t *testing.T) {
	cases := []struct {
		name  string
		title Node
		probe string
		wantW func(room Points) Points
	}{
		{
			name: "row with growing search field",
			title: View{TestID: "row", Style: Style{Direction: Row, Gap: 8}, Children: []Node{
				View{Style: Style{Width: Pt(20), Height: Pt(20)}},
				View{TestID: "field", Style: Style{FlexGrow: 1, Height: Pt(28)}},
			}},
			probe: "field",
			wantW: func(room Points) Points { return room - 28 },
		},
		{
			name:  "view that only grows",
			title: View{TestID: "grow", Style: Style{FlexGrow: 1, Height: Pt(28)}},
			probe: "grow",
			wantW: func(room Points) Points { return room },
		},
		{
			name:  "percentage width (w-full)",
			title: View{TestID: "full", Class: "w-full", Style: Style{Height: Pt(28)}},
			probe: "full",
			wantW: func(room Points) Points { return room },
		},
		{
			name: "space-between row",
			title: View{TestID: "bar", Style: Style{Direction: Row, JustifyContent: JustifySpaceBetween}, Children: []Node{
				View{Style: Style{Width: Pt(24), Height: Pt(24)}},
				View{Style: Style{Width: Pt(24), Height: Pt(24)}},
			}},
			probe: "bar",
			wantW: func(room Points) Points { return room },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newPhone(t, phone, func(cx *Scope) Node {
				return Stack{Style: Style{FlexGrow: 1}, Children: []Node{
					Screen{
						Title:       "Search",
						TestID:      "screen",
						HeaderLeft:  View{Style: Style{Width: Pt(40), Height: Pt(24)}},
						HeaderRight: View{Style: Style{Width: Pt(50), Height: Pt(24)}},
						HeaderTitle: tc.title,
					},
				}}
			})
			b := h.commit()
			screen := h.find("screen")
			c := screenItems(b)[screen.id]
			if c.TitleView == 0 || !c.TitleFills {
				t.Fatalf("screen config = %+v, want TitleView with TitleFills=true", c)
			}
			// Initial estimate before the platform measures the bar: 300 - 40 - 50 = 210.
			if got, want := h.find(tc.probe).frame.Size.Width, tc.wantW(210); got != want {
				t.Fatalf("initial %s width = %v, want %v", tc.probe, got, want)
			}
			// Platform reports the exact room between the bar's ends (and bar height).
			h.tree.Dispatch(driver.Event{Kind: driver.EventScreenHeader, ID: screen.id, Value: 50, Size: Size{Width: 180}})
			b2 := h.commit()
			if got := screenItems(b2)[screen.id].HeaderHeight; got != 90 {
				t.Errorf("header height after report = %v, want 90", got)
			}
			if got, want := h.find(tc.probe).frame.Size.Width, tc.wantW(180); got != want {
				t.Fatalf("after platform report: %s width = %v, want %v", tc.probe, got, want)
			}
			// Platform widens the room (e.g. rotation or trailing button removed).
			h.tree.Dispatch(driver.Event{Kind: driver.EventScreenHeader, ID: screen.id, Size: Size{Width: 240}})
			h.commit()
			if got, want := h.find(tc.probe).frame.Size.Width, tc.wantW(240); got != want {
				t.Fatalf("after widen report: %s width = %v, want %v", tc.probe, got, want)
			}
		})
	}
}

func TestScreenHeaderTitleIntrinsicAndToggle(t *testing.T) {
	// Fixed-width view containing a growing child stays fixed-width (TitleFills=false).
	h := newPhone(t, phone, func(cx *Scope) Node {
		return Stack{Style: Style{FlexGrow: 1}, Children: []Node{
			Screen{
				Title:  "Fixed",
				TestID: "fixed",
				HeaderTitle: View{TestID: "box", Style: Style{Width: Pt(120), Direction: Row}, Children: []Node{
					View{TestID: "inner", Style: Style{FlexGrow: 1, Height: Pt(24)}},
				}},
			},
		}}
	})
	b := h.commit()
	if c := screenItems(b)[h.find("fixed").id]; c.TitleFills {
		t.Fatalf("fixed-width title with growing child set TitleFills=true: %+v", c)
	}
	if got := h.find("box").frame.Size.Width; got != 120 {
		t.Fatalf("box width = %v, want 120", got)
	}
	if got := h.find("inner").frame.Size.Width; got != 120 {
		t.Fatalf("inner width = %v, want 120", got)
	}

	// Dynamic toggle between an intrinsic title and a filling search row updates TitleFills.
	var searching *Signal[bool]
	h2 := newPhone(t, phone, func(cx *Scope) Node {
		searching = NewSignal(cx, false)
		return Stack{Style: Style{FlexGrow: 1}, Children: []Node{
			Screen{
				Title:  "Toggle",
				TestID: "toggle",
				HeaderTitle: Show{
					When: searching.Get,
					Then: func(*Scope) Node {
						return View{TestID: "search-row", Style: Style{Direction: Row}, Children: []Node{
							View{TestID: "search-field", Style: Style{FlexGrow: 1, Height: Pt(28)}},
						}}
					},
					Else: func(*Scope) Node {
						return View{TestID: "compact", Style: Style{Direction: Row, Gap: 6}, Children: []Node{
							View{Style: Style{Width: Pt(10), Height: Pt(10)}},
							View{Style: Style{Width: Pt(50), Height: Pt(18)}},
						}}
					},
				},
			},
		}}
	})
	screen := h2.find("toggle")
	if c := screenItems(h2.commit())[screen.id]; c.TitleFills {
		t.Fatalf("compact title set TitleFills=true: %+v", c)
	}
	if got := h2.find("compact").frame.Size.Width; got != 66 {
		t.Fatalf("compact width = %v, want 66", got)
	}

	searching.Set(true)
	if c, ok := screenItems(h2.commit())[screen.id]; !ok || !c.TitleFills {
		t.Fatalf("after enabling search: screen config = %+v (ok=%v), want TitleFills=true", c, ok)
	}
	h2.tree.Dispatch(driver.Event{Kind: driver.EventScreenHeader, ID: screen.id, Size: Size{Width: 210}})
	h2.commit()
	if got := h2.find("search-field").frame.Size.Width; got != 210 {
		t.Fatalf("search-field width = %v, want 210", got)
	}

	searching.Set(false)
	if c, ok := screenItems(h2.commit())[screen.id]; !ok || c.TitleFills {
		t.Fatalf("after disabling search: screen config = %+v (ok=%v), want TitleFills=false", c, ok)
	}
	if got := h2.find("compact").frame.Size.Width; got != 66 {
		t.Fatalf("compact width after toggle back = %v, want 66", got)
	}
}
