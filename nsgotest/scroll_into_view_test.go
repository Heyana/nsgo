package nsgotest

import (
	"math"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestScrollIntoViewLongPageAndPress(t *testing.T) {
	var dragStarts, scrolls, dragEnds, scrollEnds, presses int
	app := Start(t, func(*core.Scope) core.Node {
		return core.Scroll{
			TestID:            "page",
			Style:             core.Style{Width: core.Pt(300), Height: core.Pt(400)},
			OnScrollDragStart: func(core.ScrollEvent) { dragStarts++ },
			OnScroll:          func(core.ScrollEvent) { scrolls++ },
			OnScrollDragEnd:   func(core.ScrollEvent) { dragEnds++ },
			OnScrollEnd:       func(core.ScrollEvent) { scrollEnds++ },
			Children: []core.Node{
				core.View{TestID: "top-btn", Style: core.Style{Height: core.Pt(40)}, OnPress: func(core.PressEvent) {}},
				core.View{Style: core.Style{Height: core.Pt(1000)}},
				core.View{
					TestID:  "bottom-btn",
					Style:   core.Style{Height: core.Pt(40)},
					OnPress: func(core.PressEvent) { presses++ },
					Children: []core.Node{
						core.Text{Content: "Save"},
					},
				},
				core.View{Style: core.Style{Height: core.Pt(80)}},
			},
		}
	}, Size(300, 400))

	app.ScrollIntoView("id:bottom-btn")
	if got := app.Find("id:page").Offset.Y; got != 680 {
		t.Fatalf("offset after ScrollIntoView(bottom-btn) = %v, want 680", got)
	}
	if dragStarts != 1 || scrolls == 0 || dragEnds != 1 || scrollEnds != 1 {
		t.Fatalf("scroll events = start:%d scroll:%d end:%d scrollEnd:%d, want 1/>0/1/1", dragStarts, scrolls, dragEnds, scrollEnds)
	}
	wf := app.Find("id:bottom-btn").WindowFrame()
	if wf.Origin.Y != 360 || wf.Origin.Y+wf.Size.Height != 400 {
		t.Fatalf("bottom-btn WindowFrame = %+v, want Y in [360, 400]", wf)
	}

	app.Press("id:bottom-btn")
	if presses != 1 {
		t.Fatalf("presses = %d, want 1", presses)
	}

	// Scroll back up to the top button: should align its top edge to viewport top (offset 0).
	app.ScrollIntoView("id:top-btn")
	if got := app.Find("id:page").Offset.Y; got != 0 {
		t.Fatalf("offset after ScrollIntoView(top-btn) = %v, want 0", got)
	}
	if top := app.Find("id:top-btn").WindowFrame(); top.Origin.Y != 0 {
		t.Fatalf("top-btn WindowFrame = %+v, want Y=0", top)
	}
}

func TestScrollIntoViewAlreadyInViewDoesNotScroll(t *testing.T) {
	var dragStarts, scrolls, dragEnds, scrollEnds int
	app := Start(t, func(*core.Scope) core.Node {
		return core.Scroll{
			TestID:            "page",
			Style:             core.Style{Width: core.Pt(300), Height: core.Pt(400)},
			OnScrollDragStart: func(core.ScrollEvent) { dragStarts++ },
			OnScroll:          func(core.ScrollEvent) { scrolls++ },
			OnScrollDragEnd:   func(core.ScrollEvent) { dragEnds++ },
			OnScrollEnd:       func(core.ScrollEvent) { scrollEnds++ },
			Children: []core.Node{
				core.View{Style: core.Style{Height: core.Pt(50)}},
				core.View{TestID: "visible", Style: core.Style{Height: core.Pt(60)}},
				core.View{Style: core.Style{Height: core.Pt(900)}},
			},
		}
	}, Size(300, 400))

	app.ScrollIntoView("id:visible")
	if got := app.Find("id:page").Offset; got != (core.Point{}) {
		t.Fatalf("already-visible ScrollIntoView moved offset to %+v, want 0", got)
	}
	if dragStarts != 0 || scrolls != 0 || dragEnds != 0 || scrollEnds != 0 {
		t.Fatalf("already-visible ScrollIntoView fired events start:%d scroll:%d end:%d scrollEnd:%d", dragStarts, scrolls, dragEnds, scrollEnds)
	}
}

func TestScrollIntoViewNestedScrollViews(t *testing.T) {
	t.Run("vertical inside vertical scrolls innermost", func(t *testing.T) {
		var outerScrolls, innerScrolls int
		app := Start(t, func(*core.Scope) core.Node {
			return core.Scroll{
				TestID:   "outer",
				Style:    core.Style{Width: core.Pt(300), Height: core.Pt(400)},
				OnScroll: func(core.ScrollEvent) { outerScrolls++ },
				Children: []core.Node{
					core.View{Style: core.Style{Height: core.Pt(20)}},
					core.Scroll{
						TestID:   "inner",
						Style:    core.Style{Width: core.Pt(260), Height: core.Pt(160), MinHeight: core.Pt(160)},
						OnScroll: func(core.ScrollEvent) { innerScrolls++ },
						Children: []core.Node{
							core.View{Style: core.Style{Height: core.Pt(400)}},
							core.View{TestID: "deep-item", Style: core.Style{Height: core.Pt(40)}},
						},
					},
					core.View{Style: core.Style{Height: core.Pt(800)}},
				},
			}
		}, Size(300, 400))

		app.ScrollIntoView("id:deep-item")
		if got := app.Find("id:outer").Offset.Y; got != 0 || outerScrolls != 0 {
			t.Fatalf("outer scroll moved: offset=%v events=%d, want 0", got, outerScrolls)
		}
		if got := app.Find("id:inner").Offset.Y; got != 280 || innerScrolls == 0 {
			t.Fatalf("inner scroll offset=%v events=%d, want 280 and >0 events", got, innerScrolls)
		}
		inner := app.Find("id:inner").WindowFrame()
		item := app.Find("id:deep-item").WindowFrame()
		if item.Origin.Y < inner.Origin.Y || item.Origin.Y+item.Size.Height > inner.Origin.Y+inner.Size.Height {
			t.Fatalf("item %+v not inside inner %+v", item, inner)
		}
	})

	t.Run("horizontal inside vertical scrolls inner horizontal", func(t *testing.T) {
		app := Start(t, func(*core.Scope) core.Node {
			return core.Scroll{
				TestID: "outer",
				Style:  core.Style{Width: core.Pt(300), Height: core.Pt(400)},
				Children: []core.Node{
					core.Scroll{
						TestID:     "hstrip",
						Horizontal: true,
						Style:      core.Style{Width: core.Pt(200), Height: core.Pt(80), MinHeight: core.Pt(80)},
						Children: []core.Node{
							core.View{Style: core.Style{Width: core.Pt(500), Height: core.Pt(80), FlexShrink: 0}},
							core.View{TestID: "h-item", Style: core.Style{Width: core.Pt(60), Height: core.Pt(80), FlexShrink: 0}},
						},
					},
					core.View{Style: core.Style{Height: core.Pt(800)}},
				},
			}
		}, Size(300, 400))

		app.ScrollIntoView("id:h-item")
		if got := app.Find("id:outer").Offset; got != (core.Point{}) {
			t.Fatalf("outer offset = %+v, want 0", got)
		}
		if got := app.Find("id:hstrip").Offset.X; got != 360 {
			t.Fatalf("hstrip offset.X = %v, want 360", got)
		}
	})

	t.Run("targeting inner scroll view scrolls outer", func(t *testing.T) {
		app := Start(t, func(*core.Scope) core.Node {
			return core.Scroll{
				TestID: "outer",
				Style:  core.Style{Width: core.Pt(300), Height: core.Pt(300)},
				Children: []core.Node{
					core.View{Style: core.Style{Height: core.Pt(500)}},
					core.Scroll{
						TestID: "inner",
						Style:  core.Style{Width: core.Pt(260), Height: core.Pt(100), MinHeight: core.Pt(100)},
						Children: []core.Node{
							core.View{Style: core.Style{Height: core.Pt(400)}},
						},
					},
				},
			}
		}, Size(300, 300))

		app.ScrollIntoView("id:inner")
		if got := app.Find("id:outer").Offset.Y; got != 300 {
			t.Fatalf("outer offset.Y = %v, want 300", got)
		}
		if got := app.Find("id:inner").Offset.Y; got != 0 {
			t.Fatalf("inner offset.Y = %v, want 0", got)
		}
	})
}

func TestScrollIntoViewTallerThanViewport(t *testing.T) {
	var scrolls int
	app := Start(t, func(*core.Scope) core.Node {
		return core.Scroll{
			TestID:   "page",
			Style:    core.Style{Width: core.Pt(300), Height: core.Pt(200)},
			OnScroll: func(core.ScrollEvent) { scrolls++ },
			Children: []core.Node{
				core.View{Style: core.Style{Height: core.Pt(300)}},
				core.View{TestID: "tall", Style: core.Style{Height: core.Pt(350)}},
				core.View{Style: core.Style{Height: core.Pt(400)}},
			},
		}
	}, Size(300, 200))

	// Tall view starts below viewport [300, 650]: nearest edge is its top -> offset 300.
	app.ScrollIntoView("id:tall")
	if got := app.Find("id:page").Offset.Y; got != 300 {
		t.Fatalf("offset from above = %v, want 300", got)
	}
	if wf := app.Find("id:tall").WindowFrame(); wf.Origin.Y != 0 {
		t.Fatalf("tall WindowFrame = %+v, want top aligned at Y=0", wf)
	}

	// Already covering the entire viewport [0, 200]: should NOT scroll.
	app.ScrollTo("id:page", core.Point{Y: 350})
	scrolls = 0
	app.ScrollIntoView("id:tall")
	if got := app.Find("id:page").Offset.Y; got != 350 || scrolls != 0 {
		t.Fatalf("spanning tall view scrolled: offset=%v scrolls=%d, want 350 and 0", got, scrolls)
	}

	// Scroll past the tall view (offset 750 puts tall at [-450, -100]): nearest edge is its bottom -> offset 450 (tall at [-150, 200]).
	app.ScrollTo("id:page", core.Point{Y: 750})
	app.ScrollIntoView("id:tall")
	if got := app.Find("id:page").Offset.Y; got != 450 {
		t.Fatalf("offset from below = %v, want 450", got)
	}
	if wf := app.Find("id:tall").WindowFrame(); wf.Origin.Y+wf.Size.Height != 200 {
		t.Fatalf("tall WindowFrame = %+v, want bottom aligned at Y=200", wf)
	}
}

func TestScrollIntoViewErrors(t *testing.T) {
	if msg := fails(t, func(tb testing.TB) {
		app := Start(tb, func(*core.Scope) core.Node {
			return core.View{TestID: "plain", Style: core.Style{Width: core.Pt(100), Height: core.Pt(40)}}
		})
		app.ScrollIntoView("id:plain")
	}); !strings.Contains(msg, "no scroll view encloses") {
		t.Errorf("without scroll view failed with %q, want 'no scroll view encloses'", msg)
	}

	if msg := fails(t, func(tb testing.TB) {
		app := Start(tb, func(*core.Scope) core.Node {
			return core.Scroll{
				TestID:   "locked",
				Behavior: core.ScrollBehavior{NoScroll: true},
				Style:    core.Style{Width: core.Pt(200), Height: core.Pt(100)},
				Children: []core.Node{core.View{TestID: "item", Style: core.Style{Height: core.Pt(40)}}},
			}
		})
		app.ScrollIntoView("id:item")
	}); !strings.Contains(msg, "scrolling disabled") {
		t.Errorf("NoScroll ancestor failed with %q, want 'scrolling disabled'", msg)
	}

	if msg := fails(t, func(tb testing.TB) {
		app := Start(tb, func(*core.Scope) core.Node {
			return core.Scroll{
				TestID:   "page",
				Style:    core.Style{Width: core.Pt(200), Height: core.Pt(100)},
				Children: []core.Node{core.View{Style: core.Style{Height: core.Pt(400)}}},
			}
		})
		app.ScrollIntoView("id:missing")
	}); !strings.Contains(msg, "no view matches") {
		t.Errorf("missing selector failed with %q, want 'no view matches'", msg)
	}
}

func TestScrollIntoViewHorizontalAndContentInsets(t *testing.T) {
	t.Run("vertical content insets", func(t *testing.T) {
		var scrolls int
		app := Start(t, func(*core.Scope) core.Node {
			return core.Scroll{
				TestID:        "page",
				ContentOffset: core.Point{Y: -50},
				Behavior:      core.ScrollBehavior{ContentInset: core.Edges{Top: 50, Bottom: 40}},
				Style:         core.Style{Width: core.Pt(300), Height: core.Pt(300)},
				OnScroll:      func(core.ScrollEvent) { scrolls++ },
				Children: []core.Node{
					core.View{TestID: "first", Style: core.Style{Height: core.Pt(40)}},
					core.View{Style: core.Style{Height: core.Pt(700)}},
					core.View{TestID: "last", Style: core.Style{Height: core.Pt(40)}},
				},
			}
		}, Size(300, 300))

		// At offset -50, first sits at Y=[50, 90], inside inset viewport [50, 260]: no scroll.
		scrolls = 0
		app.ScrollIntoView("id:first")
		if got := app.Find("id:page").Offset.Y; got != -50 || scrolls != 0 {
			t.Fatalf("first inside inset viewport scrolled: offset=%v scrolls=%d", got, scrolls)
		}

		// last is at content Y=[740, 780]; bringing it clear of Bottom inset (260) requires offset 520.
		app.ScrollIntoView("id:last")
		if got := app.Find("id:page").Offset.Y; got != 520 {
			t.Fatalf("offset after ScrollIntoView(last) = %v, want 520", got)
		}
		if wf := app.Find("id:last").WindowFrame(); wf.Origin.Y != 220 || wf.Origin.Y+wf.Size.Height != 260 {
			t.Fatalf("last WindowFrame = %+v, want Y in [220, 260]", wf)
		}

		// Scrolling back to first must reach -50 so it clears Top inset (50).
		app.ScrollIntoView("id:first")
		if got := app.Find("id:page").Offset.Y; got != -50 {
			t.Fatalf("offset after ScrollIntoView(first) = %v, want -50", got)
		}
		if wf := app.Find("id:first").WindowFrame(); wf.Origin.Y != 50 {
			t.Fatalf("first WindowFrame = %+v, want Y=50", wf)
		}
	})

	t.Run("horizontal content insets", func(t *testing.T) {
		app := Start(t, func(*core.Scope) core.Node {
			return core.Scroll{
				TestID:        "row",
				Horizontal:    true,
				ContentOffset: core.Point{X: -30},
				Behavior:      core.ScrollBehavior{ContentInset: core.Edges{Left: 30, Right: 20}},
				Style:         core.Style{Width: core.Pt(250), Height: core.Pt(100)},
				Children: []core.Node{
					core.View{TestID: "start", Style: core.Style{Width: core.Pt(40), Height: core.Pt(60), FlexShrink: 0}},
					core.View{Style: core.Style{Width: core.Pt(500), Height: core.Pt(60), FlexShrink: 0}},
					core.View{TestID: "end", Style: core.Style{Width: core.Pt(60), Height: core.Pt(60), FlexShrink: 0}},
				},
			}
		}, Size(300, 120))

		// end is at content X=[540, 600]; inset viewport right is 230 -> offset 370.
		app.ScrollIntoView("id:end")
		if got := app.Find("id:row").Offset.X; got != 370 {
			t.Fatalf("offset after ScrollIntoView(end) = %v, want 370", got)
		}
		if wf := app.Find("id:end").WindowFrame(); wf.Origin.X != 170 || wf.Origin.X+wf.Size.Width != 230 {
			t.Fatalf("end WindowFrame = %+v, want X in [170, 230]", wf)
		}

		// Back to start: clears Left inset 30 -> offset -30.
		app.ScrollIntoView("id:start")
		if got := app.Find("id:row").Offset.X; got != -30 {
			t.Fatalf("offset after ScrollIntoView(start) = %v, want -30", got)
		}
	})
}

func TestScrollIntoViewZoomedTwoDimensional(t *testing.T) {
	var scrollRef core.Ref
	app := Start(t, func(*core.Scope) core.Node {
		return core.Scroll{
			TestID:       "zoom",
			Ref:          &scrollRef,
			MaxZoomScale: 2,
			Style:        core.Style{Width: core.Pt(150), Height: core.Pt(100)},
			Children: []core.Node{
				core.View{
					Style: core.Style{Width: core.Pt(300), Height: core.Pt(300)},
					Children: []core.Node{
						core.View{
							TestID: "corner",
							Style:  core.Style{Position: core.Absolute, Left: core.Pt(200), Top: core.Pt(220), Width: core.Pt(40), Height: core.Pt(30)},
						},
					},
				},
			},
		}
	}, Size(200, 200))

	app.Post(func() { scrollRef.ZoomTo(2, nil, false) })
	app.Settle()

	app.ScrollIntoView("id:corner")
	// At zoom 2, corner occupies zoomed [400..480] x [440..500] in a 150x100 viewport -> offset (330, 400).
	if got := app.Find("id:zoom").Offset; got != (core.Point{X: 330, Y: 400}) {
		t.Fatalf("zoomed offset = %+v, want {330 400}", got)
	}
	wf := app.Find("id:corner").WindowFrame()
	if wf != (core.Bounds{Origin: core.Point{X: 70, Y: 40}, Size: core.Size{Width: 80, Height: 60}}) {
		t.Fatalf("zoomed corner WindowFrame = %+v, want {70,40 80x60}", wf)
	}
}

func TestViewWindowFrame(t *testing.T) {
	var zoomRef core.Ref
	app := Start(t, func(*core.Scope) core.Node {
		return core.View{
			Style: core.Style{Padding: core.Edges{Left: 12, Top: 18}},
			Children: []core.Node{
				core.Scroll{
					TestID: "outer",
					Style:  core.Style{Width: core.Pt(260), Height: core.Pt(200)},
					Children: []core.Node{
						core.View{Style: core.Style{Height: core.Pt(40)}},
						core.Scroll{
							TestID: "inner",
							Style:  core.Style{Margin: core.Edges{Left: 8}, Width: core.Pt(200), Height: core.Pt(100), MinHeight: core.Pt(100)},
							Children: []core.Node{
								core.View{
									TestID: "shifted",
									Style: core.Style{
										Margin:    core.Edges{Left: 5, Top: 120},
										Width:     core.Pt(50),
										Height:    core.Pt(30),
										Transform: core.Transform{TranslateX: 10, TranslateY: -4},
									},
								},
							},
						},
						core.View{Style: core.Style{Height: core.Pt(200)}},
					},
				},
				core.Scroll{
					TestID:       "zoom",
					Ref:          &zoomRef,
					MaxZoomScale: 2,
					Style:        core.Style{Width: core.Pt(120), Height: core.Pt(80)},
					Children: []core.Node{
						core.View{
							TestID: "zchild",
							Style:  core.Style{Position: core.Absolute, Left: core.Pt(30), Top: core.Pt(20), Width: core.Pt(40), Height: core.Pt(25)},
						},
					},
				},
			},
		}
	}, Size(320, 400))

	app.ScrollTo("id:outer", core.Point{Y: 15})
	app.ScrollTo("id:inner", core.Point{Y: 60})
	// Layout Bounds ignores Transform: X = 12 + 8 + 5 = 25, Y = 18 + 40 - 15 + 120 - 60 = 103.
	if b := app.Find("id:shifted").Bounds(); b != (core.Bounds{Origin: core.Point{X: 25, Y: 103}, Size: core.Size{Width: 50, Height: 30}}) {
		t.Fatalf("shifted Bounds = %+v, want {25,103 50x30}", b)
	}
	// WindowFrame includes Transform (+10, -4): X = 35, Y = 99.
	if wf := app.Find("id:shifted").WindowFrame(); wf != (core.Bounds{Origin: core.Point{X: 35, Y: 99}, Size: core.Size{Width: 50, Height: 30}}) {
		t.Fatalf("shifted WindowFrame = %+v, want {35,99 50x30}", wf)
	}

	app.Post(func() { zoomRef.ZoomTo(2, nil, false); zoomRef.ScrollTo(core.Point{X: 10, Y: 12}, false) })
	app.Settle()
	// zoom view starts at (12, 18 + 200 = 218). At zoom 2 and offset (10, 12), zchild (30,20 40x25) lands at:
	// X = 12 + 30*2 - 10 = 62, Y = 218 + 20*2 - 12 = 246, Size = 80x50.
	if wf := app.Find("id:zchild").WindowFrame(); wf != (core.Bounds{Origin: core.Point{X: 62, Y: 246}, Size: core.Size{Width: 80, Height: 50}}) {
		t.Fatalf("zoomed zchild WindowFrame = %+v, want {62,246 80x50}", wf)
	}
}

func TestScrollIntoViewTargetAndSurface(t *testing.T) {
	t.Run("target loops through capped gestures and skips when visible", func(t *testing.T) {
		app := Start(t, func(*core.Scope) core.Node {
			return core.Scroll{
				TestID: "page",
				Style:  core.Style{Width: core.Pt(240), Height: core.Pt(200)},
				Children: []core.Node{
					core.View{TestID: "top", Style: core.Style{Height: core.Pt(40)}},
					core.View{Style: core.Style{Height: core.Pt(600)}},
					core.View{TestID: "bottom", Style: core.Style{Height: core.Pt(40)}},
				},
			}
		}, Size(240, 200))

		scroll := app.Find("id:page")
		var calls int
		surf := revealSurface{scroll: func(_, delta driver.Point) {
			calls++
			// Simulate macOS integer truncation per scroll call.
			stepY := driver.Points(math.Trunc(float64(delta.Y)))
			scroll.Offset = scroll.clamp(driver.Point{X: scroll.Offset.X, Y: scroll.Offset.Y + stepY})
		}}
		app.target = &target{a: app, surface: surf}
		defer func() { app.target = nil }()

		// Already visible: zero surface.Scroll calls.
		app.ScrollIntoView("id:top")
		if calls != 0 {
			t.Fatalf("already-visible target ScrollIntoView made %d Scroll calls, want 0", calls)
		}

		// Bottom item needs offset 480 across a 200pt viewport (capped at 160pt/gesture -> 3 calls).
		app.ScrollIntoView("id:bottom")
		if calls != 3 {
			t.Fatalf("target ScrollIntoView made %d calls, want 3", calls)
		}
		if got := scroll.Offset.Y; got != 480 {
			t.Fatalf("target ScrollIntoView offset.Y = %v, want 480", got)
		}
	})

	t.Run("target compensates for touch slop on long and short scrolls", func(t *testing.T) {
		const slop driver.Points = 8
		app := Start(t, func(*core.Scope) core.Node {
			return core.Scroll{
				TestID: "page",
				Style:  core.Style{Width: core.Pt(240), Height: core.Pt(200)},
				Children: []core.Node{
					core.View{TestID: "top", Style: core.Style{Height: core.Pt(40)}},
					core.View{Style: core.Style{Height: core.Pt(166)}},
					core.View{TestID: "edge", Style: core.Style{Height: core.Pt(40)}},
					core.View{Style: core.Style{Height: core.Pt(600)}},
					core.View{TestID: "bottom", Style: core.Style{Height: core.Pt(40)}},
				},
			}
		}, Size(240, 200))

		scroll := app.Find("id:page")
		applySlop := func(d driver.Points) driver.Points {
			if math.Abs(float64(d)) <= float64(slop) {
				return 0
			}
			if d > 0 {
				return d - slop
			}
			return d + slop
		}
		var calls int
		surf := revealSurface{scroll: func(_, delta driver.Point) {
			calls++
			scroll.Offset = scroll.clamp(driver.Point{
				X: scroll.Offset.X + applySlop(delta.X),
				Y: scroll.Offset.Y + applySlop(delta.Y),
			})
		}}
		app.target = &target{a: app, surface: surf}
		defer func() { app.target = nil }()

		// Long scroll to bottom (content Y=[846, 886], needs offset 686): must not stop 8pt short.
		app.ScrollIntoView("id:bottom")
		if wf := app.Find("id:bottom").WindowFrame(); wf.Origin.Y < 0 || wf.Origin.Y+wf.Size.Height > 200 {
			t.Fatalf("bottom WindowFrame %+v not fully inside [0, 200] (offset=%v)", wf, scroll.Offset.Y)
		}

		// Short initial scroll smaller than touch slop: edge is at Y=[206, 246], only 6pt past bottom at offset 40.
		scroll.Offset = driver.Point{Y: 40}
		calls = 0
		app.ScrollIntoView("id:edge")
		if wf := app.Find("id:edge").WindowFrame(); wf.Origin.Y < 0 || wf.Origin.Y+wf.Size.Height > 200 {
			t.Fatalf("edge WindowFrame %+v not fully inside [0, 200] after short slop scroll (offset=%v calls=%d)", wf, scroll.Offset.Y, calls)
		}

		// Unmovable native scroll view exits promptly after probing slop (never spins 30 times).
		scroll.Offset = driver.Point{}
		calls = 0
		app.target = &target{a: app, surface: revealSurface{scroll: func(_, _ driver.Point) { calls++ }}}
		app.ScrollIntoView("id:bottom")
		if calls > 4 {
			t.Fatalf("stuck native scroll made %d calls, want <= 4", calls)
		}
	})

	t.Run("Surface.ScrollIntoView delegates to secondary window", func(t *testing.T) {
		app := StartApp(t, func(cx *core.Scope) {
			cx.SetMainView(func(*core.Scope) core.Node { return core.Text{Content: "main"} })
			cx.OpenWindow(core.WindowOptions{Title: "Settings", Width: 260, Height: 200}, func(*core.Scope) core.Node {
				return core.Scroll{
					TestID: "settings-scroll",
					Style:  core.Style{Width: core.Pt(260), Height: core.Pt(200)},
					Children: []core.Node{
						core.View{Style: core.Style{Height: core.Pt(500)}},
						core.View{TestID: "reset", Style: core.Style{Height: core.Pt(40)}},
					},
				}
			})
		}, Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))

		win := app.Surface("title:Settings")
		win.ScrollIntoView("id:reset")
		if got := win.Find("id:settings-scroll").Offset.Y; got != 340 {
			t.Fatalf("secondary window offset.Y = %v, want 340", got)
		}
	})
}
