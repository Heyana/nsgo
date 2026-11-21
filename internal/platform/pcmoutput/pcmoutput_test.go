package pcmoutput

import (
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type fakeDevice struct {
	mu           sync.Mutex
	bufferFrames int
	periodFrames int
	channels     int

	prepared    int
	dropped     int
	closed      int
	queued      int64 // frames currently in ring buffer
	extraDelay  int64 // extra downstream hardware latency
	delayBroken bool

	writtenSamples []float32
	writeLimit     int // if > 0, cap frames accepted per Write call

	nextWriteErr error
	nextAvailErr error
	nextWaitErr  error
	prepareErr   error

	wakeCh chan struct{}
}

func newFakeDevice(bufferFrames, periodFrames, channels int) *fakeDevice {
	return &fakeDevice{
		bufferFrames: bufferFrames,
		periodFrames: periodFrames,
		channels:     channels,
		wakeCh:       make(chan struct{}, 16),
	}
}

func (d *fakeDevice) BufferFrames() int { return d.bufferFrames }
func (d *fakeDevice) PeriodFrames() int { return d.periodFrames }

func (d *fakeDevice) Prepare() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.prepareErr != nil {
		err := d.prepareErr
		d.prepareErr = nil
		return err
	}
	d.prepared++
	d.queued = 0
	return nil
}

func (d *fakeDevice) AvailDelay() (int, int64, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.nextAvailErr != nil {
		err := d.nextAvailErr
		d.nextAvailErr = nil
		return 0, 0, false, err
	}
	avail := max(d.bufferFrames-int(d.queued), 0)
	if d.delayBroken {
		return avail, 0, false, nil
	}
	return avail, d.queued + d.extraDelay, true, nil
}

func (d *fakeDevice) Wait(timeout time.Duration) (bool, error) {
	d.mu.Lock()
	if d.nextWaitErr != nil {
		err := d.nextWaitErr
		d.nextWaitErr = nil
		d.mu.Unlock()
		return false, err
	}
	if d.bufferFrames-int(d.queued) >= d.periodFrames {
		d.mu.Unlock()
		return true, nil
	}
	ch := d.wakeCh
	d.mu.Unlock()

	select {
	case <-ch:
		return true, nil
	case <-time.After(timeout):
		return false, nil
	}
}

func (d *fakeDevice) Write(samples []float32) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.nextWriteErr != nil {
		err := d.nextWriteErr
		d.nextWriteErr = nil
		return 0, err
	}
	avail := max(d.bufferFrames-int(d.queued), 0)
	if avail == 0 {
		return 0, ErrAgain
	}
	frames := len(samples) / d.channels
	if frames > avail {
		frames = avail
	}
	if d.writeLimit > 0 && frames > d.writeLimit {
		frames = d.writeLimit
	}
	d.writtenSamples = append(d.writtenSamples, samples[:frames*d.channels]...)
	d.queued += int64(frames)
	return frames, nil
}

func (d *fakeDevice) Drop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dropped++
	d.queued = 0
}

func (d *fakeDevice) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed++
}

// consume simulates hardware playing n frames out of the ring buffer.
func (d *fakeDevice) consume(frames int64) {
	d.mu.Lock()
	if frames > d.queued {
		d.queued = 0
	} else {
		d.queued -= frames
	}
	d.mu.Unlock()
	select {
	case d.wakeCh <- struct{}{}:
	default:
	}
}

func (d *fakeDevice) queuedFrames() int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.queued
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal(msg)
}

func TestLatencyCandidates(t *testing.T) {
	if got := LatencyCandidates(0); !slices.Equal(got, []uint32{DefaultLatencyMicros, FallbackLatencyMicros}) {
		t.Fatalf("zero hint = %v", got)
	}
	if got := LatencyCandidates(10 * time.Millisecond); !slices.Equal(got, []uint32{10_000, DefaultLatencyMicros, FallbackLatencyMicros}) {
		t.Fatalf("10ms hint = %v", got)
	}
	if got := LatencyCandidates(time.Millisecond); !slices.Equal(got, []uint32{MinLatencyMicros, DefaultLatencyMicros, FallbackLatencyMicros}) {
		t.Fatalf("1ms clamped hint = %v", got)
	}
	if got := LatencyCandidates(2 * time.Second); !slices.Equal(got, []uint32{MaxLatencyMicros, DefaultLatencyMicros, FallbackLatencyMicros}) {
		t.Fatalf("2s clamped hint = %v", got)
	}
	if got := LatencyCandidates(40 * time.Millisecond); !slices.Equal(got, []uint32{DefaultLatencyMicros, FallbackLatencyMicros}) {
		t.Fatalf("40ms deduplicated hint = %v", got)
	}
}

