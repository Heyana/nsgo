//go:build android || nsgoandroidhost

package android

import (
	"sync/atomic"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type shaderOps struct {
	track func(*surface, *treeView, *driver.MountBatch) ([]shaderItem, []driver.MountItem)
	write func(*writer, *driver.Shader, shaderItem)
}

type canvasOps struct {
	trackPixels func(*treeView, *driver.MountBatch, *[]pixelFrame) map[int][]string
	write       func(*writer, *driver.Drawing)
}

type canvasRequestOps struct {
	update       func(*canvasRequests, uint32, *driver.MountBatch)
	request      func(*surface, *reader)
	closeSurface func(*canvasRequests, uint32)
	closeAll     func(*canvasRequests)
}

type videoOps struct {
	writeVideo func(*writer, *driver.VideoConfig)
	writeSeek  func(*writer, *driver.MountItem)
}

type webViewOps struct {
	withoutCookies func(*surface, uint32, *driver.MountBatch) *driver.MountBatch
	cookiesSet     func(*surface, *reader)
	writeWeb       func(*writer, *driver.WebConfig)
	writeCommand   func(*writer, *driver.MountItem)
}

type nativeOps struct {
	track        func(*surface, uint32, *driver.MountBatch) []nativeItem
	closeAll     func(*surface)
	closeSurface func(*surface, uint32)
	writeNative  func(*writer, *driver.MountItem, nativeItem)
	writeCommand func(*writer, *driver.MountItem)
}

var (
	shaders    atomic.Pointer[shaderOps]
	canvases   atomic.Pointer[canvasOps]
	canvasReqs atomic.Pointer[canvasRequestOps]
	videos     atomic.Pointer[videoOps]
	webViews   atomic.Pointer[webViewOps]
	natives    atomic.Pointer[nativeOps]
)

func enableCanvasRequests() {
	if canvasReqs.Load() != nil {
		return
	}
	canvasReqs.Store(&canvasRequestOps{
		update:       (*canvasRequests).update,
		request:      (*surface).canvasRequest,
		closeSurface: (*canvasRequests).closeSurface,
		closeAll:     (*canvasRequests).close,
	})
}

func (*surface) EnableShaders() {
	enableCanvasRequests()
	if shaders.Load() != nil {
		return
	}
	shaders.Store(&shaderOps{
		track: (*surface).trackShaders,
		write: writeShader,
	})
}

func (*surface) EnableCanvas() {
	enableCanvasRequests()
	if canvases.Load() != nil {
		return
	}
	canvases.Store(&canvasOps{
		trackPixels: (*treeView).trackCanvasPixels,
		write:       writeDrawing,
	})
}

func (*surface) EnableVideo() {
	if videos.Load() != nil {
		return
	}
	videos.Store(&videoOps{
		writeVideo: writeVideo,
		writeSeek:  writeVideoSeek,
	})
}

func (*surface) EnableWebView() {
	if webViews.Load() != nil {
		return
	}
	webViews.Store(&webViewOps{
		withoutCookies: (*surface).withoutWebCookies,
		cookiesSet:     (*surface).webCookiesSet,
		writeWeb:       writeWebConfig,
		writeCommand:   writeWebCommand,
	})
}

func (*surface) EnableNative() {
	if natives.Load() != nil {
		return
	}
	natives.Store(&nativeOps{
		track:        (*surface).trackNatives,
		closeAll:     (*surface).closeNatives,
		closeSurface: (*surface).closeSurfaceNatives,
		writeNative:  writeNative,
		writeCommand: writeNativeCommand,
	})
}

func (*overlay) EnableShaders() { (*surface)(nil).EnableShaders() }
func (*overlay) EnableCanvas()  { (*surface)(nil).EnableCanvas() }
func (*overlay) EnableVideo()   { (*surface)(nil).EnableVideo() }
func (*overlay) EnableWebView() { (*surface)(nil).EnableWebView() }
func (*overlay) EnableNative()  { (*surface)(nil).EnableNative() }

func writeWebCommand(w *writer, it *driver.MountItem) {
	w.byte(byte(it.WebCommand))
	if it.WebCommand == driver.WebPostMessage {
		w.str(driver.WebMessageScript(it.Value))
	} else if it.WebCommand == driver.WebLoad {
		if src, ok := it.Data.(*driver.WebSource); ok && src != nil {
			w.str(src.Src)
			w.str(src.HTML)
			w.str(src.BaseURL)
		} else {
			w.str("")
			w.str("")
			w.str("")
		}
	} else {
		w.str(it.Value)
	}
}

func writeVideo(w *writer, c *driver.VideoConfig) {
	w.str(c.Source)
	w.flags(c.Paused, c.Muted, c.Repeat, c.Controls, c.PictureInPicture, c.AutoPictureInPicture)
	w.float(c.Volume)
	w.float(c.Rate)
	w.byte(byte(c.ResizeMode - driver.Cover)) // Java's are 0-based
	w.u32(uint32(c.ProgressInterval.Milliseconds()))
	w.byte(byte(c.AudioSession))
	w.u32(uint32(c.Headers.Len()))
	for name, value := range c.Headers.All {
		w.str(name)
		w.str(value)
	}
}

func writeVideoSeek(w *writer, it *driver.MountItem) {
	w.u64(uint64(it.Time.Milliseconds()))
}

func writeNative(w *writer, it *driver.MountItem, n nativeItem) {
	w.str(n.class)
	w.u32(uint32(len(n.props)))
	for _, p := range n.props {
		w.str(p.Name)
		w.str(jsonValue(p.Value))
	}
	// The props animated nodes drive, when they changed: Java hands the view their values on the frames they change.
	w.flags(it.Native.Bind)
	if it.Native.Bind {
		w.u32(uint32(len(it.Native.Bindings)))
		for _, b := range it.Native.Bindings {
			w.str(b.Name)
			w.u32(b.Node)
			w.flags(b.Color)
		}
	}
}

func writeNativeCommand(w *writer, it *driver.MountItem) {
	w.str(it.Value)
	w.str(jsonValue(it.Data))
}
