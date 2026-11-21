//go:build darwin && !ios

package macos

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	alphaclips "github.com/nsgo-dev/nsgo/internal/platform/videoalpha/clips"
)

// silence writes a WAV file of d of silence, which AVPlayer plays as it does a video's sound.
func silence(t *testing.T, d time.Duration) string {
	const rate = 8000
	samples := int(d.Seconds() * rate)
	b := make([]byte, 44+2*samples)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(36+2*samples))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1) // PCM
	binary.LittleEndian.PutUint16(b[22:], 1) // mono
	binary.LittleEndian.PutUint32(b[24:], rate)
	binary.LittleEndian.PutUint32(b[28:], 2*rate)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(2*samples))
	path := filepath.Join(t.TempDir(), "silence.wav")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// videoPlayer is the AVPlayer of the app's one video, which shows without controls: what its controls would pause and play.
func videoPlayer(t *testing.T) objc.ID {
	for _, v := range views {
		if v.video != nil && v.video.View() != 0 {
			return v.video.View().Send(sel("layer")).Send(sel("player"))
		}
	}
	t.Error("no video shows")
	return 0
}

// TestVideoReportsWhatItsPlayerDidByItself pauses and plays a playing video's AVPlayer as its controls do: each is reported once, the pause holds through a config that changes only the volume, the controls play at the app's Rate, the app's own pause and play are not reported, and the end is EventEnd's alone. Run ends a process's app for good, so the test runs in a process of its own.
func TestVideoReportsWhatItsPlayerDidByItself(t *testing.T) {
	if os.Getenv("NSGO_TEST_VIDEO_PROCESS") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_VIDEO_PROCESS="+t.Name(), "HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	source := silence(t, 2*time.Second)
	var mu sync.Mutex
	var heard []string
	// replayed is the rate the controls' play gave the player.
	var replayed float32
	hear := func(s string) {
		mu.Lock()
		heard = append(heard, s)
		mu.Unlock()
	}
	// later runs fn on the main thread after d, as the user would act.
	later := func(d time.Duration, fn func()) { time.AfterFunc(d, func() { apple.OnMain(fn) }) }
	app := func(cx *core.Scope) core.Node {
		paused, volume := core.NewSignal(cx, false), core.NewSignal(cx, float32(0.5))
		pauses := 0
		return core.Video{Source: source, PausedFunc: paused.Get, VolumeFunc: volume.Get, Rate: 1.5,
			Style: core.Style{Width: core.Pt(160), Height: core.Pt(90)},
			OnLoad: func(core.VideoInfo) {
				hear("load")
				later(300*time.Millisecond, func() { videoPlayer(t).Send(sel("pause")) })
			},
			OnPause: func(r core.VideoPauseReason) {
				hear("pause " + r.String())
				if pauses++; pauses > 1 {
					return
				}
				cx.Post(func() { volume.Set(0.4) })
				later(300*time.Millisecond, func() {
					if objc.Send[float32](videoPlayer(t), sel("rate")) == 0 {
						hear("held")
					}
					videoPlayer(t).Send(sel("play"))
					mu.Lock()
					replayed = objc.Send[float32](videoPlayer(t), sel("rate"))
					mu.Unlock()
				})
			},
			OnPlay: func() {
				hear("play")
				// The app's own pause and play are not reported.
				cx.Post(func() { paused.Set(true) })
				later(200*time.Millisecond, func() { cx.Post(func() { paused.Set(false) }) })
			},
			OnEnd: func() {
				hear("end")
				later(500*time.Millisecond, func() { cx.Post(cx.Quit) })
			},
			OnError: func(err error) { t.Error(err); cx.Post(cx.Quit) },
		}
	}
	watchdog := time.AfterFunc(30*time.Second, func() {
		mu.Lock()
		defer mu.Unlock()
		panic("the video did not play to its end, having reported " + strings.Join(heard, ", "))
	})
	defer watchdog.Stop()
	onMain(func() { Run(app, Options{Title: "Video", Width: 200, Height: 120}) })
	mu.Lock()
	defer mu.Unlock()
	if want := []string{"load", "pause controls", "held", "play", "end"}; !slices.Equal(heard, want) {
		t.Errorf("heard %q, want %q", heard, want)
	}
	if replayed != 1.5 {
		t.Errorf("the controls played at %v, want the app's 1.5", replayed)
	}
}

