package ui

import (
	"math"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

// sidebarSlide is how long the sidebar takes to show or hide.
const sidebarSlide = 220 * time.Millisecond

// SidebarShellVariant is how a SidebarShell shows its content.
type SidebarShellVariant uint8

const (
	// SidebarShellPlain puts the content on the page's background from
	// the sidebar to the window's edges. It is the default.
	SidebarShellPlain SidebarShellVariant = iota
	// SidebarShellInset floats the content as a rounded card on the page's
	// background at 90%, the window keeping a margin of 5 points around
	// the sidebar and the card, and between them, while the sidebar
	// shows, the gap between them its handle; the card's edge, outside
	// it, and its shadow lift it off the chrome there, in place of the
	// sidebar's line. The margin goes as the sidebar hides, the window
	// controls and the toggle moving with it as it slides, and the edge
	// and the shadow fade as the last of the sidebar slides away.
	SidebarShellInset
)

// The margin of SidebarShellInset, its card's corners, and how much of
// the sidebar is left to show when its card's edge and shadow start to
// fade as it hides.
const (
	shellInset       core.Points = 5
	shellInsetRadius core.Points = 10
	insetFade        core.Points = 0.25
)

// SidebarShell is a window of a sidebar beside the content, each under a
// header: the sidebar's toggle stays after the window controls, the
// app's SidebarActions end the sidebar's header, and its Header and
// Actions are the content's. Hiding the sidebar slides the content over
// it, the sidebar keeping its width, and the content's header with it
// until it meets the toggle, where it stays; its edge is a handle the user drags
// to resize it and double-clicks to restore DefaultWidth. SidebarFooter
// is a bar at the sidebar's bottom, with no line above it.
//
// Fixed keeps the sidebar at its width, its edge a line with no handle.
//
// Open and Width are the app's, set from OnOpenChange and OnWidthChange.
// The sidebar and its header are the chrome (Chrome), the content and
// its header on the page's background, and the right panel on that
// background at 80%. Variant chooses how the content shows on the chrome,
// and ChromeStyle, when set, paints the chrome in place of Chrome, such
// as with a gradient; it is a StyleFunc. Backdrop is drawn over the
// chrome, filling the window behind the sidebar and the content, such as
// a texture; it takes no presses.
type SidebarShell struct {
	Variant        SidebarShellVariant
	ChromeStyle    func() core.Style
	Backdrop       []core.Node
	Sidebar        []core.Node
	SidebarActions []core.Node
	SidebarFooter  []core.Node
	Header         []core.Node
	Actions        []core.Node
	Content        []core.Node
	Open           func() bool
	OnOpenChange   func(open bool)
	// ToggleLabel names the toggle, which shows while OnOpenChange is set.
	ToggleLabel string
	// Width is the sidebar's width, DefaultWidth (240 points when 0)
	// where it is nil or 0. The user drags it between MinWidth (160 when
	// 0) and MaxWidth (none when 0) on the handle named ResizeLabel.
	Width              func() core.Points
	OnWidthChange      func(width core.Points)
	DefaultWidth       core.Points
	MinWidth, MaxWidth core.Points
	ResizeLabel        string
	Fixed              bool
	// Right is a panel at the content's end under RightHeader, and
	// RightOpen whether it shows, with a toggle ending the content's
	// header, named RightToggleLabel, while OnRightOpenChange is set. The
	// content slides over it as over the sidebar, and draws its line, a
	// handle of its own named RightResizeLabel, on its edge; RightWidth,
	// RightDefaultWidth (280 when 0), RightMinWidth (200 when 0) and
	// RightMaxWidth (none when 0) work as the sidebar's do.
	Right, RightHeader                 []core.Node
	RightOpen                          func() bool
	OnRightOpenChange                  func(open bool)
	RightToggleLabel, RightResizeLabel string
	RightWidth                         func() core.Points
	OnRightWidthChange                 func(width core.Points)
	RightDefaultWidth                  core.Points
	RightMinWidth, RightMaxWidth       core.Points
	TestID                             string
}

// Render implements core.Node.
func (s SidebarShell) Render(cx *core.Scope) core.Node {
	hideTitleBar(cx)
	colors := ColorsOf(cx)
	env := cx.Env()
	open := s.Open
	if open == nil {
		open = func() bool { return true }
	}
	closed := func() bool { return !open() }
	def, lo, hi := s.DefaultWidth, s.MinWidth, s.MaxWidth
	if lo == 0 {
		lo = 160
	}
	if hi == 0 {
		hi = core.Points(math.MaxFloat32)
	}
	hi = max(hi, lo)
	if def == 0 {
		def = 240
	}
	def = min(max(def, lo), hi)
	own := core.NewSignal(cx, def)
	chosen := func() core.Points {
		if s.Width == nil {
			return own.Get()
		}
		if w := s.Width(); w > 0 {
			return min(max(w, lo), hi)
		}
		return def
	}
	// width is what shows: the handle stays within the window.
	width := func() core.Points { return max(min(chosen(), env.Window().Width-12), 0) }
	resize := func(w core.Points) {
		w = min(max(w, lo), hi)
		if s.Width == nil {
			own.Set(w)
		}
		if s.OnWidthChange != nil {
			s.OnWidthChange(w)
		}
	}

	inset := s.Variant == SidebarShellInset
	// reveal is how much of the sidebar shows, from 0 to 1, moved a
	// display frame at a time with the content's title and, in the inset
	// variant, the margin and the window controls.
	shown, reveal := slideFrames(cx, open)
	// margin is the inset variant's around the sidebar and the content,
	// by which the window controls and the toggle move too.
	margin := func() core.Points {
		if inset {
			return shellInset * reveal()
		}
		return 0
	}
	controls := func() core.Points { return controlsEnd(cx) }
	if inset {
		controls = insetControls(cx, margin, func() bool { r := reveal(); return r == 0 || r == 1 })
	}

	// The toggle follows the window controls, which keep a margin of
	// their own, or the window's edge at a margin; the headers keep clear
	// of it.
	toggleStart := func() core.Points {
		if desktop.TitleBar(cx).Left > 0 {
			return controls()
		}
		return controls() + 12 + margin()
	}
	const toggleSize, toggleIcon = 32, 16.25
	reserve := controls
	var toggle core.Node
	if s.OnOpenChange != nil {
		toggle = core.View{StyleFunc: func() core.Style {
			return core.Style{Position: core.Absolute, Top: core.Pt(core.Insets(cx).Top + margin()), Left: core.Pt(toggleStart()),
				Height: core.Pt(ShellHeaderHeight(cx)), Direction: core.Row, AlignItems: core.AlignCenter, ZIndex: 2}
		}, Children: []core.Node{IconButton{Icon: lucide.PanelLeft, Label: s.ToggleLabel, Size: toggleSize, IconSize: toggleIcon,
			OnPress: func() { s.OnOpenChange(!open()) }}}}
		reserve = func() core.Points { return toggleStart() - 12 + toggleSize + 8 }
	}

	sidebar := []core.Node{
		shellHeader(cx, "sidebar-header", func() core.Points { return reserve() - margin() }, false, func() core.Color { return 0 },
			append([]core.Node{Spacer{}}, s.SidebarActions...)),
		clearOfSides(FillRest, func() core.EnvEdges { return core.EnvEdges{Left: core.EnvSafeArea} }, core.View{Style: FillRest, Children: s.Sidebar}),
	}
	if s.SidebarFooter != nil {
		sidebar = append(sidebar, clearOfSides(core.Style{FlexShrink: 0}, func() core.EnvEdges { return core.EnvEdges{Left: core.EnvSafeArea} },
			core.View{TestID: "sidebar-footer", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4, Padding: core.All(8)}, Children: s.SidebarFooter}))
	}
	// The slot narrows over the sidebar, which keeps its width.
	slot := core.View{TestID: "sidebar-slot", StyleFunc: func() core.Style {
		return core.Style{Width: core.Pt(width() * reveal()), FlexShrink: 0, Overflow: core.Hidden}
	}, Children: []core.Node{core.View{TestID: "sidebar-panel", Disabled: closed, Children: sidebar, StyleFunc: func() core.Style {
		st := core.Style{Width: core.Pt(width()), Height: core.Pct(100), MinHeight: core.Pt(0)}
		if !shown.Get() {
			st.Visibility = core.VisibilityHidden
		}
		return st
	}}}}

	var right, rightHandle core.Node
	if s.Right != nil {
		right, rightHandle = s.rightPanel(cx)
	}
	head := []core.Node{core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
		FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Height: core.Pct(100), Overflow: core.Hidden}, Children: s.Header}}
	headerFill := colors.Background
	if inset {
		headerFill = func() core.Color { return 0 }
	}
	var content core.Node = core.View{TestID: "content", Style: FillRest.Merge(core.Style{Overflow: core.Hidden}), StyleFunc: Filled(colors.Background), Children: []core.Node{
		shellHeader(cx, "content-header", func() core.Points {
			// The title slides with the content until it meets the toggle,
			// where it stays.
			return max(reserve()-(2*margin()+width()*reveal()), 0)
		}, true, headerFill, append(append(head, s.Actions...), s.rightToggle()...)),
		// The content keeps clear of the right inset unless the right panel is open there, and of the left while the sidebar is closed.
		clearOfSides(FillRest, func() core.EnvEdges {
			var e core.EnvEdges
			if !s.rightOpen() {
				e.Right = core.EnvSafeArea
			}
			if !open() {
				e.Left = core.EnvSafeArea
			}
			return e
		}, core.View{Style: FillRest, Children: s.Content}),
		rightHandle,
	}}
	if inset {
		content = insetCard(cx, content.(core.View), reveal, margin)
	}

	// The handle's band, 3 points across, centers its line on the
	// sidebar's last point; a double click restores the default width.
	// In the inset variant the band fills the gap before the card, whose
	// edge is the line there.
	line := HandleLine(cx)
	band, lineMargin := core.Points(0), core.Edges{Left: 1}
	if inset {
		line = func() core.Style { return core.Style{} }
		band, lineMargin = shellInset, core.Edges{}
	}
	var start core.Points
	var edge core.Node = base.ResizeHandle{Label: s.ResizeLabel, Edge: base.HandleLeading, Thickness: band, Disabled: closed,
		Value: func() core.AccessibilityValue {
			return core.AccessibilityValue{Now: float32(chosen()), Min: float32(lo), Max: float32(min(hi, env.Window().Width))}
		},
		OnDragStart:   func() { start = chosen() },
		OnDragMove:    func(d core.Points) { resize(min(start+d, env.Window().Width-12)) },
		OnDragEnd:     func(d core.Points) { resize(min(start+d, env.Window().Width-12)) },
		OnDoublePress: func() { resize(def) },
		Style:         core.Style{Margin: lineMargin}, StyleFunc: line,
		ThumbStyle: HandleThumb(cx), ThumbStyleFunc: HandleThumbColor(cx)}
	if s.Fixed {
		edge = core.View{Style: core.Style{Width: core.Pt(1), Height: core.Pct(100), Margin: core.Edges{Left: 1}}, StyleFunc: line}
	}
	handle := core.View{TestID: "sidebar-resize", Disabled: closed, StyleFunc: func() core.Style {
		st := core.Style{Position: core.Absolute, Top: core.Pt(0), Bottom: core.Pt(0), Width: core.Pt(3), ZIndex: 1,
			Left: core.Pt(width()*reveal() - 2)}
		if inset {
			st.Width, st.Left = core.Pt(shellInset), core.Pt(margin()+width()*reveal())
		}
		if s.Fixed {
			st.PointerEvents = core.PointerEventsNone
		}
		if !shown.Get() {
			st.Visibility = core.VisibilityHidden
		}
		return st
	}, Children: []core.Node{edge}}

	row := []core.Node{slot, content}
	if right != nil {
		row = append(row, right)
	}
	var children []core.Node
	if s.Backdrop != nil {
		children = append(children, core.View{TestID: "backdrop", Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0),
			Right: core.Pt(0), Bottom: core.Pt(0), PointerEvents: core.PointerEventsNone}, Children: s.Backdrop})
	}
	children = append(children, core.View{Style: FillRest.Merge(core.Style{Direction: core.Row}), StyleFunc: func() core.Style {
		return core.Style{Margin: core.All(margin())}
	}, Children: row}, handle)
	if toggle != nil {
		children = append(children, toggle)
	}
	return core.View{TestID: s.TestID, Style: FillRest, StyleFunc: func() core.Style {
		if s.ChromeStyle != nil {
			return s.ChromeStyle().Merge(core.Style{Color: colors.Text()})
		}
		return core.Style{Background: Chrome(cx), Color: colors.Text()}
	}, Children: children}
}

