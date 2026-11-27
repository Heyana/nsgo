//go:build ios

package ios

import (
	"slices"
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
	paint            func(*view, uintptr, bool)
	redrawAnimated   func(*view)
	releaseView      func(*view)
	releaseSurface   func(*surface)
	keepImage        func(*surface, string)
	releaseImage     func(*surface, string)
	releaseOffscreen func(uint64)
}

type videoOps struct {
	create     func(*surface, *view)
	configure  func(*view, *driver.VideoConfig)
	seek       func(*view, time.Duration)
	pipCommand func(*view, uint32)
	layout     func(*view)
	close      func(*view)
}

type webViewOps struct {
	create    func(*view)
	configure func(*view, *driver.WebConfig)
	command   func(*view, *driver.MountItem)
	cookies   func(uint32, driver.WebCookiesRequest, func(driver.Event))
	close     func(*view)
}

type nativeOps struct {
	configure  func(*surface, *view, *driver.NativeConfig)
	command    func(*view, string, any)
	close      func(*view)
	switchView func(driver.NativeComponent) objc.ID
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
			s.endEffect(v)
			if sl := v.shaderLayer(); sl != nil {
				sl.Release()
				v.shader = nil
			}
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
			was.Release()
			s.animator.BindCanvas(v.id, d.Slots)
			v.setCanvasNodes(d.Nodes)
			v.redraw()
		},
		paint: func(v *view, ctx uintptr, baseUp bool) {
			ops := v.surface.animator.CanvasOps(v.id, v.drawing)
			apple.Play(ctx, v.drawing, ops, apple.CanvasHost{
				Ratio: v.contentScale(), Redraw: v.redraw, Owner: v, BaseUp: baseUp})
			v.played = append(v.played[:0], ops...)
		},
		redrawAnimated: (*view).redrawAnimated,
		releaseView: func(v *view) {
			v.drawing.Release()
			v.drawing = nil
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
		create: func(s *surface, v *view) {
			v.video = apple.NewVideo(v.native)
			v.video.Dispatch = v.report
			v.video.Parent = s.viewController()
			v.native.Send(sel("setClipsToBounds:"), true)
			s.videos = append(s.videos, v)
		},
		configure: func(v *view, c *driver.VideoConfig) {
			if v.video != nil {
				v.video.Configure(c)
			}
		},
		seek: func(v *view, d time.Duration) {
			if v.video != nil {
				v.video.Seek(d)
			}
		},
		pipCommand: func(v *view, cmd uint32) {
			if v.video != nil {
				v.video.PictureInPictureCommand(cmd)
			}
		},
		layout: func(v *view) {
			if v.video != nil {
				v.video.Layout()
			}
		},
		close: func(v *view) {
			if v.video != nil {
				v.video.Close()
				v.surface.videos = slices.DeleteFunc(v.surface.videos, func(video *view) bool { return video == v })
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
			v.web = apple.NewWeb(v.native)
			v.web.Dispatch = v.report
		},
		configure: func(v *view, c *driver.WebConfig) {
			if v.web != nil {
				v.web.Configure(c)
			}
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
		cookies: apple.WebCookies,
		close: func(v *view) {
			if v.web != nil {
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
			if sw, ok := c.(*uiSwitch); ok {
				return sw.view
			}
			return 0
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

// nsgo:allow-duplicate the two Apple backends install the same GPU table.
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
