package watchos

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestAuthSessionHostProtocol(t *testing.T) {
	var sessions authSessions
	var w writer
	var events []driver.Event
	dispatch := func(ev driver.Event) { events = append(events, ev) }
	request := driver.AuthRequest{Command: driver.AuthStart, Scheme: "https", Host: "auth.example.test", Path: "/callback", Ephemeral: true}
	sessions.handle(42, request, &w, dispatch)
	if len(events) != 1 || events[0].Count != 42 || events[0].Data.(*driver.AuthEvent).Redirect != "https://auth.example.test/callback" || len(w.buf) != 0 {
		t.Fatal("preparing the redirect opened native UI or lost the callback")
	}
	sessions.handle(42, driver.AuthRequest{Command: driver.AuthOpen, URL: "https://provider.example.test/authorize"}, &w, dispatch)
	r := bytes.NewReader(w.buf)
	byteValue := func(want byte) {
		t.Helper()
		got, err := r.ReadByte()
		if err != nil || got != want {
			t.Fatalf("host byte = %d, %v; want %d", got, err, want)
		}
	}
	u32 := func() uint32 {
		t.Helper()
		var v uint32
		if err := binary.Read(r, binary.LittleEndian, &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	text := func(want string) {
		t.Helper()
		data := make([]byte, u32())
		if _, err := io.ReadFull(r, data); err != nil || string(data) != want {
			t.Fatalf("host text = %q, %v; want %q", data, err, want)
		}
	}
	byteValue(opAuthSession)
	if id := u32(); id != 42 {
		t.Fatalf("host session = %d", id)
	}
	byteValue(byte(driver.AuthOpen))
	text("https://provider.example.test/authorize")
	text(request.Scheme)
	text(request.Host)
	text(request.Path)
	byteValue(1)
	if r.Len() != 0 {
		t.Fatal("open command has trailing fields")
	}
	w.buf = nil
	sessions.handle(42, driver.AuthRequest{Command: driver.AuthOpen, URL: "https://other.example.test"}, &w, dispatch)
	if len(w.buf) != 0 {
		t.Fatal("an opened request opened another native session")
	}
	sessions.handle(42, driver.AuthRequest{Command: driver.AuthCancel}, &w, dispatch)
	if !bytes.Equal(w.buf, []byte{opAuthSession, 42, 0, 0, 0, byte(driver.AuthCancel)}) {
		t.Fatalf("cancel protocol %v", w.buf)
	}
	sessions.finished(42, 0, "https://auth.example.test/callback?code=late", dispatch)
	if len(events) != 1 || len(sessions.pending) != 0 {
		t.Fatal("cancelled session accepted a late completion")
	}
}

func TestAuthSessionHostOutcomes(t *testing.T) {
	for _, test := range []struct {
		name    string
		outcome uint8
		value   string
		error   string
	}{
		{"callback", 0, "nsgo-test:/callback?code=synthetic&state=%E5%80%A4", ""},
		{"cancelled", 1, "", driver.AuthCancelled},
		{"failure", 2, "native failure", driver.AuthFailed},
		{"unavailable", 3, "needs a newer OS", driver.AuthUnavailable},
		{"unknown", 99, "unknown native result", driver.AuthFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			var sessions authSessions
			var w writer
			var events []driver.Event
			dispatch := func(ev driver.Event) { events = append(events, ev) }
			sessions.handle(7, driver.AuthRequest{Command: driver.AuthStart, Scheme: "nsgo-test", Path: "/callback"}, &w, dispatch)
			sessions.handle(7, driver.AuthRequest{Command: driver.AuthOpen, URL: "https://provider.example.test"}, &w, dispatch)
			sessions.finished(7, test.outcome, test.value, dispatch)
			sessions.finished(7, test.outcome, test.value, dispatch)
			if len(events) != 2 || events[1].Kind != driver.EventAuthSession || events[1].Count != 7 || len(sessions.pending) != 0 {
				t.Fatalf("completion events %v", events)
			}
			report := events[1].Data.(*driver.AuthEvent)
			if report.Phase != driver.AuthEnded || report.Error != test.error ||
				test.error == "" && report.Callback != test.value || test.error != "" && report.Callback != "" ||
				test.outcome != 0 && test.outcome != 1 && report.Message != test.value {
				t.Fatalf("completion %+v", report)
			}
		})
	}
}

func TestAuthSessionWithoutSchemeAndBeforeOpen(t *testing.T) {
	var sessions authSessions
	var w writer
	var events []driver.Event
	dispatch := func(ev driver.Event) { events = append(events, ev) }
	sessions.handle(1, driver.AuthRequest{Command: driver.AuthStart, Path: "/callback"}, &w, dispatch)
	if len(events) != 1 || events[0].Data.(*driver.AuthEvent).Error != driver.AuthUnavailable || len(sessions.pending) != 0 {
		t.Fatal("watchOS offered a loopback redirect")
	}
	sessions.handle(2, driver.AuthRequest{Command: driver.AuthStart, Scheme: "nsgo-test", Path: "/callback"}, &w, dispatch)
	sessions.finished(2, 0, "unexpected", dispatch)
	sessions.handle(2, driver.AuthRequest{Command: driver.AuthCancel}, &w, dispatch)
	sessions.handle(2, driver.AuthRequest{Command: driver.AuthOpen, URL: "https://provider.example.test"}, &w, dispatch)
	if len(events) != 2 || len(w.buf) != 0 || len(sessions.pending) != 0 {
		t.Fatal("a request cancelled before opening reached the host")
	}
}
