package base

import (
	"time"

	"github.com/oneo-me/nsgo/core"
)

// Popups (Popover, Tooltip, Menu, ContextMenu, Dialog, Sheet) share one
// contract, NSH's: Open makes them controlled and DefaultOpen starts them
// open otherwise; OnOpenChange reports each request that would change
// what is shown, with its reason, and a controlled popup shows what Open
// returns. OnOpen and OnClose follow it being shown and gone. OnCancel
// hears every attempt to close it (Escape or back, a press outside or on
// the backdrop), and NoCloseOn... flags keep it open on that attempt.

// Reason is why a popup asks to open or close.
type Reason uint8

const (
	ReasonTriggerPress Reason = iota + 1
	ReasonPressOutside
	ReasonBackdropPress
	ReasonCancel
	ReasonSelect
	ReasonHover
	ReasonLongPress
	ReasonContextMenu
)

func (r Reason) String() string {
	return [...]string{"", "triggerPress", "pressOutside", "backdropPress", "cancel", "select",
		"hover", "longPress", "contextMenu"}[r]
}

// popup is the open state of one popup.
type popup struct {
	open     func() bool
	own      *core.Signal[bool]
	onChange func(bool, Reason)
	onOpen   func()
	onClose  func()
	onCancel func()
}

func newPopup(cx *core.Scope, open func() bool, def bool, onChange func(bool, Reason), onOpen, onClose func()) *popup {
	p := &popup{open: open, onChange: onChange, onOpen: onOpen, onClose: onClose}
	if open == nil {
		p.own = core.NewSignal(cx, def)
		p.open = p.own.Get
	}
	if onOpen != nil || onClose != nil {
		var was bool
		cx.Effect(func() {
			is := p.open()
			switch {
			case is && !was && onOpen != nil:
				onOpen()
			case !is && was && onClose != nil:
				onClose()
			}
			was = is
		})
	}
	return p
}

// set asks to open or close the popup for reason; a request matching
// what is shown reports nothing.
func (p *popup) set(open bool, r Reason) {
	if p.open() == open {
		return
	}
	if p.own != nil {
		p.own.Set(open)
	}
	if p.onChange != nil {
		p.onChange(open, r)
	}
}

// cancel reports an attempt to close the popup, which closes it unless
// keep holds.
func (p *popup) cancel(r Reason, keep bool) {
	if p.onCancel != nil {
		p.onCancel()
	}
	if !keep {
		p.set(false, r)
	}
}

// Popover shows content beside its trigger: pressing the trigger opens
// it, pressing outside or Escape closes it.
type Popover struct {
	Open         func() bool
	DefaultOpen  bool
	OnOpenChange func(open bool, reason Reason)
	OnOpen       func()
	OnClose      func()
	// OnCancel hears every attempt to close it, allowed or not.
	OnCancel func()

	Placement             core.Placement
	Align                 core.Align
	Offset, Margin        core.Points
	NoCloseOnEscape       bool
	NoCloseOnPressOutside bool

	Style          core.Style // the trigger's
	StyleFunc      func() core.Style
	PopupStyle     core.Style
	PopupStyleFunc func() core.Style
	Trigger        core.Node
	Children       []core.Node
	Label          string
	TestID         string
}

// Render implements core.Node.
func (p Popover) Render(cx *core.Scope) core.Node {
	st := newPopup(cx, p.Open, p.DefaultOpen, p.OnOpenChange, p.OnOpen, p.OnClose)
	st.onCancel = p.OnCancel
	anchor := new(core.Ref)
	return core.Group{
		core.View{Ref: anchor, Style: p.Style, StyleFunc: p.StyleFunc, Label: p.Label, TestID: p.TestID, Role: core.RoleButton,
			OnPress:  func() { st.set(!st.open(), ReasonTriggerPress) },
			Children: []core.Node{p.Trigger}},
		core.Show{When: st.open, Then: func(cx *core.Scope) core.Node {
			return core.Overlay{
				Anchor: anchor, Placement: p.Placement, Align: p.Align, Offset: p.Offset, Margin: p.Margin,
				Style: p.PopupStyle, StyleFunc: p.PopupStyleFunc, Children: p.Children, Role: core.RoleDialog,
				OnPressOutside: func(core.Point) { st.cancel(ReasonPressOutside, p.NoCloseOnPressOutside) },
				OnCancel:       func() { st.cancel(ReasonCancel, p.NoCloseOnEscape) },
			}
		}},
	}
}

// Tooltip shows a hint beside its trigger while the pointer rests on it
// (on desktops), or while the app opens it.
type Tooltip struct {
	Open         func() bool
	DefaultOpen  bool
	OnOpenChange func(open bool, reason Reason)
	OnOpen       func()
	OnClose      func()

	// Placement defaults to above the trigger.
	Placement core.Placement
	Align     core.Align
	// Offset and Margin are 4 when 0.
	Offset, Margin core.Points
	// Delay before it opens on hover; half a second when 0.
	Delay    time.Duration
	Disabled bool

	Style          core.Style // the trigger's
	StyleFunc      func() core.Style
	PopupStyle     core.Style
	PopupStyleFunc func() core.Style
	Trigger        core.Node
	Children       []core.Node
	TestID         string
}

