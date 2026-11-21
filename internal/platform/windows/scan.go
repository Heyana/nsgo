//go:build windows

package windows

import (
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/desktop/locales"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/codescan"
)

// Scanning a code on Windows: WinRT's MediaCapture opens the default video
// capture device in CPU memory mode and streams BGRA8 frames through a
// MediaFrameReader; a modal card drawn in Direct2D shows the live preview with
// a Cancel button while codescan decodes frames off the UI thread.

var (
	mf                  = syscall.NewLazyDLL("mf.dll")
	mfEnumDeviceSources = mf.NewProc("MFEnumDeviceSources")
)

var (
	mfDevSourceAttrSourceType = parseGUID("c60ac5fe-252a-478f-a0ef-bc8fa5f7cad3")
	mfDevSourceAttrVidCapGUID = parseGUID("8ac3587a-4ae7-42d8-99e0-0a6013eef90f")
	iidMediaCapture           = parseGUID("c61afbb4-fb10-4a34-ac18-ca80d9c8e7ee")
	iidMediaCapture5          = parseGUID("da787c22-3a9b-4720-a71e-97900a316e5a")
	iidMediaCaptureInit       = parseGUID("9782ba70-ea65-4900-9356-8ca887726884")
	iidMediaCaptureInit5      = parseGUID("d5a2e3b8-2626-4e94-b7b3-5308a0f64b1a")
	iidMediaFrameReader       = parseGUID("e4c94395-2028-48ed-90b0-d1c1b162e24c")
	iidMediaFrameReference    = parseGUID("f6b88641-f0dc-4044-8dc9-961cedd05bad")
	iidVideoMediaFrame        = parseGUID("00dd4ccb-32bd-4fe1-a013-7cc13cf5dbcf")
	iidMediaFrameSource       = parseGUID("d6782953-90db-46a8-8add-2aa884a8d253")
	iidMediaFrameSourceInfo   = parseGUID("87bdc9cd-4601-408f-91cf-038318cd0af3")
	iidSoftwareBitmap         = parseGUID("689e0708-7eef-483f-963f-da938818e073")
	iidBitmapBuffer           = parseGUID("a53e04c4-399c-438c-b28f-a63a6b83d1a1")
	iidClosable               = parseGUID("30d5a829-7fa4-4026-83bb-d75bae4ea99e")
	iidMemoryBuffer           = parseGUID("fbc4dd2a-245b-11e4-af98-689423260cf8")
	iidMemoryBufferByteAccess = parseGUID("5b0d3235-4dba-4d44-865e-8f1d0e4fd04d")
	iidAsyncInfo              = parseGUID("00000036-0000-0000-c000-000000000046")
	iidAsyncActionCompleted   = parseGUID("a4ed5c81-76c9-40bd-8be6-b1d90fb20ae7")
	iidFrameReaderCompleted   = parseGUID("a6214dad-b917-5c89-a068-e32c9a7037d3")
	iidFrameStartCompleted    = parseGUID("9f49b2e5-2f68-5c58-8d8b-12176ff6ea50")
	iidFrameArrivedHandler    = parseGUID("d3dd49cb-8d25-591a-80f7-8363d5c03ec9")
	iidFrameSourceIterable    = parseGUID("a038e80f-0b3d-5cd4-849d-13036e1f506e")
)

