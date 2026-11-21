package core

import (
	"fmt"
	"image"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestResizeModeFit(t *testing.T) {
	box := Size{Width: 100, Height: 50}
	end := ObjectPosition{X: PositionEnd, Y: PositionEnd}
	for _, c := range []struct {
		name    string
		mode    ResizeMode
		natural Size
		pos     ObjectPosition
		want    Bounds
	}{
		{"cover", Cover, Size{Width: 20, Height: 20}, ObjectPosition{}, Bounds{Origin: Point{X: 0, Y: -25}, Size: Size{Width: 100, Height: 100}}},
		{"cover at the top", Cover, Size{Width: 20, Height: 20}, ObjectPosition{Y: PositionStart}, Bounds{Origin: Point{X: 0, Y: 0}, Size: Size{Width: 100, Height: 100}}},
		{"contain", Contain, Size{Width: 20, Height: 20}, ObjectPosition{}, Bounds{Origin: Point{X: 25, Y: 0}, Size: Size{Width: 50, Height: 50}}},
		{"contain at the end", Contain, Size{Width: 20, Height: 20}, end, Bounds{Origin: Point{X: 50, Y: 0}, Size: Size{Width: 50, Height: 50}}},
		{"stretch", Stretch, Size{Width: 20, Height: 20}, end, Bounds{Size: box}},
		{"center keeps a small size", Center, Size{Width: 20, Height: 10}, ObjectPosition{}, Bounds{Origin: Point{X: 40, Y: 20}, Size: Size{Width: 20, Height: 10}}},
		{"center keeps a large size", Center, Size{Width: 200, Height: 100}, ObjectPosition{}, Bounds{Origin: Point{X: -50, Y: -25}, Size: Size{Width: 200, Height: 100}}},
		{"scale-down keeps a small size", ScaleDown, Size{Width: 20, Height: 10}, ObjectPosition{}, Bounds{Origin: Point{X: 40, Y: 20}, Size: Size{Width: 20, Height: 10}}},
		{"scale-down shrinks a large one", ScaleDown, Size{Width: 200, Height: 200}, ObjectPosition{}, Bounds{Origin: Point{X: 25, Y: 0}, Size: Size{Width: 50, Height: 50}}},
		{"points from the end", Center, Size{Width: 20, Height: 10},
			ObjectPosition{X: PositionAxis{Offset: Pt(10), FromEnd: true}, Y: PositionAxis{Offset: Pt(5)}},
			Bounds{Origin: Point{X: 70, Y: 5}, Size: Size{Width: 20, Height: 10}}},
		{"percent", Center, Size{Width: 20, Height: 10}, ObjectPosition{X: PositionAxis{Offset: Pct(25)}}, Bounds{Origin: Point{X: 20, Y: 20}, Size: Size{Width: 20, Height: 10}}},
		{"no natural size", Contain, Size{}, end, Bounds{Size: box}},
	} {
		if got := c.mode.Fit(c.natural, box, c.pos); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestObjectPositionReachesTheImageItemOnly(t *testing.T) {
	var top *Signal[bool]
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
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
		case driver.ItemImage:
			images++
			if it.Image.Position.Y != PositionStart {
				t.Errorf("position = %+v, want the top", it.Image.Position)
			}
		case driver.ItemViewStyle:
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
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
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
	var asks []driver.MountItem
	for _, it := range b.Items {
		if it.Kind == driver.ItemImageInfo {
			asks = append(asks, it)
		}
	}
	if len(asks) != 2 || asks[0].Image.Source.URI != "asset:/a@2x.png" || asks[1].Image.Source.URI != "https://x/missing.png" {
		t.Fatalf("asks = %+v, want the @2x variant then the URL", asks)
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventImageInfo, Count: asks[1].Count, Text: "404"})
	h.tree.Dispatch(driver.Event{Kind: driver.EventImageInfo, Count: asks[0].Count, Size: Size{Width: 80, Height: 40}})
	h.tree.Dispatch(driver.Event{Kind: driver.EventImageInfo, Count: asks[0].Count, Size: Size{Width: 1, Height: 1}}) // answered already
	want := []string{"prefetched false", fmt.Sprint("size ", Size{Width: 80, Height: 40}, " <nil>")}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// pixelsItem returns the image item b sends for id, or nil.
func pixelsItem(b *driver.MountBatch, id driver.NodeID) *driver.ImageConfig {
	for _, it := range b.Items {
		if it.Kind == driver.ItemImage && it.ID == id {
			return it.Image
		}
	}
	return nil
}

func fillPixels(img *image.RGBA, v byte) {
	for i := range img.Pix {
		img.Pix[i] = v
	}
}

func TestPixelsAreCopiedOncePerVersion(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 4, 2))
	fillPixels(frame, 10)
	var version *Signal[uint64]
	var top *Signal[bool]
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		version = NewSignal(cx, uint64(1))
		top = NewSignal(cx, false)
		return Image{TestID: "frame", SourceFunc: func() ImageSource {
			return ImageSource{Pixels: frame, Version: version.Get()}
		}, StyleFunc: func() Style {
			var s Style
			if top.Get() {
				s.ObjectPosition.Y = PositionStart
			}
			return s
		}}
	})
	b := h.commit()
	id := h.find("frame").id
	first := pixelsItem(b, id)
	if first == nil || first.Source.Pixels == nil || first.Source.Pixels == frame || first.Source.Version != 1 {
		t.Fatalf("first item = %+v, want a copy of the frame at version 1", first)
	}
	if got := first.Source.Pixels.Pix[0]; got != 10 {
		t.Fatalf("copied pixel = %d, want 10", got)
	}
	// The app writes its next frame into the same buffer once the commit ran.
	fillPixels(frame, 20)
	if got := first.Source.Pixels.Pix[0]; got != 10 {
		t.Fatalf("the sent frame changed to %d with the app's buffer", got)
	}
	version.Set(1)
	if it := pixelsItem(h.commit(), id); it != nil {
		t.Fatal("the same version was sent again")
	}
	top.Set(true)
	it := pixelsItem(h.commit(), id)
	if it == nil || it.Source.Pixels != first.Source.Pixels {
		t.Fatalf("a new position sent %+v, want the same copy", it)
	}
	version.Set(2)
	it = pixelsItem(h.commit(), id)
	if it == nil || it.Source.Version != 2 || it.Source.Pixels == first.Source.Pixels || it.Source.Pixels.Pix[0] != 20 {
		t.Fatalf("version 2 sent %+v, want a new copy of the frame", it)
	}
}

