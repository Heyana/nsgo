//go:build darwin

package apple

import (
	"image"
	"image/png"
	"os"
	"strings"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/oneo-me/nsgo/internal/driver"
)

// render draws text laid out at size into a white RGBA bitmap, as a
// flipped view's drawing does.
func render(t *testing.T, text *driver.AttributedText, size driver.Size) (*image.RGBA, *Layout) {
	t.Helper()
	cg := load("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
	var (
		colorSpace func() uintptr
		bitmap     func(data unsafe.Pointer, w, h, bits, row, space uintptr, info uint32) uintptr
	)
	purego.RegisterLibFunc(&colorSpace, cg, "CGColorSpaceCreateDeviceRGB")
	purego.RegisterLibFunc(&bitmap, cg, "CGBitmapContextCreate")
	w, h := int(size.Width), int(size.Height)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	space := colorSpace()
	defer Release(space)
	const premultipliedLast = 1
	ctx := bitmap(unsafe.Pointer(&img.Pix[0]), uintptr(w), uintptr(h), 8, uintptr(img.Stride), space, premultipliedLast)
	defer Release(ctx)
	p := TextEngine{}.Prepare(text)
	defer p.Release()
	l := p.Layout(size).(*Layout)
	t.Cleanup(l.Release)
	// The bitmap's rows go top down in memory but its space is y up:
	// flip it as a flipped view's is.
	cgContextTranslateCTM(ctx, 0, float64(h))
	cgContextScaleCTM(ctx, 1, -1)
	l.DrawBackgrounds(ctx, Rect(0, 0, 0, 0))
	l.Draw(ctx, float64(h), Rect(0, 0, float64(w), float64(h)))
	if dir := os.Getenv("NSGO_TEXT_PNG"); dir != "" {
		f, err := os.Create(dir + "/" + strings.ReplaceAll(t.Name(), "/", "-") + ".png")
		if err == nil {
			png.Encode(f, img)
			f.Close()
		}
	}
	return img, l
}

// inked counts the pixels in r darker than white, and those that are red.
func inked(img *image.RGBA, r image.Rectangle) (dark, red int) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if c.R < 200 || c.G < 200 || c.B < 200 {
				dark++
			}
			if c.R > 180 && c.G < 120 && c.B < 120 {
				red++
			}
		}
	}
	return dark, red
}

func TestTextEllipsizeModes(t *testing.T) {
	text := "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor"
	for name, mode := range map[string]driver.EllipsizeMode{"tail": driver.EllipsizeTail, "head": driver.EllipsizeHead,
		"middle": driver.EllipsizeMiddle, "clip": driver.EllipsizeClip} {
		t.Run(name, func(t *testing.T) {
			at := driver.AttributedText{Text: text, Style: driver.TextStyle{Font: driver.Font{Size: 14, Weight: 400}, Color: driver.RGB(0)},
				MaxLines: 2, Ellipsize: mode}
			_, l := render(t, &at, driver.Size{Width: 200, Height: 40})
			lines := l.TextLines()
			if len(lines) != 2 {
				t.Fatalf("%d lines, want 2", len(lines))
			}
			if w := lines[1].Bounds.Size.Width; w > 201 {
				t.Errorf("last line %v wide, want within 200", w)
			}
			if lines[1].End != len(text) {
				t.Errorf("last line ends at %d, want the text's end", lines[1].End)
			}
			if lines[0].CapHeight <= 0 || lines[0].XHeight <= 0 || lines[0].Ascender <= 0 {
				t.Errorf("metrics %+v", lines[0])
			}
		})
	}
}

func TestTextDecorationsAndShadow(t *testing.T) {
	red := driver.RGB(0xff0000)
	at := driver.AttributedText{Text: "Wavy", Style: driver.TextStyle{Font: driver.Font{Size: 20, Weight: 400}, Color: driver.RGB(0x000000),
		Decoration: driver.TextDecorationUnderline, DecorationStyle: driver.DecorationWavy, DecorationColor: red}}
	img, _ := render(t, &at, driver.Size{Width: 80, Height: 30})
	if _, r := inked(img, img.Rect); r < 10 {
		t.Errorf("a wavy red underline drew %d red pixels", r)
	}
	plain := driver.AttributedText{Text: "Shadow", Style: driver.TextStyle{Font: driver.Font{Size: 20, Weight: 400}, Color: driver.RGB(0xffffff)}}
	img, _ = render(t, &plain, driver.Size{Width: 90, Height: 30})
	if d, _ := inked(img, img.Rect); d != 0 {
		t.Fatalf("white text drew %d dark pixels", d)
	}
	plain.Shadow = driver.TextShadow{Color: driver.RGB(0x000000), Offset: driver.Point{X: 2, Y: 2}}
	img, _ = render(t, &plain, driver.Size{Width: 90, Height: 30})
	if d, _ := inked(img, img.Rect); d < 20 {
		t.Errorf("a black shadow drew %d dark pixels", d)
	}
}

func TestTextJustified(t *testing.T) {
	at := driver.AttributedText{Text: "aa bb cc dd ee ff gg hh ii jj kk ll mm nn oo pp", Style: driver.TextStyle{
		Font: driver.Font{Size: 14, Weight: 400}, Color: driver.RGB(0), Align: driver.TextAlignJustify}}
	_, l := render(t, &at, driver.Size{Width: 120, Height: 80})
	lines := l.TextLines()
	if len(lines) < 2 {
		t.Fatalf("%d lines, want several", len(lines))
	}
	if w := lines[0].Bounds.Size.Width; w < 118 {
		t.Errorf("first line %v wide, want stretched to 120", w)
	}
	if lines[1].Start != lines[0].End {
		t.Errorf("lines %+v do not follow each other", lines[:2])
	}
}
