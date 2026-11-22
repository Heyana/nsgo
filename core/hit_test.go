package core

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestRefHitTestFollowsClippingTransformsStackingAndDisabledAncestors(t *testing.T) {
	target, cover := new(Ref), new(Ref)
	var disabled *Signal[bool]
	var covering *Signal[bool]
	h := newHost(t, Size{Width: 300, Height: 200}, func(cx *Scope) Node {
		disabled, covering = NewSignal(cx, false), NewSignal(cx, false)
		return View{Style: Style{Width: Pt(100), Height: Pt(100), Overflow: Hidden}, Disabled: disabled.Get,
			Children: []Node{
				View{Ref: target, Style: Style{Width: Pt(100), Height: Pt(100), Transform: Transform{TranslateX: 50}}},
				Show{When: covering.Get, Then: func(*Scope) Node {
					return View{Ref: cover, Style: Style{Position: Absolute, Left: Pt(0), Top: Pt(0), Width: Pt(100), Height: Pt(100)}}
				}},
			}}
	})
	h.commit()
	if local, ok := target.HitTest(Point{X: 60, Y: 20}); !ok || local != (Point{X: 10, Y: 20}) {
		t.Fatalf("transformed target = %v, %t", local, ok)
	}
	if _, ok := target.HitTest(Point{X: 120, Y: 20}); ok {
		t.Fatal("target escaped its clipping parent")
	}
	covering.Set(true)
	h.commit()
	if _, ok := target.HitTest(Point{X: 60, Y: 20}); ok {
		t.Fatal("target received a hit through a covering sibling")
	}
	if _, ok := cover.HitTest(Point{X: 60, Y: 20}); !ok {
		t.Fatal("frontmost view did not receive the hit")
	}
	disabled.Set(true)
	h.commit()
	if _, ok := cover.HitTest(Point{X: 60, Y: 20}); ok {
		t.Fatal("a disabled ancestor allowed a hit")
	}
}

func TestRefHitTestRespectsPointerEventsAndModalBoundaries(t *testing.T) {
	background, cover, modal := new(Ref), new(Ref), new(Ref)
	var shown *Signal[bool]
	h := newHost(t, Size{Width: 300, Height: 200}, func(cx *Scope) Node {
		shown = NewSignal(cx, false)
		return View{Children: []Node{
			View{Ref: background, Style: Style{Width: Pt(100), Height: Pt(100)}},
			Overlay{PointerEvents: PointerEventsNone, Children: []Node{
				View{Ref: cover, Style: Style{Width: Pt(100), Height: Pt(100)}},
			}},
			Show{When: shown.Get, Then: func(*Scope) Node {
				return Modal{Children: []Node{View{Ref: modal, Style: Style{Width: Pt(100), Height: Pt(100)}}}}
			}},
		}}
	})
	h.commit()
	at := Point{X: 20, Y: 20}
	if _, ok := background.HitTest(at); !ok {
		t.Fatal("noninteractive overlay blocked the background")
	}
	if _, ok := cover.HitTest(at); ok {
		t.Fatal("pointer-events-none overlay received a hit")
	}
	shown.Set(true)
	h.commit()
	if _, ok := background.HitTest(at); ok {
		t.Fatal("background received a hit below a modal")
	}
	if _, ok := modal.HitTest(at); !ok {
		t.Fatal("modal content did not receive a hit")
	}
}

func TestRefHitTestTracksScrollAndRejectsOtherSurfacesAndDeadRefs(t *testing.T) {
	scroll, outer, inner, foreign := new(Ref), new(Ref), new(Ref), new(Ref)
	var visible *Signal[bool]
	h := newHost(t, Size{Width: 300, Height: 200}, func(cx *Scope) Node {
		visible = NewSignal(cx, true)
		return Show{When: visible.Get, Then: func(*Scope) Node {
			return Scroll{Ref: scroll, Style: Style{Width: Pt(100), Height: Pt(100)}, Children: []Node{
				View{Style: Style{Height: Pt(100)}},
				View{Ref: outer, Style: Style{Height: Pt(100)}, Children: []Node{
					View{Ref: inner, Style: Style{Width: Pt(100), Height: Pt(100)}},
				}},
			}}
		}}
	})
	h.commit()
	at := Point{X: 20, Y: 20}
	if _, ok := inner.HitTest(at); ok {
		t.Fatal("offscreen content received a hit")
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventScroll, ID: scroll.n.id, Point: Point{Y: 100}})
	h.commit()
	other := newHost(t, Size{Width: 300, Height: 200}, func(*Scope) Node { return View{Ref: foreign} })
	other.commit()
	hits := HitTest(at, nil, new(Ref), outer, foreign, inner)
	if len(hits) != 2 || hits[0].Ref != inner || hits[1].Ref != outer || hits[0].Position != at {
		t.Fatalf("scrolled hits = %+v", hits)
	}
	visible.Set(false)
	h.commit()
	if _, ok := inner.HitTest(at); ok {
		t.Fatal("removed reference received a hit")
	}
}
