//go:build !nsgodev && !nsgotest

package gpu

import "github.com/nsgo-dev/nsgo/internal/driver"

// DebugLabelsEnabled reports whether GPU debug labels, groups, and markers are
// recorded in this build (true under nsgodev or nsgotest, false in release builds).
const DebugLabelsEnabled = false

// Release builds inline these empty helpers away so debug labels, groups, and
// markers carry zero runtime or allocation cost.
func debugLabel(string) string             { return "" }
func debugLabelJoin(string, string) string { return "" }

func (*Encoder) pushDebugGroup(string)    {}
func (*Encoder) popDebugGroup()           {}
func (*Encoder) insertDebugMarker(string) {}

func (*RenderPass) pushDebugGroup(string)    {}
func (*RenderPass) popDebugGroup()           {}
func (*RenderPass) insertDebugMarker(string) {}

func appendFrameDebugOps(*driver.GPUFrame, []driver.GPUDebugOp, int) {}
