package harmonyos

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// fakeBridge simulates ArkUI's MeasureText.measureTextSize behavior (wrapped
// height with no maxLines limit, width = min(constraintWidth, unwrapped width)
// when bounded, and configurable line height + inter-line gap).
type fakeBridge struct {
	charW   driver.Points
	lineH   driver.Points
	lineGap driver.Points
	calls   int
}

func (f *fakeBridge) measure(text string, _ driver.TextStyle, maxWidth driver.Points) driver.Size {
	f.calls++
	paras := splitParagraphs(text)
	bounded := !maxWidth.IsAuto() && maxWidth > 0
	var unwrappedW driver.Points
	totalLines := 0
	for _, p := range paras {
		pw := driver.Points(utf8.RuneCountInString(p)) * f.charW
		if pw > unwrappedW {
			unwrappedW = pw
		}
		if !bounded || pw <= maxWidth {
			totalLines++
			continue
		}
		// Greedy word wrap matching ArkUI wrapping.
		words := strings.Fields(p)
		if len(words) == 0 {
			totalLines++
			continue
		}
		cur := driver.Points(0)
		for _, w := range words {
			ww := driver.Points(utf8.RuneCountInString(w)) * f.charW
			if cur > 0 && cur+f.charW+ww > maxWidth {
				totalLines++
				cur = 0
			}
			if cur == 0 && ww > maxWidth {
				perLine := max(int(maxWidth/f.charW), 1)
				runes := utf8.RuneCountInString(w)
				full := (runes + perLine - 1) / perLine
				totalLines += full - 1
				rem := runes - (full-1)*perLine
				cur = driver.Points(rem) * f.charW
				continue
			}
			if cur > 0 {
				cur += f.charW
			}
			cur += ww
		}
		totalLines++
	}
	h := f.lineH
	if totalLines > 1 {
		h += driver.Points(totalLines-1) * (f.lineH + f.lineGap)
	}
	w := unwrappedW
	if bounded && w > maxWidth {
		w = maxWidth
	}
	return driver.Size{Width: w, Height: h}
}

func TestMeasureLimitedSoftWrapClampsHeightAndKeepsAvailableWidth(t *testing.T) {
	b := &fakeBridge{charW: 6, lineH: 18, lineGap: 2}
	st := driver.TextStyle{Font: driver.Font{Size: 13}}

	// "Earth Science Weekly" (20 chars = 120 pts) wraps at 72 pts to 3 lines.
	got := measureLimited("Earth Science Weekly", st, 1, driver.EllipsizeTail, 72, b.measure)
	if got.Width != 72 || got.Height != 18 {
		t.Errorf("1-line wrapped = %+v, want {Width:72 Height:18}", got)
	}

	// Two lines with inter-line gap: 18 + (18 + 2) = 38.
	got2 := measureLimited("Short line\nA Supercalifragilisticexpialidocious", st, 2, driver.EllipsizeTail, 72, b.measure)
	if got2.Width != 72 || got2.Height != 38 {
		t.Errorf("2-line wrapped = %+v, want {Width:72 Height:38}", got2)
	}
}

func TestMeasureLimitedHardBreakDropsHiddenParagraphsAndKeepsIntrinsicWidth(t *testing.T) {
	b := &fakeBridge{charW: 6, lineH: 18, lineGap: 2}
	st := driver.TextStyle{Font: driver.Font{Size: 13}}

	// Hidden second paragraph is much wider than "Short" (5 chars = 30 pts; "Short…" = 36 pts).
	text := "Short\nA very very long second line that must not widen line one"
	bounded := measureLimited(text, st, 1, driver.EllipsizeTail, 200, b.measure)
	intrinsic := measureLimited(text, st, 1, driver.EllipsizeTail, driver.Auto, b.measure)
	if bounded != intrinsic {
		t.Fatalf("bounded %+v != intrinsic %+v", bounded, intrinsic)
	}
	if bounded.Width != 36 || bounded.Height != 18 {
		t.Errorf("hard break = %+v, want {Width:36 Height:18}", bounded)
	}

	// Multi-line hard break drops third paragraph and adds ellipsis to second.
	two := measureLimited("P1\nSecond\nVery long third paragraph", st, 2, driver.EllipsizeTail, 200, b.measure)
	// "Second…" is 7 chars = 42 pts; 2 lines = 18 + 20 = 38.
	if two.Width != 42 || two.Height != 38 {
		t.Errorf("2-line hard break = %+v, want {Width:42 Height:38}", two)
	}

	// CRLF line endings behave the same way.
	crlf := measureLimited("Short\r\nLong second line", st, 1, driver.EllipsizeTail, 200, b.measure)
	if crlf != bounded {
		t.Errorf("CRLF = %+v, want %+v", crlf, bounded)
	}
}

func TestMeasureLimitedHardBreakOverflowingWithEllipsis(t *testing.T) {
	b := &fakeBridge{charW: 6, lineH: 18, lineGap: 2}
	st := driver.TextStyle{Font: driver.Font{Size: 13}}

	// "Short" = 30 pts fits in 32, but "Short…" = 36 pts wraps past 1 line at 32.
	got := measureLimited("Short\nline", st, 1, driver.EllipsizeTail, 32, b.measure)
	if got.Width != 32 || got.Height != 18 {
		t.Errorf("tight hard break = %+v, want {Width:32 Height:18}", got)
	}
}

func TestMeasureLimitedClipAndUnlimited(t *testing.T) {
	b := &fakeBridge{charW: 6, lineH: 18, lineGap: 2}
	st := driver.TextStyle{Font: driver.Font{Size: 13}}

	// Clip on hard break does not add ellipsis width ("Short" = 30 pts).
	clippedHard := measureLimited("Short\nline", st, 1, driver.EllipsizeClip, 200, b.measure)
	if clippedHard.Width != 30 || clippedHard.Height != 18 {
		t.Errorf("clipped hard break = %+v, want {Width:30 Height:18}", clippedHard)
	}

	// Clip on soft wrap still clamps height without expanding beyond measured width.
	narrowMeasurer := func(text string, st driver.TextStyle, maxWidth driver.Points) driver.Size {
		s := b.measure(text, st, maxWidth)
		if text == "Earth Science Weekly" && !maxWidth.IsAuto() {
			s.Width = 54 // narrower than offered 72
		}
		return s
	}
	clippedSoft := measureLimited("Earth Science Weekly", st, 1, driver.EllipsizeClip, 72, narrowMeasurer)
	if clippedSoft.Width != 54 || clippedSoft.Height != 18 {
		t.Errorf("clipped soft wrap = %+v, want {Width:54 Height:18}", clippedSoft)
	}

	// Unlimited (MaxLines == 0) returns the full unwrapped/wrapped height.
	unlimited := measureLimited("One\nTwo\nThree", st, 0, driver.EllipsizeTail, 200, b.measure)
	if unlimited.Height != 58 { // 18 + 2*20
		t.Errorf("unlimited height = %v, want 58", unlimited.Height)
	}

	// Text fitting within MaxLines stays unchanged.
	fits := measureLimited("One\nTwo", st, 3, driver.EllipsizeTail, 200, b.measure)
	if fits.Width != 18 || fits.Height != 38 {
		t.Errorf("fitting text = %+v, want {Width:18 Height:38}", fits)
	}
}
