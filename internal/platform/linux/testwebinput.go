//go:build linux && !android

package linux

import (
	"strings"
	"unsafe"

	"github.com/ebitengine/purego"
)

// The X11 translator retains the provenance GTK's GdkEvent omits: the XI2 source device and the request serial. Match only NSGo's own XTest requests, then retain their GDK events so every controller admits the same event. All state belongs to the GTK thread; ordinary apps never install this hook.
var testWebInput struct {
	display      uintptr
	nextRequest  func(uintptr) uintptr
	translate    func(uintptr, *xInputCookie, uintptr) int32
	sources      map[int32]bool
	requests     [128]uintptr
	requestNext  int
	events       []testWebEvent
	approved     [64]uintptr
	approvedNext int
	ref          func(uintptr) uintptr
	unref        func(uintptr)
	surface      func(uintptr) uintptr
	keycode      func(uintptr) uint32
}

type xInputCookie struct {
	kind             int32
	serial           uintptr
	send             int32
	display          uintptr
	extension, event int32
	cookie           uint32
	data             unsafe.Pointer
}

type xInputDeviceEvent struct {
	kind                   int32
	serial                 uintptr
	send                   int32
	display                uintptr
	extension, event       int32
	time                   uintptr
	device, source, detail int32
	root, window, child    uintptr
}

type xiDeviceInfo struct {
	id                                int32
	name                              uintptr
	use, attachment, enabled, classes int32
	classList                         uintptr
}

type testWebEvent struct {
	kind         int32
	time, detail uint32
	window       uintptr
	trusted      bool
}

func prepareTestWebInput(display uintptr) bool {
	if testWebInput.display != 0 {
		return testWebInput.display == display
	}
	xi, err := purego.Dlopen("libXi.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return false
	}
	var query func(uintptr, int32, *int32) *xiDeviceInfo
	var free func(*xiDeviceInfo)
	purego.RegisterLibFunc(&query, xi, "XIQueryDevice")
	purego.RegisterLibFunc(&free, xi, "XIFreeDeviceInfo")
	var count int32
	devices := query(display, 0, &count)
	testWebInput.sources = map[int32]bool{}
	if devices != nil {
		for _, device := range unsafe.Slice(devices, count) {
			name := goString(device.name)
			if strings.HasSuffix(name, " XTEST pointer") || strings.HasSuffix(name, " XTEST keyboard") {
				testWebInput.sources[device.id] = true
			}
		}
		free(devices)
	}
	if len(testWebInput.sources) == 0 {
		return false
	}
	x11, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return false
	}
	var extension func(uintptr, string, *int32, *int32, *int32) int32
	var setTranslator func(uintptr, int32, uintptr) uintptr
	purego.RegisterLibFunc(&extension, x11, "XQueryExtension")
	purego.RegisterLibFunc(&setTranslator, x11, "XESetWireToEventCookie")
	purego.RegisterLibFunc(&testWebInput.nextRequest, x11, "XNextRequest")
	purego.RegisterLibFunc(&testWebInput.ref, gtkLib, "gdk_event_ref")
	purego.RegisterLibFunc(&testWebInput.unref, gtkLib, "gdk_event_unref")
	purego.RegisterLibFunc(&testWebInput.surface, gtkLib, "gdk_event_get_surface")
	purego.RegisterLibFunc(&testWebInput.keycode, gtkLib, "gdk_key_event_get_keycode")
	var opcode, firstEvent, firstError int32
	if extension(display, "XInputExtension", &opcode, &firstEvent, &firstError) == 0 {
		return false
	}
	callback := purego.NewCallback(func(d uintptr, cookie *xInputCookie, wire uintptr) int32 {
		result := testWebInput.translate(d, cookie, wire)
		if result == 0 || cookie.data == nil || cookie.event < 2 || cookie.event > 6 {
			return result
		}
		e := (*xInputDeviceEvent)(cookie.data)
		rememberTestWebInput(*e)
		return result
	})
	previous := setTranslator(display, opcode, callback)
	if previous == 0 {
		setTranslator(display, opcode, previous)
		return false
	}
	purego.RegisterFunc(&testWebInput.translate, previous)
	testWebInput.display = display
	return true
}

func recordTestWebInput(display uintptr) {
	if testWebInput.display != display {
		return
	}
	i := testWebInput.requestNext % len(testWebInput.requests)
	testWebInput.requests[i] = testWebInput.nextRequest(display)
	testWebInput.requestNext++
}

func isTestWebInputEvent(event uintptr) bool {
	if event == 0 || testWebInput.display == 0 {
		return false
	}
	for _, approved := range testWebInput.approved {
		if event == approved {
			return true
		}
	}
	kind := gdkEventGetEventType(event)
	var detail uint32
	switch kind {
	case 2, 3:
		detail = gdkButtonEventGetButton(event)
	case 4, 5:
		detail = testWebInput.keycode(event)
	case 1:
	default:
		return false
	}
	tm := gdkEventGetTime(event)
	surface := testWebInput.surface(event)
	if surface == 0 {
		return false
	}
	window := gdkX11SurfaceGetXID(surface)
	// Wire events are queued in dispatch order. Include rejected physical events so one with the same millisecond and detail cannot borrow the approval of the following scripted event.
	for n, e := range testWebInput.events {
		if e.kind != kind || e.time != tm || e.detail != detail || e.window != window {
			continue
		}
		testWebInput.events = testWebInput.events[n+1:]
		if len(testWebInput.events) == 0 {
			testWebInput.events = nil
		}
		if !e.trusted {
			return false
		}
		i := testWebInput.approvedNext % len(testWebInput.approved)
		if old := testWebInput.approved[i]; old != 0 {
			testWebInput.unref(old)
		}
		testWebInput.approved[i] = testWebInput.ref(event)
		testWebInput.approvedNext++
		return true
	}
	return false
}

func rememberTestWebInput(e xInputDeviceEvent) {
	kind := int32(0)
	switch e.event {
	case 2:
		kind = 4
	case 3:
		kind = 5
	case 4:
		kind = 2
	case 5:
		kind = 3
	case 6:
		kind = 1
	}
	trusted := false
	if testWebInput.sources[e.source] {
		for _, serial := range testWebInput.requests {
			if serial != 0 && serial == e.serial {
				trusted = true
				break
			}
		}
	}
	detail := uint32(e.detail)
	if kind == 1 {
		detail = 0
	}
	testWebInput.events = append(testWebInput.events, testWebEvent{kind: kind, time: uint32(e.time), detail: detail, window: e.window, trusted: trusted})
}
