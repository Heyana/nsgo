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

// WASAPI vtable indexes, from mmdeviceapi.h and audioclient.h. IAudioClient.Initialize takes two REFERENCE_TIME words, so this file builds only where an int64 is one argument word.
const (
	mmGetDefaultAudioEndpoint = 4
	mmActivate                = 3
	acInitialize              = 3
	acGetMixFormat            = 8
	acGetDevicePeriod         = 9
	acStart                   = 10
	acStop                    = 11
	acGetService              = 14
	ccGetBuffer               = 3
	ccReleaseBuffer           = 4
	ccGetNextPacketSize       = 5

	audclntStreamFlagsLoopback = 0x00020000
	audclntBufferFlagsSilent   = 0x2
	clsctxAll                  = 0x17
)

var (
	clsidMMDeviceEnumerator = parseGUID("BCDE0395-E52F-467C-8E3D-C4579291692E")
	iidIMMDeviceEnumerator  = parseGUID("A95664D2-9614-4F35-A746-DE8DB63617E6")
	iidIAudioClient         = parseGUID("1CB9AD4C-DBFA-4C32-B178-C2F568A703B2")
	iidIAudioCaptureClient  = parseGUID("C8ADBD64-E71E-48A0-A4DE-185C395CD317")
	ksdataformatFloat       = parseGUID("00000003-0000-0010-8000-00AA00389B71")

	queryPerformanceCounter   = kernel32.NewProc("QueryPerformanceCounter")
	queryPerformanceFrequency = kernel32.NewProc("QueryPerformanceFrequency")
)

// loopbackCapture records what the default render endpoint's engine mixes, timestamped on the QPC clock.
type loopbackCapture struct {
	client, capture *comObject
	channels        int
	rate            float64
	float           bool // 32-bit float samples, otherwise 16-bit integers
}

func openLoopbackCapture(t *testing.T) *loopbackCapture {
	t.Helper()
	var enumerator, device, client, capture *comObject
	if hr, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)), 0, clsctxAll, uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)), uintptr(unsafe.Pointer(&enumerator))); failed(hr) || enumerator == nil {
		t.Skip(hresultError("cannot create the audio device enumerator", hr))
	}
	defer enumerator.release()
	const eRender, eConsole = 0, 0
	if hr := enumerator.call(mmGetDefaultAudioEndpoint, eRender, eConsole, uintptr(unsafe.Pointer(&device))); failed(hr) || device == nil {
		t.Skip(hresultError("no default render endpoint for loopback capture", hr))
	}
	defer device.release()
	if hr := device.call(mmActivate, uintptr(unsafe.Pointer(&iidIAudioClient)), clsctxAll, 0, uintptr(unsafe.Pointer(&client))); failed(hr) || client == nil {
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
	queryPerformanceCounter.Call(uintptr(unsafe.Pointer(&counter)))
	queryPerformanceFrequency.Call(uintptr(unsafe.Pointer(&frequency)))
	return int64(float64(counter) * 1e7 / float64(frequency))
}

// firstSound drains captured packets until one holds a sample louder than the threshold and returns that sample's QPC time in 100 ns units, or false at the deadline.
func (c *loopbackCapture) firstSound(deadline time.Time) (int64, bool) {
	const threshold = 0.005
	for time.Now().Before(deadline) {
		var size uint32
		if failed(c.capture.call(ccGetNextPacketSize, uintptr(unsafe.Pointer(&size)))) {
			return 0, false
		}
		if size == 0 {
			time.Sleep(time.Millisecond)
			continue
		}
		var data unsafe.Pointer
		var frames, flags uint32
		var position, qpc uint64
		if failed(c.capture.call(ccGetBuffer, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&frames)), uintptr(unsafe.Pointer(&flags)), uintptr(unsafe.Pointer(&position)), uintptr(unsafe.Pointer(&qpc)))) {
			return 0, false
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
		if found >= 0 {
			return int64(qpc) + int64(float64(found/c.channels)*1e7/c.rate), true
		}
	}
	return 0, false
}

// drain discards captured audio until the endpoint has been silent for quiet.
func (c *loopbackCapture) drain(t *testing.T, quiet time.Duration) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; {
		if _, loud := c.firstSound(time.Now().Add(quiet)); !loud {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the default endpoint did not fall silent; another app may be playing")
		}
	}
}

// TestWindowsAudioLatencyThroughLoopback measures Play to the first samples the default endpoint's engine mixes, as WASAPI loopback capture sees them: first with the output closed, as an app's first sound finds it, then warm. A device adds its own output latency after the mix.
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
		latency := time.Duration(heard-start) * 100
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
	slices.Sort(warm)
	t.Logf("Play to endpoint mix, warm output, %d runs: %v to %v, median %v", len(warm), warm[0], warm[len(warm)-1], warm[len(warm)/2])
}
