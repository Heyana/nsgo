//go:build js && wasm

// Package harmonyos is NSGo's HarmonyOS backend: the engine and the app run
// in WebAssembly inside ArkWeb, and an ArkTS render layer draws each mount
// batch with ArkUI views. The engine lays the app out as it does on every
// target and measures text through the platform's own measurer, so a view
// lands where ArkUI would put it; the render layer positions native views
// at those frames (docs/architecture/harmonyos.md).
//
// The page holds the bridge the host answers, nsgoHarmony: what it calls in
// Go (submit, measure, device, ready, tick), Go sets on it, and what Go
// calls it provides, as the mini programs' runtime does for nsgoMP.
package harmonyos

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"path/filepath"
	"syscall/js"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

// Options configure the app.
type Options struct {
	// ID names the app's entries in the host's storage.
	ID string
	// Appearance is the appearance the app starts in.
	Appearance driver.Appearance
	// Perf shows the performance monitor over the app (--nsgo-fps).
	Perf bool
}

// surface is the app's surface: the engine's platform.
type surface struct {
	bridge js.Value
	m      *mounter
	text   *textEngine
	engine *engine.Engine
	env    driver.Environment
	size0  driver.Size
	anim   *driver.Animator
	// clock times the frames the display shows; perf measures them when
	// --nsgo-fps asks, and lastPerf times its readings. dirty records that
	// a batch was applied since the last frame: only a frame that changed
	// something is one the screen showed.
	clock *driver.Clock
	perf  *engine.PerfMonitor
	// display brings the host's frame stamps to Go's clock, which the web view cannot read.
	display  driver.DisplayClock
	lastPerf time.Time
	dirty    bool
	// dispatchFn overrides engine.Dispatch in unit tests that exercise surface without a running engine.
	dispatchFn func(driver.Event)
}

// Run runs app in ArkWeb. Call it from main; it does not return.
func Run(app engine.App, opts Options) {
	s := newSurface(opts)
	engineOpts := engine.Options{
		ReadAsset: readAsset,
		Size:      s.size0, Environment: s.env, Appearance: opts.Appearance, Perf: s.perf,
	}
	if paths := js.Global().Get("nsgoSandboxPaths"); paths.Type() == js.TypeObject {
		engineOpts.DataDir = paths.Get("data").String()
		engineOpts.CacheDir = paths.Get("cache").String()
		engineOpts.LogsDir = filepath.Join(engineOpts.DataDir, "logs")
	}
	s.engine = engine.Start(s, app, engineOpts)
	s.install()
	s.bridge.Call("ready")
	// A monitor measures what a display shows, so its clock keeps running
	// while the app is still; without one the clock waits for an animation.
	if s.clock.Watched() {
		s.bridge.Call("tick")
	}
	select {}
}

// newSurface makes the surface an app runs on, before its engine starts.
func newSurface(opts Options) *surface {
	s := &surface{bridge: js.Global().Get("nsgoHarmony")}
	s.text = &textEngine{bridge: s.bridge}
	s.m = newMounter(s)
	s.anim = driver.NewAnimator(func(ev driver.Event) { s.engine.Dispatch(ev) })
	s.clock = driver.NewClock(s.anim)
	s.m.anim = s.anim
	s.m.scene.anim = s.anim
	s.m.frame = func() { s.frame(time.Time{}) }
	s.env = s.environment()
	s.size0 = s.size()
	s.m.packedAtlas = s.device().CanvasAtlas
	if opts.Perf {
		s.perf = engine.NewPerfMonitor()
		s.clock.Watch(s.perf)
	}
	return s
}