// Vtable slots, verified against Windows SDK / windows-rs definitions.
const (
	// IMFAttributes
	attributesSetGUID = 24

	// IMediaCapture / IMediaCapture5
	mcInitWithSettings      = 7
	mc5FrameSources         = 9
	mc5CreateFrameReaderSub = 11

	// IMediaCaptureInitializationSettings / 5
	mcInitSetStreamingMode    = 10
	mcInit5SetSharingMode     = 9
	mcInit5SetMemoryPref      = 11
	streamingCaptureModeVideo = 2
	sharingModeSharedReadOnly = 1
	memoryPreferenceCPU       = 1

	// IMediaFrameSource / IMediaFrameSourceInfo
	frameSourceInfo      = 6
	frameSourceInfoKind  = 8
	frameSourceKindColor = 1

	// IMediaFrameReader
	frameReaderAddArrived    = 6
	frameReaderRemoveArrived = 7
	frameReaderTryAcquire    = 8
	frameReaderStartAsync    = 9
	frameReaderStopAsync     = 10

	// IMediaFrameReference / IVideoMediaFrame
	frameRefVideoFrame   = 12
	videoFrameSoftBitmap = 8

	// ISoftwareBitmap / IBitmapBuffer / IMemoryBuffer / IMemoryBufferByteAccess / IClosable
	softBitmapPixelFormat = 6
	softBitmapPixelWidth  = 8
	softBitmapPixelHeight = 9
	softBitmapLockBuffer  = 15
	bitmapBufPlaneDesc    = 7
	memBufCreateRef       = 6
	memBufByteAccessGet   = 3
	closableClose         = 6
	bitmapFormatBGRA8     = 87
	bitmapAccessRead      = 0

	// IIterable / IIterator / IKeyValuePair / IAsyncInfo
	iterableFirst    = 6
	iteratorCurrent  = 6
	iteratorHasCur   = 7
	iteratorMoveNext = 8
	kvpValue         = 7
	asyncInfoErrCode = 8

	hrAccessDenied        = 0x80070005
	hrNoCaptureDevices    = 0xC00DABE0
	scanDecodeInterval    = 200 * time.Millisecond
	scanCardW             = 520
	scanPreviewH          = 360
	scanCardPad           = 16
	scanCardRadius        = 8
	scanButtonW           = 96
	scanButtonH           = 32
	scanBandH             = scanButtonH + 2*scanCardPad
	mediaCaptureClass     = "Windows.Media.Capture.MediaCapture"
	mediaCaptureInitClass = "Windows.Media.Capture.MediaCaptureInitializationSettings"
)

type bitmapPlaneDesc struct {
	startIndex int32
	width      int32
	height     int32
	stride     int32
}

var (
	mfScanStartupOnce sync.Once
	mfScanStartupOK   bool

	// Test hooks for capability and permission checks.
	scanCameraAvailable = cameraAvailable
	scanPermissionState = permissionStatus
)

// cameraAvailable reports whether Media Foundation enumerates at least one
// video capture device and WinRT's MediaCapture class can be reached.
func cameraAvailable() bool {
	comApartment()
	if mfCreateAttributes.Find() != nil || mfEnumDeviceSources.Find() != nil {
		return false
	}
	mfScanStartupOnce.Do(func() {
		if mfStartup.Find() == nil {
			hr, _, _ := mfStartup.Call(mfVersion, mfStartupFull)
			mfScanStartupOK = !failed(hr)
		}
	})
	if !mfScanStartupOK {
		return false
	}
	var attrs *comObject
	if hr, _, _ := mfCreateAttributes.Call(uintptr(unsafe.Pointer(&attrs)), 1); failed(hr) || attrs == nil {
		return false
	}
	defer attrs.release()
	if failed(attrs.call(attributesSetGUID, uintptr(unsafe.Pointer(&mfDevSourceAttrSourceType)), uintptr(unsafe.Pointer(&mfDevSourceAttrVidCapGUID)))) {
		return false
	}
	var devices **comObject
	var count uint32
	hr, _, _ := mfEnumDeviceSources.Call(attrs.ptr(), uintptr(unsafe.Pointer(&devices)), uintptr(unsafe.Pointer(&count)))
	if failed(hr) {
		return false
	}
	if devices != nil {
		for _, dev := range unsafe.Slice(devices, count) {
			dev.release()
		}
		coTaskMemFree.Call(uintptr(unsafe.Pointer(devices)))
	}
	return count > 0
}

// scanHRESULTMessage converts a WinRT camera initialization HRESULT into a user-readable error string.
func scanHRESULTMessage(hr uint32) string {
	switch hr {
	case hrAccessDenied:
		return "camera access denied"
	case hrNoCaptureDevices:
		return "this device has no camera"
	}
	return "the camera could not be started"
}

