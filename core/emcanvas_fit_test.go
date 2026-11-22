package core

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Fitting text shrinks font-relative attachments with its glyphs and restores them when space returns.
func TestFittedTextSizesItsEmAttachment(t *testing.T) {
	var width *Signal[Points]
	h := newHost(t, Size{Width: 500, Height: 200}, func(cx *Scope) Node {
		width = NewSignal[Points](cx, 150)
		return Text{TestID: "text", AdjustsFontSizeToFit: true, MinimumFontScale: 0.25, MaxLines: 1,
			StyleFunc: func() Style { return Style{Width: Pt(width.Get()), Height: Pt(80), FontSize: 32} },
			Spans:     []Span{{Content: "before "}, {Content: "$x$", Attachment: Canvas{TestID: "formula", Em: EmBox{Width: 2, Ascent: 0.75, Descent: 0.25}}}, {Content: " after"}},
		}
	})
	check := func(shrunk bool) {
		t.Helper()
		batch := h.commit()
		var shown *driver.TextContent
		for _, item := range batch.Items {
			if item.Kind == driver.ItemText && item.ID == h.find("text").id {
				shown = item.Text
			}
		}
		if shown == nil {
			t.Fatal("the text was not sent")
		}
		fontSize := shown.Style.Font.Size
		if shrunk && fontSize >= 32 {
			t.Fatalf("font size %v did not shrink", fontSize)
		}
		if !shrunk && fontSize != 32 {
			t.Fatalf("font size %v did not restore", fontSize)
		}
		canvas := h.find("formula")
		if math.Abs(float64(canvas.frame.Size.Height-fontSize)) > 1 || math.Abs(float64(canvas.frame.Size.Width-2*fontSize)) > 1 {
			t.Errorf("fitted font %v, canvas %v: em geometry must share its fitted font size", fontSize, canvas.frame.Size)
		}
		if got := canvas.canvas.size.GetUntracked(); got != canvas.frame.Size {
			t.Errorf("drawing size %v differs from frame %v", got, canvas.frame.Size)
		}
	}
	check(true)
	width.Set(500)
	check(false)
	width.Set(150)
	check(true)
}

// Fitting changes font-relative ink while preserving sizes and padding declared in points.
func TestFittedTextPreservesPointSizedAttachments(t *testing.T) {
	h := newHost(t, Size{Width: 500, Height: 200}, func(cx *Scope) Node {
		return Text{TestID: "text", AdjustsFontSizeToFit: true, MinimumFontScale: 0.25, MaxLines: 1,
			Style: Style{Width: Pt(160), Height: Pt(80), FontSize: 32},
			Spans: []Span{{Content: "before "}, {Attachment: View{TestID: "wrapper", Style: Style{Direction: Row, Padding: All(4)}, Children: []Node{
				View{TestID: "fixed", Style: Style{Width: Pt(10), Height: Pt(12)}},
				Canvas{TestID: "em", Em: EmBox{Width: 2, Ascent: 0.75, Descent: 0.25}},
			}}}, {Attachment: Canvas{TestID: "sized", Em: EmBox{Width: 2, Ascent: 1}, Style: Style{Width: Pt(24), Height: Pt(12)}}}, {Content: " after"}},
		}
	})
	batch := h.commit()
	var font Points
	for _, item := range batch.Items {
		if item.Kind == driver.ItemText && item.ID == h.find("text").id {
			font = item.Text.Style.Font.Size
		}
	}
	if font <= 0 || font >= 32 {
		t.Fatalf("fitted font %v", font)
	}
	if got := h.find("fixed").frame.Size; got != (Size{Width: 10, Height: 12}) {
		t.Errorf("fixed view shrank to %v", got)
	}
	if got := h.find("sized").frame.Size; got != (Size{Width: 24, Height: 12}) {
		t.Errorf("explicit canvas size shrank to %v", got)
	}
	if got := h.find("em").frame.Size; math.Abs(float64(got.Height-font)) > 1 {
		t.Errorf("em size %v at fitted font %v", got, font)
	}
	if got := h.find("wrapper").frame.Size.Width; math.Abs(float64(got-(2*font+18))) > 1 {
		t.Errorf("wrapper width %v at font %v: point padding and sibling must remain", got, font)
	}
}

