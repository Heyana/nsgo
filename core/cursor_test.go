package core

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func testCursorPNG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.SetRGBA(0, 0, c)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestStyleCursorImageMergeAndViewStyle(t *testing.T) {
	png1 := testCursorPNG(t, 16, 16, color.RGBA{R: 255, A: 255})
	png2 := testCursorPNG(t, 16, 16, color.RGBA{B: 255, A: 255})
	c1 := &CustomCursor{Image: ImageSource{Data: png1}, Hotspot: Point{X: 4, Y: 4}}
	c2 := &CustomCursor{Image: ImageSource{Data: png2}, Hotspot: Point{X: 8, Y: 8}, Fallback: CursorPointer}

	// Zero-value Style keeps CursorAuto and nil CursorImage.
	zeroStyle := Style{}
	zero := zeroStyle.viewStyle(Size{}, 0, false)
	if zero.Cursor != CursorAuto || zero.CursorImage != nil {
		t.Fatalf("zero style: Cursor=%v CursorImage=%v", zero.Cursor, zero.CursorImage)
	}

	// Named Cursor on the same Style becomes Fallback when CustomCursor.Fallback is CursorAuto.
	s := Style{Cursor: CursorCrosshair, CursorImage: c1}
	vs := s.viewStyle(Size{}, 0, false)
	if vs.CursorImage == nil || vs.CursorImage.Fallback != CursorCrosshair {
		t.Fatalf("expected named Cursor as fallback, got %+v", vs.CursorImage)
	}

	// Merging a state style that sets Cursor clears CursorImage.
	overNamed := s.Merge(Style{Cursor: CursorNotAllowed})
	if overNamed.Cursor != CursorNotAllowed || overNamed.CursorImage != nil {
		t.Fatalf("Merge(Cursor) = (%v, %v), want (NotAllowed, nil)", overNamed.Cursor, overNamed.CursorImage)
	}

	// Merging a state style without Cursor/CursorImage preserves CursorImage.
	overOpacity := s.Merge(Style{Opacity: Alpha(0.5)})
	if overOpacity.CursorImage != c1 {
		t.Fatalf("Merge(Opacity) dropped CursorImage: %v", overOpacity.CursorImage)
	}

	// Merging another CursorImage replaces it.
	overCustom := s.Merge(Style{CursorImage: c2})
	if overCustom.CursorImage != c2 {
		t.Fatalf("Merge(CursorImage) = %v, want c2", overCustom.CursorImage)
	}
}

