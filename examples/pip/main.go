// Command pip demonstrates native video and a captioned video tree in iOS
// Picture in Picture. Run with go tool nsgo dev -target ios examples/pip.
package main

import (
	"fmt"
	"time"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/mobile"
	"github.com/nsgo-dev/nsgo/ui"
)

func main() { nsgo.Run(Root, nsgo.WindowSize(420, 640)) }

func Root(cx *core.Scope) core.Node {
	const clip = "https://interactive-examples.mdn.mozilla.net/media/cc0-videos/flower.mp4"
	video, script, captionVideo := &core.Ref{}, &core.Ref{}, &core.Ref{}
	videoState := core.NewSignal(cx, core.PictureInPictureState{})
	scriptState := core.NewSignal(cx, mobile.PictureInPictureState{})
	scriptPaused := core.NewSignal(cx, false)
	scriptPosition := core.NewSignal(cx, time.Duration(0))
	scriptDuration := core.NewSignal(cx, time.Duration(0))
	text := core.NewSignal(cx, "Welcome to the live stream")
	line := core.NewSignal(cx, 1)
	status := func(s core.PictureInPictureState) string {
		if !s.Supported {
			return "Picture in Picture is unavailable on this system"
		}
		if s.Error != "" {
			return s.Error
		}
		if s.Active {
			return "Picture in Picture is active"
		}
		if s.Possible {
			return "Ready for Picture in Picture"
		}
		return "Preparing Picture in Picture"
	}
	return core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
		core.View{Style: core.Style{Padding: core.All(24), Gap: 12}, Children: []core.Node{
			core.Text{Content: "Video", Style: ui.TypeHeading},
			core.Video{Ref: video, Source: clip,
				Controls: true, Repeat: true, Muted: true, PictureInPicture: true, AutoPictureInPicture: true,
				OnPictureInPicture: videoState.Set,
				Style:              core.Style{Height: core.Pt(180)}, TestID: "video"},
			core.Text{ContentFunc: func() string { return status(videoState.Get()) }, TestID: "video-status"},
			ui.Button{Text: "Start video PiP", OnPress: video.StartPictureInPicture, Disabled: func() bool { return !videoState.Get().Possible }, TestID: "start-video"},
			ui.Button{Text: "Stop video PiP", OnPress: video.StopPictureInPicture},
			core.Text{Content: "Script", Style: ui.TypeHeading},
			ui.Input{Value: text.Get, OnChange: text.Set, TestID: "script-text"},
			ui.Button{Text: "Next line", OnPress: func() { line.Update(func(n int) int { return n + 1 }) }},
			core.Text{ContentFunc: func() string { return status(scriptState.Get()) }, TestID: "script-status"},
			ui.Button{Text: "Start script PiP", OnPress: script.StartPictureInPicture, Disabled: func() bool { return !scriptState.Get().Possible }, TestID: "start-script"},
			ui.Button{Text: "Stop script PiP", OnPress: script.StopPictureInPicture},
		}},
		mobile.PictureInPicture{Ref: script, Size: core.Size{Width: 320, Height: 180}, OnState: scriptState.Set, TestID: "caption-pip",
			PausedFunc: scriptPaused.Get, PositionFunc: scriptPosition.Get, DurationFunc: scriptDuration.Get,
			OnPlay: func() { scriptPaused.Set(false) }, OnPause: func() { scriptPaused.Set(true) },
			OnSkip: func(d time.Duration) {
				position := max(scriptPosition.Get()+d, 0)
				if duration := scriptDuration.Get(); duration > 0 {
					position = min(position, duration)
				}
				scriptPosition.Set(position)
				captionVideo.Seek(position)
			},
			Children: []core.Node{
				core.Video{Ref: captionVideo, Source: clip, PausedFunc: scriptPaused.Get, Muted: true, Repeat: true, TestID: "caption-video",
					ResizeMode: core.Cover, Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)},
					OnLoad:     func(info core.VideoInfo) { scriptDuration.Set(info.Duration) },
					OnProgress: func(position, _ time.Duration) { scriptPosition.Set(position) },
					OnPlay:     func() { scriptPaused.Set(false) },
					OnPause:    func(core.VideoPauseReason) { scriptPaused.Set(true) }},
				core.View{Style: core.Style{
					Position: core.Absolute, Bottom: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0),
					Padding: core.All(12), Gap: 4, Background: core.RGBA(0x172033, .85), Color: core.RGB(0xffffff),
				}, Children: []core.Node{
					core.Text{ContentFunc: func() string { return fmt.Sprintf("Line %d", line.Get()) }},
					core.Text{ContentFunc: text.Get, Style: core.Style{FontSize: 20}},
				}},
			},
		},
	}}
}
