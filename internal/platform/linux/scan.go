//go:build linux && !android

package linux

import (
	"fmt"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/desktop/locales"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/codescan"
)

// Scanning a code on Linux: the XDG Desktop Portal's Camera interface (org.freedesktop.portal.Camera) checks whether a camera is present (IsCameraPresent), asks the user for access (AccessCamera), and returns a PipeWire file descriptor (OpenPipeWireRemote). GStreamer's pipewiresrc pulls BGRA frames from that stream into a modal GTK 4 window while codescan decodes QR codes and barcodes off the main thread.

const (
	cameraPortalIface = "org.freedesktop.portal.Camera"
	dbusPropsIface    = "org.freedesktop.DBus.Properties"

	gstMessageEOS      = 1 << 0
	gdkKeyEscape       = 0xff1b
	gtkAlignEnd        = 2
	scanDecodeInterval = 200 * time.Millisecond
	scanPullTimeoutNS  = 100_000_000 // 100 ms
)

var scanAPI struct {
	once sync.Once
	ok   bool

	// GDBus / GVariant / GUnixFDList
	callWithFDList   func(conn uintptr, busName, objectPath, iface, method *byte, params, replyType uintptr, flags uint32, timeout int32, inFDList uintptr, outFDList *uintptr, cancellable uintptr, err *uintptr) uintptr
	unixFDListGet    func(list uintptr, index int32, err *uintptr) int32
	variantGetHandle func(v uintptr) int32
	variantGetBool   func(v uintptr) int32

	// GStreamer sample/caps inspection
	sampleGetCaps    func(sample uintptr) uintptr
	capsGetStructure func(caps uintptr, index uint32) uintptr
	structureGetInt  func(s uintptr, field *byte, value *int32) int32

	// GTK modal dialog widgets
	buttonNewWithLabel  func(label *byte) uintptr
	windowSetModal      func(win uintptr, modal int32)
	windowSetTransient  func(win, parent uintptr)
	widgetSetHalign     func(widget uintptr, align int32)
	widgetSetMarginTop  func(widget uintptr, margin int32)
	widgetSetMarginBot  func(widget uintptr, margin int32)
	widgetSetMarginEnd  func(widget uintptr, margin int32)
	widgetSetMarginInit func(widget uintptr, margin int32)

	// Static purego callbacks
	cbPortalResponse uintptr
	cbCancelClicked  uintptr
	cbCloseRequest   uintptr
	cbKeyPressed     uintptr
}

var (
	scanProbeOnce sync.Once
	scanProbeOK   bool

	// Test hooks for capability and pipeline checks.
	scanCameraAvailable = cameraAvailable
	scanGSTAvailable    = scanGSTReady
)

// bindScanCamera loads the GDBus, GStreamer, and GTK symbols used by the code scanner.
func bindScanCamera() bool {
	scanAPI.once.Do(func() {
		bindGlobalHotKeys()
		if !loadSound() {
			return
		}
		gstLib, err := purego.Dlopen("libgstreamer-1.0.so.0", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			return
		}
		bind := func(fn any, lib uintptr, name string) bool {
			sym, err := purego.Dlsym(lib, name)
			if err != nil {
				return false
			}
			purego.RegisterFunc(fn, sym)
			return true
		}
		for _, item := range []struct {
			fn   any
			lib  uintptr
			name string
		}{
			{&scanAPI.callWithFDList, gioLib, "g_dbus_connection_call_with_unix_fd_list_sync"},
			{&scanAPI.unixFDListGet, gioLib, "g_unix_fd_list_get"},
			{&scanAPI.variantGetHandle, glibLib, "g_variant_get_handle"},
			{&scanAPI.variantGetBool, glibLib, "g_variant_get_boolean"},
			{&scanAPI.sampleGetCaps, gstLib, "gst_sample_get_caps"},
			{&scanAPI.capsGetStructure, gstLib, "gst_caps_get_structure"},
			{&scanAPI.structureGetInt, gstLib, "gst_structure_get_int"},
			{&scanAPI.buttonNewWithLabel, gtkLib, "gtk_button_new_with_label"},
			{&scanAPI.windowSetModal, gtkLib, "gtk_window_set_modal"},
			{&scanAPI.windowSetTransient, gtkLib, "gtk_window_set_transient_for"},
			{&scanAPI.widgetSetHalign, gtkLib, "gtk_widget_set_halign"},
			{&scanAPI.widgetSetMarginTop, gtkLib, "gtk_widget_set_margin_top"},
			{&scanAPI.widgetSetMarginBot, gtkLib, "gtk_widget_set_margin_bottom"},
			{&scanAPI.widgetSetMarginEnd, gtkLib, "gtk_widget_set_margin_end"},
			{&scanAPI.widgetSetMarginInit, gtkLib, "gtk_widget_set_margin_start"},
		} {
			if !bind(item.fn, item.lib, item.name) {
				return
			}
		}
		scanAPI.cbPortalResponse = newCallback(scanPortalResponse)
		scanAPI.cbCancelClicked = newCallback(scanCancelClicked)
		scanAPI.cbCloseRequest = newCallback(scanCloseRequest)
		scanAPI.cbKeyPressed = newCallback(scanKeyPressed)
		scanAPI.ok = true
	})
	return scanAPI.ok
}