func TestShortFillZeroesRemainderAndCountsUnderruns(t *testing.T) {
	dev := newFakeDevice(12, 4, 2)
	var calls atomic.Int32
	s := New(dev, driver.AudioOutputConfig{
		SampleRate: 1000,
		Channels:   2,
		Render: func(buf []float32) int {
			// Fill with sentinel non-zero values so zeroing is verified.
			for i := range buf {
				buf[i] = 9
			}
			switch calls.Add(1) {
			case 1:
				buf[0], buf[1] = 0.25, -0.25
				return 1 // short fill: 1 of 4 frames
			case 2:
				return -3 // negative clamped to 0: full silence
			default:
				for i := range buf {
					buf[i] = 0.5
				}
				return 99 // clamped to periodFrames (4)
			}
		},
	})
	defer s.Close()

	if frames, at := s.Position(); frames != 0 || !at.IsZero() || s.Latency() != 0 || calls.Load() != 0 {
		t.Fatalf("before Start: pos=%d at=%v latency=%v calls=%d", frames, at, s.Latency(), calls.Load())
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, time.Second, func() bool { return dev.queuedFrames() == 12 }, "buffer did not fill")
	s.Pause()

	if got := s.Underruns(); got != 2 {
		t.Fatalf("underruns = %d, want 2", got)
	}
	want := []float32{
		0.25, -0.25, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5,
	}
	dev.mu.Lock()
	got := slices.Clone(dev.writtenSamples)
	dev.mu.Unlock()
	if !slices.Equal(got, want) {
		t.Fatalf("written samples = %v, want %v", got, want)
	}
}

func TestPositionFlushAndPauseMonotonicity(t *testing.T) {
	dev := newFakeDevice(30, 10, 2)
	var calls atomic.Int32
	s := New(dev, driver.AudioOutputConfig{
		SampleRate: 1000,
		Channels:   2,
		Render: func(buf []float32) int {
			switch calls.Add(1) {
			case 1:
				return 10
			case 2:
				return 2 // 2 client frames + 8 silent frames
			case 3:
				return 10
			default:
				return 10
			}
		},
	})
	defer s.Close()

	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	// Buffer holds 3 periods (30 frames): client produced 10 + 2 + 10 = 22.
	waitUntil(t, time.Second, func() bool { return dev.queuedFrames() == 30 }, "initial fill did not complete")

	if frames, at := s.Position(); frames != 0 || !at.IsZero() {
		t.Fatalf("before hardware consumes frames: pos=%d at=%v", frames, at)
	}
	if lat := s.Latency(); lat != 30*time.Millisecond {
		t.Fatalf("latency = %v, want 30ms", lat)
	}

	// Hardware consumes 12 frames: period 1 (10) + first 2 frames of period 2 (both client).
	// Remaining unplayed client frames should be period 3's 10 frames!
	// Note: consuming 12 frames creates space >= periodFrames (10), so to test exact Flush
	// count we first consume 5 frames (less than a period so no 4th render fires yet!).
	dev.consume(5)
	waitUntil(t, time.Second, func() bool {
		frames, _ := s.Position()
		return frames == 5
	}, "position did not advance to 5")

	// At frame 5: period 1 has 5 left, period 2 has 2 client left, period 3 has 10 client left -> 17 pending!
	fBefore, atBefore := s.Position()
	if fBefore != 5 || atBefore.IsZero() {
		t.Fatalf("before flush: pos=%d at=%v", fBefore, atBefore)
	}
	dropped := s.Flush()
	if dropped != 17 {
		t.Fatalf("Flush dropped %d client frames, want 17", dropped)
	}
	// Position must not regress and must not count discarded frames.
	if fAfter, atAfter := s.Position(); fAfter != 5 || !atAfter.Equal(atBefore) {
		t.Fatalf("after flush: pos=%d at=%v, want 5 at %v", fAfter, atAfter, atBefore)
	}

	// Because stream was active, Flush restarted it and refilled the buffer.
	waitUntil(t, time.Second, func() bool { return dev.queuedFrames() == 30 }, "post-flush refill did not complete")
	dev.consume(8)
	waitUntil(t, time.Second, func() bool {
		frames, _ := s.Position()
		return frames == 13
	}, "position did not advance monotonically past 5 to 13")

	// Pause freezes position and stops render callbacks.
	s.Pause()
	pausedFrames, pausedAt := s.Position()
	pausedCalls := calls.Load()
	time.Sleep(25 * time.Millisecond)
	if f, at := s.Position(); f != pausedFrames || !at.Equal(pausedAt) || calls.Load() != pausedCalls {
		t.Fatalf("paused stream changed: pos=%d at=%v calls=%d (want %d at %v calls=%d)",
			f, at, calls.Load(), pausedFrames, pausedAt, pausedCalls)
	}

	// Flush while paused discards 0 and keeps stream paused.
	if d := s.Flush(); d != 0 {
		t.Fatalf("Flush while paused dropped %d, want 0", d)
	}
	if calls.Load() != pausedCalls {
		t.Fatalf("Flush while paused triggered render")
	}

	// Resume advances monotonically past pausedFrames.
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, time.Second, func() bool { return dev.queuedFrames() == 30 }, "post-resume refill did not complete")
	dev.consume(6)
	waitUntil(t, time.Second, func() bool {
		f, at := s.Position()
		return f > pausedFrames && at.After(pausedAt)
	}, "resumed stream did not advance position and timestamp")
}

