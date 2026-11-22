//go:build windows && (amd64 || arm64)

package windows

import (
	"encoding/binary"
	"math"
	"runtime"
	"slices"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// WASAPI vtable indexes and flags the loopback capture needs beyond those audiooutput.go declares, from audioclient.h. IAudioClient.Initialize takes two REFERENCE_TIME words, so this file builds only where an int64 is one argument word.
const (
	acGetMixFormat      = 8
	acGetDevicePeriod   = 9
	ccGetBuffer         = 3
	ccReleaseBuffer     = 4
	ccGetNextPacketSize = 5

	audclntStreamFlagsLoopback       = 0x00020000
	audclntBufferFlagsDiscontinuity  = 0x1
	audclntBufferFlagsSilent         = 0x2
	audclntBufferFlagsTimestampError = 0x4
)

var (
	iidIAudioCaptureClient = parseGUID("C8ADBD64-E71E-48A0-A4DE-185C395CD317")
	ksdataformatFloat      = parseGUID("00000003-0000-0010-8000-00AA00389B71")
)

// loopbackCapture records what the default render endpoint's engine mixes, timestamped on the QPC clock.
type loopbackCapture struct {
	client, capture *comObject
	channels        int
	rate            float64
	float           bool // 32-bit float samples, otherwise 16-bit integers

	// The last packet whose QPC timestamp WASAPI marked valid, as a device frame position and its QPC time in 100 ns units, to time a packet whose own timestamp is flagged invalid.
	anchorPosition, anchorQPC uint64
	anchored                  bool
}

// sound is the first loud sample a capture saw.
type sound struct {
	at        int64 // QPC time in 100 ns units, meaningful only when timed
	timed     bool  // false when WASAPI flagged the packet's timestamp invalid and no earlier packet had a valid one
	estimated bool  // the time comes from an earlier packet's timestamp and the device frame position
	glitch    bool  // WASAPI flagged a discontinuity on the packet, so the engine may have dropped audio before it
}

func openLoopbackCapture(t *testing.T) *loopbackCapture {
	t.Helper()
	var enumerator, device, client, capture *comObject
	if hr, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)), 0, clsctxAll, uintptr(unsafe.Pointer(&iidMMDeviceEnumerator)), uintptr(unsafe.Pointer(&enumerator))); failed(hr) || enumerator == nil {
		t.Skip(hresultError("cannot create the audio device enumerator", hr))
	}
	defer enumerator.release()
	const eRender, eConsole = 0, 0
	if hr := enumerator.call(mmEnumGetDefaultAudioEndpoint, eRender, eConsole, uintptr(unsafe.Pointer(&device))); failed(hr) || device == nil {
		t.Skip(hresultError("no default render endpoint for loopback capture", hr))
	}
	defer device.release()
	if hr := device.call(mmDeviceActivate, uintptr(unsafe.Pointer(&iidAudioClient)), clsctxAll, 0, uintptr(unsafe.Pointer(&client))); failed(hr) || client == nil {
		t.Fatal(hresultError("cannot activate the endpoint's audio client", hr))
	}
	var format unsafe.Pointer
	if hr := client.call(acGetMixFormat, uintptr(unsafe.Pointer(&format))); failed(hr) || format == nil {
		client.release()
		t.Fatal(hresultError("cannot read the endpoint's mix format", hr))
	}
	defer coTaskMemFree.Call(uintptr(format))
	// WAVEFORMATEX, then WAVEFORMATEXTENSIBLE's SubFormat at byte 24 when the tag is 0xFFFE.
	header := unsafe.Slice((*byte)(format), 18)
	tag := binary.LittleEndian.Uint16(header[0:])
	c := &loopbackCapture{
		channels: int(binary.LittleEndian.Uint16(header[2:])),
		rate:     float64(binary.LittleEndian.Uint32(header[4:])),
	}
	bits := binary.LittleEndian.Uint16(header[14:])
	const waveFloat, wavePCM, waveExtensible = 3, 1, 0xfffe
	switch {
	case tag == waveFloat && bits == 32:
		c.float = true
	case tag == wavePCM && bits == 16:
	case tag == waveExtensible:
		sub := *(*guid)(unsafe.Add(format, 24))
		c.float = sub == ksdataformatFloat && bits == 32
		if !c.float && bits != 16 {
			client.release()
			t.Skipf("unsupported %d-bit extensible mix format", bits)
		}
	default:
		client.release()
		t.Skipf("unsupported mix format tag %#x with %d bits", tag, bits)
	}
	const bufferDuration = 10_000_000 // 1 s in 100 ns units
	if hr := client.call(acInitialize, 0, audclntStreamFlagsLoopback, bufferDuration, 0, uintptr(format), 0); failed(hr) {
		client.release()
		t.Fatal(hresultError("cannot initialize loopback capture", hr))
	}
	if hr := client.call(acGetService, uintptr(unsafe.Pointer(&iidIAudioCaptureClient)), uintptr(unsafe.Pointer(&capture))); failed(hr) || capture == nil {
		client.release()
		t.Fatal(hresultError("cannot get the loopback capture client", hr))
	}
	c.client, c.capture = client, capture
	var period, minimum int64
	client.call(acGetDevicePeriod, uintptr(unsafe.Pointer(&period)), uintptr(unsafe.Pointer(&minimum)))
	t.Logf("default endpoint mix: %d channel(s) at %g Hz, float %v, engine period %v", c.channels, c.rate, c.float, time.Duration(period*100))
	if hr := client.call(acStart); failed(hr) {
		c.close()
		t.Fatal(hresultError("cannot start loopback capture", hr))
	}
	return c
}

