package android

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestAuthSessionJavaStateAndWire(t *testing.T) {
	var w writer
	w.byte(byte(driver.AuthRedirect))
	w.byte(byte(driver.AuthEnded))
	writeAuthRequest(&w, 0x80000010, &driver.AuthRequest{Command: driver.AuthStart, Scheme: "dev.nsgo.fixture", Path: "/old/../回调", Ephemeral: true})
	writeAuthRequest(&w, 0x80000010, &driver.AuthRequest{Command: driver.AuthOpen, URL: "https://provider.invalid/authorize?state=owned"})
	writeAuthRequest(&w, 16, &driver.AuthRequest{Command: driver.AuthStart, Scheme: "dev.nsgo.fixture", Path: "/oauth"})
	writeAuthRequest(&w, 16, nil)
	if out := runJavaAlgorithm(t, "auth", "AuthSessionCheck", w.buf, "Protocol", "AuthSession"); strings.TrimSpace(string(out)) != "ok" {
		t.Fatalf("Java authentication regression: %s", out)
	}
}

func TestAuthSessionReportsDecode(t *testing.T) {
	for _, tc := range []struct {
		name string
		ev   driver.Event
		data string
		want driver.AuthEvent
	}{
		{"redirect", driver.Event{Index: int(driver.AuthRedirect), Text: "dev.app:/callback"}, "", driver.AuthEvent{Phase: driver.AuthRedirect, Redirect: "dev.app:/callback"}},
		{"callback", driver.Event{Index: int(driver.AuthEnded)}, "dev.app:/callback?code=a%2Bb&state=owned#fragment", driver.AuthEvent{Phase: driver.AuthEnded, Callback: "dev.app:/callback?code=a%2Bb&state=owned#fragment"}},
		{"cancelled", driver.Event{Index: int(driver.AuthEnded), Text: driver.AuthCancelled}, "", driver.AuthEvent{Phase: driver.AuthEnded, Error: driver.AuthCancelled}},
		{"unavailable", driver.Event{Index: int(driver.AuthEnded), Text: driver.AuthUnavailable}, "no browser", driver.AuthEvent{Phase: driver.AuthEnded, Error: driver.AuthUnavailable, Message: "no browser"}},
		{"negative phase", driver.Event{Index: -1}, "", driver.AuthEvent{Phase: driver.AuthEnded, Error: driver.AuthFailed, Message: "invalid Android authentication response"}},
		{"overflow phase", driver.Event{Index: 256}, "", driver.AuthEvent{Phase: driver.AuthEnded, Error: driver.AuthFailed, Message: "invalid Android authentication response"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := readAuthSession(tc.ev, tc.data); *got != tc.want {
				t.Fatalf("response %+v, want %+v", *got, tc.want)
			}
		})
	}
}