// scanCancelLabel returns Windows' localized Cancel label, falling back to desktop/locales.
func scanCancelLabel(lang string) string {
	// MB_GetString(1) is IDCANCEL in the user's language.
	if mbGetString.Find() == nil {
		if r, _, _ := mbGetString.Call(1); r != 0 {
			p := *(**uint16)(unsafe.Pointer(&r))
			if s := syscall.UTF16ToString(unsafe.Slice(p, wcslen(p))); s != "" {
				return s
			}
		}
	}
	return locales.In(lang).Cancel()
}

// shownScanner is an active code scanner overlay on a window.
type shownScanner struct {
	w        *window
	id       uint32
	req      driver.ScanRequest
	done     atomic.Bool
	reading  atomic.Bool
	lastScan atomic.Int64 // UnixNano of last decode attempt

	// WinRT capture objects, owned and released by stopCapture.
	capMu        sync.Mutex
	capture      *comObject // IMediaCapture
	reader       *comObject // IMediaFrameReader
	arrivedToken eventToken

	// Latest BGRA8 preview frame written by frame callbacks and painted on the UI thread.
	frameMu   sync.Mutex
	framePix  []byte
	frameW    int
	frameH    int
	frameSeq  uint64
	bitmap    *comObject
	bitmapW   int
	bitmapH   int
	bitmapGen uint64
	bitmapSeq uint64

	// UI state on the UI thread.
	labelLayout *comObject
	card        rectF
	preview     rectF
	band        rectF
	cancelBtn   rectF
	hover       bool
	pressed     bool
	restore     *textInput
	ownWindow   bool
}

// scanCode starts a Windows camera scan for r and answers id with EventPick.
func (w *window) scanCode(r driver.ScanRequest, id uint32) {
	fail := func(why string) {
		w.engine.Dispatch(driver.Event{Kind: driver.EventPick, Count: id, Text: why})
	}
	if !codescan.HasReader(r) {
		fail("the camera reads none of the codes asked for")
		return
	}
	if !scanCameraAvailable() {
		fail("this device has no camera")
		return
	}
	if status, _ := scanPermissionState(driver.PermissionCamera); status == driver.PermissionDenied {
		fail("camera access denied")
		return
	}
	if w.scanner != nil {
		w.finishScanner(w.scanner, nil, "")
	}
	sc := &shownScanner{w: w, id: id, req: r}
	w.scanner = sc
	if in := w.focusedField(); in != nil {
		sc.restore = in
		w.blurAll()
	}
	if w.primary && w.opts.Hidden {
		const margin = 16
		w.placeNew(scanCardW+2*margin, scanPreviewH+scanBandH+2*margin, true)
		sc.ownWindow = true
		w.orderFront()
	}
	w.redrawAll()
	sc.startCapture()
}

