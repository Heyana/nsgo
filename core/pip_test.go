package core

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestAndroidVideoPictureInPictureKeepsCustomContentUnsupported(t *testing.T) {
	for _, video := range []bool{true, false} {
		var ref Ref
		states := 0
		h := newHost(t, Size{Width: 400, Height: 300}, func(cx *Scope) Node {
			if video {
				return Video{Ref: &ref, PictureInPicture: true, OnPictureInPicture: func(PictureInPictureState) { states++ }}
			}
			return PictureInPicture{Ref: &ref, OnState: func(PictureInPictureState) { states++ }}
		})
		h.tree.SetEnvironment(Environment{OS: OSAndroid})
		h.commit()
		ref.StartPictureInPicture()
		batch := h.commit()
		commands := 0
		for _, item := range batch.Items {
			if item.Kind == driver.ItemPictureInPictureCommand {
				commands++
			}
		}
		if video && (commands != 1 || states != 0) || !video && (commands != 0 || states == 0) {
			t.Fatalf("video %v: commands %d, unsupported states %d", video, commands, states)
		}
	}
}

func TestPictureInPictureHasIndependentLayoutAndOwner(t *testing.T) {
	var ref Ref
	var visible *Signal[bool]
	var message *Signal[string]
	h := newHost(t, Size{Width: 800, Height: 600}, func(cx *Scope) Node {
		visible = NewSignal(cx, true)
		message = NewSignal(cx, "First")
		return View{Children: []Node{
			Text{Content: "Inline", TestID: "inline"},
			Show{When: visible.Get, Then: func(*Scope) Node {
				return PictureInPicture{
					Ref: &ref, Size: Size{Width: 240, Height: 120}, TestID: "pip",
					Children: []Node{Text{ContentFunc: message.Get, TestID: "frame"}},
				}
			}},
		}}
	})
	h.tree.SetEnvironment(Environment{OS: OSIOS})
	b := h.commit()
	pip := h.find("pip")
	child := h.find("frame")
	if pip.frame.Size != (Size{Width: 240, Height: 120}) {
		t.Fatalf("PiP frame %v", pip.frame)
	}
	if child.frame.Size.Width <= 0 || child.frame.Size.Width > 240 {
		t.Fatalf("content frame %v", child.frame)
	}
	for _, it := range b.Items {
		if it.Kind == driver.ItemInsert && it.ID == pip.id {
			t.Fatal("PiP tree joined the inline hierarchy")
		}
	}
	message.Set("Second")
	h.commit()
	if text := h.find("frame").text.content; text != "Second" {
		t.Fatalf("content %q", text)
	}
	ref.StartPictureInPicture()
	b = h.commit()
	if kinds(b)[driver.ItemPictureInPictureCommand] != 1 {
		t.Fatal("missing start command")
	}
	visible.Set(false)
	b = h.commit()
	if ref.n != nil || kinds(b)[driver.ItemDelete] == 0 {
		t.Fatal("disposing the owner kept its PiP tree")
	}
	ref.StartPictureInPicture()
	if kinds(h.commit())[driver.ItemPictureInPictureCommand] != 0 {
		t.Fatal("disposed ref started PiP")
	}
}

func TestPictureInPictureControlsAndRestore(t *testing.T) {
	var ref Ref
	var paused *Signal[bool]
	var skips []time.Duration
	var restores int
	var states []PictureInPictureState
	h := newHost(t, Size{Width: 300, Height: 400}, func(cx *Scope) Node {
		paused = NewSignal(cx, false)
		return PictureInPicture{Ref: &ref, TestID: "pip", PausedFunc: paused.Get,
			OnPlay: func() { paused.Set(false) }, OnPause: func() { paused.Set(true) }, OnSkip: func(d time.Duration) { skips = append(skips, d) },
			OnRestore: func() { restores++ }, OnState: func(s PictureInPictureState) { states = append(states, s) },
		}
	})
	h.tree.SetEnvironment(Environment{OS: OSIOS})
	h.commit()
	send := func(e driver.PictureInPictureEvent) {
		h.tree.Dispatch(driver.Event{Kind: driver.EventNative, ID: ref.n.id, Text: driver.PictureInPictureEventName, Data: &e})
	}
	send(driver.PictureInPictureEvent{Action: "pause"})
	h.commit()
	if !ref.n.pip.config.Paused {
		t.Fatal("native pause did not update the session")
	}
	send(driver.PictureInPictureEvent{Action: "play"})
	h.commit()
	if ref.n.pip.config.Paused {
		t.Fatal("native play did not update the session")
	}
	send(driver.PictureInPictureEvent{Action: "skip", Time: -15 * time.Second})
	send(driver.PictureInPictureEvent{State: PictureInPictureState{Supported: true, Possible: true, Active: true}})
	send(driver.PictureInPictureEvent{Action: "restore", Token: 42})
	b := h.commit()
	if len(skips) != 1 || skips[0] != -15*time.Second || restores != 1 || len(states) != 1 || !states[0].Active {
		t.Fatalf("skips %v, restores %d, states %v", skips, restores, states)
	}
	found := false
	for _, it := range b.Items {
		if it.Kind == driver.ItemPictureInPictureCommand && it.Count == driver.PictureInPictureRestored && it.Before == 42 {
			found = true
		}
	}
	if !found {
		t.Fatal("restoration was not acknowledged in the resulting commit")
	}
}

