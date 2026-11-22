package main

import (
	"testing"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestMain hands the main thread to the platform under nsgo test -target.
func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestShows(t *testing.T) {
	app := nsgotest.Start(t, Root, nsgotest.Size(390, 640))
	if got := app.Find("id:title").Text; got != "Cafe" {
		t.Errorf("title = %q", got)
	}
}

func TestShowsClipOrder(t *testing.T) {
	app := nsgotest.Start(t, Root, nsgotest.Storage(map[string]string{"cups": `{"version":0,"value":2}`}))
	if got := app.Find("id:last-order").Text; got != "Last order: 2 × Flat white" {
		t.Fatalf("last order = %q", got)
	}
}
