//go:build js && wasm

// Package miniprogram is NSGo's mini program backend, for WeChat, QQ,
// Alipay and Douyin alike: the engine
// and the app run in WebAssembly in the mini program's logic layer, each
// batch becomes operations its runtime applies to a tree of plain objects
// and hands the render layer with setData, and one recursive component
// draws each view at its frame. Text is measured on an offscreen canvas
// and broken into lines here (text.go), canvases are drawn on one and
// shown as pictures, and taps, touches, scrolls and edits come back as
// events (mount.go, touch.go).
//
// The runtime (cmd/nsgo/miniprogram) starts the module with Go's
// wasm_exec.js in a global object of its own, nsgoMP being the bridge:
// what it calls in Go, Go sets on it, and what Go calls it provides,
// the platform's APIs (api) among them, as WeChat's whichever platform it
// is.
package miniprogram

import (
	"encoding/base64"
	"image"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall/js"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/canvas2d"
	"github.com/nsgo-dev/nsgo/internal/platform/canvasmesh"
)

// Options configure the app.
type Options struct {
	// Name and Version identify the running app in update replies.
	Name, Version string
	// Development and UpdateChannel retain explicitly disabled build policy.
	Development   bool
	UpdateChannel string
	// ID names the app's entries in the mini program's storage.
	ID string
	// Fonts are the app's bundled font file paths under resources/.
	Fonts []string
	// Appearance is the appearance the app starts in.
	Appearance driver.Appearance
}

// surface is the app's page: the engine's platform.
type surface struct {
	mu     sync.Mutex
	bridge js.Value
	// api is the platform's APIs, with WeChat's names and options.
	api     js.Value
	m       *mounter
	text    *TextEngine
	engine  *engine.Engine
	env     driver.Environment
	started time.Time
	// canvas is the offscreen canvas drawings are drawn on, one at a time, and images what they draw, bounded by count and byte budget.
	canvas js.Value
	images canvasImageCache[*canvasImage]
	// meshCanvas is a second offscreen canvas, where meshes' textures are read and their rasters composited.
	meshCanvas js.Value
	// stretchCanvas is where a canvas drawn below the pixel ratio is
	// stretched to it pixel by pixel (Drawing.Upscale nearest).
	stretchCanvas js.Value
	pixelImages   bool
	pixelFrames   []canvasPixelFrame
	pixelCount    int
	// files are images of bytes written for <image> to load, by key.
	files map[string]string
	// appearance is the one the app forces, AppearanceSystem for none.
	appearance    driver.Appearance
	updates       *miniUpdater
	updateUnsub   js.Value
	updateChanged js.Func
	updateClose   js.Func
}

// Run runs app in the mini program's page. Call it from main; it does
// not return.
func Run(app engine.App, opts Options) {
	g := js.Global()
	bridge := g.Get("nsgoMP")
	s := &surface{bridge: bridge, api: bridge.Get("api"), started: time.Now(),
		files:       map[string]string{},
		pixelImages: bridge.Get("pixelImages").Truthy()}
	s.images.onEvict = func(im *canvasImage) { im.release() }
	s.text = NewTextEngine(measurer{s.bridge})
	s.m = newMounter(s)
	s.m.frame = func() { s.engine.Frame() }
	s.appearance = opts.Appearance
	s.env = s.environment()
	s.m.reduce = s.env.ReduceMotion
	size := s.size()
	s.m.viewport = size
	id := opts.ID
	if id == "" {
		id = "default"
	}
	updater := s.initUpdates(opts)
	s.engine = engine.Start(s, app, engine.Options{
		Size: size, Environment: s.env, Appearance: opts.Appearance,
		Updater: updater,
		Storage: engine.NewStorage(&storage{api: s.api, prefix: "nsgo:" + id + ":"}),
		Secrets: s.secretsStore(),
	})
	s.registerFonts(opts.Fonts)
	s.registerAutomation()
	s.bridge.Set("event", js.FuncOf(func(_ js.Value, args []js.Value) any {
		s.event(args)
		return nil
	}))
	s.bridge.Call("ready")
	select {}
}

