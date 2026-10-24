package core

import "github.com/oneo-me/nsgo/internal/driver"

// Services the platform performs for the app, asked for with command
// items and answered with events.

// ShowAlert shows a native alert and calls done with the index of the
// button chosen, or -1 when it was dismissed without one. done runs even
// when the owner that showed the alert is gone, such as a closed menu.
func (cx *Scope) ShowAlert(a AlertConfig, done func(index int)) {
	t := cx.rt.tree
	if t == nil {
		panic("core: ShowAlert needs a mounted tree")
	}
	t.alertID++
	a.ID = t.alertID
	if t.alerts == nil {
		t.alerts = map[uint32]alertWait{}
	}
	t.alerts[a.ID] = alertWait{cx.current(), done}
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemAlert, Alert: &a})
	t.changed = true
}

type alertWait struct {
	owner *Scope
	done  func(int)
}

func (t *Tree) answerAlert(id uint32, index int) {
	w, ok := t.alerts[id]
	if !ok {
		return
	}
	delete(t.alerts, id)
	owner := w.owner
	if owner.disposed {
		owner = t.root
	}
	if w.done != nil {
		t.within(owner, func() { w.done(index) })
	}
}

// DismissKeyboard takes focus away from the focused text field, hiding
// the software keyboard.
func (cx *Scope) DismissKeyboard() {
	if t := cx.rt.tree; t != nil {
		t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemDismissKeyboard})
		t.changed = true
	}
}

// SetClipboard puts text on the system clipboard.
func (cx *Scope) SetClipboard(text string) {
	if t := cx.rt.tree; t != nil {
		t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemSetClipboard, Value: text})
		t.changed = true
	}
}

// ReadClipboard calls done with the clipboard's text, "" when it holds
// none.
func (cx *Scope) ReadClipboard(done func(text string)) {
	t := cx.rt.tree
	if t == nil {
		return
	}
	t.alertID++
	if t.reads == nil {
		t.reads = map[uint32]readWait{}
	}
	t.reads[t.alertID] = readWait{cx.current(), done}
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemReadClipboard, Count: t.alertID})
	t.changed = true
}

type readWait struct {
	owner *Scope
	done  func(string)
}

func (t *Tree) answerRead(id uint32, text string) {
	w, ok := t.reads[id]
	if !ok {
		return
	}
	delete(t.reads, id)
	owner := w.owner
	if owner.disposed {
		owner = t.root
	}
	t.within(owner, func() { w.done(text) })
}

// Announce has the screen reader say text.
func (cx *Scope) Announce(text string) {
	if t := cx.rt.tree; t != nil {
		t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemAnnounce, Value: text})
		t.changed = true
	}
}
