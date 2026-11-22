package authscheme

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestSchemeSessionPreparesAndOpensTheBrowser(t *testing.T) {
	var events []driver.Event
	dispatch := func(ev driver.Event) { events = append(events, ev) }
	var opened string
	open := func(u string, answer func(bool)) { opened = u; answer(true) }
	if Handle(9, driver.AuthRequest{Command: driver.AuthStart}, open, dispatch) || len(events) != 0 {
		t.Fatal("an empty scheme took the custom-scheme flow")
	}
	r := driver.AuthRequest{Command: driver.AuthStart, Scheme: "com.example.app", Path: "/a/../callback"}
	if !Handle(9, r, open, dispatch) || len(events) != 1 {
		t.Fatal("the custom scheme did not prepare its redirect")
	}
	event := events[0].Data.(*driver.AuthEvent)
	if event.Phase != driver.AuthRedirect || !event.OpenLinks || event.Redirect != "com.example.app:/callback" {
		t.Fatalf("prepared %+v", event)
	}
	r.Command, r.URL = driver.AuthOpen, "https://provider.example/authorize"
	Handle(9, r, open, dispatch)
	if opened != r.URL || len(events) != 1 {
		t.Fatal("the browser did not open or the session ended before its callback")
	}
	r.Command = driver.AuthCancel
	Handle(9, r, open, dispatch)
	if len(events) != 1 {
		t.Fatal("cancelling a browser session reported a second answer")
	}
}

func TestSchemeSessionReportsBrowserFailureAndUnavailableHTTPS(t *testing.T) {
	var events []driver.Event
	dispatch := func(ev driver.Event) { events = append(events, ev) }
	Handle(3, driver.AuthRequest{Command: driver.AuthOpen, Scheme: "app", URL: "https://provider.example"},
		func(_ string, answer func(bool)) { answer(false) }, dispatch)
	Handle(4, driver.AuthRequest{Command: driver.AuthStart, Scheme: "https", Host: "example.com", Path: "/oauth"}, nil, dispatch)
	if len(events) != 2 || events[0].Data.(*driver.AuthEvent).Error != driver.AuthFailed || events[1].Data.(*driver.AuthEvent).Error != driver.AuthUnavailable {
		t.Fatalf("errors = %+v", events)
	}
}