// TestVideoKeepsHEVCAlphaAndRefusesWebM plays HEVC with alpha, transparent around a blue square, over a red view, and reads the window as WindowServer composites it: the red shows through around the square. It also reports that WebM with alpha, which AVFoundation cannot open, does not play. Run ends a process's app for good, so the test runs in a process of its own.
func TestVideoKeepsHEVCAlphaAndRefusesWebM(t *testing.T) {
	if os.Getenv("NSGO_TEST_VIDEO_PROCESS") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_VIDEO_PROCESS="+t.Name(), "HOME="+t.TempDir())
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		if strings.Contains(string(out), "--- SKIP") {
			t.Skipf("%s", out)
		}
		return
	}
	dir := t.TempDir()
	hevc, webm := filepath.Join(dir, "alpha.mov"), filepath.Join(dir, "alpha.webm")
	for path, b := range map[string][]byte{hevc: alphaclips.HEVCAlpha, webm: alphaclips.VP9Alpha} {
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	var loaded core.Size
	var refused, pixels string
	var once sync.Once
	app := func(cx *core.Scope) core.Node {
		done := func() {
			if loaded.Width > 0 && refused != "" {
				once.Do(func() {
					go func() {
						got := videoPixels()
						mu.Lock()
						pixels = got
						mu.Unlock()
						cx.Post(cx.Quit)
					}()
				})
			}
		}
		box := core.Style{Width: core.Pt(64), Height: core.Pt(64)}
		return core.View{Style: core.Style{Direction: core.Row, Background: core.RGB(0xff0000)}, Children: []core.Node{
			core.Video{Source: hevc, Muted: true, Repeat: true, Style: box,
				OnLoad: func(i core.VideoInfo) {
					mu.Lock()
					defer mu.Unlock()
					loaded = i.Size
					done()
				},
				OnError: func(err error) { t.Error(err); cx.Post(cx.Quit) }},
			core.Video{Source: webm, Muted: true, Style: box,
				OnLoad: func(core.VideoInfo) { t.Error("WebM loaded"); cx.Post(cx.Quit) },
				OnError: func(err error) {
					mu.Lock()
					defer mu.Unlock()
					refused = err.Error()
					if me, ok := err.(*core.MediaError); !ok || me.Reason != core.MediaErrorAlphaUnsupported {
						refused = "no alpha reason: " + refused
					}
					done()
				}},
		}}
	}
	watchdog := time.AfterFunc(30*time.Second, func() { panic("the videos neither loaded nor failed") })
	defer watchdog.Stop()
	onMain(func() { Run(app, Options{Title: "Video alpha", Width: 200, Height: 120}) })
	mu.Lock()
	defer mu.Unlock()
	if loaded != (core.Size{Width: 64, Height: 64}) {
		t.Errorf("HEVC with alpha loaded at %v, want 64x64", loaded)
	}
	if !strings.HasPrefix(refused, webm+": AVFoundation cannot open WebM, so VP9 with alpha does not play") {
		t.Errorf("WebM with alpha reported %q", refused)
	}
	switch pixels {
	case "":
	case errNoCapture:
		t.Skip("WindowServer gives no picture of the window here, so the composited pixels are not checked")
	default:
		t.Error(pixels)
	}
}

const errNoCapture = "no capture"

// videoPixels waits up to five seconds for the app's one video that shows to composite as red around a blue square in its middle half, and returns "" when it does, errNoCapture when WindowServer gave no picture of its window in that time, or what it saw last.
func videoPixels() string {
	last := errNoCapture
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		shot := make(chan image.Image)
		apple.OnMain(func() { shot <- captureVideo() })
		img := <-shot
		if img == nil {
			continue
		}
		b := img.Bounds()
		at := func(fx, fy float64) color.RGBA {
			return color.RGBAModel.Convert(img.At(b.Min.X+int(fx*float64(b.Dx())), b.Min.Y+int(fy*float64(b.Dy())))).(color.RGBA)
		}
		red := func(c color.RGBA) bool { return c.R > 200 && c.G < 60 && c.B < 60 }
		blue := func(c color.RGBA) bool { return c.B > 200 && c.R < 60 && c.G < 60 }
		corner, far, middle := at(0.08, 0.08), at(0.92, 0.92), at(0.5, 0.5)
		if red(corner) && red(far) && blue(middle) {
			return ""
		}
		last = fmt.Sprintf("want red around a blue square, have %v and %v at the video's corners and %v in its middle (RGBA) in %dx%d", corner, far, middle, b.Dx(), b.Dy())
	}
	return last
}

