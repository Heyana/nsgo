// Package pcmoutput drives pull-based PCM playback over a frame-oriented
// audio device such as Linux ALSA.
package pcmoutput

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	// ErrAgain reports that the device buffer is full or a call was
	// interrupted by a signal and should be retried after waiting.
	ErrAgain = errors.New("nsgo: audio: PCM device temporarily busy")
	// ErrXRun reports a hardware or server underrun (such as ALSA -EPIPE)
	// that requires Prepare before writing resumes.
	ErrXRun = errors.New("nsgo: audio: PCM buffer underrun")
	// ErrSuspended reports that the audio device was suspended (such as
	// ALSA -ESTRPIPE).
	ErrSuspended = errors.New("nsgo: audio: PCM device suspended")
	// ErrDisconnected reports that the audio device was disconnected or
	// became unusable (such as ALSA -ENODEV).
	ErrDisconnected = errors.New("nsgo: audio: PCM device disconnected")
)

// Device is the low-level PCM sink owned by one Stream. After New, Device
// methods are called only under the stream's control lock while stopped or
// exclusively from the pull goroutine while running.
type Device interface {
	// BufferFrames and PeriodFrames return the negotiated ring buffer and
	// per-callback period lengths in frames.
	BufferFrames() int
	PeriodFrames() int
	// Prepare transitions the device into a ready-to-write state.
	Prepare() error
	// AvailDelay returns the frames currently available to write and the
	// frames still queued ahead of playback. delayOK is false if the device
	// cannot report delay.
	AvailDelay() (avail int, delay int64, delayOK bool, err error)
	// Wait blocks up to timeout until the device can accept more frames,
	// reporting whether space became available.
	Wait(timeout time.Duration) (ready bool, err error)
	// Write writes interleaved Float32 samples and returns the frames accepted.
	Write(samples []float32) (frames int, err error)
	// Drop immediately stops playback and discards queued frames.
	Drop()
	// Close releases the underlying device handle.
	Close()
}

// DefaultLatencyMicros and fallback bounds used when negotiating a device
// buffer duration in microseconds from an optional latency hint.
const (
	MinLatencyMicros      = 4_000
	DefaultLatencyMicros  = 40_000
	FallbackLatencyMicros = 100_000
	MaxLatencyMicros      = 500_000
)

// LatencyCandidates returns ordered ALSA-style buffer latencies in
// microseconds to try for hint, falling back to safe defaults if the device
// rejects a tight hint.
func LatencyCandidates(hint time.Duration) []uint32 {
	out := make([]uint32, 0, 3)
	add := func(us uint32) {
		for _, prev := range out {
			if prev == us {
				return
			}
		}
		out = append(out, us)
	}
	if hint > 0 {
		us := hint.Microseconds()
		if us < MinLatencyMicros {
			us = MinLatencyMicros
		} else if us > MaxLatencyMicros {
			us = MaxLatencyMicros
		}
		add(uint32(us))
	}
	add(DefaultLatencyMicros)
	add(FallbackLatencyMicros)
	return out
}

// Duration returns the time spanned by frames at sampleRate Hz.
func Duration(frames int64, sampleRate int) time.Duration {
	if frames <= 0 || sampleRate <= 0 {
		return 0
	}
	return time.Duration(frames * int64(time.Second) / int64(sampleRate))
}

type writtenSlice struct {
	first    int64
	frames   int64
	produced int64
}

// Stream implements driver.AudioOutput by pulling PCM periods on a dedicated
// goroutine and feeding them to a Device.
type Stream struct {
	dev          Device
	conf         driver.AudioOutputConfig
	bufferFrames int
	periodFrames int
	waitTimeout  time.Duration
	now          func() time.Time

	// ctrlMu serializes Start, Pause, Flush and Close. The pull goroutine
	// never acquires ctrlMu, so stopping and joining it under ctrlMu cannot
	// deadlock.
	ctrlMu sync.Mutex
	done   chan struct{}

	// stateMu guards position, run accounting and lifecycle flags read
	// concurrently by Position and the pull goroutine.
	stateMu         sync.Mutex
	closed          bool
	active          bool
	base            int64
	frames          int64
	at              time.Time
	runWritten      int64
	runStart        time.Time
	runSlices       []writtenSlice
	unwrittenClient int

	stopping  atomic.Bool
	latency   atomic.Int64
	underruns atomic.Uint64
}

