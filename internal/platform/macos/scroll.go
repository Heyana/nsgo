//go:build darwin && !ios

package macos

import (
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/internal/driver"
	"github.com/oneo-me/nsgo/internal/platform/apple"
)

// Scroll behavior on NSScrollView, as NSH's macOS backend scrolls:
// trackpad gestures rubber-band past the edges unless NoBounces, and the
// momentum after them is the system's, so DecelerationRate does nothing
// here. Snapping views snap from the release instead of gliding, and a
// mouse wheel settles on a snap point once it rests. A gesture brackets
// the drag events and the system's momentum the momentum events; a
// scroller drag is a drag too. Pulling a refreshable view past its top
// with the trackpad refreshes it, and a spinner keeps room above the
// content while it refreshes. The keyboard props and zooming do nothing
// on the Mac, as in NSH.

const (
	// refreshPull is how far past the top a release refreshes, and
	// refreshSpace the room the spinner keeps above the content.
	refreshPull  = 64
	refreshSpace = 56
	// snapDuration is how long a snap's animation runs.
	snapDuration = 300 * time.Millisecond
	// wheelIdle is how long after its last notch a mouse wheel rests.
	wheelIdle = 150 * time.Millisecond
	// wheelSnap is the speed (points per millisecond) a rested wheel
	// snaps with: on to the next snap point that way.
	wheelSnap = 0.6
)

// phaseMayBegin is the NSEvent phase of fingers resting on the trackpad
// (the others are gesture.go's).
const phaseMayBegin = 32

// edgeInsets is NSEdgeInsets.
type edgeInsets struct{ Top, Left, Bottom, Right float64 }

// scrollState is what a scroll view keeps beside its NSScrollView.
type scrollState struct {
	config driver.ScrollConfig
	// gesture is a trackpad gesture under way; bar a scroller drag.
	gesture, bar bool
	// snapping swallows the system's momentum while a snap runs, which
	// ends with EventMomentumEnd when snapGen still matches.
	snapping bool
	snapGen  int
	// The gesture's last move, for its velocity (points per
	// millisecond along the axis, positive as the offset grows).
	last     time.Time
	velocity float32
	// A mouse wheel's notches since it last rested.
	wheelGen   int
	wheelTotal float64
	// refreshing shows spinner, which keeps refreshSpace above the
	// content.
	refreshing bool
	spinner    objc.ID
	// applied counts the core's ItemScrollTo and ItemScrollBy items,
	// reported with each offset; sticky are its sticky cells.
	applied uint32
	sticky  []driver.StickyCell
	// shown is the offset the clip view last reported, which decides
	// what of the content can be seen (view.shown).
	shown driver.Point
}

// insets returns the content insets the view applies: the app's and the
// spinner's room.
func (s *scrollState) insets() driver.Edges {
	in := s.config.Behavior.ContentInset
	if s.refreshing {
		in.Top += refreshSpace
	}
	return in
}

// offset returns the scroll view's offset: UIKit's contentOffset.
func (v *view) offset() driver.Point {
	o := objc.Send[apple.CGRect](v.clip, selBounds).Origin
	return driver.Point{X: driver.Points(o.X), Y: driver.Points(o.Y)}
}

// main returns p along the view's axis.
func (v *view) main(p driver.Point) driver.Points {
	if v.sc.config.Horizontal {
		return p.X
	}
	return p.Y
}

// limits returns the least and greatest offsets.
func (v *view) limits() (lo, hi driver.Point) {
	doc := objc.Send[apple.CGRect](v.content, selBounds).Size
	seen := objc.Send[apple.CGRect](v.clip, selBounds).Size
	in := v.sc.insets()
	lo = driver.Point{X: -in.Left, Y: -in.Top}
	hi = driver.Point{X: max(driver.Points(doc.Width-seen.Width)+in.Right, lo.X),
		Y: max(driver.Points(doc.Height-seen.Height)+in.Bottom, lo.Y)}
	return lo, hi
}

// configureScroll applies a scroll view's configuration.
func (v *view) configureScroll(c *driver.ScrollConfig) {
	s := v.sc
	old := s.insets()
	wasRefreshing := s.refreshing
	s.config = *c
	bars := !c.HideIndicator && !c.Behavior.NoScroll
	v.native.Send(sel("setHasVerticalScroller:"), !c.Horizontal && bars)
	v.native.Send(sel("setHasHorizontalScroller:"), c.Horizontal && bars)
	elastic := 0 // NSScrollElasticityAutomatic
	if c.Behavior.NoBounces {
		elastic = 1 // NSScrollElasticityNone
	}
	v.native.Send(sel("setVerticalScrollElasticity:"), elastic)
	v.native.Send(sel("setHorizontalScrollElasticity:"), elastic)
	s.refreshing = c.Refreshing && !c.Horizontal
	v.showSpinner(s.refreshing)
	if in := s.insets(); in != old {
		at := v.offset()
		v.native.Send(sel("setContentInsets:"), edgeInsets{float64(in.Top), float64(in.Left), float64(in.Bottom), float64(in.Right)})
		// A view resting at its start stays there.
		if at.Y <= -old.Top+0.5 {
			at.Y = -in.Top
		}
		if at.X <= -old.Left+0.5 {
			at.X = -in.Left
		}
		v.scrollTo(at, !wasRefreshing && s.refreshing || wasRefreshing && !s.refreshing)
	}
}

