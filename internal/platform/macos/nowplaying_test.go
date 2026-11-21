//go:build darwin && !ios

package macos

import (
	"image"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func TestNowPlayingMediaPlayer(t *testing.T) {
	path := silence(t, time.Second)
	onMain(func() {
		loadAppKit()
		pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
		defer pool.Send(sel("drain"))
		p := loadMediaPlayer()
		if p.info == 0 || p.commands == 0 {
			t.Fatal("MediaPlayer centers are unavailable")
		}
		owner, other := &surface{}, &surface{}
		defer owner.clearNowPlaying()
		defer other.clearNowPlaying()
		makeVideo := func() (*apple.Video, func()) {
			holder := objc.ID(apple.Class("NSView")).Send(selNew)
			video := apple.NewVideo(holder)
			video.Dispatch = func(driver.Event) {}
			video.Configure(&driver.VideoConfig{Source: path, Controls: true, Paused: true, Rate: 1, ProgressInterval: time.Second})
			if video.View() == 0 {
				holder.Send(selRelease)
				t.Fatal("no AVPlayerView for controlled video")
			}
			return video, func() { video.Close(); holder.Send(selRelease) }
		}
		before, closeBefore := makeVideo()
		defer closeBefore()
		updates := func(v *apple.Video) bool {
			return objc.Send[bool](v.View(), sel("updatesNowPlayingInfoCenter"))
		}
		if !updates(before) {
			t.Fatal("a controlled video initially cannot publish")
		}
		update := &driver.NowPlayingUpdate{
			At: time.Now().Add(-3 * time.Second), HandlesCommands: true,
			Info: driver.NowPlaying{
				Title: "Test Track", Artist: "Test Artist", Album: "Test Album", Rate: 2,
				Position: 4 * time.Second, Duration: time.Minute,
				Artwork:  &driver.ImageSource{Pixels: image.NewRGBA(image.Rect(0, 0, 16, 9))},
				Commands: []driver.MediaCommandKind{driver.MediaPlay, driver.MediaSeek},
			},
		}
		owner.setNowPlaying(1, update)
		after, closeAfter := makeVideo()
		defer closeAfter()
		if updates(before) || updates(after) {
			t.Error("a controlled video can overwrite the manual publication")
		}
		other.clearNowPlaying()
		info := func(key objc.ID) objc.ID {
			return p.info.Send(sel("nowPlayingInfo")).Send(sel("objectForKey:"), key)
		}
		for key, want := range map[objc.ID]string{p.key.title: "Test Track", p.key.artist: "Test Artist", p.key.album: "Test Album"} {
			if got := apple.GoString(info(key)); got != want {
				t.Errorf("metadata = %q, want %q", got, want)
			}
		}
		if got := objc.Send[float64](info(p.key.elapsed), sel("doubleValue")); got < 10 || got > 4+2*time.Since(update.At).Seconds() {
			t.Errorf("elapsed playback time did not advance from its four-second starting position: %.2f", got)
		}
		if got := objc.Send[uint](p.info, sel("playbackState")); got != playbackPlaying {
			t.Errorf("playbackState = %d, want playing", got)
		}
		for _, control := range nowPlaying.controls {
			want := control.kind == driver.MediaPlay || control.kind == driver.MediaSeek
			if enabled := objc.Send[bool](control.command, sel("isEnabled")); enabled != want || (control.token != 0) != want {
				t.Errorf("command %d enabled = %t, want %t", control.kind, enabled, want)
			}
		}
		loop := objc.ID(apple.Class("NSRunLoop")).Send(sel("currentRunLoop"))
		for until := time.Now().Add(3 * time.Second); info(p.key.artwork) == 0 && time.Now().Before(until); {
			date := objc.ID(apple.Class("NSDate")).Send(sel("dateWithTimeIntervalSinceNow:"), 0.01)
			loop.Send(sel("runUntilDate:"), date)
		}
		art := info(p.key.artwork)
		if art == 0 {
			t.Fatal("the native info center never received artwork")
		}
		art.Send(sel("retain"))
		defer art.Send(selRelease)
		paused := *update
		paused.At = time.Now()
		paused.Info = driver.CloneNowPlaying(update.Info)
		paused.Info.Rate = 0
		paused.Info.Position = 15 * time.Second
		owner.setNowPlaying(2, &paused)
		if got := info(p.key.artwork); got != art {
			t.Errorf("changing playback replaced the artwork: got %v, want %v", got, art)
		}
		if got := objc.Send[float64](info(p.key.elapsed), sel("doubleValue")); got != 15 {
			t.Errorf("updated elapsed time = %v, want 15", got)
		}
		changed := *update
		changed.At = time.Now()
		changed.Info.Rate, changed.Info.Duration, changed.Info.Artwork = 0, 0, nil
		changed.HandlesCommands = false
		other.setNowPlaying(1, &changed)
		owner.clearNowPlaying()
		if got := objc.Send[uint](p.info, sel("playbackState")); got != playbackPaused {
			t.Errorf("playbackState = %d, want paused", got)
		}
		if info(p.key.duration) != 0 || !objc.Send[bool](info(p.key.live), sel("boolValue")) {
			t.Error("the new publication did not describe a live stream")
		}
		if info(p.key.artwork) != 0 {
			t.Error("a new publication without artwork kept the old image")
		}
		for _, control := range nowPlaying.controls {
			if control.token != 0 || objc.Send[bool](control.command, sel("isEnabled")) {
				t.Errorf("command %d is enabled without a handler", control.kind)
			}
		}
		requested := apple.CGSize{Width: 8, Height: 4}
		img := art.Send(sel("imageWithSize:"), requested)
		if size := objc.Send[apple.CGSize](img, sel("size")); img == 0 || size != requested {
			t.Errorf("retained artwork returned image %v size %v, want %v", img, size, requested)
		}
		other.clearNowPlaying()
		if p.info.Send(sel("nowPlayingInfo")) != 0 || objc.Send[uint](p.info, sel("playbackState")) != playbackStopped {
			t.Error("clearing left native metadata or the playback state")
		}
		if !updates(before) || !updates(after) {
			t.Error("clearing did not restore automatic video publication")
		}
	})
}