// scanGSTReady reports whether GStreamer and the PipeWire video elements needed for scanning are installed.
func scanGSTReady() bool {
	return bindScanCamera() && gstFeature("pipewiresrc") && gstFeature("videoconvert") && gstFeature("appsink")
}

// cameraAvailable reports whether GStreamer has PipeWire camera support and org.freedesktop.portal.Camera reports IsCameraPresent == true.
func cameraAvailable() bool {
	scanProbeOnce.Do(func() {
		if !scanGSTAvailable() {
			return
		}
		bus := portalBus("")
		if bus == 0 {
			return
		}
		scanProbeOK = portalCameraPresent(bus)
	})
	return scanProbeOK
}

// portalCameraPresent reads org.freedesktop.portal.Camera's IsCameraPresent property with a bounded 250 ms timeout so headless systems without a portal fail fast.
func portalCameraPresent(bus uintptr) bool {
	args := [2]uintptr{
		gVariantTakeRef(gVariantNewString(cstr(cameraPortalIface))),
		gVariantTakeRef(gVariantNewString(cstr("IsCameraPresent"))),
	}
	defer gVariantUnref(args[0])
	defer gVariantUnref(args[1])
	params := gVariantNewTuple(&args[0], 2)
	var err uintptr
	reply := gDBusConnectionCallSync(bus, cstr(globalShortcutName), cstr(globalShortcutPath),
		cstr(dbusPropsIface), cstr("Get"), params, 0, 0, 250, 0, &err)
	if err != 0 {
		gErrorFree(err)
		return false
	}
	if reply == 0 {
		return false
	}
	defer gVariantUnref(reply)
	if gVariantNChildren(reply) < 1 {
		return false
	}
	outer := gVariantGetChildValue(reply, 0)
	defer gVariantUnref(outer)
	inner := gVariantGetVariant(outer)
	if inner == 0 {
		return false
	}
	defer gVariantUnref(inner)
	return scanAPI.variantGetBool(inner) != 0
}

// scanPipelineDescription builds the GStreamer pipeline pulling BGRA frames from PipeWire fd.
func scanPipelineDescription(fd int32) string {
	return fmt.Sprintf("pipewiresrc fd=%d ! videoconvert ! video/x-raw,format=BGRA ! appsink name=sink drop=true max-buffers=1 sync=false", fd)
}

// scanPortalResponseMessage translates an XDG Desktop Portal Response code into an error message.
func scanPortalResponseMessage(code uint32) string {
	switch code {
	case 0:
		return ""
	case 1:
		return "camera access denied"
	default:
		return "camera access failed"
	}
}

// codeScanner holds the state of an active Linux camera scan.
type codeScanner struct {
	w        *window
	id       uint32
	req      driver.ScanRequest
	done     atomic.Bool
	reading  atomic.Bool
	lastScan atomic.Int64

	// Portal request subscription, owned by GTK main thread.
	bus        uintptr
	requestSub uint32

	// GTK dialog widgets, owned by GTK main thread.
	dialog  uintptr
	picture uintptr
}

