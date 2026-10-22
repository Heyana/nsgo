package base

import "github.com/oneo-me/nsgo/core"

// AlertDialog is a native alert: a title, a description and its actions.
type AlertDialog struct {
	Title       string
	Description string
	Actions     []core.AlertAction
}

// ShowAlertDialog shows d and calls done with the label of the action
// chosen, or "" when it was dismissed without one.
func ShowAlertDialog(cx *core.Scope, d AlertDialog, done func(label string)) {
	cx.ShowAlert(core.AlertConfig{Title: d.Title, Message: d.Description, Actions: d.Actions}, func(i int) {
		if done == nil {
			return
		}
		if i < 0 || i >= len(d.Actions) {
			done("")
			return
		}
		done(d.Actions[i].Label)
	})
}

// DismissKeyboard hides the software keyboard, taking focus away from
// the focused text field.
func DismissKeyboard(cx *core.Scope) { cx.DismissKeyboard() }
