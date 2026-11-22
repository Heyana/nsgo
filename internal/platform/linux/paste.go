//go:build linux && !android

package linux

import (
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// The caret and pastes of text inputs. The caret is GTK's own: the entry's text widget computes its extents, a text view its cursor locations. A paste is heard from the editor's paste-clipboard signal; GDK reads clipboards asynchronously, so the paste is reported after the text it inserts, with the clipboard's local files, else its image, its HTML (text/html), and its text.

type gdkRectangle struct{ x, y, w, h int32 }

// pasting is the paste being read.
type pasting struct {
	v       *view
	clip    uintptr
	ev      *driver.PasteEvent
	hasHTML bool
	html    []byte
}

var (
	gtkTextComputeCursorExtents      func(uintptr, uintptr, *grapheneRect, *grapheneRect)
	gtkEditableGetPosition           func(uintptr) int32
	gtkTextViewGetCursorLocations    func(uintptr, uintptr, *gdkRectangle, *gdkRectangle)
	gtkTextViewBufferToWindowCoords  func(uintptr, int32, int32, int32, *int32, *int32)
	gtkWidgetGetClipboard            func(uintptr) uintptr
	gdkClipboardGetFormats           func(uintptr) uintptr
	gdkContentFormatsContainGtype    func(uintptr, uintptr) bool
	gdkContentFormatsContainMimeType func(uintptr, *byte) int32
	gdkTextureGetType                func() uintptr
	gdkClipboardReadValueAsync       func(uintptr, uintptr, int32, uintptr, uintptr, uintptr)
	gdkClipboardReadValueFinish      func(uintptr, uintptr, uintptr) uintptr
	gdkClipboardReadTextureAsync     func(uintptr, uintptr, uintptr, uintptr)
	gdkClipboardReadTextureFinish    func(uintptr, uintptr, uintptr) uintptr
	gdkClipboardReadAsync            func(uintptr, uintptr, int32, uintptr, uintptr, uintptr)
	gdkClipboardReadFinish           func(uintptr, uintptr, uintptr, uintptr) uintptr
	gdkTextureSaveToPngBytes         func(uintptr) uintptr
	gInputStreamReadBytesAsync       func(uintptr, uintptr, int32, uintptr, uintptr, uintptr)
	gInputStreamReadBytesFinish      func(uintptr, uintptr, uintptr) uintptr
	gInputStreamCloseAsync           func(uintptr, int32, uintptr, uintptr, uintptr)
	gBytesGetData                    func(uintptr, *uintptr) uintptr
	gBytesUnref                      func(uintptr)
	cbPasteClipboard                 uintptr
)

func bindPaste() {
	gtk := func(fn any, name string) { purego.RegisterLibFunc(fn, gtkLib, name) }
	gtk(&gtkTextComputeCursorExtents, "gtk_text_compute_cursor_extents")
	gtk(&gtkEditableGetPosition, "gtk_editable_get_position")
	gtk(&gtkTextViewGetCursorLocations, "gtk_text_view_get_cursor_locations")
	gtk(&gtkTextViewBufferToWindowCoords, "gtk_text_view_buffer_to_window_coords")
	gtk(&gtkWidgetGetClipboard, "gtk_widget_get_clipboard")
	gtk(&gdkClipboardGetFormats, "gdk_clipboard_get_formats")
	gtk(&gdkContentFormatsContainGtype, "gdk_content_formats_contain_gtype")
	gtk(&gdkContentFormatsContainMimeType, "gdk_content_formats_contain_mime_type")
	gtk(&gdkTextureGetType, "gdk_texture_get_type")
	gtk(&gdkClipboardReadValueAsync, "gdk_clipboard_read_value_async")
	gtk(&gdkClipboardReadValueFinish, "gdk_clipboard_read_value_finish")
	gtk(&gdkClipboardReadTextureAsync, "gdk_clipboard_read_texture_async")
	gtk(&gdkClipboardReadTextureFinish, "gdk_clipboard_read_texture_finish")
	gtk(&gdkClipboardReadAsync, "gdk_clipboard_read_async")
	gtk(&gdkClipboardReadFinish, "gdk_clipboard_read_finish")
	gtk(&gdkTextureSaveToPngBytes, "gdk_texture_save_to_png_bytes")
	purego.RegisterLibFunc(&gInputStreamReadBytesAsync, gioLib, "g_input_stream_read_bytes_async")
	purego.RegisterLibFunc(&gInputStreamReadBytesFinish, gioLib, "g_input_stream_read_bytes_finish")
	purego.RegisterLibFunc(&gInputStreamCloseAsync, gioLib, "g_input_stream_close_async")
	purego.RegisterLibFunc(&gBytesGetData, glibLib, "g_bytes_get_data")
	purego.RegisterLibFunc(&gBytesUnref, glibLib, "g_bytes_unref")
	cbPasteClipboard = newCallback(pasteClipboardCallback)
}

// caretMoved reports where v's caret is, while its editor has the focus.
func (w *window) caretMoved(v *view) {
	if w.focus != v || v.entry == 0 || !v.events.Has(driver.EventCaretMove) {
		return
	}
	var at grapheneRect
	var from uintptr
	if v.textView != 0 {
		var strong, weak gdkRectangle
		gtkTextViewGetCursorLocations(v.textView, 0, &strong, &weak)
		var x, y int32
		gtkTextViewBufferToWindowCoords(v.textView, 1, strong.x, strong.y, &x, &y) // GTK_TEXT_WINDOW_WIDGET
		at, from = grapheneRect{float32(x), float32(y), float32(max(strong.w, 1)), float32(strong.h)}, v.textView
	} else if text := gtkEditableGetDelegate(v.entry); text != 0 {
		var weak grapheneRect
		gtkTextComputeCursorExtents(text, uintptr(gtkEditableGetPosition(v.entry)), &at, &weak)
		at.w, from = max(at.w, 1), text
	}
	ev := driver.Event{Kind: driver.EventCaretMove, ID: v.id}
	var out graphenePoint
	if at.h > 0 && gtkWidgetComputePoint(from, v.widget, &graphenePoint{at.x, at.y}, &out) != 0 {
		ev.Point = driver.Point{X: driver.Points(out.x), Y: driver.Points(out.y)}
		ev.Size = driver.Size{Width: driver.Points(at.w), Height: driver.Points(at.h)}
	}
	w.engine.Dispatch(ev)
}

// pasteClipboardCallback hears the user paste into an editor: it keeps
// the text out when the input only reports pastes, and reads the
// clipboard to report it.
func pasteClipboardCallback(widget uintptr, data uintptr) {
	w, v := editorOf(widget, data)
	if v == nil || !v.events.Has(driver.EventPaste) {
		return
	}
	if v.fs.Config.PasteBehavior == driver.PasteReport {
		gSignalStopEmissionByName(widget, cstr("paste-clipboard"))
	}
	clip := gtkWidgetGetClipboard(widget)
	if clip == 0 {
		return
	}
	formats := gdkClipboardGetFormats(clip)
	hasHTML := formats != 0 && hasPasteHTMLFormat(func(mime string) bool {
		return gdkContentFormatsContainMimeType(formats, cstr(mime)) != 0
	})
	p := &pasting{v: v, clip: clip, ev: &driver.PasteEvent{}, hasHTML: hasHTML}
	w.paste = p
	switch {
	case formats != 0 && gdkContentFormatsContainGtype(formats, gdkFileListGetType()):
		gdkClipboardReadValueAsync(clip, gdkFileListGetType(), 0, 0, cbAsync, async(func(source, result uintptr) {
			value := gdkClipboardReadValueFinish(source, result, 0)
			if w.closing || w.paste != p {
				return
			}
			if value != 0 {
				p.ev.URIs = fileURIs(value)
			}
			w.pasteHTMLOrText(p)
		}))
	case formats != 0 && gdkContentFormatsContainGtype(formats, gdkTextureGetType()):
		gdkClipboardReadTextureAsync(clip, 0, cbAsync, async(func(source, result uintptr) {
			texture := gdkClipboardReadTextureFinish(source, result, 0)
			if texture != 0 {
				defer gObjectUnref(texture)
			}
			if w.closing || w.paste != p {
				return
			}
			if texture != 0 {
				if b := texturePNG(texture); len(b) > 0 {
					p.ev.Files = []driver.PastedFile{{MIME: "image/png", Data: b}}
				}
			}
			w.pasteHTMLOrText(p)
		}))
	default:
		w.pasteHTMLOrText(p)
	}
}

// pasteHTMLOrText reads text/html asynchronously when offered, then reads plain text and dispatches EventPaste.
func (w *window) pasteHTMLOrText(p *pasting) {
	if w.closing || w.paste != p {
		return
	}
	if !p.hasHTML {
		w.readPasteText(p)
		return
	}
	mimes := stringArray(pasteHTMLMIMETypes)
	gdkClipboardReadAsync(p.clip, mimes.array(), 0, 0, cbAsync, async(func(source, result uintptr) {
		stream := gdkClipboardReadFinish(source, result, 0, 0)
		if stream == 0 {
			w.readPasteText(p)
			return
		}
		if w.closing || w.paste != p {
			closePasteStream(stream)
			return
		}
		w.readPasteHTMLChunk(p, stream)
	}))
	runtime.KeepAlive(mimes)
}

func (w *window) readPasteHTMLChunk(p *pasting, stream uintptr) {
	gInputStreamReadBytesAsync(stream, pasteHTMLChunk, 0, 0, cbAsync, async(func(source, result uintptr) {
		b := gInputStreamReadBytesFinish(source, result, 0)
		if b == 0 {
			closePasteStream(stream)
			w.readPasteText(p)
			return
		}
		var n uintptr
		ptr := gBytesGetData(b, &n)
		if w.closing || w.paste != p {
			gBytesUnref(b)
			closePasteStream(stream)
			return
		}
		if n == 0 {
			gBytesUnref(b)
			closePasteStream(stream)
			p.ev.HTML = decodeClipboardHTML(p.html)
			w.readPasteText(p)
			return
		}
		var capped bool
		if ptr != 0 {
			p.html, capped = appendPasteHTML(p.html, unsafe.Slice((*byte)(cPointer(ptr)), n))
		}
		gBytesUnref(b)
		if capped {
			closePasteStream(stream)
			p.ev.HTML = decodeClipboardHTML(p.html)
			w.readPasteText(p)
			return
		}
		w.readPasteHTMLChunk(p, stream)
	}))
}

func closePasteStream(stream uintptr) {
	gInputStreamCloseAsync(stream, 0, 0, 0, 0)
	gObjectUnref(stream)
}

func (w *window) readPasteText(p *pasting) {
	if w.closing || w.paste != p {
		return
	}
	gdkClipboardReadTextAsync(p.clip, 0, cbAsync, async(func(source, result uintptr) {
		s := gdkClipboardReadTextFinish(source, result, 0)
		if s != 0 {
			defer gFree(s)
		}
		if w.closing || w.paste != p {
			return
		}
		w.paste = nil
		if s != 0 {
			p.ev.Text = goString(s)
		}
		if !p.v.deleted {
			w.engine.Dispatch(driver.Event{Kind: driver.EventPaste, ID: p.v.id, Data: p.ev})
		}
	}))
}
