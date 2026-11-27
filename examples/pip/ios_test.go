//go:build ios && nsgotest

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/videoalpha/clips"
	"github.com/nsgo-dev/nsgo/mobile"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestIOSVideoPictureInPicture(t *testing.T) {
	t.Run("inline", func(t *testing.T) { testIOSVideoPictureInPicture(t, false, core.AudioSessionDefault) })
	t.Run("controls", func(t *testing.T) { testIOSVideoPictureInPicture(t, true, core.AudioSessionDefault) })
	t.Run("mix_with_others", func(t *testing.T) { testIOSVideoPictureInPicture(t, false, core.AudioSessionMixWithOthers) })
}

func testIOSVideoPictureInPicture(t *testing.T, controls bool, audio core.AudioSession) {
	if target := nsgotest.Target(); target != "ios_simulator" && target != "ios" {
		t.Skip("requires iOS")
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
				TestID: "video", Ref: &ref, Source: source, Controls: controls, AudioSession: audio,
				PictureInPicture: true, AutoPictureInPicture: true, Repeat: true, Muted: true,
				Style:              core.Style{Width: core.Pt(320), Height: core.Pt(180)},
				OnLoad:             func(core.VideoInfo) { loaded.Store(true) },
				OnError:            func(err error) { message := err.Error(); failure.Store(&message) },
				OnPictureInPicture: func(s core.PictureInPictureState) { state.Store(&s) },
			},
			base.Button{TestID: "start", Label: "Start Video PiP", Style: core.Style{Width: core.Pt(200), Height: core.Pt(48)}, OnPress: func(core.PressEvent) { ref.StartPictureInPicture() }},
			base.Button{TestID: "stop", Label: "Stop Video PiP", Style: core.Style{Width: core.Pt(200), Height: core.Pt(48)}, OnPress: func(core.PressEvent) { ref.StopPictureInPicture() }},
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
		if got := state.Load(); got.Active || got.Supported {
			t.Fatalf("unsupported simulator reported unexpected video PiP state: %+v", *got)
		}
		t.Logf("video loaded (controls=%v audio=%v); AVKit reports PiP unsupported on this iOS simulator", controls, audio)
		return
	}
	app.WaitUntil(15*time.Second, func() bool { s := state.Load(); return s.Possible || s.Error != "" })
	if s := state.Load(); s.Error != "" {
		t.Fatal(s.Error)
	}
	app.Press("id:start")
	app.WaitUntil(15*time.Second, func() bool { s := state.Load(); return s.Active || s.Error != "" })
	if s := state.Load(); s.Error != "" || !s.Active {
		t.Fatalf("native video PiP did not start: %+v", *s)
	}
	app.Press("id:stop")
	app.WaitUntil(15*time.Second, func() bool { return !state.Load().Active })
}

func TestIOSCustomPictureInPicture(t *testing.T) {
	if target := nsgotest.Target(); target != "ios_simulator" && target != "ios" {
		t.Skip("requires iOS")
	}
	var ref core.Ref
	var state atomic.Pointer[mobile.PictureInPictureState]
	var line *core.Signal[int]
	var frameInterval atomic.Int64
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		line = core.NewSignal(cx, 1)
		paused := core.NewSignal(cx, false)
		return core.View{Children: []core.Node{
			base.Button{TestID: "next", Label: "Next line", Style: core.Style{Width: core.Pt(200), Height: core.Pt(48)}, OnPress: func(core.PressEvent) { line.Update(func(n int) int { return n + 1 }) }},
			base.Button{TestID: "start", Label: "Start Custom PiP", Style: core.Style{Width: core.Pt(200), Height: core.Pt(48)}, OnPress: func(core.PressEvent) { ref.StartPictureInPicture() }},
			base.Button{TestID: "stop", Label: "Stop Custom PiP", Style: core.Style{Width: core.Pt(200), Height: core.Pt(48)}, OnPress: func(core.PressEvent) { ref.StopPictureInPicture() }},
			mobile.PictureInPicture{
				Ref: &ref, Size: core.Size{Width: 320, Height: 180}, TestID: "custom-pip",
				PausedFunc: paused.Get, Duration: 30 * time.Second,
				OnState: func(s mobile.PictureInPictureState) {
					state.Store(&s)
					if s.FrameInterval > 0 {
						frameInterval.Store(int64(s.FrameInterval))
					}
				},
				OnPlay:  func() { paused.Set(false) },
				OnPause: func() { paused.Set(true) },
				Children: []core.Node{
					core.View{
						Style: core.Style{FlexGrow: 1, Padding: core.All(12), Background: core.RGB(0x172033)},
						Children: []core.Node{
							core.Text{ContentFunc: func() string { return fmt.Sprintf("Line %d", line.Get()) }, Style: core.Style{Color: core.RGB(0xffffff), FontSize: 20}},
						},
					},
				},
			},
		}}
	})
	app.WaitUntil(10*time.Second, func() bool { return state.Load() != nil })
	s := state.Load()
	if s.Error != "" {
		t.Fatal(s.Error)
	}
	if !s.Supported {
		app.Press("id:next")
		app.Press("id:start")
		app.Settle()
		if got := state.Load(); got.Active || got.Supported {
			t.Fatalf("unsupported simulator reported unexpected custom PiP state: %+v", *got)
		}
		t.Log("custom PiP mounted cleanly; AVKit reports PiP unsupported on this iOS simulator")
		return
	}
	app.WaitUntil(15*time.Second, func() bool { s := state.Load(); return s.Possible || s.Error != "" })
	if s := state.Load(); s.Error != "" {
		t.Fatal(s.Error)
	}
	app.Press("id:start")
	app.WaitUntil(15*time.Second, func() bool { s := state.Load(); return s.Active || s.Error != "" })
	if s := state.Load(); s.Error != "" || !s.Active {
		t.Fatalf("custom PiP did not start: %+v", *s)
	}
	// Drive continuous updates across >1s so AVSampleBufferDisplayLayer renders
	// multiple frames and reports a measured FrameInterval.
	for i := 0; i < 25; i++ {
		app.Post(func() { line.Update(func(n int) int { return n + 1 }) })
		time.Sleep(50 * time.Millisecond)
	}
	app.WaitUntil(3*time.Second, func() bool { return frameInterval.Load() > 0 })
	if interval := time.Duration(frameInterval.Load()); interval <= 0 {
		t.Fatalf("custom PiP did not report a rendered frame interval: %v", interval)
	}
	app.Press("id:stop")
	app.WaitUntil(15*time.Second, func() bool { return !state.Load().Active })
}

