package core

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestVideoConfiguresSeeksAndReports(t *testing.T) {
	var ref Ref
	var paused *Signal[bool]
	var info []VideoInfo
	var positions []time.Duration
	var ended, buffering int
	var pauses []VideoPauseReason
	var plays int
	var failed error
	h := newHost(t, Size{Width: 300, Height: 400}, func(cx *Scope) Node {
		paused = NewSignal(cx, true)
		return Video{
			Source:      "asset:/clip.mp4",
			AudioSource: "asset:/sound.m4a",

			PausedFunc: paused.Get,
			Volume:     2,
			Repeat:     true,
			Ref:        &ref,
			TestID:     "video",
			Style:      Style{Width: Pt(300), Height: Pt(200)},
			OnLoad:     func(i VideoInfo) { info = append(info, i) },
			OnProgress: func(p, _ time.Duration) { positions = append(positions, p) },
			OnEnd:      func() { ended++ },
			OnBuffer:   func(bool) { buffering++ },
			OnPause:    func(r VideoPauseReason) { pauses = append(pauses, r) },
			OnPlay:     func() { plays++ },
			OnError:    func(err error) { failed = err },
		}
	})
	b := h.commit()
	var cfg *driver.VideoConfig
	var mask driver.EventMask
	for _, it := range b.Items {
		switch it.Kind {
		case driver.ItemVideo:
			cfg = it.Video
		case driver.ItemEvents:
			if it.ID == h.find("video").id {
				mask = it.Events
			}
		}
	}
	if n := h.find("video"); n.kind != driver.KindVideo {
		t.Fatalf("kind %v, want KindVideo", n.kind)
	}
	want := driver.VideoConfig{Source: "asset:/clip.mp4", AudioSource: "asset:/sound.m4a", Paused: true, Volume: 1, Rate: 1, Repeat: true, ResizeMode: Contain,
		ProgressInterval: 250 * time.Millisecond}
	if cfg == nil || *cfg != want {
		t.Fatalf("config %+v, want %+v", cfg, want)
	}
	if !mask.Has(driver.EventLoad) || !mask.Has(driver.EventProgress) || !mask.Has(driver.EventEnd) ||
		!mask.Has(driver.EventVideoPause) || !mask.Has(driver.EventVideoPlay) || mask.Has(driver.EventLoadStart) {
		t.Fatalf("mask %b", mask)
	}
	if k := kinds(h.commit()); k[driver.ItemVideo] != 0 {
		t.Fatalf("resent an unchanged config: %v", k)
	}

	paused.Set(false)
	ref.Seek(-time.Second)
	ref.Seek(3 * time.Second)
	b = h.commit()
	var seeks []time.Duration
	for _, it := range b.Items {
		switch it.Kind {
		case driver.ItemVideo:
			if it.Video.Paused {
				t.Fatalf("still paused: %+v", it.Video)
			}
		case driver.ItemVideoSeek:
			seeks = append(seeks, it.Time)
		}
	}
	if k := kinds(b); k[driver.ItemVideo] != 1 || len(seeks) != 2 || seeks[0] != 0 || seeks[1] != 3*time.Second {
		t.Fatalf("items %v, seeks %v", k, seeks)
	}

	id := h.find("video").id
	h.tree.Dispatch(driver.Event{Kind: driver.EventLoad, ID: id, Duration: 90 * time.Second, Size: Size{Width: 640, Height: 360}})
	h.tree.Dispatch(driver.Event{Kind: driver.EventProgress, ID: id, Time: time.Second, Duration: 90 * time.Second})
	h.tree.Dispatch(driver.Event{Kind: driver.EventBuffer, ID: id, Buffering: true})
	h.tree.Dispatch(driver.Event{Kind: driver.EventEnd, ID: id})
	h.tree.Dispatch(driver.Event{Kind: driver.EventVideoPause, ID: id, Index: int(VideoPauseRouteLost)})
	h.tree.Dispatch(driver.Event{Kind: driver.EventVideoPlay, ID: id})
	h.tree.Dispatch(driver.Event{Kind: driver.EventError, ID: id, Text: "no decoder"})
	if len(info) != 1 || info[0].Duration != 90*time.Second || info[0].Size.Width != 640 ||
		len(positions) != 1 || positions[0] != time.Second || ended != 1 || buffering != 1 ||
		len(pauses) != 1 || pauses[0] != VideoPauseRouteLost || plays != 1 {
		t.Fatalf("info %v, positions %v, ended %d, buffering %d, pauses %v, plays %d", info, positions, ended, buffering, pauses, plays)
	}
	if me, ok := failed.(*MediaError); !ok || me.Message != "no decoder" || me.Reason != MediaErrorOther {
		t.Fatalf("error %#v", failed)
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventError, ID: id, Text: "alpha refused", Index: driver.VideoErrorAlphaUnsupported})
	if me, ok := failed.(*MediaError); !ok || me.Message != "alpha refused" || me.Reason != MediaErrorAlphaUnsupported {
		t.Fatalf("error %#v, want the alpha reason", failed)
	}
}

