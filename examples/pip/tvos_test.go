//go:build ios && nsgotest

package main

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/videoalpha/clips"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestTVVideoPictureInPicture(t *testing.T) { testTVVideoPictureInPicture(t, false) }

func TestTVControlledVideoPictureInPicture(t *testing.T) { testTVVideoPictureInPicture(t, true) }

func testTVVideoPictureInPicture(t *testing.T, controls bool) {
	if nsgotest.Target() != "tvos_simulator" {
		t.Skip("requires tvOS")
	}
	source := filepath.Join(t.TempDir(), "pip.mov")
	if err := os.WriteFile(source, clips.PiP, 0o600); err != nil {
		t.Fatal(err)
	}
	var ref core.Ref
	var loaded atomic.Bool
	var state atomic.Pointer[core.PictureInPictureState]
	var failure atomic.Pointer[string]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			core.Video{
				TestID: "video", Ref: &ref, Source: source, Controls: controls,
				PictureInPicture: true, AutoPictureInPicture: true, Repeat: true, Muted: true,
				Style:              core.Style{Width: core.Pt(320), Height: core.Pt(180)},
				OnLoad:             func(core.VideoInfo) { loaded.Store(true) },
				OnError:            func(err error) { message := err.Error(); failure.Store(&message) },
				OnPictureInPicture: func(s core.PictureInPictureState) { state.Store(&s) },
			},
			base.Button{TestID: "start", Label: "Start Picture in Picture", Style: core.Style{Width: core.Pt(200), Height: core.Pt(48)}, OnPress: func(core.PressEvent) { ref.StartPictureInPicture() }},
			base.Button{TestID: "stop", Label: "Stop Picture in Picture", Style: core.Style{Width: core.Pt(200), Height: core.Pt(48)}, OnPress: func(core.PressEvent) { ref.StopPictureInPicture() }},
		}}
	})
	app.WaitUntil(15*time.Second, func() bool { return loaded.Load() && state.Load() != nil || failure.Load() != nil })
	if message := failure.Load(); message != nil {
		t.Fatal(*message)
	}
	s := state.Load()
	if s.Error != "" {
		t.Fatal(s.Error)
	}
	if !s.Supported {
		app.Press("id:start")
		app.Settle()
		if state.Load().Active {
			t.Fatal("an unsupported simulator entered PiP")
		}
		t.Log("video loaded with native controls configured; AVKit reports PiP unsupported on this simulator, so entry requires physical Apple TV verification")
		return
	}
	app.WaitUntil(15*time.Second, func() bool { s := state.Load(); return s.Possible || s.Error != "" })
	if s := state.Load(); s.Error != "" {
		t.Fatal(s.Error)
	}
	app.Press("id:start")
	app.WaitUntil(15*time.Second, func() bool { s := state.Load(); return s.Active || s.Error != "" })
	if s := state.Load(); s.Error != "" || !s.Active {
		t.Fatalf("native PiP did not start: %+v", *s)
	}
	app.Press("id:stop")
	app.WaitUntil(15*time.Second, func() bool { return !state.Load().Active })
}
