//go:build android || nsgoandroidhost

package android

import "github.com/nsgo-dev/nsgo/internal/driver"

// A runtime built before native video PiP consumes the reserved command
// safely but cannot answer it. Report unavailable instead of leaving the
// app waiting for a state that will never arrive.
func (s *surface) reportUnsupportedPiP(surface uint32, b *driver.MountBatch) {
	if s.videoPiP.Load() {
		return
	}
	for _, item := range b.Items {
		if item.Kind == driver.ItemVideo && item.Video.PictureInPicture || item.Kind == driver.ItemPictureInPictureCommand {
			s.dispatchTo(surface, driver.Event{Kind: driver.EventNative, ID: item.ID, Text: driver.PictureInPictureEventName,
				Data: &driver.PictureInPictureEvent{}})
		}
	}
}
