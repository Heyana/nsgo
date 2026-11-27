//go:build linux && !android

package linux

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

// renderNodePixels draws a GskRenderNode into a fresh Cairo ARGB32 surface and
// returns its bytes copy (BGRA premultiplied) and stride.
func renderNodePixels(t *testing.T, node uintptr, width, height int) ([]byte, int) {
	t.Helper()
	surface := cairoImageSurfaceCreate(0, int32(width), int32(height))
	defer cairoSurfaceDestroy(surface)
	cr := cairoCreate(surface)
	defer cairoDestroy(cr)
	if node != 0 {
		gskRenderNodeDraw(node, cr)
	}
	cairoSurfaceFlush(surface)
	data, stride := cairoImageSurfaceGetData(surface), int(cairoImageSurfaceGetStride(surface))
	out := make([]byte, stride*height)
	for y := range height {
		for x := range width {
			p := pixelAt(data, stride, x, y)
			off := y*stride + x*4
			copy(out[off:off+4], p[:])
		}
	}
	return out, stride
}

// TestDirectTextNodeMatchesPangoLayoutPixels verifies that buildDirectNode
// (engine-shaped runs via gsk_text_node_new) renders the same glyphs, colors,
// alignment, ellipsis, attachments and custom decorations as buildLayoutNode
// (main-thread PangoLayout via gtk_snapshot_append_layout).
func TestDirectTextNodeMatchesPangoLayoutPixels(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	w := &window{}
	defer w.releaseTextContexts()

	centerStyle := textStyle
	centerStyle.Align = driver.TextAlignCenter
	endStyle := textStyle
	endStyle.Align = driver.TextAlignEnd
	decoStyle := textStyle
	decoStyle.Decoration = driver.TextDecorationUnderline
	decoStyle.DecorationColor = driver.RGB(0xff0000)
	decoStyle.DecorationThickness = 2
	decoStyle.UnderlineOffset = driver.Pt(4)

	placeholder := string(driver.AttachmentRune)
	cases := []struct {
		name  string
		text  driver.AttributedText
		width driver.Points
	}{
		{
			name:  "plain-ascii",
			text:  driver.AttributedText{Text: "1810.HK -9.27%", Style: textStyle},
			width: driver.Auto,
		},
		{
			name: "multi-span-colors",
			text: driver.AttributedText{
				Text:  "Red Green Blue",
				Style: textStyle,
				Spans: []driver.TextSpan{
					{Start: 0, End: 3, Style: driver.TextStyle{Font: textStyle.Font, Color: driver.RGB(0xcc0000)}},
					{Start: 4, End: 9, Style: driver.TextStyle{Font: textStyle.Font, Color: driver.RGB(0x008800)}},
					{Start: 10, End: 14, Style: driver.TextStyle{Font: textStyle.Font, Color: driver.RGB(0x0000cc)}},
				},
			},
			width: driver.Auto,
		},
		{
			name:  "multiline-centered",
			text:  driver.AttributedText{Text: "First line\nSecond longer line", Style: centerStyle},
			width: 220,
		},
		{
			name:  "end-aligned",
			text:  driver.AttributedText{Text: "42,180.50", Style: endStyle},
			width: 180,
		},
		{
			name:  "bidi-mixed",
			text:  driver.AttributedText{Text: "ab אבג cd", Style: textStyle},
			width: 200,
		},
		{
			name:  "tail-ellipsis",
			text:  driver.AttributedText{Text: "Lorem ipsum dolor sit amet, consectetur adipiscing elit", Style: textStyle, MaxLines: 1, Ellipsize: driver.EllipsizeTail},
			width: 120,
		},
		{
			name: "attachment-gap",
			text: driver.AttributedText{
				Text:        "ab " + placeholder + " cd",
				Style:       textStyle,
				Attachments: []driver.TextAttachment{{Offset: 3, Size: driver.Size{Width: 24, Height: 16}}},
			},
			width: driver.Auto,
		},
		{
			name:  "custom-decoration",
			text:  driver.AttributedText{Text: "Underline", Style: decoStyle},
			width: driver.Auto,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := layoutText(t, w, &tc.text, tc.width)
			defer l.Release()
			if !l.runsBuilt {
				t.Fatalf("placeRuns did not populate shaped runs for %s", tc.name)
			}
			direct, ok := l.buildDirectNode(4, 4)
			if !ok {
				t.Fatalf("buildDirectNode fell back for %s", tc.name)
			}
			if direct != 0 {
				defer gskRenderNodeUnref(direct)
			}
			fallback := l.buildLayoutNode(4, 4)
			if fallback != 0 {
				defer gskRenderNodeUnref(fallback)
			}
			wPx := int(math.Ceil(float64(l.width))) + 24
			hPx := int(math.Ceil(float64(l.height))) + 24
			got, _ := renderNodePixels(t, direct, wPx, hPx)
			want, _ := renderNodePixels(t, fallback, wPx, hPx)

			inked := 0
			mismatched := 0
			for i := 0; i < len(got); i += 4 {
				if want[i+3] > 0x40 {
					inked++
				}
				diff := max(
					absInt(int(got[i])-int(want[i])),
					absInt(int(got[i+1])-int(want[i+1])),
					absInt(int(got[i+2])-int(want[i+2])),
					absInt(int(got[i+3])-int(want[i+3])),
				)
				if diff > 4 {
					mismatched++
				}
			}
			if inked == 0 {
				t.Fatalf("%s: reference layout drew zero inked pixels", tc.name)
			}
			// Allow tiny subpixel hinting differences if fonts differ slightly;
			// pixel positions and colors should match tightly.
			if mismatched > max(4, inked/50) {
				t.Fatalf("%s: %d mismatched pixels out of %d inked pixels", tc.name, mismatched, inked)
			}
		})
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// TestTextNodeCacheReuseAndInvalidation checks that textLayout reuses its
// cached GskRenderNode across snapshots at the same origin, rebuilds when the
// origin changes, and releases both node and PangoLayout cleanly.
func TestTextNodeCacheReuseAndInvalidation(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	w := &window{}
	defer w.releaseTextContexts()

	l := layoutText(t, w, &driver.AttributedText{Text: "Cached glyphs", Style: textStyle}, driver.Auto)
	snap1 := gtkSnapshotNew()
	l.snapshot(snap1, 2, 3)
	if n := gtkSnapshotFreeToNode(snap1); n != 0 {
		gskRenderNodeUnref(n)
	}
	if !l.nodeReady || l.node == 0 {
		t.Fatalf("expected cached node after first snapshot, got ready=%v node=%x", l.nodeReady, l.node)
	}
	if l.layout != 0 {
		t.Fatalf("direct text path unexpectedly created main-thread PangoLayout %x", l.layout)
	}
	first := l.node

	snap2 := gtkSnapshotNew()
	l.snapshot(snap2, 2, 3)
	if n := gtkSnapshotFreeToNode(snap2); n != 0 {
		gskRenderNodeUnref(n)
	}
	if l.node != first {
		t.Fatalf("cached node rebuilt on identical origin: got %x, want %x", l.node, first)
	}

	snap3 := gtkSnapshotNew()
	l.snapshot(snap3, 14, 9)
	if n := gtkSnapshotFreeToNode(snap3); n != 0 {
		gskRenderNodeUnref(n)
	}
	if !l.nodeReady || l.node == 0 || l.nodeX != 14 || l.nodeY != 9 {
		t.Fatalf("origin change did not update cached node: ready=%v node=%x pos=(%v,%v)", l.nodeReady, l.node, l.nodeX, l.nodeY)
	}

	l.Release()
	if l.node != 0 || l.nodeReady || l.layout != 0 {
		t.Fatalf("Release left live state: node=%x ready=%v layout=%x", l.node, l.nodeReady, l.layout)
	}

	// Clearing the main-thread font cache reloads fonts cleanly on next draw.
	clearMainFonts()
	if len(mainFonts.fonts) != 0 || mainFonts.context != 0 {
		t.Fatal("clearMainFonts did not reset cache")
	}
	l2 := layoutText(t, w, &driver.AttributedText{Text: "After clear", Style: textStyle}, driver.Auto)
	defer l2.Release()
	if node, ok := l2.buildDirectNode(0, 0); !ok || node == 0 {
		t.Fatalf("buildDirectNode after clearMainFonts failed: ok=%v node=%x", ok, node)
	} else {
		gskRenderNodeUnref(node)
	}
}

// TestContinuousPathCacheAndRectClipPixels verifies that continuous-corner
// GskPath caching reuses handles and that axis-aligned (0-radius) and rounded
// clips both clip pixels cleanly.
func TestContinuousPathCacheAndRectClipPixels(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	defer continuousPaths.clear(gskPathUnref)

	s := driver.DefaultViewStyle
	s.CornerRadius = 10
	s.ContinuousCorners = true
	sh := newShape(&s, 80, 40)
	p1 := sh.cachedPath(0)
	p2 := sh.cachedPath(0)
	if p1 == 0 || p1 != p2 {
		t.Fatalf("continuous path cache miss: p1=%x p2=%x", p1, p2)
	}

	// Zero-radius shape uses fast axis-aligned gtkSnapshotPushClip.
	plain := driver.DefaultViewStyle
	plain.Clip = true
	shRect := newShape(&plain, 20, 20)
	snap := gtkSnapshotNew()
	shRect.pushClip(snap)
	red := rgba(driver.RGB(0xff0000))
	big := grapheneRect{0, 0, 40, 40}
	gtkSnapshotAppendColor(snap, &red, &big)
	gtkSnapshotPop(snap)
	node := gtkSnapshotFreeToNode(snap)
	defer gskRenderNodeUnref(node)
	pix, stride := renderNodePixels(t, node, 40, 40)
	in := pix[10*stride+10*4 : 10*stride+10*4+4]
	out := pix[30*stride+30*4 : 30*stride+30*4+4]
	if in[2] != 0xff || in[3] != 0xff {
		t.Fatalf("inside rect clip = %v, want opaque red", in)
	}
	if out[3] != 0 {
		t.Fatalf("outside rect clip = %v, want transparent", out)
	}
}

// TestPerfHUDCachesRenderNode checks that the Linux perf HUD caches its
// GskRenderNode between 500 ms refreshes and releases it on stopPerf.
func TestPerfHUDCachesRenderNode(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	w := &window{hud: &hud{
		monitor: engine.NewPerfMonitor(),
		text:    "FPS 60\nMAIN 1.2 ms",
		done:    make(chan struct{}),
	}}
	stopped := false
	defer func() {
		if !stopped {
			w.stopPerf()
		}
	}()

	snap1 := gtkSnapshotNew()
	w.snapshotPerf(snap1, 400)
	if n := gtkSnapshotFreeToNode(snap1); n != 0 {
		gskRenderNodeUnref(n)
	}
	first := w.hud.node
	if first == 0 {
		t.Fatal("snapshotPerf did not cache HUD render node")
	}

	snap2 := gtkSnapshotNew()
	w.snapshotPerf(snap2, 400)
	if n := gtkSnapshotFreeToNode(snap2); n != 0 {
		gskRenderNodeUnref(n)
	}
	if w.hud.node != first {
		t.Fatalf("HUD node rebuilt unchanged: got %x, want %x", w.hud.node, first)
	}

	// Width change invalidates and rebuilds cached HUD node.
	snap3 := gtkSnapshotNew()
	w.snapshotPerf(snap3, 520)
	if n := gtkSnapshotFreeToNode(snap3); n != 0 {
		gskRenderNodeUnref(n)
	}
	if w.hud.node == 0 || w.hud.nodeWidth != 520 {
		t.Fatalf("HUD node did not update for width 520: node=%x width=%v", w.hud.node, w.hud.nodeWidth)
	}

	w.stopPerf()
	stopped = true
	if w.hud.node != 0 {
		t.Fatalf("stopPerf leaked HUD node %x", w.hud.node)
	}
}

// TestPrepareRolesReusesMaps verifies that ticking batches without ItemCreate
// incur zero allocations in prepareRoles.
func TestPrepareRolesReusesMaps(t *testing.T) {
	w := &window{}
	batch := &driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemText, ID: 1},
		{Kind: driver.ItemFrame, ID: 1},
	}}
	allocs := testing.AllocsPerRun(100, func() {
		w.prepareRoles(batch)
	})
	if allocs != 0 {
		t.Fatalf("prepareRoles on non-create batch allocated %.1f times, want 0", allocs)
	}
}