// startCapture activates MediaCapture with CPU BGRA8 frame reading.
func (sc *shownScanner) startCapture() {
	comApartment()
	mc := activate(mediaCaptureClass, &iidMediaCapture)
	if mc == nil {
		sc.w.finishScanner(sc, nil, "MediaCapture is unavailable")
		return
	}
	settings := activate(mediaCaptureInitClass, &iidMediaCaptureInit)
	if settings == nil {
		closeAndRelease(mc)
		sc.w.finishScanner(sc, nil, "MediaCapture is unavailable")
		return
	}
	settings.call(mcInitSetStreamingMode, streamingCaptureModeVideo)
	if s5 := settings.query(&iidMediaCaptureInit5); s5 != nil {
		s5.call(mcInit5SetMemoryPref, memoryPreferenceCPU)
		s5.call(mcInit5SetSharingMode, sharingModeSharedReadOnly)
		s5.release()
	}
	var op *comObject
	hr := mc.call(mcInitWithSettings, settings.ptr(), uintptr(unsafe.Pointer(&op)))
	settings.release()
	if failed(hr) || op == nil {
		closeAndRelease(mc)
		sc.w.finishScanner(sc, nil, scanHRESULTMessage(uint32(hr)))
		return
	}
	sc.capMu.Lock()
	sc.capture = mc
	sc.capMu.Unlock()

	completed := newHandler(&iidAsyncActionCompleted, func(_ uintptr, status unsafe.Pointer) {
		var errHR uint32
		ok := uintptr(status) == asyncStatusCompleted
		if !ok {
			if info := op.query(&iidAsyncInfo); info != nil {
				info.call(asyncInfoErrCode, uintptr(unsafe.Pointer(&errHR)))
				info.release()
			}
		}
		op.release()
		sc.w.post(func() {
			if sc.done.Load() {
				return
			}
			if !ok {
				sc.w.finishScanner(sc, nil, scanHRESULTMessage(errHR))
				return
			}
			sc.openFrameReader()
		})
	})
	completed.agile = true
	hr = op.call(asyncPutCompleted, completed.ptr())
	completed.release()
	if failed(hr) {
		op.release()
		sc.w.finishScanner(sc, nil, "the camera could not be started")
	}
}

// openFrameReader selects the first color MediaFrameSource and creates a BGRA8 MediaFrameReader.
func (sc *shownScanner) openFrameReader() {
	sc.capMu.Lock()
	mc := sc.capture
	sc.capMu.Unlock()
	if mc == nil {
		return
	}
	mc5 := mc.query(&iidMediaCapture5)
	if mc5 == nil {
		sc.w.finishScanner(sc, nil, "camera frame capture is unavailable")
		return
	}
	defer mc5.release()

	var sources *comObject
	if failed(mc5.call(mc5FrameSources, uintptr(unsafe.Pointer(&sources)))) || sources == nil {
		sc.w.finishScanner(sc, nil, "this device has no camera")
		return
	}
	source := findColorFrameSource(sources)
	sources.release()
	if source == nil {
		sc.w.finishScanner(sc, nil, "this device has no camera")
		return
	}
	defer source.release()

	subtype := hstring("BGRA8")
	var op *comObject
	hr := mc5.call(mc5CreateFrameReaderSub, source.ptr(), subtype, uintptr(unsafe.Pointer(&op)))
	deleteHString(subtype)
	if failed(hr) || op == nil {
		sc.w.finishScanner(sc, nil, "camera frame capture could not start")
		return
	}
	completed := newHandler(&iidFrameReaderCompleted, func(_ uintptr, status unsafe.Pointer) {
		var reader *comObject
		ok := uintptr(status) == asyncStatusCompleted && !failed(op.call(asyncGetResults, uintptr(unsafe.Pointer(&reader)))) && reader != nil
		op.release()
		sc.w.post(func() {
			if sc.done.Load() {
				closeAndRelease(reader)
				return
			}
			if !ok {
				closeAndRelease(reader)
				sc.w.finishScanner(sc, nil, "camera frame capture could not start")
				return
			}
			sc.startFrameReader(reader)
		})
	})
	completed.agile = true
	hr = op.call(asyncPutCompleted, completed.ptr())
	completed.release()
	if failed(hr) {
		op.release()
		sc.w.finishScanner(sc, nil, "camera frame capture could not start")
	}
}

