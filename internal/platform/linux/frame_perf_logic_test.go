package linux

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestClassifyStyleInvalidation(t *testing.T) {
	base := driver.DefaultViewStyle
	cases := []struct {
		name          string
		mutate        func(old, next *driver.ViewStyle)
		hadTransition bool
		hasTransition bool
		wantInv       viewInvalidation
		wantAllocSelf bool
	}{
		{
			name:    "identical style",
			wantInv: invalidateNone,
		},
		{
			name:    "background paint only",
			mutate:  func(_, next *driver.ViewStyle) { next.Background = driver.RGB(0x112233) },
			wantInv: invalidateDraw,
		},
		{
			name:    "opacity paint only",
			mutate:  func(_, next *driver.ViewStyle) { next.Opacity = 0.5 },
			wantInv: invalidateDraw,
		},
		{
			name: "border and radius paint only",
			mutate: func(_, next *driver.ViewStyle) {
				next.CornerRadius = 8
				next.BorderWidth = 1
				next.BorderColor = driver.RGB(0xffffff)
			},
			wantInv: invalidateDraw,
		},
		{
			name:    "clip change paint only",
			mutate:  func(_, next *driver.ViewStyle) { next.Clip = true },
			wantInv: invalidateDraw,
		},
		{
			name:    "blend mode paint only",
			mutate:  func(_, next *driver.ViewStyle) { next.BlendMode = driver.BlendMultiply },
			wantInv: invalidateDraw,
		},
		{
			name:    "effects pointer change paint only",
			mutate:  func(_, next *driver.ViewStyle) { next.Effects = &driver.Effects{} },
			wantInv: invalidateDraw,
		},
		{
			name:    "transform only repositions without redraw",
			mutate:  func(_, next *driver.ViewStyle) { next.Transform.TranslateX = 24 },
			wantInv: invalidateReposition,
		},
		{
			name:    "origin only repositions without redraw",
			mutate:  func(_, next *driver.ViewStyle) { next.Origin = driver.Point{X: 0.25, Y: 0.5} },
			wantInv: invalidateReposition,
		},
		{
			name: "transform with backface hidden redraws and repositions",
			mutate: func(old, next *driver.ViewStyle) {
				old.BackfaceHidden = true
				next.BackfaceHidden = true
				next.Transform.RotateY = 180
			},
			wantInv: invalidateBoth,
		},
		{
			name: "transform plus background redraws and repositions",
			mutate: func(_, next *driver.ViewStyle) {
				next.Transform.TranslateY = 10
				next.Background = driver.RGB(0xabcdef)
			},
			wantInv: invalidateBoth,
		},
		{
			name:    "hidden toggles both",
			mutate:  func(_, next *driver.ViewStyle) { next.Hidden = true },
			wantInv: invalidateBoth,
		},
		{
			name:          "active transition invalidates both",
			hasTransition: true,
			wantInv:       invalidateBoth,
		},
		{
			name:          "ending transition invalidates both",
			hadTransition: true,
			wantInv:       invalidateBoth,
		},
		{
			name:          "child perspective allocates self only",
			mutate:        func(_, next *driver.ViewStyle) { next.ChildPerspective = 800 },
			wantInv:       invalidateNone,
			wantAllocSelf: true,
		},
		{
			name:    "cursor only needs no draw or allocate",
			mutate:  func(_, next *driver.ViewStyle) { next.Cursor = driver.Cursor(1) },
			wantInv: invalidateNone,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old, next := base, base
			if tc.mutate != nil {
				tc.mutate(&old, &next)
			}
			inv, allocSelf := classifyStyleInvalidation(&old, &next, tc.hadTransition, tc.hasTransition)
			if inv != tc.wantInv || allocSelf != tc.wantAllocSelf {
				t.Fatalf("classifyStyleInvalidation() = (%v, %v), want (%v, %v)", inv, allocSelf, tc.wantInv, tc.wantAllocSelf)
			}
		})
	}
}

