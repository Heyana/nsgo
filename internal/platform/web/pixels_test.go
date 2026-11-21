//go:build js && wasm

package web

import (
	goimage "image"
	"image/color"
	"image/draw"
	"syscall/js"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWebPixelImageUpdatesInPlace(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("run with nsgo test -target web")
	}
	s := &surface{doc: doc, animator: driver.NewAnimator(nil)}
	v := s.create(1, driver.KindImage)
	v.size = driver.Size{Width: 960, Height: 540}
	doc.Get("body").Call("appendChild", v.el)
	t.Cleanup(func() {
		v.image.release()
		v.listeners.release()
		v.el.Call("remove")
	})
	source := &v.image.source
	c := driver.ImageConfig{ResizeMode: driver.Contain}

	tick := make(chan struct{}, 1)
	onFrame := js.FuncOf(func(js.Value, []js.Value) any {
		tick <- struct{}{}
		return nil
	})
	defer onFrame.Release()
	var canvas js.Value
	for version := range 120 {
		js.Global().Call("requestAnimationFrame", onFrame)
		<-tick
		rgba := color.RGBA{R: 255, A: 255}
		if version%2 != 0 {
			rgba = color.RGBA{G: 255, A: 255}
		}
		frame := goimage.NewRGBA(goimage.Rect(0, 0, 960, 540))
		draw.Draw(frame, frame.Bounds(), goimage.NewUniform(rgba), goimage.Point{}, draw.Src)
		c.Source = driver.ImageSource{Pixels: frame, Version: uint64(version)}
		s.setImage(v, &c)
		el := source.layer.Call("querySelector", "canvas")
		if !el.Truthy() {
			t.Fatalf("version %d has no canvas", version)
		}
		if version == 0 {
			canvas = el
		}
		if !el.Equal(canvas) || source.img.Call("hasAttribute", "src").Bool() {
			t.Fatalf("version %d replaced its canvas or started an image load", version)
		}
		if got := rgbaAt(canvas, 480, 270); got != rgba {
			t.Fatalf("version %d painted %v, want %v", version, got, rgba)
		}
	}

	p := goimage.NewRGBA(goimage.Rect(0, 0, 20, 10))
	p.SetRGBA(4, 3, color.RGBA{R: 128, A: 128})
	c.Source = driver.ImageSource{Pixels: p}
	c.ResizeMode = driver.Center
	c.BlurRadius = 2
	s.setImage(v, &c)
	if source.bitmap.el.Get("width").Int() != 20 || source.bitmap.el.Get("height").Int() != 10 ||
		canvas.Get("style").Get("left").String() != "470px" || canvas.Get("style").Get("top").String() != "265px" ||
		source.layer.Get("style").Get("filter").String() != "blur(2px)" {
		t.Fatal("resized canvas did not keep the image's fit or blur")
	}
	if got, want := rgbaAt(canvas, 4, 3), (color.RGBA{R: 255, A: 128}); got != want {
		t.Fatalf("translucent pixel = %v, want %v", got, want)
	}
	v.image.tint = driver.RGBA(0x0000ff, .5)
	v.image.draw(v.size)
	if got, want := rgbaAt(canvas, 4, 3), (color.RGBA{B: 255, A: 64}); got != want {
		t.Fatalf("tinted pixel = %v, want %v", got, want)
	}
	drawing := s.canvasFrame(c.Source)
	if got, want := rgbaAt(drawing.img, 4, 3), (color.RGBA{R: 255, A: 128}); got != want {
		t.Fatalf("canvas drawing pixel = %v, want %v", got, want)
	}
	if s.canvasFrame(c.Source) != drawing {
		t.Fatal("redrawing a canvas at the same version created a second bitmap")
	}

	c.Source = driver.ImageSource{Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="10"/>`)}
	s.setImage(v, &c)
	if source.bitmap != nil || canvas.Get("isConnected").Bool() || !source.img.Call("hasAttribute", "src").Bool() {
		t.Fatal("an encoded source did not replace the canvas with an image load")
	}
	c.Source = driver.ImageSource{Pixels: p}
	s.setImage(v, &c)
	if source.bitmap == nil || !source.loaded || source.img.Call("hasAttribute", "src").Bool() {
		t.Fatal("switching back to pixels did not cancel the image load")
	}
}

func rgbaAt(canvas js.Value, x, y int) color.RGBA {
	data := canvas.Call("getContext", "2d").Call("getImageData", x, y, 1, 1).Get("data")
	return color.RGBA{R: byte(data.Index(0).Int()), G: byte(data.Index(1).Int()), B: byte(data.Index(2).Int()), A: byte(data.Index(3).Int())}
}
