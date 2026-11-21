package videoheaders

import (
	"bytes"
	_ "embed"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	// The Android test runner needs the app's platform host.
	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// clip was generated from a solid blue frame with ffmpeg:
// ffmpeg -f lavfi -i color=c=blue:s=96x64:r=15 -t 4 -c:v libx264 -pix_fmt yuv420p -movflags +faststart clip.mp4.
//
//go:embed clip.mp4
var clip []byte

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestVideoPlaysWithRequestHeaders(t *testing.T) {
	if nsgotest.Target() != "android" {
		t.Skip("needs the Android video player")
	}
	headers := map[string]string{
		"Referer":    "https://example.invalid/player",
		"User-Agent": "NSGo video test",
		"Cookie":     "fixture=video",
	}
	var accepted, refused atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for name, want := range headers {
			if r.Header.Get(name) != want {
				refused.Add(1)
				http.Error(w, "the player did not send "+name, http.StatusForbidden)
				return
			}
		}
		accepted.Add(1)
		if r.URL.Path == "/protected.mp4" {
			http.Redirect(w, r, "/clip.mp4", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		http.ServeContent(w, r, "clip.mp4", time.Time{}, bytes.NewReader(clip))
	}))
	t.Cleanup(server.Close)

	var loaded atomic.Pointer[core.VideoInfo]
	var progress atomic.Int64
	var failure atomic.Value
	failure.Store("")
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.Video{
			Source:           server.URL + "/protected.mp4",
			Headers:          headers,
			Style:            core.Style{Width: core.Pt(160), Height: core.Pt(90)},
			ProgressInterval: 30 * time.Millisecond,
			OnLoad:           func(info core.VideoInfo) { loaded.Store(&info) },
			OnProgress:       func(at, _ time.Duration) { progress.Store(int64(at)) },
			OnError:          func(err error) { failure.Store(err.Error()) },
		}
	})

	app.WaitUntil(12*time.Second, func() bool {
		return progress.Load() >= int64(250*time.Millisecond) || failure.Load().(string) != ""
	})
	if err := failure.Load().(string); err != "" {
		t.Fatalf("player error %s with %d accepted and %d refused requests", err, accepted.Load(), refused.Load())
	}
	if info := loaded.Load(); info == nil || info.Duration < 3*time.Second || info.Size.Width <= 0 || info.Size.Height <= 0 {
		t.Fatalf("the native player did not load the protected MP4: %+v", info)
	}
	if accepted.Load() < 2 || refused.Load() != 0 {
		t.Fatalf("requests accepted %d, refused %d", accepted.Load(), refused.Load())
	}
}
