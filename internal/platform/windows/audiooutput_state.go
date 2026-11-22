package windows

import (
	"math"
	"math/bits"
	"time"
)

// WASAPI WAVEFORMATEXTENSIBLE constants (mmreg.h / ksmedia.h).
const (
	waveFormatExtensibleTag = 0xFFFE
	waveFormatExtCBSize     = 22
	speakerFrontLeft        = 0x00000001
	speakerFrontRight       = 0x00000002
	speakerFrontCenter      = 0x00000004
	speakerMonoMask         = speakerFrontCenter
	speakerStereoMask       = speakerFrontLeft | speakerFrontRight
)

// WASAPI HRESULTs that indicate the audio endpoint or audio service was lost and the stream must be paused, invalidated and reopened on the next Start.
const (
	audclntEDeviceInvalidated    uint32 = 0x88890004
	audclntEEndpointCreateFailed uint32 = 0x8889000F
	audclntEServiceNotRunning    uint32 = 0x88890010
	audclntEResourcesInvalidated uint32 = 0x88890026
)

// wasapiGUID mirrors COM GUID's 16-byte memory layout without depending on the windows-only com.go file so format and layout tests run on every host OS.
type wasapiGUID struct {
	a    uint32
	b, c uint16
	d    [8]byte
}

// ksDataFormatSubtypeIEEEFloat is KSDATAFORMAT_SUBTYPE_IEEE_FLOAT ({00000003-0000-0010-8000-00AA00389B71}).
var ksDataFormatSubtypeIEEEFloat = wasapiGUID{
	a: 0x00000003,
	b: 0x0000,
	c: 0x0010,
	d: [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71},
}

// waveFormatExtensible matches the packed 40-byte WAVEFORMATEXTENSIBLE layout from mmreg.h. Its WAVEFORMATEX prefix fields are flattened directly so Go inserts no padding between cbSize (offset 16) and validBitsPerSample (offset 18).
type waveFormatExtensible struct {
	formatTag          uint16
	channels           uint16
	samplesPerSec      uint32
	avgBytesPerSec     uint32
	blockAlign         uint16
	bitsPerSample      uint16
	cbSize             uint16
	validBitsPerSample uint16
	channelMask        uint32
	subFormat          wasapiGUID
}

// wasapiWaveFormat constructs an interleaved 32-bit IEEE float WAVEFORMATEXTENSIBLE for 1- or 2-channel PCM output.
func wasapiWaveFormat(sampleRate, channels int) waveFormatExtensible {
	blockAlign := uint16(channels * 4)
	mask := uint32(speakerStereoMask)
	if channels == 1 {
		mask = speakerMonoMask
	}
	return waveFormatExtensible{
		formatTag:          waveFormatExtensibleTag,
		channels:           uint16(channels),
		samplesPerSec:      uint32(sampleRate),
		avgBytesPerSec:     uint32(sampleRate) * uint32(blockAlign),
		blockAlign:         blockAlign,
		bitsPerSample:      32,
		cbSize:             waveFormatExtCBSize,
		validBitsPerSample: 32,
		channelMask:        mask,
		subFormat:          ksDataFormatSubtypeIEEEFloat,
	}
}

// wasapiBufferDuration100ns converts a latency hint into WASAPI REFERENCE_TIME (100-nanosecond units). Zero or negative requests WASAPI's default shared buffer.
func wasapiBufferDuration100ns(latency time.Duration) uint64 {
	if latency <= 0 {
		return 0
	}
	hns := uint64(latency) / 100
	if hns == 0 {
		return 1
	}
	return hns
}

// wasapiLatency returns the total output latency combining the allocated shared-mode buffer duration and IAudioClient.GetStreamLatency (in 100ns units).
func wasapiLatency(bufferFrames uint32, sampleRate int, hnsStreamLatency int64) time.Duration {
	if sampleRate <= 0 {
		return 0
	}
	bufDur := time.Duration(int64(bufferFrames) * int64(time.Second) / int64(sampleRate))
	var streamDur time.Duration
	if hnsStreamLatency > 0 {
		streamDur = time.Duration(hnsStreamLatency) * 100 * time.Nanosecond
	}
	return bufDur + streamDur
}

// wasapiIsDeviceError reports whether hr means the WASAPI device or service was invalidated and cannot continue without reopening on a new endpoint.
func wasapiIsDeviceError(hr uintptr) bool {
	switch uint32(hr) {
	case audclntEDeviceInvalidated,
		audclntEEndpointCreateFailed,
		audclntEServiceNotRunning,
		audclntEResourcesInvalidated:
		return true
	}
	return false
}

// scaleTicksToU64 computes floor(ticks * numer / denom) using 128-bit intermediate arithmetic to avoid uint64 overflow.
func scaleTicksToU64(ticks, numer, denom uint64) uint64 {
	if ticks == 0 || numer == 0 || denom == 0 {
		return 0
	}
	hi, lo := bits.Mul64(ticks, numer)
	if hi >= denom {
		return ^uint64(0)
	}
	q, _ := bits.Div64(hi, lo, denom)
	return q
}

// scaleTicksToFrames converts IAudioClock ticks at freq into client sample frames.
func scaleTicksToFrames(ticks, sampleRate, freq uint64) int64 {
	q := scaleTicksToU64(ticks, sampleRate, freq)
	if q > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(q)
}

