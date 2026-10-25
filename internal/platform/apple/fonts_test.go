//go:build darwin

package apple

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font/gofont/gosmallcaps"

	"github.com/oneo-me/nsgo/internal/driver"
	"github.com/oneo-me/nsgo/internal/platform/imageload"
)

// TestRegisteredFontsLayOut registers a font file from the resources
// and checks that text in its family lays out in it: in small capitals,
// which the system's font draws narrower.
func TestRegisteredFontsLayOut(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "fonts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fonts", "smallcaps.ttf"), gosmallcaps.TTF, 0o644); err != nil {
		t.Fatal(err)
	}
	root := imageload.ResourceRoot
	imageload.ResourceRoot = dir
	defer func() { imageload.ResourceRoot = root }()
	width := func(family string) driver.Points {
		p := TextEngine{}.Prepare(&driver.AttributedText{Text: "lowercase letters",
			Style: driver.TextStyle{Font: driver.Font{Family: family, Size: 20, Weight: 400}, Color: driver.RGB(0)}})
		defer p.Release()
		return p.Measure(driver.Auto).Width
	}
	// Families are cached once looked up: measure each the first time.
	RegisterFonts([]string{"fonts/smallcaps.ttf"})
	fallback := width("Go Smallcaps Unregistered")
	if got := width("Go Smallcaps"); got <= fallback*1.05 {
		t.Fatalf("Go Smallcaps measures %v, the system's font %v: not its own glyphs", got, fallback)
	}
}
