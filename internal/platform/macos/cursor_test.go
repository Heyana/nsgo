//go:build darwin && !ios

package macos

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// TestDiagonalCursors asks AppKit for the frame cursors with the values
// NSCursor.h gives them.
func TestDiagonalCursors(t *testing.T) {
	if _, err := purego.Dlopen(appKitPath, purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		t.Fatal(err)
	}
	pointer := nsCursor(driver.CursorPointer)
	nwse, nesw := nsCursor(driver.CursorNwseResize), nsCursor(driver.CursorNeswResize)
	if nwse == 0 || nesw == 0 || nwse == nesw || nwse == pointer || nesw == pointer {
		t.Errorf("pointer %v, nwse %v, nesw %v", pointer, nwse, nesw)
	}
}

// TestSpanCursorGoesAheadOfItsText finds a span's cursor over its characters, and the text's own cursor over the others.
func TestSpanCursorGoesAheadOfItsText(t *testing.T) {
	onMain(func() { spanCursorGoesAheadOfItsText(t) })
}

func spanCursorGoesAheadOfItsText(t *testing.T) {
	loadAppKit()
	style := driver.TextStyle{Font: driver.Font{Size: 20}, Color: driver.RGB(0)}
	at := &driver.AttributedText{Text: "plain link", Style: style, Spans: []driver.TextSpan{
		{Start: 0, End: 6, Style: style},
		{Start: 6, End: 10, Style: style, Cursor: driver.CursorPointer},
	}}
	p := apple.TextEngine{}.Prepare(at)
	defer p.Release()
	size := p.Measure(driver.Auto)
	l := p.Layout(size).(*apple.Layout)
	defer l.Release()

	up := objc.ID(classView).Send(selNew)
	defer up.Send(selRelease)
	up.Send(selSetFrame, apple.Rect(0, 0, 400, 100))
	n := objc.ID(classView).Send(selNew)
	defer n.Send(selRelease)
	const left, top = 10, 20
	n.Send(selSetFrame, apple.Rect(left, top, float64(size.Width), float64(size.Height)))
	up.Send(selAddSubview, n)
	v := &view{kind: driver.KindText, native: n, content: n, style: driver.DefaultViewStyle, layout: l,
		box: driver.Bounds{Size: size}, spans: at.Spans}
	v.style.Cursor = driver.CursorHelp
	views[n] = v
	defer delete(views, n)

	// The window's point of (x, middle) in the text, NsgoView being flipped.
	cursorOver := func(x float64) (driver.Cursor, bool) {
		loc := objc.Send[apple.CGPoint](n, sel("convertPoint:toView:"), apple.CGPoint{X: x, Y: float64(size.Height) / 2}, objc.ID(0))
		c, _, own := cursorAt(n, up, loc)
		return c, own
	}
	if c, own := cursorOver(2); c != driver.CursorHelp || !own {
		t.Errorf("over the plain span: %v, %v; want the text's help cursor", c, own)
	}
	if c, own := cursorOver(float64(size.Width) - 2); c != driver.CursorPointer || !own {
		t.Errorf("over the link: %v, %v; want the span's pointer", c, own)
	}
	v.style.Cursor = driver.CursorAuto
	if c, own := cursorOver(2); c != driver.CursorAuto || !own {
		t.Errorf("over the plain span of a text with no cursor: %v, %v; want auto", c, own)
	}
}

func makeCursorPNG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestCustomCursorRetinaRepsAndCache builds an NSCursor with both 1x and 2x
// NSBitmapImageReps, verifies clamped hotspot and NSImage representations, and
// confirms repeated lookups return the cached NSCursor.
func TestCustomCursorRetinaRepsAndCache(t *testing.T) {
	onMain(func() {
		loadAppKit()
		defer clearCustomCursorCache()

		png1x := makeCursorPNG(t, 16, 16, color.RGBA{R: 255, A: 255})
		png2x := makeCursorPNG(t, 32, 32, color.RGBA{G: 255, A: 255})
		cc := &driver.CustomCursor{
			Image:    driver.ImageSource{Data: png1x},
			Image2x:  driver.ImageSource{Data: png2x},
			Hotspot:  driver.Point{X: 40, Y: -2}, // clamps to (15, 0)
			Fallback: driver.CursorCrosshair,
		}
		var s surface
		cur := s.customNSCursor(cc)
		if cur == 0 {
			t.Fatal("customNSCursor returned 0")
		}
		if again := s.customNSCursor(cc); again != cur {
			t.Fatalf("cache miss on identical CustomCursor: %v != %v", again, cur)
		}
		hot := objc.Send[apple.CGPoint](cur, sel("hotSpot"))
		if hot.X != 15 || hot.Y != 0 {
			t.Errorf("hotSpot = (%v, %v), want (15, 0)", hot.X, hot.Y)
		}
		nsImg := cur.Send(sel("image"))
		sz := objc.Send[apple.CGSize](nsImg, sel("size"))
		if sz.Width != 16 || sz.Height != 16 {
			t.Errorf("NSImage size = (%v, %v), want (16, 16)", sz.Width, sz.Height)
		}
		reps := nsImg.Send(sel("representations"))
		if n := objc.Send[uint](reps, sel("count")); n != 2 {
			t.Fatalf("representations count = %d, want 2 (1x and 2x)", n)
		}

		// Pixels source works too.
		px := image.NewRGBA(image.Rect(0, 0, 12, 12))
		px.SetRGBA(0, 0, color.RGBA{B: 255, A: 255})
		pxCur := s.customNSCursor(&driver.CustomCursor{
			Image:   driver.ImageSource{Pixels: px, Version: 1},
			Hotspot: driver.Point{X: 6, Y: 6},
		})
		if pxCur == 0 {
			t.Fatal("customNSCursor(Pixels) returned 0")
		}

		// Invalid data returns 0 so caller uses Fallback.
		if bad := s.customNSCursor(&driver.CustomCursor{Image: driver.ImageSource{Data: []byte("not-an-image")}}); bad != 0 {
			t.Errorf("invalid image returned non-zero NSCursor %v", bad)
		}
	})
}
