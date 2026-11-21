package nsgotest

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// NowPlaying returns a copy of the last system media publication, or nil
// when the app has none.
func (a *App) NowPlaying() *core.NowPlaying {
	if a.nowPlaying == nil {
		return nil
	}
	info := driver.CloneNowPlaying(a.nowPlaying.Info)
	return &info
}

// MediaCommand sends a system media command to the app's publication.
// It works headless and on targets that support Now Playing.
func (a *App) MediaCommand(command core.MediaCommand) {
	a.step("MediaCommand %d", command.Kind)
	defer a.done()
	if a.nowPlaying == nil {
		return
	}
	if a.target != nil {
		if surface, ok := a.target.surface.(testhost.MediaSurface); ok {
			surface.MediaCommand(a.mediaRevision, command)
		}
	} else {
		a.dispatchOn(a.mediaTree, driver.Event{Kind: driver.EventMediaCommand, Count: a.mediaRevision, Data: command})
	}
}
