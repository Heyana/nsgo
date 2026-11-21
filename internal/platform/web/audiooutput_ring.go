package web

import (
	"encoding/binary"
	"errors"
	"math"
	"time"
)

const (
	webPCMQuantum          = 128
	webPCMMaxSlices        = 256
	webPCMMaxChunksPerTick = 8
	webPCMDefaultLatency   = 24 * time.Millisecond
)

var (
	errWebPCMClosed      = errors.New("nsgo: audio: PCM output is closed")
	errWebPCMInterrupted = errors.New("nsgo: audio: Web Audio output interrupted")
	errWebPCMSuspended   = errors.New("nsgo: audio: Web Audio output suspended")
	errWebPCMContextGone = errors.New("nsgo: audio: Web Audio context closed")
)

// webPCMGeometry holds the derived frame counts and scheduling cadence for a
// Web pull PCM stream.
type webPCMGeometry struct {
	sampleRate     int
	channels       int
	chunkFrames    int
	targetFrames   int
	ringCapFrames  int
	reportFrames   int
	pumpIntervalMs int
}

func computeWebPCMGeometry(sampleRate, channels int, latency time.Duration) webPCMGeometry {
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	if channels < 1 || channels > 2 {
		channels = 2
	}
	if latency <= 0 {
		latency = webPCMDefaultLatency
	}
	maxFrames := max(sampleRate/2, 2048)
	desired := int(math.Ceil(latency.Seconds() * float64(sampleRate)))
	desired = min(max(desired, 2*webPCMQuantum), maxFrames)
	desired = roundUpMultiple(desired, webPCMQuantum)

	var chunk int
	switch {
	case desired <= 2*webPCMQuantum:
		chunk = webPCMQuantum
	case desired <= 8*webPCMQuantum:
		chunk = 2 * webPCMQuantum
	default:
		chunk = 4 * webPCMQuantum
	}
	target := max(desired, chunk*2)
	target = roundUpMultiple(target, chunk)

	ringCap := max(target*4, 4096)
	ringCap = roundUpMultiple(ringCap, chunk)

	chunkMs := int(framesToDuration(int64(chunk), sampleRate) / time.Millisecond)
	intervalMs := min(max(chunkMs/2, 4), 20)

	return webPCMGeometry{
		sampleRate:     sampleRate,
		channels:       channels,
		chunkFrames:    chunk,
		targetFrames:   target,
		ringCapFrames:  ringCap,
		reportFrames:   chunk,
		pumpIntervalMs: intervalMs,
	}
}

func roundUpMultiple(n, step int) int {
	if step <= 1 {
		return n
	}
	rem := n % step
	if rem == 0 {
		return n
	}
	return n + (step - rem)
}

func framesToDuration(frames int64, sampleRate int) time.Duration {
	if frames <= 0 || sampleRate <= 0 {
		return 0
	}
	return time.Duration(frames * int64(time.Second) / int64(sampleRate))
}

func sanitizeSeconds(sec float64) time.Duration {
	if math.IsNaN(sec) || math.IsInf(sec, 0) || sec <= 0 {
		return 0
	}
	return time.Duration(math.Round(sec * float64(time.Second)))
}

func secToNanos(sec float64) int64 {
	if math.IsNaN(sec) || math.IsInf(sec, 0) || sec <= 0 {
		return 0
	}
	return int64(math.Round(sec * float64(time.Second)))
}

