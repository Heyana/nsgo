package formula

import (
	"github.com/caelyreth/untie"

	"github.com/nsgo-dev/nsgo/core"
)

// formulaPoint converts a canvas-local point back into Untie's reference box.
// View draws the reference box uniformly from its em height, while Box.Hits
// and Box.CaretStops remain in the reference coordinates.
func formulaPoint(box *untie.Box, size core.Size, p core.Point) (untie.Point, bool) {
	if box == nil || size.Height <= 0 {
		return untie.Point{}, false
	}
	bounds := box.Bounds()
	scale := float32(size.Height) / bounds.Height
	if !(scale > 0) {
		return untie.Point{}, false
	}
	return untie.Point{X: bounds.X + float32(p.X)/scale, Y: bounds.Y + float32(p.Y)/scale}, true
}

func diagnosticsAt(hit untie.Hit, found bool, diagnostics []untie.Diagnostic) []untie.Diagnostic {
	if !found {
		return nil
	}
	var out []untie.Diagnostic
	for _, diagnostic := range diagnostics {
		if diagnostic.Range.IsDetached() || !diagnostic.Range.IsValid() {
			continue
		}
		if diagnostic.Range.End <= hit.Source.Start || diagnostic.Range.Start >= hit.Source.End {
			continue
		}
		out = append(out, diagnostic)
	}
	return out
}