func TestClassifyAnimatedInvalidation(t *testing.T) {
	cases := []struct {
		name      string
		old, next driver.AnimatedValues
		backface  bool
		wantInv   viewInvalidation
		wantScale bool
	}{
		{
			name:    "unchanged",
			old:     driver.AnimatedValues{Set: 1 << driver.PropTranslateX, TranslateX: 10},
			next:    driver.AnimatedValues{Set: 1 << driver.PropTranslateX, TranslateX: 10},
			wantInv: invalidateNone,
		},
		{
			name:    "translate only repositions",
			old:     driver.AnimatedValues{Set: 1 << driver.PropTranslateX, TranslateX: 10},
			next:    driver.AnimatedValues{Set: 1 << driver.PropTranslateX, TranslateX: 25},
			wantInv: invalidateReposition,
		},
		{
			name:      "scale repositions and flags raster scale",
			old:       driver.AnimatedValues{Set: 1 << driver.PropScale, Scale: 1},
			next:      driver.AnimatedValues{Set: 1 << driver.PropScale, Scale: 2},
			wantInv:   invalidateReposition,
			wantScale: true,
		},
		{
			name:     "translate with backface hidden invalidates both",
			old:      driver.AnimatedValues{Set: 1 << driver.PropRotateY, RotateY: 10},
			next:     driver.AnimatedValues{Set: 1 << driver.PropRotateY, RotateY: 110},
			backface: true,
			wantInv:  invalidateBoth,
		},
		{
			name:    "opacity only redraws",
			old:     driver.AnimatedValues{Set: 1 << driver.PropOpacity, Opacity: 1},
			next:    driver.AnimatedValues{Set: 1 << driver.PropOpacity, Opacity: 0.4},
			wantInv: invalidateDraw,
		},
		{
			name:    "text color only redraws",
			old:     driver.AnimatedValues{Set: 1 << driver.PropTextColor, TextColor: driver.RGB(0x000000)},
			next:    driver.AnimatedValues{Set: 1 << driver.PropTextColor, TextColor: driver.RGB(0xff0000)},
			wantInv: invalidateDraw,
		},
		{
			name:    "translate and opacity invalidates both",
			old:     driver.AnimatedValues{Set: 1<<driver.PropTranslateY | 1<<driver.PropOpacity, TranslateY: 0, Opacity: 1},
			next:    driver.AnimatedValues{Set: 1<<driver.PropTranslateY | 1<<driver.PropOpacity, TranslateY: 12, Opacity: 0.8},
			wantInv: invalidateBoth,
		},
		{
			name:      "clearing scale bit repositions and flags scale",
			old:       driver.AnimatedValues{Set: 1 << driver.PropScaleX, ScaleX: 1.5},
			next:      driver.AnimatedValues{},
			wantInv:   invalidateReposition,
			wantScale: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inv, scale := classifyAnimatedInvalidation(tc.old, tc.next, tc.backface)
			if inv != tc.wantInv || scale != tc.wantScale {
				t.Fatalf("classifyAnimatedInvalidation() = (%v, %v), want (%v, %v)", inv, scale, tc.wantInv, tc.wantScale)
			}
		})
	}
}

func TestClassifyFrameInvalidation(t *testing.T) {
	a := driver.Bounds{Origin: driver.Point{X: 10, Y: 20}, Size: driver.Size{Width: 100, Height: 40}}
	if got := classifyFrameInvalidation(a, a); got != invalidateNone {
		t.Fatalf("identical frame = %v, want invalidateNone", got)
	}
	moved := driver.Bounds{Origin: driver.Point{X: 10, Y: 60}, Size: driver.Size{Width: 100, Height: 40}}
	if got := classifyFrameInvalidation(a, moved); got != invalidateReposition {
		t.Fatalf("moved frame = %v, want invalidateReposition", got)
	}
	resized := driver.Bounds{Origin: driver.Point{X: 10, Y: 20}, Size: driver.Size{Width: 120, Height: 40}}
	if got := classifyFrameInvalidation(a, resized); got != invalidateBoth {
		t.Fatalf("resized frame = %v, want invalidateBoth", got)
	}
}

