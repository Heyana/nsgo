package android

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func TestAudioOutputProtocolMatchesJava(t *testing.T) {
	testhost.RequireSource(t)
	java, err := os.ReadFile(filepath.Join("java", "dev", "oneo", "nsgo", "Protocol.java"))
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]int{}
	for _, m := range regexp.MustCompile(`static final int (\w+) = (\d+);`).FindAllStringSubmatch(string(java), -1) {
		values[m[1]], _ = strconv.Atoi(m[2])
	}
	for name, want := range map[string]int{
		"PCM_OPEN":        pcmOpen,
		"PCM_START":       pcmStart,
		"PCM_PAUSE":       pcmPause,
		"PCM_CLOSE":       pcmClose,
		"PCM_DATA":        pcmData,
		"PCM_REPLY":       pcmReply,
		"PCM_PULL":        pcmPull,
		"PCM_INTERRUPTED": pcmInterrupted,
	} {
		if got, ok := values[name]; !ok || got != want {
			t.Errorf("Protocol.java's %s is %d (found %v), Go's %d", name, got, ok, want)
		}
	}
}

func TestPCMTrackerPositionFlushAndUnderruns(t *testing.T) {
	var tr pcmTracker
	t0 := time.Unix(1000, 0)
	tr.push(10, 10)
	tr.push(10, 2) // 2 client frames + 8 silence frames (underrun)
	tr.push(10, 10)

	if frames, at := tr.position(); frames != 0 || !at.IsZero() {
		t.Fatalf("before advance: got %d at %v", frames, at)
	}

	tr.advance(5, t0.Add(5*time.Millisecond))
	if frames, at := tr.position(); frames != 5 || !at.Equal(t0.Add(5*time.Millisecond)) {
		t.Fatalf("after 5 frames: got %d at %v", frames, at)
	}

	// Backward hardware report never moves Position back.
	tr.advance(3, t0.Add(3*time.Millisecond))
	if frames, at := tr.position(); frames != 5 || !at.Equal(t0.Add(5*time.Millisecond)) {
		t.Fatalf("after backward report: got %d at %v", frames, at)
	}

	// Flush at frame 12: chunk 0 fully played (0 pending), chunk 1 played 2 of 2 produced (0 pending), chunk 2 played 0 of 10 produced (10 pending).
	dropped := tr.finishGeneration(12, t0.Add(12*time.Millisecond))
	if dropped != 10 {
		t.Fatalf("Flush dropped %d client frames, want 10", dropped)
	}
	if frames, at := tr.position(); frames != 12 || !at.Equal(t0.Add(12*time.Millisecond)) {
		t.Fatalf("after Flush: got %d at %v, want 12", frames, at)
	}

	// Next generation starts at base 12; discarded frames are never counted.
	tr.push(8, 8)
	tr.advance(4, t0.Add(20*time.Millisecond))
	if frames, at := tr.position(); frames != 16 || !at.Equal(t0.Add(20*time.Millisecond)) {
		t.Fatalf("next gen: got %d at %v, want 16", frames, at)
	}

	// Flush before chunk finishes: 4 remaining client frames dropped.
	if dropped := tr.finishGeneration(4, t0.Add(20*time.Millisecond)); dropped != 4 {
		t.Fatalf("second Flush dropped %d, want 4", dropped)
	}
}

// fakeAndroidPCM simulates AudioOutput.java over the real binary wire protocol.
type fakeAndroidPCM struct {
	mgr          *audioOutputManager
	bufferFrames uint32
	chunkFrames  uint32
	latencyNanos int64

	mu             sync.Mutex
	active         bool
	streamID       uint32
	gen            uint32
	sampleRate     int
	channels       int
	writtenInGen   int64
	playedInGen    int64
	keepFocusSeen  []bool
	closed         bool
	receivedChunks [][]float32
	onData         func(chunkIndex int, frames int64)
}

func newFakeAndroidPCM(bufferFrames, chunkFrames uint32, latencyNanos int64) (*audioOutputManager, *fakeAndroidPCM) {
	mgr := &audioOutputManager{}
	f := &fakeAndroidPCM{
		mgr:          mgr,
		bufferFrames: bufferFrames,
		chunkFrames:  chunkFrames,
		latencyNanos: latencyNanos,
	}
	mgr.init(func(body func(*writer)) error {
		var w writer
		body(&w)
		f.handle(w.buf)
		return nil
	})
	return mgr, f
}

func (f *fakeAndroidPCM) sendReport(body func(*writer)) {
	var w writer
	body(&w)
	r := reader{w.buf}
	f.mgr.report(&r)
}