func TestPixelsMeasureTheirPixels(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 64, 32))
	var src *Signal[ImageSource]
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		src = NewSignalFunc(cx, ImageSource{Pixels: frame}, nil)
		return View{Class: "items-start", Children: []Node{Image{TestID: "frame", SourceFunc: src.Get}}}
	})
	h.commit()
	if got := h.find("frame").frame.Size; got != (Size{Width: 64, Height: 32}) {
		t.Fatalf("size = %v, want the pixels' 64 by 32", got)
	}
	src.Set(ImageSource{Pixels: frame, Size: Size{Width: 32, Height: 16}})
	h.commit()
	if got := h.find("frame").frame.Size; got != (Size{Width: 32, Height: 16}) {
		t.Fatalf("size = %v, want the declared 32 by 16", got)
	}
}

func TestPixelsCopyASubImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := range img.Pix {
		img.Pix[i] = byte(i)
	}
	sub := img.SubImage(image.Rect(1, 2, 3, 4)).(*image.RGBA)
	c := driver.CopyPixels(sub)
	if c.Rect != image.Rect(0, 0, 2, 2) || c.Stride != 8 {
		t.Fatalf("copy is %v with stride %d, want 2 by 2 at the origin, tight", c.Rect, c.Stride)
	}
	for y := range 2 {
		for x := range 2 {
			if got, want := c.RGBAAt(x, y), sub.RGBAAt(1+x, 2+y); got != want {
				t.Fatalf("pixel %d,%d = %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestPixelsFireNoLoadEvents(t *testing.T) {
	var events []string
	frame := image.NewRGBA(image.Rect(0, 0, 2, 2))
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		return Image{TestID: "frame", Source: ImageSource{Pixels: frame},
			OnLoadStart: func() { events = append(events, "start") },
			OnLoad:      func(Size) { events = append(events, "load") },
			OnLoadEnd:   func() { events = append(events, "end") }}
	})
	h.commit()
	id := h.find("frame").id
	for _, k := range []driver.EventKind{driver.EventLoadStart, driver.EventLoad, driver.EventLoadEnd} {
		h.tree.Dispatch(driver.Event{Kind: k, ID: id})
	}
	if len(events) != 0 {
		t.Fatalf("events = %v, want none for pixels", events)
	}
}

func TestCanvasCopiesThePixelsItDraws(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 2, 2))
	fillPixels(frame, 1)
	var version, width *Signal[uint64]
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		version = NewSignal(cx, uint64(1))
		width = NewSignal(cx, uint64(10))
		return Canvas{Style: Style{Height: Pt(40)}, Draw: func(d *Drawing, size Size) {
			src := ImageSource{Pixels: frame, Version: version.Get()}
			d.DrawImageIn(src, 0, 0, float32(width.Get()), 10)
		}}
	})
	sent := func(b *driver.MountBatch) *ImageSource {
		for _, it := range b.Items {
			if it.Kind == driver.ItemCanvas {
				for _, op := range it.Drawing.Ops {
					if op.Kind == driver.DrawImage {
						return op.Image
					}
				}
			}
		}
		return nil
	}
	first := sent(h.commit())
	if first == nil || first.Pixels == nil || first.Pixels == frame || first.Pixels.Pix[0] != 1 {
		t.Fatalf("drew %+v, want a copy of the frame", first)
	}
	width.Set(20)
	if again := sent(h.commit()); again == nil || again.Pixels != first.Pixels {
		t.Fatalf("a redraw at the same version drew %+v, want the same copy", again)
	}
	fillPixels(frame, 2)
	version.Set(2)
	if next := sent(h.commit()); next == nil || next.Pixels == first.Pixels || next.Pixels.Pix[0] != 2 || first.Pixels.Pix[0] != 1 {
		t.Fatalf("version 2 drew %+v, want a new copy", next)
	}
}

