package core

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestScopeTextMeasurementsFollowFontChanges(t *testing.T) {
	registered := false
	h := newHost(t, Size{Width: 400, Height: 100}, nil)
	h.tree = NewTree(h.root, fontEngine{&registered})
	h.tree.Resize(Size{Width: 400, Height: 100}, Geometry{})
	measurements := 0
	h.tree.Mount(func(cx *Scope) Node {
		width := NewMemo(cx, func() float32 {
			measurements++
			return cx.MeasureText("hello", Font{Size: 20}).Width
		})
		return Canvas{TestID: "measured", EmFunc: func() EmBox {
			return EmBox{Width: width.Get() / 20, Ascent: .8, Descent: .2}
		}, Style: Style{FontSize: 20, AlignSelf: AlignStart}}
	})
	h.commit()
	before := h.find("measured").frame.Size.Width
	registered = true
	h.tree.FontsChanged()
	h.commit()
	if got := h.find("measured").frame.Size.Width; got != before*2 || measurements != 2 {
		t.Fatalf("font change left stale measurements: width=%v, before=%v, measurements=%d", got, before, measurements)
	}
	h.commit()
	if measurements != 2 {
		t.Fatal("idle commit measured the text again")
	}
}

func TestCanvasDrawsAgainWhenFontsChange(t *testing.T) {
	for _, async := range []bool{false, true} {
		name := "RegisterFont"
		if async {
			name = "FontsChanged"
		}
		t.Run(name, func(t *testing.T) {
			registered := false
			var scope *Scope
			var scale *Signal[float32]
			draws := 0
			h := newHost(t, Size{Width: 300, Height: 100}, nil)
			h.tree = NewTree(h.root, fontEngine{&registered})
			h.tree.Resize(Size{Width: 300, Height: 100}, Geometry{})
			h.tree.Mount(func(cx *Scope) Node {
				scope = cx
				scale = NewSignal(cx, float32(1))
				return Group{Icon{SVG: MustParseSVG([]byte(`<svg width="10" height="10"><path d="M0 0L10 10"/></svg>`))}, Canvas{TestID: "measured", Style: Style{Height: Pt(60)}, Draw: func(d *Drawing, _ Size) {
					draws++
					// Measuring text can affect geometry without recording a text op.
					width := d.TextMetrics("hello", Font{Family: "late font", Size: 20}).Width
					d.FillRect(0, 0, width*scale.Get(), 20)
				}}}
			})
			h.commit()
			width := func() float32 {
				for _, op := range h.find("measured").canvas.drawing.Ops {
					if op.Kind == DrawRect {
						return op.W
					}
				}
				t.Fatal("the canvas recorded no rectangle")
				return 0
			}
			before := width()
			if async {
				registered = true
				h.tree.FontsChanged()
			} else if err := scope.RegisterFont([]byte("font")); err != nil {
				t.Fatal(err)
			}
			batch := h.commit()
			if draws != 2 || h.drawings(batch, "measured") != 1 || width() != before*2 {
				t.Fatalf("after font change: draws=%d, width=%v (before %v), canvas updates=%d", draws, width(), before, h.drawings(batch, "measured"))
			}
			scale.Set(2)
			h.commit()
			if draws != 3 || width() != before*4 {
				t.Fatalf("drawing lost its signal dependency: draws=%d, width=%v", draws, width())
			}
			if h.drawings(h.commit(), "measured") != 0 || draws != 3 {
				t.Fatal("an idle commit drew the canvas again")
			}
		})
	}
}

func TestFontChangeDoesNotDrawDisposedCanvases(t *testing.T) {
	var shown *Signal[bool]
	draws := 0
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		shown = NewSignal(cx, true)
		return Show{When: shown.Get, Then: func(*Scope) Node {
			return Canvas{Style: Style{Height: Pt(40)}, Draw: func(d *Drawing, _ Size) {
				draws++
				d.FillText("hello", 0, 20, Font{Size: 20})
			}}
		}}
	})
	h.commit()
	shown.Set(false)
	h.commit()
	h.tree.FontsChanged()
	batch := h.commit()
	if draws != 1 || kinds(batch)[driver.ItemCanvas] != 0 {
		t.Fatalf("disposed canvas drew again: draws=%d, items=%v", draws, kinds(batch))
	}
}

