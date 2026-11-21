package webstyle

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func makePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDataURL(t *testing.T) {
	p := makePNG(t, 4, 4)
	if u := DataURL(p); !strings.HasPrefix(u, "data:image/png;base64,") {
		t.Errorf("DataURL(png) = %q, want data:image/png;base64,...", u)
	}
	if u := DataURL([]byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)); !strings.HasPrefix(u, "data:image/svg+xml;base64,") {
		t.Errorf("DataURL(svg) = %q, want data:image/svg+xml;base64,...", u)
	}
	if u := DataURL(nil); u != "" {
		t.Errorf("DataURL(nil) = %q, want empty", u)
	}
}

func TestCustomCursorDeclarations(t *testing.T) {
	if got := CustomCursorDeclarations(driver.CursorAuto, nil, "", "", 1); got != nil {
		t.Errorf("auto + nil = %v, want nil", got)
	}
	if got := CustomCursorDeclarations(driver.CursorPointer, nil, "", "", 1); len(got) != 1 || got[0] != "pointer" {
		t.Errorf("pointer + nil = %v, want [pointer]", got)
	}

	png1x := makePNG(t, 16, 16)
	png2x := makePNG(t, 32, 32)
	cc := &driver.CustomCursor{
		Image:    driver.ImageSource{Data: png1x},
		Image2x:  driver.ImageSource{Data: png2x},
		Hotspot:  driver.Point{X: 25, Y: -3}, // clamps to (15, 0)
		Fallback: driver.CursorCrosshair,
	}

	// 1x only
	got1x := CustomCursorDeclarations(driver.CursorAuto, cc, "data:image/png;base64,AAA", "", 1)
	if len(got1x) != 1 || got1x[0] != `url("data:image/png;base64,AAA") 15 0, crosshair` {
		t.Errorf("1x only = %q", got1x)
	}

	// 1x + 2x emits url() fallback + -webkit-image-set + image-set
	gotHiDPI := CustomCursorDeclarations(driver.CursorAuto, cc, "cur1x.png", "cur2x.png", 2)
	if len(gotHiDPI) != 3 {
		t.Fatalf("1x+2x len = %d (%q), want 3", len(gotHiDPI), gotHiDPI)
	}
	if gotHiDPI[0] != `url("cur1x.png") 15 0, crosshair` {
		t.Errorf("fallback decl = %q", gotHiDPI[0])
	}
	if gotHiDPI[1] != `-webkit-image-set(url("cur1x.png") 1x, url("cur2x.png") 2x) 15 0, crosshair` {
		t.Errorf("-webkit-image-set decl = %q", gotHiDPI[1])
	}
	if gotHiDPI[2] != `image-set(url("cur1x.png") 1x, url("cur2x.png") 2x) 15 0, crosshair` {
		t.Errorf("image-set decl = %q", gotHiDPI[2])
	}

	// Empty custom cursor falls back to Fallback keyword.
	empty := &driver.CustomCursor{Fallback: driver.CursorGrab}
	if got := CustomCursorDeclarations(driver.CursorAuto, empty, "", "", 1); len(got) != 1 || got[0] != "grab" {
		t.Errorf("empty with fallback = %q, want [grab]", got)
	}

	// Auto fallback becomes default in CSS url() declaration.
	autoFB := &driver.CustomCursor{Image: driver.ImageSource{Data: png1x}, Hotspot: driver.Point{X: 2, Y: 3}}
	if got := CustomCursorDeclarations(driver.CursorAuto, autoFB, "a.png", "", 1); len(got) != 1 || got[0] != `url("a.png") 2 3, default` {
		t.Errorf("auto fallback = %q, want default keyword", got)
	}
}
