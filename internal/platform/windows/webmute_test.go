//go:build windows

package windows

import (
	"io"
	"net/http"
	"net/http/httptest"
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<!doctype html><p>Web audio mute</p>")
	}))
	t.Cleanup(server.Close)
	start, next := server.URL+"/start", server.URL+"/next"
	loads := make(chan string, 2)
	app := func(*core.Scope) core.Node {
		return core.WebView{Source: core.WebSrc(start), Muted: true, Incognito: true,
			Style:  core.Style{Width: core.Pt(200), Height: core.Pt(100)},
			OnLoad: func(page core.WebNavigation) { loads <- page.URL }}
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
	waitLoad := func(url string) {
		t.Helper()
		deadline := time.After(30 * time.Second)
		for {
			select {
			case loaded := <-loads:
				if loaded == url {
					return
				}
			case <-deadline:
				t.Fatalf("the web view did not load %q", url)
			}
		}
	}
	waitLoad(start)
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
					hr = audio.call(web8GetIsMuted, uintptr(unsafe.Pointer(&muted)))
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
	check(true, func(c *driver.WebConfig) { c.Source = core.WebSrc(next) })
	waitLoad(next)
	check(true, nil)
	check(false, func(c *driver.WebConfig) { c.Muted = false })
}
