package android

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

const androidWebErrorDomain = "android.webkit.WebViewClient"

func encodeErrorEvent(surface uint32, id driver.NodeID, text string, index int32, nav *driver.WebNavigation, data string) []byte {
	var w writer
	w.u32(surface)
	w.byte(byte(driver.EventError))
	w.u32(uint32(id))
	w.u32(0) // count
	w.points(0)
	w.points(0)
	w.str(text)
	w.points(0)
	w.points(0)
	w.u32(uint32(index))
	w.byte(0) // phase
	w.points(0)
	w.points(0)
	w.float(0) // value
	if nav == nil {
		w.byte(0)
	} else {
		w.flags(true, nav.Loading, nav.CanGoBack, nav.CanGoForward)
		w.str(nav.URL)
		w.str(nav.Title)
	}
	w.u64(0) // time
	w.u64(0) // duration
	w.byte(0)
	w.str(data)
	w.byte(0) // finished
	w.points(0)
	w.points(0)
	w.points(0)
	w.points(0)
	w.byte(0) // mods
	return w.buf
}

// TestReadEventWebErrorFailure verifies that readEvent decodes Android web view failures into *driver.WebFailure (preserving signed negative WebViewClient.ERROR_* codes), leaves refused navigations without a failure domain/code, and keeps video error indices untouched.
func TestReadEventWebErrorFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		url  string
		msg  string
		code int
	}{
		{"ERROR_UNKNOWN", "https://unknown.invalid/", "net::ERR_FAILED", -1},
		{"ERROR_HOST_LOOKUP", "https://missing.invalid/", "net::ERR_NAME_NOT_RESOLVED", -2},
		{"ERROR_CONNECT", "https://closed.invalid/", "net::ERR_CONNECTION_REFUSED", -6},
		{"ERROR_TIMEOUT", "https://slow.invalid/", "net::ERR_TIMED_OUT", -8},
		{"ERROR_FAILED_SSL_HANDSHAKE", "https://tls.invalid/", "net::ERR_SSL_PROTOCOL_ERROR", -11},
		{"ERROR_FILE_NOT_FOUND", "file:///missing.html", "net::ERR_FILE_NOT_FOUND", -14},
		{"ERROR_UNSAFE_RESOURCE", "https://unsafe.invalid/", "net::ERR_BLOCKED_BY_CLIENT", -16},
	} {
		raw := encodeErrorEvent(0, 12, tc.msg, int32(tc.code), &driver.WebNavigation{URL: tc.url}, androidWebErrorDomain)
		r := reader{raw}
		from, ev, data := readEvent(&r)
		if len(r.buf) != 0 || from != 0 || data != "" {
			t.Fatalf("%s: from=%d data=%q remaining=%d", tc.name, from, data, len(r.buf))
		}
		if ev.Kind != driver.EventError || ev.ID != 12 || ev.Text != tc.msg || ev.Index != 0 {
			t.Fatalf("%s: unexpected event %+v", tc.name, ev)
		}
		if ev.Navigation == nil || ev.Navigation.URL != tc.url {
			t.Fatalf("%s: navigation = %+v, want URL %q", tc.name, ev.Navigation, tc.url)
		}
		failure, ok := ev.Data.(*driver.WebFailure)
		if !ok || failure == nil || failure.Domain != androidWebErrorDomain || failure.Code != tc.code {
			t.Errorf("%s: Data = %#v, want &WebFailure{Domain: %q, Code: %d}", tc.name, ev.Data, androidWebErrorDomain, tc.code)
		}
	}

	// Refused navigation leaves Domain and Code empty (ev.Data == nil).
	refusedURL := "https://evil.invalid/"
	refusedMsg := driver.RefusedNavigation(refusedURL)
	r := reader{encodeErrorEvent(0, 12, refusedMsg, 0, &driver.WebNavigation{URL: refusedURL}, "")}
	_, ev, data := readEvent(&r)
	if len(r.buf) != 0 || data != "" || ev.Data != nil || ev.Index != 0 || ev.Text != refusedMsg ||
		ev.Navigation == nil || ev.Navigation.URL != refusedURL {
		t.Errorf("refused navigation: ev=%+v data=%q left=%d", ev, data, len(r.buf))
	}

	// Non-web error (video alpha unsupported) preserves ev.Index and leaves ev.Data nil.
	r = reader{encodeErrorEvent(0, 34, "alpha unsupported", driver.VideoErrorAlphaUnsupported, nil, "")}
	_, ev, data = readEvent(&r)
	if len(r.buf) != 0 || data != "" || ev.Data != nil || ev.Index != driver.VideoErrorAlphaUnsupported {
		t.Errorf("video error: ev=%+v data=%q left=%d", ev, data, len(r.buf))
	}

	// Verify Java runtime constants and NsgoView wiring match Go's decoder.
	testhost.RequireSource(t)
	webNode, err := os.ReadFile(filepath.Join("java", "dev", "oneo", "nsgo", "WebNode.java"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(webNode), `static final String ERROR_DOMAIN = "`+androidWebErrorDomain+`";`) {
		t.Errorf("WebNode.java missing ERROR_DOMAIN = %q", androidWebErrorDomain)
	}
	nsgoView, err := os.ReadFile(filepath.Join("java", "dev", "oneo", "nsgo", "NsgoView.java"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(nsgoView), "event(Protocol.EVENT_ERROR, n.id, 0, 0, 0, text, 0, 0, code, 0, 0, 0, 0, nav,") {
		t.Error("NsgoView.java does not pass error code in index for WebNode.Reporter.error")
	}
}

// TestWebNodeErrorsJava runs WebNode.java on the JVM against Android stubs, exercising onReceivedError and shouldOverrideUrlLoading, and decodes the emitted wire frames with Go's readEvent.
func TestWebNodeErrorsJava(t *testing.T) {
	out := runJavaAlgorithm(t, "weberror", "WebErrorCheck", nil, "WebNode", "WebMedia", "Protocol")
	r := reader{out}
	count := int(r.u32())

	want := []struct {
		url     string
		text    string
		failure *driver.WebFailure
	}{
		{"https://missing.invalid/", "net::ERR_NAME_NOT_RESOLVED", &driver.WebFailure{Domain: androidWebErrorDomain, Code: -2}},
		{"https://closed.invalid:8443/", "net::ERR_CONNECTION_REFUSED", &driver.WebFailure{Domain: androidWebErrorDomain, Code: -6}},
		{"https://slow.invalid/", "net::ERR_TIMED_OUT", &driver.WebFailure{Domain: androidWebErrorDomain, Code: -8}},
		{"https://tls.invalid/", "net::ERR_SSL_PROTOCOL_ERROR", &driver.WebFailure{Domain: androidWebErrorDomain, Code: -11}},
		{"file:///nonexistent.html", "net::ERR_FILE_NOT_FOUND", &driver.WebFailure{Domain: androidWebErrorDomain, Code: -14}},
		{"https://unsafe.invalid/", "net::ERR_BLOCKED_BY_CLIENT", &driver.WebFailure{Domain: androidWebErrorDomain, Code: -16}},
		{"https://empty-desc.invalid/", "", &driver.WebFailure{Domain: androidWebErrorDomain, Code: -1}},
		{"https://evil.invalid/page", driver.RefusedNavigation("https://evil.invalid/page"), nil},
	}

	if count != len(want) {
		t.Fatalf("WebNode emitted %d error events, want %d", count, len(want))
	}
	for i, w := range want {
		frame := r.bytes()
		fr := reader{frame}
		from, ev, data := readEvent(&fr)
		if len(fr.buf) != 0 || from != 0 || data != "" {
			t.Fatalf("event %d: from=%d data=%q left=%d", i, from, data, len(fr.buf))
		}
		if ev.Kind != driver.EventError || ev.ID != 77 || ev.Index != 0 {
			t.Errorf("event %d: kind=%v id=%d index=%d", i, ev.Kind, ev.ID, ev.Index)
		}
		if ev.Navigation == nil || ev.Navigation.URL != w.url {
			t.Errorf("event %d: URL=%+v, want %q", i, ev.Navigation, w.url)
		}
		if ev.Text != w.text {
			t.Errorf("event %d: Text=%q, want %q", i, ev.Text, w.text)
		}
		if w.failure == nil {
			if ev.Data != nil {
				t.Errorf("event %d: Data=%#v, want nil", i, ev.Data)
			}
			continue
		}
		got, ok := ev.Data.(*driver.WebFailure)
		if !ok || got == nil || *got != *w.failure {
			t.Errorf("event %d: Data=%#v, want %#v", i, ev.Data, w.failure)
		}
	}
	if len(r.buf) != 0 {
		t.Errorf("Java wrote %d unexpected trailing bytes", len(r.buf))
	}
}
