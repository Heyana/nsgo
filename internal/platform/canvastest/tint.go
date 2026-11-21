package canvastest

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const TintWidth, TintHeight = 160, 120

var tintProbes = []struct {
	name string
	x, y driver.Points
	want driver.Color
}{
	{name: "opaque source multiplied and composited", x: 16, y: 24, want: 0x404040ff},
	{name: "partial alpha multiplied and composited", x: 32, y: 24, want: 0x285854ff},
	{name: "transparent source preserves the backdrop", x: 48, y: 24, want: 0x204060ff},
	{name: "cropped source uses its own tint", x: 16, y: 48, want: 0x28e020ff},
	{name: "zero tint alpha preserves the backdrop", x: 28, y: 48, want: 0x204060ff},
	{name: "later draw keeps the original colors", x: 32, y: 72, want: 0xc04080ff},
}

// tintStripes are the sprite's four stripes, each 8 by 16 pixels: two opaque, one translucent, and one clear.
var tintStripes = []color.NRGBA{
	{R: 192, G: 64, B: 128, A: 255},
	{R: 128, G: 160, B: 192, A: 128},
	{},
	{R: 160, G: 224, B: 32, A: 255},
}

const tintSpriteWidth, tintSpriteHeight = 32, 16

func tintSprite() *image.NRGBA {
	sprite := image.NewNRGBA(image.Rect(0, 0, tintSpriteWidth, tintSpriteHeight))
	for y := range sprite.Bounds().Dy() {
		for x := range sprite.Bounds().Dx() {
			sprite.SetNRGBA(x, y, tintStripes[x/8])
		}
	}
	return sprite
}

// TintSource makes the sprite as an encoded PNG.
func TintSource() (driver.ImageSource, error) {
	var data bytes.Buffer
	err := png.Encode(&data, tintSprite())
	return driver.ImageSource{Data: data.Bytes()}, err
}

// TintPixelsSource makes the sprite as a pixel buffer, a point a pixel.
func TintPixelsSource() driver.ImageSource {
	sprite := tintSprite()
	pixels := image.NewRGBA(sprite.Bounds())
	draw.Draw(pixels, pixels.Bounds(), sprite, image.Point{}, draw.Src)
	return driver.ImageSource{Pixels: pixels, Version: 1}
}

// TintSpriteSize is the sprite's size in points, for an offscreen image of it.
var TintSpriteSize = driver.Size{Width: tintSpriteWidth, Height: tintSpriteHeight}

// DrawTintSprite draws the sprite's stripes into an offscreen image TintSpriteSize large.
func DrawTintSprite(d *driver.Drawing) {
	for i, c := range tintStripes {
		if c.A == 0 {
			continue
		}
		d.SetFillColor(driver.Color(uint32(c.R)<<24|uint32(c.G)<<16|uint32(c.B)<<8|uint32(c.A))).FillRect(float32(i*8), 0, 8, tintSpriteHeight)
	}
}

// TintOffscreenSource makes the sprite as an offscreen canvas drawn by DrawTintSprite.
func TintOffscreenSource(d *driver.Drawing, id uint64) driver.ImageSource {
	DrawTintSprite(d)
	return driver.ImageSource{Offscreen: &driver.OffscreenCanvas{ID: id, Version: 1, Size: TintSpriteSize, Scale: 1, Drawing: d}}
}

// DrawTintSample records tinted, cropped and original draws of src over a solid backdrop.
func DrawTintSample(d *driver.Drawing, src driver.ImageSource, width, height float32) {
	d.SetFillColor(0x204060ff).FillRect(0, 0, width, height)
	// Reference fills undergo the same display color conversion as the image.
	for i, c := range tintProbes {
		d.SetFillColor(c.want).FillRect(float32(8+i*12), 96, 10, 10)
	}
	d.DrawTintedImageIn(src, 0x80ff4080, 8, 8, 64, 32)
	d.DrawTintedImageFrom(src, 0x40ffffff, 24, 0, 8, 16, 8, 40, 16, 16)
	d.DrawTintedImage(src, 0, 24, 40)
	d.DrawImageIn(src, 24, 56, 64, 32)
}

// CheckTintSample compares the drawing to its reference fills, in the captured color space.
func CheckTintSample(img image.Image, origin driver.Point, scale float32) []string {
	at := func(x, y driver.Points) color.RGBA {
		b := img.Bounds()
		return color.RGBAModel.Convert(img.At(b.Min.X+int(float32(origin.X+x)*scale), b.Min.Y+int(float32(origin.Y+y)*scale))).(color.RGBA)
	}
	near := func(a, b color.RGBA) bool {
		return !far(a.R, b.R) && !far(a.G, b.G) && !far(a.B, b.B) && !far(a.A, b.A)
	}
	if a, b := at(13, 101), at(49, 101); near(a, b) {
		return []string{fmt.Sprintf("the reference fills look alike: %v and %v", a, b)}
	}
	var failed []string
	for i, c := range tintProbes {
		got, want := at(c.x, c.y), at(driver.Points(13+i*12), 101)
		if want.A != 255 || !near(got, want) {
			failed = append(failed, fmt.Sprintf("%s at (%v, %v) shows %v, want %v", c.name, c.x, c.y, got, want))
		}
	}
	return failed
}
