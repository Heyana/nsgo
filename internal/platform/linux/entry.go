//go:build linux && !android

package linux

import (
	"runtime"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// setClipboard puts text on the clipboard.
func (w *window) setClipboard(text string) {
	clip := gdkDisplayGetClipboard(gdkDisplayGetDefault())
	if clip == 0 {
		return
	}
	gdkClipboardSetText(clip, cstr(text))
}

// setClipboardItem writes a multi-representation item to the GDK clipboard as
// a union of MIME content providers.
func (w *window) setClipboardItem(item driver.ClipboardItem) bool {
	clip := gdkDisplayGetClipboard(gdkDisplayGetDefault())
	if clip == 0 || len(item.Representations) == 0 {
		return false
	}
	seen := make(map[string]bool, len(item.Representations)+1)
	var providers []uintptr
	add := func(mime string, data []byte) {
		if mime == "" || seen[mime] {
			return
		}
		seen[mime] = true
		var ptr unsafe.Pointer
		if len(data) > 0 {
			ptr = unsafe.Pointer(&data[0])
		}
		b := gBytesNew(ptr, uintptr(len(data)))
		if b == 0 {
			return
		}
		p := gdkContentProviderNewForBytes(cstr(mime), b)
		gBytesUnref(b)
		if p != 0 {
			providers = append(providers, p)
		}
	}
	for _, rep := range item.Representations {
		if rep.Type == "text/plain" {
			add("text/plain;charset=utf-8", rep.Data)
		}
		add(rep.Type, rep.Data)
	}
	if len(providers) == 0 {
		return false
	}
	var content uintptr
	if len(providers) == 1 {
		content = providers[0]
	} else {
		// gdk_content_provider_new_union takes ownership of each provider element.
		content = gdkContentProviderNewUnion(&providers[0], uintptr(len(providers)))
		runtime.KeepAlive(providers)
	}
	if content == 0 {
		return false
	}
	ok := gdkClipboardSetContent(clip, content) != 0
	gObjectUnref(content)
	return ok
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
	w.alerting, w.alertingID, w.alertShown = true, a.ID, a
	// The constructor formats its message: a NULL format crashes GTK.
	dialog := gtkAlertDialogNew(cstr(""))
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
		strs := stringArray(labels)
		gtkAlertDialogSetButtons(dialog, strs.array())
		runtime.KeepAlive(strs)
		gtkAlertDialogSetDefaultButton(dialog, int32(def))
		if cancel >= 0 {
			gtkAlertDialogSetCancelButton(dialog, int32(cancel))
		}
	}
	gtkAlertDialogChoose(dialog, w.toplevel(), 0, cbAlertChosen, w.handle)
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
