package ui

import (
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// AlertAction is an action of an AlertDialog, a Button of its Variant,
// or filled with Color.
type AlertAction struct {
	Text     string
	TextFunc func() string
	Variant  ButtonVariant
	Color    func() core.Color
	// Role is core.AlertCancel for the action Escape and back run before the alert closes with ReasonCancel, the first such action; core.AlertDestructive marks one that destroys, for TSX's role names. Variant and Color still choose its look.
	Role core.AlertRole
	// Default is the action Return chooses, the first such action: it takes the keyboard focus as the alert opens, and Return runs it while the focus is on no other action or field, then closes the alert with ReasonSelect.
	Default bool
	OnPress func()
	TestID  string
}

// AlertDialog asks the user to confirm or acknowledge something: a Dialog
// whose actions close it, kept open by presses outside it.
type AlertDialog struct {
	Open         func() bool
	OnOpenChange func(open bool, reason base.Reason)
	// OnClose follows it being gone, once its motion ended.
	OnClose         func()
	Title           string
	TitleFunc       func() string
	Description     string
	DescriptionFunc func() string
	Actions         []AlertAction
	TestID          string
	// Style merges over the dialog's surface; StyleFunc merges over Style and follows the signals it reads.
	Style     core.Style
	StyleFunc func() core.Style
}

// Render implements core.Node.
func (a AlertDialog) Render(cx *core.Scope) core.Node {
	// answered closes it once an action ran, whatever the app does with OnOpenChange, until the app sets Open false and true again, as base.Dialog stays closed after Escape; a key pressed on a focused action while it goes runs nothing more.
	answered := core.NewSignal(cx, false)
	open := a.Open
	if open != nil {
		cx.Effect(func() {
			if !a.Open() {
				answered.Set(false)
			}
		})
		open = func() bool { return a.Open() && !answered.Get() }
	}
	// answer reports whether an action may run, as the alert is open and unanswered, and marks it answered.
	answer := func() bool {
		if open == nil || !open() {
			return false
		}
		answered.Set(true)
		return true
	}
	run := func(f func()) {
		if f != nil {
			f()
		}
	}
	var footer []core.Node
	var cancel, confirm func()
	cancelSet, defaultSet := false, false
	for _, action := range a.Actions {
		if action.Role == core.AlertCancel && !cancelSet {
			cancel, cancelSet = action.OnPress, true
		}
		focus := action.Default && !defaultSet
		if focus {
			defaultSet = true
			// base.Dialog's OnConfirm asks to close with ReasonSelect after it.
			confirm = func() {
				answered.Set(true)
				run(action.OnPress)
			}
		}
		footer = append(footer, Button{Variant: action.Variant, Color: action.Color, Text: action.Text, TextFunc: action.TextFunc, TestID: action.TestID, AutoFocus: focus, OnPress: func() {
			if !answer() {
				return
			}
			run(action.OnPress)
			if a.OnOpenChange != nil {
				a.OnOpenChange(false, base.ReasonSelect)
			}
		}})
	}
	change := func(open bool, reason base.Reason) {
		if !open && reason == base.ReasonCancel {
			answered.Set(true)
			run(cancel)
		}
		if a.OnOpenChange != nil {
			a.OnOpenChange(open, reason)
		}
	}
	return Dialog{Open: open, OnOpenChange: change, OnClose: a.OnClose, OnConfirm: confirm, Title: a.Title, TitleFunc: a.TitleFunc, Description: a.Description, DescriptionFunc: a.DescriptionFunc, Footer: footer,
		Role: core.RoleAlertDialog, NoCloseOnBackdropPress: true, TestID: a.TestID, Style: a.Style, StyleFunc: a.StyleFunc}
}