func (f *fakeAndroidPCM) sendPull(id, gen uint32, requested uint32, played, offsetNanos, latencyNanos int64) {
	f.sendReport(func(w *writer) {
		w.byte(pcmPull)
		w.u32(id)
		w.u32(gen)
		w.u32(requested)
		w.u64(uint64(played))
		w.u64(uint64(offsetNanos))
		w.u64(uint64(latencyNanos))
	})
}

func (f *fakeAndroidPCM) sendInterrupt(id, gen uint32, played, offsetNanos int64, reason string) {
	f.mu.Lock()
	if f.active && f.streamID == id && f.gen == gen {
		f.active = false
		f.playedInGen = played
	}
	f.mu.Unlock()
	f.sendReport(func(w *writer) {
		w.byte(pcmInterrupted)
		w.u32(id)
		w.u32(gen)
		w.u64(uint64(played))
		w.u64(uint64(offsetNanos))
		w.str(reason)
	})
}

func (f *fakeAndroidPCM) handle(payload []byte) {
	r := reader{payload}
	op := r.byte()
	switch op {
	case pcmOpen:
		id, req := r.u32(), r.u32()
		rate, ch := int(r.u32()), int(r.byte())
		_ = r.u64()
		f.mu.Lock()
		f.streamID = id
		f.sampleRate = rate
		f.channels = ch
		f.mu.Unlock()
		go f.sendReport(func(w *writer) {
			w.byte(pcmReply)
			w.u32(id)
			w.u32(req)
			w.str("")
			w.u32(f.bufferFrames)
			w.u32(f.chunkFrames)
			w.u64(0)
			w.u64(0)
			w.u64(uint64(f.latencyNanos))
		})
	case pcmStart:
		id, req, gen := r.u32(), r.u32(), r.u32()
		f.mu.Lock()
		f.active = true
		f.gen = gen
		f.writtenInGen = 0
		f.playedInGen = 0
		f.mu.Unlock()
		go func() {
			f.sendReport(func(w *writer) {
				w.byte(pcmReply)
				w.u32(id)
				w.u32(req)
				w.str("")
				w.u32(f.bufferFrames)
				w.u32(f.chunkFrames)
				w.u64(0)
				w.u64(0)
				w.u64(uint64(f.latencyNanos))
			})
			f.sendPull(id, gen, f.chunkFrames, 0, 0, f.latencyNanos)
		}()
	case pcmPause:
		id, req, _ := r.u32(), r.u32(), r.u32()
		keepFocus := r.byte()&1 != 0
		f.mu.Lock()
		f.active = false
		f.keepFocusSeen = append(f.keepFocusSeen, keepFocus)
		played := f.playedInGen
		f.mu.Unlock()
		go f.sendReport(func(w *writer) {
			w.byte(pcmReply)
			w.u32(id)
			w.u32(req)
			w.str("")
			w.u32(f.bufferFrames)
			w.u32(f.chunkFrames)
			w.u64(uint64(played))
			w.u64(0)
			w.u64(uint64(f.latencyNanos))
		})
	case pcmClose:
		id, req := r.u32(), r.u32()
		f.mu.Lock()
		f.active = false
		f.closed = true
		f.mu.Unlock()
		go f.sendReport(func(w *writer) {
			w.byte(pcmReply)
			w.u32(id)
			w.u32(req)
			w.str("")
			w.u32(0)
			w.u32(0)
			w.u64(0)
			w.u64(0)
			w.u64(0)
		})
	case pcmData:
		id, gen, frames := r.u32(), r.u32(), int64(r.u32())
		raw := r.bytes()
		samples := make([]float32, len(raw)/4)
		for i := range samples {
			samples[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
		}
		f.mu.Lock()
		if !f.active || f.streamID != id || f.gen != gen {
			f.mu.Unlock()
			return
		}
		f.receivedChunks = append(f.receivedChunks, samples)
		idx := len(f.receivedChunks) - 1
		f.writtenInGen += frames
		cb := f.onData
		f.mu.Unlock()
		if cb != nil {
			cb(idx, frames)
		}
	}
}

func TestAudioOutputRenderShortFillTimingFlushAndTeardown(t *testing.T) {
	const (
		rate         = 48000
		channels     = 2
		chunkFrames  = 4
		bufferFrames = 16
	)
	const wantLatency = 20 * time.Millisecond
	mgr, fake := newFakeAndroidPCM(bufferFrames, chunkFrames, int64(wantLatency))

	now := time.Unix(2000, 0)
	var clockMu sync.Mutex
	mgr.now = func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		return now
	}
	advanceClock := func(d time.Duration) {
		clockMu.Lock()
		now = now.Add(d)
		clockMu.Unlock()
	}

	chunkDone := make(chan int, 16)
	fake.onData = func(idx int, _ int64) {
		chunkDone <- idx
	}

	var calls atomic.Int32
	out, err := mgr.open(driver.AudioOutputConfig{
		SampleRate: rate,
		Channels:   channels,
		Latency:    10 * time.Millisecond,
		Render: func(buf []float32) int {
			// Fill every slot with sentinel first so we verify silence overwrite.
			for i := range buf {
				buf[i] = 9.0
			}
			n := calls.Add(1)
			switch n {
			case 1:
				// Short fill: 1 frame of 4 -> remainder must be zeroed and underrun counted.
				buf[0], buf[1] = 0.25, -0.25
				return 1
			case 2:
				// Full fill: 4 frames.
				for i := 0; i < len(buf); i += 2 {
					buf[i], buf[i+1] = 0.5, -0.5
				}
				return len(buf) / channels
			default:
				// Subsequent renders after Flush.
				for i := 0; i < len(buf); i += 2 {
					buf[i], buf[i+1] = 0.75, -0.75
				}
				return len(buf) / channels
			}
		},
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// Opened paused: zero position, zero latency, no render callbacks yet.
	if frames, at := out.Position(); frames != 0 || !at.IsZero() || out.Latency() != 0 || calls.Load() != 0 {
		t.Fatalf("before Start: pos=%d at=%v latency=%v calls=%d", frames, at, out.Latency(), calls.Load())
	}

	if err := out.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for chunk 0 (short fill).
	select {
	case idx := <-chunkDone:
		if idx != 0 {
			t.Fatalf("chunk index %d, want 0", idx)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first chunk")
	}

	fake.mu.Lock()
	gotChunk0 := slices.Clone(fake.receivedChunks[0])
	streamID, gen := fake.streamID, fake.gen
	fake.mu.Unlock()
	wantChunk0 := []float32{0.25, -0.25, 0, 0, 0, 0, 0, 0}
	if !slices.Equal(gotChunk0, wantChunk0) {
		t.Fatalf("chunk 0 samples = %v, want %v", gotChunk0, wantChunk0)
	}
	if out.Underruns() != 1 {
		t.Fatalf("Underruns() = %d, want 1", out.Underruns())
	}
	if out.Latency() != wantLatency {
		t.Fatalf("Latency() = %v, want %v", out.Latency(), wantLatency)
	}

	// Device plays 2 frames of chunk 0 and requests chunk 1, with a slightly positive offset (clamped to <= now).
	advanceClock(10 * time.Millisecond)
	fake.mu.Lock()
	fake.playedInGen = 2
	fake.mu.Unlock()
	fake.sendPull(streamID, gen, chunkFrames, 2, int64(5*time.Millisecond), int64(25*time.Millisecond))

	select {
	case idx := <-chunkDone:
		if idx != 1 {
			t.Fatalf("chunk index %d, want 1", idx)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second chunk")
	}

	frames, at := out.Position()
	if frames != 2 || at.After(mgr.clock()) || at.IsZero() {
		t.Fatalf("Position after 2 played frames = (%d, %v), clock=%v", frames, at, mgr.clock())
	}
	if out.Latency() != 25*time.Millisecond {
		t.Fatalf("updated Latency() = %v, want 25ms", out.Latency())
	}

	// Hardware reports 3 played frames total (still inside chunk 0's silence; all 4 client frames of chunk 1 are pending).
	advanceClock(5 * time.Millisecond)
	fake.mu.Lock()
	fake.playedInGen = 3
	fake.mu.Unlock()
	dropped := out.Flush()
	if dropped != 4 {
		t.Fatalf("Flush() dropped %d client frames, want 4", dropped)
	}
	// Position advanced monotonically to 3 and did NOT count the 4 discarded frames.
	framesAfterFlush, atAfterFlush := out.Position()
	if framesAfterFlush != 3 || atAfterFlush.Before(at) {
		t.Fatalf("Position after Flush = (%d, %v), prev=(%d, %v)", framesAfterFlush, atAfterFlush, frames, at)
	}

	// Flush restarted the stream and triggered chunk 2 in the new generation.
	select {
	case idx := <-chunkDone:
		if idx != 2 {
			t.Fatalf("chunk index after Flush = %d, want 2", idx)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for chunk after Flush")
	}

	// Play 4 frames in the new generation -> Position must be 3 + 4 = 7.
	advanceClock(10 * time.Millisecond)
	fake.mu.Lock()
	newGen := fake.gen
	fake.playedInGen = 4
	fake.mu.Unlock()
	fake.sendPull(streamID, newGen, 0, 4, int64(-1*time.Millisecond), int64(20*time.Millisecond))
	frames2, at2 := out.Position()
	if frames2 != 7 || !at2.After(atAfterFlush) {
		t.Fatalf("Position after resume = (%d, %v), want 7 after %v", frames2, at2, atAfterFlush)
	}

	// Pause freezes position and stops further renders.
	out.Pause()
	pausedFrames, pausedAt := out.Position()
	pausedCalls := calls.Load()
	// Stale pull from old generation must be ignored while paused.
	fake.sendPull(streamID, newGen, chunkFrames, 99, 0, int64(99*time.Millisecond))
	time.Sleep(15 * time.Millisecond)
	if f3, a3 := out.Position(); f3 != pausedFrames || !a3.Equal(pausedAt) || calls.Load() != pausedCalls {
		t.Fatalf("paused stream changed: pos=(%d,%v) calls=%d", f3, a3, calls.Load())
	}
	// Flush while paused discards 0 and stays paused.
	if d := out.Flush(); d != 0 {
		t.Fatalf("Flush while paused returned %d, want 0", d)
	}

	// Verify Flush requested keepFocus=true and Pause requested keepFocus=false.
	fake.mu.Lock()
	keepFocus := slices.Clone(fake.keepFocusSeen)
	fake.mu.Unlock()
	if !slices.Equal(keepFocus, []bool{true, false}) {
		t.Fatalf("keepFocus history = %v, want [true false]", keepFocus)
	}

	// Close is idempotent, releases the Android stream, and prevents Start.
	out.Close()
	out.Close()
	fake.mu.Lock()
	wasClosed := fake.closed
	fake.mu.Unlock()
	if !wasClosed {
		t.Fatal("Close did not send pcmClose")
	}
	if err := out.Start(); err == nil {
		t.Fatal("Start succeeded after Close")
	}
}

func TestAudioOutputInterruptionAndResume(t *testing.T) {
	mgr, fake := newFakeAndroidPCM(16, 4, int64(15*time.Millisecond))
	chunkDone := make(chan struct{}, 8)
	fake.onData = func(int, int64) { chunkDone <- struct{}{} }

	interrupts := make(chan error, 4)
	out, err := mgr.open(driver.AudioOutputConfig{
		SampleRate: 48000,
		Channels:   1,
		Render: func(buf []float32) int {
			for i := range buf {
				buf[i] = 0.1
			}
			return len(buf)
		},
		Interrupt: func(err error) {
			interrupts <- err
		},
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer out.Close()

	if err := out.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-chunkDone

	fake.mu.Lock()
	id, gen := fake.streamID, fake.gen
	fake.mu.Unlock()

	// Audio focus loss interrupts and pauses the stream.
	fake.sendInterrupt(id, gen, 2, 0, "audio focus lost")
	select {
	case err := <-interrupts:
		if err == nil || !strings.Contains(err.Error(), "audio focus lost") {
			t.Fatalf("interrupt error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for focus loss interruption")
	}
	framesAfterFocusLoss, _ := out.Position()
	if framesAfterFocusLoss != 2 {
		t.Fatalf("position after focus loss = %d, want 2", framesAfterFocusLoss)
	}

	// Duplicate/stale interruption for the same generation must not fire again.
	fake.sendInterrupt(id, gen, 2, 0, "duplicate")
	select {
	case err := <-interrupts:
		t.Fatalf("unexpected duplicate interrupt: %v", err)
	case <-time.After(30 * time.Millisecond):
	}

	// Start resumes output on a new generation, and route change interrupts it too.
	if err := out.Start(); err != nil {
		t.Fatalf("Start after interrupt: %v", err)
	}
	<-chunkDone

	fake.mu.Lock()
	gen2 := fake.gen
	fake.mu.Unlock()
	if gen2 == gen {
		t.Fatal("expected new generation on Start after interrupt")
	}

	fake.sendInterrupt(id, gen2, 3, 0, "audio output route changed")
	select {
	case err := <-interrupts:
		if err == nil || !strings.Contains(err.Error(), "route changed") {
			t.Fatalf("route change error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for route change interruption")
	}
	if frames, _ := out.Position(); frames != 5 {
		t.Fatalf("position after second interrupt = %d, want 5", frames)
	}
}

func TestAudioOutputPipeDisconnectDoesNotHang(t *testing.T) {
	mgr := &audioOutputManager{}
	started := make(chan struct{})
	mgr.init(func(body func(*writer)) error {
		close(started)
		return nil
	})
	errCh := make(chan error, 1)
	go func() {
		_, err := mgr.open(driver.AudioOutputConfig{
			SampleRate: 48000,
			Channels:   2,
			Render:     func([]float32) int { return 0 },
		})
		errCh <- err
	}()
	<-started
	mgr.closeAll()
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error when runtime disconnects")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("open hung after closeAll")
	}
}
