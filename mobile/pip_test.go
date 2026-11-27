package mobile_test

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/mobile"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestPictureInPictureLifecycle(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("headless AVKit model; native delegates are covered in apple tests")
	}
	var ref core.Ref
	var paused *core.Signal[bool]
	var visible *core.Signal[bool]
	var state mobile.PictureInPictureState
	var restored int
	var skipped time.Duration
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		paused = core.NewSignal(cx, false)
		visible = core.NewSignal(cx, true)
		return core.View{Children: []core.Node{
			core.View{TestID: "start", Style: core.Style{Width: core.Pt(100), Height: core.Pt(30)}, OnPress: func(core.PressEvent) { ref.StartPictureInPicture(); ref.StartPictureInPicture() }},
			core.View{TestID: "stop", Style: core.Style{Width: core.Pt(100), Height: core.Pt(30)}, OnPress: func(core.PressEvent) { ref.StopPictureInPicture(); ref.StopPictureInPicture() }},
			core.View{TestID: "remove", Style: core.Style{Width: core.Pt(100), Height: core.Pt(30)}, OnPress: func(core.PressEvent) { visible.Set(false) }},
			core.Show{When: visible.Get, Then: func(*core.Scope) core.Node {
				return mobile.PictureInPicture{
					Ref: &ref, TestID: "pip", Size: core.Size{Width: 256, Height: 128}, PausedFunc: paused.Get,
					OnState: func(s mobile.PictureInPictureState) { state = s }, OnRestore: func() { restored++ },
					OnPlay: func() { paused.Set(false) }, OnPause: func() { paused.Set(true) }, OnSkip: func(d time.Duration) { skipped = d },
					Children: []core.Node{core.Text{Content: "Script", TestID: "script"}},
				}
			}},
		}}
	}, nsgotest.Environment(core.Environment{OS: core.OSIOS}), nsgotest.PictureInPictureSupport(true, true))
	if !state.Supported || !state.Possible {
		t.Fatalf("initial state %+v", state)
	}
	if app.Exists("id:script") {
		t.Fatal("PiP content appeared inline")
	}
	if frame := app.FindMounted("id:pip").Frame.Size; frame != (core.Size{Width: 256, Height: 128}) {
		t.Fatalf("frame %+v", frame)
	}
	app.Press("id:start")
	if !state.Active || app.FindMounted("id:pip").PiPStarts != 1 {
		t.Fatal("duplicate start made two sessions")
	}
	app.PictureInPictureControl("id:pip", "pause", 0)
	if !app.FindMounted("id:pip").PictureInPicture.Paused {
		t.Fatal("pause did not update delegate state")
	}
	app.PictureInPictureControl("id:pip", "skip", -15*time.Second)
	if skipped != -15*time.Second {
		t.Fatalf("skip %v", skipped)
	}
	app.SetEnvironment(core.Environment{OS: core.OSIOS, Lifecycle: core.LifecycleBackground})
	if !state.Active {
		t.Fatal("backgrounding ended PiP")
	}
	app.PictureInPictureControl("id:pip", "restore", 0)
	if restored != 1 || app.FindMounted("id:pip").PiPRestores != 1 {
		t.Fatal("restore was not acknowledged")
	}
	app.Press("id:stop")
	if state.Active || app.FindMounted("id:pip").PiPStops != 1 {
		t.Fatal("duplicate stop was not idempotent")
	}
	app.Press("id:start")
	app.Press("id:remove")
	if len(app.QueryMounted("id:pip")) != 0 || len(app.QueryMounted("id:script")) != 0 {
		t.Fatal("owner deletion leaked the PiP tree")
	}
}

func TestPictureInPictureUnavailableAndNotReady(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("headless support model")
	}
	for _, support := range []bool{false, true} {
		t.Run(map[bool]string{false: "unsupported", true: "not-ready"}[support], func(t *testing.T) {
			var ref core.Ref
			var state mobile.PictureInPictureState
			app := nsgotest.Start(t, func(*core.Scope) core.Node {
				return core.View{Children: []core.Node{
					core.View{TestID: "start", Style: core.Style{Width: core.Pt(100), Height: core.Pt(30)}, OnPress: func(core.PressEvent) { ref.StartPictureInPicture() }},
					mobile.PictureInPicture{Ref: &ref, TestID: "pip", OnState: func(s mobile.PictureInPictureState) { state = s }},
				}}
			}, nsgotest.Environment(core.Environment{OS: core.OSIOS}), nsgotest.PictureInPictureSupport(support, false))
			app.Press("id:start")
			if state.Active || state.Supported != support || (state.Error != "") != support {
				t.Fatalf("state %+v", state)
			}
		})
	}
}