// qpcToWallTime converts IAudioClock.GetPosition's QPC timestamp (already in 100-nanosecond units per WASAPI's contract) into a host time.Time anchored to (qpcNow, wallNow) at performance-counter frequency qpcFreq.
func qpcToWallTime(qpcPos100ns, qpcNow, qpcFreq uint64, wallNow time.Time) time.Time {
	if qpcPos100ns == 0 || qpcFreq == 0 {
		return wallNow
	}
	now100ns := scaleTicksToU64(qpcNow, 10_000_000, qpcFreq)
	if qpcPos100ns >= now100ns {
		// Clamp slight rounding or clock skew so Position never reports a future time.
		return wallNow
	}
	diff100ns := now100ns - qpcPos100ns
	// Guard against virtual or broken drivers reporting a nonsensical QPC origin.
	const maxSkew100ns = 10 * 10_000_000
	if diff100ns > maxSkew100ns {
		return wallNow
	}
	return wallNow.Add(-time.Duration(diff100ns) * 100 * time.Nanosecond)
}

type wasapiChunk struct {
	start    int64 // total frames submitted before this chunk since the last Reset
	frames   int64 // frames submitted in this chunk (client + silence)
	produced int64 // frames filled by the client's Render callback (<= frames)
}

// wasapiState tracks monotonic presentation position, underruns and pending client-produced frames across WASAPI Start/Stop/Reset cycles. All mutations run on the output's dedicated worker goroutine.
type wasapiState struct {
	sampleRate int
	channels   int

	base      int64     // monotonic frames accumulated before the last Reset
	runFrames int64     // highest clock frames observed since the last Reset
	frames    int64     // monotonic total frames (= base + runFrames)
	at        time.Time // monotonic host time at frames (zero before the first frame)

	written   int64 // total frames submitted to WASAPI since the last Reset
	chunks    [64]wasapiChunk
	head      int
	count     int
	underruns uint64
}

func newWASAPIState(sampleRate, channels int) wasapiState {
	return wasapiState{sampleRate: sampleRate, channels: channels}
}

// fillAndRecord invokes render for buf, zero-fills any unwritten tail on a short render, increments underruns when short, and records the chunk.
func (s *wasapiState) fillAndRecord(buf []float32, render func([]float32) int) (frames, produced int, underrun bool) {
	if len(buf) == 0 || s.channels <= 0 || len(buf)%s.channels != 0 {
		return 0, 0, false
	}
	frames = len(buf) / s.channels
	produced = render(buf)
	if produced < 0 {
		produced = 0
	} else if produced > frames {
		produced = frames
	}
	if produced < frames {
		clear(buf[produced*s.channels:])
		underrun = true
	}
	s.recordSubmission(int64(frames), int64(produced))
	return frames, produced, underrun
}

func (s *wasapiState) recordSubmission(frames, produced int64) {
	if frames <= 0 {
		return
	}
	if produced < 0 {
		produced = 0
	} else if produced > frames {
		produced = frames
	}
	if produced < frames {
		s.underruns++
	}
	s.prunePlayed()
	if s.count == len(s.chunks) {
		s.head = (s.head + 1) % len(s.chunks)
		s.count--
	}
	idx := (s.head + s.count) % len(s.chunks)
	s.chunks[idx] = wasapiChunk{start: s.written, frames: frames, produced: produced}
	s.count++
	s.written += frames
}

// sampleClock advances the monotonic position from IAudioClock.GetPosition and QPC. The reported time remains zero until at least one frame has reached the device.
func (s *wasapiState) sampleClock(devPos, devFreq, qpcPos100ns, qpcNow, qpcFreq uint64, wallNow time.Time) (int64, time.Time) {
	clockFrames := scaleTicksToFrames(devPos, uint64(s.sampleRate), devFreq)
	if clockFrames > s.runFrames {
		s.runFrames = clockFrames
		s.frames = s.base + s.runFrames
		when := qpcToWallTime(qpcPos100ns, qpcNow, qpcFreq, wallNow)
		if s.at.IsZero() || when.After(s.at) {
			s.at = when
		}
		s.prunePlayed()
	}
	return s.frames, s.at
}

func (s *wasapiState) prunePlayed() {
	for s.count > 0 {
		c := s.chunks[s.head]
		if s.runFrames < c.start+c.frames {
			break
		}
		s.head = (s.head + 1) % len(s.chunks)
		s.count--
	}
}

// pendingFrames counts client-produced frames submitted since the last Reset that have not yet reached the device, excluding silence rendered for underruns.
func (s *wasapiState) pendingFrames() int {
	var pending int64
	heard := s.runFrames
	for i := 0; i < s.count; i++ {
		c := s.chunks[(s.head+i)%len(s.chunks)]
		if heard >= c.start+c.frames {
			continue
		}
		played := max(heard-c.start, 0)
		pending += max(c.produced-played, 0)
	}
	return int(pending)
}

// resetRun freezes monotonic position across IAudioClient.Stop + Reset, clears queued chunk tracking, and returns the number of unplayed client frames discarded.
func (s *wasapiState) resetRun() int {
	dropped := s.pendingFrames()
	s.base = s.frames
	s.runFrames = 0
	s.written = 0
	s.head = 0
	s.count = 0
	return dropped
}

func (s *wasapiState) position() (int64, time.Time) {
	return s.frames, s.at
}