// captureVideo is the picture WindowServer composites of the app's one video that shows, nil when the video is not in a window yet or WindowServer gives none.
func captureVideo() image.Image {
	var view objc.ID
	for _, v := range views {
		if v.video != nil && v.video.View() != 0 {
			view = v.video.View()
		}
	}
	if view == 0 {
		return nil
	}
	return captureView(view)
}

// captureView is the picture WindowServer composites of view in its window, nil when view is in no window or WindowServer gives none.
func captureView(view objc.ID) image.Image {
	window := view.Send(sel("window"))
	cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW)
	if err != nil || window == 0 {
		return nil
	}
	// Deprecated since macOS 14, but still the one call that gives a window as WindowServer composites it.
	if _, err := purego.Dlsym(cg, "CGWindowListCreateImage"); err != nil {
		return nil
	}
	var windowImage func(bounds apple.CGRect, option, window, imageOption uint32) uintptr
	purego.RegisterLibFunc(&windowImage, cg, "CGWindowListCreateImage")
	inWindow := objc.Send[apple.CGRect](view, sel("convertRect:toView:"), objc.Send[apple.CGRect](view, selBounds), objc.ID(0))
	r := objc.Send[apple.CGRect](window, sel("convertRectToScreen:"), inWindow)
	screens := objc.ID(apple.Class("NSScreen")).Send(sel("screens"))
	h := objc.Send[apple.CGRect](screens.Send(sel("objectAtIndex:"), uint(0)), sel("frame")).Size.Height
	const (
		windowListIncludingWindow    = 1 << 3
		windowImageBoundsIgnoreFrame = 1 << 0
		windowImageBestResolution    = 1 << 3
	)
	shot := windowImage(apple.Rect(r.Origin.X, h-r.Origin.Y-r.Size.Height, r.Size.Width, r.Size.Height),
		windowListIncludingWindow, uint32(objc.Send[int](window, sel("windowNumber"))), windowImageBoundsIgnoreFrame|windowImageBestResolution)
	if shot == 0 {
		return nil
	}
	defer apple.Release(shot)
	b, err := apple.PNGImage(shot)
	if err != nil {
		return nil
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil || img.Bounds().Dx() == 0 {
		return nil
	}
	return img
}

// TestVideoPlaysSeparateAudioAndSeeksItsComposition uses distinct HTTP
// resources and a native player. The audio stays blocked until the app
// requests a seek, which must take effect once both tracks are loaded.
func TestVideoPlaysSeparateAudioAndSeeksItsComposition(t *testing.T) {
	if os.Getenv("NSGO_TEST_VIDEO_PROCESS") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_VIDEO_PROCESS="+t.Name(), "HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}

	sound, err := os.ReadFile(silence(t, time.Second))
	if err != nil {
		t.Fatal(err)
	}
	var videoRequests, audioRequests atomic.Int32
	seekRequested := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/picture.mov":
			videoRequests.Add(1)
			w.Header().Set("Content-Type", "video/quicktime")
			http.ServeContent(w, r, "picture.mov", time.Time{}, bytes.NewReader(alphaclips.HEVC))
		case "/sound.wav":
			audioRequests.Add(1)
			select {
			case <-seekRequested:
			case <-r.Context().Done():
				return
			}
			// Give the posted seek time to reach the player before its item exists.
			select {
			case <-time.After(150 * time.Millisecond):
			case <-r.Context().Done():
				return
			}
			w.Header().Set("Content-Type", "audio/wav")
			http.ServeContent(w, r, "sound.wav", time.Time{}, bytes.NewReader(sound))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	var info atomic.Pointer[core.VideoInfo]
	var tracksChecked atomic.Bool
	var firstProgress atomic.Int64
	var ref core.Ref
	app := func(cx *core.Scope) core.Node {
		paused := core.NewSignal(cx, true)
		cx.AfterFunc(50*time.Millisecond, func() {
			ref.Seek(250 * time.Millisecond)
			paused.Set(false)
			close(seekRequested)
		})
		return core.Video{
			Source:           server.URL + "/picture.mov",
			AudioSource:      server.URL + "/sound.wav",
			Ref:              &ref,
			PausedFunc:       paused.Get,
			Muted:            true,
			Style:            core.Style{Width: core.Pt(160), Height: core.Pt(120)},
			ProgressInterval: 30 * time.Millisecond,
			OnLoad: func(loaded core.VideoInfo) {
				info.Store(&loaded)
				apple.OnMain(func() {
					player := videoPlayer(t)
					asset := player.Send(sel("currentItem")).Send(sel("asset"))
					for _, kind := range []string{"vide", "soun"} {
						tracks := asset.Send(sel("tracksWithMediaType:"), apple.NSString(kind))
						if got := objc.Send[int](tracks, sel("count")); got != 1 {
							t.Errorf("the player's asset has %d %s tracks, want 1", got, kind)
						}
					}
					tracksChecked.Store(true)
				})
			},
			OnProgress: func(at, _ time.Duration) {
				if at > 0 {
					firstProgress.CompareAndSwap(0, int64(at))
				}
			},
			OnEnd: func() { cx.Post(cx.Quit) },
			OnError: func(err error) {
				t.Error(err)
				cx.Post(cx.Quit)
			},
		}
	}
	watchdog := time.AfterFunc(30*time.Second, func() { panic("the composed video did not finish") })
	defer watchdog.Stop()
	onMain(func() { Run(app, Options{Title: "Separate audio", Width: 200, Height: 140}) })

	loaded := info.Load()
	if loaded == nil || loaded.Duration != 500*time.Millisecond || loaded.Size.Width <= 0 || loaded.Size.Height <= 0 {
		t.Errorf("loaded video = %+v, want the half-second picture", loaded)
	}
	if !tracksChecked.Load() || videoRequests.Load() == 0 || audioRequests.Load() == 0 {
		t.Errorf("tracks checked %v, HTTP requests %d video and %d audio",
			tracksChecked.Load(), videoRequests.Load(), audioRequests.Load())
	}
	if at := time.Duration(firstProgress.Load()); at < 200*time.Millisecond || at >= 450*time.Millisecond {
		t.Errorf("first progress after the pending seek = %v, want near 250ms", at)
	}
}

