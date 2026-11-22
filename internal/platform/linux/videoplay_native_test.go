//go:build linux && !android

package linux

import (
	"github.com/ebitengine/purego"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestVideoControlsReportAndHoldPlayback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controls.webm")
	if err := os.WriteFile(path, videoRateClip, 0600); err != nil {
		t.Fatal(err)
	}
	events := make(chan string, 20)
	loads := make(chan struct{}, 1)
	failures := make(chan error, 1)
	w := startTestWindow(t, func(cx *core.Scope) core.Node {
		return core.Video{Source: path, Muted: true, Controls: true, Style: core.Style{Width: core.Pt(320), Height: core.Pt(240)}, OnLoad: func(core.VideoInfo) { loads <- struct{}{} }, OnPause: func(reason core.VideoPauseReason) {
			if reason != core.VideoPauseControls {
				events <- "wrong reason"
			} else {
				events <- "pause"
			}
		}, OnPlay: func() { events <- "play" }, OnError: func(err error) { failures <- err }}
	}, Options{Width: 400, Height: 300})
	select {
	case <-loads:
	case err := <-failures:
		t.Fatal(err)
	case <-time.After(10 * time.Second):
		t.Fatal("video did not load")
	}
	var v *view
	w.onLoop(t, func() {
		for _, candidate := range w.views {
			if candidate != nil && candidate.video != nil {
				v = candidate
				break
			}
		}
	})
	if v == nil {
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
	var iconName func(uintptr) uintptr
	purego.RegisterLibFunc(&iconName, gtkLib, "gtk_button_get_icon_name")
	// Activate GtkMediaControls’ own button, which must report through the
	// stream’s notification and then core’s callbacks.
	control := func() {
		var button uintptr
		w.onLoop(t, func() {
			wanted := "media-playback-start"
			if gtkMediaStreamGetPlaying(v.video.stream) != 0 {
				wanted = "media-playback-pause"
			}
			var find func(uintptr)
			find = func(widget uintptr) {
				if button != 0 {
					return
				}
				if goString(gTypeNameFromInstance(widget)) == "GtkButton" && strings.HasPrefix(goString(iconName(widget)), wanted) {
					button = widget
					return
				}
				for child := gtkWidgetGetFirstChild(widget); child != 0; child = gtkWidgetGetNextSibling(child) {
					find(child)
				}
			}
			find(v.video.child)
			if button != 0 {
				gSignalEmitByName(button, cstr("clicked"))
			}
		})
		if button == 0 {
			t.Fatal("GtkMediaControls has no play/pause button")
		}
	}
	configure := func(change func(*driver.VideoConfig)) {
		w.onLoop(t, func() { c := v.video.config; change(&c); w.configureVideo(v, &c) })
	}
	check := func(playing bool) {
		t.Helper()
		var got bool
		w.onLoop(t, func() { got = gtkMediaStreamGetPlaying(v.video.stream) != 0 })
		if got != playing {
			t.Fatalf("playing=%v, want %v", got, playing)
		}
	}
	quiet()
	control()
	event("pause")
	configure(func(c *driver.VideoConfig) { c.Volume = .3 })
	quiet()
	check(false)
	control()
	event("play")
	configure(func(c *driver.VideoConfig) { c.Paused = true })
	quiet()
	check(false)
	control()
	event("play")
	configure(func(c *driver.VideoConfig) { c.Rate = 1.25 })
	quiet()
	check(true)
	configure(func(c *driver.VideoConfig) { c.Paused = false })
	quiet()
	configure(func(c *driver.VideoConfig) { c.Paused = true })
	quiet()
	check(false)
}