func TestStyleHasTransform(t *testing.T) {
	s := driver.DefaultViewStyle
	if styleHasTransform(&s) {
		t.Fatal("default style reported transform")
	}
	s.Transform.TranslateX = 5
	if !styleHasTransform(&s) {
		t.Fatal("translated style reported no transform")
	}
	s = driver.DefaultViewStyle
	s.Animation.Keyframes = 1
	if !styleHasTransform(&s) {
		t.Fatal("keyframe animation style reported no transform")
	}
}

func TestBoundedPathCache(t *testing.T) {
	var cache boundedPathCache
	var released []uintptr
	release := func(p uintptr) { released = append(released, p) }

	key := func(i int) pathCacheKey {
		return pathCacheKey{w: float32(i + 1), h: 20, continuous: true}
	}
	if p, ok := cache.get(key(0)); ok || p != 0 {
		t.Fatalf("empty cache hit: (%v, %v)", p, ok)
	}
	for i := range maxCachedPaths {
		cache.put(key(i), uintptr(i+1), release)
	}
	if len(released) != 0 {
		t.Fatalf("unexpected eviction before full: %v", released)
	}
	// Touch key(0) so it becomes MRU and key(1) becomes LRU.
	if p, ok := cache.get(key(0)); !ok || p != 1 {
		t.Fatalf("get(key(0)) = (%v, %v), want (1, true)", p, ok)
	}
	// Inserting one more evicts key(1) (handle 2).
	cache.put(key(maxCachedPaths), uintptr(maxCachedPaths+1), release)
	if len(released) != 1 || released[0] != 2 {
		t.Fatalf("evicted = %v, want [2]", released)
	}
	if _, ok := cache.get(key(1)); ok {
		t.Fatal("evicted key(1) still present")
	}
	if p, ok := cache.get(key(0)); !ok || p != 1 {
		t.Fatalf("promoted key(0) = (%v, %v), want (1, true)", p, ok)
	}
	cache.clear(release)
	if len(cache.entries) != 0 {
		t.Fatalf("entries after clear = %d, want 0", len(cache.entries))
	}
	if len(released) != maxCachedPaths+1 {
		t.Fatalf("total released = %d, want %d", len(released), maxCachedPaths+1)
	}
}

type perfFocusNode struct {
	id       int
	kind     driver.ViewKind
	hidden   bool
	deleted  bool
	stopSelf bool
	children []*perfFocusNode
	parent   *perfFocusNode
}

func (n *perfFocusNode) focusKind() driver.ViewKind      { return n.kind }
func (n *perfFocusNode) focusHidden() bool               { return n.hidden }
func (n *perfFocusNode) focusDeleted() bool              { return n.deleted }
func (n *perfFocusNode) focusChildren() []*perfFocusNode { return n.children }
func (n *perfFocusNode) focusStopSelf() bool             { return n.stopSelf }

func (n *perfFocusNode) add(c *perfFocusNode) *perfFocusNode {
	c.parent = n
	n.children = append(n.children, c)
	return c
}