// videoOrder lists a batch's video items for the video with id: "video" for its config, "seek" for a seek.
func videoOrder(b *driver.MountBatch, id NodeID) []string {
	var got []string
	for _, it := range b.Items {
		switch {
		case it.ID != id:
		case it.Kind == driver.ItemVideo:
			got = append(got, "video")
		case it.Kind == driver.ItemVideoSeek:
			got = append(got, fmt.Sprint("seek ", it.Time))
		}
	}
	return got
}

// A seek in the update that changes a video's config goes first while the source stays, so a video at its end leaves it before it plays again, and after a config that opens a source, so it applies to that source.
func TestVideoSeeksAroundItsConfig(t *testing.T) {
	var ref Ref
	var source *Signal[string]
	var paused *Signal[bool]
	h := newHost(t, Size{Width: 300, Height: 400}, func(cx *Scope) Node {
		source, paused = NewSignal(cx, "asset:/a.mp4"), NewSignal(cx, true)
		cx.Effect(func() { ref.Seek(5 * time.Second) })
		return Video{SourceFunc: source.Get, PausedFunc: paused.Get, Ref: &ref, TestID: "video", Style: Style{Width: Pt(300), Height: Pt(200)}}
	})
	b := h.commit()
	id := h.find("video").id
	check := func(step string, b *driver.MountBatch, want ...string) {
		t.Helper()
		if got := videoOrder(b, id); !slices.Equal(got, want) {
			t.Fatalf("%s: %q, want %q", step, got, want)
		}
	}
	check("mounted", b, "video", "seek 5s")

	paused.Set(false)
	ref.Seek(0)
	ref.Seek(time.Second)
	check("played again", h.commit(), "seek 0s", "seek 1s", "video")

	ref.Seek(2 * time.Second)
	check("sought", h.commit(), "seek 2s")

	source.Set("asset:/b.mp4")
	ref.Seek(3 * time.Second)
	check("opened another", h.commit(), "video", "seek 3s")

	paused.Set(true)
	ref.Seek(4 * time.Second)
	check("paused", h.commit(), "seek 4s", "video")
}