func TestCursorImageCommitDiffAndPixelsCopy(t *testing.T) {
	png1 := testCursorPNG(t, 16, 16, color.RGBA{R: 255, A: 255})
	pix := image.NewRGBA(image.Rect(0, 0, 8, 8))
	pix.SetRGBA(0, 0, color.RGBA{R: 200, A: 255})

	var curSig *Signal[*CustomCursor]
	var bgSig *Signal[Color]

	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		curSig = NewSignal(cx, &CustomCursor{
			Image:    ImageSource{Data: png1},
			Hotspot:  Point{X: 3, Y: 4},
			Fallback: CursorCrosshair,
		})
		bgSig = NewSignal(cx, RGB(0x111111))
		return View{
			TestID: "box",
			StyleFunc: func() Style {
				return Style{
					Width:       Pt(100),
					Height:      Pt(100),
					Background:  bgSig.Get(),
					CursorImage: curSig.Get(),
				}
			},
		}
	})

	styleItems := func(b *driver.MountBatch) []*driver.ViewStyle {
		var out []*driver.ViewStyle
		for _, it := range b.Items {
			if it.Kind == driver.ItemViewStyle && it.ViewStyle != nil {
				out = append(out, it.ViewStyle)
			}
		}
		return out
	}

	// Initial commit includes CursorImage.
	first := h.commit()
	var found *driver.ViewStyle
	for _, st := range styleItems(first) {
		if st.CursorImage != nil {
			found = st
		}
	}
	if found == nil || found.CursorImage.Hotspot != (Point{X: 3, Y: 4}) || found.CursorImage.Fallback != CursorCrosshair {
		t.Fatalf("initial commit missing expected CursorImage: %+v", found)
	}

	// Re-committing an equal (newly allocated) CustomCursor with identical bytes emits NO ItemViewStyle.
	curSig.Set(&CustomCursor{
		Image:    ImageSource{Data: append([]byte(nil), png1...)},
		Hotspot:  Point{X: 3, Y: 4},
		Fallback: CursorCrosshair,
	})
	if b := h.commit(); len(styleItems(b)) != 0 {
		t.Fatalf("equal CustomCursor emitted %d ItemViewStyle(s), want 0", len(styleItems(b)))
	}

	// Changing Hotspot emits one ItemViewStyle with the new Hotspot.
	curSig.Set(&CustomCursor{
		Image:    ImageSource{Data: png1},
		Hotspot:  Point{X: 7, Y: 9},
		Fallback: CursorCrosshair,
	})
	b := h.commit()
	if got := styleItems(b); len(got) != 1 || got[0].CursorImage == nil || got[0].CursorImage.Hotspot != (Point{X: 7, Y: 9}) {
		t.Fatalf("hotspot change emitted %+v", got)
	}

	// Pixels are copied on commit and stay stable across unrelated commits.
	curSig.Set(&CustomCursor{Image: ImageSource{Pixels: pix, Version: 1}})
	b = h.commit()
	got := styleItems(b)
	if len(got) != 1 || got[0].CursorImage == nil || got[0].CursorImage.Image.Pixels == nil {
		t.Fatalf("pixels cursor commit failed: %+v", got)
	}
	sentPix := got[0].CursorImage.Image.Pixels
	if sentPix == pix {
		t.Fatal("core must copy CursorImage Pixels before handing to platform")
	}
	// Mutating the app's buffer does not affect the committed copy.
	pix.SetRGBA(0, 0, color.RGBA{G: 255, A: 255})
	if sentPix.RGBAAt(0, 0).R != 200 {
		t.Fatalf("committed pixel copy was mutated: %+v", sentPix.RGBAAt(0, 0))
	}

	// Same Pixels and Version with a new CustomCursor struct emits no diff.
	curSig.Set(&CustomCursor{Image: ImageSource{Pixels: pix, Version: 1}})
	if b := h.commit(); len(styleItems(b)) != 0 {
		t.Fatalf("unchanged pixel Version emitted %d ItemViewStyle(s), want 0", len(styleItems(b)))
	}

	// Bumping Version emits a new copy with the updated pixels.
	curSig.Set(&CustomCursor{Image: ImageSource{Pixels: pix, Version: 2}})
	b = h.commit()
	got = styleItems(b)
	if len(got) != 1 || got[0].CursorImage == nil || got[0].CursorImage.Image.Pixels.RGBAAt(0, 0).G != 255 {
		t.Fatalf("bumped pixel Version did not commit new pixels: %+v", got)
	}

	// Clearing CursorImage back to nil emits an ItemViewStyle with CursorImage == nil.
	curSig.Set(nil)
	b = h.commit()
	got = styleItems(b)
	if len(got) != 1 || got[0].CursorImage != nil {
		t.Fatalf("clearing CursorImage emitted %+v", got)
	}
}

func TestEnvSupportsCustomCursor(t *testing.T) {
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		return View{}
	})
	h.commit()
	env := h.root.Env()

	for _, tc := range []struct {
		name string
		env  Environment
		want bool
	}{
		{"zero", Environment{}, false},
		{"macos", Environment{OS: OSMacOS, Idiom: IdiomDesktop}, true},
		{"windows", Environment{OS: OSWindows, Idiom: IdiomDesktop}, true},
		{"linux", Environment{OS: OSLinux, Idiom: IdiomDesktop}, true},
		{"web", Environment{OS: OSWeb, Idiom: IdiomDesktop}, true},
		{"ios", Environment{OS: OSIOS, Idiom: IdiomPhone}, false},
		{"android", Environment{OS: OSAndroid, Idiom: IdiomPhone}, false},
		{"simulated ios on web", Environment{OS: OSIOS, Idiom: IdiomPhone, Simulated: true}, false},
		{"explicit flag", Environment{CustomCursor: true}, true},
	} {
		h.tree.SetEnvironment(tc.env)
		if got := env.SupportsCustomCursor(); got != tc.want {
			t.Errorf("%s: SupportsCustomCursor() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