func TestSyncFocusSubtreeMatchesFullSync(t *testing.T) {
	root := &perfFocusNode{id: 1, kind: driver.KindView}
	panel := root.add(&perfFocusNode{id: 2, kind: driver.KindView})
	btn := panel.add(&perfFocusNode{id: 3, kind: driver.KindView, stopSelf: true})
	input := panel.add(&perfFocusNode{id: 4, kind: driver.KindInput, stopSelf: true})
	other := root.add(&perfFocusNode{id: 5, kind: driver.KindView, stopSelf: true})

	state := map[int][2]bool{}
	set := func(n *perfFocusNode, vf, ef bool) { state[n.id] = [2]bool{vf, ef} }
	syncFocusTree(root, set)

	verify := func(label string) {
		t.Helper()
		want := map[int][2]bool{}
		syncFocusTree(root, func(n *perfFocusNode, vf, ef bool) { want[n.id] = [2]bool{vf, ef} })
		if len(state) != len(want) {
			t.Fatalf("%s: len(state)=%d, want %d", label, len(state), len(want))
		}
		for id, w := range want {
			if state[id] != w {
				t.Fatalf("%s: node %d = %v, want %v", label, id, state[id], w)
			}
		}
	}
	verify("initial")

	// Hide panel incrementally.
	panel.hidden = true
	syncFocusSubtree(panel, root, true, false, set)
	verify("panel hidden")

	// Toggle input stop while panel is still hidden.
	input.stopSelf = false
	syncFocusSubtree(input, root, true, true, set)
	verify("input toggled under hidden panel")

	// Show panel again and insert a new focusable child.
	panel.hidden = false
	added := panel.add(&perfFocusNode{id: 6, kind: driver.KindView, stopSelf: true})
	syncFocusSubtree(panel, root, true, false, set)
	verify("panel shown with added child")

	// Toggle single leaf stop.
	btn.stopSelf = false
	syncFocusSubtree(btn, root, true, false, set)
	verify("btn disabled")
	_ = other
	_ = added

	// Modal scopes focus to itself via full sync.
	modal := root.add(&perfFocusNode{id: 7, kind: driver.KindModal})
	modalBtn := modal.add(&perfFocusNode{id: 8, kind: driver.KindView, stopSelf: true})
	syncFocusTree(root, set)
	if state[5][0] || !state[modalBtn.id][0] {
		t.Fatalf("modal focus scope failed: other=%v modalBtn=%v", state[5], state[modalBtn.id])
	}
}

func BenchmarkClassifyStyleInvalidation(b *testing.B) {
	old := driver.DefaultViewStyle
	next := old
	next.Transform.TranslateY = 42
	b.ReportAllocs()
	for b.Loop() {
		inv, alloc := classifyStyleInvalidation(&old, &next, false, false)
		if inv != invalidateReposition || alloc {
			b.Fatal("unexpected classification")
		}
	}
}

func BenchmarkClassifyAnimatedInvalidation(b *testing.B) {
	old := driver.AnimatedValues{Set: 1 << driver.PropTranslateX, TranslateX: 10}
	next := driver.AnimatedValues{Set: 1 << driver.PropTranslateX, TranslateX: 12.5}
	b.ReportAllocs()
	for b.Loop() {
		inv, scale := classifyAnimatedInvalidation(old, next, false)
		if inv != invalidateReposition || scale {
			b.Fatal("unexpected classification")
		}
	}
}

func BenchmarkBoundedPathCacheHit(b *testing.B) {
	var cache boundedPathCache
	for i := range 32 {
		cache.put(pathCacheKey{w: float32(i + 10), h: 32, continuous: true}, uintptr(i+1), nil)
	}
	target := pathCacheKey{w: 15, h: 32, continuous: true}
	b.ReportAllocs()
	for b.Loop() {
		if p, ok := cache.get(target); !ok || p == 0 {
			b.Fatal("cache miss")
		}
	}
}

func BenchmarkSyncFocusFullVsIncremental(b *testing.B) {
	root := &perfFocusNode{id: 0, kind: driver.KindView}
	var target *perfFocusNode
	for i := range 500 {
		row := root.add(&perfFocusNode{id: i*2 + 1, kind: driver.KindView})
		cell := row.add(&perfFocusNode{id: i*2 + 2, kind: driver.KindText, stopSelf: i%10 == 0})
		if i == 250 {
			target = cell
		}
	}
	sink := 0
	set := func(_ *perfFocusNode, vf, ef bool) {
		if vf || ef {
			sink++
		}
	}
	b.Run("full-500-rows", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			syncFocusTree(root, set)
		}
	})
	b.Run("incremental-1-cell", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			syncFocusSubtree(target, root, true, false, set)
		}
	})
}