func TestIOSCustomVideoPictureInPicture(t *testing.T) {
	if target := nsgotest.Target(); target != "ios_simulator" && target != "ios" {
		t.Skip("requires iOS")
	}
	source := filepath.Join(t.TempDir(), "pip.mov")
	if err := os.WriteFile(source, clips.PiP, 0o600); err != nil {
		t.Fatal(err)
	}
	var pipRef, videoRef core.Ref
	var loaded atomic.Bool
	var state atomic.Pointer[mobile.PictureInPictureState]
	var failure atomic.Pointer[string]
	var frameInterval atomic.Int64
	var line *core.Signal[int]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		line = core.NewSignal(cx, 1)
		paused := core.NewSignal(cx, false)
		position := core.NewSignal(cx, time.Duration(0))
		duration := core.NewSignal(cx, time.Duration(0))
		return core.View{Children: []core.Node{
			base.Button{TestID: "next", Label: "Next line", Style: core.Style{Width: core.Pt(200), Height: core.Pt(48)}, OnPress: func(core.PressEvent) { line.Update(func(n int) int { return n + 1 }) }},
			base.Button{TestID: "start", Label: "Start Captioned PiP", Style: core.Style{Width: core.Pt(200), Height: core.Pt(48)}, OnPress: func(core.PressEvent) { pipRef.StartPictureInPicture() }},
			base.Button{TestID: "stop", Label: "Stop Captioned PiP", Style: core.Style{Width: core.Pt(200), Height: core.Pt(48)}, OnPress: func(core.PressEvent) { pipRef.StopPictureInPicture() }},
			mobile.PictureInPicture{
				Ref: &pipRef, Size: core.Size{Width: 320, Height: 180}, TestID: "captioned-pip",
				PausedFunc: paused.Get, PositionFunc: position.Get, DurationFunc: duration.Get,
				OnState: func(s mobile.PictureInPictureState) {
					state.Store(&s)
					if s.FrameInterval > 0 {
						frameInterval.Store(int64(s.FrameInterval))
					}
				},
				OnPlay:  func() { paused.Set(false) },
				OnPause: func() { paused.Set(true) },
				OnSkip: func(d time.Duration) {
					next := max(position.Get()+d, 0)
					if dur := duration.Get(); dur > 0 {
						next = min(next, dur)
					}
					position.Set(next)
					videoRef.Seek(next)
				},
				Children: []core.Node{
					core.Video{
						Ref: &videoRef, Source: source, PausedFunc: paused.Get, Muted: true, Repeat: true,
						ResizeMode: core.Cover, Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)},
						OnLoad: func(info core.VideoInfo) {
							duration.Set(info.Duration)
							loaded.Store(true)
						},
						OnProgress: func(pos, _ time.Duration) { position.Set(pos) },
						OnError:    func(err error) { message := err.Error(); failure.Store(&message) },
					},
					core.View{
						Style: core.Style{
							Position: core.Absolute, Bottom: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0),
							Padding: core.All(8), Background: core.RGBA(0x172033, .85),
						},
						Children: []core.Node{
							core.Text{ContentFunc: func() string { return fmt.Sprintf("Subtitle %d", line.Get()) }, Style: core.Style{Color: core.RGB(0xffffff), FontSize: 18}},
						},
					},
				},
			},
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
		app.Press("id:next")
		app.Press("id:start")
		app.Settle()
		if got := state.Load(); got.Active || got.Supported {
			t.Fatalf("unsupported simulator reported unexpected captioned video PiP state: %+v", *got)
		}
		t.Log("captioned video PiP loaded cleanly; AVKit reports PiP unsupported on this iOS simulator")
		return
	}
	app.WaitUntil(15*time.Second, func() bool { s := state.Load(); return s.Possible || s.Error != "" })
	if s := state.Load(); s.Error != "" {
		t.Fatal(s.Error)
	}
	app.Press("id:start")
	app.WaitUntil(15*time.Second, func() bool { s := state.Load(); return s.Active || s.Error != "" })
	if s := state.Load(); s.Error != "" || !s.Active {
		t.Fatalf("captioned video PiP did not start: %+v", *s)
	}
	for i := 0; i < 25; i++ {
		app.Post(func() { line.Update(func(n int) int { return n + 1 }) })
		time.Sleep(50 * time.Millisecond)
	}
	app.WaitUntil(3*time.Second, func() bool { return frameInterval.Load() > 0 })
	if interval := time.Duration(frameInterval.Load()); interval <= 0 {
		t.Fatalf("captioned video PiP did not report a rendered frame interval: %v", interval)
	}
	app.Press("id:stop")
	app.WaitUntil(15*time.Second, func() bool { return !state.Load().Active })
}
