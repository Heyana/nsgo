//go:build linux && !android

package linux

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type shaderOps struct {
	setShader       func(*window, *view, *driver.Shader)
	setEffect       func(*window, *view, *driver.Shader)
	startTransition func(*window, *view)
	redrawEffect    func(*window, *view)
	changedShaders  func(*window)
	staleEffects    func(*window)
	snapshotEffect  func(*window, *view, uintptr) bool
	effectArea      func(*window, *view) driver.Bounds
	release         func(*window, *view)
}

type canvasOps struct {
	setCanvas        func(*window, *view, *driver.Drawing)
	changedCanvases  func(*window)
	snapshotCanvas   func(*window, *view, uintptr, float64, float64)
	showOffscreen    func(*window, *view, *imageSlot, driver.ImageSource)
	keepImage        func(*window, string)
	releaseImage     func(*window, string)
	releaseOffscreen func(*window, uint64)
	releaseView      func(*view)
	closeWindow      func(*window)
}

type videoOps struct {
	configure    func(*window, *view, *driver.VideoConfig)
	seek         func(*view, time.Duration)
	stopProgress func(*window, *view)
	close        func(*window, *view)
}

type webViewOps struct {
	configure  func(*window, *view, *driver.WebConfig)
	command    func(*window, *view, driver.WebCommand, string, any)
	background func(*window, *view)
	cookies    func(*window, uint32, driver.WebCookiesRequest)
	focused    func(*window) *view
	edit       func(*window, driver.MenuRole) bool
	close      func(*window, *view)
}

type nativeOps struct {
	configure      func(*window, *view, *driver.NativeConfig)
	command        func(*view, string, any)
	changedNatives func(*window)
	closeView      func(*window, *view)
	closeAll       func(*window)
}

type gpuOps struct {
	set   func(*window, *view, driver.GPUViewConfig)
	close func(*window, *view)
}

var (
	shaderOnce sync.Once
	videoOnce  sync.Once
	webOnce    sync.Once
	gpuOnce    sync.Once

	shaders  atomic.Pointer[shaderOps]
	canvases atomic.Pointer[canvasOps]
	videos   atomic.Pointer[videoOps]
	webViews atomic.Pointer[webViewOps]
	natives  atomic.Pointer[nativeOps]
	gpus     atomic.Pointer[gpuOps]
)

func (*window) EnableShaders() {
	shaderOnce.Do(func() {
		cbShaderRender = newCallback(shaderRenderCallback)
		cbShaderUnrealize = newCallback(shaderUnrealizeCallback)
		shaders.Store(&shaderOps{
			setShader:       (*window).setShader,
			setEffect:       (*window).setEffect,
			startTransition: (*window).startTransition,
			redrawEffect:    (*window).redrawEffect,
			changedShaders:  (*window).changedShaders,
			staleEffects:    (*window).staleEffects,
			snapshotEffect: func(w *window, v *view, cur uintptr) bool {
				e := w.effectOn(v)
				return e != nil && (w.backdropStop == nil || w.backdropPath[v] == nil) && w.snapshotEffect(e, v, cur)
			},
			effectArea: (*window).effectArea,
			release: func(w *window, v *view) {
				w.endEffect(v)
				if v.shader != nil {
					w.unhost(v, &v.shader.hosted)
					v.shader = nil
				}
			},
		})
	})
}

func (*window) EnableCanvas() {
	if canvases.Load() != nil {
		return
	}
	canvases.Store(&canvasOps{
		setCanvas: func(w *window, v *view, d *driver.Drawing) {
			w.setCanvas(v, d)
			w.setCanvasParts(v, d.Nodes)
		},
		changedCanvases:  (*window).changedCanvases,
		snapshotCanvas:   (*window).snapshotCanvas,
		showOffscreen:    (*window).showOffscreen,
		keepImage:        (*window).keepCanvasImage,
		releaseImage:     (*window).releaseCanvasImage,
		releaseOffscreen: (*window).releaseOffscreen,
		releaseView: func(v *view) {
			v.releaseCanvasParts()
			v.canvas.raster.release()
			v.canvas.raster = nil
		},
		closeWindow: (*window).closeCanvasImages,
	})
}