func TestCanvasSteadyStateRedrawAllocatesNothing(t *testing.T) {
	atlas := Src("asset:sprites.png")
	rects := make([]Bounds, 100)
	xforms := make([]RSXform, 100)
	alphas := make([]float32, 100)
	for i := range 100 {
		rects[i] = Bounds{Origin: Point{X: Points((i % 8) * 32), Y: Points((i / 8) * 32)}, Size: Size{Width: 32, Height: 32}}
		xforms[i] = RSXform{SCos: 1, TX: float32((i % 10) * 16), TY: float32((i / 10) * 16)}
		alphas[i] = 1
	}
	var frame *Signal[int]
	root := NewRoot(RootOptions{Post: func(func()) {}, RequestFrame: func() {}})
	tree := NewTree(root, ApproxTextEngine{})
	tree.Resize(Size{Width: 800, Height: 600}, Geometry{})
	tree.Mount(func(cx *Scope) Node {
		frame = NewSignal(cx, 0)
		return Canvas{
			Style:           Style{Width: Pt(800), Height: Pt(600)},
			ResolutionScale: 0.5,
			Draw: func(d *Drawing, _ Size) {
				f := frame.Get()
				shift := float32(f % 32)
				for range 3 {
					d.DrawAtlas(atlas, rects, xforms, alphas)
				}
				for i := range 200 {
					d.DrawImageFrom(atlas, 0, 0, 32, 32, float32(i%25)*16+shift, float32(i/25)*16, 16, 16)
				}
			},
		}
	})
	// Warm up initial commit and both double-buffered drawings.
	for i := range 4 {
		frame.Set(i)
		if b := tree.CommitShared(); b == nil {
			t.Fatalf("warmup commit %d returned nil", i)
		}
	}
	step := 4
	allocs := testing.AllocsPerRun(20, func() {
		frame.Set(step)
		step++
		if b := tree.CommitShared(); b == nil {
			t.Fatal("steady commit returned nil")
		}
	})
	if allocs != 0 {
		t.Fatalf("steady-state canvas redraw allocated %.2f times per frame, want 0", allocs)
	}
}

func TestCanvasBufferReuseRespectsPlatformRetain(t *testing.T) {
	atlas := Src("asset:sprites.png")
	var frame *Signal[int]
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		frame = NewSignal(cx, 1)
		return Canvas{
			TestID: "c",
			Style:  Style{Width: Pt(200), Height: Pt(100)},
			Draw: func(d *Drawing, _ Size) {
				d.DrawImageFrom(atlas, 0, 0, 16, 16, float32(frame.Get()), 0, 16, 16)
			},
		}
	})
	canvasDrawing := func(b *driver.MountBatch) *Drawing {
		for _, it := range b.Items {
			if it.Kind == driver.ItemCanvas {
				return it.Drawing
			}
		}
		return nil
	}
	d1 := canvasDrawing(h.commit())
	if d1 == nil || d1.Ops[0].X != 1 {
		t.Fatalf("frame 1 drawing = %+v", d1)
	}
	// Platform retains frame 1 while frames 2 and 3 commit.
	d1.Retain()
	frame.Set(2)
	d2 := canvasDrawing(h.commit())
	frame.Set(3)
	d3 := canvasDrawing(h.commit())
	if d3 == d1 || d1.Ops[0].X != 1 {
		t.Fatalf("retained frame 1 was overwritten: d1=%p d3=%p d1.X=%v", d1, d3, d1.Ops[0].X)
	}
	if d2 == d1 || d3 == d2 || d3.Ops[0].X != 3 {
		t.Fatalf("unexpected drawings: d2=%p d3=%p d3.X=%v", d2, d3, d3.Ops[0].X)
	}
	// Once the platform releases frame 1, a subsequent frame reuses it.
	d1.Release()
	frame.Set(4)
	d4 := canvasDrawing(h.commit())
	if d4 != d1 || d4.Ops[0].X != 4 {
		t.Fatalf("released drawing was not reused: d1=%p d4=%p d4.X=%v", d1, d4, d4.Ops[0].X)
	}
}
