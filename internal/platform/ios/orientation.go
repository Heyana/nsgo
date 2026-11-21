//go:build ios

package ios

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func (s *surface) setOrientation(mask driver.Orientation) {
	if s.embedded || s.env.OS != driver.OSIOS || s.env.Idiom == driver.IdiomTV || s.requestedOrientation == mask {
		return
	}
	if s.requestedOrientation == 0 && mask != 0 && s.previousOrientation == 0 && s.modalOrientations() == 0 {
		s.previousOrientation = sceneOrientation(s.uiWindow().Send(sel("windowScene")))
	}
	s.requestedOrientation = mask
	s.updateOrientations()
}

// updateSceneOrientation asks UIKit to rotate immediately when the current
// geometry falls outside the app's request. A modal's own orientation policy
// continues to take precedence.
func (s *surface) updateSceneOrientation() {
	if s.modalOrientations() != 0 {
		return
	}
	window := s.uiWindow()
	scene := window.Send(sel("windowScene"))
	request := sel("requestGeometryUpdateWithPreferences:errorHandler:")
	if scene == 0 {
		return
	}
	if !objc.Send[bool](scene, sel("respondsToSelector:"), request) {
		s.previousOrientation = 0
		return
	}
	current := sceneOrientation(scene)
	mask := interfaceOrientationMask(s.requestedOrientation)
	if mask != 0 {
		if s.previousOrientation == 0 {
			s.previousOrientation = current
		}
	} else {
		mask = s.previousOrientation
		s.previousOrientation = 0
		// The app may have changed its usual supported orientations.
		app := objc.ID(apple.Class("UIApplication")).Send(sel("sharedApplication"))
		mask &= objc.Send[uint](app, sel("supportedInterfaceOrientationsForWindow:"), window)
	}
	if mask == 0 || current&mask != 0 {
		return
	}
	preferences := objc.ID(apple.Class("UIWindowSceneGeometryPreferencesIOS")).Send(selAlloc).Send(sel("initWithInterfaceOrientations:"), mask)
	scene.Send(request, preferences, objc.ID(0))
	preferences.Send(selRelease)
}

func sceneOrientation(scene objc.ID) uint {
	orientation := objc.Send[uint](scene, sel("interfaceOrientation"))
	if orientation == 0 {
		return 0
	}
	return 1 << orientation
}
