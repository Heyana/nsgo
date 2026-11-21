//go:build darwin && !ios

package macos

import (
	"context"
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

const (
	playbackPlaying = 1 // MPNowPlayingPlaybackState
	playbackPaused  = 2
	playbackStopped = 3
)

type nowPlayingKeys struct {
	title, artist, album, artwork, duration, elapsed, rate, live objc.ID
}

type mediaPlayer struct {
	info, commands, artworkClass objc.ID
	key                          nowPlayingKeys
}

var loadMediaPlayer = sync.OnceValue(func() mediaPlayer {
	lib, err := purego.Dlopen("/System/Library/Frameworks/MediaPlayer.framework/MediaPlayer", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return mediaPlayer{}
	}
	key := func(name string) objc.ID {
		addr, err := purego.Dlsym(lib, name)
		if err != nil {
			return 0
		}
		return **(**objc.ID)(unsafe.Pointer(&addr))
	}
	return mediaPlayer{
		info:         objc.ID(apple.Class("MPNowPlayingInfoCenter")).Send(sel("defaultCenter")),
		commands:     objc.ID(apple.Class("MPRemoteCommandCenter")).Send(sel("sharedCommandCenter")),
		artworkClass: objc.ID(apple.Class("MPMediaItemArtwork")),
		key: nowPlayingKeys{
			title:    key("MPMediaItemPropertyTitle"),
			artist:   key("MPMediaItemPropertyArtist"),
			album:    key("MPMediaItemPropertyAlbumTitle"),
			artwork:  key("MPMediaItemPropertyArtwork"),
			duration: key("MPMediaItemPropertyPlaybackDuration"),
			elapsed:  key("MPNowPlayingInfoPropertyElapsedPlaybackTime"),
			rate:     key("MPNowPlayingInfoPropertyPlaybackRate"),
			live:     key("MPNowPlayingInfoPropertyIsLiveStream"),
		},
	}
})

type mediaControl struct {
	kind       driver.MediaCommandKind
	command    objc.ID
	token      objc.ID
	wasEnabled bool
}

// The system has one default info center and one command center per
// process, shared by all surfaces and AVPlayerViews. Only the current
// owner can clear them. The main thread owns the publication.
var nowPlaying nowPlayingState

type nowPlayingState struct {
	owner    *surface
	update   *driver.NowPlayingUpdate
	input    *mediaInput
	controls []mediaControl
	artwork  objc.ID
	cancel   context.CancelFunc
}

type mediaInput struct {
	revision uint32
	dispatch func(driver.Event)
	active   atomic.Bool
}

func (m *mediaInput) send(command driver.MediaCommand) bool {
	if !m.active.Load() {
		return false
	}
	m.dispatch(driver.Event{Kind: driver.EventMediaCommand, Count: m.revision, Data: command})
	return true
}

func (s *surface) setNowPlaying(revision uint32, update *driver.NowPlayingUpdate) {
	if update == nil {
		s.clearNowPlaying()
		return
	}
	p := loadMediaPlayer()
	if s.closed || p.info == 0 {
		return
	}
	first := nowPlaying.owner == nil
	sameOwner := nowPlaying.owner == s
	sameMedia := sameOwner && nowPlaying.update.At == update.At
	sameArtwork := sameOwner && sameArtworkSource(nowPlaying.update.Info.Artwork, update.Info.Artwork)
	if first {
		apple.SetManualNowPlaying(true)
		nowPlaying.controls = p.mediaControls()
	}
	nowPlaying.removeTargets()
	nowPlaying.owner, nowPlaying.update = s, update
	input := &mediaInput{revision: revision, dispatch: s.engine.Dispatch}
	input.active.Store(true)
	nowPlaying.input = input
	for i := range nowPlaying.controls {
		control := &nowPlaying.controls[i]
		enabled := control.kind != 0 && update.HandlesCommands && slices.Contains(update.Info.Commands, control.kind)
		if enabled {
			control.token = addMediaTarget(control.command, control.kind, input)
			enabled = control.token != 0
		}
		control.command.Send(sel("setEnabled:"), enabled)
	}
	if sameMedia {
		return
	}
	if !sameArtwork {
		nowPlaying.releaseArtwork()
	}
	p.publish(update, nowPlaying.artwork)
	if src := update.Info.Artwork; src != nil && nowPlaying.artwork == 0 && nowPlaying.cancel == nil {
		ctx, cancel := context.WithCancel(context.Background())
		nowPlaying.cancel = cancel
		go func() {
			image, size := apple.LoadStillImage(ctx, *src)
			apple.OnMain(func() {
				if ctx.Err() != nil {
					apple.Release(image)
					return
				}
				nowPlaying.cancel = nil
				cancel()
				if image != 0 {
					nowPlaying.artwork = p.newArtwork(image, size)
					p.publish(nowPlaying.update, nowPlaying.artwork)
				}
			})
		}()
	}
}

// The engine copies the artwork on each publication, so compare its content.
func sameArtworkSource(a, b *driver.ImageSource) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Pixels != nil || b.Pixels != nil {
		return a.Pixels != nil && b.Pixels != nil &&
			a.Pixels.Rect == b.Pixels.Rect && slices.Equal(a.Pixels.Pix, b.Pixels.Pix)
	}
	return a.URI == b.URI && slices.Equal(a.Data, b.Data)
}

