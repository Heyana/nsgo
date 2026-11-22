//go:build darwin && !ios

package macos

import (
	"strconv"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// AppKit hands a tracking area's events to its owner by the area's rect, whatever the window's hit test finds there, so a WKWebView under an NSGo overlay takes the pointer's entries, moves and cursor updates. Some WebKit versions drop them when the hit test lands elsewhere, and others (macOS 26's) do not. NSGo gives each owner of a WKWebView's tracking areas a subclass of its own class that drops them while the web view is covered, so every version behaves alike. Exits always pass, so a covered page can clear :hover (leaveWeb).
var (
	// webTrackingClasses maps a tracking area owner's class to the NSGo subclass that filters it.
	webTrackingClasses = map[objc.Class]objc.Class{}
	// webTrackingOwn holds those subclasses.
	webTrackingOwn = map[objc.Class]bool{}
	// webTrackers maps a filtered owner to the WKWebView whose areas it owns.
	webTrackers = map[objc.ID]objc.ID{}
)

// webTrackingSelectors are the events a filtered owner drops while its web view is covered.
var webTrackingSelectors = []objc.SEL{sel("mouseEntered:"), sel("mouseMoved:"), sel("cursorUpdate:")}

// guardWebTracking gives the owners of web's tracking areas their filtering subclasses. Views, the web view among them, are left alone: key-value observing gives the web view a runtime subclass of its own, which a second one would break.
func guardWebTracking(web objc.ID) {
	if web == 0 {
		return
	}
	areas := web.Send(sel("trackingAreas"))
	n := objc.Send[uint](areas, selCount)
	for i := uint(0); i < n; i++ {
		owner := areas.Send(sel("objectAtIndex:"), i).Send(sel("owner"))
		if owner == 0 || owner == web || objc.Send[bool](owner, sel("isKindOfClass:"), objc.ID(apple.Class("NSView"))) {
			continue
		}
		webTrackers[owner] = web
		class := owner.Class()
		if webTrackingOwn[class] {
			continue
		}
		sub, ok := webTrackingClasses[class]
		if !ok {
			sub = webTrackingClass(class)
			webTrackingClasses[class] = sub
		}
		if sub != 0 {
			loadSetClass()
			objectSetClass(owner, sub)
		}
	}
}

// webTrackingClass registers the subclass of class that drops covered events, 0 when class takes none of them.
func webTrackingClass(class objc.Class) objc.Class {
	var methods []objc.MethodDef
	for _, cmd := range webTrackingSelectors {
		if objc.Send[bool](objc.ID(class), sel("instancesRespondToSelector:"), cmd) {
			methods = append(methods, objc.MethodDef{Cmd: cmd, Fn: webTrackingEvent})
		}
	}
	if len(methods) == 0 {
		return 0
	}
	sub := register("NSGoWebTracking"+strconv.Itoa(len(webTrackingClasses)), class, nil, methods)
	webTrackingOwn[sub] = true
	return sub
}

// webTrackingEvent passes an owner's event on unless its web view is covered where the event happened.
func webTrackingEvent(self objc.ID, cmd objc.SEL, event objc.ID) {
	if webCoveredAt(webTrackers[self], event) {
		return
	}
	apple.Super(self, cmd, event)
}

// webCoveredAt reports whether the surface's hit test at event's window point lands on a view other than web and its descendants.
func webCoveredAt(web, event objc.ID) bool {
	v := views[web]
	if web == 0 || event == 0 || v == nil || v.surface == nil || v.surface.window == 0 || web.Send(selWindow) != v.surface.window {
		return false
	}
	hit := v.surface.hitAt(objc.Send[apple.CGPoint](event, selLocationInWindow))
	return hit != 0 && hit != web && !objc.Send[bool](hit, sel("isDescendantOf:"), web)
}

// forgetWebTracking drops the owners of web's tracking areas from webTrackers. They keep their subclass, which passes everything once its owner is unknown.
func forgetWebTracking(web objc.ID) {
	for owner, w := range webTrackers {
		if w == web {
			delete(webTrackers, owner)
		}
	}
}