// scanCode starts a Linux camera scan via the desktop portal and GStreamer.
func (w *window) scanCode(r driver.ScanRequest, id uint32) {
	fail := func(why string) {
		w.engine.Dispatch(driver.Event{Kind: driver.EventPick, Count: id, Text: why})
	}
	if !codescan.HasReader(r) {
		fail("the camera reads none of the codes asked for")
		return
	}
	if !scanGSTAvailable() {
		fail("GStreamer pipewiresrc is unavailable")
		return
	}
	if w.scanner != nil {
		w.finishScanner(w.scanner, nil, "")
	}
	sc := &codeScanner{w: w, id: id, req: r}
	w.scanner = sc

	appID := w.appID
	portalQueue() <- func() {
		bus := portalBus(appID)
		if bus == 0 {
			w.post(func() { w.finishScanner(sc, nil, "the desktop camera portal is unavailable") })
			return
		}
		w.post(func() {
			if sc.done.Load() || w.scanner != sc {
				return
			}
			sc.bus = bus
			token := portalToken("nsgo_cam")
			path := portalRequestPath(bus, token)
			if path == "" {
				w.finishScanner(sc, nil, "the desktop camera portal is unavailable")
				return
			}
			sc.requestSub = gDBusConnectionSignalSubscribe(bus, cstr(globalShortcutName), cstr(globalRequestIface),
				cstr("Response"), cstr(path), nil, 0, scanAPI.cbPortalResponse, w.handle, 0)
			if sc.requestSub == 0 {
				w.finishScanner(sc, nil, "the desktop camera portal is unavailable")
				return
			}
			portalQueue() <- func() {
				reply := portalCall(bus, globalShortcutPath, cameraPortalIface, "AccessCamera", portalOptions(token))
				if reply == 0 {
					w.post(func() { w.finishScanner(sc, nil, "the desktop camera portal is unavailable") })
					return
				}
				gVariantUnref(reply)
			}
		})
	}
}

// scanPortalResponse handles org.freedesktop.portal.Request.Response for AccessCamera.
func scanPortalResponse(conn, sender, path, iface, signal, params, data uintptr) {
	w := windowOf(data)
	if w == nil || w.scanner == nil || w.scanner.done.Load() {
		return
	}
	sc := w.scanner
	if sc.requestSub != 0 {
		gDBusConnectionSignalUnsubscribe(sc.bus, sc.requestSub)
		sc.requestSub = 0
	}
	if gVariantNChildren(params) < 1 {
		w.finishScanner(sc, nil, "camera access failed")
		return
	}
	if msg := scanPortalResponseMessage(portalTupleUint32(params, 0)); msg != "" {
		w.finishScanner(sc, nil, msg)
		return
	}
	bus := sc.bus
	portalQueue() <- func() {
		fd := openPipeWireRemote(bus)
		if fd < 0 {
			w.post(func() { w.finishScanner(sc, nil, "failed to open camera stream") })
			return
		}
		w.post(func() {
			if sc.done.Load() || w.scanner != sc {
				syscall.Close(int(fd))
				return
			}
			sc.startPipelineAndDialog(fd)
		})
	}
}

// openPipeWireRemote calls org.freedesktop.portal.Camera.OpenPipeWireRemote and returns the PipeWire fd, or -1 on error.
func openPipeWireRemote(bus uintptr) int32 {
	opts := portalVariant("a{sv}", "{}")
	if opts == 0 {
		return -1
	}
	args := [1]uintptr{gVariantTakeRef(opts)}
	defer gVariantUnref(args[0])
	params := gVariantNewTuple(&args[0], 1)
	var outFDList, gerr uintptr
	reply := scanAPI.callWithFDList(bus, cstr(globalShortcutName), cstr(globalShortcutPath),
		cstr(cameraPortalIface), cstr("OpenPipeWireRemote"), params, 0, 0, -1, 0, &outFDList, 0, &gerr)
	if gerr != 0 {
		gErrorFree(gerr)
	}
	if reply == 0 {
		if outFDList != 0 {
			gObjectUnref(outFDList)
		}
		return -1
	}
	defer gVariantUnref(reply)
	if outFDList == 0 {
		return -1
	}
	defer gObjectUnref(outFDList)
	if gVariantNChildren(reply) < 1 {
		return -1
	}
	child := gVariantGetChildValue(reply, 0)
	defer gVariantUnref(child)
	handleIdx := scanAPI.variantGetHandle(child)
	var fdErr uintptr
	fd := scanAPI.unixFDListGet(outFDList, handleIdx, &fdErr)
	if fdErr != 0 {
		gErrorFree(fdErr)
		return -1
	}
	return fd
}

