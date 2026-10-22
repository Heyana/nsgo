package base

import "github.com/oneo-me/nsgo/core"

// menuKey gives a menu's items the way to close it.
var menuKey = core.NewKey[func(Reason)]("menu", nil)

// Menu opens a list of MenuItems beside its trigger when the trigger is
// pressed. Choosing an item closes it, as do a press outside and Escape.
type Menu struct {
	Open         func() bool
	DefaultOpen  bool
	OnOpenChange func(open bool, reason Reason)
	OnOpen       func()
	OnClose      func()
	OnCancel     func()

	Placement      core.Placement
	Align          core.Align
	Offset, Margin core.Points

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
func (m Menu) Render(cx *core.Scope) core.Node {
	st := newPopup(cx, m.Open, m.DefaultOpen, m.OnOpenChange, m.OnOpen, m.OnClose)
	anchor := new(core.Ref)
	return core.Group{
		core.View{Ref: anchor, Style: m.Style, StyleFunc: m.StyleFunc, Label: m.Label, TestID: m.TestID, Role: core.RoleButton,
			OnPress:  func() { st.set(!st.open(), ReasonTriggerPress) },
			Children: []core.Node{m.Trigger}},
		menuPopup(st, anchor, m.Placement, m.Align, m.Offset, m.Margin, m.PopupStyle, m.PopupStyleFunc, m.Children, m.OnCancel),
	}
}

func menuPopup(st *popup, anchor *core.Ref, place core.Placement, align core.Align, offset, margin core.Points,
	style core.Style, styleFunc func() core.Style, items []core.Node, onCancel func()) core.Node {
	return core.Show{When: st.open, Then: func(cx *core.Scope) core.Node {
		menuKey.Provide(cx, func(r Reason) { st.set(false, r) })
		return core.Overlay{
			Anchor: anchor, Placement: place, Align: align, Offset: offset, Margin: margin,
			Style: style, StyleFunc: styleFunc, Children: items, Role: core.RoleMenu,
			OnPressOutside: func(core.Point) { st.set(false, ReasonPressOutside) },
			OnCancel: func() {
				if onCancel != nil {
					onCancel()
				}
				st.set(false, ReasonCancel)
			},
		}
	}}
}

// ContextMenu opens a menu of MenuItems on its content on a secondary
// click (desktops) or a long press (phones).
type ContextMenu struct {
	Open         func() bool
	OnOpenChange func(open bool, reason Reason)
	OnOpen       func()
	OnClose      func()
	OnCancel     func()
	Disabled     bool

	Offset, Margin core.Points
	Style          core.Style // the content's
	StyleFunc      func() core.Style
	PopupStyle     core.Style
	PopupStyleFunc func() core.Style
	Trigger        core.Node
	Children       []core.Node
	TestID         string
}

// Render implements core.Node.
func (m ContextMenu) Render(cx *core.Scope) core.Node {
	st := newPopup(cx, m.Open, false, m.OnOpenChange, m.OnOpen, m.OnClose)
	anchor := new(core.Ref)
	return core.Group{
		core.View{Ref: anchor, Style: m.Style, StyleFunc: m.StyleFunc, TestID: m.TestID,
			OnLongPress: func() {
				if !m.Disabled {
					st.set(true, ReasonLongPress)
				}
			},
			OnContextMenu: func(core.Point) {
				if !m.Disabled {
					st.set(true, ReasonContextMenu)
				}
			},
			Children: []core.Node{m.Trigger}},
		menuPopup(st, anchor, core.PlaceDefault, core.AlignStart, m.Offset, m.Margin, m.PopupStyle, m.PopupStyleFunc, m.Children,
			m.OnCancel),
	}
}

// MenuItem is one choice of a menu: pressing it reports OnSelect and
// closes the menu. Its look is the app's, with its state styles.
type MenuItem struct {
	OnSelect        func()
	Disabled        func() bool
	NoCloseOnSelect bool
	Style           core.Style
	StyleFunc       func() core.Style
	Children        []core.Node
	Label           string
	TestID          string
}

// Render implements core.Node.
func (i MenuItem) Render(cx *core.Scope) core.Node {
	close := menuKey.Use(cx)
	return core.View{Style: i.Style, StyleFunc: i.StyleFunc, Role: core.RoleMenuItem, Label: i.Label, TestID: i.TestID,
		Disabled: i.Disabled, Children: i.Children,
		OnPress: func() {
			if i.OnSelect != nil {
				i.OnSelect()
			}
			if close != nil && !i.NoCloseOnSelect {
				close(ReasonSelect)
			}
		}}
}

// MenuSeparator separates groups of items.
type MenuSeparator struct {
	Style     core.Style
	StyleFunc func() core.Style
}

// Render implements core.Node.
func (s MenuSeparator) Render(*core.Scope) core.Node {
	return core.View{Style: s.Style, StyleFunc: s.StyleFunc}
}

// Submenu is an item that opens more items beside it; choosing one of
// them closes the whole menu.
type Submenu struct {
	Disabled       func() bool
	Style          core.Style // the item's
	StyleFunc      func() core.Style
	PopupStyle     core.Style
	PopupStyleFunc func() core.Style
	Trigger        core.Node
	Children       []core.Node
	Label          string
	TestID         string
}

// Render implements core.Node.
func (s Submenu) Render(cx *core.Scope) core.Node {
	closeAll := menuKey.Use(cx)
	open := core.NewSignal(cx, false)
	anchor := new(core.Ref)
	return core.Group{
		core.View{Ref: anchor, Style: s.Style, StyleFunc: s.StyleFunc, Role: core.RoleMenuItem, Label: s.Label, TestID: s.TestID,
			Disabled: s.Disabled,
			OnPress:  func() { open.Update(func(o bool) bool { return !o }) },
			OnHoverIn: func() {
				if cx.Env().Desktop() {
					open.Set(true)
				}
			},
			Children: []core.Node{s.Trigger}},
		core.Show{When: open.Get, Then: func(cx *core.Scope) core.Node {
			menuKey.Provide(cx, func(r Reason) {
				open.Set(false)
				if closeAll != nil {
					closeAll(r)
				}
			})
			return core.Overlay{Anchor: anchor, Placement: core.PlaceRight, Align: core.AlignStart,
				Style: s.PopupStyle, StyleFunc: s.PopupStyleFunc, Children: s.Children, Role: core.RoleMenu,
				OnCancel: func() { open.Set(false) }}
		}},
	}
}
