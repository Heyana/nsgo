//go:build darwin && !ios

package macos

import (
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/videoalpha/clips"
)

func TestVideoPictureInPicture(t *testing.T) {
	mode := os.Getenv("NSGO_TEST_PIP_CONTROLS")
	if mode == "" {
		for _, controls := range []string{"false", "true", "replace"} {
			t.Run("controls="+controls, func(t *testing.T) {
				cmd := exec.Command(os.Args[0], "-test.run=^TestVideoPictureInPicture$", "-test.v")
				cmd.Env = append(os.Environ(), "NSGO_TEST_PIP_CONTROLS="+controls, "HOME="+t.TempDir())
				out, err := cmd.CombinedOutput()
				t.Logf("%s", out)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(out), "--- SKIP") {
					t.Skipf("%s", out)
				}
			})
		}
		return
	}
	var available bool
	onMain(func() {
		_, err := purego.Dlopen("/System/Library/Frameworks/AVKit.framework/AVKit", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err == nil {
			class := apple.Class("AVPictureInPictureController")
			available = class != 0 && objc.Send[bool](objc.ID(class), apple.Sel("isPictureInPictureSupported"))
		}
	})
	if !available {
		t.Skip("AVKit reports Picture in Picture unavailable on this Mac")
	}
	path := t.TempDir() + "/clip.mov"
	if err := os.WriteFile(path, clips.PiP, 0o600); err != nil {
		t.Fatal(err)
	}
	second := t.TempDir() + "/second.mov"
	if err := os.WriteFile(second, clips.PiP, 0o600); err != nil {
		t.Fatal(err)
	}
	var active, restarted, stopped atomic.Bool
	var ref core.Ref
	app := func(cx *core.Scope) core.Node {
		started := false
		replaced := false
		source := core.NewSignal(cx, path)
		stop := func() {
			cx.AfterFunc(300*time.Millisecond, func() { ref.StopPictureInPicture(); ref.StopPictureInPicture() })
		}
		return core.Video{Ref: &ref, SourceFunc: source.Get, Controls: mode != "false", Muted: true, Repeat: true,
			PictureInPicture: true, AutoPictureInPicture: true,
			Style: core.Style{Width: core.Pt(320), Height: core.Pt(180)},
			OnPictureInPicture: func(s core.PictureInPictureState) {
				t.Logf("PiP state: %+v", s)
				if !s.Supported || s.Error != "" {
					t.Errorf("native AVKit supports PiP but video reported %+v", s)
					cx.Post(cx.Quit)
					return
				}
				if s.Active && !active.Swap(true) {
					if mode == "replace" {
						cx.AfterFunc(300*time.Millisecond, func() { replaced, started = true, false; source.Set(second) })
					} else {
						stop()
					}
				} else if s.Active && replaced && !restarted.Swap(true) {
					stop()
				} else if !s.Active && active.Load() && (mode != "replace" || restarted.Load()) {
					stopped.Store(true)
					cx.Post(cx.Quit)
				} else if s.Possible && !started {
					started = true
					cx.Post(func() { ref.StartPictureInPicture(); ref.StartPictureInPicture() })
				}
			},
			OnError: func(err error) { t.Error(err); cx.Post(cx.Quit) },
		}
	}
	watchdog := time.AfterFunc(30*time.Second, func() { panic("the Mac video did not enter and leave Picture in Picture") })
	defer watchdog.Stop()
	onMain(func() {
		Run(func(cx *core.Scope) core.Node { cx.SetMainView(app); return nil }, Options{Title: "Video Picture in Picture", Width: 360, Height: 240})
	})
	if !active.Load() || !stopped.Load() {
		t.Fatalf("PiP entered=%v, stopped=%v", active.Load(), stopped.Load())
	}
	if mode == "replace" && !restarted.Load() {
		t.Fatal("replacement source did not enter PiP")
	}
}