func (s *surface) clearNowPlaying() {
	if nowPlaying.owner != s {
		return
	}
	nowPlaying.removeTargets()
	for _, control := range nowPlaying.controls {
		control.command.Send(sel("setEnabled:"), control.wasEnabled)
	}
	p := loadMediaPlayer()
	p.info.Send(sel("setNowPlayingInfo:"), objc.ID(0))
	p.info.Send(sel("setPlaybackState:"), uint(playbackStopped))
	nowPlaying.releaseArtwork()
	nowPlaying.owner, nowPlaying.update, nowPlaying.input, nowPlaying.controls = nil, nil, nil, nil
	apple.SetManualNowPlaying(false)
}

func (n *nowPlayingState) removeTargets() {
	if n.input != nil {
		n.input.active.Store(false)
	}
	for i := range n.controls {
		control := &n.controls[i]
		if control.token != 0 {
			control.command.Send(sel("removeTarget:"), control.token)
			control.token.Send(selRelease)
			control.token = 0
		}
	}
}

func (n *nowPlayingState) releaseArtwork() {
	if n.cancel != nil {
		n.cancel()
		n.cancel = nil
	}
	if n.artwork != 0 {
		n.artwork.Send(selRelease)
		n.artwork = 0
	}
}

func (p mediaPlayer) mediaControls() []mediaControl {
	var controls []mediaControl
	for _, entry := range []struct {
		name string
		kind driver.MediaCommandKind
	}{
		{"playCommand", driver.MediaPlay},
		{"pauseCommand", driver.MediaPause},
		{"togglePlayPauseCommand", driver.MediaTogglePlayPause},
		{"stopCommand", driver.MediaStop},
		{"nextTrackCommand", driver.MediaNext},
		{"previousTrackCommand", driver.MediaPrevious},
		{"changePlaybackPositionCommand", driver.MediaSeek},
		{"skipForwardCommand", driver.MediaSkipForward},
		{"skipBackwardCommand", driver.MediaSkipBackward},
		{"enableLanguageOptionCommand", 0},
		{"disableLanguageOptionCommand", 0},
		{"changePlaybackRateCommand", 0},
		{"changeRepeatModeCommand", 0},
		{"changeShuffleModeCommand", 0},
		{"seekForwardCommand", 0},
		{"seekBackwardCommand", 0},
		{"ratingCommand", 0},
		{"likeCommand", 0},
		{"dislikeCommand", 0},
		{"bookmarkCommand", 0},
	} {
		if command := p.commands.Send(sel(entry.name)); command != 0 {
			controls = append(controls, mediaControl{
				kind: entry.kind, command: command, wasEnabled: objc.Send[bool](command, sel("isEnabled")),
			})
		}
	}
	return controls
}

func addMediaTarget(command objc.ID, kind driver.MediaCommandKind, input *mediaInput) objc.ID {
	const success, commandFailed = 0, 200 // MPRemoteCommandHandlerStatus
	block := objcblock.New(func(_ objc.Block, event objc.ID) int {
		request := driver.MediaCommand{Kind: kind}
		if kind == driver.MediaSeek {
			seconds := objc.Send[float64](event, sel("positionTime"))
			if math.IsNaN(seconds) || seconds < 0 || seconds >= float64(math.MaxInt64)/float64(time.Second) {
				return commandFailed
			}
			request.Position = time.Duration(seconds * float64(time.Second))
		}
		if input.send(request) {
			return success
		}
		return commandFailed
	})
	defer block.Release()
	return command.Send(sel("addTargetWithHandler:"), block).Send(sel("retain"))
}

func (p mediaPlayer) publish(update *driver.NowPlayingUpdate, artwork objc.ID) {
	m := update.Info
	dict := objc.ID(apple.Class("NSMutableDictionary")).Send(sel("dictionary"))
	set := func(key, value objc.ID) {
		if key != 0 && value != 0 {
			dict.Send(sel("setObject:forKey:"), value, key)
		}
	}
	number := func(n float64) objc.ID {
		return objc.ID(apple.Class("NSNumber")).Send(sel("numberWithDouble:"), n)
	}
	set(p.key.title, apple.NSString(m.Title))
	set(p.key.artist, apple.NSString(m.Artist))
	set(p.key.album, apple.NSString(m.Album))
	set(p.key.artwork, artwork)
	elapsed := max(0, m.Position.Seconds()+time.Since(update.At).Seconds()*float64(m.Rate))
	if m.Duration > 0 {
		elapsed = min(elapsed, m.Duration.Seconds())
		set(p.key.duration, number(m.Duration.Seconds()))
	} else {
		set(p.key.live, objc.ID(apple.Class("NSNumber")).Send(sel("numberWithBool:"), true))
	}
	set(p.key.elapsed, number(elapsed))
	set(p.key.rate, number(float64(m.Rate)))
	p.info.Send(sel("setNowPlayingInfo:"), dict)
	state := playbackPaused
	if m.Rate != 0 {
		state = playbackPlaying
	}
	p.info.Send(sel("setPlaybackState:"), uint(state))
}

// newArtwork takes ownership of image. The system can copy the request
// block or keep the artwork after the next publication.
func (p mediaPlayer) newArtwork(image uintptr, size apple.CGSize) objc.ID {
	block := objcblock.NewWithCleanup(func(_ objc.Block, requested apple.CGSize) objc.ID {
		if requested.Width <= 0 || requested.Height <= 0 {
			requested = size
		}
		return objc.ID(apple.Class("NSImage")).Send(sel("alloc")).Send(sel("initWithCGImage:size:"), image, requested).Send(sel("autorelease"))
	}, func() { apple.Release(image) })
	defer block.Release()
	return p.artworkClass.Send(sel("alloc")).Send(sel("initWithBoundsSize:requestHandler:"), size, block)
}
