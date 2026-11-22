//go:build ios && nsgotest

package ios_test

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	platformios "github.com/nsgo-dev/nsgo/internal/platform/ios"
	"github.com/nsgo-dev/nsgo/microphone"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func startTVUnsupportedTest(t *testing.T, root func(*core.Scope) core.Node) *nsgotest.App {
	t.Helper()
	if nsgotest.Target() != "tvos_simulator" {
		t.Skip("requires the native tvOS simulator")
	}
	var television atomic.Bool
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		television.Store(cx.Env().OS() == core.OSIOS && cx.Env().Idiom() == core.IdiomTV)
		return root(cx)
	})
	if !television.Load() {
		t.Fatal("the native host did not report Apple TV")
	}
	return app
}

func TestTVUnavailableWebView(t *testing.T) {
	results := make(chan error, 4)
	var source *core.Signal[core.WebSource]
	var ref core.Ref
	app := startTVUnsupportedTest(t, func(cx *core.Scope) core.Node {
		source = core.NewSignal(cx, core.WebSrc("https://unavailable.example/first"))
		return core.WebView{Ref: &ref, SourceFunc: source.Get, OnError: func(err error) { results <- err }}
	})
	for _, path := range []string{"first", "second"} {
		if path == "second" {
			app.Post(func() { source.Set(core.WebSrc("https://unavailable.example/second")) })
		}
		app.WaitUntil(5*time.Second, func() bool { return len(results) > 0 })
		err := <-results
		var failure *core.WebError
		if !errors.As(err, &failure) || failure.URL != "https://unavailable.example/"+path || !strings.Contains(failure.Message, "unavailable on tvOS") || failure.Domain != "" || failure.Code != 0 {
			t.Fatalf("unavailable WebView error = %#v, want a WebError for %s", err, path)
		}
	}
	app.Post(func() { ref.Reload(); ref.GoBack(); ref.StopLoading() })
	app.Settle()
}

func TestTVUnavailableCamera(t *testing.T) {
	results := make(chan error, 1)
	var scope *core.Scope
	app := startTVUnsupportedTest(t, func(cx *core.Scope) core.Node { scope = cx; return core.View{} })
	app.Post(func() {
		scope.TakePhoto(core.PhotoOptions{}, func(photo *core.PickedFile, err error) {
			if photo != nil {
				results <- errors.New("a TV without a photo picker returned a photo")
			} else {
				results <- err
			}
		})
	})
	app.WaitUntil(5*time.Second, func() bool { return len(results) > 0 })
	if err := <-results; !errors.Is(err, core.ErrNoCamera) {
		t.Fatalf("TakePhoto = %v, want ErrNoCamera", err)
	}
}

func TestTVUnavailablePhotoPicker(t *testing.T) {
	results := make(chan []core.PickedFile, 1)
	var scope *core.Scope
	app := startTVUnsupportedTest(t, func(cx *core.Scope) core.Node { scope = cx; return core.View{} })
	app.Post(func() { scope.PickImages(core.PickOptions{}, func(files []core.PickedFile) { results <- files }) })
	app.WaitUntil(5*time.Second, func() bool { return len(results) > 0 })
	if files := <-results; len(files) != 0 {
		t.Fatalf("PickImages returned %d files without a photo picker", len(files))
	}
}

func TestTVUnavailableSpeechRecognition(t *testing.T) {
	results := make(chan error, 1)
	var scope *core.Scope
	app := startTVUnsupportedTest(t, func(cx *core.Scope) core.Node { scope = cx; return core.View{} })
	app.Post(func() {
		_, err := scope.StartSpeech(core.SpeechOptions{OnEnd: func(_ string, err error) { results <- err }})
		if err != nil {
			results <- err
		}
	})
	app.WaitUntil(5*time.Second, func() bool { return len(results) > 0 })
	if err := <-results; !errors.Is(err, core.SpeechUnavailable) {
		t.Fatalf("speech recognition = %v, want SpeechUnavailable", err)
	}
}

func TestTVRecordingRefusal(t *testing.T) {
	startTVUnsupportedTest(t, func(*core.Scope) core.Node { return core.View{} })
	// tvOS 17 supports external microphones. This test bundle declares no
	// microphone usage, so recording must refuse without a permission prompt.
	refused := func(err error) bool {
		return errors.Is(err, microphone.ErrUnsupported) || errors.Is(err, microphone.ErrDenied) || errors.Is(err, microphone.ErrNoUsageDescription)
	}
	if microphone.Allowed() {
		t.Fatal("a fresh TV simulator unexpectedly allows this test app to record")
	}
	if err := microphone.Authorize(); !refused(err) {
		t.Fatalf("Authorize = %v, want the existing permission/configuration refusal", err)
	}
	stream, err := microphone.Capture(8000, func([]int16) {})
	if stream != nil {
		stream.Stop()
	}
	if !refused(err) {
		t.Fatalf("Capture = %v, want a recording refusal", err)
	}
	recording, err := microphone.RecordWAV("tv-refusal.wav", microphone.RecordOptions{Rate: 8000})
	if recording != nil {
		recording.Stop()
	}
	if !refused(err) {
		t.Fatalf("RecordWAV = %v, want a recording refusal", err)
	}
}

func TestTVUnavailableClipboard(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	results := make(chan struct{}, 5)
	var scope *core.Scope
	app := startTVUnsupportedTest(t, func(cx *core.Scope) core.Node { scope = cx; return core.View{} })
	if !platformios.TVClipboardUnavailableForTest() {
		t.Fatal("tvOS pasteboard gate did not reject the public UIPasteboard")
	}
	app.Post(func() {
		scope.SetClipboard("the TV has no pasteboard")
		scope.SetClipboardImage(data.Bytes())
		scope.ReadClipboard(func(string) { results <- struct{}{} })
		scope.HasClipboardText(func(bool) { results <- struct{}{} })
		scope.HasClipboardImage(func(bool) { results <- struct{}{} })
		scope.ReadClipboardFiles(func([]string) { results <- struct{}{} })
		scope.ReadClipboardImage(func(_ core.ClipboardImage, ok bool, err error) {
			results <- struct{}{}
		})
	})
	app.WaitUntil(5*time.Second, func() bool { return len(results) == cap(results) })
	for range cap(results) {
		<-results
	}
}