// startPipelineAndDialog opens the GTK 4 modal preview dialog and launches the GStreamer PipeWire capture pipeline.
func (sc *codeScanner) startPipelineAndDialog(fd int32) {
	var gerr uintptr
	pipeline := gstParseLaunch(cstr(scanPipelineDescription(fd)), &gerr)
	if gerr != 0 {
		msg := soundError(gerr)
		if pipeline != 0 {
			gstObjectUnref(pipeline)
		}
		syscall.Close(int(fd))
		sc.w.finishScanner(sc, nil, msg)
		return
	}
	if pipeline == 0 {
		syscall.Close(int(fd))
		sc.w.finishScanner(sc, nil, "failed to start camera pipeline")
		return
	}
	sink := gstBinGetByName(pipeline, cstr("sink"))
	if sink == 0 {
		gstObjectUnref(pipeline)
		syscall.Close(int(fd))
		sc.w.finishScanner(sc, nil, "failed to start camera pipeline")
		return
	}
	bus := gstElementGetBus(pipeline)

	// Build modal GTK 4 preview dialog.
	win := gtkWindowNew()
	if top := sc.w.toplevel(); top != 0 {
		scanAPI.windowSetTransient(win, top)
		scanAPI.windowSetModal(win, 1)
	}
	gtkWindowSetDefaultSize(win, 520, 420)
	gtkWindowSetDecorated(win, 1)

	vbox := gtkBoxNew(1, 0) // GTK_ORIENTATION_VERTICAL = 1
	pic := gtkPictureNew()
	gtkPictureSetCanShrink(pic, 1)
	gtkPictureSetContentFit(pic, gtkContentFitContain)
	gtkWidgetSetHexpand(pic, 1)
	gtkWidgetSetVexpand(pic, 1)
	gtkBoxAppend(vbox, pic)

	bar := gtkBoxNew(0, 8) // GTK_ORIENTATION_HORIZONTAL = 0
	scanAPI.widgetSetHalign(bar, gtkAlignEnd)
	scanAPI.widgetSetMarginTop(bar, 12)
	scanAPI.widgetSetMarginBot(bar, 12)
	scanAPI.widgetSetMarginInit(bar, 16)
	scanAPI.widgetSetMarginEnd(bar, 16)
	cancelBtn := scanAPI.buttonNewWithLabel(cstr(locales.In(sc.w.language).Cancel()))
	gSignalConnect(cancelBtn, "clicked", scanAPI.cbCancelClicked, sc.w.handle)
	gtkBoxAppend(bar, cancelBtn)
	gtkBoxAppend(vbox, bar)

	gtkWindowSetChild(win, vbox)
	gSignalConnect(win, "close-request", scanAPI.cbCloseRequest, sc.w.handle)
	keys := gtkEventControllerKeyNew()
	gSignalConnect(keys, "key-pressed", scanAPI.cbKeyPressed, sc.w.handle)
	gtkWidgetAddController(win, keys)

	sc.dialog = win
	sc.picture = pic
	gtkWindowPresent(win)

	gstElementSetState(pipeline, gstStatePlaying)
	go sc.pullFrames(pipeline, sink, bus, fd)
}

