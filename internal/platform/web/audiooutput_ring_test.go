package web

import (
	"encoding/binary"
	"errors"
	"math"
	"slices"
	"testing"
	"time"
)

func TestWebPCMGeometry(t *testing.T) {
	def := computeWebPCMGeometry(48000, 2, 0)
	if def.chunkFrames%webPCMQuantum != 0 || def.targetFrames < def.chunkFrames*2 || def.ringCapFrames < def.targetFrames*4 {
		t.Fatalf("invalid default geometry: %+v", def)
	}
	if def.pumpIntervalMs < 4 || def.pumpIntervalMs > 20 {
		t.Fatalf("pump interval out of bounds: %+v", def)
	}
	low := computeWebPCMGeometry(48000, 1, 2*time.Millisecond)
	if low.chunkFrames != webPCMQuantum || low.targetFrames < 2*webPCMQuantum {
		t.Fatalf("invalid low-latency geometry: %+v", low)
	}
	high := computeWebPCMGeometry(48000, 2, 100*time.Millisecond)
	if high.chunkFrames != 4*webPCMQuantum || high.targetFrames < 4800 {
		t.Fatalf("invalid high-latency geometry: %+v", high)
	}
}

func TestWebPCMRingWrapAndFlush(t *testing.T) {
	r := newPCMRing(6, 2)
	c1 := []float32{1, -1, 2, -2, 3, -3, 4, -4}
	if n := r.write(c1, 4, 4); n != 4 {
		t.Fatalf("wrote %d frames, want 4", n)
	}
	dst := make([]float32, 8)
	frames, produced, ok := r.readChunk(dst)
	if !ok || frames != 4 || produced != 4 || !slices.Equal(dst, c1) {
		t.Fatalf("readChunk = (%d, %d, %v, %v), want (4, 4, true, %v)", frames, produced, ok, dst, c1)
	}
	// Wrap writePos around end of ring (writePos is 4, cap is 6; write 4 frames).
	c2 := []float32{5, -5, 6, -6, 0, 0, 0, 0}
	if n := r.write(c2, 4, 2); n != 4 {
		t.Fatalf("wrapped write = %d, want 4", n)
	}
	// Add 2 more frames (full).
	c3 := []float32{7, -7, 8, -8}
	if n := r.write(c3, 2, 2); n != 2 {
		t.Fatalf("fill write = %d, want 2", n)
	}
	if r.free() != 0 || r.buffered() != 6 || r.pendingClient() != 4 {
		t.Fatalf("ring state buffered=%d free=%d pendingClient=%d", r.buffered(), r.free(), r.pendingClient())
	}
	// Reading wrapped chunk should preserve samples and client-produced count.
	frames, produced, ok = r.readChunk(dst)
	if !ok || frames != 4 || produced != 2 || !slices.Equal(dst, c2) {
		t.Fatalf("wrapped readChunk = (%d, %d, %v, %v)", frames, produced, ok, dst)
	}
	// Flush remaining chunk: should report 2 client frames discarded.
	if dropped := r.flush(); dropped != 2 {
		t.Fatalf("flush dropped %d, want 2", dropped)
	}
	if r.buffered() != 0 || r.pendingClient() != 0 {
		t.Fatalf("non-empty ring after flush: buffered=%d", r.buffered())
	}
}

