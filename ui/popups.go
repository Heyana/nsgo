package ui

import (
	"time"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
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
		PopupStyleFunc: look(cx, m.PopupStyle, popupStyle),
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
		Style: m.Style, PopupStyleFunc: look(cx, m.PopupStyle, popupStyle),
		Trigger: m.Trigger, Children: m.Children, TestID: m.TestID,
	}
}

// MenuItem is a base.MenuItem highlighted while hovered; its content is
// its children or Label as text.
type MenuItem struct {
	OnSelect func()
	Disabled func() bool
	// Checked, when set, leads the item with a check mark while it returns
	// true, keeping the mark's room while it does not.
	Checked         func() bool
	NoCloseOnSelect bool
	Style           core.Style
	Children        []core.Node
	Label           string
	TestID          string
}

// Render implements core.Node.
func (i MenuItem) Render(cx *core.Scope) core.Node {
	children := orLabel(i.Children, i.Label)
	if i.Checked != nil {
		mark := core.Icon{SVG: lucide.Check(), StyleFunc: func() core.Style {
			style := core.Style{Width: core.Pt(16), Height: core.Pt(16), FlexShrink: 0, Opacity: core.Alpha(0)}
			if i.Checked() {
				style.Opacity = core.Alpha(1)
			}
			return style
		}}
		children = append([]core.Node{mark}, children...)
	}
	return base.MenuItem{OnSelect: i.OnSelect, Disabled: i.Disabled, NoCloseOnSelect: i.NoCloseOnSelect,
		StyleFunc: look(cx, i.Style, menuItemStyle), Children: children,
		Label: i.Label, TestID: i.TestID}
}

// MenuSeparator is a base.MenuSeparator drawn as a line.
type MenuSeparator struct{ Style core.Style }

// Render implements core.Node.
func (s MenuSeparator) Render(cx *core.Scope) core.Node {
	return base.MenuSeparator{StyleFunc: look(cx, s.Style, menuSeparatorStyle)}
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
		StyleFunc: look(cx, s.Style, menuItemStyle), PopupStyleFunc: look(cx, s.PopupStyle, popupStyle),
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
		PopupStyleFunc: look(cx, p.PopupStyle, popupStyle),
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
		Style: t.Style, PopupStyleFunc: look(cx, t.PopupStyle, tooltipStyle),
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
	// Bare leaves out the panel's padding and gap, for content that lays
	// itself out to the panel's edges, such as Settings.
	Bare bool

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
		BackdropStyleFunc: look(cx, d.BackdropStyle, backdropStyle),
		PopupStyleFunc: look(cx, d.PopupStyle, func(t Tokens) core.Style {
			s := dialogStyle(t)
			if d.Bare {
				s.Padding, s.Gap = core.Edges{}, 0
			}
			return s
		}),
		Children: d.Children, Label: d.Label, TestID: d.TestID,
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
		BackdropStyleFunc: look(cx, s.BackdropStyle, backdropStyle),
		PopupStyleFunc:    look(cx, s.PopupStyle, func(t Tokens) core.Style { return sheetStyle(t, s.Side) }),
		Children:          s.Children, Label: s.Label, TestID: s.TestID,
	}
}

// trigger is the look of a popup's trigger: a button of variant.
func trigger(cx *core.Scope, v Variant, style core.Style) func() core.Style {
	return look(cx, style, func(t Tokens) core.Style { return buttonStyle(t, v) })
}

// orText returns node, or label as text when it is nil.
func orText(node core.Node, label string) core.Node {
	if node == nil && label != "" {
		return core.Text{Content: label}
	}
	return node
}

// The popups' looks.

func popupStyle(t Tokens) core.Style {
	return core.Style{Background: t.Surface, Color: t.SurfaceForeground, FontSize: t.FontSize,
		CornerRadius: t.RadiusLarge, BorderWidth: 1, BorderColor: t.Border, Padding: core.All(6),
		ShadowColor: t.Shadow, ShadowRadius: 16, ShadowOffset: core.Point{Y: 4}}
}

func menuItemStyle(t Tokens) core.Style {
	return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
		Padding: core.Symmetric(6, 10), CornerRadius: t.Radius - 2, MinWidth: core.Pt(160),
		Hover:    &core.Style{Background: t.Accent, Color: t.AccentForeground},
		Active:   &core.Style{Background: t.Pressed(t.Accent), Color: t.AccentForeground},
		Disabled: disabled}
}

func menuSeparatorStyle(t Tokens) core.Style {
	return core.Style{Height: core.Pt(1), Background: t.Border, Margin: core.Symmetric(4, 0)}
}

func tooltipStyle(t Tokens) core.Style {
	return core.Style{Background: t.Foreground, Color: t.Background, FontSize: t.FontSize - 2,
		CornerRadius: t.Radius - 2, Padding: core.Symmetric(4, 8)}
}

func dialogStyle(t Tokens) core.Style {
	return core.Style{Background: t.Surface, Color: t.SurfaceForeground, FontSize: t.FontSize,
		CornerRadius: t.RadiusLarge, BorderWidth: 1, BorderColor: t.Border, Padding: core.All(24), Gap: 12,
		MinWidth: core.Pt(280), MaxWidth: core.Pct(90),
		ShadowColor: t.Shadow, ShadowRadius: 24, ShadowOffset: core.Point{Y: 8}}
}

func sheetStyle(t Tokens, side core.Placement) core.Style {
	s := core.Style{Background: t.Surface, Color: t.SurfaceForeground, FontSize: t.FontSize,
		Padding: core.All(24), Gap: 12, ShadowColor: t.Shadow, ShadowRadius: 24}
	switch side {
	case core.PlaceLeft, core.PlaceRight:
		s.Width, s.MaxWidth = core.Pt(320), core.Pct(85)
	default:
		s.CornerRadius = t.RadiusLarge
	}
	return s
}

func backdropStyle(t Tokens) core.Style { return core.Style{Background: t.Overlay} }