// webOutputClock resolves the audio presentation clock (in AudioContext
// seconds) and corresponding wall-clock reference from AudioContext.currentTime
// and AudioContext.getOutputTimestamp(). When getOutputTimestamp reports a
// valid positive contextTime it is preferred; otherwise currentTime minus
// outputLatency is used.
func webOutputClock(curSec, tsContextSec, tsPerfMs, perfNowMs, outputLatencySec float64, wallNow time.Time) (float64, time.Time) {
	if math.IsNaN(curSec) || math.IsInf(curSec, 0) || curSec < 0 {
		curSec = 0
	}
	if math.IsNaN(outputLatencySec) || math.IsInf(outputLatencySec, 0) || outputLatencySec < 0 {
		outputLatencySec = 0
	}
	if !math.IsNaN(tsContextSec) && !math.IsInf(tsContextSec, 0) && tsContextSec > 0 && tsContextSec <= curSec+0.05 {
		wallRef := wallNow
		if !math.IsNaN(tsPerfMs) && !math.IsInf(tsPerfMs, 0) && !math.IsNaN(perfNowMs) && !math.IsInf(perfNowMs, 0) && tsPerfMs > 0 && perfNowMs > 0 {
			delta := time.Duration((tsPerfMs - perfNowMs) * float64(time.Millisecond))
			if delta < 0 && delta > -time.Second {
				wallRef = wallNow.Add(delta)
			}
		}
		return tsContextSec, wallRef
	}
	nowSec := curSec - outputLatencySec
	if nowSec < 0 {
		nowSec = 0
	}
	return nowSec, wallNow
}

type pcmRingChunk struct {
	frames   int
	produced int
}

// pcmRing is a fixed-capacity circular buffer of interleaved Float32 samples
// that preserves per-chunk client-produced frame counts so Flush reports only
// discarded client frames and never counts underrun silence.
type pcmRing struct {
	samples        []float32
	channels       int
	capFrames      int
	readPos        int
	writePos       int
	bufferedFrames int
	chunks         []pcmRingChunk
}

func newPCMRing(capFrames, channels int) *pcmRing {
	return &pcmRing{
		samples:   make([]float32, capFrames*channels),
		channels:  channels,
		capFrames: capFrames,
		chunks:    make([]pcmRingChunk, 0, 16),
	}
}

func (r *pcmRing) buffered() int { return r.bufferedFrames }
func (r *pcmRing) free() int     { return r.capFrames - r.bufferedFrames }

func (r *pcmRing) pendingClient() int {
	total := 0
	for _, c := range r.chunks {
		total += c.produced
	}
	return total
}

func (r *pcmRing) write(src []float32, frames, produced int) int {
	if frames <= 0 {
		return 0
	}
	n := min(frames, r.free())
	if n <= 0 {
		return 0
	}
	if produced < 0 {
		produced = 0
	}
	if produced > n {
		produced = n
	}
	first := min(n, r.capFrames-r.writePos)
	copy(r.samples[r.writePos*r.channels:(r.writePos+first)*r.channels], src[:first*r.channels])
	if rem := n - first; rem > 0 {
		copy(r.samples[:rem*r.channels], src[first*r.channels:n*r.channels])
	}
	r.writePos = (r.writePos + n) % r.capFrames
	r.bufferedFrames += n
	r.chunks = append(r.chunks, pcmRingChunk{frames: n, produced: produced})
	return n
}

func (r *pcmRing) readChunk(dst []float32) (frames, produced int, ok bool) {
	if len(r.chunks) == 0 {
		return 0, 0, false
	}
	c := r.chunks[0]
	if len(dst) < c.frames*r.channels {
		return 0, 0, false
	}
	first := min(c.frames, r.capFrames-r.readPos)
	copy(dst[:first*r.channels], r.samples[r.readPos*r.channels:(r.readPos+first)*r.channels])
	if rem := c.frames - first; rem > 0 {
		copy(dst[first*r.channels:c.frames*r.channels], r.samples[:rem*r.channels])
	}
	r.readPos = (r.readPos + c.frames) % r.capFrames
	r.bufferedFrames -= c.frames
	copy(r.chunks, r.chunks[1:])
	r.chunks = r.chunks[:len(r.chunks)-1]
	return c.frames, c.produced, true
}

func (r *pcmRing) flush() int {
	dropped := r.pendingClient()
	r.readPos = 0
	r.writePos = 0
	r.bufferedFrames = 0
	r.chunks = r.chunks[:0]
	return dropped
}

type webPCMSlice struct {
	startNs  int64
	first    int64
	frames   int64
	produced int64
}

// webPCMTimeline tracks chunks submitted to Web Audio on the AudioContext
// timeline and computes monotonic presented frame counts and discarded pending
// client frames on Flush/Pause.
type webPCMTimeline struct {
	sampleRate    int
	base          int64
	frames        int64
	at            time.Time
	segmentFrames int64
	nextNs        int64
	hasNext       bool
	slices        []webPCMSlice
}

