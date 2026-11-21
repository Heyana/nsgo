package nsgotest

import (
	"image"
	"reflect"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestNowPlaying(t *testing.T) {
	if onTarget() {
		t.Skip("this test controls the headless platform's capability")
	}
	pixels := image.NewRGBA(image.Rect(0, 0, 2, 2))
	pixels.Pix[0] = 25
	commands := []core.MediaCommandKind{core.MediaPlay, core.MediaSeek, core.MediaPlay, 255}
	var set, clear, stop func()
	var primary, latest []core.MediaCommand
	app := StartApp(t, func(cx *core.Scope) {
		cx.SetMainView(func(cx *core.Scope) core.Node { return core.Text{Content: "Player"} })
		cx.OnMediaCommand(func(cmd core.MediaCommand) { primary = append(primary, cmd) })
		stop = cx.Effect(func() {
			cx.OnMediaCommand(func(cmd core.MediaCommand) { latest = append(latest, cmd) })
		})
		set = func() {
			cx.SetNowPlaying(core.NowPlaying{
				Title: "Track", Position: 5 * time.Second, Duration: time.Minute, Rate: 1,
				Artwork: &core.ImageSource{Pixels: pixels}, Commands: commands,
			})
		}
		clear = cx.ClearNowPlaying
		set()
	}, Environment(driver.Environment{OS: core.OSMacOS, NowPlaying: true}))
	pixels.Pix[0], commands[0] = 0, core.MediaPause
	info := app.NowPlaying()
	if info == nil || info.Title != "Track" || info.Artwork.Pixels.Pix[0] != 25 ||
		!reflect.DeepEqual(info.Commands, []core.MediaCommandKind{core.MediaPlay, core.MediaSeek}) {
		t.Fatalf("incorrect media snapshot: %+v", info)
	}
	info.Artwork.Pixels.Pix[0], info.Commands[0] = 8, core.MediaStop
	if later := app.NowPlaying(); later.Artwork.Pixels.Pix[0] != 25 || later.Commands[0] != core.MediaPlay {
		t.Fatalf("changing the test's snapshot changed the publication: %+v", later)
	}
	seek := core.MediaCommand{Kind: core.MediaSeek, Position: 18 * time.Second}
	play := core.MediaCommand{Kind: core.MediaPlay}
	app.MediaCommand(core.MediaCommand{Kind: core.MediaPause})
	app.MediaCommand(seek)
	if len(primary) != 0 || !reflect.DeepEqual(latest, []core.MediaCommand{seek}) {
		t.Fatalf("initial routing: primary %v, latest %v", primary, latest)
	}
	stale := app.mediaRevision
	app.Post(stop)
	app.Settle()
	app.dispatchOn(app.mediaTree, driver.Event{Kind: driver.EventMediaCommand, Count: stale, Data: play})
	app.MediaCommand(play)
	if !reflect.DeepEqual(primary, []core.MediaCommand{play}) || len(latest) != 1 {
		t.Fatalf("routing after handler disposal: primary %v, latest %v", primary, latest)
	}
	app.Post(clear)
	app.Settle()
	app.MediaCommand(play)
	if app.NowPlaying() != nil || len(primary) != 1 {
		t.Fatal("clearing left metadata or delivered commands")
	}
	app.Post(set)
	app.Settle()
	if info := app.NowPlaying(); info == nil || info.Commands[0] != core.MediaPause {
		t.Fatalf("a fresh publication did not replace the cleared item: %+v", info)
	}
}

func TestNowPlayingUnavailable(t *testing.T) {
	if onTarget() {
		t.Skip("this test controls the headless platform's capability")
	}
	app := StartApp(t, func(cx *core.Scope) {
		if cx.Env().CanShowNowPlaying() {
			t.Error("the default test platform unexpectedly supports Now Playing")
		}
		cx.SetNowPlaying(core.NowPlaying{Title: "Hidden", Commands: []core.MediaCommandKind{core.MediaPlay}})
		cx.OnMediaCommand(func(core.MediaCommand) { t.Error("an unsupported platform delivered a command") })
	})
	app.MediaCommand(core.MediaCommand{Kind: core.MediaPlay})
	if app.NowPlaying() != nil {
		t.Fatal("an unsupported platform published metadata")
	}
}