// A video's pause and play share their kinds with image-size answers, which name no view: neither reaches the other, and a removed video's report goes nowhere.
func TestVideoPauseKeepsApartFromAnswers(t *testing.T) {
	var got []string
	var root *Scope
	var shown *Signal[bool]
	h := newHost(t, Size{Width: 300, Height: 300}, func(cx *Scope) Node {
		root, shown = cx, NewSignal(cx, true)
		return View{Children: []Node{Show{When: shown.Get, Then: func(*Scope) Node {
			return Video{Source: "asset:/clip.mp4", TestID: "video", Style: Style{Width: Pt(100), Height: Pt(100)},
				OnPause: func(r VideoPauseReason) { got = append(got, "pause "+r.String()) },
				OnPlay:  func() { got = append(got, "play") }}
		}}}}
	})
	h.commit()
	id := h.find("video").id
	root.ImageSize(Src("asset:/a.png"), func(px Size, err error) { got = append(got, fmt.Sprint("size ", px)) })
	var ask uint32
	for _, it := range h.commit().Items {
		if it.Kind == driver.ItemImageInfo {
			ask = it.Count
		}
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventVideoPause, ID: id, Count: ask, Index: driver.VideoPauseControls})
	h.tree.Dispatch(driver.Event{Kind: driver.EventVideoPlay, ID: id})
	h.tree.Dispatch(driver.Event{Kind: driver.EventImageInfo, Count: ask, Size: Size{Width: 8, Height: 4}})
	shown.Set(false)
	h.commit()
	h.tree.Dispatch(driver.Event{Kind: driver.EventVideoPause, ID: id, Index: driver.VideoPauseInterruption})
	want := []string{"pause controls", "play", fmt.Sprint("size ", Size{Width: 8, Height: 4})}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// The reasons are the driver's, which platforms report.
func TestVideoPauseReasons(t *testing.T) {
	for r, want := range map[VideoPauseReason]struct {
		name   string
		driver int
	}{
		VideoPauseInterruption: {"interruption", driver.VideoPauseInterruption},
		VideoPauseRouteLost:    {"route-lost", driver.VideoPauseRouteLost},
		VideoPauseControls:     {"controls", driver.VideoPauseControls},
	} {
		if got := r.String(); got != want.name || int(r) != want.driver {
			t.Errorf("%d.String() = %q, want %q; the driver's %d", r, got, want.name, want.driver)
		}
	}
}

func TestVideoDefaults(t *testing.T) {
	for _, c := range []struct {
		volume, rate       float32
		interval           time.Duration
		wantVolume, wantRt float32
		wantInterval       time.Duration
	}{
		{0, 0, 0, 1, 1, 250 * time.Millisecond},
		{0.5, 2, time.Millisecond, 0.5, 2, 16 * time.Millisecond},
		{-1, -1, time.Second, 1, 1, time.Second},
	} {
		if v, r, i := videoVolume(c.volume), videoRate(c.rate), videoInterval(c.interval); v != c.wantVolume || r != c.wantRt || i != c.wantInterval {
			t.Errorf("%v %v %v: got %v %v %v", c.volume, c.rate, c.interval, v, r, i)
		}
	}
}

func TestVideoChangesItsAudioSessionInPlace(t *testing.T) {
	var session *Signal[AudioSession]
	h := newHost(t, Size{Width: 300, Height: 400}, func(cx *Scope) Node {
		session = NewSignal(cx, AudioSessionPlayback)
		return Video{Source: "asset:/talk.mp4", AudioSessionFunc: session.Get, TestID: "video", Style: Style{Width: Pt(300), Height: Pt(200)}}
	})
	config := func(b *driver.MountBatch) []driver.VideoConfig {
		var got []driver.VideoConfig
		for _, it := range b.Items {
			if it.Kind == driver.ItemVideo {
				got = append(got, *it.Video)
			}
		}
		return got
	}
	if got := config(h.commit()); len(got) != 1 || got[0].AudioSession != AudioSessionPlayback {
		t.Fatalf("mounted with %+v, want AudioSessionPlayback", got)
	}
	id := h.find("video").id

	session.Set(AudioSessionMixWithOthers)
	b := h.commit()
	if got := config(b); len(got) != 1 || got[0].AudioSession != AudioSessionMixWithOthers || got[0].Source != "asset:/talk.mp4" || got[0].Paused {
		t.Fatalf("after the change %+v, want the same playing source with AudioSessionMixWithOthers", got)
	}
	if k := kinds(b); k[driver.ItemCreate] != 0 || k[driver.ItemDelete] != 0 || h.find("video").id != id {
		t.Fatalf("the change remounted the video: %v", k)
	}
	if k := kinds(h.commit()); k[driver.ItemVideo] != 0 {
		t.Fatalf("resent an unchanged session: %v", k)
	}
}