func TestPictureInPictureVideoWithSubtitles(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("headless AVKit model; native video composition is covered in apple and examples/pip tests")
	}
	var pipRef, videoRef core.Ref
	var state mobile.PictureInPictureState
	var subtitle *core.Signal[string]
	var restored int
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		paused := core.NewSignal(cx, false)
		position := core.NewSignal(cx, time.Duration(0))
		duration := core.NewSignal(cx, time.Duration(0))
		subtitle = core.NewSignal(cx, "Original line")
		return core.View{Children: []core.Node{
			core.View{TestID: "start", Style: core.Style{Width: core.Pt(100), Height: core.Pt(30)}, OnPress: func(core.PressEvent) { pipRef.StartPictureInPicture() }},
			mobile.PictureInPicture{
				Ref: &pipRef, TestID: "pip", Size: core.Size{Width: 320, Height: 180}, Automatically: true,
				PausedFunc: paused.Get, PositionFunc: position.Get, DurationFunc: duration.Get,
				OnState:   func(s mobile.PictureInPictureState) { state = s },
				OnRestore: func() { restored++ },
				OnPlay:    func() { paused.Set(false) },
				OnPause:   func() { paused.Set(true) },
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
						Ref: &videoRef, TestID: "video", Source: "asset:lesson.mp4",
						PausedFunc: paused.Get, Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)},
						OnLoad:     func(info core.VideoInfo) { duration.Set(info.Duration) },
						OnProgress: func(pos, _ time.Duration) { position.Set(pos) },
						OnPlay:     func() { paused.Set(false) },
						OnPause:    func(core.VideoPauseReason) { paused.Set(true) },
					},
					core.Text{
						TestID:      "subtitle",
						ContentFunc: subtitle.Get,
						Style:       core.Style{Position: core.Absolute, Bottom: core.Pt(8), Left: core.Pt(8), Right: core.Pt(8)},
					},
				},
			},
		}}
	}, nsgotest.Environment(core.Environment{OS: core.OSIOS}), nsgotest.PictureInPictureSupport(true, true))
	app.Play("id:video", core.VideoInfo{Duration: 2 * time.Minute, Size: core.Size{Width: 1920, Height: 1080}})
	if got := app.FindMounted("id:pip").PictureInPicture.Duration; got != 2*time.Minute {
		t.Fatalf("PiP duration = %v, want 2m", got)
	}
	app.Progress("id:video", 30*time.Second, 2*time.Minute)
	if got := app.FindMounted("id:pip").PictureInPicture.Position; got != 30*time.Second {
		t.Fatalf("PiP position = %v, want 30s", got)
	}
	app.Press("id:start")
	if !state.Active {
		t.Fatalf("PiP did not activate: %+v", state)
	}
	app.SetEnvironment(core.Environment{OS: core.OSIOS, Lifecycle: core.LifecycleBackground})
	app.Post(func() { subtitle.Set("Translated line") })
	app.Settle()
	if got := app.FindMounted("id:subtitle").Text; got != "Translated line" {
		t.Fatalf("subtitle in background PiP = %q, want Translated line", got)
	}
	app.PictureInPictureControl("id:pip", "skip", 15*time.Second)
	if seeks := app.FindMounted("id:video").Seeks; len(seeks) != 1 || seeks[0] != 45*time.Second {
		t.Fatalf("video seeks = %v, want [45s]", seeks)
	}
	app.PictureInPictureControl("id:pip", "pause", 0)
	if !app.FindMounted("id:video").Video.Paused {
		t.Fatal("PiP pause did not pause embedded video")
	}
	app.PictureInPictureControl("id:pip", "play", 0)
	if app.FindMounted("id:video").Video.Paused {
		t.Fatal("PiP play did not resume embedded video")
	}
	app.PauseVideo("id:video", core.VideoPauseInterruption)
	if !app.FindMounted("id:pip").PictureInPicture.Paused {
		t.Fatal("system interruption on video did not update PiP paused state")
	}
	app.PlayVideo("id:video")
	if app.FindMounted("id:pip").PictureInPicture.Paused {
		t.Fatal("system resume on video did not update PiP paused state")
	}
	app.PictureInPictureControl("id:pip", "restore", 0)
	if restored != 1 || app.FindMounted("id:pip").PiPRestores != 1 {
		t.Fatalf("restored = %d, acks = %d", restored, app.FindMounted("id:pip").PiPRestores)
	}
}
