package main

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

// One application exercises the terminal's editor, pointer activation,
// keyboard focus and keymap, then its resized layout, through the backend.
func TestTerminalInteractions(t *testing.T) {
	app := nsgotest.Start(t, Root, nsgotest.Size(640, 384), nsgotest.Environment(core.Environment{OS: core.OSLinux, Idiom: core.IdiomDesktop}))
	app.Type("id:name", "界éX")
	app.Key("Backspace")
	if got := app.Find("id:result").Text; got != "Hello 界é: 0" {
		t.Fatalf("Unicode editing: %q", got)
	}
	if nsgotest.Target() == "tui" {
		app.Type("id:name", "👩‍💻")
		app.Key("Backspace")
		if got := app.Find("id:result").Text; got != "Hello 界é: 0" {
			t.Fatalf("emoji grapheme deletion: %q", got)
		}
	}
	app.Key("mod+k")
	app.Key("Tab")
	app.Press("id:count")
	app.Focus("id:count")
	app.Key("Enter")
	if got := app.Find("id:result").Text; got != "Hello 界é: 3" {
		t.Fatalf("keyboard/mouse activation: %q", got)
	}
	app.Key("shift+tab")
	app.Key("mod+a")
	app.Type("id:name", "Ada")
	if got := app.Find("id:result").Text; !strings.Contains(got, "Ada") {
		t.Fatalf("selection replacement: %q", got)
	}
	app.Resize(480, 320)
	if got := app.Find("id:size").Text; got != "Surface: 480 × 320" {
		t.Fatalf("resize: %q", got)
	}
	app.Screenshot("terminal-interactions")
}
