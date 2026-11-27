package windows

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestMergedDamageRects(t *testing.T) {
	t.Run("drops empty and merges touching", func(t *testing.T) {
		in := []rect{
			{0, 0, 0, 10},
			{10, 10, 50, 30},
			{50, 10, 90, 30},
			{10, 30, 90, 50},
			{200, 200, 200, 200},
		}
		got := merged(append([]rect(nil), in...))
		if len(got) != 1 || got[0] != (rect{10, 10, 90, 50}) {
			t.Fatalf("merged = %v, want [{10 10 90 50}]", got)
		}
	})

	t.Run("caps at maxDamage while covering every input rect", func(t *testing.T) {
		var in []rect
		// 60 disjoint table cells across 12 rows x 5 columns.
		for row := range 12 {
			for col := range 5 {
				x := int32(col * 160)
				y := int32(row * 44)
				in = append(in, rect{x, y, x + 120, y + 24})
			}
		}
		got := merged(append([]rect(nil), in...))
		if len(got) > maxDamage {
			t.Fatalf("len(merged) = %d, want <= %d", len(got), maxDamage)
		}
		for _, want := range in {
			covered := false
			for _, g := range got {
				if g.Left <= want.Left && g.Top <= want.Top && g.Right >= want.Right && g.Bottom >= want.Bottom {
					covered = true
					break
				}
			}
			if !covered {
				t.Fatalf("input rect %v not covered by %v", want, got)
			}
		}
		// No two final rectangles should still touch.
		for i := range got {
			for j := i + 1; j < len(got); j++ {
				if got[i].touches(got[j]) {
					t.Fatalf("merged rects %v and %v still touch", got[i], got[j])
				}
			}
		}
	})
}

func TestClassifyInnerClip(t *testing.T) {
	cases := []struct {
		name       string
		style      driver.ViewStyle
		size       driver.Size
		wantKind   innerClipKind
		wantRadius float32
	}{
		{
			name:     "square bottom border row",
			style:    driver.ViewStyle{Borders: driver.Edges{Bottom: 1}, BorderColors: driver.EdgeColors{Bottom: 0x333333ff}},
			size:     driver.Size{Width: 800, Height: 36},
			wantKind: innerClipRect,
		},
		{
			name:       "uniform circular rounded container",
			style:      driver.ViewStyle{CornerRadius: 8, BorderWidth: 1, BorderColor: 0x333333ff},
			size:       driver.Size{Width: 800, Height: 600},
			wantKind:   innerClipUniformRounded,
			wantRadius: 7,
		},
		{
			name:     "border wider than radius collapses to rect",
			style:    driver.ViewStyle{CornerRadius: 2, BorderWidth: 4, BorderColor: 0x333333ff},
			size:     driver.Size{Width: 100, Height: 40},
			wantKind: innerClipRect,
		},
		{
			name:     "asymmetric borders on rounded corners stay path",
			style:    driver.ViewStyle{CornerRadius: 8, Borders: driver.Edges{Top: 1, Left: 2}},
			size:     driver.Size{Width: 200, Height: 100},
			wantKind: innerClipPath,
		},
		{
			name:     "continuous rounded corners stay path",
			style:    driver.ViewStyle{CornerRadius: 8, ContinuousCorners: true, BorderWidth: 1},
			size:     driver.Size{Width: 200, Height: 100},
			wantKind: innerClipPath,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, _, inner := tc.style.ClipBox(tc.size)
			r, kind := classifyInnerClip(inner, tc.style.ContinuousCorners && tc.style.Rounded())
			if kind != tc.wantKind || r != tc.wantRadius {
				t.Fatalf("classifyInnerClip = (%v, %v), want (%v, %v)", r, kind, tc.wantRadius, tc.wantKind)
			}
		})
	}
}

func BenchmarkMergedDamageRects(b *testing.B) {
	makeCells := func(rows, cols int) []rect {
		out := make([]rect, 0, rows*cols)
		for row := range rows {
			for col := range cols {
				x := int32(col * 140)
				y := int32(row * 34)
				out = append(out, rect{x, y, x + 110, y + 22})
			}
		}
		return out
	}
	for _, tc := range []struct {
		name       string
		rows, cols int
	}{
		{"DataTableTick40Cells", 10, 4},
		{"DataTableScroll100Cells", 20, 5},
	} {
		b.Run(tc.name, func(b *testing.B) {
			src := makeCells(tc.rows, tc.cols)
			scratch := make([]rect, len(src))
			b.ReportAllocs()
			for b.Loop() {
				copy(scratch, src)
				_ = merged(scratch)
			}
		})
	}
}

func BenchmarkClassifyInnerClip(b *testing.B) {
	rowStyle := driver.ViewStyle{Borders: driver.Edges{Bottom: 1}, BorderColors: driver.EdgeColors{Bottom: 0x333333ff}}
	size := driver.Size{Width: 960, Height: 34}
	b.ReportAllocs()
	for b.Loop() {
		_, _, _, _, inner := rowStyle.ClipBox(size)
		_, _ = classifyInnerClip(inner, false)
	}
}
