//go:build android && nsgotest

package android

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/testhost"
	"github.com/nsgo-dev/nsgo/widget"
)

// This opt-in test runs with the launcher controller: it confirms the system's
// pin dialog, checks launcher pixels and taps the real widget PendingIntent.
// The installed test APK owns its providers, timeline files and URLs.
func TestAndroidLauncherWidgetTimeline(t *testing.T) {
	if os.Getenv("NSGO_ANDROID_WIDGET_ACCEPTANCE") != "1" {
		t.Skip("run with the Android launcher widget controller")
	}
	root := os.Getenv("NSGO_WIDGETS_DIR")
	if root == "" || !widget.Available() {
		t.Fatal("packaged widgets are unavailable before nsgo.Run")
	}
	entry := func(color uint32, label, url string) widget.Entry {
		return widget.Entry{URL: url, Background: widget.Rectangle{Fill: widget.RGB(color)}, View: widget.Column{Style: widget.Style{Gap: 8, FillWidth: true, FillHeight: true, Link: strings.Replace(url, "/entry/", "/link/", 1)}, Children: []widget.View{
			widget.Text{Content: label, Style: widget.Style{Color: widget.RGB(0xffffff), FontSize: 24, FontWeight: core.FontWeightBold}},
			widget.Progress{Value: 0.75, Style: widget.Style{Width: 100, Color: widget.RGB(0xffffff)}},
			widget.Date{Time: time.Now().Add(time.Minute), Format: widget.Timer, Style: widget.Style{Color: widget.RGB(0xffffff), FontSize: 16}},
		}}}
	}
	red := entry(0xc02020, "Timeline 17", "nsgo-widget://entry/red")
	if err := widget.Update("acceptance", red); err != nil {
		t.Fatal(err)
	}
	links := make(chan string, 4)
	window, err := testhost.Current().Open(func(cx *core.Scope) core.Node {
		cx.OnOpenLinks(func(opened []string) {
			for _, url := range opened {
				select {
				case links <- url:
				default:
				}
			}
		})
		return core.View{Style: core.Style{Width: core.Pct(100), Height: core.Pct(100), Background: core.RGB(0xffffff)}, Children: []core.Node{core.Text{Content: "Android widget acceptance"}}}
	}, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	surface := window.(*testSurface)
	surface.Sync()
	marker := func(phase string) {
		fmt.Fprintln(os.Stderr, "   widget acceptance "+phase)
		if err := os.WriteFile(filepath.Join(root, "accept-phase"), []byte(phase), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ack := func(phase string) {
		t.Helper()
		marker(phase)
		file := filepath.Join(root, "accept-"+phase)
		defer os.Remove(file)
		for deadline := time.Now().Add(2 * time.Minute); ; {
			if _, err := os.Stat(file); err == nil {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("launcher controller did not acknowledge %s", phase)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	pin, err := surface.ask(map[string]any{"op": "widget-pin", "kind": "acceptance"})
	if err != nil || pin != "true" {
		t.Fatalf("pin request %q,%v", pin, err)
	}
	marker("pin-requested")
	for deadline := time.Now().Add(time.Minute); ; {
		data, err := surface.ask(map[string]any{"op": "widget-probe", "kind": "acceptance"})
		if err != nil {
			t.Fatal(err)
		}
		var probe struct {
			IDs []int `json:"ids"`
		}
		if err := json.Unmarshal([]byte(data), &probe); err != nil {
			t.Fatal(err)
		}
		if len(probe.IDs) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("launcher never bound the widget")
		}
		time.Sleep(100 * time.Millisecond)
	}
	ack("red-visible")
	green := entry(0x208040, "Timeline 29", "nsgo-widget://entry/green")
	green.Date = time.Now().Add(10 * time.Second)
	if err := widget.Update("acceptance", red, green); err != nil {
		t.Fatal(err)
	}
	ack("green-visible")
	marker("tap-green-link")
	select {
	case url := <-links:
		if url != "nsgo-widget://link/green" {
			t.Fatalf("widget opened %q", url)
		}
	case <-time.After(time.Minute):
		t.Fatal("real widget tap did not reach the app's URL handler")
	}
	marker("tap-green-entry")
	select {
	case url := <-links:
		if url != "nsgo-widget://entry/green" {
			t.Fatalf("widget entry opened %q", url)
		}
	case <-time.After(time.Minute):
		t.Fatal("widget margin tap did not open Entry.URL")
	}
	blue := entry(0x2040b0, "Timeline 41", "nsgo-widget://entry/blue")
	if err := widget.Update("acceptance", blue); err != nil {
		t.Fatal(err)
	}
	ack("blue-visible")
	if data, err := os.ReadFile(filepath.Join(root, "acceptance", "timeline.json")); err != nil || !strings.Contains(string(data), "Timeline 41") {
		t.Fatalf("persisted timeline %q,%v", data, err)
	}
	marker("complete")
}
