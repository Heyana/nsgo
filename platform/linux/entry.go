//go:build linux

package linux

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/oneo-me/nsgo/core"
)

// entryStyle is the CSS a GtkEntry takes: no look of GTK's own, the
// view draws its box, and the input's text style.
func entryStyle(in *core.InputConfig) string {
	f := in.Style.Font
	color := in.Style.Color
	var b strings.Builder
	b.WriteString("* { padding: 0; margin: 0; min-width: 0; min-height: 0; border: none; border-radius: 0; box-shadow: none; background: none; background-color: transparent; }\n")
	r, g, bl, a := color.Components()
	fmt.Fprintf(&b, "* { color: rgba(%d,%d,%d,%.3f); caret-color: rgba(%d,%d,%d,%.3f); }\n",
		int(r*255), int(g*255), int(bl*255), a, int(r*255), int(g*255), int(bl*255), a)
	if f.Family != "" {
		fmt.Fprintf(&b, "* { font-family: %q; }\n", f.Family)
	}
	if f.Size > 0 {
		fmt.Fprintf(&b, "* { font-size: %.3fpt; }\n", float64(f.Size))
	}
	if f.Weight > 0 {
		fmt.Fprintf(&b, "* { font-weight: %d; }\n", int(f.Weight))
	}
	if f.Italic {
		b.WriteString("* { font-style: italic; }\n")
	}
	return b.String()
}

// configure applies an input's look and behavior to its GtkEntry,
// making it on the first configuration.
func (w *window) configure(v *view, in *core.InputConfig) {
	v.input = *in
	if v.kind != core.KindInput {
		return
	}
	if v.entry == 0 {
		v.entry = gtkEntryNew()
		gtkWidgetSetHexpand(v.entry, 1)
		gtkWidgetSetVexpand(v.entry, 1)
		gtkFixedPut(w.fixed, v.entry, 0, 0)
		w.entries[v.entry] = v
		gSignalConnect(v.entry, "changed", cbEntryChanged, w.handle)
		gSignalConnect(v.entry, "activate", cbEntryActivate, w.handle)
		gSignalConnect(v.entry, "notify::has-focus", cbEntryFocus, w.handle)
		v.provider = gtkCssProviderNew()
		gtkStyleContextAddProvider(gtkWidgetGetStyleContext(v.entry), v.provider, 800)
	}
	gtkEntrySetPlaceholderText(v.entry, cstr(in.Placeholder))
	gtkEntrySetVisibility(v.entry, boolInt(!in.Masked))
	gtkEditableSetEditable(v.entry, boolInt(!in.Readonly && !in.Disabled))
	gtkWidgetSetSensitive(v.entry, boolInt(!in.Disabled))
	switch in.Style.Align {
	case core.TextAlignCenter:
		gtkEntrySetAlignment(v.entry, 0.5)
	case core.TextAlignEnd:
		gtkEntrySetAlignment(v.entry, 1)
	default:
		gtkEntrySetAlignment(v.entry, 0)
	}
	w.styleEntry(v)
}

// styleEntry reloads the entry's CSS from its input configuration.
func (w *window) styleEntry(v *view) {
	if v.provider == 0 {
		return
	}
	gtkCssProviderLoadFromString(v.provider, cstr(entryStyle(&v.input)))
}

// setValue sets an input's text unless the user edited it after edit
// number count.
func (w *window) setValue(v *view, value string, count uint32) {
	if v.entry == 0 || v.edits > count || goString(gtkEditableGetText(v.entry)) == value {
		return
	}
	v.setting = true
	gtkEditableSetText(v.entry, cstr(value))
	v.setting = false
}

// placeEntries moves each entry to its input's content and hides the ones
// of inputs that are hidden, detached or clipped away.
func (w *window) placeEntries() {
	for entry, v := range w.entries {
		if v.deleted || v.style.Hidden || v.style.Opacity <= 0 {
			if v.entryShown {
				gtkWidgetSetVisible(entry, 0)
				v.entryShown = false
			}
			continue
		}
		at := w.frameAt(v)
		b := core.Bounds{
			Origin: core.Point{X: at.X + v.input.Content.Origin.X, Y: at.Y + v.input.Content.Origin.Y},
			Size:   v.input.Content.Size,
		}
		clip := w.surfaceClip(v)
		visible := intersectBounds(b, clip)
		if visible.Size.Width <= 0 || visible.Size.Height <= 0 {
			if v.entryShown {
				gtkWidgetSetVisible(entry, 0)
				v.entryShown = false
			}
			continue
		}
		// A partly clipped entry is hidden rather than drawn outside
		// its scroll view; GTK has no per-widget clip rectangle.
		if visible != b {
			if v.entryShown {
				gtkWidgetSetVisible(entry, 0)
				v.entryShown = false
			}
			continue
		}
		gtkFixedMove(w.fixed, entry, float64(b.Origin.X), float64(b.Origin.Y))
		gtkWidgetSetSizeRequest(entry, int32(b.Size.Width), int32(b.Size.Height))
		if !v.entryShown {
			gtkWidgetSetVisible(entry, 1)
			v.entryShown = true
		}
	}
}