// install sets the functions the page calls into the module.
func (s *surface) install() {
	g := js.Global()
	g.Set("nsgoEvent", js.FuncOf(func(_ js.Value, args []js.Value) any {
		s.event(args)
		return nil
	}))
	g.Set("nsgoTouches", js.FuncOf(func(_ js.Value, args []js.Value) any {
		s.touches(args)
		return nil
	}))
	// nsgoFrame is a display frame, with its displaySync targetTimestamp in nanoseconds, when it shows.
	g.Set("nsgoFrame", js.FuncOf(func(_ js.Value, args []js.Value) any {
		var at time.Time
		if len(args) > 0 && args[0].Type() == js.TypeNumber {
			at = s.display.At(time.Duration(args[0].Float()), time.Now())
		}
		s.frame(at)
		return nil
	}))
	// nsgoEnv is the host saying the window or the system bars changed, once
	// it has measured them.
	g.Set("nsgoEnv", js.FuncOf(func(js.Value, []js.Value) any {
		s.changed()
		return nil
	}))
}

// touches reads what the page reported about the fingers on the surface: the phase, the time in milliseconds and each finger's id, position and whether the event is about it, then snapshots the current presentation targets and local coordinates before dispatching to the engine.
func (s *surface) touches(args []js.Value) {
	if len(args) < 3 {
		return
	}
	var fingers []struct {
		ID      int     `json:"i"`
		X       float64 `json:"x"`
		Y       float64 `json:"y"`
		Changed bool    `json:"c"`
	}
	if err := json.Unmarshal([]byte(args[2].String()), &fingers); err != nil {
		log.Printf("nsgo harmony: touches: %v", err)
		return
	}
	ev := driver.Event{Kind: driver.EventTouch, Phase: driver.GesturePhase(args[0].Int()),
		Time: time.Duration(args[1].Float() * float64(time.Millisecond))}
	for _, f := range fingers {
		p := driver.Point{X: driver.Points(f.X), Y: driver.Points(f.Y)}
		ev.Touches = append(ev.Touches, driver.Touch{ID: f.ID, Position: p, PagePosition: p, Changed: f.Changed})
	}
	s.m.captureTouches(&ev)
	s.dispatch(ev)
}

// Prepare implements engine.Platform.
func (s *surface) Prepare(t *driver.AttributedText) driver.PreparedText { return s.text.prepare(t) }

// MountsAtOnce implements engine.SharedMounter: the batch is applied before
// Mount returns, as the engine and the host share one thread.
func (s *surface) MountsAtOnce() {}

// Mount implements engine.Platform.
func (s *surface) Mount(b *driver.MountBatch) {
	s.dirty = true
	s.m.mount(b)
}

// RequestFrame implements engine.Platform: the host keeps a display frame
// clock running, which calls nsgoFrame until the animator stops.
func (s *surface) RequestFrame()             { s.bridge.Call("tick") }
func (s *surface) SubmitGPU(driver.GPUFrame) {}

// frame runs one display frame, which shows at at, or the zero time where the host gave none: the animator moves its values, the views that changed are written again, and the engine commits. The host's clock stops when nothing animates.
func (s *surface) frame(at time.Time) {
	s.clock.BeginFrame(time.Now())
	s.clock.Step(s.clock.Elapsed())
	if s.m.applyAnimated() {
		// A frame the animation moved shows, even when the engine
		// committed nothing: the monitor counts frames that changed.
		s.dirty = true
	}
	s.engine.FrameAt(at)
	dirty := s.dirty
	s.dirty = false
	s.clock.EndFrame(time.Now(), 0, dirty)
	if s.perf != nil && time.Since(s.lastPerf) >= 500*time.Millisecond {
		s.lastPerf = time.Now()
		s.bridge.Call("perf", s.perf.Snapshot().Readings())
	}
	// A watched clock keeps the display running while the app is still.
	if !s.anim.Running() && !s.clock.Watched() {
		s.bridge.Call("idle")
	}
}

// submit hands a batch's operations to the render layer.
func (s *surface) submit(ops []byte) { s.bridge.Call("submit", string(ops)) }

// dispatch hands an event to the engine.
func (s *surface) dispatch(ev driver.Event) {
	if s.dispatchFn != nil {
		s.dispatchFn(ev)
		return
	}
	s.engine.Dispatch(ev)
}

func (s *surface) canDispatch() bool {
	return s != nil && (s.dispatchFn != nil || s.engine != nil)
}

func (s *surface) hasBridge(name string) bool {
	return s != nil && !s.bridge.IsUndefined() && !s.bridge.IsNull() && s.bridge.Get(name).Type() == js.TypeFunction
}

