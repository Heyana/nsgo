//go:build darwin || windows || (linux && !android)

package microphone

import (
	"encoding/binary"
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type fakeLocalCapture struct {
	mu          sync.Mutex
	rate        int
	channels    int
	openErr     error
	onSamples   func([]int16)
	onInterrupt func(bool)
	onError     func(error)
	stopped     atomic.Bool
	resumed     atomic.Int32
}

func (f *fakeLocalCapture) emit(samples []int16) {
	f.mu.Lock()
	cb := f.onSamples
	f.mu.Unlock()
	if cb != nil {
		cb(samples)
	}
}

func (f *fakeLocalCapture) interrupt(silenced bool) {
	f.mu.Lock()
	cb := f.onInterrupt
	f.mu.Unlock()
	if cb != nil {
		cb(silenced)
	}
}

func withLocalRecorder(t *testing.T, h localRecorderHooks) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "recordings")
	prevDir := driver.RecordingsDir()
	driver.SetRecordingsDir(dir)
	if h.authorize == nil {
		h.authorize = func() error { return nil }
	}
	if h.freeSpace == nil {
		h.freeSpace = func(string) int64 { return 1 << 30 }
	}
	if h.maxData == 0 {
		h.maxData = uint64(driver.MaxWAVDataBytes)
	}
	if h.reserve == 0 {
		h.reserve = diskSpaceReserve
	}
	localRec.mu.Lock()
	prevHooks := localRec.hooks
	localRec.hooks = &h
	localRec.mu.Unlock()
	t.Cleanup(func() {
		localRec.mu.Lock()
		localRec.hooks = prevHooks
		localRec.active = nil
		localRec.mu.Unlock()
		driver.SetRecordingsDir(prevDir)
	})
	return dir
}

func fakeOpener(fc *fakeLocalCapture) func(int, int, func([]int16), func(bool), func(error)) (*captureSession, error) {
	return func(rate, channels int, onSamples func([]int16), onInterrupt func(bool), onError func(error)) (*captureSession, error) {
		if fc.openErr != nil {
			return nil, fc.openErr
		}
		actualRate := fc.rate
		if actualRate == 0 {
			actualRate = rate
			if actualRate == 0 {
				actualRate = 48000
			}
		}
		actualCh := fc.channels
		if actualCh == 0 {
			actualCh = channels
		}
		fc.stopped.Store(false)
		fc.mu.Lock()
		fc.onSamples = onSamples
		fc.onInterrupt = onInterrupt
		fc.onError = onError
		fc.mu.Unlock()
		return &captureSession{
			rate:     actualRate,
			channels: actualCh,
			stop:     func() { fc.stopped.Store(true) },
			resume:   func() { fc.resumed.Add(1) },
		}, nil
	}
}

func TestLocalRecordWAVMonoAndStereo(t *testing.T) {
	fc := &fakeLocalCapture{rate: 16000, channels: 2}
	dir := withLocalRecorder(t, localRecorderHooks{open: fakeOpener(fc)})

	r, err := RecordWAV("stereo.wav", RecordOptions{Rate: 44100, Channels: 2})
	if err != nil {
		t.Fatal(err)
	}
	if f := r.File(); f.Rate != 16000 || f.Channels != 2 || f.Path != filepath.Join(dir, "stereo.wav") {
		t.Fatalf("File = %+v", f)
	}

	samples := []int16{1000, -1000, 2000, -2000, 3000, -3000, 4000, -4000}
	fc.emit(samples)
	r.Stop()
	r.Stop() // idempotent

	if err := r.Err(); err != nil {
		t.Fatalf("Err = %v", err)
	}
	if got := r.File().Duration; got != 4*time.Second/16000 {
		t.Errorf("Duration = %v, want %v", got, 4*time.Second/16000)
	}
	// .part file must be renamed away.
	if _, err := os.Stat(filepath.Join(dir, ".stereo.wav.part")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf(".part file still exists: %v", err)
	}

	raw, err := os.ReadFile(r.File().Path)
	if err != nil {
		t.Fatal(err)
	}
	rate, ch, dataLen, ok := driver.ParseWAVHeader(raw)
	if !ok || rate != 16000 || ch != 2 || dataLen != uint32(len(samples)*2) {
		t.Fatalf("ParseWAVHeader = (%d, %d, %d, %v)", rate, ch, dataLen, ok)
	}
	if len(raw) != driver.WAVHeaderSize+len(samples)*2 {
		t.Fatalf("file size = %d, want %d", len(raw), driver.WAVHeaderSize+len(samples)*2)
	}
	for i, want := range samples {
		got := int16(binary.LittleEndian.Uint16(raw[driver.WAVHeaderSize+2*i:]))
		if got != want {
			t.Fatalf("sample %d = %d, want %d", i, got, want)
		}
	}
}

