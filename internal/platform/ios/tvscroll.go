//go:build ios

package ios

import (
	"sync"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// Apple TV remote-surface scrolling for UIScrollView: when a Scroll or List
// overflows its viewport and contains no focusable interactive views (such as
// a long text), its panGestureRecognizer accepts UITouchTypeIndirect and the
// scroll view itself becomes focusable so swiping the Siri Remote's touch
// surface (or clicking its directional ring) scrolls smoothly with native
// inertia. When focusable views exist inside it, UIKit's default touch types
// and focus-driven scrolling are preserved unchanged.

var (
	selAllowedTouchTypes    = sel("allowedTouchTypes")
	selSetAllowedTouchTypes = sel("setAllowedTouchTypes:")
	selCanBecomeFocused     = sel("canBecomeFocused")
)

var indirectTouchTypeNumber = sync.OnceValue(func() objc.ID {
	return objc.ID(apple.Class("NSNumber")).Send(sel("numberWithInteger:"), int(touchTypeIndirect)).Send(sel("retain"))
})

// initTVScroll snapshots UIKit's default panGestureRecognizer.allowedTouchTypes
// on tvOS so indirect scrolling can be enabled and cleanly restored.
func initTVScroll(v *view) {
	if !onTV() || v.sc == nil {
		return
	}
	if pan := v.native.Send(selPanRecognizer); pan != 0 {
		if orig := pan.Send(selAllowedTouchTypes); orig != 0 {
			v.sc.defaultTouchTypes = orig.Send(sel("copy"))
		}
	}
}

// releaseTVScroll releases retained tvOS scroll state when v is deleted.
func releaseTVScroll(v *view) {
	if v.sc != nil && v.sc.defaultTouchTypes != 0 {
		v.sc.defaultTouchTypes.Send(selRelease)
		v.sc.defaultTouchTypes = 0
	}
}

// setTVIndirectPan adds or removes UITouchTypeIndirect on v's UIScrollView pan
// gesture recognizer on tvOS.
func (v *view) setTVIndirectPan(on bool) {
	if !onTV() || v.sc == nil || v.sc.indirectPan == on {
		return
	}
	v.sc.indirectPan = on
	pan := v.native.Send(selPanRecognizer)
	if pan == 0 {
		return
	}
	if on {
		num := indirectTouchTypeNumber()
		var arr objc.ID
		switch {
		case v.sc.defaultTouchTypes == 0:
			arr = objc.ID(apple.Class("NSArray")).Send(sel("arrayWithObject:"), num)
		case objc.Send[bool](v.sc.defaultTouchTypes, sel("containsObject:"), num):
			arr = v.sc.defaultTouchTypes
		default:
			arr = v.sc.defaultTouchTypes.Send(sel("arrayByAddingObject:"), num)
		}
		pan.Send(selSetAllowedTouchTypes, arr)
		return
	}
	if v.sc.defaultTouchTypes != 0 {
		pan.Send(selSetAllowedTouchTypes, v.sc.defaultTouchTypes)
	} else {
		pan.Send(selSetAllowedTouchTypes, objc.ID(apple.Class("NSArray")).Send(sel("array")))
	}
}

// tvScrollEligible reports whether scroll view v is mounted, visible, enabled,
// interactive and inside the active tvOS focus scope (modal / capture overlay).
func (v *view) tvScrollEligible() bool {
	if !onTV() || v.sc == nil || v.native == 0 || v.native.Send(sel("window")) == 0 ||
		hiddenWithin(v.native) || v.props.Disabled || v.props.PointerEvents == driver.PointerEventsNone {
		return false
	}
	capture := v.surface.topmostCapture()
	inCapture := capture != nil && objc.Send[bool](v.native, sel("isDescendantOfView:"), capture.native)
	top := v.surface.topModal()
	inModal := top != nil && objc.Send[bool](v.native, sel("isDescendantOfView:"), top.native)
	return tvFocusAllowed(inCapture, capture != nil, inModal, top != nil, v.inTopmostOverlay())
}

// tvScrollFocusable reports whether scroll view v currently accepts tvOS focus
// for remote-surface scrolling.
func (v *view) tvScrollFocusable() bool {
	return onTV() && v.sc != nil && v.sc.tvFocusable &&
		v.native != 0 && v.native.Send(sel("window")) != 0 && !hiddenWithin(v.native)
}

// canTakeTVFocus reports whether v may receive focus on tvOS, either as an
// interactive focus stop or as a remote-scrollable scroll view.
func (v *view) canTakeTVFocus() bool {
	return v.focusable() || v.tvScrollFocusable()
}

// scrollCanBecomeFocused answers UIView.canBecomeFocused on NsgoScrollView.
func scrollCanBecomeFocused(self objc.ID, cmd objc.SEL) bool {
	if !onTV() {
		return apple.SendSuper[bool](self, cmd)
	}
	v := scrollView(self)
	if v == nil || v.native != self {
		return apple.SendSuper[bool](self, cmd)
	}
	return v.tvScrollFocusable()
}

// scrollDidUpdateFocus keeps surface.focused and core focus state in sync when
// a remote-scrollable NsgoScrollView gains or loses tvOS focus.
func scrollDidUpdateFocus(self objc.ID, cmd objc.SEL, context, coordinator objc.ID) {
	apple.Super(self, cmd, context, coordinator)
	if !onTV() {
		return
	}
	v := scrollView(self)
	if v == nil || v.native != self {
		return
	}
	switch self {
	case context.Send(sel("nextFocusedView")):
		v.surface.focused = v
		v.dispatch(driver.EventFocus)
	case context.Send(sel("previouslyFocusedView")):
		if v.surface.focused == v {
			v.surface.focused = nil
		}
		v.dispatch(driver.EventBlur)
	}
}

type tvScrollEval struct {
	interactive bool
	focusScroll bool
}

// updateTVScrolls recomputes remote-surface scrolling and focusability for all
// mounted scroll views on s. It returns true when the currently focused scroll
// view stopped being focusable and needs a focus update.
func (s *surface) updateTVScrolls() bool {
	if !onTV() {
		return false
	}
	hasScroll := false
	for _, v := range s.views {
		if v != nil && v.sc != nil && v.native != 0 {
			hasScroll = true
			break
		}
	}
	if !hasScroll {
		return false
	}
	seen := map[*view]tvScrollEval{}
	needsRefocus := false

	var evalScroll func(v *view) tvScrollEval
	var scan func(parent objc.ID) (interactive, focusScroll bool)

	scan = func(parent objc.ID) (interactive, focusScroll bool) {
		for _, sub := range subviews(parent) {
			if objc.Send[bool](sub, sel("isHidden")) {
				continue
			}
			if sv := scrollView(sub); sv != nil && sv.native == sub {
				res := evalScroll(sv)
				if res.interactive {
					interactive = true
				}
				if sv.sc.tvFocusable || res.focusScroll {
					focusScroll = true
				}
				continue
			}
			if child := views[sub]; child != nil {
				if child.native == sub && child.kind != driver.KindInput && child.focusable() {
					interactive = true
				}
			} else if objc.Send[bool](sub, selCanBecomeFocused) {
				interactive = true
			}
			subI, subFS := scan(sub)
			interactive = interactive || subI
			focusScroll = focusScroll || subFS
		}
		return interactive, focusScroll
	}

	evalScroll = func(v *view) tvScrollEval {
		if res, ok := seen[v]; ok {
			return res
		}
		seen[v] = tvScrollEval{}
		interactive, focusScroll := scan(v.native)
		eligible := v.tvScrollEligible()
		bounds := objc.Send[apple.CGRect](v.native, selBounds).Size
		content := objc.Send[apple.CGSize](v.native, sel("contentSize"))
		in := objc.Send[edgeInsets](v.native, sel("adjustedContentInset"))
		canPan := tvScrollCanPan(
			v.sc.config.Horizontal,
			v.sc.config.Zooms(),
			v.sc.config.Behavior.NoScroll,
			driver.Size{Width: driver.Points(content.Width), Height: driver.Points(content.Height)},
			driver.Size{Width: driver.Points(bounds.Width), Height: driver.Points(bounds.Height)},
			driver.Points(in.Top), driver.Points(in.Left), driver.Points(in.Bottom), driver.Points(in.Right),
		)
		indirect := tvScrollAllowsIndirectPan(true, eligible, canPan, interactive)
		focusable := tvScrollCanBecomeFocused(true, eligible, canPan, interactive, focusScroll)
		v.setTVIndirectPan(indirect)
		wasFocusable := v.sc.tvFocusable
		v.sc.tvFocusable = focusable
		if wasFocusable && !focusable && s.focused == v {
			needsRefocus = true
		}
		res := tvScrollEval{interactive: interactive, focusScroll: focusScroll || focusable}
		seen[v] = res
		return res
	}

	for _, v := range s.views {
		if v != nil && v.sc != nil && v.native != 0 {
			evalScroll(v)
		}
	}
	return needsRefocus
}
