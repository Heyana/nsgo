//go:build windows

package windows

import (
	"math"
	"testing"

	"github.com/oneo-me/nsgo/core"
)

// coverOf returns how much of the target's pixel (x, y) s covers, with the
// painter drawing straight onto a target of its own.
func coverOf(s shape, x, y int32) uint8 {
	p := &painter{m: identity}
	return p.sampler().cover(s, x, y)
}

func TestCoverageBlendsACirclesEdge(t *testing.T) {
	// A circle of radius 20 at (30, 30), whose arc crosses the pixel at
	// (15, 15) along the diagonal.
	s := shape{r: rect{10, 10, 50, 50}, radius: 20}
	if got := coverOf(s, 10, 10); got != 0 {
		t.Errorf("the pixel past the arc covers %d, want 0", got)
	}
	if got := coverOf(s, 30, 30); got != 255 {
		t.Errorf("the centre covers %d, want 255", got)
	}
	if got := coverOf(s, 15, 15); got == 0 || got == 255 {
		t.Errorf("a pixel the arc crosses covers %d, want a blend", got)
	}
	// The measured quarter circle has the area of the real one, give or
	// take the 16 samples a pixel is measured at.
	var sum int
	for y := int32(10); y < 30; y++ {
		for x := int32(10); x < 30; x++ {
			sum += int(coverOf(s, x, y))
		}
	}
	area := math.Pi * 20 * 20 / 4
	if got := float64(sum) / 255; math.Abs(got-area) > area/50 {
		t.Errorf("quarter circle area = %.1f, want %.1f", got, area)
	}
}

func TestCoverageLeavesARingsHole(t *testing.T) {
	// A 6 pixel border around a 40 by 40 box rounded by 10: the hole's
	// corners round by 10 - 6 = 4 around the same centres.
	s := shape{r: rect{0, 0, 40, 40}, radius: 10, hole: rect{6, 6, 34, 34}, holeRadius: 4}
	if got := coverOf(s, 20, 20); got != 0 {
		t.Errorf("the hole covers %d, want 0", got)
	}
	if got := coverOf(s, 2, 20); got != 255 {
		t.Errorf("a side of the ring covers %d, want 255", got)
	}
	if got := coverOf(s, 1, 1); got != 0 {
		t.Errorf("the pixel past a corner covers %d, want 0", got)
	}
	if got := coverOf(s, 2, 3); got == 0 || got == 255 {
		t.Errorf("the outer corner arc covers %d, want a blend", got)
	}
	if got := coverOf(s, 7, 7); got == 0 || got == 255 {
		t.Errorf("the inner corner arc covers %d, want a blend", got)
	}
	// A corner belongs to the two sides meeting there, each holding its
	// half of the ring.
	for _, c := range []struct {
		x, y int32
		side uint8
	}{{8, 2, 1}, {2, 8, 4}} {
		for _, side := range []uint8{1, 4} {
			s.side = side
			got := coverOf(s, c.x, c.y)
			if want := side == c.side; (got > 0) != want {
				t.Errorf("pixel (%d, %d) on side %d covers %d", c.x, c.y, side, got)
			}
		}
	}
}

func TestWedgeSplitsACornerBetweenItsSides(t *testing.T) {
	s := shape{r: rect{0, 0, 100, 100}, radius: 20}
	for _, c := range []struct {
		x, y float64
		side uint8
	}{
		{90, 3, 1}, {3, 90, 4}, {97, 90, 2}, {90, 97, 3}, // top, left, right, bottom
	} {
		for side := uint8(1); side <= 4; side++ {
			s.side = side
			if got := s.wedge(c.x, c.y); got != (side == c.side) {
				t.Errorf("(%v, %v) on side %d: wedge = %v", c.x, c.y, side, got)
			}
		}
	}
}

func TestPartsTileTheRectangle(t *testing.T) {
	for _, r := range []rect{{0, 0, 40, 40}, {10, 20, 130, 100}, {0, 0, 33, 17}, {5, 5, 6, 6}} {
		for _, radius := range []float64{0, 0.5, 3, 12, 40} {
			corners, middle := parts(r, radius)
			covered := map[point]int{}
			for _, part := range append(corners[:], middle[:]...) {
				for y := part.Top; y < part.Bottom; y++ {
					for x := part.Left; x < part.Right; x++ {
						covered[point{x, y}]++
					}
				}
			}
			for y := r.Top; y < r.Bottom; y++ {
				for x := r.Left; x < r.Right; x++ {
					if got := covered[point{x, y}]; got != 1 {
						t.Fatalf("%+v at radius %v: pixel (%d, %d) is in %d parts", r, radius, x, y, got)
					}
				}
			}
			if got, want := len(covered), int((r.Right-r.Left)*(r.Bottom-r.Top)); got != want {
				t.Fatalf("%+v at radius %v: the parts cover %d pixels, want %d", r, radius, got, want)
			}
		}
	}
}

func TestSubLeavesTheRestOfARectangle(t *testing.T) {
	for _, a := range []rect{{0, 0, 40, 40}, {10, 10, 30, 60}, {5, 5, 6, 6}} {
		for _, b := range []rect{{0, 0, 0, 0}, {4, 4, 20, 20}, {-10, 10, 20, 50}, {25, 25, 60, 60}, {0, 0, 40, 40}} {
			covered := map[point]int{}
			for _, part := range sub(a, b) {
				if part.intersect(b).empty() == false && !b.empty() {
					t.Fatalf("%+v less %+v: part %+v overlaps it", a, b, part)
				}
				for y := part.Top; y < part.Bottom; y++ {
					for x := part.Left; x < part.Right; x++ {
						covered[point{x, y}]++
					}
				}
			}
			for y := a.Top; y < a.Bottom; y++ {
				for x := a.Left; x < a.Right; x++ {
					want := 1
					if !b.empty() && x >= b.Left && x < b.Right && y >= b.Top && y < b.Bottom {
						want = 0
					}
					if got := covered[point{x, y}]; got != want {
						t.Fatalf("%+v less %+v: pixel (%d, %d) is in %d parts, want %d", a, b, x, y, got, want)
					}
				}
			}
		}
	}
}

func TestBlendSidesBlendsBothAtOnce(t *testing.T) {
	const white = 0xffffffff
	red, orange := core.RGB(0xff0000), core.RGB(0xff9500)
	// Half a pixel of each color covers it whole: the result is their
	// mix, so red blended and then orange would be too dark.
	if got, want := blendSides(white, red, 8, orange, 8), uint32(0xffff4a00); got != want {
		t.Errorf("half of each = %08x, want %08x", got, want)
	}
	if got := blendSides(white, red, 16, orange, 0); got != 0xffff0000 {
		t.Errorf("all red = %08x", got)
	}
	// A half transparent color over white lightens towards it.
	if got := blendSides(white, core.RGBA(0xff0000, 0.5), 16, 0, 0); got != 0xffff7f7f {
		t.Errorf("half red over white = %08x", got)
	}
	if got := blendSides(white, 0, 16, 0, 0); got != white {
		t.Errorf("nothing = %08x", got)
	}
}
