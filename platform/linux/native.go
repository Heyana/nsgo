//go:build linux && !android

package linux

import (
	"github.com/ebitengine/purego"
)

// hosted is a native widget filling a view, as NSH's natives are: a web
// view's page or placeholder, or a video's picture. It is a child of the
// view's widget, after the view's children, given the view's frame at
// every allocation, so ancestors clip it to their rounded bounds, later
// views cover it and the view's opacity and transform apply to it.
type hosted struct {
	child   uintptr
	signals []signal // connected to the view's own objects
}

// signal is a handler connected to an instance, disconnected when its
// view goes so nothing reaches a view the window has forgotten.
type signal struct {
	instance uintptr
	id       uint64
}

// hosted returns v's hosted widget, nil for views without one.
func (v *view) hosted() *hosted {
	switch {
	case v.web != nil:
		return &v.web.hosted
	case v.video != nil:
		return &v.video.hosted
	}
	return nil
}

// host makes child v's hosted widget. A child that is not a target
// leaves pointer events to the view.
func (w *window) host(v *view, h *hosted, child uintptr, target bool) {
	w.setHostedChild(v, h, child)
	if child != 0 {
		gtkWidgetSetCanTarget(child, boolInt(target))
	}
}

// setHostedChild replaces what v shows of its own.
func (w *window) setHostedChild(v *view, h *hosted, child uintptr) {
	if h.child != 0 {
		w.unparentWidget(h.child)
		gObjectUnref(h.child)
	}
	h.child = child
	if child != 0 {
		gObjectRefSink(child)
		gtkWidgetSetParent(child, v.widget)
		gtkWidgetQueueAllocate(v.widget)
	}
}

// connect connects fn to the signal of instance, one of v's native
// objects, until v's widget goes; the callback finds v by instance.
func (w *window) connect(v *view, instance uintptr, name string, fn uintptr) {
	h := v.hosted()
	w.natives[instance] = v
	h.signals = append(h.signals, signal{instance, gSignalConnect(instance, name, fn, w.handle)})
}

// unhost disconnects h's handlers and lets what it shows go.
func (w *window) unhost(v *view, h *hosted) {
	for _, s := range h.signals {
		gSignalHandlerDisconnect(s.instance, s.id)
		delete(w.natives, s.instance)
	}
	h.signals = nil
	w.setHostedChild(v, h, 0)
}

// nativeOf returns the live view that one of its native objects, p,
// belongs to.
func nativeOf(data, p uintptr) (*window, *view) {
	w := windowOf(data)
	if w == nil || w.closing {
		return nil, nil
	}
	v := w.natives[p]
	if v == nil || v.deleted {
		return nil, nil
	}
	return w, v
}

// controlAt returns the view whose hosted control (a page, or a video
// with GTK's controls) is or holds widget, or nil.
func (w *window) controlAt(widget uintptr) *view {
	for ; widget != 0 && widget != w.surface; widget = gtkWidgetGetParent(widget) {
		r, ok := lookupWidget(gtkWidgetGetParent(widget))
		if !ok || r.v == nil {
			continue
		}
		v := r.v
		if h := v.hosted(); h != nil && h.child == widget &&
			(v.web != nil && v.web.widget == widget || v.video != nil && v.video.controls) {
			return v
		}
		return nil
	}
	return nil
}

// hostsControls reports whether a page or a video's controls is hosted.
func (w *window) hostsControls() bool {
	for p, v := range w.natives {
		if v.web != nil && v.web.widget == p || v.video != nil && v.video.controls {
			return true
		}
	}
	return false
}

// nativeAt reports whether widget is, or is inside, a native widget that
// takes its own presses: an entry, a page or a video's controls.
func (w *window) nativeAt(widget uintptr) bool {
	for p := widget; p != 0 && p != w.surface; p = gtkWidgetGetParent(p) {
		if _, ok := w.entries[p]; ok {
			return true
		}
	}
	return w.controlAt(widget) != nil
}

// GTK's enums for hosted widgets.
const (
	gtkJustifyCenter = 2 // GTK_JUSTIFY_CENTER

	gtkContentFitFill      = 0
	gtkContentFitContain   = 1
	gtkContentFitCover     = 2
	gtkContentFitScaleDown = 3
)