// showSpinner shows or hides the refresh spinner at the view's top.
func (v *view) showSpinner(on bool) {
	s := v.sc
	if on && s.spinner == 0 {
		const side = 20
		p := objc.ID(objc.GetClass("NSProgressIndicator")).Send(selNew)
		p.Send(sel("setStyle:"), 1) // spinning
		p.Send(sel("setIndeterminate:"), true)
		p.Send(sel("setDisplayedWhenStopped:"), false)
		h := objc.Send[apple.CGRect](v.native, selBounds).Size
		y := h.Height - (refreshSpace+side)/2 // AppKit's y grows upward
		if objc.Send[bool](v.native, sel("isFlipped")) {
			y = (refreshSpace - side) / 2
		}
		p.Send(selSetFrame, apple.Rect((h.Width-side)/2, y, side, side))
		p.Send(sel("setAutoresizingMask:"), uint(1|4|8)) // centered, at the top
		v.native.Send(selAddSubview, p)
		p.Send(sel("startAnimation:"), objc.ID(0))
		s.spinner = p
	} else if !on && s.spinner != 0 {
		s.spinner.Send(sel("stopAnimation:"), objc.ID(0))
		s.spinner.Send(sel("removeFromSuperview"))
		s.spinner.Send(selRelease)
		s.spinner = 0
	}
}

// scrollEvent reports kind on the scroll view at its offset.
func (v *view) scrollEvent(kind driver.EventKind) {
	v.report(driver.Event{Kind: kind, Point: v.offset(), Count: v.sc.applied})
}

// scrollBy moves the offset by d unclamped: the core keeps the visible
// content in place as rows above it change size.
func (v *view) scrollBy(d driver.Point) {
	if v.clip == 0 {
		return
	}
	at := v.offset()
	v.clip.Send(sel("setBoundsOrigin:"), apple.CGPoint{X: float64(at.X + d.X), Y: float64(at.Y + d.Y)})
	v.native.Send(sel("reflectScrolledClipView:"), v.clip)
}

// pinSticky moves the scroll view's sticky cells for its offset, as
// NSH's StickyCells: each by clamp(offset - start, 0, end - start) from
// its frame.
func (v *view) pinSticky() {
	at := v.main(v.offset())
	for _, c := range v.sc.sticky {
		if int(c.Cell) < len(v.surface.views) {
			if cv := v.surface.views[c.Cell]; cv != nil {
				cv.stick(c.StickyShift(at), v.sc.config.Horizontal)
			}
		}
	}
}

// setSticky replaces the scroll view's sticky cells; cells no longer
// sticky go back to their frames.
func (v *view) setSticky(cells []driver.StickyCell) {
	for _, c := range v.sc.sticky {
		if int(c.Cell) < len(v.surface.views) {
			if cv := v.surface.views[c.Cell]; cv != nil {
				cv.stick(0, false)
			}
		}
	}
	v.sc.sticky = cells
	v.pinSticky()
}

// stick moves a sticky cell d along its list from its frame.
func (v *view) stick(d driver.Points, horizontal bool) {
	shift := driver.Point{Y: d}
	if horizontal {
		shift = driver.Point{X: d}
	}
	if shift != v.shift {
		v.shift = shift
		if v.framed {
			v.place(v.frame)
		}
	}
}

// interruptSnap ends a running snap, which ends its momentum.
func (v *view) interruptSnap() {
	if v.sc.snapping {
		v.sc.snapping = false
		v.sc.snapGen++
		v.scrollEvent(driver.EventMomentumEnd)
	}
}

