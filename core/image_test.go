package core

import (
	"fmt"
	"slices"
	"testing"
)

func TestResizeModeFit(t *testing.T) {
	box := Size{100, 50}
	end := ObjectPosition{X: PositionEnd, Y: PositionEnd}
	for _, c := range []struct {
		name    string
		mode    ResizeMode
		natural Size
		pos     ObjectPosition
		want    Bounds
	}{
		{"cover", Cover, Size{20, 20}, ObjectPosition{}, Bounds{Point{0, -25}, Size{100, 100}}},
		{"cover at the top", Cover, Size{20, 20}, ObjectPosition{Y: PositionStart}, Bounds{Point{0, 0}, Size{100, 100}}},
		{"contain", Contain, Size{20, 20}, ObjectPosition{}, Bounds{Point{25, 0}, Size{50, 50}}},
		{"contain at the end", Contain, Size{20, 20}, end, Bounds{Point{50, 0}, Size{50, 50}}},
		{"stretch", Stretch, Size{20, 20}, end, Bounds{Size: box}},
		{"center keeps a small size", Center, Size{20, 10}, ObjectPosition{}, Bounds{Point{40, 20}, Size{20, 10}}},
		{"center keeps a large size", Center, Size{200, 100}, ObjectPosition{}, Bounds{Point{-50, -25}, Size{200, 100}}},
		{"scale-down keeps a small size", ScaleDown, Size{20, 10}, ObjectPosition{}, Bounds{Point{40, 20}, Size{20, 10}}},
		{"scale-down shrinks a large one", ScaleDown, Size{200, 200}, ObjectPosition{}, Bounds{Point{25, 0}, Size{50, 50}}},
		{"points from the end", Center, Size{20, 10},
			ObjectPosition{X: PositionAxis{Offset: Pt(10), FromEnd: true}, Y: PositionAxis{Offset: Pt(5)}},
			Bounds{Point{70, 5}, Size{20, 10}}},
		{"percent", Center, Size{20, 10}, ObjectPosition{X: PositionAxis{Offset: Pct(25)}}, Bounds{Point{20, 20}, Size{20, 10}}},
		{"no natural size", Contain, Size{}, end, Bounds{Size: box}},
	} {
		if got := c.mode.Fit(c.natural, box, c.pos); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestObjectPositionReachesTheImageItemOnly(t *testing.T) {
	var top *Signal[bool]
	h := newHost(t, Size{200, 200}, func(cx *Scope) Node {
		top = NewSignal(cx, false)
		return Image{Source: Src("https://x/y.png"), TestID: "image", StyleFunc: func() Style {
			s := Style{Width: Pt(100), Height: Pt(100)}
			if top.Get() {
				s.ObjectPosition.Y = PositionStart
			}
			return s
		}}
	})
	h.commit()
	top.Set(true)
	b := h.commit()
	id := h.find("image").id
	var images, styles int
	for _, it := range b.Items {
		if it.ID != id {
			continue
		}
		switch it.Kind {
		case ItemImage:
			images++
			if it.Image.Position.Y != PositionStart {
				t.Errorf("position = %+v, want the top", it.Image.Position)
			}
		case ItemViewStyle:
			styles++
		}
	}
	if images != 1 || styles != 0 {
		t.Errorf("sent %d images and %d view styles, want the image only", images, styles)
	}
}

func TestImageSizeAsksThePlatform(t *testing.T) {
	var got []string
	var root *Scope
	h := newHost(t, Size{200, 200}, func(cx *Scope) Node {
		root = cx
		return View{}
	})
	h.commit()
	h.tree.SetEnvironment(Environment{PixelRatio: 2})
	root.ImageSize(ImageSource{URI: "asset:/a.png", Scales: ScalesOf(1, 2)}, func(px Size, err error) {
		got = append(got, fmt.Sprint("size ", px, " ", err))
	})
	root.PrefetchImage(Src("https://x/missing.png"), func(ok bool) {
		got = append(got, fmt.Sprint("prefetched ", ok))
	})
	b := h.commit()
	var asks []MountItem
	for _, it := range b.Items {
		if it.Kind == ItemImageInfo {
			asks = append(asks, it)
		}
	}
	if len(asks) != 2 || asks[0].Image.Source.URI != "asset:/a@2x.png" || asks[1].Image.Source.URI != "https://x/missing.png" {
		t.Fatalf("asks = %+v, want the @2x variant then the URL", asks)
	}
	h.tree.Dispatch(Event{Kind: EventImageInfo, Count: asks[1].Count, Text: "404"})
	h.tree.Dispatch(Event{Kind: EventImageInfo, Count: asks[0].Count, Size: Size{80, 40}})
	h.tree.Dispatch(Event{Kind: EventImageInfo, Count: asks[0].Count, Size: Size{1, 1}}) // answered already
	want := []string{"prefetched false", fmt.Sprint("size ", Size{80, 40}, " <nil>")}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
