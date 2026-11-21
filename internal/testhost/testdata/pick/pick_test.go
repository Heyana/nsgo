package pick_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

// pickApp is a button that asks for a photo and shows how many files came back.
func pickApp(cx *core.Scope) core.Node {
	got := core.NewSignal(cx, "none")
	return core.View{Children: []core.Node{
		core.View{Role: core.RoleButton, Label: "Pick", TestID: "pick", Style: core.Style{Width: core.Pt(80), Height: core.Pt(30)},
			OnPress: func(core.PressEvent) {
				cx.PickImages(core.PickOptions{Limit: 1}, func(files []core.PickedFile) {
					got.Set(strings.TrimSpace(fmt.Sprintf("picked %d %s", len(files), names(files))))
				})
			}},
		core.Text{ContentFunc: got.Get},
	}}
}

func names(files []core.PickedFile) string {
	s := ""
	for _, f := range files {
		s += f.Name
	}
	return s
}

func cameraApp(cx *core.Scope) core.Node {
	got := core.NewSignal(cx, "none")
	return core.View{Children: []core.Node{
		core.View{Role: core.RoleButton, Label: "Take", TestID: "take", Style: core.Style{Width: core.Pt(80), Height: core.Pt(30)},
			OnPress: func(core.PressEvent) {
				cx.TakePhoto(core.PhotoOptions{}, func(photo *core.PickedFile, err error) {
					switch {
					case err != nil:
						got.Set("error: " + err.Error())
					case photo == nil:
						got.Set("cancelled")
					default:
						got.Set("photo " + photo.Name)
					}
				})
			}},
		core.Text{ContentFunc: got.Get},
	}}
}

func skipOtherTargets(t *testing.T) {
	switch nsgotest.Target() {
	case "", "macos", "windows", "linux":
	default:
		t.Skip("PickImages answers headless and on the desktops; the other targets show the system's picker")
	}
}

// TestPickImagesAnswersTheApp checks that App.PickImages, set after Start as a test sets it, answers the app's pick in place of the system's picker, headless and on a desktop target alike (#866).
func TestPickImagesAnswersTheApp(t *testing.T) {
	skipOtherTargets(t)
	app := nsgotest.Start(t, pickApp)
	called := 0
	app.PickImages = func(nsgotest.ImagePick) []core.PickedFile {
		called++
		return []core.PickedFile{{Name: "a.png", MIME: "image/png", Data: []byte("png")}}
	}
	app.Press("id:pick")
	app.WaitFor("text:picked 1 a.png", 5*time.Second)
	if called != 1 || len(app.Picks) != 1 || app.Picks[0].Limit != 1 {
		t.Fatalf("PickImages was asked %d times for %+v, want once for a limit of 1", called, app.Picks)
	}
}

// TestPickImagesWithoutAnAnswerCancels checks that without App.PickImages the user cancels, headless and on a desktop target alike, rather than the system's picker opening over the test.
func TestPickImagesWithoutAnAnswerCancels(t *testing.T) {
	skipOtherTargets(t)
	app := nsgotest.Start(t, pickApp)
	app.Press("id:pick")
	app.WaitFor("text:picked 0", 5*time.Second)
}

// TestTakePhotoAnswersTheApp checks that App.TakePhoto, set after Start, reaches the app under every target without opening a camera (#897, #948).
func TestTakePhotoAnswersTheApp(t *testing.T) {
	app := nsgotest.Start(t, cameraApp, nsgotest.Environment(core.Environment{Camera: true}))
	called := 0
	app.TakePhoto = func(nsgotest.ImagePick) (*core.PickedFile, error) {
		called++
		return &core.PickedFile{Name: "capture.jpg", MIME: "image/jpeg", Data: []byte("jpeg")}, nil
	}
	app.Press("id:take")
	app.WaitFor("text:photo capture.jpg", 5*time.Second)
	if called != 1 || len(app.Photos) != 1 {
		t.Fatalf("TakePhoto was asked %d times for %d requests, want once", called, len(app.Photos))
	}
}

// TestTakePhotoWithoutAnAnswerCancels checks that a target test without a camera hook does not open a system camera.
func TestTakePhotoWithoutAnAnswerCancels(t *testing.T) {
	app := nsgotest.Start(t, cameraApp, nsgotest.Environment(core.Environment{Camera: true}))
	app.Press("id:take")
	app.WaitFor("text:cancelled", 5*time.Second)
}

// scanApp is a button that asks to scan a code and shows what came back.
func scanApp(cx *core.Scope) core.Node {
	got := core.NewSignal(cx, "none")
	return core.View{Children: []core.Node{
		core.View{Role: core.RoleButton, Label: "Scan", TestID: "scan", Style: core.Style{Width: core.Pt(80), Height: core.Pt(30)},
			OnPress: func(core.PressEvent) {
				cx.ScanCode(core.ScanOptions{}, func(code *core.ScannedCode, err error) {
					switch {
					case err != nil:
						got.Set("failed " + err.Error())
					case code == nil:
						got.Set("closed")
					default:
						got.Set("read " + code.Text)
					}
				})
			}},
		core.Text{ContentFunc: got.Get},
	}}
}

// TestScanCodeAnswersTheApp checks that App.ScanCode answers the app's scan in place of the camera, headless and on every target (#429), and that without it the user closes the scanner.
func TestScanCodeAnswersTheApp(t *testing.T) {
	app := nsgotest.Start(t, scanApp, nsgotest.Environment(core.Environment{ScanCode: true}))
	app.Press("id:scan")
	app.WaitFor("text:closed", 5*time.Second)
	app.ScanCode = func(nsgotest.CodeScan) (*core.ScannedCode, error) {
		return &core.ScannedCode{Kind: core.CodeQR, Text: "myapp://pair"}, nil
	}
	app.Press("id:scan")
	app.WaitFor("text:read myapp://pair", 5*time.Second)
	if len(app.Scans) != 2 || !app.Scans[1].Wants(core.CodeQR) {
		t.Fatalf("scans recorded %+v, want two of a QR code", app.Scans)
	}
}
