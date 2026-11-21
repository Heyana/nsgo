package driver

import (
	"image"
	"image/color"
	"testing"
)

func TestMenuIconMethods(t *testing.T) {
	if !(MenuIcon{}).Empty() || (MenuIcon{}).Key() != "" || (MenuIcon{}).IsTemplate() {
		t.Fatal("zero MenuIcon should be empty, have empty key, and not be template")
	}

	mono, err := ParseSVG([]byte(`<svg viewBox="0 0 24 24"><path d="M2 2h20v20H2z" fill="currentColor"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	colored, err := ParseSVG([]byte(`<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="8" fill="#ff0000"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	if !mono.Monochrome() {
		t.Error("mono SVG should report Monochrome")
	}
	if colored.Monochrome() {
		t.Error("colored SVG should not report Monochrome")
	}

	svgIcon := MenuIcon{SVG: mono}
	if svgIcon.Empty() || !svgIcon.IsTemplate() || svgIcon.Key() == "" {
		t.Fatalf("svgIcon = %+v, key=%q, template=%v", svgIcon, svgIcon.Key(), svgIcon.IsTemplate())
	}
	if (MenuIcon{SVG: colored}).IsTemplate() {
		t.Error("colored SVG without Template should not report IsTemplate")
	}
	if !(MenuIcon{SVG: colored, Template: true}).IsTemplate() {
		t.Error("colored SVG with Template should report IsTemplate")
	}

	raw := []byte{1, 2, 3, 4}
	a := MenuIcon{Data: raw, Template: true}
	b := a.Clone()
	if !a.Equal(b) || a.Key() != b.Key() {
		t.Fatalf("cloned Data icon differs: a=%+v b=%+v", a, b)
	}
	raw[0] = 99
	if a.Equal(b) {
		t.Fatal("mutating source slice affected cloned MenuIcon.Data")
	}

	pix := image.NewRGBA(image.Rect(0, 0, 2, 2))
	pix.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	srcIcon := MenuIcon{Source: ImageSource{Pixels: pix, Version: 1, Variants: map[float32]string{2: "asset:/x@2x.png"}}}
	cloned := srcIcon.Clone()
	if cloned.Source.Pixels == pix {
		t.Fatal("Clone did not copy Pixels")
	}
	pix.SetRGBA(0, 0, color.RGBA{G: 255, A: 255})
	if cloned.Source.Pixels.RGBAAt(0, 0).R != 255 {
		t.Fatal("mutating original Pixels affected cloned MenuIcon")
	}
	srcIcon.Source.Variants[2] = "other"
	if cloned.Source.Variants[2] != "asset:/x@2x.png" {
		t.Fatal("mutating original Variants affected cloned MenuIcon")
	}
}
