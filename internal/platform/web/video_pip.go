//go:build js && wasm

package web

import (
	"fmt"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// A video owns only its element's browser PiP session. Source replacement,
// disabling and deletion invalidate requests that may still be settling.
type videoPictureInPicture struct {
	el                                   js.Value
	enabled, pending, canceled, released bool
	exiting                              bool
	generation, requestGeneration        uint64
	state                                driver.PictureInPictureState
	reported                             bool
	report                               func(driver.PictureInPictureState)
}

func (p *videoPictureInPicture) document() js.Value { return p.el.Get("ownerDocument") }
func (p *videoPictureInPicture) active() bool {
	return p.document().Get("pictureInPictureElement").Equal(p.el)
}
func (p *videoPictureInPicture) snapshot(err string) driver.PictureInPictureState {
	doc := p.document()
	supported := p.enabled && p.el.Get("requestPictureInPicture").Type() == js.TypeFunction &&
		doc.Get("pictureInPictureEnabled").Truthy() && doc.Get("exitPictureInPicture").Type() == js.TypeFunction
	return driver.PictureInPictureState{Supported: supported,
		Possible: supported && p.el.Get("readyState").Int() >= 1 && p.el.Get("videoWidth").Int() > 0,
		Active:   supported && p.active(), Error: err}
}
func (p *videoPictureInPicture) emit(err string) {
	if p.released || !p.enabled {
		return
	}
	state := p.snapshot(err)
	if p.reported && state == p.state {
		return
	}
	p.state, p.reported = state, true
	if p.report != nil {
		p.report(state)
	}
}
func (p *videoPictureInPicture) configure(enabled, sourceChanged bool) {
	changed := p.enabled != enabled
	if changed || sourceChanged {
		p.invalidate()
	}
	p.enabled = enabled
	p.el.Set("disablePictureInPicture", !enabled)
	if changed || sourceChanged {
		p.emit("")
	}
}
func (p *videoPictureInPicture) invalidate() {
	p.generation++
	p.canceled = true
	p.exit()
}
func (p *videoPictureInPicture) entered() {
	if p.released || !p.enabled || p.pending && (p.canceled || p.requestGeneration != p.generation) {
		p.exit()
		return
	}
	p.emit("")
}
func (p *videoPictureInPicture) command(command uint32) {
	if p.released || !p.enabled {
		return
	}
	switch command {
	case driver.PictureInPictureStart:
		if p.pending || p.exiting || p.active() {
			return
		}
		state := p.snapshot("")
		if !state.Supported {
			p.emit("")
			return
		}
		if !state.Possible {
			p.emit("the video is not ready for Picture in Picture")
			return
		}
		p.pending, p.canceled, p.requestGeneration = true, false, p.generation
		generation := p.generation
		pipPromise(func() js.Value { return p.el.Call("requestPictureInPicture") }, func(err string) {
			p.pending = false
			if p.released || generation != p.generation || p.canceled {
				p.exit()
				return
			}
			p.emit(err)
		})
	case driver.PictureInPictureStop:
		p.canceled = true
		p.exit()
	}
}
func (p *videoPictureInPicture) exit() {
	if p.exiting || !p.active() || p.document().Get("exitPictureInPicture").Type() != js.TypeFunction {
		return
	}
	p.exiting = true
	generation := p.generation
	pipPromise(func() js.Value { return p.document().Call("exitPictureInPicture") }, func(err string) {
		p.exiting = false
		if generation == p.generation {
			p.emit(err)
		}
	})
}

// Both native Promise rejection and a synchronous DOM exception become state
// errors; callbacks remain alive until the browser settles the request.
func pipPromise(call func() js.Value, settled func(string)) {
	defer func() {
		if fault := recover(); fault != nil {
			if e, ok := fault.(js.Error); ok {
				settled(e.Error())
				return
			}
			panic(fault)
		}
	}()
	promise := call()
	var ok, fail js.Func
	finish := func(err string) { ok.Release(); fail.Release(); settled(err) }
	ok = js.FuncOf(func(js.Value, []js.Value) any { finish(""); return nil })
	fail = js.FuncOf(func(_ js.Value, args []js.Value) any {
		message := "Picture in Picture request failed"
		if len(args) > 0 {
			e := args[0]
			if e.Type() == js.TypeObject && e.Truthy() {
				message = e.Get("message").String()
			} else {
				message = fmt.Sprint(e)
			}
		}
		finish(message)
		return nil
	})
	promise.Call("then", ok, fail)
}