// snapFrom settles a snapping view released at velocity (points per
// millisecond along its axis), and reports whether it snaps.
func (v *view) snapFrom(velocity float32) bool {
	b := &v.sc.config.Behavior
	if !b.Snaps() {
		return false
	}
	at := v.offset()
	lo, hi := v.limits()
	pos := v.main(at)
	if pos < v.main(lo)-0.5 || pos > v.main(hi)+0.5 {
		return false // the rubber band springs back first
	}
	doc := objc.Send[apple.CGRect](v.content, selBounds).Size
	seen := objc.Send[apple.CGRect](v.clip, selBounds).Size
	viewport, content := driver.Points(seen.Height), driver.Points(doc.Height)+b.ContentInset.Bottom
	if v.sc.config.Horizontal {
		viewport, content = driver.Points(seen.Width), driver.Points(doc.Width)+b.ContentInset.Right
	}
	to, _ := b.SnapTarget(pos, driver.Projected(pos, velocity, b.Deceleration()), velocity, viewport, content)
	target := at
	if v.sc.config.Horizontal {
		target.X = to
	} else {
		target.Y = to
	}
	v.sc.snapping = true
	v.sc.snapGen++
	gen := v.sc.snapGen
	v.scrollEvent(driver.EventMomentumStart)
	v.scrollTo(target, true)
	time.AfterFunc(snapDuration, func() {
		apple.OnMain(func() {
			if v.sc.snapping && v.sc.snapGen == gen {
				v.sc.snapping = false
				v.scrollEvent(driver.EventMomentumEnd)
			}
		})
	})
	return true
}

// scrollWheelOwn follows the wheel events a scroll view takes: the drag
// and momentum events, snapping and pulls to refresh. It reports whether
// AppKit should not see the event, while a snap replaces its momentum.
func (v *view) scrollWheelOwn(event objc.ID, phase, momentum uint, dx, dy float64) bool {
	s := v.sc
	d := dy
	if s.config.Horizontal && dx != 0 {
		d = dx
	}
	switch {
	case phase&(phaseBegan|phaseMayBegin) != 0:
		v.interruptSnap()
		if phase&phaseBegan != 0 {
			s.gesture, s.velocity, s.last = true, 0, time.Now()
			v.scrollEvent(driver.EventScrollDragStart)
		}
	case phase&phaseChanged != 0:
		now := time.Now()
		if dt := float32(now.Sub(s.last).Seconds() * 1000); dt > 0 {
			// Deltas show what is above or left as they grow.
			s.velocity = 0.5*s.velocity + 0.5*float32(-d)/max(dt, 4)
		}
		s.last = now
	case phase&(phaseEnded|phaseCancelled) != 0:
		if !s.gesture {
			return false
		}
		s.gesture = false
		v.scrollEvent(driver.EventScrollDragEnd)
		if time.Since(s.last) > 50*time.Millisecond {
			s.velocity = 0 // the fingers rested before lifting
		}
		if c := &s.config; c.Refreshable && !c.Horizontal && !s.refreshing &&
			v.offset().Y <= -c.Behavior.ContentInset.Top-refreshPull {
			// Shown right away, as UIRefreshControl does; the next config
			// says whether the app keeps it.
			s.refreshing = true
			v.report(driver.Event{Kind: driver.EventRefresh, Point: v.offset(), Count: v.sc.applied})
			v.showSpinner(true)
			in := s.insets()
			v.native.Send(sel("setContentInsets:"), edgeInsets{float64(in.Top), float64(in.Left), float64(in.Bottom), float64(in.Right)})
			return false
		}
		v.snapFrom(s.velocity)
	case momentum != 0 && s.snapping:
		return true
	case momentum&phaseBegan != 0:
		v.scrollEvent(driver.EventMomentumStart)
	case momentum&(phaseEnded|phaseCancelled) != 0:
		v.scrollEvent(driver.EventMomentumEnd)
	case phase == 0 && momentum == 0 && s.config.Behavior.Snaps():
		// A mouse wheel: settle once it rests.
		v.interruptSnap()
		s.wheelTotal += -d
		s.wheelGen++
		gen := s.wheelGen
		time.AfterFunc(wheelIdle, func() {
			apple.OnMain(func() {
				if s.wheelGen != gen {
					return
				}
				var velocity float32
				switch {
				case s.wheelTotal >= 1:
					velocity = wheelSnap
				case s.wheelTotal <= -1:
					velocity = -wheelSnap
				}
				s.wheelTotal = 0
				v.snapFrom(velocity)
			})
		})
	}
	return false
}

// liveScroll follows NSScrollView's live scrolls: one outside a trackpad
// gesture or a wheel is a scroller drag, reported as a drag.
func liveScroll(note objc.ID, start bool) {
	v := views[note.Send(sel("object"))]
	if v == nil || v.sc == nil {
		return
	}
	s := v.sc
	switch {
	case start && !s.gesture && !s.snapping && !s.bar:
		pressed := objc.Send[uint](objc.ID(objc.GetClass("NSEvent")), sel("pressedMouseButtons"))
		if pressed&1 != 0 {
			s.bar = true
			v.scrollEvent(driver.EventScrollDragStart)
		}
	case !start && s.bar:
		s.bar = false
		v.scrollEvent(driver.EventScrollDragEnd)
		v.snapFrom(0)
	}
}

// canScrollAxis reports whether the offset can move by a wheel delta d
// along the axis: positive deltas show what is above or left.
func canScrollAxis(d, at, lo, hi float64) bool {
	const slack = 0.5 // offsets land on fractions of a point
	return d > 0 && at > lo+slack || d < 0 && at < hi-slack
}