// measurer measures text on the runtime's offscreen canvas.
type measurer struct{ bridge js.Value }

func (m measurer) Width(font, text string) float64 {
	return m.bridge.Call("measure", font, text, "").Float()
}

func (m measurer) WidthLang(font, text, language string) float64 {
	return m.bridge.Call("measure", font, text, language).Float()
}

func (m measurer) Metrics(font, language string) (ascent, descent, xHeight, capHeight float64, ok bool) {
	fn := m.bridge.Get("measureMetrics")
	if fn.Type() != js.TypeFunction {
		return 0, 0, 0, 0, false
	}
	res := m.bridge.Call("measureMetrics", font, language)
	if res.Type() != js.TypeObject || res.IsNull() || res.Length() < 4 {
		return 0, 0, 0, 0, false
	}
	return res.Index(0).Float(), res.Index(1).Float(), res.Index(2).Float(), res.Index(3).Float(), true
}

// Prepare implements engine.Platform.
func (s *surface) Prepare(t *driver.AttributedText) driver.PreparedText { return s.text.Prepare(t) }

// MountsAtOnce implements engine.SharedMounter: Mount applies a batch
// before it returns, as the engine shares the logic layer's one thread.
func (s *surface) MountsAtOnce() {}

// Mount implements engine.Platform.
func (s *surface) Mount(b *driver.MountBatch) {
	s.mu.Lock()
	defer s.mu.Unlock()
	driver.Apply(b, s.m.mount)
}

// RequestFrame implements engine.Platform.
func (s *surface) RequestFrame()             { s.m.requestFrame() }
func (s *surface) SubmitGPU(driver.GPUFrame) {}

func (s *surface) submit(ops []byte) { s.bridge.Call("submit", string(ops)) }

func (s *surface) dispatch(ev driver.Event) { s.engine.Dispatch(ev) }

func (s *surface) tick() { s.bridge.Call("tick") }

// device is what the runtime reports about the page and the device.
func (s *surface) device() js.Value { return s.bridge.Call("device") }

func (s *surface) size() driver.Size {
	d := s.device()
	return driver.Size{Width: driver.Points(d.Get("width").Float()), Height: driver.Points(d.Get("height").Float())}
}

// environment reads the environment from the runtime's device.
func (s *surface) environment() driver.Environment {
	d := s.device()
	num := func(name string) driver.Points { return driver.Points(d.Get(name).Float()) }
	var model string
	if m := d.Get("deviceModel"); m.Type() == js.TypeString {
		model = m.String()
	}
	env := s.env
	env.MiniProgramSignIn = s.bridge.Get("canSignIn").Truthy()
	return buildEnvironment(env, deviceInfo{
		SafeArea:     driver.Edges{Top: num("safeTop"), Right: num("safeRight"), Bottom: num("safeBottom"), Left: num("safeLeft")},
		Dark:         d.Get("dark").Truthy(),
		PixelRatio:   float32(d.Get("pixelRatio").Float()),
		FontScale:    float32(d.Get("fontScale").Float()),
		Language:     d.Get("language").String(),
		OS:           d.Get("os").String(),
		OSVersion:    d.Get("osVersion").String(),
		Tablet:       d.Get("tablet").Truthy(),
		HasCameraAPI: s.api.Get("chooseMedia").Type() == js.TypeFunction,
		DeviceModel:  model,
	})
}

// changed sends the environment and size again: the window resized, the
// theme or the keyboard changed.
func (s *surface) changed() {
	env := s.environment()
	if !env.Equal(s.env) {
		s.env = env
		s.m.reduce = env.ReduceMotion
		s.engine.SetEnvironment(env)
	}
	size := s.size()
	s.m.viewport = size
	s.engine.Resize(size, driver.Geometry{})
}

