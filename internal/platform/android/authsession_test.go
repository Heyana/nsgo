package android

import (
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