func TestVideoPictureInPictureKeepsItsOptions(t *testing.T) {
	var ref Ref
	var states []PictureInPictureState
	var pauses []VideoPauseReason
	h := newHost(t, Size{Width: 300, Height: 400}, func(cx *Scope) Node {
		return Video{Ref: &ref, Source: "asset:video.mp4", Controls: true, PictureInPicture: true, AutoPictureInPicture: true,
			OnPictureInPicture: func(s PictureInPictureState) { states = append(states, s) },
			OnPause:            func(r VideoPauseReason) { pauses = append(pauses, r) }}
	})
	h.tree.SetEnvironment(Environment{OS: OSIOS})
	b := h.commit()
	if kinds(b)[driver.ItemVideo] != 1 || !ref.n.video.config.PictureInPicture || !ref.n.video.config.AutoPictureInPicture {
		t.Fatal("PiP options were lost")
	}
	if m := ref.n.video.events(); !m.Has(driver.EventNative) || !m.Has(driver.EventVideoPause) {
		t.Fatalf("events %b do not hear PiP's state and the window's pauses", m)
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventNative, ID: ref.n.id, Text: driver.PictureInPictureEventName, Data: &driver.PictureInPictureEvent{State: PictureInPictureState{Supported: true, Possible: true}}})
	// The PiP window's pause is the player's, reported as its controls' (apple.Video's rate observer).
	h.tree.Dispatch(driver.Event{Kind: driver.EventVideoPause, ID: ref.n.id, Index: driver.VideoPauseControls})
	if len(states) != 1 || !states[0].Possible || len(pauses) != 1 || pauses[0] != VideoPauseControls {
		t.Fatalf("states %v, pauses %v", states, pauses)
	}
}

func TestMacVideoPictureInPictureCommands(t *testing.T) {
	var video, custom Ref
	var states []PictureInPictureState
	h := newHost(t, Size{Width: 320, Height: 240}, func(cx *Scope) Node {
		return View{Children: []Node{
			Video{Ref: &video, Source: "asset:video.mp4", PictureInPicture: true,
				OnPictureInPicture: func(s PictureInPictureState) { states = append(states, s) }},
			PictureInPicture{Ref: &custom},
		}}
	})
	h.tree.SetEnvironment(Environment{OS: OSMacOS})
	h.commit()
	if len(states) != 0 {
		t.Fatalf("core reported Mac video unsupported before its backend answered: %v", states)
	}
	video.StartPictureInPicture()
	video.StopPictureInPicture()
	custom.StartPictureInPicture()
	b := h.commit()
	var commands []uint32
	for _, item := range b.Items {
		if item.Kind == driver.ItemPictureInPictureCommand {
			if item.ID != video.n.id {
				t.Fatal("Mac custom content sent a native PiP command")
			}
			commands = append(commands, item.Count)
		}
	}
	if len(commands) != 2 || commands[0] != driver.PictureInPictureStart || commands[1] != driver.PictureInPictureStop {
		t.Fatalf("Mac video commands = %v", commands)
	}
}