func BenchmarkSnapshotTextDirectCached(b *testing.B) {
	if err := openLibs(); err != nil {
		b.Skip(err)
	}
	w := &window{}
	defer w.releaseTextContexts()
	p := w.Prepare(&driver.AttributedText{Text: "1810.HK  24.85  +3.42%", Style: textStyle})
	defer p.Release()
	size := p.Measure(driver.Auto)
	l := p.Layout(size).(*textLayout)
	defer l.Release()
	// Warm the cached node once.
	warm := gtkSnapshotNew()
	l.snapshot(warm, 0, 0)
	if n := gtkSnapshotFreeToNode(warm); n != 0 {
		gskRenderNodeUnref(n)
	}
	b.ReportAllocs()
	for b.Loop() {
		snap := gtkSnapshotNew()
		l.snapshot(snap, 0, 0)
		if n := gtkSnapshotFreeToNode(snap); n != 0 {
			gskRenderNodeUnref(n)
		}
	}
}

func BenchmarkBuildTextDirectVsLayout(b *testing.B) {
	if err := openLibs(); err != nil {
		b.Skip(err)
	}
	w := &window{}
	defer w.releaseTextContexts()
	p := w.Prepare(&driver.AttributedText{Text: "1810.HK  24.85  +3.42%", Style: textStyle})
	defer p.Release()
	size := p.Measure(driver.Auto)
	l := p.Layout(size).(*textLayout)
	defer l.Release()

	b.Run("direct-gsk-text-node", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			node, ok := l.buildDirectNode(0, 0)
			if !ok {
				b.Fatal("direct build fell back")
			}
			if node != 0 {
				gskRenderNodeUnref(node)
			}
		}
	})
	b.Run("pango-layout-fallback", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			node := l.buildLayoutNode(0, 0)
			if node != 0 {
				gskRenderNodeUnref(node)
			}
		}
	})
}
