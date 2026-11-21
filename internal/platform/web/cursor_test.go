//go:build js && wasm

package web

import (
	"encoding/base64"
	goimage "image"
	"image/color"
	"image/png"
	"strings"
	"syscall/js"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestPixelCursorEncodesItsSubimage(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("run with nsgo test -target web")
	}
	parent := goimage.NewRGBA(goimage.Rect(0, 0, 4, 4))
	parent.SetRGBA(2, 2, color.RGBA{R: 128, A: 128})
	pixels := parent.SubImage(goimage.Rect(1, 1, 3, 3)).(*goimage.RGBA)
	s := &surface{doc: doc}
	src := driver.ImageSource{Pixels: pixels}
	cursor := s.cursorURL(src)
	data, ok := strings.CutPrefix(cursor, "data:image/png;base64,")
	if !ok {
		t.Fatal("pixel cursor did not produce a PNG data URL")
	}
	decoded, err := png.Decode(base64.NewDecoder(base64.StdEncoding, strings.NewReader(data)))
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.Bounds(); got != goimage.Rect(0, 0, 2, 2) {
		t.Fatalf("cursor bounds = %v, want 2 × 2", got)
	}
	if got, want := color.NRGBAModel.Convert(decoded.At(1, 1)), (color.NRGBA{R: 255, A: 128}); got != want {
		t.Fatalf("cursor pixel = %v, want %v", got, want)
	}
	if got := s.cursorURL(src); got != cursor || len(s.cursorURLs) != 1 {
		t.Fatal("the same source did not reuse its cached URL")
	}
}
