//go:build darwin && !ios

package macos

import (
	"sync/atomic"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

type shaderOps struct {
	newView         func(*view)
	setShader       func(*surface, *view, *driver.Shader)
	setEffect       func(*surface, *view, *driver.Shader)
	startTransition func(*surface, *view)
	shaderInput     func(*surface, *view)
	shadeLater      func(*surface, *view)
	drawShaders     func(*surface)
	staleEffects    func(*surface)
	sizeShader      func(*view)
	release         func(*surface, *view)
}

type canvasOps struct {
	apply            func(*surface, *view, *driver.Drawing)
	paint            func(*view, uintptr, objc.ID, bool)
	redrawAnimated   func(*view)
	releaseView      func(*view)
	releaseSurface   func(*surface)
	keepImage        func(*surface, string)
	releaseImage     func(*surface, string)
	releaseOffscreen func(uint64)
}

type videoOps struct {
	create    func(*view)
	configure func(*view, *driver.VideoConfig)
	seek      func(*view, time.Duration)
	close     func(*view)
}

type webViewOps struct {
	create       func(*view)
	configure    func(*view, *driver.WebConfig)
	command      func(*view, *driver.MountItem)
	cookies      func(uint32, driver.WebCookiesRequest, func(driver.Event))
	observeFocus func(*surface)
	reportFocus  func(*surface)
	close        func(*view)
}

type nativeOps struct {
	configure        func(*surface, *view, *driver.NativeConfig)
	command          func(*view, string, any)
	close            func(*view)
	switchView       func(driver.NativeComponent) objc.ID
	setSwitchInvalid func(driver.NativeComponent, bool)
}

type gpuOps struct {
	newView  func(*view, driver.GPUViewConfig)
	sizeView func(*view)
	release  func(*surface, *view)
}

var (
	shaders  atomic.Pointer[shaderOps]
	canvases atomic.Pointer[canvasOps]
	videos   atomic.Pointer[videoOps]
	webViews atomic.Pointer[webViewOps]
	natives  atomic.Pointer[nativeOps]
	gpus     atomic.Pointer[gpuOps]
)

func (*surface) EnableShaders() {
	if shaders.Load() != nil {
		return
	}
	shaders.Store(&shaderOps{
		newView:         (*view).newShader,
		setShader:       (*surface).setShader,
		setEffect:       (*surface).setEffect,
		startTransition: (*surface).startTransition,
		shaderInput:     (*surface).shaderInput,
		shadeLater:      (*surface).shadeLater,
		drawShaders:     (*surface).drawShaders,
		staleEffects:    (*surface).staleEffects,
		sizeShader:      (*view).sizeShader,
		release: func(s *surface, v *view) {
			if sl := v.shaderLayer(); sl != nil {
				sl.Release()
				v.shader = nil
			}
			s.endEffect(v)
		},
	})
}

// EnableCanvasShaders implements driver.CanvasShaderEnabler.
func (*surface) EnableCanvasShaders() { apple.EnableCanvasShaders() }

func (*surface) EnableCanvas() {
	apple.EnableCanvas()
	if canvases.Load() != nil {
		return
	}
	canvases.Store(&canvasOps{
		apply: func(s *surface, v *view, d *driver.Drawing) {
			was := v.drawing
			v.drawing = d
			v.played = v.played[:0] // drawn whole first
			v.restretch(was)
			cp := v.canvasState()
			if cp == nil {
				cp = &canvasPaint{}
				v.canvas = cp
			}
			cp.set(d)
			s.animator.BindCanvas(v.id, d.Slots)
			v.setCanvasNodes(d.Nodes)
			v.redraw()
		},
		paint:          (*view).paintCanvas,
		redrawAnimated: (*view).redrawAnimated,
		releaseView: func(v *view) {
			if cp := v.canvasState(); cp != nil {
				cp.set(nil)
				v.canvas = nil
			}
			v.releaseCanvasNodes()
		},
		releaseSurface: func(s *surface) {
			apple.ReleaseCanvasImages(s)
		},
		keepImage: func(s *surface, key string) {
			apple.KeepCanvasImage(s, key)
		},
		releaseImage: func(s *surface, key string) {
			apple.ReleaseCanvasImage(s, key)
		},
		releaseOffscreen: apple.ReleaseOffscreen,
	})
}

func (*surface) EnableVideo() {
	if videos.Load() != nil {
		return
	}
	videos.Store(&videoOps{
		create: func(v *view) {
			v.video = apple.NewVideo(v.native)
			v.video.Dispatch = v.report
		},
		configure: func(v *view, c *driver.VideoConfig) {
			if v.video == nil {
				return
			}
			s := v.surface
			delete(views, v.video.View())
			v.video.Configure(c)
			if c.HitTestAlpha > 0 && !c.Controls {
				if s.pointer.videos == nil {
					s.pointer.videos = make(map[*apple.Video]struct{})
				}
				s.pointer.videos[v.video] = struct{}{}
			} else {
				delete(s.pointer.videos, v.video)
				v.video.ClearHitTestAlpha()
			}
			if shown := v.video.View(); shown != 0 {
				views[shown] = v
			}
		},
		seek: func(v *view, d time.Duration) {
			if v.video != nil {
				v.video.Seek(d)
			}
		},
		close: func(v *view) {
			if v.video != nil {
				delete(v.surface.pointer.videos, v.video)
				delete(views, v.video.View())
				v.video.Close()
			}
		},
	})
}

func (*surface) EnableWebView() {
	if webViews.Load() != nil {
		return
	}
	webViews.Store(&webViewOps{
		create: func(v *view) {
			if v.surface.stopWebFocus == nil {
				v.surface.observeWebFocus()
			}
			v.web = apple.NewWeb(v.native)
			v.web.Dispatch = v.report
		},
		configure: func(v *view, c *driver.WebConfig) {
			if v.web == nil {
				return
			}
			s := v.surface
			delete(views, v.web.View())
			v.web.Configure(c)
			views[v.web.View()] = v
			if s.webs == nil {
				s.webs = map[*view]struct{}{}
			}
			s.webs[v] = struct{}{}
			guardWebTracking(v.web.View())
		},
		command: func(v *view, it *driver.MountItem) {
			if v.web == nil {
				return
			}
			if in, ok := it.Data.(*driver.WebInput); ok && it.WebCommand == driver.WebSendInput {
				if web := v.web.View(); web != 0 {
					sendWebInput(web, *in)
				}
				return
			}
			v.web.Command(it.WebCommand, it.Value, it.Data)
		},
		cookies:      apple.WebCookies,
		observeFocus: (*surface).observeWebFocus,
		reportFocus:  (*surface).reportWebFocus,
		close: func(v *view) {
			if v.web != nil {
				s := v.surface
				if s.webOver == v {
					s.webOver = nil
				}
				delete(s.webs, v)
				forgetWebTracking(v.web.View())
				delete(views, v.web.View())
				v.web.Close()
			}
		},
	})
}

func (*surface) EnableNative() {
	if natives.Load() != nil {
		return
	}
	natives.Store(&nativeOps{
		configure: (*surface).configureNative,
		command: func(v *view, name string, data any) {
			if v.component != nil {
				v.component.Command(name, data)
			}
		},
		close: func(v *view) {
			if v.component != nil {
				v.component.Close()
				v.component = nil
			}
		},
		switchView: func(c driver.NativeComponent) objc.ID {
			if sw, ok := c.(*nsSwitch); ok {
				return sw.view
			}
			return 0
		},
		setSwitchInvalid: func(c driver.NativeComponent, invalid bool) {
			if sw, ok := c.(*nsSwitch); ok {
				sw.invalid = invalid
			}
		},
	})
}

func switchViewOf(c driver.NativeComponent) objc.ID {
	if c == nil {
		return 0
	}
	if nh := natives.Load(); nh != nil {
		return nh.switchView(c)
	}
	return 0
}

func setSwitchInvalid(c driver.NativeComponent, invalid bool) {
	if c == nil {
		return
	}
	if nh := natives.Load(); nh != nil {
		nh.setSwitchInvalid(c, invalid)
	}
}

func (*surface) EnableGPU() {
	if gpus.Load() != nil {
		return
	}
	gpus.Store(&gpuOps{
		newView:  (*view).newGPU,
		sizeView: (*view).sizeGPU,
		release:  (*surface).releaseGPU,
	})
}