func TestImageSizeAnswersPixelsAtOnce(t *testing.T) {
	var got []string
	var root *Scope
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		root = cx
		return View{}
	})
	h.commit()
	frame := image.NewRGBA(image.Rect(0, 0, 64, 32))
	root.ImageSize(ImageSource{Pixels: frame, Size: Size{Width: 32, Height: 16}}, func(px Size, err error) {
		got = append(got, fmt.Sprint("size ", px, " ", err))
	})
	root.PrefetchImage(ImageSource{Pixels: frame}, func(ok bool) { got = append(got, fmt.Sprint("prefetched ", ok)) })
	want := []string{fmt.Sprint("size ", Size{Width: 64, Height: 32}, " <nil>"), "prefetched true"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for _, it := range h.commit().Items {
		if it.Kind == driver.ItemImageInfo {
			t.Fatalf("asked the platform for pixels: %+v", it)
		}
	}
}

func TestShaderImagesCopyPixelsOncePerVersion(t *testing.T) {
	prog := MustCompileShader(testShader)
	frame := image.NewRGBA(image.Rect(0, 0, 4, 4))
	fillPixels(frame, 10)
	var version *Signal[uint64]
	var count *Signal[int]
	var empty *Signal[bool]
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		version = NewSignal(cx, uint64(1))
		count = NewSignal(cx, 1)
		empty = NewSignal(cx, false)
		return Shader{Program: prog, UniformsFunc: func() Uniforms { return Uniforms{"Count": count.Get()} },
			ImagesFunc: func() []ImageSource {
				px := frame
				if empty.Get() {
					px = image.NewRGBA(image.Rectangle{})
				}
				return []ImageSource{{Pixels: px, Version: version.Get()}, Src("https://x/a.png")}
			}}
	})
	first := shaderItem(h.commit())
	if first == nil || len(first.Images) != 2 || first.Images[0].Pixels == nil || first.Images[0].Pixels == frame ||
		first.Images[0].Version != 1 || first.Images[1].URI != "https://x/a.png" || first.Images[1].Pixels != nil {
		t.Fatalf("shader = %+v, want a copy of the frame at version 1 and the URL", first)
	}
	copied := first.Images[0].Pixels
	// The app writes its next frame into the same buffer once the commit ran.
	fillPixels(frame, 20)
	if copied.Pix[0] != 10 {
		t.Fatalf("the sent frame changed to %d with the app's buffer", copied.Pix[0])
	}
	version.Set(1)
	if sh := shaderItem(h.commit()); sh != nil {
		t.Fatalf("the same version sent %+v", sh)
	}
	count.Set(2)
	if sh := shaderItem(h.commit()); sh == nil || sh.Images[0].Pixels != copied {
		t.Fatalf("a new uniform sent %+v, want the same copy, which uploads nothing", sh)
	}
	version.Set(2)
	sh := shaderItem(h.commit())
	if sh == nil || sh.Images[0].Version != 2 || sh.Images[0].Pixels == copied || sh.Images[0].Pixels.Pix[0] != 20 || copied.Pix[0] != 10 {
		t.Fatalf("version 2 sent %+v, want a new copy of the frame", sh)
	}
	empty.Set(true)
	if sh := shaderItem(h.commit()); sh == nil || !sh.Images[0].Empty() {
		t.Fatalf("an empty buffer sent %+v, want no pixels", sh)
	}
}