func TestWebPCMShortFillSilencesAndCountsUnderruns(t *testing.T) {
	st := newWebPCMState(48000, 2, 10*time.Millisecond)
	if err := st.start(); err != nil {
		t.Fatal(err)
	}
	// Pre-poison scratch buffer to verify silence zero-fill.
	for i := range st.scratch {
		st.scratch[i] = 0.99
	}
	frames, produced, underrun := st.pullIntoRing(func(buf []float32) int {
		buf[0], buf[1] = 0.5, -0.5
		return 1
	})
	if !underrun || frames != st.geom.chunkFrames || produced != 1 || st.underruns != 1 {
		t.Fatalf("short fill: frames=%d produced=%d underrun=%v count=%d", frames, produced, underrun, st.underruns)
	}
	out := make([]float32, st.geom.chunkFrames*2)
	rf, rp, ok := st.ring.readChunk(out)
	if !ok || rf != st.geom.chunkFrames || rp != 1 {
		t.Fatalf("readChunk = (%d, %d, %v)", rf, rp, ok)
	}
	if out[0] != 0.5 || out[1] != -0.5 {
		t.Fatalf("first frame = [%v, %v], want [0.5, -0.5]", out[0], out[1])
	}
	for i := 2; i < len(out); i++ {
		if out[i] != 0 {
			t.Fatalf("sample[%d] = %v, want 0 silence", i, out[i])
		}
	}

	// Negative return value clamps to 0 and counts an underrun.
	for i := range st.scratch {
		st.scratch[i] = -0.75
	}
	_, produced, underrun = st.pullIntoRing(func([]float32) int { return -5 })
	if !underrun || produced != 0 || st.underruns != 2 {
		t.Fatalf("negative render: produced=%d underrun=%v count=%d", produced, underrun, st.underruns)
	}
	// Oversized return value clamps to chunkFrames with no underrun.
	_, produced, underrun = st.pullIntoRing(func(buf []float32) int {
		for i := range buf {
			buf[i] = 0.25
		}
		return st.geom.chunkFrames + 100
	})
	if underrun || produced != st.geom.chunkFrames || st.underruns != 2 {
		t.Fatalf("oversized render: produced=%d underrun=%v count=%d", produced, underrun, st.underruns)
	}
	// Worklet underruns add to total.
	st.addWorkletUnderruns(3)
	if st.underruns != 5 {
		t.Fatalf("underruns = %d, want 5", st.underruns)
	}
}

func TestWebPCMPositionFlushPauseResume(t *testing.T) {
	st := newWebPCMState(1000, 2, 20*time.Millisecond)
	t0 := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

	// Before Start, position is 0 with zero time and latency is 0.
	if f, at := st.position(0, t0); f != 0 || !at.IsZero() {
		t.Fatalf("before start: frames=%d at=%v", f, at)
	}
	if lat := st.latency(0.01, 0.02, 0); lat != 0 {
		t.Fatalf("latency before render = %v, want 0", lat)
	}

	if err := st.start(); err != nil {
		t.Fatal(err)
	}
	// Schedule 3 slices of 10 frames each (10ms each at 1000 Hz): slice 0: [100ms, 110ms) produced 10 slice 1: [110ms, 120ms) produced 2 (short fill -> 8 frames silence) slice 2: [120ms, 130ms) produced 10
	if start := st.timeline.schedule(0.100, 10, 10); start != 0.100 {
		t.Fatalf("slice 0 start = %v, want 0.100", start)
	}
	if start := st.timeline.schedule(0.100, 10, 2); math.Abs(start-0.110) > 1e-9 {
		t.Fatalf("slice 1 start = %v, want 0.110", start)
	}
	if start := st.timeline.schedule(0.100, 10, 10); math.Abs(start-0.120) > 1e-9 {
		t.Fatalf("slice 2 start = %v, want 0.120", start)
	}

	// Before presentation time arrives (98ms): frames=0, at=zero.
	if f, at := st.position(0.098, t0.Add(98*time.Millisecond)); f != 0 || !at.IsZero() {
		t.Fatalf("at 98ms: frames=%d at=%v, want 0 zero", f, at)
	}
	// At 105ms: 5 frames played.
	f1, at1 := st.position(0.105, t0.Add(105*time.Millisecond))
	if f1 != 5 || !at1.Equal(t0.Add(105*time.Millisecond)) {
		t.Fatalf("at 105ms: frames=%d at=%v", f1, at1)
	}
	// Clock jitter backwards to 103ms: must remain monotonic!
	fJit, atJit := st.position(0.103, t0.Add(106*time.Millisecond))
	if fJit != 5 || !atJit.Equal(at1) {
		t.Fatalf("jitter went backwards: frames=%d at=%v", fJit, atJit)
	}
	// At 112ms: 12 frames played (10 from slice 0 + 2 from slice 1). Remaining client frames: 0 from slice 1 (produced=2, played=2) + 10 from slice 2 = 10. Also put 4 client frames into ring buffer.
	st.ring.write(make([]float32, 8), 4, 4)
	dropped := st.flush(0.112, t0.Add(112*time.Millisecond))
	if dropped != 14 {
		t.Fatalf("flush dropped %d, want 14 (10 scheduled + 4 ring)", dropped)
	}
	// Position right after flush must be 12 (monotonic, never counting discarded frames).
	fFlush, atFlush := st.position(0.112, t0.Add(112*time.Millisecond))
	if fFlush != 12 || atFlush.Before(at1) {
		t.Fatalf("after flush: frames=%d at=%v", fFlush, atFlush)
	}

	// Schedule new slice after flush starting at 115ms (10 frames, produced 10).
	if start := st.timeline.schedule(0.115, 10, 10); start != 0.115 {
		t.Fatalf("post-flush start = %v, want 0.115", start)
	}
	// At 120ms: 5 new frames played -> total 12 + 5 = 17.
	f2, at2 := st.position(0.120, t0.Add(120*time.Millisecond))
	if f2 != 17 || !at2.Equal(t0.Add(120*time.Millisecond)) {
		t.Fatalf("after resume at 120ms: frames=%d at=%v, want 17", f2, at2)
	}

	// Pause at 122ms -> 7 frames of new slice played (total 19), 3 frames dropped.
	if pDropped := st.pause(0.122, t0.Add(122*time.Millisecond)); pDropped != 3 {
		t.Fatalf("pause dropped %d, want 3", pDropped)
	}
	fPaused, atPaused := st.position(0.500, t0.Add(500*time.Millisecond))
	if fPaused != 19 || !atPaused.Equal(t0.Add(122*time.Millisecond)) {
		t.Fatalf("paused position moved: frames=%d at=%v, want 19 at +122ms", fPaused, atPaused)
	}

	// Resume with Start, schedule at 600ms, advance to 606ms -> 19 + 6 = 25.
	if err := st.start(); err != nil {
		t.Fatal(err)
	}
	st.timeline.schedule(0.600, 10, 10)
	f3, at3 := st.position(0.606, t0.Add(606*time.Millisecond))
	if f3 != 25 || !at3.After(atPaused) {
		t.Fatalf("resumed position: frames=%d at=%v, want 25 after %v", f3, at3, atPaused)
	}

	// Close is idempotent and prevents Start.
	if !st.close(0.606, t0.Add(606*time.Millisecond)) {
		t.Fatal("first close returned false")
	}
	if st.close(0.606, t0.Add(606*time.Millisecond)) {
		t.Fatal("second close returned true")
	}
	if err := st.start(); !errors.Is(err, errWebPCMClosed) {
		t.Fatalf("Start after Close = %v, want errWebPCMClosed", err)
	}
}

