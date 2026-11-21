package linux

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestPangoUnitsRoundTrip(t *testing.T) {
	for _, units := range []int32{0, 1, 2, 511, 512, 513, 1023, 1024, 1025, 20787, 102605, 1_048_576, 16_000_000} {
		pts := fromPango(units)
		if got := toPango(pts); got != units {
			t.Errorf("toPango(fromPango(%d)) = %d (pts=%v)", units, got, pts)
		}
	}
	if got := toPango(driver.Points(1e9)); got != math.MaxInt32 {
		t.Errorf("toPango(1e9) = %d, want MaxInt32", got)
	}
	if got := toPango(driver.Points(-1e9)); got != math.MinInt32 {
		t.Errorf("toPango(-1e9) = %d, want MinInt32", got)
	}
}

func TestPangoWrapWidth(t *testing.T) {
	if got := pangoWrapWidth(driver.Auto, false); got != -1 {
		t.Errorf("pangoWrapWidth(Auto, false) = %d, want -1", got)
	}
	if got := pangoWrapWidth(120, true); got != -1 {
		t.Errorf("pangoWrapWidth(120, true) = %d, want -1", got)
	}
	if got := pangoWrapWidth(0, false); got != 0 {
		t.Errorf("pangoWrapWidth(0, false) = %d, want 0", got)
	}
	if got := pangoWrapWidth(-5, false); got != 0 {
		t.Errorf("pangoWrapWidth(-5, false) = %d, want 0", got)
	}
	if got := pangoWrapWidth(100.2, false); got != 102605 {
		t.Errorf("pangoWrapWidth(100.2, false) = %d, want 102605", got)
	}
	if got := pangoWrapWidth(driver.Points(1e9), false); got != math.MaxInt32 {
		t.Errorf("pangoWrapWidth(1e9, false) = %d, want MaxInt32", got)
	}
}

func TestPangoMeasuredSizeKeepsFractions(t *testing.T) {
	// 102605 / 1024 = 100.2001953125, 20787 / 1024 = 20.2998046875
	const wUnits, hUnits int32 = 102605, 20787
	got := pangoMeasuredSize(wUnits, hUnits, driver.Auto, -1)
	if float64(got.Width) == math.Ceil(float64(got.Width)) {
		t.Fatalf("width %v rounded to whole point", got.Width)
	}
	if float64(got.Height) == math.Ceil(float64(got.Height)) {
		t.Fatalf("height %v rounded to whole point", got.Height)
	}
	if wantW := driver.Points(wUnits) / pangoScale; got.Width != wantW {
		t.Errorf("width = %v, want %v", got.Width, wantW)
	}
	if wantH := driver.Points(hUnits) / pangoScale; got.Height != wantH {
		t.Errorf("height = %v, want %v", got.Height, wantH)
	}

	// Laying out at the exact measured width or at the core's device-pixel
	// ceiling at any scale must never yield a Pango wrap width narrower than
	// the measured Pango units.
	for _, ratio := range []float64{0, 1, 1.25, 1.5, 2, 2.625, 3} {
		w := got.Width
		if ratio > 0 {
			up := max(float32(math.Ceil(float64(float32(w))*ratio-1e-3)), 0) / float32(ratio)
			if up < float32(w) {
				up += 1 / float32(ratio)
			}
			w = driver.Points(up)
		}
		if wrap := pangoWrapWidth(w, false); wrap < wUnits {
			t.Errorf("at %vx scale, rounded width %v -> %d Pango units < measured %d", ratio, w, wrap, wUnits)
		}
	}
}

func TestPangoMeasuredSizeBoundedLimit(t *testing.T) {
	// 100.3336 * 1024 = 102741.6064 -> limit = 102742 (fromPango(102742) = 100.333984375 > 100.3336).
	const bound driver.Points = 100.3336
	limit := pangoWrapWidth(bound, false)
	if limit != 102742 {
		t.Fatalf("limit = %d, want 102742", limit)
	}

	// When the layout fills limit exactly, the returned width stays <= bound
	// while still converting back to limit for Layout.
	atLimit := pangoMeasuredSize(limit, 20787, bound, limit)
	if atLimit.Width > bound {
		t.Errorf("w == limit: width %v > bound %v", atLimit.Width, bound)
	}
	if got := pangoWrapWidth(atLimit.Width, false); got != limit {
		t.Errorf("pangoWrapWidth(%v) = %d, want %d", atLimit.Width, got, limit)
	}

	// Below limit keeps its exact fractional Pango width and stays < bound.
	below := pangoMeasuredSize(limit-1, 20787, bound, limit)
	if below.Width != fromPango(limit-1) || below.Width >= bound {
		t.Errorf("w == limit-1: width = %v, want %v (< %v)", below.Width, fromPango(limit-1), bound)
	}

	// Above limit (e.g. an unbreakable overflowing word) preserves its true width.
	over := pangoMeasuredSize(limit+5000, 20787, bound, limit)
	if over.Width != fromPango(limit+5000) {
		t.Errorf("w > limit: width = %v, want %v", over.Width, fromPango(limit+5000))
	}
}