func TestFittedEmAttachmentHonorsTextScalingLimits(t *testing.T) {
	for _, test := range []struct {
		name    string
		noScale bool
		cap     float32
	}{
		{name: "scaled"}, {name: "fixed", noScale: true}, {name: "capped", cap: 1.5},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newHost(t, Size{Width: 400, Height: 100}, func(cx *Scope) Node {
				return Text{TestID: "text", AdjustsFontSizeToFit: true, MinimumFontScale: 0.5, MaxLines: 1,
					NoFontScaling: test.noScale, MaxFontSizeMultiplier: test.cap,
					Style: Style{Width: Pt(20), Height: Pt(80), FontSize: 20},
					Spans: []Span{{Content: "before "}, {Attachment: Canvas{TestID: "em", Em: EmBox{Width: 2, Ascent: 1}, Style: Style{FontSize: 10}}}},
				}
			})
			h.tree.SetEnvironment(Environment{FontScale: 2})
			batch := h.commit()
			var font Points
			for _, item := range batch.Items {
				if item.Kind == driver.ItemText && item.ID == h.find("text").id {
					font = item.Text.Style.Font.Size
				}
			}
			want := Points(20) * Points(fontMultiplier(2, test.noScale, test.cap)) * 0.5
			if font != want {
				t.Errorf("minimum font %v, want %v", font, want)
			}
			if got := h.find("em").frame.Size.Height; math.Abs(float64(got-font/2)) > 1 {
				t.Errorf("em height %v at font %v", got, font)
			}
		})
	}
}

// Equal natural sizes must not keep an old fitted child position after its style changes.
func TestFittedAttachmentRestylesWithoutChangingItsNaturalSize(t *testing.T) {
	var left *Signal[bool]
	h := newHost(t, Size{Width: 400, Height: 200}, func(cx *Scope) Node {
		left = NewSignal(cx, true)
		return Text{TestID: "text", AdjustsFontSizeToFit: true, MaxLines: 1, Style: Style{FontSize: 32, Width: Pt(140), Height: Pt(80)},
			Spans: []Span{{Content: "before "}, {Attachment: View{TestID: "wrapper", StyleFunc: func() Style {
				if left.Get() {
					return Style{Padding: Edges{Left: 8}}
				}
				return Style{Padding: Edges{Right: 8}}
			}, Children: []Node{Canvas{TestID: "em", Em: EmBox{Width: 2, Ascent: 1}, Style: Style{AlignSelf: AlignStart}}}}}, {Content: " after"}},
		}
	})
	h.commit()
	width := h.find("wrapper").parent.attach.size.Width
	if got := h.find("em").frame.Origin.X; got != 8 {
		t.Fatalf("initial x %v", got)
	}
	left.Set(false)
	h.commit()
	if got := h.find("wrapper").parent.attach.size.Width; got != width {
		t.Fatalf("natural width changed from %v to %v", width, got)
	}
	if got := h.find("em").frame.Origin.X; got != 0 {
		t.Errorf("restyled em x %v, want 0", got)
	}
}

func TestFittedTextPlacesReplacementAttachment(t *testing.T) {
	var changed *Signal[bool]
	h := newHost(t, Size{Width: 400, Height: 200}, func(cx *Scope) Node {
		changed = NewSignal(cx, false)
		return Text{TestID: "text", AdjustsFontSizeToFit: true, MaxLines: 1, Style: Style{FontSize: 32, Width: Pt(150), Height: Pt(80)},
			SpansFunc: func() []Span {
				id := "old"
				if changed.Get() {
					id = "new"
				}
				return []Span{{Content: "before "}, {Content: "$x$", Attachment: Canvas{TestID: id, Em: EmBox{Width: 2, Ascent: 1}}}, {Content: " after"}}
			},
		}
	})
	h.commit()
	before := h.find("old").parent.frame
	changed.Set(true)
	h.commit()
	if got := h.find("new").parent.frame; got != before {
		t.Errorf("replacement frame %v, want %v", got, before)
	}
}

// A width-aware em canvas is measured at the fitted font size too.
func TestFittedTextSizesItsWidthAwareEmAttachment(t *testing.T) {
	var fonts []Points
	h := newHost(t, Size{Width: 500, Height: 200}, func(cx *Scope) Node {
		return Text{TestID: "text", AdjustsFontSizeToFit: true, MinimumFontScale: 0.25, MaxLines: 1,
			Style: Style{Width: Pt(150), Height: Pt(80), FontSize: 32},
			Spans: []Span{{Content: "before "}, {Content: "$x$", Attachment: Canvas{TestID: "formula", MeasureEm: func(_, fs Points) EmBox {
				fonts = append(fonts, fs)
				return EmBox{Width: 2, Ascent: 0.75, Descent: 0.25}
			}}}, {Content: " after"}},
		}
	})
	batch := h.commit()
	var font Points
	for _, item := range batch.Items {
		if item.Kind == driver.ItemText && item.ID == h.find("text").id {
			font = item.Text.Style.Font.Size
		}
	}
	if font <= 0 || font >= 32 {
		t.Fatalf("fitted font %v", font)
	}
	if got := h.find("formula").frame.Size; math.Abs(float64(got.Height-font)) > 1 || math.Abs(float64(got.Width-2*font)) > 1 {
		t.Errorf("width-aware canvas %v at fitted font %v (measured at %v)", got, font, fonts)
	}
}
