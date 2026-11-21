//go:build linux && !android

package linux

import (
	"net/http"
	"runtime"
	"strings"
	"sync"

	"github.com/ebitengine/purego"
	"golang.org/x/net/http/httpguts"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var videoHTTP struct {
	setup, filterEvent uintptr
	sourceURI          func(uintptr) uintptr
	newHeaders         func(*byte) uintptr
	freeHeaders        func(uintptr)
	setHeader          func(uintptr, *byte, uintptr, *byte, uintptr)
	typeName           func(uintptr) uintptr
	findProperty       func(uintptr, *byte) uintptr
	resourceError      func() uint32
	messageFull        func(uintptr, uint32, uint32, int32, uintptr, uintptr, *byte, *byte, int32)
	dup                func(*byte) uintptr
	sourcePad          func(uintptr, *byte) uintptr
	addProbe           func(uintptr, uint32, uintptr, uintptr, uintptr) uint64
	probeEvent         func(uintptr) uintptr
	eventHasName       func(uintptr, *byte) int32
}

// source-setup may run on a streaming thread. Its data is immutable and
// indexed by an integer handle owned by the GTK thread.
var videoHTTPRequests sync.Map

type videoHTTPRequest struct {
	uri     string
	headers *driver.VideoHeaders
}

func bindVideoHTTP(gst uintptr) {
	for name, fn := range map[string]any{
		"gst_uri_handler_get_uri":      &videoHTTP.sourceURI,
		"gst_structure_new_empty":      &videoHTTP.newHeaders,
		"gst_structure_free":           &videoHTTP.freeHeaders,
		"gst_structure_set":            &videoHTTP.setHeader,
		"gst_resource_error_quark":     &videoHTTP.resourceError,
		"gst_element_message_full":     &videoHTTP.messageFull,
		"gst_element_get_static_pad":   &videoHTTP.sourcePad,
		"gst_pad_add_probe":            &videoHTTP.addProbe,
		"gst_pad_probe_info_get_event": &videoHTTP.probeEvent,
		"gst_event_has_name":           &videoHTTP.eventHasName,
	} {
		purego.RegisterLibFunc(fn, gst, name)
	}
	purego.RegisterLibFunc(&videoHTTP.typeName, gobjectLib, "g_type_name_from_instance")
	purego.RegisterLibFunc(&videoHTTP.findProperty, gobjectLib, "g_object_class_find_property")
	purego.RegisterLibFunc(&videoHTTP.dup, glibLib, "g_strdup")
	videoHTTP.setup = newCallback(videoSourceSetup)
	videoHTTP.filterEvent = newCallback(videoFilterHTTPEvent)
}

func videoSourceSetup(pipeline, source, key uintptr) {
	value, ok := videoHTTPRequests.Load(key)
	if !ok {
		return
	}
	request := value.(videoHTTPRequest)
	location := videoHTTP.sourceURI(source)
	uri := goString(location)
	gFree(location)
	if uri != request.uri {
		return
	}
	if goString(videoHTTP.typeName(source)) != "GstSoupHTTPSrc" {
		const errorMessage = 1 << 1 // GST_MESSAGE_ERROR
		const settingsError = 13    // GST_RESOURCE_ERROR_SETTINGS
		msg := videoHTTP.dup(cstr("Video.Headers requires GStreamer's souphttpsrc HTTP source."))
		videoHTTP.messageFull(pipeline, errorMessage, videoHTTP.resourceError(), settingsError, msg, 0, cstr("videohttp.go"), cstr("videoSourceSetup"), 0)
		return
	}

	fields := videoHTTP.newHeaders(cstr("headers"))
	defer videoHTTP.freeHeaders(fields)
	var userAgent bool
	for name, value := range request.headers.All {
		if !httpguts.ValidHeaderFieldName(name) || !httpguts.ValidHeaderFieldValue(value) {
			continue
		}
		switch name = http.CanonicalHeaderKey(name); name {
		case "Range", "Accept-Encoding":
			// GStreamer controls byte ranges and must receive the original bytes.
		case "Cookie":
			// souphttpsrc disables its cookie jar when given explicit cookies.
			cookies := stringArray([]string{value})
			videoGST.setObject(source, cstr("cookies"), cookies.array(), uintptr(0))
			runtime.KeepAlive(cookies)
		case "User-Agent":
			userAgent = true
			videoGST.setObject(source, cstr("user-agent"), cstr(value), uintptr(0))
		default:
			const gTypeString = 16 << 2
			videoHTTP.setHeader(fields, cstr(name), gTypeString, cstr(value), 0)
		}
	}
	// Old souphttpsrc versions forward extra headers across origins. Even
	// versions with location-trusted do not strip the user-agent property.
	class := *(*uintptr)(cPointer(source))
	if userAgent || videoHTTP.findProperty(class, cstr("location-trusted")) == 0 {
		videoGST.setObject(source, cstr("automatic-redirect"), int32(0), uintptr(0))
	} else {
		videoGST.setObject(source, cstr("location-trusted"), int32(0), uintptr(0))
	}
	videoGST.setObject(source, cstr("extra-headers"), fields, uintptr(0))

	// Adaptive demuxers use souphttpsrc's HTTP event to copy request headers
	// to their own segment downloads. Limit the app's headers to this source.
	const downstreamEvent = 1 << 6 // GST_PAD_PROBE_TYPE_EVENT_DOWNSTREAM
	pad := videoHTTP.sourcePad(source, cstr("src"))
	videoHTTP.addProbe(pad, downstreamEvent, videoHTTP.filterEvent, 0, 0)
	gObjectUnref(pad)
}

func videoFilterHTTPEvent(_, info, _ uintptr) int32 {
	const drop, pass = 0, 1 // GST_PAD_PROBE_DROP, GST_PAD_PROBE_OK
	if event := videoHTTP.probeEvent(info); event != 0 && videoHTTP.eventHasName(event, cstr("http-headers")) != 0 {
		return drop
	}
	return pass
}

func videoHTTPSource(source string) bool {
	scheme, _, found := strings.Cut(source, "://")
	return found && (strings.EqualFold(scheme, "http") || strings.EqualFold(scheme, "https"))
}