func TestXRunRecovery(t *testing.T) {
	dev := newFakeDevice(20, 10, 2)
	var calls atomic.Int32
	s := New(dev, driver.AudioOutputConfig{
		SampleRate: 1000,
		Channels:   2,
		Render: func(buf []float32) int {
			calls.Add(1)
			return 10
		},
	})
	defer s.Close()

	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, time.Second, func() bool { return dev.queuedFrames() == 20 }, "initial fill did not complete")

	// Inject a hardware -EPIPE underrun on the next AvailDelay.
	dev.mu.Lock()
	dev.nextAvailErr = ErrXRun
	dev.mu.Unlock()
	select {
	case dev.wakeCh <- struct{}{}:
	default:
	}

	waitUntil(t, time.Second, func() bool { return s.Underruns() >= 1 }, "xrun was not counted")
	waitUntil(t, time.Second, func() bool {
		dev.mu.Lock()
		defer dev.mu.Unlock()
		return dev.prepared >= 2 && dev.queued == 20
	}, "xrun did not re-prepare and refill device")

	if frames, _ := s.Position(); frames < 20 {
		t.Fatalf("xrun did not commit drained frames: pos=%d", frames)
	}
}

func TestPartialWritesAndUnwrittenFlushAccounting(t *testing.T) {
	dev := newFakeDevice(20, 10, 1)
	dev.writeLimit = 4 // force partial writes within a single period
	s := New(dev, driver.AudioOutputConfig{
		SampleRate: 1000,
		Channels:   1,
		Render:     func(buf []float32) int { return len(buf) },
	})
	defer s.Close()

	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	// Each 10-frame period writes as 4 + 4 + 2 until all 20 frames fill the buffer.
	waitUntil(t, time.Second, func() bool { return dev.queuedFrames() == 20 }, "buffer did not fill via partial writes")
	if dropped := s.Flush(); dropped != 20 {
		t.Fatalf("Flush after partial writes dropped %d, want 20", dropped)
	}

	// Now stall writes with ErrAgain mid-period so 4 frames are in the device
	// and 6 client frames remain unwritten in userspace when Flush runs.
	s.Pause()
	dev.mu.Lock()
	dev.writeLimit = 4
	dev.mu.Unlock()
	stalledDev := &stallAfterOneWriteDevice{fakeDevice: dev}
	s2 := New(stalledDev, driver.AudioOutputConfig{
		SampleRate: 1000,
		Channels:   1,
		Render:     func(buf []float32) int { return 8 }, // 8 client frames + 2 silent frames
	})
	defer s2.Close()
	if err := s2.Start(); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, time.Second, func() bool { return stalledDev.stalled.Load() }, "device did not stall mid-period")
	// 4 frames accepted by device + 4 remaining client frames unwritten = 8 total client frames!
	if dropped := s2.Flush(); dropped != 8 {
		t.Fatalf("Flush mid-period dropped %d client frames, want 8", dropped)
	}
}