func (s *surface) device() deviceData {
	var d deviceData
	json.Unmarshal([]byte(s.bridge.Call("device").String()), &d)
	if d.Width == 0 {
		d.Width, d.Height = 420, 320
	}
	if d.PixelRatio == 0 {
		d.PixelRatio = 3
	}
	return d
}

func (s *surface) size() driver.Size {
	d := s.device()
	return driver.Size{Width: driver.Points(d.Width), Height: driver.Points(d.Height)}
}

// environment reads the environment from the host's device.
func (s *surface) environment() driver.Environment {
	return buildEnvironment(s.env, s.device())
}

// changed reads the environment and the size again, and sends what differs:
// the system bars or the keyboard moved, or the window resized.
func (s *surface) changed() {
	env := s.environment()
	if !env.Equal(s.env) {
		if env.Lifecycle != driver.LifecycleActive && s.env.Lifecycle == driver.LifecycleActive {
			s.m.key(0, 2, 0, "")
		}
		s.env = env
		s.engine.SetEnvironment(env)
		if s.m != nil {
			s.m.reportScreenInsets()
			s.m.reportTabBarSizes()
		}
	}
	size := s.size()
	if size != s.size0 {
		s.size0 = size
		s.engine.Resize(size, driver.Geometry{})
		if s.m != nil {
			s.m.reportScreenInsets()
			s.m.reportTabBarSizes()
		}
	}
}

// pickedFile is one image the host picked, its bytes base64-encoded.
type pickedFile struct {
	Name string `json:"n"`
	MIME string `json:"m"`
	Data string `json:"d"`
}

// picked answers ItemPickImages from the host's JSON: the images it read,
// or none when the user cancelled.
func (s *surface) picked(count uint32, data string) {
	var in []pickedFile
	if data != "" {
		_ = json.Unmarshal([]byte(data), &in)
	}
	files := make([]driver.PickedFile, 0, len(in))
	for _, f := range in {
		b, err := base64.StdEncoding.DecodeString(f.Data)
		if err != nil {
			continue
		}
		mime, name := f.MIME, f.Name
		if mime == "" {
			mime = "image/jpeg"
		}
		if name == "" {
			name = "image.jpg"
		}
		files = append(files, driver.PickedFile{Name: name, MIME: mime, Data: b})
	}
	s.dispatch(driver.Event{Kind: driver.EventPick, Count: count, Data: files})
}

// pickedPath is one file the host picked: its name, media type, size and
// the path it copied it to in the app's cache.
type pickedPath struct {
	Name string `json:"n"`
	MIME string `json:"m"`
	Size int64  `json:"s"`
	Path string `json:"p"`
}

// pickedFiles answers ItemPickFiles from the host's JSON: the files it
// copied, or none when the user cancelled.
// readFolderFile answers a picked folder's synchronous read on the host.
func (s *surface) readFolderFile(method, uri, name string) string {
	return s.bridge.Call(method, uri, name).String()
}

// pickedFolders turns the folders the user picked into Directories whose
// fs.FS reads them through the host, as Android's document tree does.
func (s *surface) pickedFolders(count uint32, data string) {
	var in []pickedFolder
	if data != "" {
		_ = json.Unmarshal([]byte(data), &in)
	}
	dirs := make([]driver.Directory, 0, len(in))
	for _, f := range in {
		src := &folderSource{uri: f.URI, read: s.readFolderFile}
		dirs = append(dirs, driver.Directory{Name: f.Name, FS: driver.FolderFS(src, driver.FolderEntry{})})
	}
	s.dispatch(driver.Event{Kind: driver.EventFiles, Count: count, Data: dirs})
}

func (s *surface) pickedFiles(count uint32, data string) {
	var in []pickedPath
	if data != "" {
		_ = json.Unmarshal([]byte(data), &in)
	}
	files := make([]driver.File, 0, len(in))
	for _, f := range in {
		files = append(files, driver.File{Name: f.Name, MIME: f.MIME, Size: f.Size, Path: f.Path, Temporary: true})
	}
	s.dispatch(driver.Event{Kind: driver.EventFiles, Count: count, Data: files})
}

