package windows

import (
	"math"
	"slices"
	"testing"
	"time"
	"unsafe"
)

func TestWASAPIWaveFormatLayoutAndFields(t *testing.T) {
	var f waveFormatExtensible
	if got := unsafe.Sizeof(f); got != 40 {
		t.Fatalf("sizeof(waveFormatExtensible) = %d, want 40", got)
	}
	if got := unsafe.Offsetof(f.cbSize); got != 16 {
		t.Fatalf("offsetof(cbSize) = %d, want 16", got)
	}
	if got := unsafe.Offsetof(f.validBitsPerSample); got != 18 {
		t.Fatalf("offsetof(validBitsPerSample) = %d, want 18", got)
	}
	if got := unsafe.Offsetof(f.channelMask); got != 20 {
		t.Fatalf("offsetof(channelMask) = %d, want 20", got)
	}
	if got := unsafe.Offsetof(f.subFormat); got != 24 {
		t.Fatalf("offsetof(subFormat) = %d, want 24", got)
	}

	stereo := wasapiWaveFormat(48000, 2)
	if stereo.formatTag != waveFormatExtensibleTag ||
		stereo.channels != 2 ||
		stereo.samplesPerSec != 48000 ||
		stereo.blockAlign != 8 ||
		stereo.avgBytesPerSec != 384000 ||
		stereo.bitsPerSample != 32 ||
		stereo.cbSize != 22 ||
		stereo.validBitsPerSample != 32 ||
		stereo.channelMask != speakerStereoMask ||
		stereo.subFormat != ksDataFormatSubtypeIEEEFloat {
		t.Fatalf("unexpected stereo format: %+v", stereo)
	}

	mono := wasapiWaveFormat(44100, 1)
	if mono.channels != 1 || mono.blockAlign != 4 || mono.avgBytesPerSec != 176400 || mono.channelMask != speakerMonoMask {
		t.Fatalf("unexpected mono format: %+v", mono)
	}
}

func TestWASAPIBufferDurationAndLatency(t *testing.T) {
	if got := wasapiBufferDuration100ns(0); got != 0 {
		t.Errorf("0 latency -> %d, want 0", got)
	}
	if got := wasapiBufferDuration100ns(-time.Millisecond); got != 0 {
		t.Errorf("negative latency -> %d, want 0", got)
	}
	if got := wasapiBufferDuration100ns(50 * time.Nanosecond); got != 1 {
		t.Errorf("50ns latency -> %d, want 1", got)
	}
	if got := wasapiBufferDuration100ns(10 * time.Millisecond); got != 100_000 {
		t.Errorf("10ms latency -> %d, want 100000", got)
	}

	// 480 frames at 48 kHz = 10ms buffer + 5ms (50_000 * 100ns) engine latency = 15ms.
	if got := wasapiLatency(480, 48000, 50_000); got != 15*time.Millisecond {
		t.Errorf("wasapiLatency = %v, want 15ms", got)
	}
	if got := wasapiLatency(960, 48000, 0); got != 20*time.Millisecond {
		t.Errorf("wasapiLatency(zero stream) = %v, want 20ms", got)
	}
	if got := wasapiLatency(960, 48000, -100); got != 20*time.Millisecond {
		t.Errorf("wasapiLatency(negative stream) = %v, want 20ms", got)
	}
}

func TestWASAPIFillSilencesUnderrunAndClamps(t *testing.T) {
	s := newWASAPIState(48000, 2)

	// Short render: 1 of 3 frames filled -> remainder zeroed and 1 underrun counted.
	buf := []float32{1, 1, 1, 1, 1, 1}
	frames, produced, underrun := s.fillAndRecord(buf, func(out []float32) int {
		out[0], out[1] = 0.25, -0.25
		return 1
	})
	if frames != 3 || produced != 1 || !underrun || s.underruns != 1 {
		t.Fatalf("short fill: frames=%d produced=%d underrun=%v count=%d", frames, produced, underrun, s.underruns)
	}
	if want := []float32{0.25, -0.25, 0, 0, 0, 0}; !slices.Equal(buf, want) {
		t.Fatalf("short fill buf = %v, want %v", buf, want)
	}

	// Negative return value clamps to 0, silences whole buffer, counts underrun.
	buf2 := []float32{0.5, 0.5, 0.5, 0.5}
	frames, produced, underrun = s.fillAndRecord(buf2, func([]float32) int { return -5 })
	if frames != 2 || produced != 0 || !underrun || s.underruns != 2 {
		t.Fatalf("negative fill: frames=%d produced=%d underrun=%v count=%d", frames, produced, underrun, s.underruns)
	}
	if want := []float32{0, 0, 0, 0}; !slices.Equal(buf2, want) {
		t.Fatalf("negative fill buf = %v, want %v", buf2, want)
	}

	// Overshoot return clamps to frames with no underrun.
	buf3 := []float32{0, 0, 0, 0}
	frames, produced, underrun = s.fillAndRecord(buf3, func(out []float32) int {
		out[0], out[1], out[2], out[3] = 0.1, 0.2, 0.3, 0.4
		return 99
	})
	if frames != 2 || produced != 2 || underrun || s.underruns != 2 {
		t.Fatalf("overshoot fill: frames=%d produced=%d underrun=%v count=%d", frames, produced, underrun, s.underruns)
	}
	// Pending client-produced frames = 1 (chunk1) + 0 (chunk2) + 2 (chunk3) = 3 (silence excluded).
	if got := s.pendingFrames(); got != 3 {
		t.Fatalf("pendingFrames = %d, want 3", got)
	}
}

