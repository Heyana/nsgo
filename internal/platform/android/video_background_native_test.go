//go:build android && nsgotest

package android

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func TestEmbeddedVideoBackgroundPlayback(t *testing.T) {
	// Use an audio-only source, which can play without a TextureView surface.
	path := filepath.Join(t.TempDir(), "background.wav")
	if err := os.WriteFile(path, silentAudioWAV(8*time.Second), 0o600); err != nil {
		t.Fatal(err)
	}
	var scope *core.Scope
	var paused *core.Signal[bool]
	var ref core.Ref
	var progress, loads, ends, interruptions atomic.Int32
	var mediaError atomic.Pointer[string]
	window, err := testhost.Current().Open(func(cx *core.Scope) core.Node {
		scope = cx
		paused = core.NewSignal(cx, false)
		return core.Video{
			Source: path, Repeat: true, PausedFunc: paused.Get, Ref: &ref,
			Style:            core.Style{Width: core.Pt(160), Height: core.Pt(90)},
			ProgressInterval: 50 * time.Millisecond,
			OnLoad:           func(core.VideoInfo) { loads.Add(1) },
			OnProgress:       func(time.Duration, time.Duration) { progress.Add(1) },
			OnEnd:            func() { ends.Add(1) },
			OnPause: func(reason core.VideoPauseReason) {
				if reason == core.VideoPauseInterruption {
					paused.Set(true)
					interruptions.Add(1)
				}
			},
			OnError: func(err error) { message := err.Error(); mediaError.Store(&message) },
		}
	}, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := window.(*testSurface)
	s.Sync()
	do := func(op string, args map[string]any) {
		t.Helper()
		if args == nil {
			args = make(map[string]any)
		}
		args["op"] = op
		if err := s.do(args); err != nil {
			t.Fatal(err)
		}
	}
	wait := func(why string, done func() bool) {
		t.Helper()
		for deadline := time.Now().Add(6 * time.Second); !done(); time.Sleep(10 * time.Millisecond) {
			if message := mediaError.Load(); message != nil {
				t.Fatal(*message)
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: loads=%d progress=%d ends=%d interruptions=%d", why, loads.Load(), progress.Load(), ends.Load(), interruptions.Load())
			}
		}
	}
	visible := func(show bool) { do("view-visibility", map[string]any{"visible": show}) }
	wait("initial playback", func() bool { return progress.Load() > 0 })
	visible(false)
	wait("default visibility pause", func() bool { return interruptions.Load() == 1 })
	visible(true)
	do("background-video-playback", map[string]any{"enabled": true})
	before := progress.Load()
	scope.Post(func() { paused.Set(false) })
	wait("playback resumed", func() bool { return progress.Load() >= before+2 })
	visible(false)
	before = progress.Load()
	wait("progress while hidden", func() bool { return progress.Load() >= before+2 })

	visible(true)
	do("remove-view-from-window", nil)
	detached := true
	t.Cleanup(func() {
		if detached {
			_ = s.do(map[string]any{"op": "return-view-to-window"})
		}
	})
	before = progress.Load()
	wait("progress without a window", func() bool { return progress.Load() >= before+2 })
	end := ends.Load()
	scope.Post(func() { ref.Seek(7700 * time.Millisecond) })
	wait("completion without a window", func() bool { return ends.Load() > end })
	before = progress.Load()
	wait("progress after repeating without a window", func() bool { return progress.Load() >= before+2 })
	do("return-view-to-window", nil)
	detached = false
	before = progress.Load()
	wait("progress after returning to the window", func() bool { return progress.Load() >= before+2 })
	if loads.Load() != 1 || interruptions.Load() != 1 {
		t.Fatalf("retained playback reloaded or paused: loads=%d interruptions=%d", loads.Load(), interruptions.Load())
	}
	data, err := s.ask(map[string]any{"op": "detach-view"})
	if err != nil {
		t.Fatal(err)
	}
	var detachedState nativeViewState
	if err := json.Unmarshal([]byte(data), &detachedState); err != nil {
		t.Fatal(err)
	}
	if !detachedState.Alive || detachedState.Nodes != 0 || detachedState.Children != 0 {
		t.Fatalf("explicit detach kept the native tree: %+v", detachedState)
	}
}
