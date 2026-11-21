//go:build js && wasm

package web

import (
	"log"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type browserScreen struct {
	orientation driver.Orientation
	lockID      uint64
	lockAsked   bool
	entering    bool
	// leave is an exit asked while an entry was under way, made once the entry settles.
	leave  bool
	legacy bool
}

func (s *surface) browserFullscreen() bool {
	return s.doc.Get("fullscreenElement").Truthy() || s.doc.Get("webkitFullscreenElement").Truthy()
}

// toggleBrowserFullscreen calls the browser during the triggering press's
// commit. The browser can consume that press's activation here.
func (s *surface) toggleBrowserFullscreen() { s.setBrowserFullscreen(!s.browserFullscreen()) }

// setBrowserFullscreen enters or leaves the browser's full screen, doing nothing when the page already is as asked. It calls the browser during the triggering press's commit, as entry needs the press's activation.
func (s *surface) setBrowserFullscreen(enter bool) {
	if s.screen.entering {
		s.screen.leave = !enter
		return
	}
	if enter == s.browserFullscreen() {
		return
	}
	target, method, legacy, action := s.doc, "exitFullscreen", "webkitExitFullscreen", "exit full screen"
	if enter {
		target, method, legacy, action = s.doc.Get("documentElement"), "requestFullscreen", "webkitRequestFullscreen", "enter full screen"
	}
	if target.Get(method).Type() != js.TypeFunction {
		method = legacy
		if target.Get(method).Type() != js.TypeFunction {
			log.Printf("nsgo: %s: unavailable in this browser", action)
			return
		}
	}
	promise, ok := tryCall(target, method)
	if !ok {
		log.Printf("nsgo: %s: the browser refused the request", action)
		return
	}
	s.screen.entering = enter
	if promise.Type() != js.TypeObject || promise.Get("then").Type() != js.TypeFunction {
		// Older WebKit reports entry through its prefixed events.
		s.screen.legacy = enter
		return
	}
	then(promise, func(js.Value) {
		if enter {
			s.enteredBrowserFullscreen()
		}
	}, func(err js.Value) {
		if enter {
			s.screen.entering, s.screen.leave = false, false
		}
		log.Printf("nsgo: %s: %v", action, jsError(err))
	})
}

// enteredBrowserFullscreen ends an entry, leaving again when an exit was asked meanwhile.
func (s *surface) enteredBrowserFullscreen() {
	s.screen.entering = false
	if s.screen.leave {
		s.screen.leave = false
		s.setBrowserFullscreen(false)
		return
	}
	s.lockOrientation()
}

func (s *surface) screenChanged(js.Value) {
	fullscreen := s.browserFullscreen()
	if !fullscreen {
		s.unlockOrientation()
	}
	if s.screen.legacy {
		s.screen.legacy = false
		if fullscreen {
			s.enteredBrowserFullscreen()
		} else {
			s.screen.entering, s.screen.leave = false, false
		}
	}
}

func (s *surface) screenFailed(js.Value) {
	if s.screen.legacy {
		log.Print("nsgo: enter full screen: the browser refused the request")
		s.screen.entering, s.screen.legacy, s.screen.leave = false, false, false
	}
}

func (s *surface) setOrientation(mask driver.Orientation) {
	s.screen.orientation = mask
	if mask == 0 {
		s.unlockOrientation()
		return
	}
	s.lockOrientation()
}

func (s *surface) lockOrientation() {
	mask := s.screen.orientation
	if mask == 0 || s.screen.entering {
		return
	}
	orientation := js.Global().Get("screen").Get("orientation")
	if !orientation.Truthy() || orientation.Get("lock").Type() != js.TypeFunction {
		log.Print("nsgo: lock screen orientation: unavailable in this browser")
		return
	}
	want := browserOrientation(mask, orientation.Get("type").String())
	promise, ok := tryCall(orientation, "lock", want)
	if !ok {
		log.Print("nsgo: lock screen orientation: the browser refused the request")
		return
	}
	s.screen.lockID++
	s.screen.lockAsked = true
	id := s.screen.lockID
	then(promise, func(js.Value) {}, func(err js.Value) {
		if s.screen.lockID == id {
			log.Printf("nsgo: lock screen orientation: %v", jsError(err))
		}
	})
}

func (s *surface) unlockOrientation() {
	if !s.screen.lockAsked {
		return
	}
	s.screen.lockID++
	s.screen.lockAsked = false
	orientation := js.Global().Get("screen").Get("orientation")
	if orientation.Get("unlock").Type() == js.TypeFunction {
		if _, ok := tryCall(orientation, "unlock"); !ok {
			log.Print("nsgo: unlock screen orientation: the browser refused the request")
		}
	}
}

func browserOrientation(mask driver.Orientation, current string) string {
	const portrait = driver.OrientationPortrait | driver.OrientationPortraitUpsideDown
	switch mask {
	case portrait | driver.OrientationLandscape:
		return "any"
	case portrait:
		return "portrait"
	case driver.OrientationLandscape:
		return "landscape"
	}
	first := ""
	for _, direction := range []struct {
		flag driver.Orientation
		name string
	}{
		{driver.OrientationPortrait, "portrait-primary"},
		{driver.OrientationPortraitUpsideDown, "portrait-secondary"},
		{driver.OrientationLandscapeLeft, "landscape-primary"},
		{driver.OrientationLandscapeRight, "landscape-secondary"},
	} {
		if mask&direction.flag != 0 {
			if direction.name == current {
				return current
			}
			if first == "" {
				first = direction.name
			}
		}
	}
	return first
}