func newWebPCMTimeline(sampleRate int) webPCMTimeline {
	return webPCMTimeline{
		sampleRate: sampleRate,
		slices:     make([]webPCMSlice, 0, 32),
	}
}

func (t *webPCMTimeline) schedule(nowSec float64, frames, produced int64) float64 {
	startNs := secToNanos(nowSec)
	if t.hasNext && t.nextNs > startNs {
		startNs = t.nextNs
	}
	if len(t.slices) >= webPCMMaxSlices {
		copy(t.slices, t.slices[1:])
		t.slices = t.slices[:len(t.slices)-1]
	}
	t.slices = append(t.slices, webPCMSlice{
		startNs:  startNs,
		first:    t.segmentFrames,
		frames:   frames,
		produced: produced,
	})
	t.segmentFrames += frames
	t.nextNs = startNs + int64(framesToDuration(frames, t.sampleRate))
	t.hasNext = true
	return float64(startNs) / float64(time.Second)
}

func (t *webPCMTimeline) advance(nowSec float64, wallNow time.Time) int {
	nowNs := secToNanos(nowSec)
	pending := 0
	for i := len(t.slices) - 1; i >= 0; i-- {
		s := t.slices[i]
		durNs := int64(framesToDuration(s.frames, t.sampleRate))
		elapsedNs := nowNs - s.startNs
		var played int64
		switch {
		case elapsedNs >= durNs:
			played = s.frames
		case elapsedNs > 0:
			played = min(elapsedNs*int64(t.sampleRate)/int64(time.Second), s.frames)
		}
		if s.produced > played {
			pending += int(s.produced - played)
		}
		if played > 0 {
			heardFrames := s.first + played
			heardNs := s.startNs + int64(framesToDuration(played, t.sampleRate))
			delta := min(time.Duration(heardNs-nowNs), 0)
			when := wallNow.Add(delta)
			if total := t.base + heardFrames; total > t.frames {
				t.frames = total
				if when.After(t.at) {
					t.at = when
				}
			}
			break
		}
	}
	return pending
}

func (t *webPCMTimeline) position(nowSec float64, wallNow time.Time, active bool) (int64, time.Time) {
	if active {
		t.advance(nowSec, wallNow)
	}
	return t.frames, t.at
}

func (t *webPCMTimeline) resetSegment(nowSec float64, wallNow time.Time, active bool) int {
	dropped := 0
	if active {
		dropped = t.advance(nowSec, wallNow)
	}
	t.base = t.frames
	t.segmentFrames = 0
	t.nextNs = 0
	t.hasNext = false
	t.slices = t.slices[:0]
	return dropped
}

func (t *webPCMTimeline) aheadFrames(nowSec float64) int {
	if !t.hasNext {
		return 0
	}
	nowNs := secToNanos(nowSec)
	if t.nextNs <= nowNs {
		return 0
	}
	aheadNs := t.nextNs - nowNs
	return int((aheadNs*int64(t.sampleRate) + int64(time.Second) - 1) / int64(time.Second))
}

// webPCMState encapsulates the platform-independent state machine for Web pull
// PCM output: ring buffering, short-fill silence padding, underrun accounting,
// monotonic position tracking, flush/pause/close transitions, latency reporting
// and AudioContext statechange interruption classification.
type webPCMState struct {
	geom        webPCMGeometry
	ring        *pcmRing
	timeline    webPCMTimeline
	scratch     []float32
	active      bool
	pausing     bool
	closed      bool
	seenRunning bool
	rendered    bool
	underruns   int
}

func newWebPCMState(sampleRate, channels int, latency time.Duration) *webPCMState {
	g := computeWebPCMGeometry(sampleRate, channels, latency)
	return &webPCMState{
		geom:     g,
		ring:     newPCMRing(g.ringCapFrames, g.channels),
		timeline: newWebPCMTimeline(g.sampleRate),
		scratch:  make([]float32, g.chunkFrames*g.channels),
	}
}

func (s *webPCMState) start() error {
	if s.closed {
		return errWebPCMClosed
	}
	s.active = true
	s.pausing = false
	return nil
}

