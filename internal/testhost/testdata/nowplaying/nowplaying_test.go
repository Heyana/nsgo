package nowplaying

import (
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestNowPlayingFromMainWindow(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		if !cx.Env().CanShowNowPlaying() {
			return core.Text{Content: "Unavailable"}
		}
		position := core.NewSignal(cx, time.Duration(0))
		cx.OnMediaCommand(func(command core.MediaCommand) {
			position.Set(command.Position)
			cx.ClearNowPlaying()
		})
		cx.SetNowPlaying(core.NowPlaying{
			Title: "Test Track", Duration: time.Minute,
			Commands: []core.MediaCommandKind{core.MediaSeek},
		})
		return core.Text{ContentFunc: func() string { return position.Get().String() }}
	}, nsgotest.Environment(core.Environment{OS: core.OSMacOS, NowPlaying: true}))
	if app.Exists("text:Unavailable") {
		t.Skip("the platform has no Now Playing support")
	}
	if got := app.NowPlaying(); got == nil || got.Title != "Test Track" {
		t.Fatalf("missing Now Playing publication: %+v", got)
	}
	app.MediaCommand(core.MediaCommand{Kind: core.MediaPause})
	app.Find("text:0s")
	app.MediaCommand(core.MediaCommand{Kind: core.MediaSeek, Position: 12 * time.Second})
	app.Find("text:12s")
	if app.NowPlaying() != nil {
		t.Fatal("clearing Now Playing left a publication")
	}
}