func TestLocalRecordWAVPeriodicHeaderPatchAndDurationWithoutLevels(t *testing.T) {
	fc := &fakeLocalCapture{rate: 8000, channels: 1}
	dir := withLocalRecorder(t, localRecorderHooks{open: fakeOpener(fc)})

	r, err := RecordWAV("long.wav", RecordOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Emit 1 full second (8000 samples) to trigger the 1-second periodic header rewrite.
	chunk := make([]int16, 8000)
	for i := range chunk {
		chunk[i] = 100
	}
	fc.emit(chunk)

	// Wait until Duration updates without calling Stop().
	deadline := time.Now().Add(5 * time.Second)
	for r.File().Duration < time.Second {
		if time.Now().After(deadline) {
			t.Fatalf("Duration did not advance while recording: %v", r.File().Duration)
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Verify the .part file on disk already has a valid WAV header reflecting 1s of PCM.
	partRaw, err := os.ReadFile(filepath.Join(dir, ".long.wav.part"))
	if err != nil {
		t.Fatal(err)
	}
	rate, ch, dataLen, ok := driver.ParseWAVHeader(partRaw)
	if !ok || rate != 8000 || ch != 1 || dataLen != 16000 {
		t.Fatalf("mid-recording .part header = (%d, %d, %d, %v), want (8000, 1, 16000, true)", rate, ch, dataLen, ok)
	}

	r.Stop()
	if d := r.File().Duration; d != time.Second {
		t.Errorf("final Duration = %v, want 1s", d)
	}
}

func TestLocalRecordWAVLevelsAndBackpressure(t *testing.T) {
	fc := &fakeLocalCapture{rate: 10000, channels: 1}
	withLocalRecorder(t, localRecorderHooks{open: fakeOpener(fc)})

	release := make(chan struct{})
	var mu sync.Mutex
	var levels []Level
	r, err := RecordWAV("levels.wav", RecordOptions{
		LevelInterval: 20 * time.Millisecond, // 200 frames per level at 10 kHz
		Level: func(l Level) {
			<-release
			mu.Lock()
			levels = append(levels, l)
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Emit 20 chunks of 200 samples with increasing amplitude while callback is blocked.
	for i := 1; i <= 20; i++ {
		val := int16(i * 1000)
		buf := make([]int16, 200)
		for j := range buf {
			buf[j] = val
		}
		fc.emit(buf)
	}
	// Give writer goroutine a moment to process chunks.
	deadline := time.Now().Add(5 * time.Second)
	for r.File().Duration < 400*time.Millisecond {
		if time.Now().After(deadline) {
			t.Fatalf("writer stalled behind slow level callback: duration=%v", r.File().Duration)
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(release)

	wantPeak := float32(20000.0 / 32768.0)
	deadline = time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(levels)
		var last Level
		if n > 0 {
			last = levels[n-1]
		}
		mu.Unlock()
		if n > 0 && math.Abs(float64(last.Peak-wantPeak)) < 1e-4 && math.Abs(float64(last.RMS-wantPeak)) < 1e-4 {
			if n > 4 {
				t.Errorf("callback received %d of 20 levels, want superseded ones dropped", n)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("last level = %+v (count %d), want peak/rms ≈ %v", last, n, wantPeak)
		}
		time.Sleep(2 * time.Millisecond)
	}
	r.Stop()
}

func TestLocalRecordWAVBusyExistsAndOpenCleanup(t *testing.T) {
	fc := &fakeLocalCapture{rate: 16000, channels: 1}
	dir := withLocalRecorder(t, localRecorderHooks{open: fakeOpener(fc)})

	// Open failure cleans up the .part file and releases the active slot.
	fc.openErr = ErrUnavailable
	if _, err := RecordWAV("fail.wav", RecordOptions{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("open error = %v, want ErrUnavailable", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".fail.wav.part")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("failed open left .part behind: %v", err)
	}
	fc.openErr = nil

	// Second concurrent recording fails with ErrBusy.
	r, err := RecordWAV("one.wav", RecordOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RecordWAV("two.wav", RecordOptions{}); !errors.Is(err, ErrBusy) {
		t.Errorf("concurrent RecordWAV = %v, want ErrBusy", err)
	}
	r.Stop()

	// Recording over an existing file fails with fs.ErrExist.
	if _, err := RecordWAV("one.wav", RecordOptions{}); !errors.Is(err, fs.ErrExist) {
		t.Errorf("existing file RecordWAV = %v, want fs.ErrExist", err)
	}
}

func TestLocalRecordWAVInterruptions(t *testing.T) {
	fc := &fakeLocalCapture{rate: 16000, channels: 1}
	withLocalRecorder(t, localRecorderHooks{open: fakeOpener(fc)})

	// Default: interruption ends the recording with ErrInterrupted.
	r, err := RecordWAV("ends.wav", RecordOptions{})
	if err != nil {
		t.Fatal(err)
	}
	fc.emit(make([]int16, 1600))
	fc.interrupt(true)
	<-r.Ended()
	if !errors.Is(r.Err(), ErrInterrupted) {
		t.Errorf("Err = %v, want ErrInterrupted", r.Err())
	}

	// PauseOnInterruption: silences writes while interrupted and resumes after.
	r2, err := RecordWAV("pauses.wav", RecordOptions{PauseOnInterruption: true})
	if err != nil {
		t.Fatal(err)
	}
	fc.emit(make([]int16, 1600)) // 100 ms
	fc.interrupt(true)
	if !r2.Interrupted() {
		t.Error("Interrupted = false while silenced")
	}
	fc.emit(make([]int16, 8000)) // dropped while silenced
	fc.interrupt(false)
	if r2.Interrupted() {
		t.Error("Interrupted = true after silence ended")
	}
	if fc.resumed.Load() != 1 {
		t.Errorf("resume count = %d, want 1", fc.resumed.Load())
	}
	fc.emit(make([]int16, 1600)) // another 100 ms
	r2.Stop()
	if err := r2.Err(); err != nil {
		t.Errorf("Err = %v, want nil", err)
	}
	if d := r2.File().Duration; d != 200*time.Millisecond {
		t.Errorf("Duration = %v, want 200ms (only non-silenced audio)", d)
	}
}

func TestLocalRecordWAVLimitsAndAuth(t *testing.T) {
	fc := &fakeLocalCapture{rate: 16000, channels: 1}

	// Denied authorization.
	withLocalRecorder(t, localRecorderHooks{
		authorize: func() error { return ErrDenied },
		open:      fakeOpener(fc),
	})
	if _, err := RecordWAV("denied.wav", RecordOptions{}); !errors.Is(err, ErrDenied) {
		t.Errorf("denied RecordWAV = %v, want ErrDenied", err)
	}

	// Insufficient disk space at start.
	withLocalRecorder(t, localRecorderHooks{
		open:      fakeOpener(fc),
		freeSpace: func(string) int64 { return 1024 },
		reserve:   8 << 20,
	})
	if _, err := RecordWAV("nospace.wav", RecordOptions{}); !errors.Is(err, ErrNoSpace) {
		t.Errorf("low-space RecordWAV = %v, want ErrNoSpace", err)
	}

	// RIFF 4 GiB limit (configured to 200 bytes for fast testing).
	withLocalRecorder(t, localRecorderHooks{
		open:    fakeOpener(fc),
		maxData: 200,
	})
	r, err := RecordWAV("full.wav", RecordOptions{})
	if err != nil {
		t.Fatal(err)
	}
	fc.emit(make([]int16, 80)) // 160 bytes -> fits
	fc.emit(make([]int16, 80)) // +160 = 320 > 200 -> ErrFull
	<-r.Ended()
	if !errors.Is(r.Err(), ErrFull) {
		t.Errorf("Err = %v, want ErrFull", r.Err())
	}
	if raw, err := os.ReadFile(r.File().Path); err != nil {
		t.Fatal(err)
	} else if _, _, dataLen, ok := driver.ParseWAVHeader(raw); !ok || dataLen != 160 {
		t.Errorf("finalized full WAV dataLen = %d (ok=%v), want 160", dataLen, ok)
	}
}

func TestLocalRecordWAVPartialRepairAndEarlier(t *testing.T) {
	fc := &fakeLocalCapture{rate: 16000, channels: 1}
	dir := withLocalRecorder(t, localRecorderHooks{open: fakeOpener(fc)})

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Simulate a killed recording: valid header with stale 0-byte size + 400 bytes PCM + 1 stray byte.
	partPath := filepath.Join(dir, ".crashed.wav.part")
	hdr := driver.WAVHeader(16000, 1, 0)
	payload := append(hdr[:], make([]byte, 401)...)
	if err := os.WriteFile(partPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	// Simulate an empty/uninitialized .part file that should be discarded.
	if err := os.WriteFile(filepath.Join(dir, ".empty.wav.part"), []byte("short"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Starting a new recording triggers repair of leftover .part files.
	r, err := RecordWAV("fresh.wav", RecordOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r.Stop()

	if _, err := os.Stat(filepath.Join(dir, ".empty.wav.part")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("corrupt .part was not deleted: %v", err)
	}

	repairedPath := filepath.Join(dir, "crashed.wav")
	repairedRaw, err := os.ReadFile(repairedPath)
	if err != nil {
		t.Fatalf("repaired file missing: %v", err)
	}
	if len(repairedRaw) != driver.WAVHeaderSize+400 {
		t.Fatalf("repaired file size = %d, want %d (truncated trailing partial frame)", len(repairedRaw), driver.WAVHeaderSize+400)
	}
	if rate, ch, dataLen, ok := driver.ParseWAVHeader(repairedRaw); !ok || rate != 16000 || ch != 1 || dataLen != 400 {
		t.Fatalf("repaired WAV header = (%d, %d, %d, %v)", rate, ch, dataLen, ok)
	}

	earlier := Earlier()
	if len(earlier) != 1 {
		t.Fatalf("Earlier() = %d entries, want 1", len(earlier))
	}
	e := earlier[0]
	select {
	case <-e.Ended():
	default:
		t.Error("earlier recording's Ended is not closed")
	}
	if !errors.Is(e.Err(), ErrPartial) {
		t.Errorf("earlier Err = %v, want ErrPartial", e.Err())
	}
	if f := e.File(); f.Path != repairedPath || f.Rate != 16000 || f.Channels != 1 || f.Duration != 200*time.Second/16000 {
		t.Errorf("earlier File = %+v", f)
	}
	// Earlier reports each record only once.
	if got := Earlier(); len(got) != 0 {
		t.Errorf("second Earlier() = %d entries, want 0", len(got))
	}
}