func TestPixelsTakeNoPlaceholderAndEmptyBuffersNoCopy(t *testing.T) {
	var src *Signal[ImageSource]
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		src = NewSignalFunc(cx, ImageSource{Pixels: image.NewRGBA(image.Rect(0, 0, 2, 2)), Version: 1}, nil)
		return Image{TestID: "frame", SourceFunc: src.Get, Placeholder: Src("https://x/placeholder.png")}
	})
	b := h.commit()
	id := h.find("frame").id
	if it := pixelsItem(b, id); it == nil || it.Source.Pixels == nil || !it.Placeholder.Empty() {
		t.Fatalf("item = %+v, want the pixels without the placeholder", it)
	}
	src.Set(ImageSource{Pixels: image.NewRGBA(image.Rectangle{}), Version: 2})
	if it := pixelsItem(h.commit(), id); it == nil || !it.Source.Empty() || !it.Placeholder.Empty() {
		t.Fatalf("an empty buffer sent %+v, want no pixels", it)
	}
	src.Set(Src("https://x/a.png"))
	if it := pixelsItem(h.commit(), id); it == nil || it.Placeholder.URI != "https://x/placeholder.png" {
		t.Fatalf("a URI source sent %+v, want its placeholder back", it)
	}
}

func TestCanvasDrawsNoEmptyBuffer(t *testing.T) {
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		return Canvas{Style: Style{Height: Pt(40)}, Draw: func(d *Drawing, size Size) {
			d.DrawImageIn(ImageSource{Pixels: image.NewRGBA(image.Rectangle{}), Version: 1}, 0, 0, 10, 10)
		}}
	})
	for _, it := range h.commit().Items {
		if it.Kind != driver.ItemCanvas {
			continue
		}
		for _, op := range it.Drawing.Ops {
			if op.Kind == driver.DrawImage && op.Image != nil {
				t.Fatalf("drew %+v, want no image for an empty buffer", op.Image)
			}
		}
		return
	}
	t.Fatal("no canvas was sent")
}

