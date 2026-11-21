package mobile

import "github.com/nsgo-dev/nsgo/core"

// SystemGestureEdge is a mask of physical screen edges.
type SystemGestureEdge = core.SystemGestureEdge

const (
	GestureEdgeTop    = core.GestureEdgeTop
	GestureEdgeLeft   = core.GestureEdgeLeft
	GestureEdgeBottom = core.GestureEdgeBottom
	GestureEdgeRight  = core.GestureEdgeRight
)

// SystemGestureOptions names edges on iOS and exclusion rectangles on Android.
type SystemGestureOptions = core.SystemGestureOptions

// DeferSystemGestures asks the OS to give the current surface's touches priority
// over navigation gestures. iOS takes IOSEdges. Android 10 and later take
// AndroidRects in surface points, subject to the system's exclusion limits.
// Other targets ignore the request. The request lasts until release runs or the
// current scope is cleaned up. Simultaneous requests on one surface combine.
// Calling release more than once is safe.
func DeferSystemGestures(cx *core.Scope, opts SystemGestureOptions) (release func()) {
	return cx.DeferSystemGestures(opts)
}
