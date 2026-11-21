//go:build js && wasm

package web

import (
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// The browser grants and releases locks asynchronously. Only one request
// can be in flight for this surface, and its result still has to match
// the app's current request before motion can reach the engine.
type webPointerLock struct {
	want, pending, active *webLockRequest
	exiting               bool
}

type webLockRequest struct {
	token   driver.PointerLock
	element js.Value
	promise bool
}

func webCanLockPointer(doc js.Value) bool {
	return doc.Get("exitPointerLock").Type() == js.TypeFunction &&
		js.Global().Get("Element").Get("prototype").Get("requestPointerLock").Type() == js.TypeFunction
}

func (s *surface) setPointerLock(it *driver.MountItem) {
	p := &s.pointerLock
	token := driver.PointerLock{ID: it.ID, Count: it.Count}
	if !it.On {
		if p.want != nil && p.want.token == token {
			p.want = nil
			s.advancePointerLock()
		}
		return
	}
	v := s.view(it.ID)
	if v == nil || !webCanLockPointer(s.doc) || !v.el.Get("isConnected").Bool() {
		s.engine.Dispatch(token.Unlocked())
		return
	}
	p.want = &webLockRequest{token: token, element: v.el}
	s.advancePointerLock()
}

func (s *surface) advancePointerLock() {
	p := &s.pointerLock
	if p.pending != nil || p.exiting || p.want == p.active {
		return
	}
	if p.active != nil {
		if p.want != nil && p.want.element.Equal(p.active.element) {
			p.active = p.want
			return
		}
		p.exiting = true
		s.doc.Call("exitPointerLock")
		return
	}
	if s.doc.Get("pointerLockElement").Truthy() {
		// Another embedded surface or the embedding page holds it.
		s.losePointerLock()
		return
	}
	request := p.want
	p.pending = request
	defer func() {
		if recover() != nil {
			s.pointerLockError(request)
		}
	}()
	result := request.element.Call("requestPointerLock")
	if result.Type() == js.TypeObject && result.Get("then").Type() == js.TypeFunction {
		request.promise = true
		then(result, func(js.Value) { s.pointerLockChanged() }, func(js.Value) { s.pointerLockError(request) })
	}
}

func (s *surface) pointerLockChanged() {
	p := &s.pointerLock
	element := s.doc.Get("pointerLockElement")
	if p.active != nil && !element.Equal(p.active.element) {
		if p.want == p.active {
			s.engine.Dispatch(p.want.token.Unlocked())
			p.want = nil
		}
		p.active, p.exiting = nil, false
	}
	if p.pending != nil && element.Equal(p.pending.element) {
		p.active, p.pending = p.pending, nil
	}
	s.advancePointerLock()
}

func (s *surface) pointerLockError(request *webLockRequest) {
	p := &s.pointerLock
	if p.pending != request {
		return
	}
	if p.want == request {
		s.engine.Dispatch(p.want.token.Unlocked())
		p.want = nil
	}
	p.pending = nil
	s.advancePointerLock()
}

func (s *surface) losePointerLock() bool {
	p := &s.pointerLock
	if p.want == nil {
		return false
	}
	s.engine.Dispatch(p.want.token.Unlocked())
	p.want = nil
	s.advancePointerLock()
	return true
}

func (s *surface) pointerMotion(dx, dy float64) {
	p := &s.pointerLock
	if p.active != nil && p.want == p.active && (dx != 0 || dy != 0) {
		s.engine.Dispatch(p.active.token.Motion(dx, dy))
	}
}

func (s *surface) listenPointerLock(ls *listeners) {
	ls.on(s.doc, "pointerlockchange", func(js.Value) { s.pointerLockChanged() })
	ls.on(s.doc, "pointerlockerror", func(js.Value) {
		if pending := s.pointerLock.pending; pending != nil && !pending.promise {
			s.pointerLockError(pending)
		}
	})
	ls.on(js.Global(), "blur", func(js.Value) { s.losePointerLock() })
	ls.on(s.doc, "mousemove", func(e js.Value) {
		if s.pointerLock.active != nil {
			s.pointerMotion(e.Get("movementX").Float(), e.Get("movementY").Float())
		}
	})
}
