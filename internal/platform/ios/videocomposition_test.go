//go:build ios && nsgotest

package ios

import (
	"bytes"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	alphaclips "github.com/nsgo-dev/nsgo/internal/platform/videoalpha/clips"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestIOSVideoSeekWithSeparateAudio(t *testing.T) {
	const rate, dataSize = 8000, 16000
	wav := make([]byte, 44+dataSize)
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], rate)
	binary.LittleEndian.PutUint32(wav[28:], 2*rate)
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], dataSize)

	seekPosted := make(chan struct{})
	var releaseAudio sync.Once
	unblockAudio := func() { releaseAudio.Do(func() { close(seekPosted) }) }
	audioRequested := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/picture.mov":
			w.Header().Set("Content-Type", "video/quicktime")
			http.ServeContent(w, r, "picture.mov", time.Time{}, bytes.NewReader(alphaclips.HEVC))
		case "/sound.wav":
			select {
			case audioRequested <- struct{}{}:
			default:
			}
			select {
			case <-seekPosted:
			case <-r.Context().Done():
				return
			}
			w.Header().Set("Content-Type", "audio/wav")
			http.ServeContent(w, r, "sound.wav", time.Time{}, bytes.NewReader(wav))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(func() {
		unblockAudio()
		server.Close()
	})

	var ref core.Ref
	var paused *core.Signal[bool]
	var loaded atomic.Pointer[core.VideoInfo]
	var firstProgress atomic.Int64
	var advancing atomic.Bool
	var failure atomic.Pointer[string]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		paused = core.NewSignal(cx, true)
		return core.Video{
			Source: server.URL + "/picture.mov", AudioSource: server.URL + "/sound.wav", Ref: &ref,
			PausedFunc: paused.Get, Muted: true, ProgressInterval: 30 * time.Millisecond,
			Style:  core.Style{Width: core.Pt(160), Height: core.Pt(120)},
			OnLoad: func(info core.VideoInfo) { loaded.Store(&info) },
			OnProgress: func(at, _ time.Duration) {
				if at > 0 && !firstProgress.CompareAndSwap(0, int64(at)) && int64(at) > firstProgress.Load() {
					advancing.Store(true)
				}
			},
			OnError: func(err error) {
				message := err.Error()
				failure.Store(&message)
			},
		}
	})
	app.WaitUntil(10*time.Second, func() bool { return len(audioRequested) > 0 || failure.Load() != nil })
	if message := failure.Load(); message != nil {
		t.Fatal(*message)
	}
	// Keep the audio response blocked while the seek reaches the native view.
	app.Post(func() { ref.Seek(250 * time.Millisecond) })
	app.Settle()
	unblockAudio()
	app.Post(func() { paused.Set(false) })
	app.WaitUntil(10*time.Second, func() bool { return loaded.Load() != nil && advancing.Load() || failure.Load() != nil })
	if message := failure.Load(); message != nil {
		t.Fatal(*message)
	}
	if info := loaded.Load(); info == nil || info.Duration != 500*time.Millisecond || info.Size.Width <= 0 || info.Size.Height <= 0 {
		t.Fatalf("loaded video = %+v, want the half-second picture", info)
	}
	if at := time.Duration(firstProgress.Load()); at < 225*time.Millisecond || at >= 450*time.Millisecond {
		t.Fatalf("first progress = %v, want playback to begin near the queued 250ms seek", at)
	}
	var tracks [2]int
	onMainSync(func() {
		for _, v := range runner.views {
			if v == nil || v.video == nil {
				continue
			}
			layers := v.native.Send(sel("layer")).Send(sel("sublayers"))
			for i := 0; i < objc.Send[int](layers, sel("count")); i++ {
				layer := layers.Send(sel("objectAtIndex:"), i)
				if objc.Send[bool](layer, sel("isKindOfClass:"), objc.ID(apple.Class("AVPlayerLayer"))) {
					asset := layer.Send(sel("player")).Send(sel("currentItem")).Send(sel("asset"))
					for j, kind := range []string{"vide", "soun"} {
						tracks[j] = objc.Send[int](asset.Send(sel("tracksWithMediaType:"), apple.NSString(kind)), sel("count"))
					}
				}
			}
		}
	})
	if tracks != [2]int{1, 1} {
		t.Fatalf("player tracks = %v, want one picture and one audio track in the same asset", tracks)
	}
}