// surfaceClip returns the part of the surface v's clipping ancestors
// leave for v.
func (w *window) surfaceClip(v *view) core.Bounds {
	clip := core.Bounds{Size: core.Size{Width: w.lastWidth, Height: w.lastHeight}}
	for p := v.parent; p != nil; p = p.parent {
		if p.style.Clip || p.kind == core.KindScroll {
			at := w.frameAt(p)
			r := core.Bounds{Origin: at, Size: p.frame.Size}
			clip = intersectBounds(clip, r)
		}
	}
	return clip
}

// intersectBounds returns the overlap of a and b.
func intersectBounds(a, b core.Bounds) core.Bounds {
	x0, y0 := max(a.Origin.X, b.Origin.X), max(a.Origin.Y, b.Origin.Y)
	x1 := min(a.Origin.X+a.Size.Width, b.Origin.X+b.Size.Width)
	y1 := min(a.Origin.Y+a.Size.Height, b.Origin.Y+b.Size.Height)
	if x1 <= x0 || y1 <= y0 {
		return core.Bounds{}
	}
	return core.Bounds{Origin: core.Point{X: x0, Y: y0}, Size: core.Size{Width: x1 - x0, Height: y1 - y0}}
}

// focusedEntry returns the input with keyboard focus, or nil.
func (w *window) focusedEntry() *view { return w.focus }

// focusEntry moves keyboard focus to v's entry, or away from the focused
// entry when v is nil.
func (w *window) focusEntry(v *view) {
	if v != nil && v.entry != 0 {
		gtkWidgetGrabFocus(v.entry)
		return
	}
	gtkWindowSetFocus(w.win, 0)
}

// entryChangedCallback reports a user's edit of a field.
func entryChangedCallback(entry uintptr, data uintptr) {
	w := windowOf(data)
	if w == nil || w.closing {
		return
	}
	v := w.entries[entry]
	if v == nil || v.deleted || v.setting {
		return
	}
	v.edits++
	w.engine.Dispatch(core.Event{Kind: core.EventChangeText, ID: v.id,
		Text: goString(gtkEditableGetText(entry)), Count: v.edits})
}

// entryActivateCallback reports Return in a field.
func entryActivateCallback(entry uintptr, data uintptr) {
	w := windowOf(data)
	if w == nil || w.closing {
		return
	}
	v := w.entries[entry]
	if v == nil || v.deleted {
		return
	}
	w.engine.Dispatch(core.Event{Kind: core.EventSubmitEditing, ID: v.id,
		Text: goString(gtkEditableGetText(entry))})
}

// entryFocusCallback reports a field taking or losing keyboard focus.
func entryFocusCallback(entry uintptr, pspec uintptr, data uintptr) {
	w := windowOf(data)
	if w == nil || w.closing {
		return
	}
	v := w.entries[entry]
	if v == nil || v.deleted {
		return
	}
	if gtkWidgetHasFocus(entry) != 0 {
		w.focus = v
		if v.events.Has(core.EventFocus) {
			w.engine.Dispatch(core.Event{Kind: core.EventFocus, ID: v.id})
		}
		return
	}
	if w.focus == v {
		w.focus = nil
		if v.events.Has(core.EventBlur) {
			w.engine.Dispatch(core.Event{Kind: core.EventBlur, ID: v.id})
		}
	}
}

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
		w.engine.Dispatch(core.Event{Kind: core.EventClipboard, Count: count})
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
	w.engine.Dispatch(core.Event{Kind: core.EventClipboard, Count: w.clipboardCount, Text: text})
}

// showAlert queues a native alert and shows the next one when the last
// has been answered.
func (w *window) showAlert(a *core.AlertConfig) {
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
		if act.Role == core.AlertCancel && cancel < 0 {
			cancel = i
		}
		if act.Role == core.AlertDefault {
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
		w.engine.Dispatch(core.Event{Kind: core.EventAlert, Count: w.alertingID, Index: index})
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
