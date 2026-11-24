//go:build linux && !android

package linux

import (
	"math"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Backdrop filters draw what the window drew behind a view through them,
// under its fill and clipped to its box: the surface is snapshotted up to
// the view, walking its ancestors directly as an effect's sample_backdrop
// does (backdropNode), and the node goes through GSK's blur and color
// matrix nodes. As GTK keeps each widget's render node until the widget
// draws again, every view with backdrop filters is queued to draw at the
// start of each frame (the frame clock's before-paint), so it follows what
// changed behind it. Snapshotting a backdrop for another view reuses the
// one each view last drew rather than snapshotting again.

var (
	cbBackdropBeforePaint uintptr
	backdropsBound        bool
)

func bindBackdrops() {
	if backdropsBound {
		return
	}
	backdropsBound = true
	bindEffects()
	if gtkWidgetGetFrameClock == nil {
		purego.RegisterLibFunc(&gtkWidgetGetFrameClock, gtkLib, "gtk_widget_get_frame_clock")
	}
	cbBackdropBeforePaint = newCallback(backdropBeforePaintCallback)
}

// snapshotBackdrop draws what is behind v through filters into snap,
// clipped to sh.
func (w *window) snapshotBackdrop(v *view, snap uintptr, sh *shape, filters []driver.Filter) {
	bindBackdrops()
	if w.backdrops == nil {
		w.backdrops = map[*view]uintptr{}
	}
	node, known := w.backdrops[v]
	if w.backdropStop == nil {
		if clock := gtkWidgetGetFrameClock(w.surface); clock != 0 && clock != w.backdropClock {
			w.backdropClock = clock
			gSignalConnect(clock, "before-paint", cbBackdropBeforePaint, w.handle)
		}
		if n := w.backdropOf(v, sh, filters); n != 0 {
			if node != 0 {
				gskRenderNodeUnref(node)
			}
			node = n
		}
		w.backdrops[v] = node
	} else if !known {
		w.backdrops[v] = 0
	}
	if node == 0 {
		return
	}
	sh.pushClip(snap)
	gtkSnapshotAppendNode(snap, node)
	gtkSnapshotPop(snap)
}

// backdropOf returns a new node of what the window drew behind v, in v's
// points, through filters: what lies within the blur's reach around v's
// box; 0 where there is nothing behind it.
func (w *window) backdropOf(v *view, sh *shape, filters []driver.Filter) uintptr {
	reach := float32(math.Ceil(3 * float64(backdropBlur(filters))))
	snap := gtkSnapshotNew()
	pops := pushFilters(snap, filters)
	gtkSnapshotPushClip(snap, &grapheneRect{-reach, -reach, sh.w + 2*reach, sh.h + 2*reach})
	w.snapshotBehind(v, snap, reach)
	gtkSnapshotPop(snap)
	for range pops {
		gtkSnapshotPop(snap)
	}
	return gtkSnapshotFreeToNode(snap)
}

// backdropBlur is the widest blur among filters, as a standard deviation
// in points.
func backdropBlur(filters []driver.Filter) driver.Points {
	var blur driver.Points
	for _, f := range filters {
		switch f.Kind {
		case driver.FilterBlur:
			blur = max(blur, driver.Points(f.Amount))
		case driver.FilterDropShadow:
			blur = max(blur, f.Shadow.Blur)
		}
	}
	return blur
}

// snapshotBehind draws what the window drew behind host into snap, in
// host's points: the surface's background, reach points wider than the
// surface so a blur at its edges takes the background rather than
// nothing, and the root view up to host. It reports false where host has
// no backdrop.
func (w *window) snapshotBehind(host *view, snap uintptr, reach float32) bool {
	root := w.view(w.root)
	if w.surface == 0 || root == nil || root.widget == 0 {
		return false
	}
	path, surfaceToHost, ok := backdropPath(root, host)
	if !ok {
		return false
	}
	gtkSnapshotSave(snap)
	defer gtkSnapshotRestore(snap)
	if t := gskMatrixTransform(0, surfaceToHost); t != 0 {
		gtkSnapshotTransform(snap, t)
		gskTransformUnref(t)
	}
	prevStop, prevPath, prevStopped := w.backdropStop, w.backdropPath, w.backdropStopped
	w.backdropStop, w.backdropPath, w.backdropStopped = host, path, false
	defer func() {
		w.backdropStop, w.backdropPath, w.backdropStopped = prevStop, prevPath, prevStopped
	}()
	rect := grapheneRect{-reach, -reach, float32(gtkWidgetGetWidth(w.surface)) + 2*reach, float32(gtkWidgetGetHeight(w.surface)) + 2*reach}
	c := rgba(background(root))
	gtkSnapshotAppendColor(snap, &c, &rect)
	if gtkWidgetGetParent(root.widget) == w.surface {
		gtkSnapshotSave(snap)
		if t := gskMatrixTransform(0, root.placement(0, 0)); t != 0 {
			gtkSnapshotTransform(snap, t)
			gskTransformUnref(t)
		}
		w.snapshotView(root, snap)
		gtkSnapshotRestore(snap)
	}
	return true
}

// backdropBeforePaintCallback queues every view with backdrop filters to
// draw in the frame starting, forgetting those gone or without them.
func backdropBeforePaintCallback(clock, data uintptr) {
	w := windowOf(data)
	if w == nil || w.closing || clock != w.backdropClock {
		return
	}
	for v, node := range w.backdrops {
		if s := w.styleOf(v); v.deleted || v.widget == 0 || s.Effects == nil || len(s.Effects.BackdropFilters) == 0 {
			if node != 0 {
				gskRenderNodeUnref(node)
			}
			delete(w.backdrops, v)
			continue
		}
		gtkWidgetQueueDraw(v.widget)
	}
}
