package windows

import (
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// rect is Win32's RECT: pixel bounds in client coordinates.
type rect struct{ Left, Top, Right, Bottom int32 }

func (r rect) empty() bool { return r.Right <= r.Left || r.Bottom <= r.Top }

func (r rect) intersect(o rect) rect {
	return rect{max(r.Left, o.Left), max(r.Top, o.Top), min(r.Right, o.Right), min(r.Bottom, o.Bottom)}
}

func (r rect) union(o rect) rect {
	return rect{min(r.Left, o.Left), min(r.Top, o.Top), max(r.Right, o.Right), max(r.Bottom, o.Bottom)}
}

func (r rect) area() int64 {
	if r.empty() {
		return 0
	}
	return int64(r.Right-r.Left) * int64(r.Bottom-r.Top)
}

func (r rect) touches(o rect) bool {
	return r.Left <= o.Right && o.Left <= r.Right && r.Top <= o.Bottom && o.Top <= r.Bottom
}

// maxDamage is the most areas a frame draws in.
const maxDamage = 24

// merged joins the rectangles of rs that touch, or whose union covers
// little more than they do, then the pairs whose union adds least, until
// at most maxDamage are left.
func merged(rs []rect) []rect {
	n := 0
	for _, r := range rs {
		if !r.empty() {
			rs[n] = r
			n++
		}
	}
	rs = rs[:n]
	for joined := true; joined; {
		joined = false
		for i := 0; i < len(rs); i++ {
			for j := i + 1; j < len(rs); {
				u := rs[i].union(rs[j])
				if rs[i].touches(rs[j]) || u.area() <= rs[i].area()+rs[j].area() {
					rs[i] = u
					rs[j] = rs[len(rs)-1]
					rs = rs[:len(rs)-1]
					joined = true
					// Re-check against remaining rects from i+1 since rs[i] grew.
					j = i + 1
					continue
				}
				j++
			}
		}
	}
	if len(rs) <= maxDamage {
		return rs
	}
	// Maintain each rect's cached area and nearest-merge partner so shrinking from N to
	// maxDamage avoids recomputing all O(N^2) pairs on every merge step.
	var (
		stackAreas   [256]int64
		stackCost    [256]int64
		stackPartner [256]int
		areas        []int64
		bestCost     []int64
		partner      []int
	)
	if len(rs) <= len(stackAreas) {
		areas = stackAreas[:len(rs)]
		bestCost = stackCost[:len(rs)]
		partner = stackPartner[:len(rs)]
	} else {
		areas = make([]int64, len(rs))
		bestCost = make([]int64, len(rs))
		partner = make([]int, len(rs))
	}
	for i := range rs {
		areas[i] = rs[i].area()
		bestCost[i] = math.MaxInt64
		partner[i] = -1
	}
	pairCost := func(i, j int) int64 {
		return rs[i].union(rs[j]).area() - areas[i] - areas[j]
	}
	for i := range rs {
		for j := i + 1; j < len(rs); j++ {
			c := pairCost(i, j)
			if c < bestCost[i] {
				bestCost[i], partner[i] = c, j
			}
			if c < bestCost[j] {
				bestCost[j], partner[j] = c, i
			}
		}
	}
	recompute := func(k, limit int) {
		bc, bp := int64(math.MaxInt64), -1
		rk, ak := rs[k], areas[k]
		for j := range limit {
			if j == k {
				continue
			}
			if c := rk.union(rs[j]).area() - ak - areas[j]; c < bc {
				bc, bp = c, j
			}
		}
		bestCost[k], partner[k] = bc, bp
	}
	for len(rs) > maxDamage {
		bi := 0
		best := bestCost[0]
		for i := 1; i < len(rs); i++ {
			if bestCost[i] < best {
				bi, best = i, bestCost[i]
			}
		}
		bj := partner[bi]
		if bj < bi {
			bi, bj = bj, bi
		}
		rs[bi] = rs[bi].union(rs[bj])
		areas[bi] = rs[bi].area()
		last := len(rs) - 1
		if bj != last {
			rs[bj] = rs[last]
			areas[bj] = areas[last]
			bestCost[bj] = bestCost[last]
			partner[bj] = partner[last]
		}
		rs = rs[:last]
		areas = areas[:last]
		bestCost = bestCost[:last]
		partner = partner[:last]
		for k := range rs {
			if partner[k] == last {
				partner[k] = bj
			}
		}
		recompute(bi, len(rs))
		for k := range rs {
			if k == bi {
				continue
			}
			if partner[k] == bi || partner[k] == bj || partner[k] < 0 {
				recompute(k, len(rs))
			} else if c := pairCost(k, bi); c < bestCost[k] {
				bestCost[k], partner[k] = c, bi
			}
		}
	}
	return rs
}

type innerClipKind uint8

const (
	innerClipPath innerClipKind = iota
	innerClipRect
	innerClipUniformRounded
)

// classifyInnerClip picks the cheapest Direct2D clip for a padding box's
// inner corner radii: an axis-aligned clip for square inner corners, a
// rounded-rectangle geometry for uniform circular corners, else a path.
func classifyInnerClip(inner [4][2]float64, continuous bool) (float32, innerClipKind) {
	if inner[0][0] <= 0 && inner[0][1] <= 0 &&
		inner[1][0] <= 0 && inner[1][1] <= 0 &&
		inner[2][0] <= 0 && inner[2][1] <= 0 &&
		inner[3][0] <= 0 && inner[3][1] <= 0 {
		return 0, innerClipRect
	}
	r := inner[0][0]
	if !continuous && r > 0 &&
		inner[0][1] == r &&
		inner[1][0] == r && inner[1][1] == r &&
		inner[2][0] == r && inner[2][1] == r &&
		inner[3][0] == r && inner[3][1] == r {
		return float32(r), innerClipUniformRounded
	}
	return 0, innerClipPath
}

// formatKey identifies a reusable configured IDWriteTextFormat.
type formatKey struct {
	family   string
	language string
	weight   driver.FontWeight
	italic   bool
	size     float32
	align    driver.TextAlign
	rtl      bool
	wrap     uint32
	step     float32
	baseline float32
	trim     bool
}