func (c *loopbackCapture) close() {
	c.client.call(acStop)
	c.capture.release()
	c.client.release()
}

// qpc100ns reads the QPC clock in WASAPI's 100 ns units.
func qpc100ns() int64 {
	var counter, frequency int64
	wasapiQueryPerformanceCounter.Call(uintptr(unsafe.Pointer(&counter)))
	wasapiQueryPerformanceFrequency.Call(uintptr(unsafe.Pointer(&frequency)))
	return int64(float64(counter) * 1e7 / float64(frequency))
}

// firstSound drains captured packets until one holds a sample whose magnitude exceeds 0.005 of full scale (about -46 dBFS) and returns it, or false at the deadline.
func (c *loopbackCapture) firstSound(deadline time.Time) (sound, bool) {
	const threshold = 0.005
	for time.Now().Before(deadline) {
		var size uint32
		if failed(c.capture.call(ccGetNextPacketSize, uintptr(unsafe.Pointer(&size)))) {
			return sound{}, false
		}
		if size == 0 {
			time.Sleep(time.Millisecond)
			continue
		}
		var data unsafe.Pointer
		var frames, flags uint32
		var position, qpc uint64
		if failed(c.capture.call(ccGetBuffer, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&frames)), uintptr(unsafe.Pointer(&flags)), uintptr(unsafe.Pointer(&position)), uintptr(unsafe.Pointer(&qpc)))) {
			return sound{}, false
		}
		found := -1
		if flags&audclntBufferFlagsSilent == 0 && data != nil {
			n := int(frames) * c.channels
			if c.float {
				found = slices.IndexFunc(unsafe.Slice((*float32)(data), n), func(v float32) bool { return math.Abs(float64(v)) > threshold })
			} else {
				found = slices.IndexFunc(unsafe.Slice((*int16)(data), n), func(v int16) bool { return math.Abs(float64(v)/32768) > threshold })
			}
		}
		c.capture.call(ccReleaseBuffer, uintptr(frames))
		if s := c.packet(flags, position, qpc, found/c.channels); found >= 0 {
			return s, true
		}
	}
	return sound{}, false
}

// packet records a captured packet's timestamp as the anchor when WASAPI marks it valid and returns the time of the packet's frame at index frame. A packet flagged with a timestamp error is timed from the anchor's QPC time plus the device frames since it, or left untimed without an anchor.
func (c *loopbackCapture) packet(flags uint32, position, qpc uint64, frame int) sound {
	s := sound{glitch: flags&audclntBufferFlagsDiscontinuity != 0}
	offset := float64(frame) * 1e7 / c.rate
	switch {
	case flags&audclntBufferFlagsTimestampError == 0:
		c.anchorPosition, c.anchorQPC, c.anchored = position, qpc, true
		s.at, s.timed = int64(qpc)+int64(offset), true
	case c.anchored && position >= c.anchorPosition:
		s.at = int64(c.anchorQPC) + int64(float64(position-c.anchorPosition)*1e7/c.rate+offset)
		s.timed, s.estimated = true, true
	}
	return s
}