// findColorFrameSource iterates MediaCapture.FrameSources for the first Color camera source.
func findColorFrameSource(sources *comObject) *comObject {
	iterable := sources.query(&iidFrameSourceIterable)
	if iterable == nil {
		return nil
	}
	defer iterable.release()
	var iter *comObject
	if failed(iterable.call(iterableFirst, uintptr(unsafe.Pointer(&iter)))) || iter == nil {
		return nil
	}
	defer iter.release()

	var fallback *comObject
	for {
		var has byte
		if failed(iter.call(iteratorHasCur, uintptr(unsafe.Pointer(&has)))) || has == 0 {
			break
		}
		var pair *comObject
		if !failed(iter.call(iteratorCurrent, uintptr(unsafe.Pointer(&pair)))) && pair != nil {
			var src *comObject
			if !failed(pair.call(kvpValue, uintptr(unsafe.Pointer(&src)))) && src != nil {
				var info *comObject
				isColor := false
				if !failed(src.call(frameSourceInfo, uintptr(unsafe.Pointer(&info)))) && info != nil {
					var kind int32
					if !failed(info.call(frameSourceInfoKind, uintptr(unsafe.Pointer(&kind)))) && kind == frameSourceKindColor {
						isColor = true
					}
					info.release()
				}
				if isColor {
					fallback.release()
					pair.release()
					return src
				}
				if fallback == nil {
					fallback = src
				} else {
					src.release()
				}
			}
			pair.release()
		}
		var moved byte
		if failed(iter.call(iteratorMoveNext, uintptr(unsafe.Pointer(&moved)))) || moved == 0 {
			break
		}
	}
	return fallback
}

// startFrameReader hooks FrameArrived and calls StartAsync on reader.
func (sc *shownScanner) startFrameReader(reader *comObject) {
	handler := newHandler(&iidFrameArrivedHandler, func(_ uintptr, _ unsafe.Pointer) {
		sc.onFrameArrived()
	})
	handler.agile = true
	var token eventToken
	hr := reader.call(frameReaderAddArrived, handler.ptr(), uintptr(unsafe.Pointer(&token)))
	handler.release()
	if failed(hr) {
		closeAndRelease(reader)
		sc.w.finishScanner(sc, nil, "camera frame capture could not start")
		return
	}
	sc.capMu.Lock()
	sc.reader = reader
	sc.arrivedToken = token
	sc.capMu.Unlock()

	var op *comObject
	if failed(reader.call(frameReaderStartAsync, uintptr(unsafe.Pointer(&op)))) || op == nil {
		sc.w.finishScanner(sc, nil, "camera frame capture could not start")
		return
	}
	completed := newHandler(&iidFrameStartCompleted, func(_ uintptr, status unsafe.Pointer) {
		var startStatus int32 = -1
		ok := uintptr(status) == asyncStatusCompleted && !failed(op.call(asyncGetResults, uintptr(unsafe.Pointer(&startStatus)))) && startStatus == 0
		op.release()
		if !ok {
			sc.w.post(func() {
				if !sc.done.Load() {
					sc.w.finishScanner(sc, nil, "the camera is in use or unavailable")
				}
			})
		}
	})
	completed.agile = true
	hr = op.call(asyncPutCompleted, completed.ptr())
	completed.release()
	if failed(hr) {
		op.release()
		sc.w.finishScanner(sc, nil, "camera frame capture could not start")
	}
}