// clearOfSides is a pane's body, inner, padded by the side safe area on the edges that edges names, the ones at the window's edge. What inner holds keeps clear of insets without the sides, so a NavHeader or a page that pads itself by core.EnvSafeArea is not padded twice, as ui.TabShell's sections keep clear of insets without the bottom its bar keeps clear of.
func clearOfSides(style core.Style, edges func() core.EnvEdges, inner core.View) core.Node {
	return core.View{Style: style, StyleFunc: func() core.Style { return core.Style{PaddingEnv: edges()} }, Children: []core.Node{core.Func(func(cx *core.Scope) core.Node {
		outer := core.InsetsFunc(cx)
		core.ProvideInsets(cx, func() core.Edges {
			in := outer()
			in.Left, in.Right = 0, 0
			return in
		})
		return inner
	})}}
}

// insetCard is SidebarShellInset's card around the content, which clips
// it: apart from the sidebar by margin, with an edge and a shadow while
// the sidebar shows, which fade as the last of it, insetFade of reveal,
// hides. The edge is an outline around the card, as a dialog's rim is
// outside what it holds, so the content keeps the card's whole frame;
// the shadow is the card's and the clip the content's, so it falls
// outside.
func insetCard(cx *core.Scope, content core.View, reveal func() core.Points, margin func() core.Points) core.Node {
	colors := ColorsOf(cx)
	content.TestID = ""
	content.Style.CornerRadius = shellInsetRadius
	content.StyleFunc = nil
	return core.View{TestID: "content", Style: FillRest, Children: []core.Node{content}, StyleFunc: func() core.Style {
		st := core.Style{Background: Alpha(colors.Background(), 0.9), CornerRadius: shellInsetRadius, Margin: core.Edges{Left: margin()}}
		if k := float32(min(reveal()/insetFade, 1)); k > 0 {
			line := colors.Line()
			st.OutlineWidth, st.OutlineColor = 1, Mix(Alpha(line, 0), line, k)
			st.Shadows = []core.BoxShadow{
				{Offset: core.Point{Y: 1}, Blur: 2, Color: Black(0.05 * k)},
				{Offset: core.Point{Y: 4}, Blur: 16, Color: Black(0.07 * k)},
			}
		}
		return st
	}}
}

