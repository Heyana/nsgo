//go:build ios

package ios

import (
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// Associate videos after a mount batch has inserted, moved or removed both
// ends of the relationship. UIView ancestry includes a view's clipping holder.
func (s *surface) updatePictureInPictureVideos() {
	for _, video := range s.videos {
		var owner *apple.PictureInPicture
		for _, root := range s.pips {
			if root.pip.Layer() != 0 && objc.Send[bool](video.native, sel("isDescendantOfView:"), root.native) {
				owner = root.pip
				break
			}
		}
		video.video.SetPictureInPictureOwner(owner, s.wakePictureInPictures)
	}
}

// dirtyPictureInPictures schedules snapshots after tree changes, animation
// steps and image loads, including ones that need no engine commit.
func (s *surface) dirtyPictureInPictures() {
	for _, v := range s.pips {
		v.pip.Dirty()
	}
	s.wakePictureInPictures()
}

func (s *surface) wakePictureInPictures() {
	if s.closed || len(s.pips) == 0 {
		return
	}
	background := appLifecycle() == driver.LifecycleBackground
	for _, v := range s.pips {
		if v.pip.Layer() == 0 {
			continue
		}
		if !background {
			s.pipFramePending = true
			s.startTicking()
			return
		}
		if v.pip.ContinuesInBackground() {
			s.schedulePictureInPictureFrame(time.Second / 60)
			return
		}
	}
}

// UIKit stops its display link in the background. An active PiP session
// uses a single pending timer to read video frames while the app can run.
func (s *surface) schedulePictureInPictureFrame(delay time.Duration) {
	if s.pipTimer != nil {
		return
	}
	var timer *time.Timer
	timer = time.AfterFunc(delay, func() {
		apple.OnMain(func() {
			if s.pipTimer != timer {
				return
			}
			s.pipTimer = nil
			start := time.Now()
			if appLifecycle() == driver.LifecycleBackground && s.drawPictureInPictures() {
				s.schedulePictureInPictureFrame(max(time.Second/60-time.Since(start), time.Millisecond))
			}
		})
	})
	s.pipTimer = timer
}

func (s *surface) stopPictureInPictureTimer() {
	if s.pipTimer != nil {
		s.pipTimer.Stop()
		s.pipTimer = nil
	}
}

// drawPictureInPictures retains one pending frame under back-pressure.
func (s *surface) drawPictureInPictures() bool {
	if len(s.pips) == 0 {
		return false
	}
	background := appLifecycle() == driver.LifecycleBackground
	pending := false
	for _, v := range s.videos {
		if v.video.UpdatePictureInPictureFrame(background) {
			pending = true
		}
	}
	for _, v := range s.pips {
		if background && !v.pip.ContinuesInBackground() {
			continue
		}
		if v.pip.DrawFrame(v.native.Send(selLayer), float64(s.env.PixelRatio)) {
			pending = true
		}
	}
	return pending
}