// event handles what the runtime reports: its name, the view's ID, then
// what the event carries.
func (s *surface) event(args []js.Value) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name := args[0].String()
	arg := func(i int) js.Value {
		if i < len(args) {
			return args[i]
		}
		return js.Undefined()
	}
	id := driver.NodeID(arg(1).Int())
	pt := func(i int) driver.Point {
		return driver.Point{X: driver.Points(arg(i).Float()), Y: driver.Points(arg(i + 1).Float())}
	}
	m := s.m
	switch name {
	case "tap":
		m.press(id, driver.EventPress, pt(2))
	case "longpress":
		m.press(id, driver.EventLongPress, pt(2))
	case "pressin":
		m.press(id, driver.EventPressDown, pt(2))
	case "pressout":
		m.press(id, driver.EventPressUp, pt(2))
	case "touch":
		phase := []driver.GesturePhase{driver.GestureBegan, driver.GestureChanged, driver.GestureEnded,
			driver.GestureCancelled}[arg(3).Int()]
		m.touch(id, arg(2).Int(), phase, fingers(arg(4)), fingers(arg(5)),
			time.Duration(arg(6).Float()*float64(time.Millisecond)))
	case "scroll":
		m.scrolled(id, pt(2))
	case "dragstart":
		m.scrollPhase(id, driver.EventScrollDragStart)
	case "dragend":
		m.scrollPhase(id, driver.EventScrollDragEnd)
	case "scrollend":
		m.scrollPhase(id, driver.EventScrollEnd)
	case "refresh":
		if v := m.view(id); v != nil {
			m.send(v, driver.Event{Kind: driver.EventRefresh})
		}
	case "input":
		m.edited(id, arg(2).String())
	case "confirm":
		m.field(id, driver.EventSubmitEditing, arg(2).String())
	case "focus":
		m.field(id, driver.EventFocus, "")
	case "blur":
		m.field(id, driver.EventBlur, "")
	case "keyboard":
		s.env.KeyboardHeight = driver.Points(arg(2).Float())
		s.env.KeyboardDuration = time.Duration(arg(3).Float() * float64(time.Second))
		s.engine.SetEnvironment(s.env)
	case "imgload":
		m.imageLoaded(id, driver.Points(arg(2).Float()), driver.Points(arg(3).Float()))
	case "imgerror":
		if v := m.view(id); v != nil {
			m.send(v, driver.Event{Kind: driver.EventError, Text: arg(2).String()})
			m.send(v, driver.Event{Kind: driver.EventLoadEnd})
		}
	case "selhandle":
		phase := []driver.GesturePhase{driver.GestureBegan, driver.GestureChanged, driver.GestureEnded,
			driver.GestureCancelled}[arg(3).Int()]
		m.selectionHandle(id, arg(2).Int(), phase, pt(4))
	case "vdmeta":
		m.videoLoaded(id, arg(2).Float(), arg(3).Float(), arg(4).Float())
	case "vdplay":
		m.videoPlay(id)
	case "vdpause":
		m.videoPause(id)
	case "vdend":
		m.videoEnded(id)
	case "vdtime":
		m.videoTimeUpdate(id, arg(2).Float(), arg(3).Float())
	case "vdwait":
		m.videoWaiting(id)
	case "vderror":
		m.videoError(id, arg(2).String())
	case "back":
		s.engine.Dispatch(driver.Event{Kind: driver.EventBack})
	case "frame":
		m.tick(time.Since(s.started))
	case "device":
		s.changed()
	case "lifecycle":
		s.env.Lifecycle = driver.LifecycleActive
		if !arg(2).Truthy() {
			s.env.Lifecycle = driver.LifecycleBackground
		}
		s.engine.SetEnvironment(s.env)
	case "alert":
		s.m.answerAlert(uint32(arg(2).Int()), arg(3).Int())
	}
}

// fingers reads the runtime's fingers: [id, pageX, pageY] each.
func fingers(list js.Value) []finger {
	if list.Type() != js.TypeObject {
		return nil
	}
	out := make([]finger, list.Length())
	for i := range out {
		f := list.Index(i)
		out[i] = finger{id: f.Index(0).Int(), page: driver.Point{X: driver.Points(f.Index(1).Float()), Y: driver.Points(f.Index(2).Float())}}
	}
	return out
}

