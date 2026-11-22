//go:build linux && !android

package linux

import "sync"

// systemDirection is GTK's default direction as the app started, the
// system's, before the app's replaced it.
var systemDirection = sync.OnceValue(func() int32 { return gtkWidgetGetDefaultDirection() })

// gtkTextDir returns GTK's GtkTextDirection for rtl.
func gtkTextDir(rtl bool) int32 {
	if rtl {
		return gtkTextDirRTL
	}
	return gtkTextDirLTR
}

// directTree sets dir on widget and every descendant widget beneath it.
// GTK's gtk_widget_set_direction applies only to a single widget rather than
// cascading to children, so embedded surfaces and popovers walk their subtrees.
func directTree(widget uintptr, dir int32) {
	if widget == 0 || dir == 0 {
		return
	}
	gtkWidgetSetDirection(widget, dir)
	for c := gtkWidgetGetFirstChild(widget); c != 0; c = gtkWidgetGetNextSibling(c) {
		directTree(c, dir)
	}
}

// setDirection lays GTK's widgets out in the app's direction, as its
// language is written, whatever the system's: its menus, fields and
// native components. A standalone app makes it GTK's default, which every
// widget without one of its own follows; an embedded surface gives it to
// its own widgets, leaving the host's alone.
func (w *window) setDirection(rtl bool) {
	systemDirection()
	w.dir = gtkTextDir(rtl)
	if !w.embedded {
		gtkWidgetSetDefaultDirection(w.dir)
		if w.surface != 0 {
			gtkWidgetSetDirection(w.surface, w.dir)
			gtkWidgetQueueDraw(w.surface)
		}
	} else if w.surface != 0 {
		directTree(w.surface, w.dir)
		gtkWidgetQueueDraw(w.surface)
	}
	if w.popover != 0 {
		directTree(w.popover, w.dir)
	}
}