func (*window) EnableVideo() {
	videoOnce.Do(func() {
		cbVideoPrepared = newCallback(videoPreparedCallback)
		cbVideoPlaying = newCallback(videoPlayingCallback)
		cbVideoSize = newCallback(videoSizeCallback)
		cbVideoEnded = newCallback(videoEndedCallback)
		cbVideoError = newCallback(videoErrorCallback)
		cbVideoProgress = newCallback(videoProgressCallback)
		videos.Store(&videoOps{
			configure: (*window).configureVideo,
			seek: func(v *view, d time.Duration) {
				if v.video != nil {
					v.video.seek(d)
				}
			},
			stopProgress: func(w *window, v *view) {
				if v.video != nil {
					w.stopProgress(v.video)
				}
			},
			close: func(w *window, v *view) {
				if v.video != nil {
					w.closeVideo(v)
				}
			},
		})
	})
}

func (*window) EnableWebView() {
	webOnce.Do(func() {
		cbWebLoadChanged = newCallback(webLoadChangedCallback)
		cbWebLoadFailed = newCallback(webLoadFailedCallback)
		cbWebNavigationChanged = newCallback(webNavigationChangedCallback)
		cbWebHistoryChanged = newCallback(webHistoryChangedCallback)
		cbWebResourceStarted = newCallback(webResourceStartedCallback)
		cbWebPolicy = newCallback(webPolicyCallback)
		cbWebCreate = newCallback(webCreateCallback)
		cbWebPermission = newCallback(webPermissionCallback)
		cbWebMessage = newCallback(webMessageCallback)
		cbWebMap = newCallback(webMapCallback)
		cbWebFocusIn = newCallback(webFocusInCallback)
		cbWebFocusOut = newCallback(webFocusOutCallback)
		cbWebKey = newCallback(webKeyCallback)
		cbWebButton = newCallback(webButtonCallback)
		cbWebMotion = newCallback(webMotionCallback)
		cbWebScroll = newCallback(webScrollCallback)
		cbWebScrollBegin = newCallback(webScrollBeginCallback)
		cbWebScrollDecelerate = newCallback(webScrollDecelerateCallback)
		cbWebDownloadStarted = newCallback(webDownloadStartedCallback)
		cbWebDownloadDestination = newCallback(webDownloadDestinationCallback)
		cbWebDownloadReceived = newCallback(webDownloadReceivedCallback)
		cbWebDownloadFinished = newCallback(webDownloadFinishedCallback)
		cbWebDownloadFailed = newCallback(webDownloadFailedCallback)
		cbWebScheme = newCallback(webSchemeCallback)
		webViews.Store(&webViewOps{
			configure:  (*window).configureWeb,
			command:    (*window).webCommand,
			background: (*window).webBackground,
			cookies:    (*window).webCookies,
			focused:    (*window).focusedWebView,
			edit:       (*window).webEdit,
			close: func(w *window, v *view) {
				if v.web != nil {
					w.closeWeb(v)
				}
			},
		})
	})
}

func (*window) EnableNative() {
	if natives.Load() != nil {
		return
	}
	natives.Store(&nativeOps{
		configure: (*window).configureNative,
		command: func(v *view, name string, data any) {
			if v.native != nil && v.native.comp != nil {
				v.native.comp.Command(name, data)
			}
		},
		changedNatives: (*window).changedNatives,
		closeView: func(w *window, v *view) {
			if v.native != nil {
				w.closeNative(v)
			}
		},
		closeAll: (*window).closeNatives,
	})
}

func (w *window) focusedWeb() *view {
	if wh := webViews.Load(); wh != nil {
		return wh.focused(w)
	}
	return nil
}

func (*window) EnableGPU() {
	gpuOnce.Do(func() {
		cbGPURealize = newCallback(gpuRealizeCallback)
		cbGPUResize = newCallback(gpuResizeCallback)
		cbGPURender = newCallback(gpuRenderCallback)
		cbGPUUnrealize = newCallback(gpuUnrealizeCallback)
		cbGPUFlush = newCallback(gpuFlushCallback)
		gpus.Store(&gpuOps{
			set:   (*window).setGPU,
			close: (*window).closeGPU,
		})
	})
}