// canvasPainter holds paintDrawing once core constructs a Canvas or Icon, so mini programs that never draw a canvas drop internal/platform/canvas2d.
var canvasPainter atomic.Pointer[func(*surface, driver.NodeID, []driver.DrawOp, driver.Size, float64, bool) string]

func (*surface) EnableCanvas() {
	if canvasPainter.Load() != nil {
		return
	}
	fn := (*surface).paintDrawing
	canvasPainter.Store(&fn)
}

// paint draws ops on the offscreen canvas and returns the picture as a
// data URL, "" while an image they draw loads.
func (s *surface) paint(id driver.NodeID, ops []driver.DrawOp, size driver.Size, scale float64, smooth bool) string {
	if fn := canvasPainter.Load(); fn != nil {
		return (*fn)(s, id, ops, size, scale, smooth)
	}
	return ""
}

// paintDrawing draws ops at the pixel ratio times scale, which <image>
// stretches to the canvas's box smoothly; drawn below the pixel ratio to
// their nearest, they are stretched to it on a second canvas first.
func (s *surface) paintDrawing(id driver.NodeID, ops []driver.DrawOp, size driver.Size, scale float64, smooth bool) string {
	full := float64(s.env.PixelRatio)
	if full <= 0 {
		full = 2
	}
	ratio := full * scale
	w, h := float64(size.Width), float64(size.Height)
	if !s.canvas.Truthy() {
		s.canvas = s.api.Call("createOffscreenCanvas", map[string]any{"type": "2d", "width": 1, "height": 1})
	}
	c := s.canvas
	// Setting the size clears the canvas and resets its context.
	c.Set("width", int(w*ratio+0.999))
	c.Set("height", int(h*ratio+0.999))
	ctx := c.Call("getContext", "2d")
	ctx.Call("scale", ratio, ratio)
	waiting := false
	p := canvas2d.NewPlayer(ctx, ratio, math.Hypot(w, h))
	p.Font = canvasFont
	p.NewCanvas = func(width, height int) js.Value {
		return s.api.Call("createOffscreenCanvas", map[string]any{"type": "2d", "width": width, "height": height})
	}
	p.Image = func(src driver.ImageSource, _ bool) (js.Value, float64, bool) {
		if src.Pixels != nil {
			canvas := s.canvasPixelFrame(src.Pixels)
			return canvas, 1, canvas.Truthy()
		}
		if src.Offscreen != nil {
			// Mini programs draw no offscreen drawings yet.
			return js.Undefined(), 1, false
		}
		im := s.canvasImage(src, id)
		if !im.loaded {
			waiting = waiting || !im.failed
			return js.Undefined(), 1, false
		}
		return im.img, float64(src.Density()), true
	}
	p.MeshCanvas = s.scratchMeshCanvas
	p.Texture = func(src driver.ImageSource) canvasmesh.Texture {
		switch {
		case src.Pixels != nil:
			return canvasmesh.RGBA(src.Pixels, src.Size)
		case src.Offscreen != nil:
			return canvasmesh.Texture{}
		}
		im := s.canvasImage(src, id)
		if !im.loaded {
			waiting = waiting || !im.failed
			return canvasmesh.Texture{}
		}
		if !im.readTexture {
			im.texture = canvas2d.ReadTexture(s.scratchMeshCanvas(), im.img, float64(src.Density()))
			im.readTexture = true
			s.images.grow(src.Key(), int64(len(im.texture.Pix)))
		}
		return im.texture
	}
	for i := range ops {
		p.Play(&ops[i])
	}
	s.images.endPass()
	if waiting {
		return ""
	}
	if !smooth && scale < 1 {
		c = s.stretch(c, int(w*full+0.999), int(h*full+0.999))
	}
	return c.Call("toDataURL", "image/png").String()
}

