//go:build linux && !android

package linux

import (
	"unsafe"

	"github.com/oneo-me/nsgo/internal/driver"
)

// setClipboard puts text on the clipboard.
func (w *window) setClipboard(text string) {
	clip := gdkDisplayGetClipboard(gdkDisplayGetDefault())
	if clip == 0 {
		return
	}
	gdkClipboardSetText(clip, cstr(text))
}

// readClipboard asks the clipboard for its text and answers the engine
// with it.
func (w *window) readClipboard(count uint32) {
	clip := gdkDisplayGetClipboard(gdkDisplayGetDefault())
	if clip == 0 {
		w.engine.Dispatch(driver.Event{Kind: driver.EventClipboard, Count: count})
		return
	}
	w.clipboardCount = count
	gdkClipboardReadTextAsync(clip, 0, cbClipboardText, w.handle)
}

func clipboardTextCallback(clip uintptr, result uintptr, data uintptr) {
	w := windowOf(data)
	if w == nil || w.closing {
		return
	}
	p := gdkClipboardReadTextFinish(clip, result, 0)
	text := goString(p)
	if p != 0 {
		gFree(p)
	}
	w.engine.Dispatch(driver.Event{Kind: driver.EventClipboard, Count: w.clipboardCount, Text: text})
}

// showAlert queues a native alert and shows the next one when the last
// has been answered.
func (w *window) showAlert(a *driver.AlertConfig) {
	w.alerts = append(w.alerts, *a)
	if !w.alerting {
		w.nextAlert()
	}
}

// nextAlert shows the first queued alert, if any.
func (w *window) nextAlert() {
	if w.alerting || len(w.alerts) == 0 {
		return
	}
	a := w.alerts[0]
	w.alerts = w.alerts[1:]
	w.alerting, w.alertingID = true, a.ID
	dialog := gtkAlertDialogNew(0)
	if a.Title != "" {
		gtkAlertDialogSetMessage(dialog, cstr(a.Title))
	}
	if a.Message != "" {
		gtkAlertDialogSetDetail(dialog, cstr(a.Message))
	}
	labels := make([]string, len(a.Actions))
	cancel, def := -1, 0
	for i, act := range a.Actions {
		labels[i] = act.Label
		if act.Role == driver.AlertCancel && cancel < 0 {
			cancel = i
		}
		if act.Role == driver.AlertDefault {
			def = i
		}
	}
	if len(labels) > 0 {
		ptrs := stringArray(labels)
		gtkAlertDialogSetButtons(dialog, cStringArray(ptrs))
		gtkAlertDialogSetDefaultButton(dialog, int32(def))
		if cancel >= 0 {
			gtkAlertDialogSetCancelButton(dialog, int32(cancel))
		}
	}
	gtkAlertDialogChoose(dialog, w.win, 0, cbAlertChosen, w.handle)
}

func alertChosenCallback(dialog uintptr, result uintptr, data uintptr) {
	w := windowOf(data)
	if w == nil {
		gObjectUnref(dialog)
		return
	}
	index := int(gtkAlertDialogChooseFinish(dialog, result, 0))
	gObjectUnref(dialog)
	if !w.closing {
		w.engine.Dispatch(driver.Event{Kind: driver.EventAlert, Count: w.alertingID, Index: index})
	}
	w.alerting = false
	w.nextAlert()
}

// goString reads a NUL-terminated C string.
func goString(p uintptr) string {
	if p == 0 {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(cPointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(cPointer(p)), n))
}
