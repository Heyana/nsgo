//go:build darwin

package apple

import (
	"sync/atomic"
	"time"
)

// A timeline keeps the recent slices the device has accepted, including those whose scheduled host time is still in the future. One render thread writes it. Position reads a slice only while its index is stable.
type pcmTimeline struct {
	latest atomic.Uint64
	total  int64 // render thread only, or while the unit is stopped
	slices [256]pcmSlice
}

type pcmSlice struct {
	index                       atomic.Uint64
	at, first, frames, produced atomic.Int64
}

func (t *pcmTimeline) push(at, frames, produced int64) {
	n := t.latest.Load() + 1
	s := &t.slices[n%uint64(len(t.slices))]
	s.index.Store(0)
	s.at.Store(at)
	s.first.Store(t.total)
	s.frames.Store(frames)
	s.produced.Store(produced)
	s.index.Store(n)
	t.total += frames
	t.latest.Store(n)
}

func (t *pcmTimeline) reset() {
	t.total = 0
	t.latest.Store(0)
}

func pcmDuration(frames int64, rate int) time.Duration {
	return time.Duration(frames * int64(time.Second) / int64(rate))
}

// position returns the heard frame count and its host time. Pending counts the client's unwritten-to-device frames, excluding underruns.
func (t *pcmTimeline) position(now int64, rate int) (frames, heard int64, pending int) {
	n := t.latest.Load()
	for count := uint64(0); n > 0 && count < uint64(len(t.slices)); n, count = n-1, count+1 {
		s := &t.slices[n%uint64(len(t.slices))]
		if s.index.Load() != n {
			continue
		}
		start, first, count, produced := s.at.Load(), s.first.Load(), s.frames.Load(), s.produced.Load()
		if s.index.Load() != n {
			continue
		}
		played := count
		if elapsed := max(now-start, 0); elapsed < int64(pcmDuration(count, rate)) {
			played = elapsed * int64(rate) / int64(time.Second)
		}
		pending += int(max(produced-played, 0))
		if played != 0 {
			return first + played, start + int64(pcmDuration(played, rate)), pending
		}
	}
	return 0, 0, pending
}
