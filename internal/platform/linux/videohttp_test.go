//go:build linux && !android

package linux

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestVideoSendsHTTPHeadersWhenSourceChanges(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	const referer = "https://video.example/watch"
	const userAgent = "nsgo-video-test"
	const cookie = "session=local"
	var protectedRequests [2]atomic.Int32
	var redirected atomic.Bool
	var manifestProtected atomic.Bool
	segments := make(chan http.Header, 1)
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/segment.ts" {
			select {
			case segments <- r.Header:
			default:
			}
		} else {
			redirected.Store(true)
		}
		http.NotFound(w, r)
	}))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		configuredHeaders := r.Referer() == referer && r.UserAgent() == userAgent && r.Header.Get("Cookie") == cookie
		if r.URL.Path == "/manifest" {
			if configuredHeaders {
				manifestProtected.Store(true)
			}
			// GStreamer may fetch the playlist again through its own downloader.
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			fmt.Fprintf(w, "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:4\n#EXTINF:4,\n%s/segment.ts\n#EXT-X-ENDLIST\n", destination.URL)
			return
		}
		if !configuredHeaders {
			t.Logf("HTTP request for %s sent Referer=%q User-Agent=%q Cookie=%q", r.URL.Path, r.Referer(), r.UserAgent(), r.Header.Get("Cookie"))
			http.Error(w, "missing playback headers", http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/redirect.webm":
			http.Redirect(w, r, destination.URL+"/video.webm", http.StatusFound)
			return
		case "/one.webm":
			protectedRequests[0].Add(1)
		case "/two.webm":
			protectedRequests[1].Add(1)
		default:
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, "video.webm", time.Time{}, bytes.NewReader(videoRateClip))
	}))
	defer server.Close()

	var changeSource func(string)
	loads := make(chan core.VideoInfo, 4)
	errors := make(chan error, 4)
	w := startTestWindow(t, func(cx *core.Scope) core.Node {
		source := core.NewSignal(cx, server.URL+"/one.webm")
		changeSource = func(path string) { cx.Post(func() { source.Set(server.URL + path) }) }
		return core.Video{
			SourceFunc: source.Get,
			Headers:    map[string]string{"referer": referer, "User-Agent": userAgent, "Cookie": cookie},
			Muted:      true,
			Style:      core.Style{Width: driver.Pt(200), Height: driver.Pt(100)},
			OnLoad:     func(info core.VideoInfo) { loads <- info },
			OnError:    func(err error) { errors <- err },
		}
	}, Options{Title: "nsgo-native-video-headers", Width: 360, Height: 240})
	var requiredPlugins bool
	w.onLoop(t, func() { requiredPlugins = bindVideoGST() && gstFeature("souphttpsrc") })
	if !requiredPlugins {
		t.Skip("requires GstPlay, scaletempo and souphttpsrc")
	}
	for i := range protectedRequests {
		if i == 1 {
			changeSource("/two.webm")
		}
		select {
		case info := <-loads:
			if info.Duration < 7*time.Second || protectedRequests[i].Load() == 0 {
				t.Fatalf("source %d loaded without its protected HTTP request: %+v", i+1, info)
			}
		case err := <-errors:
			t.Fatalf("source %d: %v", i+1, err)
		case <-time.After(8 * time.Second):
			w.onLoop(t, func() {
				for _, v := range w.views {
					if v != nil && v.video != nil && v.video.player != nil {
						t.Logf("prepared=%d width=%d duration=%d", gtkMediaStreamIsPrepared(v.video.stream), videoWidth(v.video.stream), gtkMediaStreamGetDuration(v.video.stream))
					}
				}
			})
			t.Fatalf("source %d never loaded after %d protected requests", i+1, protectedRequests[i].Load())
		}
	}
	changeSource("/redirect.webm")
	select {
	case <-errors:
		if redirected.Load() {
			t.Fatal("the configured User-Agent permits a redirect to another origin")
		}
	case <-loads:
		t.Fatal("the redirected video loaded")
	case <-time.After(8 * time.Second):
		t.Fatal("the redirect did not report an error")
	}

	t.Run("adaptive request", func(t *testing.T) {
		var hasHLS bool
		w.onLoop(t, func() { hasHLS = gstFeature("hlsdemux2") })
		if !hasHLS {
			t.Skip("requires hlsdemux2")
		}
		changeSource("/manifest")
		select {
		case headers := <-segments:
			if !manifestProtected.Load() {
				t.Fatal("the first manifest request did not send app headers")
			}
			if headers.Get("Cookie") != "" || headers.Get("User-Agent") == userAgent || headers.Get("Referer") == referer {
				t.Fatal("the segment request received app headers from the manifest request")
			}
		case <-time.After(8 * time.Second):
			t.Fatal("GStreamer did not request the adaptive segment")
		}
	})
}
