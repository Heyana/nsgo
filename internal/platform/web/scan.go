//go:build js && wasm

package web

import (
	"slices"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/desktop/locales"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Scanning a code on the web: navigator.mediaDevices.getUserMedia feeds a
// live <video> inside a full-screen dismissible modal overlay while
// BarcodeDetector reads the requested code formats at a steady interval.

// barcodeFormatNames maps each driver.CodeKind to the W3C Shape Detection
// BarcodeDetector format names that read it. UPC-A is normalized to EAN-13
// with a leading 0, matching driver.CodeEAN13.
var barcodeFormatNames = map[driver.CodeKind][]string{
	driver.CodeQR:         {"qr_code"},
	driver.CodeAztec:      {"aztec"},
	driver.CodeDataMatrix: {"data_matrix"},
	driver.CodePDF417:     {"pdf417"},
	driver.CodeEAN13:      {"ean_13", "upc_a"},
	driver.CodeEAN8:       {"ean_8"},
	driver.CodeUPCE:       {"upc_e"},
	driver.CodeCode128:    {"code_128"},
	driver.CodeCode39:     {"code_39"},
	driver.CodeCode93:     {"code_93"},
	driver.CodeITF:        {"itf"},
}

// barcodeFormats returns the BarcodeDetector format names requested by r.
func barcodeFormats(r driver.ScanRequest) []string {
	var out []string
	for k := driver.CodeQR; k <= driver.CodeITF; k++ {
		if r.Wants(k) {
			out = append(out, barcodeFormatNames[k]...)
		}
	}
	return out
}

// filterBarcodeFormats keeps only the formats from requested that appear in
// supported, preserving order.
func filterBarcodeFormats(requested, supported []string) []string {
	var out []string
	for _, f := range requested {
		if slices.Contains(supported, f) {
			out = append(out, f)
		}
	}
	return out
}

// barcodeFromDetected maps a BarcodeDetector result (format, rawValue) to a
// driver.ScannedCode if it matches r, or returns nil.
func barcodeFromDetected(format, rawValue string, r driver.ScanRequest) *driver.ScannedCode {
	if rawValue == "" {
		return nil
	}
	var kind driver.CodeKind
	text := rawValue
	switch format {
	case "qr_code":
		kind = driver.CodeQR
	case "aztec":
		kind = driver.CodeAztec
	case "data_matrix":
		kind = driver.CodeDataMatrix
	case "pdf417":
		kind = driver.CodePDF417
	case "ean_13":
		kind = driver.CodeEAN13
	case "upc_a":
		kind = driver.CodeEAN13
		if len(text) == 12 {
			text = "0" + text
		}
	case "ean_8":
		kind = driver.CodeEAN8
	case "upc_e":
		kind = driver.CodeUPCE
	case "code_128":
		kind = driver.CodeCode128
	case "code_39":
		kind = driver.CodeCode39
	case "code_93":
		kind = driver.CodeCode93
	case "itf":
		kind = driver.CodeITF
	default:
		return nil
	}
	if !r.Wants(kind) {
		return nil
	}
	return &driver.ScannedCode{Kind: kind, Text: text}
}

// scanErrorMessage turns a getUserMedia rejection into a clear failure message.
func scanErrorMessage(name, fallback string) string {
	switch name {
	case "NotAllowedError", "PermissionDeniedError":
		return "camera access denied"
	case "NotFoundError", "DevicesNotFoundError":
		return "this device has no camera"
	}
	if fallback != "" {
		return fallback
	}
	return "camera failed to start"
}

// scanCodeAvailable reports whether the browser exposes both
// navigator.mediaDevices.getUserMedia and BarcodeDetector.
func scanCodeAvailable() bool {
	g := js.Global()
	if !g.Truthy() {
		return false
	}
	nav := g.Get("navigator")
	if !nav.Truthy() {
		return false
	}
	md := nav.Get("mediaDevices")
	if !md.Truthy() || md.Get("getUserMedia").Type() != js.TypeFunction {
		return false
	}
	return g.Get("BarcodeDetector").Type() == js.TypeFunction
}

// codeScanner holds the active modal camera overlay and BarcodeDetector loop.
type codeScanner struct {
	id         uint32
	req        driver.ScanRequest
	done       bool
	inFlight   bool
	layer      js.Value
	video      js.Value
	stream     js.Value
	detector   js.Value
	restore    js.Value
	intervalID js.Value
	tickFn     js.Func
	listeners  listeners
}

// scanCode starts a browser camera scan for r and answers id with EventPick.
func (s *surface) scanCode(r driver.ScanRequest, id uint32) {
	if s.scanner != nil {
		s.closeScanner(s.scanner, nil, "")
	}
	if !scanCodeAvailable() {
		s.engine.Dispatch(driver.Event{Kind: driver.EventPick, Count: id, Text: "this browser cannot scan codes"})
		return
	}
	sc := &codeScanner{id: id, req: r, restore: s.doc.Get("activeElement")}
	s.scanner = sc

	requested := barcodeFormats(r)
	if len(requested) == 0 {
		s.closeScanner(sc, nil, "the camera reads none of the codes asked for")
		return
	}
	bdCtor := js.Global().Get("BarcodeDetector")
	if getSupported := bdCtor.Get("getSupportedFormats"); getSupported.Type() == js.TypeFunction {
		then(bdCtor.Call("getSupportedFormats"), func(list js.Value) {
			if sc.done {
				return
			}
			supported := make([]string, 0, list.Length())
			for i := range list.Length() {
				if v := list.Index(i); v.Type() == js.TypeString {
					supported = append(supported, v.String())
				}
			}
			matched := filterBarcodeFormats(requested, supported)
			if len(matched) == 0 {
				s.closeScanner(sc, nil, "the camera reads none of the codes asked for")
				return
			}
			s.startWebScanner(sc, matched)
		}, func(e js.Value) {
			if !sc.done {
				s.closeScanner(sc, nil, jsError(e).Error())
			}
		})
		return
	}
	s.startWebScanner(sc, requested)
}

// startWebScanner requests camera access and opens the overlay for sc.
func (s *surface) startWebScanner(sc *codeScanner, formats []string) {
	jsFormats := make([]any, len(formats))
	for i, f := range formats {
		jsFormats[i] = f
	}
	detector := js.Global().Get("BarcodeDetector").New(map[string]any{"formats": jsFormats})
	sc.detector = detector

	constraints := map[string]any{
		"video": map[string]any{
			"facingMode": map[string]any{"ideal": "environment"},
		},
		"audio": false,
	}
	md := js.Global().Get("navigator").Get("mediaDevices")
	then(md.Call("getUserMedia", constraints), func(stream js.Value) {
		if sc.done {
			stopMediaStream(stream)
			return
		}
		sc.stream = stream
		s.showScannerOverlay(sc)
	}, func(e js.Value) {
		if !sc.done {
			s.closeScanner(sc, nil, scanErrorMessage(errorName(e), jsError(e).Error()))
		}
	})
}

// showScannerOverlay mounts the full-screen video overlay and begins detection.
func (s *surface) showScannerOverlay(sc *codeScanner) {
	add := func(tag string, parent js.Value, style map[string]string) js.Value {
		el := s.doc.Call("createElement", tag)
		setStyle(el, style)
		parent.Call("appendChild", el)
		return el
	}
	sc.layer = add("div", s.doc.Get("body"), map[string]string{
		"position": "fixed", "inset": "0", "z-index": "2147483647",
		"background": "#000000", "display": "flex", "flex-direction": "column",
		"font-family": fontFamily, "color": "#ffffff",
	})
	if s.rtl {
		sc.layer.Call("setAttribute", "dir", "rtl")
	}
	sc.layer.Call("setAttribute", "role", "dialog")
	sc.layer.Call("setAttribute", "aria-modal", "true")

	sc.video = add("video", sc.layer, map[string]string{
		"position": "absolute", "inset": "0", "width": "100%", "height": "100%",
		"object-fit": "cover", "background": "#000000",
	})
	sc.video.Set("playsInline", true)
	sc.video.Set("muted", true)
	sc.video.Set("autoplay", true)
	sc.video.Set("srcObject", sc.stream)

	bar := add("div", sc.layer, map[string]string{
		"position": "relative", "z-index": "1", "display": "flex",
		"align-items": "center", "padding": "16px 20px",
		"background": "linear-gradient(to bottom, rgba(0,0,0,0.65), rgba(0,0,0,0))",
	})
	cancelBtn := add("button", bar, map[string]string{
		"font": "inherit", "font-size": "15px", "font-weight": "500",
		"padding": "8px 16px", "border-radius": "8px",
		"border":     "1px solid rgba(255,255,255,0.28)",
		"background": "rgba(28,28,30,0.75)", "color": "#ffffff",
	})
	cancelBtn.Set("type", "button")
	cancelBtn.Set("textContent", locales.In(s.env.Locale).Cancel())
	sc.listeners.on(cancelBtn, "click", func(js.Value) { s.closeScanner(sc, nil, "") })
	sc.listeners.on(sc.layer, "keydown", func(e js.Value) {
		if e.Get("key").String() == "Escape" {
			e.Call("preventDefault")
			s.closeScanner(sc, nil, "")
		}
	})
	cancelBtn.Call("focus")

	// Start playback; browsers allow muted autoplay with a live MediaStream.
	if play := sc.video.Call("play"); play.Truthy() && play.Get("catch").Type() == js.TypeFunction {
		then(play, func(js.Value) {}, func(js.Value) {})
	}

	sc.tickFn = js.FuncOf(func(js.Value, []js.Value) any {
		if sc.done || sc.inFlight {
			return nil
		}
		if sc.video.Get("readyState").Int() < 2 {
			return nil
		}
		sc.inFlight = true
		then(sc.detector.Call("detect", sc.video), func(barcodes js.Value) {
			sc.inFlight = false
			if sc.done || !barcodes.Truthy() {
				return
			}
			for i := range barcodes.Length() {
				b := barcodes.Index(i)
				format := b.Get("format").String()
				raw := b.Get("rawValue").String()
				if code := barcodeFromDetected(format, raw, sc.req); code != nil {
					s.closeScanner(sc, code, "")
					return
				}
			}
		}, func(js.Value) {
			sc.inFlight = false
		})
		return nil
	})
	sc.intervalID = js.Global().Call("setInterval", sc.tickFn, 150)
}

// closeScanner tears down sc, stops the camera tracks, and answers its request once.
func (s *surface) closeScanner(sc *codeScanner, code *driver.ScannedCode, errText string) {
	if sc == nil || sc.done {
		return
	}
	sc.done = true
	if s.scanner == sc {
		s.scanner = nil
	}
	if sc.intervalID.Truthy() {
		js.Global().Call("clearInterval", sc.intervalID)
		sc.intervalID = js.Undefined()
	}
	if sc.tickFn.Value.Truthy() {
		sc.tickFn.Release()
	}
	stopMediaStream(sc.stream)
	sc.stream = js.Undefined()
	if sc.video.Truthy() {
		sc.video.Set("srcObject", js.Null())
	}
	sc.listeners.release()
	if sc.layer.Truthy() {
		sc.layer.Call("remove")
	}
	if r := sc.restore; r.Truthy() && r.Get("isConnected").Bool() {
		r.Call("focus", map[string]any{"preventScroll": true})
	}
	ev := driver.Event{Kind: driver.EventPick, Count: sc.id, Text: errText}
	if code != nil {
		ev.Data = code
	}
	s.engine.Dispatch(ev)
}

func stopMediaStream(stream js.Value) {
	if !stream.Truthy() || stream.Get("getTracks").Type() != js.TypeFunction {
		return
	}
	tracks := stream.Call("getTracks")
	for i := range tracks.Length() {
		if t := tracks.Index(i); t.Truthy() && t.Get("stop").Type() == js.TypeFunction {
			t.Call("stop")
		}
	}
}