// New constructs a paused Stream over dev. Call Start to begin pulling samples.
func New(dev Device, conf driver.AudioOutputConfig) *Stream {
	return newWithClock(dev, conf, time.Now)
}

func newWithClock(dev Device, conf driver.AudioOutputConfig, now func() time.Time) *Stream {
	period := dev.PeriodFrames()
	if period <= 0 {
		period = max(conf.SampleRate/100, 64)
	}
	buffer := dev.BufferFrames()
	if buffer <= 0 {
		buffer = period * 4
	}
	if period > buffer {
		period = buffer
	}
	wait := Duration(int64(period), conf.SampleRate) / 2
	if wait < time.Millisecond {
		wait = time.Millisecond
	} else if wait > 5*time.Millisecond {
		wait = 5 * time.Millisecond
	}
	return &Stream{
		dev:          dev,
		conf:         conf,
		bufferFrames: buffer,
		periodFrames: period,
		waitTimeout:  wait,
		now:          now,
	}
}

// Start prepares the device and launches the pull goroutine.
func (s *Stream) Start() error {
	s.ctrlMu.Lock()
	defer s.ctrlMu.Unlock()
	return s.startLocked()
}

func (s *Stream) startLocked() error {
	s.stateMu.Lock()
	if s.closed {
		s.stateMu.Unlock()
		return errors.New("nsgo: audio: PCM output is closed")
	}
	if s.active {
		s.stateMu.Unlock()
		return nil
	}
	s.stateMu.Unlock()

	if err := s.dev.Prepare(); err != nil {
		return err
	}

	s.stateMu.Lock()
	s.base = s.frames
	s.runWritten = 0
	s.runStart = time.Time{}
	s.runSlices = s.runSlices[:0]
	s.unwrittenClient = 0
	s.active = true
	s.stateMu.Unlock()

	s.stopping.Store(false)
	done := make(chan struct{})
	s.done = done
	go s.run(done)
	return nil
}

// Pause stops rendering, discards queued frames and freezes Position.
func (s *Stream) Pause() {
	s.ctrlMu.Lock()
	defer s.ctrlMu.Unlock()
	s.stopLocked()
}

// Flush stops rendering, discards queued frames not yet played, reports the
// number of discarded client-rendered frames and restarts if active.
func (s *Stream) Flush() int {
	s.ctrlMu.Lock()
	s.stateMu.Lock()
	wasActive := s.active
	closed := s.closed
	s.stateMu.Unlock()
	if closed {
		s.ctrlMu.Unlock()
		return 0
	}
	dropped := s.stopLocked()
	var err error
	if wasActive {
		err = s.startLocked()
	}
	s.ctrlMu.Unlock()
	if err != nil && s.conf.Interrupt != nil {
		s.conf.Interrupt(err)
	}
	return dropped
}

// Close stops rendering and releases the device. Calling it again has no effect.
func (s *Stream) Close() {
	s.ctrlMu.Lock()
	s.stateMu.Lock()
	if s.closed {
		s.stateMu.Unlock()
		s.ctrlMu.Unlock()
		return
	}
	s.closed = true
	s.stateMu.Unlock()

	s.stopping.Store(true)
	if s.done != nil {
		<-s.done
		s.done = nil
	}
	s.sampleFinalLocked()
	s.dev.Drop()
	s.dev.Close()

	s.stateMu.Lock()
	s.base = s.frames
	s.runWritten = 0
	s.runStart = time.Time{}
	s.runSlices = s.runSlices[:0]
	s.unwrittenClient = 0
	s.active = false
	s.stateMu.Unlock()
	s.ctrlMu.Unlock()
}

// stopLocked stops the pull goroutine, freezes Position, drops queued device
// frames and returns how many client-produced frames were discarded.
func (s *Stream) stopLocked() int {
	s.stopping.Store(true)
	if s.done != nil {
		<-s.done
		s.done = nil
	}
	s.stateMu.Lock()
	wasActive := s.active
	s.stateMu.Unlock()
	if !wasActive {
		return 0
	}
	s.sampleFinalLocked()
	s.stateMu.Lock()
	dropped := s.pendingClientFramesLocked()
	s.base = s.frames
	s.runWritten = 0
	s.runStart = time.Time{}
	s.runSlices = s.runSlices[:0]
	s.unwrittenClient = 0
	s.active = false
	s.stateMu.Unlock()
	s.dev.Drop()
	return dropped
}

// Position returns the monotonic count of frames that reached the device and
// the host timestamp of the most recent advance.
func (s *Stream) Position() (int64, time.Time) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.frames, s.at
}