// slideShown is whether a panel that open shows is seen: it hides once the
// content has slid over it, out of the pointer's and screen readers' way.
// Only a change of open slides: open may follow more of the app's state,
// whose other changes would otherwise animate whatever moves with them.
func slideShown(cx *core.Scope, open func() bool) *core.Signal[bool] {
	env := cx.Env()
	was := core.Untracked(cx, open)
	shown := core.NewSignal(cx, was)
	var stop func() bool
	cx.Effect(func() {
		o := open()
		if o == was {
			return
		}
		was = o
		cx.Untrack(func() {
			if !env.ReduceMotion() {
				cx.AnimateLayout(core.LayoutAnimation{Update: &core.LayoutTransition{Curve: core.LayoutEaseInEaseOut, Duration: sidebarSlide}})
			}
			if stop != nil {
				stop()
			}
			if o {
				shown.Set(true)
				return
			}
			stop = cx.AfterFunc(sidebarSlide, func() { shown.Set(open()) })
		})
	})
	return shown
}

// slideFrames is slideShown for a slide the shell moves itself a display
// frame at a time: whether the sidebar is seen, and how much of it shows.
func slideFrames(cx *core.Scope, open func() bool) (*core.Signal[bool], func() core.Points) {
	env := cx.Env()
	was := core.Untracked(cx, open)
	shown := core.NewSignal(cx, was)
	var at core.Points
	if was {
		at = 1
	}
	reveal := core.NewSignal(cx, at)
	var stop func()
	cx.Effect(func() {
		o := open()
		if o == was {
			return
		}
		was = o
		cx.Untrack(func() {
			if stop != nil {
				stop()
				stop = nil
			}
			var to core.Points
			if o {
				to = 1
				shown.Set(true)
			}
			from := reveal.Get()
			if env.ReduceMotion() {
				reveal.Set(to)
				shown.Set(o)
				return
			}
			stop = cx.Animate(sidebarSlide, core.EaseInOut, func(p float64) {
				reveal.Set(from + (to-from)*core.Points(p))
				if p == 1 && !o {
					shown.Set(false)
				}
			})
		})
	})
	return shown, reveal.Get
}

