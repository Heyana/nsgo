package ui

import (
	"cmp"
	"strconv"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/base/locales"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

// Select picks one of Options from a field that opens them as a list:
// over it on desktops, the chosen row on the field, where the keys move
// through them and letters jump, and as a sheet on touch screens. The field
// shows the chosen option's label, or Placeholder while Value matches none,
// and the list marks it with its row's selected face. The rows are made as
// they scroll in, so a list may be long. Options' Icon and Content go unused.
type Select[T comparable] struct {
	// Ref is the field's; LabelledBy and Invalid are a core.View's.
	Ref        *core.Ref
	LabelledBy []*core.Ref
	Invalid    func() bool
	Options    []Option[T]
	// OptionsFunc, when set, returns the options in place of Options and updates the open list in place when the values it reads change, keeping the field's value and focus. A Value no longer offered shows Placeholder; OnChange is not called for it.
	OptionsFunc func() []Option[T]
	Value       func() T
	OnChange    func(value T)
	Placeholder string
	// Searchable shows a field above the rows that filters them by label,
	// with SearchPlaceholder in it and Empty while none matches.
	Searchable        bool
	SearchPlaceholder string
	Empty             string
	// OnClear, when set, shows a button in the field while an option is
	// chosen, which asks to choose none.
	OnClear  func()
	Disabled func() bool
	// Presentation is base's: a popup at the field by default, or a
	// sheet, which ui chooses on no device of its own.
	Presentation base.Presentation
	// Label names the field for assistive technology. An empty label uses Placeholder.
	Label     string
	LabelFunc func() string
	// TestID names the field; each option adds "-" and its index.
	TestID string
	// Style merges over the field's, 200 pt wide: its height, padding and
	// type reach the field, as an Input's do, so a smaller one lines up
	// with a SizeSmall Button.
	Style core.Style
}

// optionItems are options as a list's, valued by their index.
func optionItems[T comparable](options []Option[T]) func() []base.SelectItem {
	return optionItemsFunc(func() []Option[T] { return options })
}

func optionItemsFunc[T comparable](options func() []Option[T]) func() []base.SelectItem {
	return func() []base.SelectItem {
		options := options()
		items := make([]base.SelectItem, len(options))
		for i, o := range options {
			items[i] = base.SelectItem{Value: strconv.Itoa(i), Title: o.label(), Group: o.Group, Disabled: o.Disabled != nil && o.Disabled()}
		}
		return items
	}
}

// optionRow draws an option of a list.
func optionRow[T comparable](cx *core.Scope, o Option[T], id string) core.Node {
	colors := ColorsOf(cx)
	var children []core.Node
	if o.Leading != nil {
		children = append(children, o.Leading)
	}
	children = append(children, core.Text{ContentFunc: o.label, MaxLines: 1, Style: TypeBody.Merge(core.Style{FlexGrow: 1, FlexShrink: 1}), StyleFunc: Colored(colors.Text)})
	if o.Trailing != nil {
		children = append(children, o.Trailing)
	}
	return core.View{TestID: id, Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, FlexGrow: 1}, Children: children}
}

// Render implements core.Node.
func (s Select[T]) Render(cx *core.Scope) core.Node {
	options := func() []Option[T] {
		if s.OptionsFunc != nil {
			return s.OptionsFunc()
		}
		return s.Options
	}
	open := core.NewSignal(cx, false)
	chosen := core.NewMemo(cx, func() int {
		options := options()
		if s.Value != nil {
			v := s.Value()
			for i, o := range options {
				if o.Value == v {
					return i
				}
			}
		}
		return -1
	})
	leading := core.Dynamic(func(*core.Scope) core.Node {
		options := options()
		if i := chosen.Get(); i >= 0 && i < len(options) && options[i].Leading != nil {
			return options[i].Leading
		}
		return core.Group(nil)
	})
	title := func() string {
		options := options()
		if i := chosen.Get(); i >= 0 && i < len(options) {
			return options[i].label()
		}
		return ""
	}
	root, field := pickerStyle(s.Style)
	face := pickerField(cx, field, leading, pickerText(cx, field, title, s.Placeholder), lucide.ChevronDown, open.Get)
	row, look := listRowStyle(cx, s.Presentation)
	group, groupLook := listGroupStyle(cx)
	popup, backdrop := pickerSurface(cx, s.Presentation)
	label, labelFunc := s.Label, s.LabelFunc
	if labelFunc == nil {
		label = cmp.Or(label, s.Placeholder)
	} else {
		labelFunc = func() string { return cmp.Or(s.LabelFunc(), s.Placeholder) }
	}
	b := base.Select{Items: optionItemsFunc(options), Disabled: s.Disabled, Presentation: s.Presentation, Open: open.Get, OnOpenChange: func(v bool, _ base.Reason) { open.Set(v) },
		Value: func() *string {
			if i := chosen.Get(); i >= 0 {
				v := strconv.Itoa(i)
				return &v
			}
			return nil
		},
		OnChange: func(v *string) {
			if v == nil {
				if s.OnClear != nil {
					s.OnClear()
				}
				return
			}
			if i, err := strconv.Atoi(*v); err == nil && s.OnChange != nil {
				options := options()
				if i < 0 || i >= len(options) {
					return
				}
				s.OnChange(options[i].Value)
			}
		},
		Offset: 4, Margin: 8, RootStyle: root,
		Style:     core.Style{Group: "button", CornerRadius: RadiusControl},
		StyleFunc: func() core.Style { return core.Style{FocusVisible: FocusRing(cx, 2)} },
		Trigger:   face, PopupStyleFunc: popup, BackdropStyle: backdrop, ListStyle: core.Style{MaxHeight: core.Pt(320)},
		ItemStyle: row, ItemStyleFunc: look, GroupStyle: group, GroupStyleFunc: groupLook, Ref: s.Ref, LabelledBy: s.LabelledBy, Invalid: s.Invalid, Label: label, LabelFunc: labelFunc, TestID: s.TestID,
		RenderItem: func(e base.SelectEntry) core.Node {
			id := ""
			if s.TestID != "" {
				id = s.TestID + "-" + strconv.Itoa(e.Index)
			}
			options := options()
			if e.Index < 0 || e.Index >= len(options) {
				return nil
			}
			return optionRow(cx, options[e.Index], id)
		}}
	if s.Searchable {
		b.Searchable = func() bool { return true }
		b.SearchPlaceholder, b.SearchLabel, b.SearchField = s.SearchPlaceholder, s.SearchPlaceholder, searchField
		if b.SearchLabel == "" {
			b.SearchLabel = locales.ListSearch(cx)
		}
		text := s.Empty
		if text == "" {
			text = locales.ListNothingFound(cx)
		}
		b.Empty = emptyList(cx, text)
	}
	if s.OnClear != nil {
		b.Cleanable, b.ClearButton, b.ClearButtonStyle, b.ClearButtonLabel = true, clearMark(cx), clearStyle, locales.ListClear(cx)
	}
	return b
}