// Latency returns the most recent device buffer latency, or zero before the
// first render.
func (s *Stream) Latency() time.Duration {
	return time.Duration(s.latency.Load())
}

// Underruns counts short Render callbacks and device xruns.
func (s *Stream) Underruns() int {
	return int(s.underruns.Load())
}

func (s *Stream) run(done chan struct{}) {
	var interruptErr error
	defer func() {
		if interruptErr != nil && s.conf.Interrupt != nil {
			s.stateMu.Lock()
			notify := !s.closed && !s.stopping.Load()
			s.stateMu.Unlock()
			if notify {
				s.conf.Interrupt(interruptErr)
			}
		}
	}()
	defer close(done)

	channels := s.conf.Channels
	buf := make([]float32, s.periodFrames*channels)
	var pendingOffset, pendingFrames, pendingProduced int
	renderedAny := false

	saveUnwritten := func() {
		rem := max(pendingProduced-pendingOffset, 0)
		if rem > 0 {
			s.stateMu.Lock()
			s.unwrittenClient += rem
			s.stateMu.Unlock()
		}
	}

	fail := func(err error) {
		s.sampleFinalLocked()
		s.dev.Drop()
		s.stateMu.Lock()
		wasActive := s.active
		s.base = s.frames
		s.runWritten = 0
		s.runStart = time.Time{}
		s.runSlices = s.runSlices[:0]
		s.unwrittenClient = 0
		s.active = false
		s.stateMu.Unlock()
		if wasActive {
			interruptErr = err
		}
	}

	recoverXRun := func() bool {
		s.underruns.Add(1)
		s.commitXRunLocked()
		if err := s.dev.Prepare(); err != nil {
			fail(err)
			return false
		}
		return true
	}

	for !s.stopping.Load() {
		if pendingOffset >= pendingFrames {
			avail, delay, delayOK, err := s.dev.AvailDelay()
			if err != nil {
				if errors.Is(err, ErrXRun) {
					if !recoverXRun() {
						return
					}
					continue
				}
				if errors.Is(err, ErrAgain) {
					s.waitBrief()
					continue
				}
				fail(err)
				return
			}
			if renderedAny {
				s.recordDelay(delay, delayOK)
			}
			needWait := avail < s.periodFrames && !(avail == 0 && s.writtenInRun() == 0)
			if needWait {
				ready, err := s.dev.Wait(s.waitTimeout)
				if s.stopping.Load() {
					return
				}
				if err != nil {
					if errors.Is(err, ErrXRun) {
						if !recoverXRun() {
							return
						}
						continue
					}
					if errors.Is(err, ErrAgain) {
						continue
					}
					fail(err)
					return
				}
				avail, delay, delayOK, err = s.dev.AvailDelay()
				if err != nil {
					if errors.Is(err, ErrXRun) {
						if !recoverXRun() {
							return
						}
						continue
					}
					if errors.Is(err, ErrAgain) {
						continue
					}
					fail(err)
					return
				}
				if renderedAny {
					s.recordDelay(delay, delayOK)
				}
				if avail < s.periodFrames {
					if ready {
						s.waitBrief()
					}
					continue
				}
			}
			if s.stopping.Load() {
				return
			}
			produced := min(max(s.conf.Render(buf), 0), s.periodFrames)
			if produced < s.periodFrames {
				clear(buf[produced*channels:])
				s.underruns.Add(1)
			}
			renderedAny = true
			pendingOffset = 0
			pendingFrames = s.periodFrames
			pendingProduced = produced
		}

		n, err := s.dev.Write(buf[pendingOffset*channels : pendingFrames*channels])
		if n > 0 {
			writtenClient := min(max(pendingProduced-pendingOffset, 0), n)
			s.recordWritten(n, writtenClient)
			pendingOffset += n
			if pendingOffset >= pendingFrames {
				pendingOffset, pendingFrames, pendingProduced = 0, 0, 0
			}
			if _, delay, delayOK, derr := s.dev.AvailDelay(); derr == nil {
				s.recordDelay(delay, delayOK)
			} else if errors.Is(derr, ErrXRun) {
				if !recoverXRun() {
					return
				}
				continue
			} else if !errors.Is(derr, ErrAgain) {
				fail(derr)
				return
			}
		}
		if err != nil {
			if errors.Is(err, ErrAgain) {
				if s.stopping.Load() {
					saveUnwritten()
					return
				}
				ready, werr := s.dev.Wait(s.waitTimeout)
				if s.stopping.Load() {
					saveUnwritten()
					return
				}
				if werr != nil {
					if errors.Is(werr, ErrXRun) {
						if !recoverXRun() {
							return
						}
						continue
					}
					if !errors.Is(werr, ErrAgain) {
						fail(werr)
						return
					}
				}
				if _, delay, delayOK, derr := s.dev.AvailDelay(); derr == nil && renderedAny {
					s.recordDelay(delay, delayOK)
				}
				if ready && n == 0 {
					s.waitBrief()
				}
				continue
			}
			if errors.Is(err, ErrXRun) {
				if !recoverXRun() {
					return
				}
				continue
			}
			fail(err)
			return
		}
	}
	saveUnwritten()
}