func TestWebVideoPictureInPictureDoesNotEnableCustomContent(t *testing.T) {
	var video, custom Ref
	var states []PictureInPictureState
	h := newHost(t, Size{Width: 300, Height: 400}, func(cx *Scope) Node {
		return View{Children: []Node{
			Video{Ref: &video, PictureInPicture: true, OnPictureInPicture: func(s PictureInPictureState) { states = append(states, s) }},
			PictureInPicture{Ref: &custom},
		}}
	})
	h.tree.SetEnvironment(Environment{OS: OSWeb})
	h.commit()
	if len(states) != 0 {
		t.Fatal("core preempted the browser's support detection")
	}
	video.StartPictureInPicture()
	video.StopPictureInPicture()
	custom.StartPictureInPicture()
	b := h.commit()
	if got := kinds(b)[driver.ItemPictureInPictureCommand]; got != 2 {
		t.Fatalf("got %d commands, want only the video's start and stop", got)
	}
	for _, it := range b.Items {
		if it.Kind == driver.ItemPictureInPictureCommand && it.ID != video.n.id {
			t.Fatal("web custom content started PiP")
		}
	}
}

func TestPictureInPictureDurationFuncAndVideoSubtitles(t *testing.T) {
	var pipRef, videoRef Ref
	var duration, position *Signal[time.Duration]
	var paused *Signal[bool]
	var subtitle *Signal[string]
	h := newHost(t, Size{Width: 390, Height: 844}, func(cx *Scope) Node {
		duration = NewSignal(cx, time.Duration(0))
		position = NewSignal(cx, time.Duration(0))
		paused = NewSignal(cx, false)
		subtitle = NewSignal(cx, "Line one")
		return View{Children: []Node{
			PictureInPicture{
				Ref: &pipRef, TestID: "pip", Size: Size{Width: 320, Height: 180},
				PausedFunc: paused.Get, PositionFunc: position.Get, DurationFunc: duration.Get,
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
				Children: []Node{
					Video{
						Ref: &videoRef, TestID: "pip-video", Source: "asset:film.mp4",
						PausedFunc: paused.Get,
						Style:      Style{Width: Pct(100), Height: Pct(100)},
						OnLoad:     func(info VideoInfo) { duration.Set(info.Duration) },
						OnProgress: func(pos, _ time.Duration) { position.Set(pos) },
					},
					Text{
						TestID:      "pip-subtitle",
						ContentFunc: subtitle.Get,
						Style:       Style{Position: Absolute, Bottom: Pt(8), Left: Pt(8), Right: Pt(8)},
					},
				},
			},
		}}
	})
	h.tree.SetEnvironment(Environment{OS: OSIOS})
	h.commit()
	if pipRef.n.pip.config.Duration != 0 {
		t.Fatalf("initial duration = %v, want 0", pipRef.n.pip.config.Duration)
	}
	if video := h.find("pip-video"); video.frame.Size != (Size{Width: 320, Height: 180}) {
		t.Fatalf("pip video frame = %+v, want 320x180", video.frame)
	}
	// Video load feeds DurationFunc dynamically without remounting PiP.
	h.tree.Dispatch(driver.Event{Kind: driver.EventLoad, ID: videoRef.n.id, Duration: 90 * time.Second})
	h.tree.Dispatch(driver.Event{Kind: driver.EventProgress, ID: videoRef.n.id, Time: 12 * time.Second, Duration: 90 * time.Second})
	subtitle.Set("Line two")
	b := h.commit()
	if got := pipRef.n.pip.config; got.Duration != 90*time.Second || got.Position != 12*time.Second {
		t.Fatalf("pip config = %+v, want duration 90s and position 12s", got)
	}
	if got := h.find("pip-subtitle").text.content; got != "Line two" {
		t.Fatalf("subtitle = %q, want Line two", got)
	}
	var updated bool
	for _, it := range b.Items {
		if it.Kind == driver.ItemPictureInPicture && it.ID == pipRef.n.id && it.PictureInPicture.Duration == 90*time.Second {
			updated = true
		}
	}
	if !updated {
		t.Fatal("DurationFunc update did not emit ItemPictureInPicture")
	}
	// Negative duration/position clamp to zero.
	duration.Set(-5 * time.Second)
	position.Set(-2 * time.Second)
	h.commit()
	if got := pipRef.n.pip.config; got.Duration != 0 || got.Position != 0 {
		t.Fatalf("negative duration/position not clamped: %+v", got)
	}
}