// onFrameArrived runs on a WinRT worker thread when a camera frame arrives.
func (sc *shownScanner) onFrameArrived() {
	if sc.done.Load() {
		return
	}
	sc.capMu.Lock()
	reader := sc.reader
	if reader != nil {
		reader.addRef()
	}
	sc.capMu.Unlock()
	if reader == nil {
		return
	}
	defer reader.release()

	var frameRef *comObject
	if failed(reader.call(frameReaderTryAcquire, uintptr(unsafe.Pointer(&frameRef)))) || frameRef == nil {
		return
	}
	defer closeAndRelease(frameRef)

	var videoFrame *comObject
	if failed(frameRef.call(frameRefVideoFrame, uintptr(unsafe.Pointer(&videoFrame)))) || videoFrame == nil {
		return
	}
	defer videoFrame.release()

	var bmp *comObject
	if failed(videoFrame.call(videoFrameSoftBitmap, uintptr(unsafe.Pointer(&bmp)))) || bmp == nil {
		return
	}
	defer closeAndRelease(bmp)

	var format, w, h int32
	if failed(bmp.call(softBitmapPixelFormat, uintptr(unsafe.Pointer(&format)))) || format != bitmapFormatBGRA8 {
		return
	}
	if failed(bmp.call(softBitmapPixelWidth, uintptr(unsafe.Pointer(&w)))) || failed(bmp.call(softBitmapPixelHeight, uintptr(unsafe.Pointer(&h)))) || w <= 0 || h <= 0 {
		return
	}

	var buf *comObject
	if failed(bmp.call(softBitmapLockBuffer, bitmapAccessRead, uintptr(unsafe.Pointer(&buf)))) || buf == nil {
		return
	}
	defer closeAndRelease(buf)

	var plane bitmapPlaneDesc
	if failed(buf.call(bitmapBufPlaneDesc, 0, uintptr(unsafe.Pointer(&plane)))) || plane.stride < w*4 {
		return
	}
	memBuf := buf.query(&iidMemoryBuffer)
	if memBuf == nil {
		return
	}
	defer memBuf.release()

	var memRef *comObject
	if failed(memBuf.call(memBufCreateRef, uintptr(unsafe.Pointer(&memRef)))) || memRef == nil {
		return
	}
	defer closeAndRelease(memRef)

	byteAccess := memRef.query(&iidMemoryBufferByteAccess)
	if byteAccess == nil {
		return
	}
	defer byteAccess.release()

	var dataPtr *byte
	var capacity uint32
	if failed(byteAccess.call(memBufByteAccessGet, uintptr(unsafe.Pointer(&dataPtr)), uintptr(unsafe.Pointer(&capacity)))) || dataPtr == nil {
		return
	}
	start := int(plane.startIndex)
	needed := start + int(h-1)*int(plane.stride) + int(w)*4
	if start < 0 || needed > int(capacity) {
		return
	}
	raw := unsafe.Slice(dataPtr, capacity)
	packed := make([]byte, int(w)*int(h)*4)
	for row := range int(h) {
		srcOff := start + row*int(plane.stride)
		copy(packed[row*int(w)*4:(row+1)*int(w)*4], raw[srcOff:srcOff+int(w)*4])
	}

	// Update preview buffer and ask UI thread to repaint.
	sc.frameMu.Lock()
	sc.framePix = packed
	sc.frameW, sc.frameH = int(w), int(h)
	sc.frameSeq++
	sc.frameMu.Unlock()
	sc.w.post(func() {
		if sc.w.scanner == sc && !sc.done.Load() {
			sc.w.redrawAll()
		}
	})

	// Decode at most every scanDecodeInterval, never stacking decodes.
	now := time.Now().UnixNano()
	if now-sc.lastScan.Load() < int64(scanDecodeInterval) || !sc.reading.CompareAndSwap(false, true) {
		return
	}
	sc.lastScan.Store(now)
	decodeBuf := append([]byte(nil), packed...)
	go func() {
		defer sc.reading.Store(false)
		code := codescan.DecodeBGRA(decodeBuf, int(w), int(h), int(w)*4, sc.req)
		if code == nil || sc.done.Load() {
			return
		}
		sc.w.post(func() {
			sc.w.finishScanner(sc, code, "")
		})
	}()
}

// stopCapture releases WinRT reader and capture objects safely.
func (sc *shownScanner) stopCapture() {
	sc.capMu.Lock()
	reader, token, mc := sc.reader, sc.arrivedToken, sc.capture
	sc.reader, sc.arrivedToken, sc.capture = nil, eventToken{}, nil
	sc.capMu.Unlock()
	if reader != nil {
		reader.call(frameReaderRemoveArrived, uintptr(token.value))
		var stopOp *comObject
		if !failed(reader.call(frameReaderStopAsync, uintptr(unsafe.Pointer(&stopOp)))) && stopOp != nil {
			stopOp.release()
		}
		closeAndRelease(reader)
	}
	closeAndRelease(mc)
}

func closeAndRelease(o *comObject) {
	if o == nil {
		return
	}
	if c := o.query(&iidClosable); c != nil {
		c.call(closableClose)
		c.release()
	}
	o.release()
}