// event handles what the host reports: its name, the view's ID, and what it
// carries — a point, or the text a field or a scroll holds.
func (s *surface) event(args []js.Value) {
	if len(args) < 2 {
		return
	}
	name := args[0].String()
	id := driver.NodeID(args[1].Int())
	num := func(i int) float64 {
		if i < len(args) {
			return args[i].Float()
		}
		return 0
	}
	str := func(i int) string {
		if i < len(args) {
			return args[i].String()
		}
		return ""
	}
	pt := driver.Point{X: driver.Points(num(2)), Y: driver.Points(num(3))}
	switch name {
	case "tap":
		s.m.press(id, driver.EventPress, pt, driver.Modifiers{})
	case "longpress":
		s.m.press(id, driver.EventLongPress, pt, driver.Modifiers{})
	case "pressin":
		s.m.press(id, driver.EventPressDown, pt, driver.Modifiers{})
	case "pressout":
		s.m.press(id, driver.EventPressUp, pt, driver.Modifiers{})
	case "input":
		s.m.edited(id, str(4))
	case "confirm":
		s.dispatch(driver.Event{Kind: driver.EventSubmitEditing, ID: id, Text: str(4)})
	case "focus":
		s.m.reportFocus(id)
	case "blur":
		s.m.reportBlur(id)
	case "key":
		s.m.key(id, int(num(2)), int(num(3)), str(4))
	case "scroll":
		// A scroll the platform moved changes the screen with no commit,
		// so the frame it lands in still counts as shown.
		s.dirty = true
		s.m.scrolled(id, pt)
	case "zoom":
		// A pinch the platform recognized: the scale, and where it left
		// the offset.
		s.dirty = true
		s.m.zoomed(id, float32(num(2)), driver.Point{X: driver.Points(num(3)), Y: driver.Points(num(4))})
	case "dragstart":
		s.m.scrollPhase(id, driver.EventScrollDragStart)
	case "dragend":
		s.m.scrollPhase(id, driver.EventScrollDragEnd)
	case "momentumstart":
		s.m.scrollPhase(id, driver.EventMomentumStart)
	case "momentumend":
		s.m.scrollPhase(id, driver.EventMomentumEnd)
	case "refresh":
		s.m.refresh(id)
	case "scrollend":
		s.m.scrollPhase(id, driver.EventScrollEnd)
	case "back":
		s.dispatch(driver.Event{Kind: driver.EventBack, ID: id})
	case "tabselect":
		s.m.tabSelected(id, int(num(2)))
	case "imageinfo":
		s.dispatch(driver.Event{Kind: driver.EventImageInfo, Count: uint32(id),
			Size: driver.Size{Width: driver.Points(num(2)), Height: driver.Points(num(3))}, Text: str(4)})
	case "videoload":
		s.dispatch(driver.Event{Kind: driver.EventLoad, ID: id, Duration: time.Duration(num(2) * float64(time.Second))})
	case "videoprogress":
		s.dispatch(driver.Event{Kind: driver.EventProgress, ID: id,
			Time: time.Duration(num(2) * float64(time.Second)), Duration: time.Duration(num(3) * float64(time.Second))})
	case "videoend":
		s.dispatch(driver.Event{Kind: driver.EventEnd, ID: id})
	case "videoerror":
		s.dispatch(driver.Event{Kind: driver.EventError, ID: id, Text: str(4)})
	case "webloadstart":
		nav := decodeWebNavigation(str(4), str(5), int(num(2)), true)
		s.dispatch(driver.Event{Kind: driver.EventLoadStart, ID: id, Navigation: nav})
		navCopy := *nav
		s.dispatch(driver.Event{Kind: driver.EventNavigationStateChange, ID: id, Navigation: &navCopy})
	case "webload":
		nav := decodeWebNavigation(str(4), str(5), int(num(2)), false)
		s.dispatch(driver.Event{Kind: driver.EventLoad, ID: id, Navigation: nav})
		navCopy := *nav
		s.dispatch(driver.Event{Kind: driver.EventNavigationStateChange, ID: id, Navigation: &navCopy})
	case "webnav":
		flags := int(num(2))
		nav := decodeWebNavigation(str(4), str(5), flags, flags&4 != 0)
		s.dispatch(driver.Event{Kind: driver.EventNavigationStateChange, ID: id, Navigation: nav})
	case "weberror":
		ev := driver.Event{
			Kind:       driver.EventError,
			ID:         id,
			Text:       str(5),
			Navigation: &driver.WebNavigation{URL: str(4)},
		}
		if code, hasDomain := int(num(2)), int(num(3)) != 0; hasDomain || code != 0 {
			domain := ""
			if hasDomain {
				domain = "ohos.web.webview"
			}
			ev.Data = driver.WebFailure{Domain: domain, Code: code}
		}
		s.dispatch(ev)
	case "webmessage":
		pageURL := str(5)
		s.dispatch(driver.Event{
			Kind:      driver.EventMessage,
			ID:        id,
			Text:      str(4),
			URL:       pageURL,
			Origin:    driver.WebOrigin(pageURL),
			MainFrame: true,
		})
	case "webdownload":
		if dl, ok := decodeWebDownload(str(4)); ok {
			s.dispatch(driver.Event{Kind: driver.EventDownload, ID: id, Data: dl})
		}
	case "webopenwindow":
		req := driver.WebWindowRequest{URL: str(4), UserInitiated: int(num(2)) != 0}
		var ev driver.Event
		if v := s.m.views[id]; v != nil && v.web != nil {
			ev = v.web.OpenWindowEvent(req)
		} else {
			ev = driver.Event{Kind: driver.EventWebOpenWindow, Data: req}
		}
		ev.ID = id
		s.dispatch(ev)
	case "webcookies":
		count := uint32(id)
		var filter driver.WebCookieFilter
		if s.m != nil && s.m.cookieFilters != nil {
			filter = s.m.cookieFilters[count]
			delete(s.m.cookieFilters, count)
		}
		s.dispatch(decodeWebCookiesEvent(count, int(num(2)), str(4), str(5), filter, time.Now()))
	case "selection":
		s.m.selectionChanged(id, driver.TextRange{Start: int(num(2)), End: int(num(3))})
	case "alert":
		// The host's alert answered: Count is the alert's ID and Index the
		// button, or -1 for a cancelled one.
		s.dispatch(driver.Event{Kind: driver.EventAlert, Count: uint32(id), Index: int(num(2))})
	case "clip":
		// A clipboard read answered: flag 0 carries its text, 1 whether it
		// holds text at all.
		switch int(num(2)) {
		case 0:
			s.dispatch(driver.Event{Kind: driver.EventClipboard, Count: uint32(id), Text: str(4)})
		case 1:
			s.dispatch(driver.Event{Kind: driver.EventClipboard, Count: uint32(id), Finished: str(4) == "1"})
		}
	case "openurl":
		s.dispatch(driver.Event{Kind: driver.EventOpenURL, Count: uint32(id), Finished: num(2) == 1})
	case "bio":
		// A biometric answer: the availability or the authentication's
		// result, as driver's kinds.
		s.dispatch(driver.Event{Kind: driver.EventBiometric, Count: uint32(id), Index: int(num(2))})
	case "perm":
		// A permission answered: its status, and whether opening the
		// settings opened one.
		s.dispatch(driver.Event{Kind: driver.EventPermission, Count: uint32(id), Index: int(num(2)), Finished: num(3) == 1})
	case "picked":
		s.picked(uint32(id), str(4))
	case "pickedfiles":
		s.pickedFiles(uint32(id), str(4))
	case "pickeddirectories":
		s.pickedFolders(uint32(id), str(4))
	case "saved":
		s.dispatch(driver.Event{Kind: driver.EventFiles, Count: uint32(id), Finished: num(2) == 1, Text: str(4)})
	case "opened":
		s.dispatch(driver.Event{Kind: driver.EventFiles, Count: uint32(id), Finished: num(2) == 1, Text: str(4)})
	}
}
