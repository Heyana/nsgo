//go:build windows

package windows

import (
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWebAudioMutePersistsAcrossNavigation(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if _, err := findWebView2(); err != nil {
		t.Skip(err)
	}
	loads := make(chan string, 2)
	app := func(*core.Scope) core.Node {
		return core.WebView{Source: webMutePage("start"), Muted: true, Incognito: true,
			Style:     core.Style{Width: core.Pt(200), Height: core.Pt(100)},
			OnMessage: func(page string) { loads <- page }}
	}
	const title = "Web audio mute"
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		Run(app, Options{Title: title, Width: 200, Height: 100})
	}()
	hwnd := waitForWindow(t, title, done)
	if hwnd == 0 {
		return
	}
	t.Cleanup(func() {
		postMessageW.Call(hwnd, wmClose, 0, 0)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("the window did not close")
		}
	})
	waitLoad := func(title string) {
		t.Helper()
		deadline := time.After(30 * time.Second)
		for {
			select {
			case loaded := <-loads:
				if loaded == title {
					return
				}
			case <-deadline:
				t.Fatalf("the web view did not load %q", title)
			}
		}
	}
	waitLoad("start")
	var widget *comObject
	check := func(want bool, change func(*driver.WebConfig)) {
		t.Helper()
		var muted int32
		var native *comObject
		var supported bool
		hr := uintptr(eFail)
		onMain(t, hwnd, func(w *window) {
			for _, v := range w.views {
				if v == nil || v.web == nil || v.web.web == nil {
					continue
				}
				if change != nil {
					cfg := v.web.config
					change(&cfg)
					v.web.configure(&cfg)
				}
				native = v.web.web
				if audio := native.query(&iidWeb8); audio != nil {
					supported = true
					hr = audio.call(83, uintptr(unsafe.Pointer(&muted))) // ICoreWebView2_8::get_IsMuted
					audio.release()
				}
				break
			}
		})
		if native != nil && !supported {
			t.Skip("WebView2 runtime lacks ICoreWebView2_8")
		}
		if native == nil || widget != nil && native != widget || failed(hr) || (muted != 0) != want {
			t.Fatalf("WebView2 view %p has mute %v, want the same view with mute %v (HRESULT %x)", native, muted != 0, want, hr)
		}
		widget = native
	}
	check(true, func(c *driver.WebConfig) { c.Source = webMutePage("next") })
	waitLoad("next")
	check(true, nil)
	check(false, func(c *driver.WebConfig) { c.Muted = false })
}

// The page's reply identifies the new document even if its title arrives after the platform's load notification.
func webMutePage(name string) core.WebSource {
	return core.WebHTML("<script>window.nsgo.postMessage('"+name+"')</script>", "")
}