// finishScanner ends sc once, releases resources, restores focus and dispatches EventPick.
func (w *window) finishScanner(sc *shownScanner, code *driver.ScannedCode, errText string) {
	if sc == nil || !sc.done.CompareAndSwap(false, true) {
		return
	}
	sc.stopCapture()
	releaseAll(&sc.bitmap, &sc.labelLayout)
	if w.scanner == sc {
		w.scanner = nil
	}
	if in := sc.restore; in != nil {
		sc.restore = nil
		if !in.v.deleted {
			w.setFocus(in)
		}
	}
	if sc.ownWindow && w.alert == nil {
		showWindow.Call(w.hwnd, swHide)
	}
	w.redrawAll()
	ev := driver.Event{Kind: driver.EventPick, Count: sc.id, Text: errText}
	if code != nil {
		ev.Data = code
	}
	if w.engine != nil {
		w.engine.Dispatch(ev)
	}
}

// endWindowScanner cancels any active scanner when w is destroyed.
func endWindowScanner(w *window) {
	if w.scanner != nil {
		w.finishScanner(w.scanner, nil, "")
	}
}

// layout places the scanner card, preview box, and Cancel button inside size.
func (sc *shownScanner) layout(size driver.Size) {
	width := float32(min(max(float64(size.Width)-32, 320), scanCardW))
	previewH := float32(min(max(float64(size.Height)-float64(scanBandH)-32, 180), scanPreviewH))
	totalH := previewH + scanBandH
	left := (float32(size.Width) - width) / 2
	top := max((float32(size.Height)-totalH)/2, 0)
	sc.card = rectF{left, top, left + width, top + totalH}
	sc.preview = rectF{left, top, left + width, top + previewH}
	sc.band = rectF{left, top + previewH, left + width, top + totalH}
	sc.cancelBtn = rectF{
		left + width - scanCardPad - scanButtonW,
		sc.band.top + scanCardPad,
		left + width - scanCardPad,
		sc.band.top + scanCardPad + scanButtonH,
	}
	if sc.labelLayout == nil {
		l, _ := alertText(scanCancelLabel(sc.w.language), driver.Font{Size: 14}, scanButtonW)
		if l != nil {
			l.call(formatSetTextAlignment, dwAlignCenter)
			l.call(formatSetParagraphAlignment, dwParagraphCenter)
			l.callF(layoutSetMaxWidth, 1, f32(scanButtonW))
			l.callF(layoutSetMaxHeight, 1, f32(scanButtonH))
		}
		sc.labelLayout = l
	}
}

// drawScanner paints the live camera preview card over the window when a scan is active.
func (p *painter) drawScanner(size driver.Size) {
	sc := p.w.scanner
	if sc == nil {
		return
	}
	sc.layout(size)
	dark := p.w.env.Dark
	text, band, button, border := driver.RGB(0x1a1a1a), driver.RGB(0xf3f3f3), driver.RGB(0xfbfbfb), driver.RGBA(0x000000, 0.08)
	if dark {
		text, band, button, border = driver.RGB(0xffffff), driver.RGB(0x202020), driver.RGB(0x373737), driver.RGBA(0xffffff, 0.08)
	}
	p.setTransform(identity)
	p.fillRect(rectF{0, 0, float32(size.Width), float32(size.Height)}, driver.RGBA(0x000000, 0.45))
	p.fillRounded(rectF{sc.card.left - 1, sc.card.top - 1, sc.card.right + 1, sc.card.bottom + 1}, scanCardRadius+1, border)
	p.fillRounded(sc.card, scanCardRadius, band)
	p.fillRect(sc.preview, driver.RGB(0x000000))

	if bmp := sc.previewBitmap(p.dc); bmp != nil {
		dst := fitAspect(sc.preview, float32(sc.bitmapW), float32(sc.bitmapH))
		p.dc.callF(rtDrawBitmap, 0b100, bmp.ptr(), uintptr(unsafe.Pointer(&dst)), f32(1), d2dInterpolationLinear, 0)
	}

	p.fillRect(rectF{sc.card.left, sc.band.top, sc.card.right, sc.band.top + 1}, border)
	fill := button
	switch {
	case sc.pressed && sc.hover:
		fill = fill&^0xff | fill&0xff*8/10
	case sc.hover:
		fill = fill&^0xff | fill&0xff*9/10
	}
	r := sc.cancelBtn
	p.fillRounded(rectF{r.left - 1, r.top - 1, r.right + 1, r.bottom + 1}, 5, border)
	p.fillRounded(r, 4, fill)
	if sc.labelLayout != nil {
		p.dc.callPoint(rtDrawTextLayout, r.left, r.top, sc.labelLayout.ptr(), p.solid(text).ptr(), d2dDrawTextEnableColorFont)
	}
}