type stallAfterOneWriteDevice struct {
	*fakeDevice
	writes  atomic.Int32
	stalled atomic.Bool
}

func (d *stallAfterOneWriteDevice) Write(samples []float32) (int, error) {
	if d.writes.Add(1) > 1 {
		d.stalled.Store(true)
		return 0, ErrAgain
	}
	return d.fakeDevice.Write(samples)
}

func TestInterruptOnDisconnectAndSuspendAllowsRestart(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"disconnected", ErrDisconnected},
		{"suspended", ErrSuspended},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dev := newFakeDevice(20, 10, 2)
			gotErr := make(chan error, 2)
			var stream *Stream
			stream = New(dev, driver.AudioOutputConfig{
				SampleRate: 1000,
				Channels:   2,
				Render:     func(buf []float32) int { return 10 },
				Interrupt: func(err error) {
					gotErr <- err
				},
			})
			defer stream.Close()

			if err := stream.Start(); err != nil {
				t.Fatal(err)
			}
			waitUntil(t, time.Second, func() bool { return dev.queuedFrames() == 20 }, "buffer did not fill")

			dev.mu.Lock()
			dev.nextAvailErr = tc.err
			dev.mu.Unlock()
			select {
			case dev.wakeCh <- struct{}{}:
			default:
			}

			select {
			case err := <-gotErr:
				if !errors.Is(err, tc.err) {
					t.Fatalf("interrupt err = %v, want %v", err, tc.err)
				}
			case <-time.After(time.Second):
				t.Fatal("OnInterrupt did not fire")
			}

			// Start must resume cleanly after an interruption.
			if err := stream.Start(); err != nil {
				t.Fatalf("Start after interrupt failed: %v", err)
			}
			waitUntil(t, time.Second, func() bool { return dev.queuedFrames() == 20 }, "restarted stream did not refill")
		})
	}
}

func TestDelayFallbackWhenDeviceReportsNoDelay(t *testing.T) {
	dev := newFakeDevice(20, 10, 2)
	dev.delayBroken = true
	var clk atomic.Int64
	base := time.Unix(1000, 0)
	clk.Store(base.UnixNano())
	s := newWithClock(dev, driver.AudioOutputConfig{
		SampleRate: 1000,
		Channels:   2,
		Render:     func(buf []float32) int { return 10 },
	}, func() time.Time { return time.Unix(0, clk.Load()) })
	defer s.Close()

	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, time.Second, func() bool { return dev.queuedFrames() == 20 }, "buffer did not fill")

	clk.Store(base.Add(15 * time.Millisecond).UnixNano())
	waitUntil(t, time.Second, func() bool {
		f, _ := s.Position()
		return f == 15
	}, "clock fallback did not advance position to 15")

	if lat := s.Latency(); lat != 20*time.Millisecond {
		t.Fatalf("fallback latency = %v, want 20ms", lat)
	}
}

func TestCloseIsIdempotentAndRejectsStart(t *testing.T) {
	dev := newFakeDevice(20, 10, 2)
	s := New(dev, driver.AudioOutputConfig{
		SampleRate: 1000,
		Channels:   2,
		Render:     func(buf []float32) int { return 10 },
	})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, time.Second, func() bool { return dev.queuedFrames() == 20 }, "buffer did not fill")
	s.Close()
	s.Close()
	if err := s.Start(); err == nil {
		t.Fatal("Start succeeded after Close")
	}
	if dropped := s.Flush(); dropped != 0 {
		t.Fatalf("Flush after Close dropped %d, want 0", dropped)
	}
	dev.mu.Lock()
	closed := dev.closed
	dev.mu.Unlock()
	if closed != 1 {
		t.Fatalf("device Close called %d times, want 1", closed)
	}
}
