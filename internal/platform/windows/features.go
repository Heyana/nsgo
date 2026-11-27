//go:build windows

package windows

import (
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type shaderOps struct {
	setShader       func(*window, *view, *driver.Shader)
	setEffect       func(*window, *view, *driver.Shader)
	startTransition func(*window, *view)
	changedShaders  func(*window)
	drawShader      func(*painter, *view, float32, float32)
	drawEffect      func(*painter, *view, matrix) bool
	pictureBackdrop func(*painter, *view, *view, matrix)
	release         func(*window, *view)
}

type canvasOps struct {
	changedCanvases  func(*window)
	draw             func(*painter, *view, float32, float32)
	drawOffscreen    func(*painter, *view, *imageSlot, rectF)
	keepImage        func(*window, string)
	releaseImage     func(*window, string)
	releaseOffscreen func(*window, uint64)
	releaseView      func(*view)
	closeWindow      func(*window)
}

type videoOps struct {
	configure   func(*window, *view, *driver.VideoConfig)
	seek        func(*view, time.Duration)
	draw        func(*painter, *view, float32, float32)
	close       func(*view)
	barAt       func(*window, int32, int32) bool
	cursor      func(*window, int32, int32) bool
	heldButton  func(*window, int, bool) bool
	pressAt     func(*window, int32, int32) bool
	drag        func(*window, int32, int32) bool
	captureLost func(*window)
}

type webViewOps struct {
	configure      func(*window, *view, *driver.WebConfig)
	command        func(*view, *driver.MountItem)
	restyle        func(*view)
	cookies        func(*window, uint32, driver.WebCookiesRequest)
	startEnvs      func()
	draw           func(*painter, *view, matrix, float32, float32, float32, bool)
	composedPlaced func(*window, []composed, []int)
	closeView      func(*view)
	closeWindow    func(*window)
	moved          func(*window)
	hitAt          func(*window, int32, int32) bool
	cursor         func(*window, int32, int32) bool
	rawButton      func(*window, int, bool, int32, int32, uint32) bool
	pressAt        func(*window, int, int32, int32, uint32) bool
	blurFocused    func(*window)
	rawMove        func(*window, int32, int32, uint32) bool
	hoverMove      func(*window, int32, int32, uint32)
	mouseLeft      func(*window)
	captureLost    func(*window)
	wheel          func(*window, int32, int32, int16, bool) bool
	pointer        func(*window, uint32, uintptr, uintptr) bool
	edit           func(*window, driver.MenuRole) bool
}

type nativeOps struct {
	setNative       func(*window, *view, *driver.NativeConfig)
	command         func(*view, string, any)
	changedNatives  func(*window)
	placeComponent  func(*painter, *view, matrix, float32, float32, float32, bool)
	placeComponents func(*window, []placedComponent)
	closeView       func(*window, *view)
	closeWindow     func(*window)
}

type gpuOps struct {
	configure func(*window, *view, driver.GPUViewConfig)
	sync      func(*window, []*view, []*view)
	process   func(*window)
	release   func(*gpuView)
}

var (
	shaders  atomic.Pointer[shaderOps]
	canvases atomic.Pointer[canvasOps]
	videos   atomic.Pointer[videoOps]
	webViews atomic.Pointer[webViewOps]
	natives  atomic.Pointer[nativeOps]
	gpus     atomic.Pointer[gpuOps]
)

func (*window) EnableGPU() {
	if gpus.Load() != nil {
		return
	}
	gpus.Store(&gpuOps{
		configure: (*window).configureGPU,
		sync:      (*window).syncGPUViews,
		process:   (*window).processGPUFrames,
		release:   (*gpuView).release,
	})
}

func (*window) EnableShaders() {
	if shaders.Load() != nil {
		return
	}
	shaders.Store(&shaderOps{
		setShader:       (*window).setShader,
		setEffect:       (*window).setEffect,
		startTransition: (*window).startTransition,
		changedShaders:  (*window).changedShaders,
		drawShader:      (*painter).drawShader,
		pictureBackdrop: (*painter).pictureBackdrop,
		drawEffect: func(p *painter, v *view, world matrix) bool {
			e := p.w.effectOn(v)
			return e != nil && p.drawEffect(e, v, world)
		},
		release: func(w *window, v *view) {
			v.shaderView.release()
			w.endEffect(v)
			v.shaderView = nil
		},
	})
}

func (*window) EnableCanvas() {
	if canvases.Load() != nil {
		return
	}
	canvases.Store(&canvasOps{
		changedCanvases:  (*window).changedCanvases,
		draw:             (*painter).canvas,
		drawOffscreen:    (*painter).drawOffscreenImage,
		keepImage:        (*window).keepCanvasImage,
		releaseImage:     (*window).releaseCanvasImage,
		releaseOffscreen: (*window).releaseOffscreen,
		releaseView: func(v *view) {
			v.canvasCache.release()
		},
		closeWindow: (*window).closeCanvasImages,
	})
}

func (*window) EnableVideo() {
	if videos.Load() != nil {
		return
	}
	videos.Store(&videoOps{
		configure: func(w *window, v *view, c *driver.VideoConfig) {
			if v.video == nil {
				v.video = &video{w: w, v: v}
			}
			v.video.configure(c)
		},
		seek: func(v *view, d time.Duration) {
			if v.video != nil {
				v.video.seek(d)
			}
		},
		draw: func(p *painter, v *view, width, height float32) {
			if v.video != nil {
				p.drawVideo(v.video, width, height)
			}
		},
		close: func(v *view) {
			if v.video != nil {
				v.video.close()
				v.video = nil
			}
		},
		barAt: func(w *window, x, y int32) bool {
			vd, _, _ := w.barAt(x, y)
			return vd != nil
		},
		cursor: func(w *window, x, y int32) bool {
			if vd, lx, _ := w.barAt(x, y); vd != nil && lx < videoButton {
				hand, _, _ := loadCursorW.Call(0, idcHand)
				setCursor.Call(hand)
				return true
			}
			return false
		},
		heldButton: func(w *window, button int, down bool) bool {
			vd := w.barVideo
			if vd == nil {
				return false
			}
			if !down && button == 0 {
				w.barVideo = nil
				releaseCapture.Call()
				vd.released()
			}
			return true
		},
		pressAt: func(w *window, x, y int32) bool {
			vd, lx, _ := w.barAt(x, y)
			if vd == nil {
				return false
			}
			w.barVideo = vd
			setCapture.Call(w.hwnd)
			vd.press(lx, float32(vd.v.frame.Size.Width))
			return true
		},
		drag: func(w *window, x, y int32) bool {
			vd := w.barVideo
			if vd == nil {
				return false
			}
			vd.drag(float32(vd.v.surfacePoint(w.point(x, y)).X), float32(vd.v.frame.Size.Width))
			return true
		},
		captureLost: func(w *window) {
			if vd := w.barVideo; vd != nil {
				w.barVideo = nil
				vd.released()
			}
		},
	})
}

func (*window) EnableWebView() {
	if webViews.Load() != nil {
		return
	}
	webViews.Store(&webViewOps{
		configure: func(w *window, v *view, c *driver.WebConfig) {
			if v.web == nil {
				v.web = w.newWeb(v, c)
			}
			v.web.configure(c)
		},
		command: func(v *view, it *driver.MountItem) {
			if v.web != nil {
				v.web.command(it.WebCommand, it.Value, it.Data)
			}
		},
		restyle: func(v *view) {
			if v.web != nil {
				v.web.restyle()
			}
		},
		cookies:   (*window).webCookies,
		startEnvs: startWebEnvironments,
		draw: func(p *painter, v *view, world matrix, width, height, radius float32, clip bool) {
			if wv := v.web; wv != nil && wv.composed() {
				corners := float32(0)
				if clip {
					corners = radius
				}
				p.compose(wv, world, width, height, corners)
			} else if wv != nil && wv.failed {
				p.drawWebPlaceholder(width, height)
			}
		},
		composedPlaced: (*window).composedPlaced,
		closeView: func(v *view) {
			if v.web != nil {
				v.web.close()
				v.web = nil
			}
		},
		closeWindow: func(w *window) {
			for wv := range w.webs {
				wv.close()
			}
			w.closeCookieView()
		},
		moved: func(w *window) {
			for wv := range w.webs {
				wv.moved()
			}
		},
		hitAt: func(w *window, x, y int32) bool {
			wv, _, _ := w.webAt(x, y)
			return wv != nil
		},
		cursor: func(w *window, x, y int32) bool {
			if wv, _, _ := w.webAt(x, y); wv != nil && wv.cursor != 0 {
				setCursor.Call(wv.cursor)
				return true
			}
			return false
		},
		rawButton: func(w *window, button int, down bool, x, y int32, tm uint32) bool {
			wv := w.rawWeb
			if wv == nil {
				return false
			}
			px, py := wv.local(x, y)
			wv.mouse(button, down, px, py, tm)
			if wv.buttons == 0 {
				w.rawWeb = nil
				releaseCapture.Call()
			}
			return true
		},
		pressAt: func(w *window, button int, x, y int32, tm uint32) bool {
			wv, px, py := w.webAt(x, y)
			if wv == nil {
				return false
			}
			w.pressOutside(x, y)
			w.rawWeb = wv
			setCapture.Call(w.hwnd)
			wv.mouse(button, true, px, py, tm)
			if wv.buttons == 0 {
				w.rawWeb = nil
				releaseCapture.Call()
			}
			return true
		},
		blurFocused: func(w *window) {
			for wv := range w.webs {
				if wv.focused {
					wv.focusWindow()
				}
			}
		},
		rawMove: func(w *window, x, y int32, tm uint32) bool {
			wv := w.rawWeb
			if wv == nil {
				return false
			}
			px, py := wv.local(x, y)
			wv.mouse(-1, false, px, py, tm)
			return true
		},
		hoverMove: func(w *window, x, y int32, tm uint32) {
			wv, px, py := w.webAt(x, y)
			if w.hoverWeb != nil && w.hoverWeb != wv {
				w.hoverWeb.leave()
				w.hoverWeb = nil
			}
			if wv != nil {
				w.hoverWeb = wv
				wv.mouse(-1, false, px, py, tm)
			}
		},
		mouseLeft: func(w *window) {
			if wv := w.hoverWeb; wv != nil && w.rawWeb == nil {
				w.hoverWeb = nil
				wv.leave()
			}
		},
		captureLost: func(w *window) {
			if wv := w.rawWeb; wv != nil {
				w.rawWeb = nil
				wv.leave()
			}
		},
		wheel: func(w *window, x, y int32, delta int16, horizontal bool) bool {
			if w.rawWeb != nil {
				return false
			}
			wv, px, py := w.webAt(x, y)
			return wv != nil && wv.wheel(delta, horizontal, px, py)
		},
		pointer: (*window).webPointer,
		edit:    (*window).webEdit,
	})
}

func (*window) EnableNative() {
	if natives.Load() != nil {
		return
	}
	natives.Store(&nativeOps{
		setNative: (*window).setNative,
		command: func(v *view, name string, data any) {
			if c := v.component; c != nil && c.c != nil {
				c.c.Command(name, data)
			}
		},
		changedNatives: (*window).changedNatives,
		placeComponent: func(p *painter, v *view, world matrix, width, height, radius float32, clip bool) {
			if c := v.component; c != nil && c.hwnd != 0 {
				corners := float32(0)
				if clip {
					corners = radius
				}
				p.placeComponent(c, world, width, height, corners)
			}
		},
		placeComponents: (*window).placeComponents,
		closeView: func(w *window, v *view) {
			if v.component != nil {
				w.closeComponent(v.component)
				v.component = nil
			}
		},
		closeWindow: func(w *window) {
			for c := range w.hosts {
				w.closeComponent(c)
			}
		},
	})
}