// insetControls moves the window controls by margin, and returns where
// they end. The title bar follows them a frame or more behind as they
// move, those at the window's start by margin, so where they end without
// it is taken only while resting says they rest.
func insetControls(cx *core.Scope, margin func() core.Points, resting func() bool) func() core.Points {
	type rest struct {
		end   core.Points
		moved bool
	}
	at := func() rest {
		r := rest{end: controlsEnd(cx), moved: desktop.TitleBar(cx).Left > 0}
		if r.moved {
			r.end -= margin()
		}
		return r
	}
	settled := core.NewSignal(cx, core.Untracked(cx, at))
	cx.Effect(func() {
		if resting() {
			r := at()
			cx.Untrack(func() { settled.Set(r) })
		}
	})
	if desktop.IsAvailable(cx) {
		cx.Effect(func() {
			d := margin()
			desktop.SetWindowControls(cx, desktop.WindowControls{Offset: core.Point{X: d, Y: d}})
		})
		cx.Cleanup(func() { desktop.SetWindowControls(cx, desktop.WindowControls{}) })
	}
	return func() core.Points {
		r := settled.Get()
		if r.moved {
			return r.end + margin()
		}
		return r.end
	}
}

// rightToggle is the toggle of the right panel, when it has one.
func (s SidebarShell) rightToggle() []core.Node {
	if s.Right == nil || s.OnRightOpenChange == nil {
		return nil
	}
	return []core.Node{IconButton{Icon: lucide.PanelRight, Label: s.RightToggleLabel, Size: 32, IconSize: 16.25,
		OnPress: func() { s.OnRightOpenChange(!s.rightOpen()) }}}
}

