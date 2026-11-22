package driver

import "time"

// PictureInPictureConfig describes a custom-content session, whose view
// is a detached layout root. Video sessions use VideoConfig instead.
type PictureInPictureConfig struct {
	Size          Size
	Paused        bool
	Duration      time.Duration // zero is a live stream
	Position      time.Duration
	Automatically bool
}

// PictureInPictureState is the native session's state. Possible says
// whether the platform can start it now; Unsupported is distinct from failure.
type PictureInPictureState struct {
	Supported, Possible, Active bool
	Error                       string
	FrameInterval               time.Duration
}

// PictureInPictureEvent uses EventNative, independent of EventMask's
// fixed-size input bits. Action is empty for state updates.
type PictureInPictureEvent struct {
	State  PictureInPictureState
	Action string // restore, play, pause, skip
	Time   time.Duration
	Token  uint32
}

const PictureInPictureEventName = "nsgo:picture-in-picture"

const (
	PictureInPictureStart uint32 = iota + 1
	PictureInPictureStop
	PictureInPictureRestored
)