// stretch draws c over width × height pixels of stretchCanvas, each of
// its pixels filling those nearest it, and returns stretchCanvas.
func (s *surface) stretch(c js.Value, width, height int) js.Value {
	if !s.stretchCanvas.Truthy() {
		s.stretchCanvas = s.api.Call("createOffscreenCanvas", map[string]any{"type": "2d", "width": 1, "height": 1})
	}
	out := s.stretchCanvas
	out.Set("width", width)
	out.Set("height", height)
	ctx := out.Call("getContext", "2d")
	ctx.Set("imageSmoothingEnabled", false)
	ctx.Call("drawImage", c, 0, 0, width, height)
	return out
}

func (s *surface) supportsPixelImages() bool { return s.pixelImages }

func (s *surface) imagePixels(id driver.NodeID, pixels *image.RGBA, tint driver.Color) {
	if pixels == nil {
		s.bridge.Call("releasePixels", int(id))
		return
	}
	bytes := s.bridge.Call("pixelBuffer", int(id), len(pixels.Pix))
	js.CopyBytesToJS(bytes, pixels.Pix)
	if tint != 0 {
		s.bridge.Call("showPixels", int(id), pixels.Rect.Dx(), pixels.Rect.Dy(),
			[]any{int(byte(tint >> 24)), int(byte(tint >> 16)), int(byte(tint >> 8)), int(byte(tint))})
		return
	}
	s.bridge.Call("showPixels", int(id), pixels.Rect.Dx(), pixels.Rect.Dy(), nil)
}

func (s *surface) setClipboard(text string) {
	s.call("setClipboardData", map[string]any{"data": text}, nil)
}

type canvasPixelFrame struct {
	pixels *image.RGBA
	canvas js.Value
}

// canvasPixelFrame keeps the recent raw frames that canvases draw, bounded
// by count and area as on the web.
func (s *surface) canvasPixelFrame(pixels *image.RGBA) js.Value {
	for i, frame := range s.pixelFrames {
		if frame.pixels == pixels {
			copy(s.pixelFrames[i:], s.pixelFrames[i+1:])
			s.pixelFrames[len(s.pixelFrames)-1] = frame
			return frame.canvas
		}
	}
	w, h := pixels.Rect.Dx(), pixels.Rect.Dy()
	bytes := js.Global().Get("Uint8Array").New(len(pixels.Pix))
	js.CopyBytesToJS(bytes, pixels.Pix)
	canvas := s.bridge.Call("pixelCanvas", bytes, w, h)
	if !canvas.Truthy() {
		return canvas
	}
	for len(s.pixelFrames) > 0 && (len(s.pixelFrames) >= 8 || s.pixelCount+w*h > 1<<23) {
		s.pixelCount -= len(s.pixelFrames[0].pixels.Pix) / 4
		s.pixelFrames = slices.Delete(s.pixelFrames, 0, 1)
	}
	s.pixelCount += w * h
	s.pixelFrames = append(s.pixelFrames, canvasPixelFrame{pixels, canvas})
	return canvas
}

// canvasImage is an image canvases draw, cached within the surface's count and byte bounds, and the pixels meshes sample, read once a mesh first draws it.
type canvasImage struct {
	img            js.Value
	loaded, failed bool
	onload         js.Func
	onerror        js.Func
	texture        canvasmesh.Texture
	readTexture    bool
}

func (im *canvasImage) release() {
	if im == nil {
		return
	}
	if !im.onload.Value.IsUndefined() {
		im.onload.Release()
		im.onload = js.Func{}
	}
	if !im.onerror.Value.IsUndefined() {
		im.onerror.Release()
		im.onerror = js.Func{}
	}
	if im.img.Truthy() {
		im.img.Set("onload", js.Null())
		im.img.Set("onerror", js.Null())
		im.img.Set("src", "")
	}
	im.texture = canvasmesh.Texture{}
}

// scratchMeshCanvas returns the offscreen canvas meshes use, creating it the first time.
func (s *surface) scratchMeshCanvas() js.Value {
	if !s.meshCanvas.Truthy() {
		s.meshCanvas = s.api.Call("createOffscreenCanvas", map[string]any{"type": "2d", "width": 1, "height": 1})
	}
	return s.meshCanvas
}