func TestKeepImageLifecycleAndScales(t *testing.T) {
	var root *Scope
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		root = cx
		return View{}
	})
	h.commit()
	h.tree.SetEnvironment(Environment{PixelRatio: 2})

	// Empty and Pixels sources are no-ops.
	root.KeepImage(ImageSource{})()
	root.KeepImage(ImageSource{Pixels: image.NewRGBA(image.Rect(0, 0, 2, 2))})()
	if b := h.commit(); command(b, driver.ItemKeepImage) != nil || command(b, driver.ItemReleaseImage) != nil {
		t.Fatalf("empty/pixels sources sent keep commands: %+v", b.Items)
	}

	// Scaled asset resolves its key at the screen's pixel ratio, and release is idempotent.
	src := ImageSource{URI: "asset:/atlas.png", Scales: ScalesOf(1, 2)}
	release := root.KeepImage(src)
	it := command(h.commit(), driver.ItemKeepImage)
	if it == nil || it.Value != "asset:/atlas@2x.png" {
		t.Fatalf("keep = %+v, want asset:/atlas@2x.png", it)
	}
	release()
	release()
	h.drain()
	if it = command(h.commit(), driver.ItemReleaseImage); it == nil || it.Value != "asset:/atlas@2x.png" {
		t.Fatalf("release = %+v, want asset:/atlas@2x.png", it)
	}
	if command(h.commit(), driver.ItemReleaseImage) != nil {
		t.Fatal("release was sent twice")
	}

	// Disposing a scope releases its pin, and releasing right before dispose sends only one release.
	child := root.Child()
	dataSrc := ImageSource{Data: []byte("atlas-bytes")}
	wantKey := dataSrc.Key()
	relChild := child.KeepImage(dataSrc)
	if it = command(h.commit(), driver.ItemKeepImage); it == nil || it.Value != wantKey {
		t.Fatalf("data keep = %+v, want %q", it, wantKey)
	}
	relChild()
	child.Dispose()
	h.drain()
	b := h.commit()
	var releases []string
	for _, item := range b.Items {
		if item.Kind == driver.ItemReleaseImage {
			releases = append(releases, item.Value)
		}
	}
	if !slices.Equal(releases, []string{wantKey}) {
		t.Fatalf("releases = %q, want [%q]", releases, wantKey)
	}
	relChild()
	h.drain()
	if command(h.commit(), driver.ItemReleaseImage) != nil {
		t.Fatal("release after dispose sent a second release")
	}

	// Two scopes keeping the same source each emit their own keep and release.
	a, c2 := root.Child(), root.Child()
	a.KeepImage(Src("https://x/sheet.png"))
	c2.KeepImage(Src("https://x/sheet.png"))
	b = h.commit()
	var keeps []string
	for _, item := range b.Items {
		if item.Kind == driver.ItemKeepImage {
			keeps = append(keeps, item.Value)
		}
	}
	if !slices.Equal(keeps, []string{"https://x/sheet.png", "https://x/sheet.png"}) {
		t.Fatalf("keeps = %q, want two pins", keeps)
	}
	a.Dispose()
	if it = command(h.commit(), driver.ItemReleaseImage); it == nil || it.Value != "https://x/sheet.png" {
		t.Fatalf("first dispose release = %+v", it)
	}
	c2.Dispose()
	if it = command(h.commit(), driver.ItemReleaseImage); it == nil || it.Value != "https://x/sheet.png" {
		t.Fatalf("second dispose release = %+v", it)
	}

	// Restricted code without AccessNetwork cannot pin a remote URI.
	held := root.Child()
	held.Run(func() {
		held.ProvideAccess(allows(func(AccessRequest) bool { return false }))
		held.KeepImage(Src("https://x/blocked.png"))
	})
	if it = command(h.commit(), driver.ItemKeepImage); it != nil {
		t.Fatalf("denied remote source emitted %+v", it)
	}
}
