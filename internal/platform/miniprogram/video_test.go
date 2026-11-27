package miniprogram

import (
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestVideoMountCommandsAndEventRoundTrip(t *testing.T) {
	h := &fakeHost{}
	m := newMounter(h)
	allVideoEvents := driver.EventMask(1<<driver.EventLoadStart | 1<<driver.EventLoad | 1<<driver.EventProgress |
		1<<driver.EventEnd | 1<<driver.EventBuffer | 1<<driver.EventError |
		1<<driver.EventVideoPlay | 1<<driver.EventVideoPause)

	m.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindVideo},
		frame(1, 0, 0, 320, 180),
		{Kind: driver.ItemEvents, ID: 1, Events: allVideoEvents},
		{Kind: driver.ItemVideo, ID: 1, Video: &driver.VideoConfig{
			Source:           "asset:clips/intro.mp4",
			Paused:           false,
			Muted:            false,
			Volume:           1,
			Rate:             1.5,
			Repeat:           false,
			Controls:         true,
			ResizeMode:       driver.Cover,
			ProgressInterval: 200 * time.Millisecond,
		}},
	}})

	if cOps := h.ops("c"); len(cOps) != 1 || cOps[0] != `[1,"video"]` {
		t.Fatalf("create ops = %v", cOps)
	}
	if vdOps := h.ops("vd"); len(vdOps) != 1 ||
		!strings.Contains(vdOps[0], `"/resources/clips/intro.mp4"`) ||
		!strings.Contains(vdOps[0], `"ap":true`) ||
		!strings.Contains(vdOps[0], `"ctrl":true`) ||
		!strings.Contains(vdOps[0], `"fit":"cover"`) {
		t.Fatalf("vd ops = %v", vdOps)
	}
	if vcOps := h.ops("vc"); len(vcOps) != 1 || vcOps[0] != `[1,[1,[["rate",1.5]]]]` {
		t.Fatalf("initial vc ops = %v", vcOps)
	}
	if len(h.events) != 1 || h.events[0].Kind != driver.EventLoadStart {
		t.Fatalf("initial events = %+v", h.events)
	}

	// App-initiated play notification does not emit a spurious EventVideoPlay.
	h.events = nil
	m.videoLoaded(1, 12.5, 1920, 1080)
	m.videoPlay(1)
	if len(h.events) != 1 || h.events[0].Kind != driver.EventLoad ||
		h.events[0].Duration != 12500*time.Millisecond ||
		h.events[0].Size != (driver.Size{Width: 1920, Height: 1080}) || !h.events[0].RateChangeable {
		t.Fatalf("load + app play events = %+v", h.events)
	}

	// Waiting -> buffering true; timeupdate -> buffering false + progress throttled by ProgressInterval.
	h.events = nil
	m.videoWaiting(1)
	m.videoTimeUpdate(1, 1.0, 12.5)
	m.videoTimeUpdate(1, 1.1, 12.5) // within 200ms interval -> throttled
	m.videoTimeUpdate(1, 1.3, 12.5) // >= 200ms -> reported
	if len(h.events) != 4 ||
		h.events[0].Kind != driver.EventBuffer || !h.events[0].Buffering ||
		h.events[1].Kind != driver.EventBuffer || h.events[1].Buffering ||
		h.events[2].Kind != driver.EventProgress || h.events[2].Time != time.Second ||
		h.events[3].Kind != driver.EventProgress || h.events[3].Time != 1300*time.Millisecond {
		t.Fatalf("buffer/progress events = %+v", h.events)
	}

	// Native controls pause & play report once each and hold across unrelated config updates.
	h.events = nil
	m.videoPause(1)
	m.videoPause(1) // duplicate ignored
	if len(h.events) != 1 || h.events[0].Kind != driver.EventVideoPause || h.events[0].Index != driver.VideoPauseControls {
		t.Fatalf("controls pause events = %+v", h.events)
	}
	// Unrelated rate change while held paused does not issue "play".
	prevVC := len(h.ops("vc"))
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemVideo, ID: 1, Video: &driver.VideoConfig{
			Source: "asset:clips/intro.mp4", Paused: false, Volume: 1, Rate: 2.0, Controls: true, ResizeMode: driver.Cover,
		}},
	}})
	if vc := h.ops("vc"); len(vc) != prevVC+1 || vc[len(vc)-1] != `[1,[2,[["rate",2]]]]` {
		t.Fatalf("rate-only vc while held paused = %v", vc)
	}

	// Controls resume reports EventVideoPlay.
	h.events = nil
	m.videoPlay(1)
	if len(h.events) != 1 || h.events[0].Kind != driver.EventVideoPlay {
		t.Fatalf("controls play events = %+v", h.events)
	}

	// Completion emits EventProgress + EventEnd (no spurious EventVideoPause).
	h.events = nil
	m.videoEnded(1)
	m.videoPause(1) // trailing pause after ended ignored
	if len(h.events) != 2 || h.events[0].Kind != driver.EventProgress || h.events[1].Kind != driver.EventEnd {
		t.Fatalf("ended events = %+v", h.events)
	}

	// Seeking before the end of an unpaused completed video queues seek + play in one vc op.
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemVideoSeek, ID: 1, Time: 3 * time.Second},
	}})
	if vc := h.ops("vc"); vc[len(vc)-1] != `[1,[3,[["seek",3],["play"]]]]` {
		t.Fatalf("seek-replay vc = %v", vc[len(vc)-1])
	}

	// Error reports EventError.
	h.events = nil
	m.videoError(1, "decode failed")
	if len(h.events) != 1 || h.events[0].Kind != driver.EventError || h.events[0].Text != "decode failed" {
		t.Fatalf("error events = %+v", h.events)
	}
}

func TestVideoSynthesizesLoadOnFirstTimeUpdateWithoutMetadata(t *testing.T) {
	h := &fakeHost{}
	m := newMounter(h)
	m.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindVideo},
		{Kind: driver.ItemEvents, ID: 1, Events: 1<<driver.EventLoad | 1<<driver.EventProgress},
		{Kind: driver.ItemVideo, ID: 1, Video: &driver.VideoConfig{Source: "https://example.com/v.mp4", Volume: 1, Rate: 1}},
	}})
	m.videoTimeUpdate(1, 0.5, 30)
	if len(h.events) != 2 || h.events[0].Kind != driver.EventLoad || h.events[0].Duration != 30*time.Second || h.events[1].Kind != driver.EventProgress {
		t.Fatalf("synthesized load events = %+v", h.events)
	}
}
