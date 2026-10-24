package main

import (
	"fmt"
	"time"

	"github.com/oneo-me/nsgo/core"
)

// flowerClip is a five-second CC0 clip from MDN's examples.
const flowerClip = "https://interactive-examples.mdn.mozilla.net/media/cc0-videos/flower.mp4"

func videoPage(cx *core.Scope, t *theme) core.Node {
	var player core.Ref
	paused := core.NewSignal(cx, true)
	muted := core.NewSignal(cx, true)
	fit := core.NewSignal(cx, core.Contain)
	status := core.NewSignal(cx, "loading")
	buffering := core.NewSignal(cx, false)
	position := core.NewSignal(cx, [2]time.Duration{})
	frame := core.Style{Height: core.Pt(220), CornerRadius: 6, Overflow: core.Hidden, Background: core.RGB(0x000000)}
	fits := []struct {
		name string
		mode core.ResizeMode
	}{{"Contain", core.Contain}, {"Cover", core.Cover}, {"Stretch", core.Stretch}}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Playback", "The app drives the player: play and pause, mute, seek and how the picture fits.",
			core.Video{Source: flowerClip, PausedFunc: paused.Get, MutedFunc: muted.Get, Repeat: true,
				ResizeModeFunc: fit.Get, Ref: &player, TestID: "player", Style: frame,
				OnLoad: func(i core.VideoInfo) {
					status.Set(fmt.Sprintf("loaded: %v, %v×%v", i.Duration.Round(10*time.Millisecond), i.Size.Width, i.Size.Height))
				},
				OnProgress: func(p, d time.Duration) { position.Set([2]time.Duration{p, d}) },
				OnBuffer: func(b bool) {
					if b {
						status.Set("buffering")
					}
				},
				OnError: func(err error) { status.Set("error: " + err.Error()) }},
			hstack(
				t.primary("Play / pause", func() { paused.Set(!paused.Get()) }),
				t.plain("Mute", func() { muted.Set(!muted.Get()) }),
				t.plain("Start over", func() { player.Seek(0) }),
			),
			hstack(func() []core.Node {
				var ns []core.Node
				for _, f := range fits {
					ns = append(ns, t.plain(f.name, func() { fit.Set(f.mode) }))
				}
				return ns
			}()...),
			t.live(func() string {
				if buffering.Get() {
					return "buffering"
				}
				return status.Get()
			}),
			t.live(func() string {
				p := position.Get()
				return fmt.Sprintf("%v of %v", p[0].Round(100*time.Millisecond), p[1].Round(100*time.Millisecond))
			}),
		),
		t.demo("Platform controls", "Controls shows the platform's playback controls; Windows draws its own bar.",
			core.Video{Source: flowerClip, Paused: true, Controls: true, TestID: "controls", Style: frame},
		),
	}}
}