func TestVideoHTTPHeadersAndReactiveSource(t *testing.T) {
	if os.Getenv("NSGO_TEST_VIDEO_PROCESS") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_VIDEO_PROCESS="+t.Name(), "HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}

	const referer = "https://media.example/player"
	var redirects, firstRanges, secondRanges atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != referer || r.Header.Get("Cookie") != "test_session=1" || r.Header.Get("User-Agent") != "NSGoTest/1" {
			t.Errorf("%s was missing the configured headers", r.URL.Path)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.Header.Get("Range") == "" {
			t.Errorf("%s was missing a player byte range", r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/redirect.mov":
			redirects.Add(1)
			http.Redirect(w, r, "/first.mov", http.StatusTemporaryRedirect)
			return
		case "/first.mov":
			firstRanges.Add(1)
		case "/second.mov":
			secondRanges.Add(1)
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "video/quicktime")
		http.ServeContent(w, r, "source.mov", time.Time{}, bytes.NewReader(alphaclips.HEVC))
	}))
	defer server.Close()

	var loaded, ended atomic.Int32
	app := func(cx *core.Scope) core.Node {
		source := core.NewSignal(cx, server.URL+"/redirect.mov")
		return core.Video{
			SourceFunc: source.Get,
			Headers:    map[string]string{"referer": referer, "Cookie": "test_session=1", "User-Agent": "NSGoTest/1"},
			Muted:      true,
			Style:      core.Style{Width: core.Pt(160), Height: core.Pt(120)},
			OnLoad:     func(core.VideoInfo) { loaded.Add(1) },
			OnEnd: func() {
				if ended.Add(1) == 1 {
					source.Set(server.URL + "/second.mov")
				} else {
					cx.Post(cx.Quit)
				}
			},
			OnError: func(err error) {
				t.Error(err)
				cx.Post(cx.Quit)
			},
		}
	}
	watchdog := time.AfterFunc(20*time.Second, func() { panic("the authenticated videos did not finish") })
	defer watchdog.Stop()
	onMain(func() { Run(app, Options{Title: "Video request headers", Width: 200, Height: 140}) })

	if loaded.Load() != 2 || ended.Load() != 2 {
		t.Errorf("loaded %d and completed %d videos, want both", loaded.Load(), ended.Load())
	}
	if redirects.Load() == 0 || firstRanges.Load() < 2 || secondRanges.Load() < 2 {
		t.Errorf("redirects = %d, first and second ranges = %d and %d, want two streamed sources",
			redirects.Load(), firstRanges.Load(), secondRanges.Load())
	}
}