// previewBitmap uploads the latest BGRA8 camera frame into a Direct2D bitmap.
func (sc *shownScanner) previewBitmap(dc *comObject) *comObject {
	sc.frameMu.Lock()
	pix, w, h, seq := sc.framePix, sc.frameW, sc.frameH, sc.frameSeq
	sc.frameMu.Unlock()
	if len(pix) == 0 || w <= 0 || h <= 0 {
		return nil
	}
	if sc.bitmap != nil && sc.bitmapGen == gfxGeneration && sc.bitmapW == w && sc.bitmapH == h {
		if sc.bitmapSeq != seq {
			if !failed(sc.bitmap.call(bitmapCopyFromMemory, 0, uintptr(unsafe.Pointer(&pix[0])), uintptr(w*4))) {
				sc.bitmapSeq = seq
			}
		}
		return sc.bitmap
	}
	releaseAll(&sc.bitmap)
	props := bitmapProperties1{format: dxgiFormatBGRA, alphaMode: d2dAlphaIgnore, dpiX: 96, dpiY: 96}
	if failed(dc.callSize(dcCreateBitmap, uint32(w), uint32(h), uintptr(unsafe.Pointer(&pix[0])),
		uintptr(w*4), uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&sc.bitmap)))) {
		sc.bitmap = nil
		return nil
	}
	sc.bitmapW, sc.bitmapH, sc.bitmapGen, sc.bitmapSeq = w, h, gfxGeneration, seq
	return sc.bitmap
}

// fitAspect centers a srcW×srcH picture inside box preserving aspect ratio.
func fitAspect(box rectF, srcW, srcH float32) rectF {
	bw, bh := box.right-box.left, box.bottom-box.top
	if srcW <= 0 || srcH <= 0 || bw <= 0 || bh <= 0 {
		return box
	}
	scale := min(bw/srcW, bh/srcH)
	dw, dh := srcW*scale, srcH*scale
	x := box.left + (bw-dw)/2
	y := box.top + (bh-dh)/2
	return rectF{x, y, x + dw, y + dh}
}

func (sc *shownScanner) hitCancel(p driver.Point) bool {
	r := sc.cancelBtn
	return float32(p.X) >= r.left && float32(p.X) < r.right && float32(p.Y) >= r.top && float32(p.Y) < r.bottom
}

// scannerMouse handles pointer input while a scanner overlay shows.
func (w *window) scannerMouse(message uint32, at driver.Point) bool {
	sc := w.scanner
	if sc == nil {
		return false
	}
	hover := sc.hitCancel(at)
	switch message {
	case wmLButtonDown:
		sc.pressed = hover
		setCapture.Call(w.hwnd)
	case wmLButtonUp:
		pressed := sc.pressed
		sc.pressed = false
		releaseCapture.Call()
		if pressed && hover {
			w.finishScanner(sc, nil, "")
			return true
		}
	}
	if hover != sc.hover || message != wmMouseMove {
		sc.hover = hover
		w.redrawAll()
	}
	return true
}

// scannerKey handles keyboard input while a scanner overlay shows.
func (w *window) scannerKey(vk uintptr) bool {
	sc := w.scanner
	if sc == nil {
		return false
	}
	switch vk {
	case vkEscape, vkReturn, vkSpace:
		w.finishScanner(sc, nil, "")
	}
	return true
}