func (s *webPCMState) pause(nowSec float64, wallNow time.Time) int {
	dropped := s.timeline.resetSegment(nowSec, wallNow, s.active)
	dropped += s.ring.flush()
	s.active = false
	s.pausing = true
	return dropped
}

func (s *webPCMState) flush(nowSec float64, wallNow time.Time) int {
	dropped := s.timeline.resetSegment(nowSec, wallNow, s.active)
	dropped += s.ring.flush()
	return dropped
}

func (s *webPCMState) close(nowSec float64, wallNow time.Time) bool {
	if s.closed {
		return false
	}
	s.pause(nowSec, wallNow)
	s.closed = true
	return true
}

// pullIntoRing invokes render for one chunk, zero-pads any short fill,
// increments underruns on short fill, and pushes the chunk into the ring buffer.
func (s *webPCMState) pullIntoRing(render func([]float32) int) (frames, produced int, underrun bool) {
	frames = s.geom.chunkFrames
	if s.ring.free() < frames {
		return 0, 0, false
	}
	buf := s.scratch[:frames*s.geom.channels]
	n := render(buf)
	produced = min(max(n, 0), frames)
	if produced < frames {
		clear(buf[produced*s.geom.channels:])
		s.underruns++
		underrun = true
	}
	s.rendered = true
	s.ring.write(buf, frames, produced)
	return frames, produced, underrun
}

func (s *webPCMState) addWorkletUnderruns(n int) {
	if n > 0 {
		s.underruns += n
	}
}

func (s *webPCMState) neededChunks(nowSec float64) int {
	if !s.active || s.closed {
		return 0
	}
	queued := s.timeline.aheadFrames(nowSec) + s.ring.buffered()
	deficit := s.geom.targetFrames - queued
	if deficit <= 0 {
		return 0
	}
	chunks := (deficit + s.geom.chunkFrames - 1) / s.geom.chunkFrames
	maxByRing := s.ring.free() / s.geom.chunkFrames
	return min(chunks, min(maxByRing, webPCMMaxChunksPerTick))
}

func (s *webPCMState) position(nowSec float64, wallNow time.Time) (int64, time.Time) {
	return s.timeline.position(nowSec, wallNow, s.active)
}

func (s *webPCMState) latency(baseSec, outputSec, nowSec float64) time.Duration {
	if !s.rendered {
		return 0
	}
	hw := sanitizeSeconds(baseSec) + sanitizeSeconds(outputSec)
	queued := s.timeline.aheadFrames(nowSec) + s.ring.buffered()
	buf := framesToDuration(int64(max(queued, s.geom.chunkFrames)), s.geom.sampleRate)
	return hw + buf
}

// onContextState updates statechange tracking for an AudioContext state name
// ("running", "suspended", "interrupted", "closed") and reports whether the
// transition is an unexpected interruption that should pause the stream and
// notify OnInterrupt.
func (s *webPCMState) onContextState(state string) (bool, error) {
	if state == "running" {
		s.seenRunning = true
		return false, nil
	}
	if !s.active || s.pausing || s.closed {
		return false, nil
	}
	switch state {
	case "interrupted":
		return true, errWebPCMInterrupted
	case "suspended":
		if s.seenRunning {
			return true, errWebPCMSuspended
		}
	case "closed":
		return true, errWebPCMContextGone
	}
	return false, nil
}

// encodeInterleavedFloat32LE encodes interleaved Float32 samples into
// little-endian bytes for bulk transfer to a JS Float32Array via CopyBytesToJS.
func encodeInterleavedFloat32LE(dst []byte, src []float32) {
	for i, v := range src {
		binary.LittleEndian.PutUint32(dst[i*4:], math.Float32bits(v))
	}
}

// encodePlanarChannelFloat32LE extracts one channel from interleaved Float32
// samples into little-endian bytes for AudioBuffer.copyToChannel in the
// AudioBufferSourceNode fallback path.
func encodePlanarChannelFloat32LE(dst []byte, interleaved []float32, channel, channels, frames int) {
	for i := range frames {
		v := interleaved[i*channels+channel]
		binary.LittleEndian.PutUint32(dst[i*4:], math.Float32bits(v))
	}
}
