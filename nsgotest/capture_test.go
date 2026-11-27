package nsgotest

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
)

func TestHeadlessRuntimeCaptureImageAndShare(t *testing.T) {
	if Target() != "" {
		t.Skip("headless model test")
	}
	var scope *core.Scope
	var targetRef core.Ref
	var preview *core.Signal[[]byte]

	app := Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		preview = core.NewSignalFunc(cx, nil, bytes.Equal)
		return core.View{
			Children: []core.Node{
				core.View{Ref: &targetRef, TestID: "capture-box", Style: core.Style{Width: core.Pt(60), Height: core.Pt(40)}},
				core.Image{TestID: "preview", Style: core.Style{Width: core.Pt(60), Height: core.Pt(40)}, SourceFunc: func() core.ImageSource {
					return core.ImageSource{Data: preview.Get()}
				}},
			},
		}
	})

	// Default headless capture reports CodeUnavailable.
	var defaultErr error
	app.Post(func() {
		scope.Capture(nil, func(_ []byte, err error) { defaultErr = err })
	})
	app.Settle()
	if core.ErrorCode(defaultErr) != core.CodeUnavailable {
		t.Fatalf("default headless capture err = %v, want CodeUnavailable", defaultErr)
	}

	// Build a valid 4x4 PNG for the fake surface capture.
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			img.SetRGBA(x, y, color.RGBA{R: 0x22, G: 0xaa, B: 0xee, A: 0xff})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	wantPNG := encoded.Bytes()

	var seenIDs []string
	app.CaptureSurface = func(v *View) ([]byte, error) {
		if v == nil {
			seenIDs = append(seenIDs, "<surface>")
		} else {
			seenIDs = append(seenIDs, v.Props.TestID)
		}
		return wantPNG, nil
	}

	// Capture target view, show in Image, save to CacheDir and pass to Share.
	var sharedOK bool
	var captureErr, shareErr error
	app.Post(func() {
		syncCalled := false
		scope.Capture(&targetRef, func(data []byte, err error) {
			syncCalled = true
			captureErr = err
			if err != nil {
				return
			}
			preview.Set(data)
			path := filepath.Join(scope.CacheDir(), "shot.png")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				shareErr = err
				return
			}
			scope.Share(core.ShareOptions{Files: []core.SharedFile{{Path: path}}}, func(shown bool, err error) {
				sharedOK, shareErr = shown, err
			})
		})
		if syncCalled {
			t.Errorf("Capture invoked its callback synchronously")
		}
	})
	app.Settle()
	if captureErr != nil {
		t.Fatalf("view capture failed: %v", captureErr)
	}
	if shareErr != nil || !sharedOK {
		t.Fatalf("share after capture = (%v, %v), want (true, nil)", sharedOK, shareErr)
	}
	if len(seenIDs) != 1 || seenIDs[0] != "capture-box" {
		t.Fatalf("CaptureSurface targets = %v, want [capture-box]", seenIDs)
	}
	if len(app.Shares) != 1 || len(app.Shares[0].Files) != 1 {
		t.Fatalf("app.Shares = %+v, want 1 shared capture file", app.Shares)
	}
	written, err := os.ReadFile(app.Shares[0].Files[0].Path)
	if err != nil || !bytes.Equal(written, wantPNG) {
		t.Fatalf("shared file bytes mismatch (err=%v, len=%d)", err, len(written))
	}
	if !bytes.Equal(app.Find("id:preview").Image.Source.Data, wantPNG) {
		t.Fatalf("preview Image did not receive captured PNG bytes")
	}

	// Whole-surface capture passes nil target.
	var surfacePNG []byte
	app.Post(func() {
		scope.Capture(nil, func(data []byte, err error) {
			if err != nil {
				t.Errorf("surface capture failed: %v", err)
			}
			surfacePNG = data
		})
	})
	app.Settle()
	if !bytes.Equal(surfacePNG, wantPNG) || len(seenIDs) != 2 || seenIDs[1] != "<surface>" {
		t.Fatalf("surface capture targets=%v len=%d", seenIDs, len(surfacePNG))
	}
}