func (s *Stream) waitBrief() {
	if !s.stopping.Load() {
		time.Sleep(time.Millisecond)
	}
}

func (s *Stream) writtenInRun() int64 {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.runWritten
}

func (s *Stream) recordWritten(frames, clientFrames int) {
	now := s.now()
	s.stateMu.Lock()
	if s.runWritten == 0 && s.runStart.IsZero() {
		s.runStart = now
	}
	s.runSlices = append(s.runSlices, writtenSlice{
		first:    s.runWritten,
		frames:   int64(frames),
		produced: int64(clientFrames),
	})
	s.runWritten += int64(frames)
	s.stateMu.Unlock()
	if s.latency.Load() == 0 {
		s.latency.Store(int64(Duration(int64(s.bufferFrames), s.conf.SampleRate)))
	}
}

func (s *Stream) recordDelay(delay int64, delayOK bool) {
	now := s.now()
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.advanceWithDelayLocked(delay, delayOK, now)
}

func (s *Stream) sampleFinalLocked() {
	_, delay, delayOK, err := s.dev.AvailDelay()
	if errors.Is(err, ErrXRun) {
		s.commitXRunLocked()
		return
	}
	if err != nil {
		return
	}
	now := s.now()
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.advanceWithDelayLocked(delay, delayOK, now)
}

func (s *Stream) commitXRunLocked() {
	now := s.now()
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if total := s.base + s.runWritten; total > s.frames {
		s.frames = total
		if now.After(s.at) {
			s.at = now
		}
	}
	s.base = s.frames
	s.runWritten = 0
	s.runStart = time.Time{}
	s.runSlices = s.runSlices[:0]
}

func (s *Stream) advanceWithDelayLocked(delay int64, delayOK bool, now time.Time) {
	if s.runWritten == 0 {
		return
	}
	var heardInRun int64
	if delayOK {
		heardInRun = s.runWritten - delay
		if delay > 0 {
			s.latency.Store(int64(Duration(delay, s.conf.SampleRate)))
		} else {
			s.latency.Store(int64(Duration(int64(s.bufferFrames), s.conf.SampleRate)))
		}
	} else {
		if !s.runStart.IsZero() && now.After(s.runStart) {
			elapsed := now.Sub(s.runStart)
			heardInRun = int64(elapsed) * int64(s.conf.SampleRate) / int64(time.Second)
		}
		s.latency.Store(int64(Duration(int64(s.bufferFrames), s.conf.SampleRate)))
	}
	if heardInRun < 0 {
		heardInRun = 0
	} else if heardInRun > s.runWritten {
		heardInRun = s.runWritten
	}
	if total := s.base + heardInRun; total > s.frames {
		s.frames = total
		if now.After(s.at) {
			s.at = now
		}
	}
	s.prunePlayedSlicesLocked(s.frames - s.base)
}

func (s *Stream) prunePlayedSlicesLocked(heardInRun int64) {
	drop := 0
	for drop < len(s.runSlices) && s.runSlices[drop].first+s.runSlices[drop].frames <= heardInRun {
		drop++
	}
	if drop == 0 {
		return
	}
	remaining := copy(s.runSlices, s.runSlices[drop:])
	s.runSlices = s.runSlices[:remaining]
}

func (s *Stream) pendingClientFramesLocked() int {
	heardInRun := max(s.frames-s.base, 0)
	var pending int64
	for _, sl := range s.runSlices {
		if sl.first+sl.frames <= heardInRun {
			continue
		}
		played := max(heardInRun-sl.first, 0)
		if sl.produced > played {
			pending += sl.produced - played
		}
	}
	return int(pending) + s.unwrittenClient
}