// pullFrames pulls BGRA camera frames from GStreamer appsink, updates the preview, and decodes codes.
func (sc *codeScanner) pullFrames(pipeline, sink, bus uintptr, fd int32) {
	defer func() {
		gstElementSetState(pipeline, gstStateNull)
		if bus != 0 {
			gstObjectUnref(bus)
		}
		gstObjectUnref(sink)
		gstObjectUnref(pipeline)
		syscall.Close(int(fd))
	}()

	for !sc.done.Load() {
		if bus != 0 {
			if msg := gstBusPopFiltered(bus, gstMessageError|gstMessageEOS); msg != 0 {
				var gerr uintptr
				gstMessageParseError(msg, &gerr, 0)
				gstMiniObjectUnref(msg)
				errText := "the camera stream ended"
				if gerr != 0 {
					errText = soundError(gerr)
				}
				sc.w.post(func() { sc.w.finishScanner(sc, nil, errText) })
				return
			}
		}
		sample := gstAppSinkTryPullSample(sink, scanPullTimeoutNS)
		if sample == 0 {
			if gstAppSinkIsEOS(sink) != 0 {
				return
			}
			continue
		}
		pix, width, height := extractSampleBGRA(sample)
		gstMiniObjectUnref(sample)
		if len(pix) == 0 || width <= 0 || height <= 0 {
			continue
		}

		// Update preview on the GTK main thread.
		previewPix := pix
		sc.w.post(func() {
			if sc.done.Load() || sc.picture == 0 {
				return
			}
			bytes := gBytesNew(unsafe.Pointer(&previewPix[0]), uintptr(len(previewPix)))
			if bytes == 0 {
				return
			}
			const bgraPremultiplied = 0
			tex := gdkMemoryTextureNew(int32(width), int32(height), bgraPremultiplied, bytes, uintptr(width*4))
			gBytesUnref(bytes)
			if tex != 0 {
				gtkPictureSetPaintable(sc.picture, tex)
				gObjectUnref(tex)
			}
		})

		// Decode at most every scanDecodeInterval without stacking decodes.
		now := time.Now().UnixNano()
		if now-sc.lastScan.Load() < int64(scanDecodeInterval) || !sc.reading.CompareAndSwap(false, true) {
			continue
		}
		sc.lastScan.Store(now)
		decodePix := append([]byte(nil), pix...)
		go func() {
			defer sc.reading.Store(false)
			code := codescan.DecodeBGRA(decodePix, width, height, width*4, sc.req)
			if code == nil || sc.done.Load() {
				return
			}
			sc.w.post(func() { sc.w.finishScanner(sc, code, "") })
		}()
	}
}

// extractSampleBGRA copies the BGRA bytes and dimensions out of a GstSample.
func extractSampleBGRA(sample uintptr) ([]byte, int, int) {
	caps := scanAPI.sampleGetCaps(sample)
	if caps == 0 {
		return nil, 0, 0
	}
	st := scanAPI.capsGetStructure(caps, 0)
	if st == 0 {
		return nil, 0, 0
	}
	var w, h int32
	if scanAPI.structureGetInt(st, cstr("width"), &w) == 0 || scanAPI.structureGetInt(st, cstr("height"), &h) == 0 || w <= 0 || h <= 0 {
		return nil, 0, 0
	}
	buf := gstSampleGetBuffer(sample)
	if buf == 0 {
		return nil, 0, 0
	}
	need := int(w) * int(h) * 4
	size := int(gstBufferGetSize(buf))
	if size < need {
		return nil, 0, 0
	}
	pix := make([]byte, need)
	if int(gstBufferExtract(buf, 0, unsafe.Pointer(&pix[0]), uintptr(need))) < need {
		return nil, 0, 0
	}
	return pix, int(w), int(h)
}

func scanCancelClicked(_, data uintptr) {
	if w := windowOf(data); w != nil && w.scanner != nil {
		w.finishScanner(w.scanner, nil, "")
	}
}

func scanCloseRequest(_, data uintptr) int32 {
	if w := windowOf(data); w != nil && w.scanner != nil {
		w.finishScanner(w.scanner, nil, "")
	}
	return 1
}

func scanKeyPressed(_, keyval, _, _, data uintptr) int32 {
	if keyval == gdkKeyEscape {
		if w := windowOf(data); w != nil && w.scanner != nil {
			w.finishScanner(w.scanner, nil, "")
			return 1
		}
	}
	return 0
}

// finishScanner tears down sc once on the GTK main thread and dispatches EventPick.
func (w *window) finishScanner(sc *codeScanner, code *driver.ScannedCode, errText string) {
	if sc == nil || !sc.done.CompareAndSwap(false, true) {
		return
	}
	if sc.requestSub != 0 && sc.bus != 0 {
		gDBusConnectionSignalUnsubscribe(sc.bus, sc.requestSub)
		sc.requestSub = 0
	}
	if sc.dialog != 0 {
		dialog := sc.dialog
		sc.dialog = 0
		sc.picture = 0
		gtkWindowDestroy(dialog)
	}
	if w.scanner == sc {
		w.scanner = nil
	}
	ev := driver.Event{Kind: driver.EventPick, Count: sc.id, Text: errText}
	if code != nil {
		ev.Data = code
	}
	if w.engine != nil {
		w.engine.Dispatch(ev)
	}
}

// closeScanner cancels any active scanner when window w closes.
func (w *window) closeScanner() {
	if w.scanner != nil {
		w.finishScanner(w.scanner, nil, "")
	}
}
