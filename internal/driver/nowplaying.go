package driver

import (
	"maps"
	"slices"
	"time"
)

// NowPlaying is the metadata the platform presents for an app's media.
type NowPlaying struct {
	Title, Artist, Album string
	Artwork              *ImageSource
	Duration, Position   time.Duration
	Rate                 float32
	Commands             []MediaCommandKind
}

// CloneNowPlaying snapshots mutable artwork and command data.
func CloneNowPlaying(info NowPlaying) NowPlaying {
	info.Commands = slices.Clone(info.Commands)
	if info.Artwork != nil {
		src := *info.Artwork
		src.Data = slices.Clone(src.Data)
		src.Pixels = CopyPixels(src.Pixels)
		src.Variants = maps.Clone(src.Variants)
		info.Artwork = &src
	}
	return info
}

// MediaCommandKind identifies a system transport control.
type MediaCommandKind uint8

const (
	MediaPlay MediaCommandKind = iota + 1
	MediaPause
	MediaTogglePlayPause
	MediaStop
	MediaNext
	MediaPrevious
	MediaSeek
	MediaSkipForward
	MediaSkipBackward
)

// MediaCommand is a request from a system transport control.
type MediaCommand struct {
	Kind     MediaCommandKind
	Position time.Duration // for MediaSeek
}

// NowPlayingUpdate is what ItemNowPlaying publishes. At anchors Position,
// and HandlesCommands says a live callback can take the listed commands.
type NowPlayingUpdate struct {
	Info            NowPlaying
	At              time.Time
	HandlesCommands bool
}
