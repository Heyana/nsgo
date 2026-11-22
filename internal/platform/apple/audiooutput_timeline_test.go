//go:build darwin

package apple

import (
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestPCMPositionAndPendingFrames(t *testing.T) {
	var timeline pcmTimeline
	const rate = 1000
	const ms = int64(time.Millisecond)
	timeline.push(100*ms, 10, 10)
	timeline.push(110*ms, 10, 2)
	timeline.push(120*ms, 10, 10)
	for _, c := range []struct {
		now, frames, at int64
		pending         int
	}{
		{now: 98, pending: 22},
		{now: 105, frames: 5, at: 105, pending: 17},
		{now: 112, frames: 12, at: 112, pending: 10},
		{now: 129, frames: 29, at: 129, pending: 1},
		{now: 132, frames: 30, at: 130},
	} {
		frames, at, pending := timeline.position(c.now*ms, rate)
		if frames != c.frames || at != c.at*ms || pending != c.pending {
			t.Errorf("at %dms, got %d at %dms with %d pending, want %d at %dms with %d pending",
				c.now, frames, at/ms, pending, c.frames, c.at, c.pending)
		}
	}
	timeline.reset()
	if frames, at, pending := timeline.position(150*ms, rate); frames != 0 || at != 0 || pending != 0 {
		t.Fatalf("reset retained %d at %d with %d pending", frames, at, pending)
	}
}

func TestPCMRenderSilencesUnderrun(t *testing.T) {
	if !loadPCMUnit() {
		t.Skip("AudioToolbox is unavailable")
	}
	const channels = 2
	o := &audioOutput{
		id: uintptr(pcmID.Add(1)),
		conf: driver.AudioOutputConfig{
			SampleRate: 48000,
			Channels:   channels,
			Render: func(samples []float32) int {
				samples[0], samples[1] = 0.5, -0.5
				return 1
			},
		},
	}
	pcmStreams.Store(o.id, o)
	defer pcmStreams.Delete(o.id)
	samples := []float32{1, 1, 1, 1, 1, 1}
	out := audioBufferList{count: 1}
	out.buffer.channels, out.buffer.bytes, out.buffer.data = channels, uint32(len(samples)*4), &samples[0]
	if status := renderPCM(o.id, nil, nil, 0, uint32(len(samples)/channels), &out); status != 0 {
		t.Fatalf("render status %d", status)
	}
	if want := []float32{0.5, -0.5, 0, 0, 0, 0}; !slices.Equal(samples, want) || o.Underruns() != 1 {
		t.Fatalf("got samples %v and %d underruns, want %v and one underrun", samples, o.Underruns(), want)
	}
}
