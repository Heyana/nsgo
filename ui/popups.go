package ui

import (
	"time"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

// Popups behave as base's do; see base for Open, OnOpenChange and the
// other fields they pass on. A trigger has the look of a button of
// Variant, and is Label as text when Trigger is nil; Style merges over the
// trigger's look and PopupStyle over the popup's.

// Menu is a base.Menu on the theme's raised surface.
type Menu struct {
	Open           func() bool
	DefaultOpen    bool
	OnOpenChange   func(open bool, reason base.Reason)
	OnOpen         func()
	OnClose        func()
	OnCancel       func()
	Placement      core.Placement
	Align          core.Align
	Offset, Margin core.Points

	Variant    Variant
	Style      core.Style
	PopupStyle core.Style
	Trigger    core.Node
	Children   []core.Node
	Label      string
	TestID     string
}

// Render implements core.Node.
func (m Menu) Render(cx *core.Scope) core.Node {
	return base.Menu{
		Open: m.Open, DefaultOpen: m.DefaultOpen, OnOpenChange: m.OnOpenChange,
		OnOpen: m.OnOpen, OnClose: m.OnClose, OnCancel: m.OnCancel,
		Placement: m.Placement, Align: m.Align, Offset: m.Offset, Margin: m.Margin,
		StyleFunc:      trigger(cx, m.Variant, m.Style),
		PopupStyleFunc: look(cx, m.PopupStyle, Tokens.popup),
		Trigger:        orText(m.Trigger, m.Label), Children: m.Children, Label: m.Label, TestID: m.TestID,
	}
}

// ContextMenu is a base.ContextMenu on the theme's raised surface. Its
// content keeps Style, with no look of its own.
type ContextMenu struct {
	Open           func() bool
	OnOpenChange   func(open bool, reason base.Reason)
	OnOpen         func()
	OnClose        func()
	OnCancel       func()
	Disabled       bool
	Offset, Margin core.Points

	Style      core.Style
	PopupStyle core.Style
	Trigger    core.Node
	Children   []core.Node
	TestID     string
}

// Render implements core.Node.
func (m ContextMenu) Render(cx *core.Scope) core.Node {
	return base.ContextMenu{
		Open: m.Open, OnOpenChange: m.OnOpenChange, OnOpen: m.OnOpen, OnClose: m.OnClose, OnCancel: m.OnCancel,
		Disabled: m.Disabled, Offset: m.Offset, Margin: m.Margin,
		Style: m.Style, PopupStyleFunc: look(cx, m.PopupStyle, Tokens.popup),
		Trigger: m.Trigger, Children: m.Children, TestID: m.TestID,
	}
}

// MenuItem is a base.MenuItem highlighted while hovered; its content is
// its children or Label as text.
type MenuItem struct {
	OnSelect        func()
	Disabled        func() bool
	NoCloseOnSelect bool
	Style           core.Style
	Children        []core.Node
	Label           string
	TestID          string
}

// Render implements core.Node.
func (i MenuItem) Render(cx *core.Scope) core.Node {
	return base.MenuItem{OnSelect: i.OnSelect, Disabled: i.Disabled, NoCloseOnSelect: i.NoCloseOnSelect,
		StyleFunc: look(cx, i.Style, Tokens.menuItem), Children: orLabel(i.Children, i.Label),
		Label: i.Label, TestID: i.TestID}
}

// MenuSeparator is a base.MenuSeparator drawn as a line.
type MenuSeparator struct{ Style core.Style }

// Render implements core.Node.
func (s MenuSeparator) Render(cx *core.Scope) core.Node {
	return base.MenuSeparator{StyleFunc: look(cx, s.Style, Tokens.menuSeparator)}
}

// Submenu is a base.Submenu: an item that looks like a MenuItem and opens
// a popup like a Menu's. Its trigger is Label as text when Trigger is nil.
type Submenu struct {
	Disabled   func() bool
	Style      core.Style
	PopupStyle core.Style
	Trigger    core.Node
	Children   []core.Node
	Label      string
	TestID     string
}

// Render implements core.Node.
func (s Submenu) Render(cx *core.Scope) core.Node {
	return base.Submenu{Disabled: s.Disabled,
		StyleFunc: look(cx, s.Style, Tokens.menuItem), PopupStyleFunc: look(cx, s.PopupStyle, Tokens.popup),
		Trigger: orText(s.Trigger, s.Label), Children: s.Children, Label: s.Label, TestID: s.TestID}
}

// Popover is a base.Popover on the theme's raised surface.
type Popover struct {
	Open                  func() bool
	DefaultOpen           bool
	OnOpenChange          func(open bool, reason base.Reason)
	OnOpen                func()
	OnClose               func()
	OnCancel              func()
	Placement             core.Placement
	Align                 core.Align
	Offset, Margin        core.Points
	NoCloseOnEscape       bool
	NoCloseOnPressOutside bool

	Variant    Variant
	Style      core.Style
	PopupStyle core.Style
	Trigger    core.Node
	Children   []core.Node
	Label      string
	TestID     string
}

// Render implements core.Node.
func (p Popover) Render(cx *core.Scope) core.Node {
	return base.Popover{
		Open: p.Open, DefaultOpen: p.DefaultOpen, OnOpenChange: p.OnOpenChange,
		OnOpen: p.OnOpen, OnClose: p.OnClose, OnCancel: p.OnCancel,
		Placement: p.Placement, Align: p.Align, Offset: p.Offset, Margin: p.Margin,
		NoCloseOnEscape: p.NoCloseOnEscape, NoCloseOnPressOutside: p.NoCloseOnPressOutside,
		StyleFunc:      trigger(cx, p.Variant, p.Style),
		PopupStyleFunc: look(cx, p.PopupStyle, Tokens.popup),
		Trigger:        orText(p.Trigger, p.Label), Children: p.Children, Label: p.Label, TestID: p.TestID,
	}
}

// Tooltip is a base.Tooltip in a small inverted bubble, showing its
// children or Text. Its trigger keeps Style, with no look of its own.
type Tooltip struct {
	Open           func() bool
	DefaultOpen    bool
	OnOpenChange   func(open bool, reason base.Reason)
	OnOpen         func()
	OnClose        func()
	Placement      core.Placement
	Align          core.Align
	Offset, Margin core.Points
	Delay          time.Duration
	Disabled       bool

	Style      core.Style
	PopupStyle core.Style
	Trigger    core.Node
	Children   []core.Node
	Text       string
	TestID     string
}

// Render implements core.Node.
func (t Tooltip) Render(cx *core.Scope) core.Node {
	return base.Tooltip{
		Open: t.Open, DefaultOpen: t.DefaultOpen, OnOpenChange: t.OnOpenChange, OnOpen: t.OnOpen, OnClose: t.OnClose,
		Placement: t.Placement, Align: t.Align, Offset: t.Offset, Margin: t.Margin, Delay: t.Delay, Disabled: t.Disabled,
		Style: t.Style, PopupStyleFunc: look(cx, t.PopupStyle, Tokens.tooltip),
		Trigger: t.Trigger, Children: orLabel(t.Children, t.Text), TestID: t.TestID,
	}
}

// Dialog is a base.Dialog: a raised panel over the theme's overlay.
type Dialog struct {
	Open                   func() bool
	DefaultOpen            bool
	OnOpenChange           func(open bool, reason base.Reason)
	OnOpen                 func()
	OnClose                func()
	OnCancel               func()
	Animation              core.ModalAnimation
	NoCloseOnEscape        bool
	NoCloseOnBackdropPress bool

	BackdropStyle core.Style
	PopupStyle    core.Style
	Children      []core.Node
	Label         string
	TestID        string
}

// Render implements core.Node.
func (d Dialog) Render(cx *core.Scope) core.Node {
	return base.Dialog{
		Open: d.Open, DefaultOpen: d.DefaultOpen, OnOpenChange: d.OnOpenChange,
		OnOpen: d.OnOpen, OnClose: d.OnClose, OnCancel: d.OnCancel, Animation: d.Animation,
		NoCloseOnEscape: d.NoCloseOnEscape, NoCloseOnBackdropPress: d.NoCloseOnBackdropPress,
		BackdropStyleFunc: look(cx, d.BackdropStyle, Tokens.backdrop),
		PopupStyleFunc:    look(cx, d.PopupStyle, Tokens.dialog),
		Children:          d.Children, Label: d.Label, TestID: d.TestID,
	}
}

// Sheet is a base.Sheet: a raised panel on Side over the theme's overlay.
type Sheet struct {
	Open                   func() bool
	DefaultOpen            bool
	OnOpenChange           func(open bool, reason base.Reason)
	OnOpen                 func()
	OnClose                func()
	OnCancel               func()
	Side                   core.Placement
	NoCloseOnEscape        bool
	NoCloseOnBackdropPress bool

	BackdropStyle core.Style
	PopupStyle    core.Style
	Children      []core.Node
	Label         string
	TestID        string
}

// Render implements core.Node.
func (s Sheet) Render(cx *core.Scope) core.Node {
	return base.Sheet{
		Open: s.Open, DefaultOpen: s.DefaultOpen, OnOpenChange: s.OnOpenChange,
		OnOpen: s.OnOpen, OnClose: s.OnClose, OnCancel: s.OnCancel, Side: s.Side,
		NoCloseOnEscape: s.NoCloseOnEscape, NoCloseOnBackdropPress: s.NoCloseOnBackdropPress,
		BackdropStyleFunc: look(cx, s.BackdropStyle, Tokens.backdrop),
		PopupStyleFunc:    look(cx, s.PopupStyle, func(t Tokens) core.Style { return t.sheet(s.Side) }),
		Children:          s.Children, Label: s.Label, TestID: s.TestID,
	}
}

// trigger is the look of a popup's trigger: a button of variant.
func trigger(cx *core.Scope, v Variant, style core.Style) func() core.Style {
	return look(cx, style, func(t Tokens) core.Style { return t.button(v) })
}

// orText returns node, or label as text when it is nil.
func orText(node core.Node, label string) core.Node {
	if node == nil && label != "" {
		return core.Text{Content: label}
	}
	return node
}