// canvasImage returns src's image, loading it the first time; the
// canvases waiting for it paint again once it loads.
func (s *surface) canvasImage(src driver.ImageSource, id driver.NodeID) *canvasImage {
	if s.images.onEvict == nil {
		s.images.onEvict = func(im *canvasImage) { im.release() }
	}
	key := src.Key()
	if im, ok := s.images.get(key); ok {
		return im
	}
	if !s.canvas.Truthy() {
		s.canvas = s.api.Call("createOffscreenCanvas", map[string]any{"type": "2d", "width": 1, "height": 1})
	}
	im := &canvasImage{img: s.canvas.Call("createImage")}
	encodedBytes := int64(len(src.Data))
	s.images.addPending(key, im, encodedBytes)
	done := func(ok bool) {
		if !im.onload.Value.IsUndefined() {
			im.onload.Release()
			im.onload = js.Func{}
		}
		if !im.onerror.Value.IsUndefined() {
			im.onerror.Release()
			im.onerror = js.Func{}
		}
		// The automation's goroutine reads the mounter under mu too (testsurface.go).
		s.mu.Lock()
		defer s.mu.Unlock()
		cur, exists := s.images.peek(key)
		if !exists || cur != im {
			return
		}
		im.loaded, im.failed = ok, !ok
		s.m.repaintWaiting()
		bytes := encodedBytes
		if ok && bytes == 0 {
			w := int64(im.img.Get("width").Float())
			h := int64(im.img.Get("height").Float())
			if w > 0 && h > 0 {
				bytes = w * h * 4
			}
		}
		s.images.complete(key, bytes)
	}
	im.onload = js.FuncOf(func(js.Value, []js.Value) any { done(true); return nil })
	im.onerror = js.FuncOf(func(js.Value, []js.Value) any { done(false); return nil })
	im.img.Set("onload", im.onload)
	im.img.Set("onerror", im.onerror)
	im.img.Set("src", s.imageURL(src))
	return im
}

// imageURL is where an <image> loads src: the package's resources for
// asset: paths, a file written in the user's data for bytes, and URLs as
// they are.
func (s *surface) imageURL(src driver.ImageSource) string {
	if len(src.Data) > 0 {
		key := src.Key()
		if path, ok := s.files[key]; ok {
			return path
		}
		ext := ".png"
		switch driver.SniffImage(src.Data) {
		case "image/jpeg":
			ext = ".jpg"
		case "image/gif":
			ext = ".gif"
		case "image/webp":
			ext = ".webp"
		case "image/svg+xml":
			ext = ".svg"
		}
		path := s.api.Get("env").Get("USER_DATA_PATH").String() + "/nsgo-image-" + strconv.Itoa(len(s.files)) + ext
		data := js.Global().Get("Uint8Array").New(len(src.Data))
		js.CopyBytesToJS(data, src.Data)
		s.api.Call("getFileSystemManager").Call("writeFileSync", path, data.Get("buffer"))
		s.files[key] = path
		return path
	}
	if path, ok := strings.CutPrefix(src.URI, "asset:"); ok {
		return "/resources/" + strings.TrimPrefix(path, "/")
	}
	return src.URI
}

// imageMaskURL returns a CSS-usable URL for mask-image in mini-program WXSS:
// remote http(s) and data URIs pass through directly, while embedded bytes and
// packaged asset: files are encoded as base64 data URIs because mini-program
// WebViews cannot load local package or wxfile:// paths from CSS url().
func (s *surface) imageMaskURL(src driver.ImageSource) string {
	dataURI := func(data []byte) string {
		mime := driver.SniffImage(data)
		if mime == "" {
			mime = "image/png"
		}
		return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
	}
	if len(src.Data) > 0 {
		return dataURI(src.Data)
	}
	if p, ok := strings.CutPrefix(src.URI, "asset:"); ok {
		if data, ok := s.readResource(p); ok {
			return dataURI(data)
		}
		return "/resources/" + strings.TrimPrefix(p, "/")
	}
	return src.URI
}
