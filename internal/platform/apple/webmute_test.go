//go:build darwin && !ios

package apple

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

func TestWebMuteSuspendsMedia(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a WebKit view")
	}
	const playing, suspended = 1, 3 // WKMediaPlaybackState
	const state = "requestMediaPlaybackStateWithCompletionHandler:"
	t.Cleanup(holdMainLoop())
	var web Web
	var window objc.ID
	t.Cleanup(func() {
		onMain(func() {
			// The test made the view itself, without make's handler and observers, which Close would remove.
			if web.web != 0 {
				web.web.Send(sel("stopLoading"))
				web.web.Send(sel("release"))
			}
			window.Send(sel("close"))
			window.Send(sel("release"))
		})
	})
	page := func(title string) driver.WebSource {
		tone := base64.StdEncoding.EncodeToString(wav(500 * time.Millisecond))
		return driver.WebSource{HTML: `<title>` + title + `</title><audio autoplay loop src="data:audio/wav;base64,` + tone + `"></audio>`}
	}
	var supported bool
	config := driver.WebConfig{Source: page("first")}
	onMain(func() {
		loadWebKit()
		objc.ID(Class("NSApplication")).Send(sel("sharedApplication"))
		frame := Rect(0, 0, 200, 100)
		window = objc.ID(Class("NSWindow")).Send(sel("alloc")).Send(sel("initWithContentRect:styleMask:backing:defer:"),
			frame, uint(1), uint(2), false)
		window.Send(sel("setReleasedWhenClosed:"), false)
		params := objc.ID(Class("WKWebViewConfiguration")).Send(sel("new"))
		defer params.Send(sel("release"))
		params.Send(sel("setMediaTypesRequiringUserActionForPlayback:"), uint(0))
		web = Web{web: objc.ID(Class("WKWebView")).Send(sel("alloc")).Send(sel("initWithFrame:configuration:"), frame, params), made: true}
		supported = responds(web.web, state)
		if supported {
			window.Send(sel("setContentView:"), web.web)
			window.Send(sel("orderFrontRegardless"))
			web.Configure(&config)
		}
	})
	if !supported {
		t.Skip("WebKit has no media playback state API")
	}
	wantState := func(title string, want int) {
		t.Helper()
		deadline := time.After(10 * time.Second)
		type playback struct {
			title string
			state int
		}
		states := make(chan playback, 1)
		for {
			onMain(func() {
				block := objcblock.New(func(_ objc.Block, state int) {
					states <- playback{title: GoString(web.web.Send(sel("title"))), state: state}
				})
				web.web.Send(sel(state), block)
				block.Release()
			})
			select {
			case got := <-states:
				if got.title == title && got.state == want {
					return
				}
			case <-deadline:
				t.Fatalf("WebKit page %q did not reach media state %d", title, want)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	wantState("first", playing)
	config.Muted = true
	onMain(func() { web.Configure(&config) })
	wantState("first", suspended)
	config.Source = page("second")
	onMain(func() { web.Configure(&config) })
	wantState("second", suspended)
	config.Muted = false
	onMain(func() { web.Configure(&config) })
	wantState("second", playing)
}
