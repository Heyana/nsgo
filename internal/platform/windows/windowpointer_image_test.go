//go:build windows

package windows

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestPaintedWindowImageAlpha(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	for y := range 2 {
		for x := 2; x < 4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	pic, err := decode(context.Background(), data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	w := &window{scale: 1, root: 1, pointerMode: driver.WindowPointerPainted}
	w.create(1, driver.KindView)
	w.create(2, driver.KindImage)
	root, imageView := w.view(1), w.view(2)
	root.frame.Size = driver.Size{Width: 100, Height: 100}
	root.children = []*view{imageView}
	imageView.parent = root
	imageView.frame = root.frame
	imageView.source.pic = pic
	imageView.events = 1 << driver.EventPress
	imageView.image = driver.ImageConfig{ResizeMode: driver.Contain, HitTestAlpha: 0.5}

	transparent := driver.Point{X: 25, Y: 50}
	opaque := driver.Point{X: 75, Y: 50}
	letterbox := driver.Point{X: 75, Y: 10}
	for _, at := range []driver.Point{transparent, letterbox} {
		if w.paintedAt(at) {
			t.Errorf("painted window took the pointer at %+v", at)
		}
	}
	if !w.paintedAt(opaque) {
		t.Error("painted window let the pointer through an opaque pixel")
	}

	w.create(3, driver.KindView)
	below := w.view(3)
	below.frame, below.parent = root.frame, root
	below.events = 1 << driver.EventPress
	root.children = []*view{below, imageView}
	pressable := func(v *view) bool { return v.events.Has(driver.EventPress) }
	if got := hit(root, transparent, pressable); got != below {
		t.Errorf("transparent pixel hit %v, want the pressable sibling below", got)
	}
	imageView.style.Background = driver.RGB(0xff0000)
	if got := hit(root, transparent, pressable); got != imageView {
		t.Errorf("transparent pixel with a view background hit %v, want the image view", got)
	}
	imageView.style.Background = 0
	root.children = []*view{imageView}
	w.pointerMode = driver.WindowPointerAuto
	if got := hit(root, transparent, pressable); got != imageView {
		t.Errorf("ordinary window hit %v, want the image view", got)
	}
	w.pointerMode = driver.WindowPointerPainted
	imageView.image.HitTestAlpha = 0
	if !w.paintedAt(transparent) {
		t.Error("zero threshold let the pointer through the image view")
	}
}
