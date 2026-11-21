package driver

import "maps"

// VideoHeaders is a snapshot of the HTTP headers for a video source. Keeping
// it behind a pointer lets playback-only updates compare a VideoConfig
// without rebuilding or comparing its headers.
type VideoHeaders struct {
	fields map[string]string
}

// NewVideoHeaders copies the fields so a mount batch can keep them.
func NewVideoHeaders(fields map[string]string) *VideoHeaders {
	if len(fields) == 0 {
		return nil
	}
	return &VideoHeaders{fields: maps.Clone(fields)}
}

// Len returns how many request headers the snapshot holds.
func (h *VideoHeaders) Len() int {
	if h == nil {
		return 0
	}
	return len(h.fields)
}

// All yields the header names and values in unspecified order.
func (h *VideoHeaders) All(yield func(name, value string) bool) {
	if h != nil {
		maps.All(h.fields)(yield)
	}
}
