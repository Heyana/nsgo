package core

import (
	"math"
	"slices"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// NowPlaying describes the app's current media for system controls.
// Duration is zero for a live stream. Position is measured when
// SetNowPlaying is called. Rate is zero while paused and one at normal
// speed. Commands lists the controls to offer. Artwork is optional.
type NowPlaying = driver.NowPlaying

// MediaCommandKind identifies a system media control.
type MediaCommandKind = driver.MediaCommandKind

const (
	MediaPlay            = driver.MediaPlay
	MediaPause           = driver.MediaPause
	MediaTogglePlayPause = driver.MediaTogglePlayPause
	MediaStop            = driver.MediaStop
	MediaNext            = driver.MediaNext
	MediaPrevious        = driver.MediaPrevious
	MediaSeek            = driver.MediaSeek
	MediaSkipForward     = driver.MediaSkipForward
	MediaSkipBackward    = driver.MediaSkipBackward
)

// MediaCommand is a request from a system media control. Position is
// the requested playback time for MediaSeek.
type MediaCommand = driver.MediaCommand

type mediaHandler struct {
	owner *Scope
	fn    func(MediaCommand)
}

// SetNowPlaying replaces the app's system media information. Call it
// when playback, position or rate changes. The system advances the
// published position between calls. Artwork is copied before returning.
// On a platform without Env.CanShowNowPlaying this does nothing.
func (cx *Scope) SetNowPlaying(info NowPlaying) {
	rt := cx.rt
	t := rt.tree
	if t == nil || !t.reported.NowPlaying {
		return
	}
	at := time.Now()
	if info.Artwork != nil {
		src := t.pickScale(*info.Artwork)
		if src.Pixels == nil && len(src.Data) == 0 && cx.gate(serviceImage, src.URI) != nil {
			info.Artwork = nil
		} else {
			info.Artwork = &src
		}
	}
	info = driver.CloneNowPlaying(info)
	info.Duration = max(info.Duration, 0)
	info.Position = max(info.Position, 0)
	if info.Duration > 0 {
		info.Position = min(info.Position, info.Duration)
	}
	if math.IsNaN(float64(info.Rate)) || math.IsInf(float64(info.Rate), 0) {
		info.Rate = 0
	}
	commands := info.Commands[:0]
	for _, kind := range info.Commands {
		if kind >= MediaPlay && kind <= MediaSkipBackward && !slices.Contains(commands, kind) {
			commands = append(commands, kind)
		}
	}
	info.Commands = commands
	rt.nowPlaying = &driver.NowPlayingUpdate{Info: info, At: at}
	rt.publishNowPlaying()
}

// ClearNowPlaying removes the app's system media information and controls.
func (cx *Scope) ClearNowPlaying() {
	if rt := cx.rt; rt.nowPlaying != nil {
		rt.nowPlaying = nil
		rt.publishNowPlaying()
	}
}

// OnMediaCommand handles commands on the engine goroutine while its
// owner lives. The most recently registered live handler takes them.
// Without a handler the publication offers no controls.
func (cx *Scope) OnMediaCommand(fn func(MediaCommand)) {
	rt := cx.rt
	if fn == nil || rt.tree == nil || !rt.tree.reported.NowPlaying {
		return
	}
	h := &mediaHandler{owner: cx.current(), fn: fn}
	rt.mediaHandlers = append(rt.mediaHandlers, h)
	if rt.nowPlaying != nil {
		rt.publishNowPlaying()
	}
	h.owner.addCleanup(func() {
		latest := h == rt.mediaHandlers[len(rt.mediaHandlers)-1]
		rt.mediaHandlers = slices.DeleteFunc(rt.mediaHandlers, func(entry *mediaHandler) bool { return entry == h })
		if latest && rt.nowPlaying != nil {
			rt.publishNowPlaying()
		}
	})
}

func (rt *runtime) publishNowPlaying() {
	if rt.root.disposed {
		return
	}
	rt.mediaRevision++
	item := driver.MountItem{Kind: driver.ItemNowPlaying, Count: rt.mediaRevision}
	if rt.nowPlaying != nil {
		publication := *rt.nowPlaying
		publication.HandlesCommands = len(rt.mediaHandlers) != 0
		item.NowPlaying = &publication
	}
	rt.tree.commands = append(rt.tree.commands, item)
	rt.tree.changed = true
}

func (rt *runtime) mediaCommand(ev driver.Event) {
	command, ok := ev.Data.(driver.MediaCommand)
	if !ok || ev.Count != rt.mediaRevision || rt.nowPlaying == nil || len(rt.mediaHandlers) == 0 ||
		!slices.Contains(rt.nowPlaying.Info.Commands, command.Kind) {
		return
	}
	h := rt.mediaHandlers[len(rt.mediaHandlers)-1]
	rt.tree.within(h.owner, func() { h.fn(command) })
}