// The bindings of hosted widgets: web view placeholders and videos.
var (
	gObjectNew                     func(uintptr, *byte, ...any) uintptr
	gSignalHandlerDisconnect       func(uintptr, uint64)
	gFileNewForPath                func(*byte) uintptr
	gFileNewForURI                 func(*byte) uintptr
	gtkWidgetSetCanTarget          func(uintptr, int32)
	gtkGestureSetState             func(uintptr, int32) int32
	gtkLabelNew                    func(*byte) uintptr
	gtkLabelSetMarkup              func(uintptr, *byte)
	gtkLabelSetWrap                func(uintptr, int32)
	gtkLabelSetEllipsize           func(uintptr, int32)
	gtkLabelSetLines               func(uintptr, int32)
	gtkLabelSetJustify             func(uintptr, int32)
	gtkMediaFileNewForFile         func(uintptr) uintptr
	gtkMediaFileClear              func(uintptr)
	gtkMediaStreamSetMuted         func(uintptr, int32)
	gtkMediaStreamSetVolume        func(uintptr, float64)
	gtkMediaStreamSetLoop          func(uintptr, int32)
	gtkMediaStreamSetPlaying       func(uintptr, int32)
	gtkMediaStreamGetPlaying       func(uintptr) int32
	gtkMediaStreamGetEnded         func(uintptr) int32
	gtkMediaStreamIsPrepared       func(uintptr) int32
	gtkMediaStreamHasVideo         func(uintptr) int32
	gtkMediaStreamHasAudio         func(uintptr) int32
	gtkMediaStreamGetDuration      func(uintptr) int64
	gtkMediaStreamGetTimestamp     func(uintptr) int64
	gtkMediaStreamIsSeekable       func(uintptr) int32
	gtkMediaStreamSeek             func(uintptr, int64)
	gtkMediaStreamGetError         func(uintptr) uintptr
	gdkPaintableGetIntrinsicWidth  func(uintptr) int32
	gdkPaintableGetIntrinsicHeight func(uintptr) int32
	gtkPictureNew                  func() uintptr
	gtkPictureSetPaintable         func(uintptr, uintptr)
	gtkPictureSetContentFit        func(uintptr, int32)
	gtkPictureSetCanShrink         func(uintptr, int32)
	gtkVideoNew                    func() uintptr
	gtkVideoSetAutoplay            func(uintptr, int32)
	gtkVideoSetMediaStream         func(uintptr, uintptr)
	gtkVideoSetLoop                func(uintptr, int32)
)

// bindNative registers the bindings of hosted widgets; bind calls it.
func bindNative() {
	purego.RegisterLibFunc(&gObjectNew, gobjectLib, "g_object_new")
	purego.RegisterLibFunc(&gSignalHandlerDisconnect, gobjectLib, "g_signal_handler_disconnect")
	purego.RegisterLibFunc(&gFileNewForPath, gioLib, "g_file_new_for_path")
	purego.RegisterLibFunc(&gFileNewForURI, gioLib, "g_file_new_for_uri")
	gtk := func(fn any, name string) { purego.RegisterLibFunc(fn, gtkLib, name) }
	gtk(&gtkWidgetSetCanTarget, "gtk_widget_set_can_target")
	gtk(&gtkGestureSetState, "gtk_gesture_set_state")
	gtk(&gtkLabelNew, "gtk_label_new")
	gtk(&gtkLabelSetMarkup, "gtk_label_set_markup")
	gtk(&gtkLabelSetWrap, "gtk_label_set_wrap")
	gtk(&gtkLabelSetEllipsize, "gtk_label_set_ellipsize")
	gtk(&gtkLabelSetLines, "gtk_label_set_lines")
	gtk(&gtkLabelSetJustify, "gtk_label_set_justify")
	gtk(&gtkMediaFileNewForFile, "gtk_media_file_new_for_file")
	gtk(&gtkMediaFileClear, "gtk_media_file_clear")
	gtk(&gtkMediaStreamSetMuted, "gtk_media_stream_set_muted")
	gtk(&gtkMediaStreamSetVolume, "gtk_media_stream_set_volume")
	gtk(&gtkMediaStreamSetLoop, "gtk_media_stream_set_loop")
	gtk(&gtkMediaStreamSetPlaying, "gtk_media_stream_set_playing")
	gtk(&gtkMediaStreamGetPlaying, "gtk_media_stream_get_playing")
	gtk(&gtkMediaStreamGetEnded, "gtk_media_stream_get_ended")
	gtk(&gtkMediaStreamIsPrepared, "gtk_media_stream_is_prepared")
	gtk(&gtkMediaStreamHasVideo, "gtk_media_stream_has_video")
	gtk(&gtkMediaStreamHasAudio, "gtk_media_stream_has_audio")
	gtk(&gtkMediaStreamGetDuration, "gtk_media_stream_get_duration")
	gtk(&gtkMediaStreamGetTimestamp, "gtk_media_stream_get_timestamp")
	gtk(&gtkMediaStreamIsSeekable, "gtk_media_stream_is_seekable")
	gtk(&gtkMediaStreamSeek, "gtk_media_stream_seek")
	gtk(&gtkMediaStreamGetError, "gtk_media_stream_get_error")
	gtk(&gdkPaintableGetIntrinsicWidth, "gdk_paintable_get_intrinsic_width")
	gtk(&gdkPaintableGetIntrinsicHeight, "gdk_paintable_get_intrinsic_height")
	gtk(&gtkPictureNew, "gtk_picture_new")
	gtk(&gtkPictureSetPaintable, "gtk_picture_set_paintable")
	gtk(&gtkPictureSetContentFit, "gtk_picture_set_content_fit")
	gtk(&gtkPictureSetCanShrink, "gtk_picture_set_can_shrink")
	gtk(&gtkVideoNew, "gtk_video_new")
	gtk(&gtkVideoSetAutoplay, "gtk_video_set_autoplay")
	gtk(&gtkVideoSetMediaStream, "gtk_video_set_media_stream")
	gtk(&gtkVideoSetLoop, "gtk_video_set_loop")
}
