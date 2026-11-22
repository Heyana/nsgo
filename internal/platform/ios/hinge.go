//go:build ios

package ios

import (
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

// regionKinds are UIViewReservedRegionKind's occlusion and division kinds, from iOS 27.1; zero before it.
var regionKinds struct {
	loaded              bool
	occlusion, division objc.ID
}

// reservedRegions returns the surface view's active reserved regions in its coordinates, as layoutSubviews reports its size; none on a system without them.
func (s *surface) reservedRegions() []reservedRegion {
	if !regionKinds.loaded {
		regionKinds.loaded = true
		if c := apple.Class("UIViewReservedRegionKind"); c != 0 {
			// The kinds come autoreleased: keep them for the life of the app.
			regionKinds.occlusion = objc.ID(c).Send(sel("occlusionRegionKind")).Send(sel("retain"))
			regionKinds.division = objc.ID(c).Send(sel("divisionRegionKind")).Send(sel("retain"))
		}
	}
	if regionKinds.occlusion == 0 || !objc.Send[bool](s.native, sel("respondsToSelector:"), sel("reservedRegionsOfKind:")) {
		return nil
	}
	var out []reservedRegion
	for _, kind := range []struct {
		id       objc.ID
		division bool
	}{{regionKinds.occlusion, false}, {regionKinds.division, true}} {
		list := s.native.Send(sel("reservedRegionsOfKind:"), kind.id)
		for i := range objc.Send[uint](list, sel("count")) {
			r := list.Send(sel("objectAtIndex:"), i)
			if !objc.Send[bool](r, sel("isActive")) {
				continue
			}
			f := objc.Send[apple.CGRect](r, sel("frame"))
			out = append(out, reservedRegion{division: kind.division, frame: driver.Bounds{
				Origin: driver.Point{X: driver.Points(f.Origin.X), Y: driver.Points(f.Origin.Y)},
				Size:   driver.Size{Width: driver.Points(f.Size.Width), Height: driver.Points(f.Size.Height)},
			}})
		}
	}
	return out
}

// geometry is the surface view's fold geometry at size, as layoutSubviews reports it: its active reserved regions, read with its safe area, and the hinge's latest reading. Regions that may be in another coordinate space, read within regionHold of a size change or reaching past the bounds, are left out, giving the one-region form with the hinge's state; after a size change the view is laid out again to read them (regionReads).
func (s *surface) geometry(size driver.Size) driver.Geometry {
	trusted, changed := s.regionClock.read(size, time.Now())
	if changed {
		s.layOutAfter(regionReads...)
	}
	var regions []reservedRegion
	if trusted {
		regions = s.reservedRegions()
		if !regionsFit(size, regions) {
			// Not read again for this alone: a region that never fits, as one past an embedded view's edge might, would lay the view out forever.
			regions = nil
		}
	}
	in := objc.Send[edgeInsets](s.native, sel("safeAreaInsets"))
	safe := driver.Edges{Top: driver.Points(in.Top), Right: driver.Points(in.Right), Bottom: driver.Points(in.Bottom), Left: driver.Points(in.Left)}
	return foldGeometry(size, safe, regions, s.hinge)
}

// layOutAfter lays the surface view out again after each delay, unless it closed or changed size again since.
func (s *surface) layOutAfter(delays ...time.Duration) {
	since := s.regionClock.since
	for _, d := range delays {
		time.AfterFunc(d, func() {
			apple.OnMain(func() {
				if !s.closed && s.regionClock.since == since {
					s.native.Send(sel("setNeedsLayout"))
				}
			})
		})
	}
}

// watchHinge adds a UIHingeInteraction to the surface view on iOS 27.1: each update keeps the hinge's status and angle and lays the view out again, so a change of state alone is a resize to the same size with the new geometry.
func (s *surface) watchHinge() {
	c := apple.Class("UIHingeInteraction")
	if c == 0 {
		return
	}
	handler := objcblock.New(func(_ objc.Block, _, update objc.ID) {
		h := update.Send(sel("hinge"))
		if h == 0 {
			s.hinge = hingeReading{}
		} else {
			s.hinge = hingeReading{known: true, status: objc.Send[int](h, sel("status")), angle: objc.Send[float64](h, sel("angle"))}
		}
		s.native.Send(sel("setNeedsLayout"))
	})
	interaction := objc.ID(c).Send(selAlloc).Send(sel("initWithUpdateHandler:"), handler)
	handler.Release()
	s.native.Send(sel("addInteraction:"), interaction)
	interaction.Send(selRelease)
}

// watchVerticalBarEdge subscribes to the system traits that affect verticalBarEdge on iOS 27.1 and later, so changes reach Env.VerticalBarEdge even if UIKit skips the deprecated traitCollectionDidChange:.
func (s *surface) watchVerticalBarEdge() {
	tc := apple.Class("UITraitCollection")
	if tc == 0 || !objc.Send[bool](objc.ID(tc), sel("respondsToSelector:"), sel("systemTraitsAffectingVerticalBarEdge")) ||
		!objc.Send[bool](s.native, sel("respondsToSelector:"), sel("registerForTraitChanges:withHandler:")) {
		return
	}
	traits := objc.ID(tc).Send(sel("systemTraitsAffectingVerticalBarEdge"))
	if traits == 0 {
		return
	}
	handler := objcblock.New(func(_ objc.Block, _, _ objc.ID) {
		s.layoutChanged()
	})
	reg := s.native.Send(sel("registerForTraitChanges:withHandler:"), traits, handler)
	handler.Release()
	if reg != 0 && objc.Send[bool](s.native, sel("respondsToSelector:"), sel("unregisterForTraitChanges:")) {
		reg.Send(sel("retain"))
		native := s.native
		s.stops = append(s.stops, func() {
			native.Send(sel("unregisterForTraitChanges:"), reg)
			reg.Send(selRelease)
		})
	}
}

// verticalBarEdge is the edge the system chose for a vertical bar, from the trait collection on iOS 27.1.
func verticalBarEdge(traits objc.ID) driver.VerticalBarEdge {
	if !objc.Send[bool](traits, sel("respondsToSelector:"), sel("verticalBarEdge")) {
		return driver.VerticalBarUnspecified
	}
	return mapVerticalBarEdge(objc.Send[int](traits, sel("verticalBarEdge")))
}