func TestWASAPIPositionClockFlushAndMonotonicity(t *testing.T) {
	s := newWASAPIState(48000, 2)
	const devFreq = 384000     // e.g. 48000 * 8 bytes/frame
	const qpcFreq = 10_000_000 // 10 MHz QPC (1 tick = 100ns)
	t0 := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

	// Before any frame reaches the device, Position stays (0, zero).
	if frames, at := s.sampleClock(0, devFreq, 1_000_000, 1_000_000, qpcFreq, t0); frames != 0 || !at.IsZero() {
		t.Fatalf("initial clock: frames=%d at=%v, want 0 and zero time", frames, at)
	}

	// Submit three chunks: 480 full, 480 with 100 produced (underrun), 480 full.
	s.recordSubmission(480, 480)
	s.recordSubmission(480, 100)
	s.recordSubmission(480, 480)

	// Device clock reaches 500 frames (480 of chunk 1 + 20 of chunk 2), QPC 2ms before wallNow.
	t1 := t0.Add(20 * time.Millisecond)
	qpcNow := uint64(2_000_000)
	qpcPos100ns := uint64(1_980_000) // 2ms before qpcNow
	frames, at := s.sampleClock(500*8, devFreq, qpcPos100ns, qpcNow, qpcFreq, t1)
	if frames != 500 {
		t.Fatalf("frames = %d, want 500", frames)
	}
	if wantAt := t1.Add(-2 * time.Millisecond); !at.Equal(wantAt) {
		t.Fatalf("at = %v, want %v", at, wantAt)
	}

	// Backward clock jitter or future QPC never regresses Position or produces a future time.
	frames, at = s.sampleClock(490*8, devFreq, qpcNow+50_000, qpcNow, qpcFreq, t1)
	if frames != 500 || !at.Equal(t1.Add(-2*time.Millisecond)) {
		t.Fatalf("jitter moved position to %d at %v", frames, at)
	}

	// Flush discards 80 remaining client frames in chunk 2 + 480 in chunk 3 = 560.
	dropped := s.resetRun()
	if dropped != 560 {
		t.Fatalf("Flush dropped %d, want 560", dropped)
	}
	if f, a := s.position(); f != 500 || !a.Equal(t1.Add(-2*time.Millisecond)) {
		t.Fatalf("after resetRun: position = %d at %v", f, a)
	}
	// A second reset while already empty drops 0.
	if again := s.resetRun(); again != 0 {
		t.Fatalf("second resetRun dropped %d, want 0", again)
	}

	// After IAudioClient.Reset, hardware clock restarts at 0, then advances to 240. Monotonic position must continue from base=500 -> 740.
	t2 := t1.Add(15 * time.Millisecond)
	if f, a := s.sampleClock(0, devFreq, 3_000_000, 3_000_000, qpcFreq, t2); f != 500 || !a.Equal(t1.Add(-2*time.Millisecond)) {
		t.Fatalf("zero after reset changed position: %d at %v", f, a)
	}
	s.recordSubmission(480, 480)
	f, a := s.sampleClock(240*8, devFreq, 3_010_000, 3_000_000, qpcFreq, t2) // QPC slightly ahead -> clamped to t2
	if f != 740 || !a.Equal(t2) {
		t.Fatalf("resumed position = %d at %v, want 740 at %v", f, a, t2)
	}
	if p := s.pendingFrames(); p != 240 {
		t.Fatalf("pending after resume = %d, want 240", p)
	}
}

func TestWASAPIPendingExcludesSilenceTail(t *testing.T) {
	s := newWASAPIState(48000, 2)
	s.recordSubmission(480, 100) // 100 client frames + 380 silence frames
	s.recordSubmission(480, 480)
	// Clock moved to 200 frames: all 100 client frames of chunk 1 already played.
	s.sampleClock(200, 48000, 0, 0, 0, time.Now())
	if got := s.pendingFrames(); got != 480 {
		t.Fatalf("pendingFrames = %d, want 480", got)
	}
}

func TestWASAPIScaleOverflowAndQPCFallbacks(t *testing.T) {
	// Large values that would overflow a naive uint64 multiplication.
	const freq = 10_000_000
	const ticks = uint64(1) << 50
	got := scaleTicksToFrames(ticks, 48000, freq)
	want := int64((ticks/freq)*48000 + ((ticks % freq) * 48000 / freq))
	if got != want {
		t.Fatalf("scaleTicksToFrames large = %d, want %d", got, want)
	}
	if got := scaleTicksToFrames(math.MaxUint64, 48000, 1); got != math.MaxInt64 {
		t.Fatalf("saturating scale = %d, want MaxInt64", got)
	}

	wall := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	if at := qpcToWallTime(0, 1000, 10_000_000, wall); !at.Equal(wall) {
		t.Fatalf("zero qpcPos = %v, want %v", at, wall)
	}
	if at := qpcToWallTime(1, 500_000_000, 10_000_000, wall); !at.Equal(wall) {
		t.Fatalf("excessive skew = %v, want %v", at, wall)
	}
}

func TestWASAPIDeviceErrorClassification(t *testing.T) {
	for _, hr := range []uintptr{
		uintptr(audclntEDeviceInvalidated),
		uintptr(audclntEEndpointCreateFailed),
		uintptr(audclntEServiceNotRunning),
		uintptr(audclntEResourcesInvalidated),
	} {
		if !wasapiIsDeviceError(hr) {
			t.Errorf("wasapiIsDeviceError(%#x) = false, want true", uint32(hr))
		}
	}
	if wasapiIsDeviceError(0) || wasapiIsDeviceError(0x80070057) {
		t.Error("non-device HRESULT classified as device error")
	}
}