// Render implements core.Node.
func (t Tooltip) Render(cx *core.Scope) core.Node {
	st := newPopup(cx, t.Open, t.DefaultOpen, t.OnOpenChange, t.OnOpen, t.OnClose)
	anchor := new(core.Ref)
	delay := t.Delay
	if delay == 0 {
		delay = 500 * time.Millisecond
	}
	place := t.Placement
	if place == core.PlaceDefault {
		place = core.PlaceTop
	}
	offset, margin := t.Offset, t.Margin
	if offset == 0 {
		offset = 4
	}
	if margin == 0 {
		margin = 4
	}
	var stop func() bool
	return core.Group{
		core.View{Ref: anchor, Style: t.Style, StyleFunc: t.StyleFunc, TestID: t.TestID,
			OnHoverIn: func() {
				if !t.Disabled && cx.Env().Desktop() {
					stop = cx.AfterFunc(delay, func() { st.set(true, ReasonHover) })
				}
			},
			OnHoverOut: func() {
				if stop != nil {
					stop()
				}
				st.set(false, ReasonHover)
			},
			Children: []core.Node{t.Trigger}},
		core.Show{When: st.open, Then: func(cx *core.Scope) core.Node {
			return core.Overlay{Anchor: anchor, Placement: place, Align: t.Align, Offset: offset, Margin: margin,
				Style: t.PopupStyle, StyleFunc: t.PopupStyleFunc, Children: t.Children, Role: core.RoleAlert}
		}},
	}
}

// Dialog is a modal layer over the app: an optional backdrop and a
// popup centered on it.
type Dialog struct {
	Open         func() bool
	DefaultOpen  bool
	OnOpenChange func(open bool, reason Reason)
	OnOpen       func()
	OnClose      func()
	OnCancel     func()

	Animation              core.ModalAnimation
	NoCloseOnEscape        bool
	NoCloseOnBackdropPress bool
	// BackdropStyle or BackdropStyleFunc, when set, draws a backdrop whose
	// press asks to close.
	BackdropStyle     core.Style
	BackdropStyleFunc func() core.Style
	PopupStyle        core.Style
	PopupStyleFunc    func() core.Style
	Children          []core.Node
	Label             string
	TestID            string
}

// Render implements core.Node.
func (d Dialog) Render(cx *core.Scope) core.Node {
	st := newPopup(cx, d.Open, d.DefaultOpen, d.OnOpenChange, d.OnOpen, d.OnClose)
	st.onCancel = d.OnCancel
	return modalLayer(st, d.Animation, d.BackdropStyle, d.BackdropStyleFunc, d.NoCloseOnBackdropPress,
		d.NoCloseOnEscape, core.Style{JustifyContent: core.JustifyCenter, AlignItems: core.AlignCenter},
		d.PopupStyle, d.PopupStyleFunc, d.Children, d.Label, d.TestID)
}

// Sheet slides a panel in from a side of the app, over a backdrop.
type Sheet struct {
	Open         func() bool
	DefaultOpen  bool
	OnOpenChange func(open bool, reason Reason)
	OnOpen       func()
	OnClose      func()
	OnCancel     func()

	// Side is the edge the panel sits on; the bottom by default.
	Side                   core.Placement
	NoCloseOnEscape        bool
	NoCloseOnBackdropPress bool
	BackdropStyle          core.Style
	BackdropStyleFunc      func() core.Style
	PopupStyle             core.Style
	PopupStyleFunc         func() core.Style
	Children               []core.Node
	Label                  string
	TestID                 string
}

// Render implements core.Node.
func (s Sheet) Render(cx *core.Scope) core.Node {
	st := newPopup(cx, s.Open, s.DefaultOpen, s.OnOpenChange, s.OnOpen, s.OnClose)
	st.onCancel = s.OnCancel
	layer := core.Style{JustifyContent: core.JustifyEnd}
	switch s.Side {
	case core.PlaceTop:
		layer = core.Style{JustifyContent: core.JustifyStart}
	case core.PlaceLeft:
		layer = core.Style{Direction: core.Row, JustifyContent: core.JustifyStart}
	case core.PlaceRight:
		layer = core.Style{Direction: core.Row, JustifyContent: core.JustifyEnd}
	}
	return modalLayer(st, core.ModalSlide, s.BackdropStyle, s.BackdropStyleFunc, s.NoCloseOnBackdropPress,
		s.NoCloseOnEscape, layer, s.PopupStyle, s.PopupStyleFunc, s.Children, s.Label, s.TestID)
}

var fill = core.Style{Position: core.Absolute, Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Left: core.Pt(0)}

func modalLayer(st *popup, anim core.ModalAnimation, backdrop core.Style, backdropFunc func() core.Style,
	keepOnBackdrop, keepOnEscape bool, layer, popupStyle core.Style, popupFunc func() core.Style,
	children []core.Node, label, testID string) core.Node {
	return core.Show{When: st.open, Then: func(cx *core.Scope) core.Node {
		var nodes []core.Node
		if backdrop != (core.Style{}) || backdropFunc != nil {
			nodes = append(nodes, core.View{Style: fill.Merge(backdrop), StyleFunc: backdropFunc,
				OnPress: func() { st.cancel(ReasonBackdropPress, keepOnBackdrop) }})
		}
		nodes = append(nodes, core.View{Style: popupStyle, StyleFunc: popupFunc, Role: core.RoleDialog,
			Label: label, TestID: testID, Children: children})
		return core.Modal{Animation: anim, Style: layer, Children: nodes,
			OnRequestClose: func(string) { st.cancel(ReasonCancel, keepOnEscape) }}
	}}
}