func TestLoopbackPacketTimestamps(t *testing.T) {
	c := &loopbackCapture{rate: 48000}
	if s := c.packet(audclntBufferFlagsTimestampError, 0, 999, 0); s.timed {
		t.Fatalf("an invalid timestamp with no anchor was timed: %+v", s)
	}
	if s := c.packet(0, 48000, 50_000_000, 480); !s.timed || s.estimated || s.at != 50_100_000 {
		t.Fatalf("valid packet, frame 480 at 48 kHz: %+v, want 50_100_000", s)
	}
	// Half a second of device frames after the anchor, with a bogus QPC value.
	s := c.packet(audclntBufferFlagsTimestampError|audclntBufferFlagsDiscontinuity, 72000, 1, 4800)
	if !s.timed || !s.estimated || !s.glitch || s.at != 56_000_000 {
		t.Fatalf("invalid timestamp after an anchor: %+v, want an estimated, glitched 56_000_000", s)
	}
	if s := c.packet(audclntBufferFlagsTimestampError, 47999, 1, 0); s.timed {
		t.Fatalf("a position before the anchor was timed: %+v", s)
	}
}

// drain discards captured audio until the endpoint has been silent for quiet.
func (c *loopbackCapture) drain(t *testing.T, quiet time.Duration) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; {
		if _, loud := c.firstSound(time.Now().Add(quiet)); !loud {
			return
		}
		if time.Now().After(deadline) {
			t.Skip("the default endpoint did not fall silent; another app may be playing")
		}
	}
}

// TestWindowsAudioLatencyThroughLoopback measures Play to the first sample above about -46 dBFS that the default endpoint's engine mixes, as WASAPI loopback capture sees it: first with XAudio2's output closed, on an endpoint the loopback capture already keeps running, then warm. A first sound on an idle endpoint also waits for the endpoint to wake, which this does not measure, and a device adds its own output latency after the mix.
func TestWindowsAudioLatencyThroughLoopback(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const apartmentThreaded = 2
	if hr, _, _ := coInitializeEx.Call(0, apartmentThreaded); !failed(hr) {
		defer coUninitialize.Call()
	}
	capture := openLoopbackCapture(t)
	defer capture.close()

	a := &windowAudio{sounds: map[uint32]*winSound{}}
	const frames = testAudioRate / 4
	buf := coAlloc(frames * 4)
	if buf == nil {
		t.Fatal("cannot allocate audio fixture")
	}
	// Native source voices must be destroyed before their sample storage.
	defer func() { a.closeOutput(); coTaskMemFree.Call(uintptr(buf)) }()
	pcm := unsafe.Slice((*float32)(buf), frames)
	for i := range pcm {
		pcm[i] = float32(0.5 * math.Sin(float64(i)*2*math.Pi*440/testAudioRate))
	}
	a.sounds[1] = &winSound{format: soundFormat{channels: 1, rate: testAudioRate}, data: buf, size: frames * 4}

	const runs = 6 // the first finds the output closed
	var warm []time.Duration
	for run := range runs {
		capture.drain(t, 150*time.Millisecond)
		cold := a.output == nil
		start := qpc100ns()
		a.play(1, &driver.AudioRequest{Volume: 1, Rate: 1})
		if a.output == nil {
			t.Skip("the default endpoint captures but XAudio2 cannot open it")
		}
		heard, ok := capture.firstSound(time.Now().Add(2 * time.Second))
		if !ok {
			t.Fatalf("run %d: the sound never reached the default endpoint", run)
		}
		if heard.glitch {
			t.Logf("run %d: WASAPI flagged a discontinuity on the first loud packet", run)
		}
		if !heard.timed {
			t.Logf("run %d: WASAPI flagged the first loud packet's timestamp invalid and no earlier packet had a valid one; not timed", run)
			a.stop(1)
			continue
		}
		if heard.estimated {
			t.Logf("run %d: WASAPI flagged the first loud packet's timestamp invalid; timed from an earlier packet and the device position", run)
		}
		latency := time.Duration(heard.at-start) * 100
		if cold {
			t.Logf("Play to endpoint mix, output closed: %v", latency)
		} else {
			warm = append(warm, latency)
		}
		if latency < 0 || latency > 500*time.Millisecond {
			t.Fatalf("run %d: Play reached the endpoint mix after %v", run, latency)
		}
		a.stop(1)
	}
	if len(warm) == 0 {
		t.Skip("WASAPI gave no valid timestamp for any warm run")
	}
	slices.Sort(warm)
	t.Logf("Play to endpoint mix, warm output, %d runs: %v to %v, median %v", len(warm), warm[0], warm[len(warm)-1], warm[len(warm)/2])
}
