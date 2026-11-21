package driver

// SystemGestureEdge is a mask of physical screen edges, in UIKit's UIRectEdge order.
type SystemGestureEdge uint8

const (
	GestureEdgeTop SystemGestureEdge = 1 << iota
	GestureEdgeLeft
	GestureEdgeBottom
	GestureEdgeRight
)

// SystemGestureOptions is what ItemSystemGestures sets on a surface.
type SystemGestureOptions struct {
	// IOSEdges asks UIKit to defer system gestures at these physical edges.
	IOSEdges SystemGestureEdge
	// AndroidRects gives Android exclusion rectangles in surface points.
	AndroidRects []Bounds
}
