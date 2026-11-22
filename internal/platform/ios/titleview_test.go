package ios

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestTitleWrapIntrinsicAndFitSize(t *testing.T) {
	frame := driver.Size{Width: 160, Height: 30}

	// Filling title views ask for the bar's width, or layoutFittingExpandedSize before the bar has laid out.
	if w, h := titleWrapIntrinsicSize(frame, 390, true); w != 390 || h != 30 {
		t.Errorf("filling with barWidth=390: got (%v, %v), want (390, 30)", w, h)
	}
	if w, h := titleWrapIntrinsicSize(frame, 0, true); w != titleWrapExpandedWidth || h != 30 {
		t.Errorf("filling with barWidth=0: got (%v, %v), want (%v, 30)", w, h, titleWrapExpandedWidth)
	}
	if w, h := titleWrapFitSize(frame, 240, 390, true); w != 240 || h != 30 {
		t.Errorf("filling fitWidth=240: got (%v, %v), want (240, 30)", w, h)
	}

	// Intrinsic-sized title views keep their core frame size.
	if w, h := titleWrapIntrinsicSize(frame, 390, false); w != 160 || h != 30 {
		t.Errorf("intrinsic: got (%v, %v), want (160, 30)", w, h)
	}
	if w, h := titleWrapFitSize(frame, 240, 390, false); w != 160 || h != 30 {
		t.Errorf("intrinsic fitWidth=240: got (%v, %v), want (160, 30)", w, h)
	}
}

func TestTitleWrapPriorities(t *testing.T) {
	hugFill, compFill := titleWrapPriorities(true)
	hugNat, compNat := titleWrapPriorities(false)
	if hugFill >= 250 || compFill >= 250 {
		t.Errorf("filling priorities = (%v, %v), want below UIKit default (250) so bar buttons keep their widths", hugFill, compFill)
	}
	if hugNat <= hugFill {
		t.Errorf("intrinsic hugging = %v should exceed filling hugging = %v", hugNat, hugFill)
	}
	if compNat >= 750 {
		t.Errorf("intrinsic compression = %v, want below 750 so overly wide titles yield to bar buttons", compNat)
	}
}

func TestReportedTitleWidth(t *testing.T) {
	// Filling title views report their pixel-snapped laid-out bounds width.
	if w, ok := reportedTitleWidth(258.3333, 358, 3, true); !ok || w != driver.Points(258.3333333333333) {
		t.Errorf("filling @3x: got (%v, %v)", w, ok)
	}
	if w, ok := reportedTitleWidth(0, 358, 2, true); ok || w != 0 {
		t.Errorf("zero bounds: got (%v, %v), want (0, false)", w, ok)
	}

	// Content-sized title views do NOT report their natural width as bar capacity...
	if w, ok := reportedTitleWidth(110, 110, 2, false); ok || w != 0 {
		t.Errorf("uncompressed intrinsic: got (%v, %v), want (0, false)", w, ok)
	}
	// ...but DO report when UIKit compressed them narrower than their core frame.
	if w, ok := reportedTitleWidth(210, 320, 2, false); !ok || w != 210 {
		t.Errorf("compressed intrinsic: got (%v, %v), want (210, true)", w, ok)
	}
}

func TestTitleWrapNeedsRelayout(t *testing.T) {
	// Filling title views do not re-trigger UIKit layout when only width converges.
	if titleWrapNeedsRelayout(driver.Size{Width: 358, Height: 32}, driver.Size{Width: 258, Height: 32}, true) {
		t.Error("filling width-only change requested relayout; would cause redundant UIKit pass")
	}
	if !titleWrapNeedsRelayout(driver.Size{Width: 258, Height: 32}, driver.Size{Width: 258, Height: 44}, true) {
		t.Error("filling height change did not request relayout")
	}
	// Content-sized title views relayout whenever either dimension changes.
	if !titleWrapNeedsRelayout(driver.Size{Width: 110, Height: 20}, driver.Size{Width: 140, Height: 20}, false) {
		t.Error("intrinsic width change did not request relayout")
	}
	if titleWrapNeedsRelayout(driver.Size{Width: 110, Height: 20}, driver.Size{Width: 110, Height: 20}, false) {
		t.Error("unchanged intrinsic size requested relayout")
	}
}