func (s SidebarShell) rightOpen() bool { return s.RightOpen != nil && s.RightOpen() }

// rightPanel is the panel at the content's end, which stays against the
// window's edge as the content slides over it, and the handle on the
// content's edge, whose line moves with the content.
func (s SidebarShell) rightPanel(cx *core.Scope) (panel, handle core.Node) {
	colors := ColorsOf(cx)
	env := cx.Env()
	def, lo, hi := s.RightDefaultWidth, s.RightMinWidth, s.RightMaxWidth
	if lo == 0 {
		lo = 200
	}
	if hi == 0 {
		hi = core.Points(math.MaxFloat32)
	}
	hi = max(hi, lo)
	if def == 0 {
		def = 280
	}
	def = min(max(def, lo), hi)
	own := core.NewSignal(cx, def)
	chosen := func() core.Points {
		if s.RightWidth == nil {
			return own.Get()
		}
		if w := s.RightWidth(); w > 0 {
			return min(max(w, lo), hi)
		}
		return def
	}
	width := func() core.Points { return max(min(chosen(), env.Window().Width-12), 0) }
	resize := func(w core.Points) {
		w = min(max(w, lo), hi)
		if s.RightWidth == nil {
			own.Set(w)
		}
		if s.OnRightWidthChange != nil {
			s.OnRightWidthChange(w)
		}
	}
	shown := slideShown(cx, s.rightOpen)
	closed := func() bool { return !s.rightOpen() }
	var start core.Points
	panel = core.View{TestID: "right-slot", StyleFunc: func() core.Style {
		var w core.Points
		if s.rightOpen() {
			w = width()
		}
		return core.Style{Width: core.Pt(w), FlexShrink: 0, Height: core.Pct(100), AlignItems: core.AlignEnd, Overflow: core.Hidden}
	}, Children: []core.Node{core.Show{When: shown.Get, Then: func(cx *core.Scope) core.Node {
		head := []core.Node{shellHeader(cx, "right-header", func() core.Points { return 0 }, false, func() core.Color { return 0 }, s.RightHeader)}
		return core.View{TestID: "right-panel", Disabled: closed, StyleFunc: func() core.Style {
			return core.Style{Width: core.Pt(width()), FlexShrink: 0, Height: core.Pct(100), MinHeight: core.Pt(0), Background: Alpha(colors.Background(), 0.8), Color: colors.Text()}
		}, Children: append(head, clearOfSides(FillRest, func() core.EnvEdges { return core.EnvEdges{Right: core.EnvSafeArea} }, core.View{Style: FillRest, Children: s.Right}))}
	}}}}
	handle = core.View{TestID: "right-resize", Disabled: closed, StyleFunc: func() core.Style {
		st := core.Style{Position: core.Absolute, Top: core.Pt(0), Bottom: core.Pt(0), Right: core.Pt(0), Width: core.Pt(3), ZIndex: 1}
		if !shown.Get() {
			st.Visibility = core.VisibilityHidden
		}
		return st
	}, Children: []core.Node{base.ResizeHandle{Label: s.RightResizeLabel, Edge: base.HandleTrailing, Disabled: closed,
		Value: func() core.AccessibilityValue {
			return core.AccessibilityValue{Now: float32(chosen()), Min: float32(lo), Max: float32(min(hi, env.Window().Width))}
		},
		OnDragStart:   func() { start = chosen() },
		OnDragMove:    func(d core.Points) { resize(min(start-d, env.Window().Width-12)) },
		OnDragEnd:     func(d core.Points) { resize(min(start-d, env.Window().Width-12)) },
		OnDoublePress: func() { resize(def) },
		StyleFunc:     HandleLine(cx), ThumbStyle: HandleThumb(cx), ThumbStyleFunc: HandleThumbColor(cx)}}}
	return panel, handle
}
