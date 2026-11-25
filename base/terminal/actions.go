package terminal

import (
	"math"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/base/terminal/locales"
	"github.com/nsgo-dev/nsgo/core"
)

// Action is a terminal action, named after Ghostty's. Actions belonging to an app's windows or tabs stay in the app's own keymap.
type Action uint8

const (
	CopyToClipboard Action = iota
	PasteFromClipboard
	SelectAll
	ClearScreen
	IncreaseFontSize
	DecreaseFontSize
	ResetFontSize
	SetFontSize
	// Ignore consumes the binding without sending input to the program.
	Ignore
)

// Keybind binds Key, as core.Scope.OnKey names it, to Action. Amount is the point size for SetFontSize and the step for Increase/DecreaseFontSize, one point when zero. Bindings run only while the terminal input is focused and no input method is composing.
type Keybind struct {
	Key    string
	Action Action
	Amount core.Points
}

// RightClickAction is what the view does on a secondary click. A program that captures the mouse takes it before these UI actions.
type RightClickAction uint8

const (
	RightClickContextMenu RightClickAction = iota
	RightClickPaste
	RightClickCopy
	RightClickCopyOrPaste
	RightClickIgnore
)

func configuredFontSize(font core.Font) core.Points {
	if font.Size <= 0 || math.IsNaN(float64(font.Size)) {
		return 13
	}
	return font.Size
}

// FontSize reports the view's current point size, tracked for controls that display it. It returns zero while the view is unmounted.
func (r *Ref) FontSize() core.Points {
	if r == nil || r.v == nil {
		return 0
	}
	return r.v.fontSize.Get()
}

// SetFontSize sets the view's point size, clamped to Ghostty's range of 1–255 points. It does not restart the terminal or replace its input.
func (r *Ref) SetFontSize(size core.Points) {
	if r != nil && r.v != nil {
		r.v.setFontSize(size)
	}
}

// Perform runs an action on the mounted view, returning false for an unknown action or an unavailable session operation.
func (r *Ref) Perform(action Action, amount core.Points) bool {
	return r != nil && r.v != nil && r.v.perform(action, amount)
}

func (v *view) setFontSize(size core.Points) bool {
	if math.IsNaN(float64(size)) || math.IsInf(float64(size), 0) {
		return false
	}
	v.fontSize.Set(min(max(size, 1), 255))
	return true
}

func (v *view) perform(action Action, amount core.Points) bool {
	if action >= IncreaseFontSize && action <= SetFontSize && (math.IsNaN(float64(amount)) || math.IsInf(float64(amount), 0)) {
		return false
	}
	if amount == 0 {
		amount = 1
	}
	switch action {
	case IncreaseFontSize:
		return v.setFontSize(v.fontSize.Get() + min(max(amount, 0), 255))
	case DecreaseFontSize:
		return v.setFontSize(v.fontSize.Get() - min(max(amount, 0), 255))
	case ResetFontSize:
		return v.setFontSize(configuredFontSize(v.p.Font))
	case SetFontSize:
		return v.setFontSize(amount)
	case Ignore:
		return true
	}
	if v.t == nil {
		return false
	}
	switch action {
	case CopyToClipboard:
		v.copy()
	case PasteFromClipboard:
		v.paste()
	case SelectAll:
		v.selectAll()
	case ClearScreen:
		return v.t.ClearScreen()
	default:
		return false
	}
	return true
}

func (v *view) bindActions(cx *core.Scope) {
	bind := func(key string, action Action, amount core.Points) {
		cx.OnKeyWith(key, core.KeyOptions{When: core.KeyAlways, Repeat: true}, func() { v.perform(action, amount) })
	}
	bind("mod+=", IncreaseFontSize, 1)
	bind("mod+shift+=", IncreaseFontSize, 1)
	bind("mod+-", DecreaseFontSize, 1)
	bind("mod+0", ResetFontSize, 0)
	for _, binding := range v.p.Keybinds {
		bind(binding.Key, binding.Action, binding.Amount)
	}
}

// withContextMenu adds a hit wrapper inside the existing root, preserving the root's style, reference and geometry. Its trigger is unstyled.
func (v *view) withContextMenu(children []core.Node) []core.Node {
	p := &v.p
	if p.RightClickAction == RightClickIgnore {
		return children
	}
	fill := core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}
	if p.RightClickAction != RightClickContextMenu {
		return []core.Node{core.View{Style: fill, OnContextMenu: func(core.ContextMenuEvent) {
			switch p.RightClickAction {
			case RightClickPaste:
				v.paste()
			case RightClickCopy:
				v.copy()
			case RightClickCopyOrPaste:
				if v.selected.Get() {
					v.copy()
				} else {
					v.paste()
				}
			}
		}, Children: children}}
	}
	m := func() locales.Messages { return locales.For(v.cx) }
	item := func(id string, label func() string, action Action, disabled func() bool) core.Node {
		var itemChildren []core.Node
		if p.MenuItemIcon != nil {
			if icon := p.MenuItemIcon(v.cx, action); icon != nil {
				itemChildren = append(itemChildren, icon)
			}
		}
		itemChildren = append(itemChildren, core.Text{ContentFunc: label, MaxLines: 1, Style: core.Style{FlexGrow: 1, FlexShrink: 1}})
		return base.MenuItem{TestID: v.testID("menu-" + id), LabelFunc: label, Disabled: disabled,
			Class: p.MenuItemClass, Style: p.MenuItemStyle, StyleFunc: p.MenuItemStyleFunc,
			OnSelect: func() { v.perform(action, 0) },
			Children: itemChildren}
	}
	return []core.Node{base.ContextMenu{Style: fill,
		Offset: 2, Margin: 8,
		Trigger:    core.View{Style: fill, Children: children},
		PopupClass: p.MenuClass, PopupStyle: p.MenuStyle, PopupStyleFunc: p.MenuStyleFunc,
		Children: []core.Node{
			item("copy", func() string { return m().Copy() }, CopyToClipboard, func() bool { return !v.selected.Get() }),
			item("paste", func() string { return m().Paste() }, PasteFromClipboard, func() bool { return v.t == nil }),
			item("select-all", func() string { return m().SelectAll() }, SelectAll, func() bool { return v.t == nil }),
			item("clear", func() string { return m().Clear() }, ClearScreen, func() bool {
				v.rev.Get()
				if v.t == nil {
					return true
				}
				v.t.mu.Lock()
				defer v.t.mu.Unlock()
				return v.t.term == nil || v.t.term.AltScreen()
			}),
		}}}
}