func TestPangoSpacedWidthKeepsFractions(t *testing.T) {
	spacing := driver.Points(1.3)
	before, after := letterSpace(spacing)
	if before+after != int32(spacing*pangoScale) {
		t.Fatalf("letterSpace(1.3) = (%d, %d), sum = %d", before, after, before+after)
	}
	// Fractional letter spacing stays in Pango units (not rounded up to whole points).
	const lineUnits int32 = 102400 // 100.0 pt
	need := lineUnits + before + after
	if got := pangoSpacedWidth(need, -1); got != need {
		t.Fatalf("unbounded pangoSpacedWidth = %d, want %d", got, need)
	}
	size := pangoMeasuredSize(pangoSpacedWidth(need, -1), 20480, driver.Auto, -1)
	if float64(size.Width) == math.Ceil(float64(size.Width)) {
		t.Errorf("spaced width %v rounded to whole point", size.Width)
	}
	if got := pangoWrapWidth(size.Width, false); got != need {
		t.Errorf("pangoWrapWidth(%v) = %d, want %d", size.Width, got, need)
	}

	// Bounded spacing clamps in Pango units, not whole points.
	const limit int32 = 103000
	if got := pangoSpacedWidth(need, limit); got != limit {
		t.Errorf("bounded pangoSpacedWidth = %d, want %d", got, limit)
	}
}

type pangoUnitEngine struct {
	wUnits, hUnits int32
}

func (e pangoUnitEngine) Prepare(at *driver.AttributedText) driver.PreparedText {
	return pangoUnitPrepared{PreparedText: core.ApproxTextEngine{}.Prepare(at), wUnits: e.wUnits, hUnits: e.hUnits}
}

type pangoUnitPrepared struct {
	driver.PreparedText
	wUnits, hUnits int32
}

func (p pangoUnitPrepared) Measure(width driver.Points) driver.Size {
	limit := pangoWrapWidth(width, false)
	return pangoMeasuredSize(p.wUnits, p.hUnits, width, limit)
}

// TestPangoMeasuredSizeWithCoreDevicePixels checks that fractional Pango-unit
// measurements pass through to core.Tree untouched so at 2x (and other scales)
// core rounds up to a device pixel rather than a whole point (#1603).
func TestPangoMeasuredSizeWithCoreDevicePixels(t *testing.T) {
	// 102605 / 1024 = 100.2001953125 pt, 20787 / 1024 = 20.2998046875 pt.
	const wUnits, hUnits int32 = 102605, 20787
	for _, c := range []struct {
		ratio  float32
		pixels core.Size
	}{
		{1, core.Size{Width: 101, Height: 21}},
		{1.5, core.Size{Width: 151, Height: 31}},
		{2, core.Size{Width: 201, Height: 41}},
		{3, core.Size{Width: 301, Height: 61}},
	} {
		root := core.NewRoot(core.RootOptions{})
		t.Cleanup(root.Dispose)
		tree := core.NewTree(root, pangoUnitEngine{wUnits: wUnits, hUnits: hUnits})
		tree.Resize(core.Size{Width: 400, Height: 400}, core.Geometry{})
		tree.SetEnvironment(core.Environment{PixelRatio: c.ratio})
		tree.Mount(func(cx *core.Scope) core.Node {
			return core.View{
				Style:    core.Style{AlignItems: core.AlignStart},
				Children: []core.Node{core.Text{Content: "measured"}},
			}
		})
		batch := tree.Commit()
		var textID driver.NodeID
		frames := map[driver.NodeID]core.Bounds{}
		for _, it := range batch.Items {
			switch it.Kind {
			case driver.ItemText:
				textID = it.ID
			case driver.ItemFrame:
				frames[it.ID] = it.Frame
			}
		}
		textFrame, ok := frames[textID]
		if !ok {
			t.Fatalf("at %vx scale, no frame committed for text node", c.ratio)
		}
		gotW := float64(textFrame.Size.Width) * float64(c.ratio)
		gotH := float64(textFrame.Size.Height) * float64(c.ratio)
		if math.Abs(gotW-float64(c.pixels.Width)) > 1e-3 || math.Abs(gotH-float64(c.pixels.Height)) > 1e-3 {
			t.Errorf("at %vx scale, Pango %dx%d units laid out %v (%vx%v px), want %v px",
				c.ratio, wUnits, hUnits, textFrame.Size, gotW, gotH, c.pixels)
		}
		if wrap := pangoWrapWidth(textFrame.Size.Width, false); wrap < wUnits {
			t.Errorf("at %vx scale, core width %v -> %d Pango units < measured %d", c.ratio, textFrame.Size.Width, wrap, wUnits)
		}
	}
}