func TestWebPCMLatencyAndClock(t *testing.T) {
	st := newWebPCMState(48000, 2, 10*time.Millisecond)
	st.start()
	st.pullIntoRing(func(buf []float32) int { return len(buf) / 2 })

	// Real baseLatency + outputLatency + buffered lead.
	lat := st.latency(0.005, 0.010, 0)
	wantMin := 15*time.Millisecond + framesToDuration(int64(st.geom.chunkFrames), 48000)
	if lat < wantMin {
		t.Fatalf("latency = %v, want >= %v", lat, wantMin)
	}
	// NaN/Inf/zero base+output latencies still return positive buffer latency.
	latZero := st.latency(math.NaN(), math.Inf(1), 0)
	if latZero <= 0 {
		t.Fatalf("latency with NaN/Inf hardware values = %v, want > 0", latZero)
	}

	// Clock resolution: prefers valid getOutputTimestamp().
	wall := time.Date(2026, 11, 21, 12, 0, 1, 0, time.UTC)
	sec, ref := webOutputClock(1.050, 1.035, 995.0, 1000.0, 0.010, wall)
	if sec != 1.035 || !ref.Equal(wall.Add(-5*time.Millisecond)) {
		t.Fatalf("getOutputTimestamp clock = (%v, %v)", sec, ref)
	}
	// Future performanceTime is clamped so ref never exceeds wall.
	sec, ref = webOutputClock(1.050, 1.035, 1010.0, 1000.0, 0.010, wall)
	if sec != 1.035 || !ref.Equal(wall) {
		t.Fatalf("future performanceTime not clamped: (%v, %v)", sec, ref)
	}
	// Fallback to currentTime - outputLatency when getOutputTimestamp returns 0.
	sec, ref = webOutputClock(1.050, 0, 0, 1000.0, 0.012, wall)
	if math.Abs(sec-1.038) > 1e-9 || !ref.Equal(wall) {
		t.Fatalf("fallback clock = (%v, %v), want (1.038, %v)", sec, ref, wall)
	}
}

