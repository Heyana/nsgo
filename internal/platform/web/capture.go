//go:build js && wasm

package web

import (
	"errors"
	"math"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func (s *surface) capture(id driver.NodeID, count uint32) {
	answer := func(data []byte, err error) {
		s.engine.Dispatch(driver.CaptureResult(count, data, err))
	}
	if s.doc.Get("hidden").Bool() {
		answer(nil, errors.New("web: capture has no visible page"))
		return
	}
	// Read layout and the GPU drawing buffers together before the browser
	// composites and discards those buffers.
	var frame js.Func
	frame = js.FuncOf(func(js.Value, []js.Value) any {
		frame.Release()
		s.captureFrame(id, answer)
		return nil
	})
	js.Global().Call("requestAnimationFrame", frame)
}

func (s *surface) captureFrame(id driver.NodeID, answer func([]byte, error)) {
	if s.root == nil {
		answer(nil, errors.New("web: capture has no surface"))
		return
	}
	root := s.root.el
	el := root
	if id != 0 {
		v := s.view(id)
		if v == nil || !root.Call("contains", v.el).Bool() {
			answer(nil, errors.New("web: capture view is outside the surface"))
			return
		}
		el = v.el
	}
	r, host := el.Call("getBoundingClientRect"), root.Call("getBoundingClientRect")
	left := max(r.Get("left").Float(), host.Get("left").Float(), 0)
	top := max(r.Get("top").Float(), host.Get("top").Float(), 0)
	right := min(r.Get("right").Float(), host.Get("right").Float(), js.Global().Get("innerWidth").Float())
	bottom := min(r.Get("bottom").Float(), host.Get("bottom").Float(), js.Global().Get("innerHeight").Float())
	if right <= left || bottom <= top {
		answer(nil, errors.New("web: capture has no visible pixels"))
		return
	}
	pictures := js.Global().Get("WeakMap").New()
	for _, v := range s.views {
		if v == nil || !root.Call("contains", v.el).Bool() {
			continue
		}
		if sh := shaders.Load(); sh != nil && v.gl != nil && v.gl.shader != nil {
			sh.draw(s, v)
		}
		if im := v.image; im != nil && im.tint == 0 {
			for _, picture := range []*picture{&im.placeholder, &im.source} {
				if picture.loaded && sameOrigin(picture.img) {
					pictures.Call("set", picture.layer, picture.img)
				}
			}
		}
	}
	if sh := shaders.Load(); sh != nil {
		sh.drawEffects(s)
	}
	ratio := float64(s.env.PixelRatio)
	if ratio <= 0 {
		ratio = 1
	}
	canvas := s.doc.Call("createElement", "canvas")
	canvas.Set("width", math.Ceil((right-left)*ratio))
	canvas.Set("height", math.Ceil((bottom-top)*ratio))
	ctx := canvas.Call("getContext", "2d")
	ctx.Call("scale", ratio, ratio)
	p := painter{ctx: ctx, x0: left, y0: top, includeEffects: true, pictures: pictures}
	p.element(root, 1)
	var result js.Func
	result = js.FuncOf(func(_ js.Value, args []js.Value) any {
		result.Release()
		if len(args) == 0 || args[0].IsNull() {
			answer(nil, errors.New("web: cannot encode the surface's pixels"))
			return nil
		}
		then(args[0].Call("arrayBuffer"), func(buf js.Value) {
			arr := js.Global().Get("Uint8Array").New(buf)
			data := make([]byte, arr.Get("length").Int())
			js.CopyBytesToGo(data, arr)
			answer(data, nil)
		}, func(err js.Value) {
			answer(nil, errors.New("web: cannot read the captured PNG"))
		})
		return nil
	})
	defer func() {
		if failure := recover(); failure != nil {
			if _, ok := failure.(js.Error); !ok {
				panic(failure)
			}
			// A cross-origin drawing may make the browser's canvas unreadable.
			result.Release()
			answer(nil, errors.New("web: cannot read the surface's pixels"))
		}
	}()
	canvas.Call("toBlob", result, "image/png")
}
