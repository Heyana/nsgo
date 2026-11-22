//go:build windows

package windows

import (
	_ "embed"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// videoControlsClip has no audio, so media playback needs no sound endpoint. Regenerate with:
//
// ffmpeg -f lavfi -i testsrc2=size=32x24:rate=12:duration=8 -an -c:v libx264 -pix_fmt yuv420p -movflags +faststart testdata/video-controls.mp4
//
//go:embed testdata/video-controls.mp4
var videoControlsClip []byte

func TestVideoControlsReportAndHoldPlayback(t *testing.T) {
	source := filepath.Join(t.TempDir(), "controls.mp4")
	if err := os.WriteFile(source, videoControlsClip, 0600); err != nil {
		t.Fatal(err)
	}
	events := make(chan string, 20)
	loads := make(chan struct{}, 1)
	ends := make(chan struct{}, 1)
	failures := make(chan error, 1)
	hwnd := startApp(t, "nsgo-video-controls-1358", func(cx *core.Scope) core.Node {
		return core.Video{Source: source, Muted: true, Controls: true, Style: core.Style{Width: core.Pt(320), Height: core.Pt(240)}, OnLoad: func(core.VideoInfo) { loads <- struct{}{} }, OnPause: func(reason core.VideoPauseReason) {
			if reason != core.VideoPauseControls {
				events <- "wrong reason"
			} else {
				events <- "pause"
			}
		}, OnPlay: func() { events <- "play" }, OnEnd: func() { ends <- struct{}{} }, OnError: func(err error) { failures <- err }}
	}, 320, 240)
	select {
	case <-loads:
	case err := <-failures:
		t.Fatal(err)
	case <-time.After(10 * time.Second):
		t.Fatal("video did not load")
	}
	var vd *video
	onMain(t, hwnd, func(w *window) {
		for _, v := range w.views {
			if v != nil && v.video != nil {
				vd = v.video
				break
			}
		}
	})
	if vd == nil {
		t.Fatal("no native video")
	}
	event := func(want string) {
		t.Helper()
		select {
		case got := <-events:
			if got != want {
				t.Fatalf("event %s, want %s", got, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("missing %s callback", want)
		}
	}
	quiet := func() {
		t.Helper()
		time.Sleep(100 * time.Millisecond)
		select {
		case got := <-events:
			t.Fatalf("unexpected callback %s", got)
		default:
		}
	}
	configure := func(change func(*driver.VideoConfig)) {
		onMain(t, hwnd, func(*window) { c := vd.config; change(&c); vd.configure(&c) })
	}
	check := func(playing bool) {
		t.Helper()
		var got bool
		onMain(t, hwnd, func(*window) { got = vd.playingNow() })
		if got != playing {
			t.Fatalf("playing=%v, want %v", got, playing)
		}
	}
	// Some sessions give Media Foundation no presentation clock, and it then runs a clip to its end as soon as it plays, before any control can pause it: an SSH session in session 0 of a VM without audio endpoints does, and so does the CI runner.
	ranThrough := func() bool {
		select {
		case <-ends:
			return true
		default:
		}
		var ended bool
		onMain(t, hwnd, func(*window) { ended = vd.engine != nil && uint32(vd.engine.call(meIsEnded)) != 0 })
		return ended
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		var playing bool
		onMain(t, hwnd, func(*window) { playing = vd.playingNow() })
		if playing {
			break
		}
		if ranThrough() {
			t.Skip("Media Foundation ran the 8-second clip to its end at once: this session gives it no presentation clock")
		}
		if time.Now().After(deadline) {
			t.Fatal("video did not start playing")
		}
	}
	quiet()
	if ranThrough() {
		t.Skip("Media Foundation ran the 8-second clip to its end at once: this session gives it no presentation clock")
	}
	click(t, hwnd, 20, 220)
	event("pause")
	configure(func(c *driver.VideoConfig) { c.Volume = .3 })
	quiet()
	check(false)
	click(t, hwnd, 20, 220)
	event("play")
	configure(func(c *driver.VideoConfig) { c.Paused = true })
	quiet()
	check(false)
	click(t, hwnd, 20, 220)
	event("play")
	configure(func(c *driver.VideoConfig) { c.Rate = 1.25 })
	quiet()
	check(true)
	configure(func(c *driver.VideoConfig) { c.Paused = false })
	quiet()
	configure(func(c *driver.VideoConfig) { c.Paused = true })
	quiet()
	check(false)
	onMain(t, hwnd, func(*window) { _, duration := vd.times(); vd.seekTo(duration-.1, false) })
	configure(func(c *driver.VideoConfig) { c.Paused = false })
	select {
	case <-ends:
	case <-time.After(5 * time.Second):
		t.Fatal("video did not end")
	}
	quiet()
	click(t, hwnd, 20, 220)
	event("play")
}
