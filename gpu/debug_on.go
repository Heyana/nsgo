//go:build nsgodev || nsgotest

package gpu

import "github.com/nsgo-dev/nsgo/internal/driver"

// DebugLabelsEnabled reports whether GPU debug labels, groups, and markers are
// recorded in this build (true under nsgodev or nsgotest, false in release builds).
const DebugLabelsEnabled = true

func debugLabel(s string) string { return s }

func debugLabelJoin(base, suffix string) string {
	if base == "" {
		return ""
	}
	return base + suffix
}

func (e *Encoder) pushDebugGroup(label string) {
	if e == nil || e.finished {
		return
	}
	e.debugOps = append(e.debugOps, driver.GPUDebugOp{
		Kind:      driver.GPUDebugPushGroup,
		Label:     label,
		PassIndex: len(e.passes),
	})
}

func (e *Encoder) popDebugGroup() {
	if e == nil || e.finished {
		return
	}
	e.debugOps = append(e.debugOps, driver.GPUDebugOp{
		Kind:      driver.GPUDebugPopGroup,
		PassIndex: len(e.passes),
	})
}

func (e *Encoder) insertDebugMarker(label string) {
	if e == nil || e.finished {
		return
	}
	e.debugOps = append(e.debugOps, driver.GPUDebugOp{
		Kind:      driver.GPUDebugInsertMarker,
		Label:     label,
		PassIndex: len(e.passes),
	})
}

func (p *RenderPass) pushDebugGroup(label string) {
	if p == nil || p.ended {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind:  driver.GPUCmdPushDebugGroup,
		Label: label,
	})
}

func (p *RenderPass) popDebugGroup() {
	if p == nil || p.ended {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind: driver.GPUCmdPopDebugGroup,
	})
}

func (p *RenderPass) insertDebugMarker(label string) {
	if p == nil || p.ended {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind:  driver.GPUCmdInsertDebugMarker,
		Label: label,
	})
}

func appendFrameDebugOps(frame *driver.GPUFrame, ops []driver.GPUDebugOp, passOffset int) {
	if len(ops) == 0 {
		return
	}
	for _, op := range ops {
		op.PassIndex += passOffset
		frame.DebugOps = append(frame.DebugOps, op)
	}
}
