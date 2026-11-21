//go:build js && wasm

package web

import "syscall/js"

type displayHold struct {
	surface *surface
	id      uint32
}

// screenAwake shares one browser sentinel among the page's display holds.
var screenAwake struct {
	holds      map[displayHold]struct{}
	events     listeners
	lock       js.Value
	pending    bool
	retry      bool
	generation uint64
}

func (s *surface) keepDisplayAwake(id uint32) {
	hold := displayHold{s, id}
	if _, exists := screenAwake.holds[hold]; exists {
		return
	}
	if len(screenAwake.holds) == 0 {
		screenAwake.holds = make(map[displayHold]struct{})
		doc := s.doc
		screenAwake.events.on(doc, "visibilitychange", func(js.Value) {
			screenAwake.generation++
			updateScreenWakeLock(doc)
		})
		// WebKit requires transient activation for the first grant.
		activate := func(e js.Value) {
			if !e.Get("isTrusted").Bool() {
				return
			}
			activation := js.Global().Get("navigator").Get("userActivation")
			if activation.Truthy() && !activation.Get("isActive").Bool() {
				return
			}
			if screenAwake.pending {
				screenAwake.retry = true
				return
			}
			updateScreenWakeLock(doc)
		}
		screenAwake.events.on(doc, "pointerup", activate)
		screenAwake.events.on(doc, "keydown", activate)
	}
	screenAwake.holds[hold] = struct{}{}
	updateScreenWakeLock(s.doc)
}

func (s *surface) releaseDisplayAwake(id uint32) {
	hold := displayHold{s, id}
	if _, exists := screenAwake.holds[hold]; exists {
		delete(screenAwake.holds, hold)
		endScreenWakeLock(s.doc)
	}
}

func (s *surface) clearDisplayHolds() {
	if len(screenAwake.holds) == 0 {
		return
	}
	for hold := range screenAwake.holds {
		if hold.surface == s {
			delete(screenAwake.holds, hold)
		}
	}
	endScreenWakeLock(s.doc)
}

func endScreenWakeLock(doc js.Value) {
	if len(screenAwake.holds) == 0 {
		screenAwake.events.release()
		screenAwake.holds = nil
		screenAwake.generation++
		updateScreenWakeLock(doc)
	}
}

func updateScreenWakeLock(doc js.Value) {
	a := &screenAwake
	if len(a.holds) == 0 || doc.Get("visibilityState").String() != "visible" {
		if a.lock.Truthy() {
			a.lock.Call("release").Call("catch", ignore)
			a.lock = js.Undefined()
		}
		return
	}
	if a.pending || a.lock.Truthy() && !a.lock.Get("released").Bool() {
		return
	}
	api := js.Global().Get("navigator").Get("wakeLock")
	if !api.Truthy() || api.Get("request").Type() != js.TypeFunction {
		return
	}
	a.pending = true
	generation := a.generation
	then(api.Call("request", "screen"), func(lock js.Value) {
		a.pending, a.retry = false, false
		if generation == a.generation {
			a.lock = lock
			return
		}
		// A request can settle after its holds ended or its page hid.
		lock.Call("release").Call("catch", ignore)
		updateScreenWakeLock(doc)
	}, func(js.Value) {
		retry := a.retry || generation != a.generation
		a.pending, a.retry = false, false
		if retry {
			updateScreenWakeLock(doc)
		}
	})
}