// TestAuthSessionCapabilityGate keeps ItemAuthSession away from a runtime built before AuthSession.java: mount items carry no length, so such a runtime would misread the rest of the batch. Go answers its starts unavailable instead, and a runtime that sets capAuthSession in the fourth capability byte gets the batch unchanged.
func TestAuthSessionCapabilityGate(t *testing.T) {
	batch := &driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemAuthSession, Count: 3, Auth: &driver.AuthRequest{Command: driver.AuthStart, Scheme: "dev.app", Path: "/cb"}},
		{Kind: driver.ItemKeepAwake},
		{Kind: driver.ItemAuthSession, Count: 3, Auth: &driver.AuthRequest{Command: driver.AuthOpen, URL: "https://provider.invalid/"}},
		{Kind: driver.ItemAuthSession, Count: 4, Auth: &driver.AuthRequest{Command: driver.AuthCancel}},
	}}
	var answers []driver.Event
	out := filterAuthSessions(false, batch, func(ev driver.Event) { answers = append(answers, ev) })
	if len(batch.Items) != 4 || len(out.Items) != 1 || out.Items[0].Kind != driver.ItemKeepAwake {
		t.Fatalf("an older runtime received auth items, or the original batch changed: %+v", out.Items)
	}
	if len(answers) != 1 || answers[0].Kind != driver.EventAuthSession || answers[0].Count != 3 {
		t.Fatalf("answers %+v, want one unavailable end for the start", answers)
	}
	if ended, ok := answers[0].Data.(*driver.AuthEvent); !ok || ended.Phase != driver.AuthEnded || ended.Error != driver.AuthUnavailable {
		t.Fatalf("start answered %+v, want AuthUnavailable", answers[0].Data)
	}
	if filterAuthSessions(true, batch, func(driver.Event) { t.Fatal("a capable runtime's session was answered in Go") }) != batch {
		t.Fatal("a capable runtime's batch was copied")
	}
	plain := &driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemKeepAwake}}}
	if filterAuthSessions(false, plain, func(driver.Event) { t.Fatal("answered without a session") }) != plain {
		t.Fatal("a batch without sessions was copied")
	}

	// Go's bit is Java's, distinct from the fourth byte's other bits, and the runtime advertises it there.
	if capAuthSession == 0 || capAuthSession&(capAuthSession-1) != 0 || capAuthSession&capExtendedSensors != 0 {
		t.Fatalf("capAuthSession %d is not a free single bit of the fourth byte", capAuthSession)
	}
	protocol, err := os.ReadFile("java/dev/oneo/nsgo/Protocol.java")
	if err != nil {
		t.Fatal(err)
	}
	view, err := os.ReadFile("java/dev/oneo/nsgo/NsgoView.java")
	if err != nil {
		t.Fatal(err)
	}
	if m := regexp.MustCompile(`static final int CAP_AUTH_SESSION = (\d+);`).FindSubmatch(protocol); m == nil || string(m[1]) != strconv.Itoa(capAuthSession) {
		t.Fatalf("Protocol.java's CAP_AUTH_SESSION is %q, protocol.go's capAuthSession %d", m, capAuthSession)
	}
	if m := regexp.MustCompile(`static final int ITEM_AUTH_SESSION = (\d+);`).FindSubmatch(protocol); m == nil || string(m[1]) != strconv.Itoa(int(driver.ItemAuthSession)) {
		t.Fatalf("Protocol.java's ITEM_AUTH_SESSION is %q, driver.ItemAuthSession %d", m, driver.ItemAuthSession)
	}
	if m := regexp.MustCompile(`static final int EVENT_AUTH_SESSION = (\d+);`).FindSubmatch(protocol); m == nil || string(m[1]) != strconv.Itoa(int(driver.EventAuthSession)) {
		t.Fatalf("Protocol.java's EVENT_AUTH_SESSION is %q, driver.EventAuthSession %d", m, driver.EventAuthSession)
	}
	if !strings.Contains(string(view), "d.writeByte(Protocol.CAP_EXTENDED_SENSORS | Protocol.CAP_AUTH_SESSION | Protocol.CAP_VIDEO_PIP);") {
		t.Fatal("NsgoView's fourth capability byte does not advertise CAP_AUTH_SESSION")
	}

	// The environment's fourth byte turns the gate on only when a runtime wrote the bit.
	for _, c := range []struct {
		fourth []byte
		want   bool
	}{{nil, false}, {[]byte{capExtendedSensors}, false}, {[]byte{capAuthSession}, true}, {[]byte{capExtendedSensors | capAuthSession}, true}} {
		var w writer
		writeBaseEnvironment(&w, "en-US")
		w.byte(1)
		for range 4 {
			w.str("")
		}
		w.byte(capExtendedMessages)
		w.flags(false)
		w.str("")
		w.buf = append(w.buf, c.fourth...)
		r := reader{w.buf}
		if _, _, fourth := readEnvironmentCapabilities(&r); (fourth&capAuthSession != 0) != c.want {
			t.Errorf("fourth byte %v: auth sessions %v, want %v", c.fourth, fourth&capAuthSession != 0, c.want)
		}
	}
}