func TestWebPCMContextStateInterruptions(t *testing.T) {
	st := newWebPCMState(48000, 2, 10*time.Millisecond)
	st.start()

	// Initial suspended before running (autoplay policy) must NOT trigger OnInterrupt.
	if intr, err := st.onContextState("suspended"); intr || err != nil {
		t.Fatalf("initial suspended triggered interrupt: %v, %v", intr, err)
	}
	// Transition to running marks seenRunning.
	if intr, err := st.onContextState("running"); intr || err != nil {
		t.Fatalf("running triggered interrupt: %v, %v", intr, err)
	}
	// Unexpected suspended after running triggers OnInterrupt.
	if intr, err := st.onContextState("suspended"); !intr || err == nil {
		t.Fatalf("unexpected suspended did not trigger interrupt: %v, %v", intr, err)
	}
	// Interrupted triggers OnInterrupt even before seenRunning.
	st2 := newWebPCMState(48000, 2, 10*time.Millisecond)
	st2.start()
	if intr, err := st2.onContextState("interrupted"); !intr || err == nil {
		t.Fatalf("interrupted did not trigger interrupt: %v, %v", intr, err)
	}
	// Expected Pause suppresses suspended statechange.
	st.pause(0, time.Now())
	if intr, err := st.onContextState("suspended"); intr || err != nil {
		t.Fatalf("expected pause triggered interrupt: %v, %v", intr, err)
	}
	// Close suppresses closed statechange.
	st.close(0, time.Now())
	if intr, err := st.onContextState("closed"); intr || err != nil {
		t.Fatalf("expected close triggered interrupt: %v, %v", intr, err)
	}
}

func TestWebPCMPrebufferThenDrain(t *testing.T) {
	st := newWebPCMState(48000, 2, 10*time.Millisecond)
	st.start()

	// Simulate Start() pumping while AudioWorklet addModule is still pending.
	calls := 0
	needed := st.neededChunks(0)
	if needed < 2 {
		t.Fatalf("neededChunks = %d, want >= 2", needed)
	}
	for range needed {
		st.pullIntoRing(func(buf []float32) int {
			calls++
			for i := range buf {
				buf[i] = float32(calls) * 0.1
			}
			return len(buf) / 2
		})
	}
	if st.neededChunks(0) != 0 {
		t.Fatalf("expected ring to satisfy targetFrames, still needed %d", st.neededChunks(0))
	}
	// Now AudioWorklet finishes loading at t=0.050s: drain ring into timeline.
	dst := make([]float32, st.geom.chunkFrames*2)
	drained := 0
	for {
		frames, produced, ok := st.ring.readChunk(dst)
		if !ok {
			break
		}
		st.timeline.schedule(0.050, int64(frames), int64(produced))
		drained += frames
	}
	if drained != needed*st.geom.chunkFrames {
		t.Fatalf("drained %d frames, want %d", drained, needed*st.geom.chunkFrames)
	}
	// Halfway through the drained audio, position should advance accurately.
	halfSec := 0.050 + float64(drained/2)/48000.0
	f, at := st.position(halfSec, time.Now())
	if f != int64(drained/2) || at.IsZero() {
		t.Fatalf("halfway position = (%d, %v), want %d", f, at, drained/2)
	}
}

func TestWebPCMEncodeFloat32LE(t *testing.T) {
	interleaved := []float32{0.5, -0.25, 1.0, -1.0}
	raw := make([]byte, len(interleaved)*4)
	encodeInterleavedFloat32LE(raw, interleaved)
	for i, want := range interleaved {
		got := math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
		if got != want {
			t.Fatalf("interleaved[%d] = %v, want %v", i, got, want)
		}
	}

	left := make([]byte, 2*4)
	right := make([]byte, 2*4)
	encodePlanarChannelFloat32LE(left, interleaved, 0, 2, 2)
	encodePlanarChannelFloat32LE(right, interleaved, 1, 2, 2)
	if math.Float32frombits(binary.LittleEndian.Uint32(left[0:])) != 0.5 ||
		math.Float32frombits(binary.LittleEndian.Uint32(left[4:])) != 1.0 {
		t.Fatalf("unexpected left planar encoding")
	}
	if math.Float32frombits(binary.LittleEndian.Uint32(right[0:])) != -0.25 ||
		math.Float32frombits(binary.LittleEndian.Uint32(right[4:])) != -1.0 {
		t.Fatalf("unexpected right planar encoding")
	}
}
