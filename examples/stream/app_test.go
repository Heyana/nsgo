package main

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestMain hands the main thread to the platform under nsgo test -target.
func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestStreamsTheReply(t *testing.T) {
	app := nsgotest.Start(t, Root, nsgotest.Size(860, 760))
	app.Press("id:pace-fast")
	app.Advance(time.Second)
	if app.Exists("text:Fade in new characters") {
		t.Fatal("the reply arrived at once")
	}
	app.Advance(30 * time.Second)
	app.Find("text:Fade in new characters")
	app.Find("role:header Streaming Markdown smoothly")
	app.Press("id:replay")
	app.Advance(100 * time.Millisecond)
	if app.Exists("text:Fade in new characters") {
		t.Error("replaying kept the reply")
	}
}
