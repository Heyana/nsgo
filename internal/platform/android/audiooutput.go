package android

import (
	"encoding/binary"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// What extAudioOutput asks Java and msgAudioOutputReport reports back, kept in sync with Protocol.java's PCM_*.
const (
	pcmOpen  = 1
	pcmStart = 2
	pcmPause = 3
	pcmClose = 4
	pcmData  = 5

	pcmReply       = 1
	pcmPull        = 2
	pcmInterrupted = 3
)

type pcmChunk struct {
	first    int64
	frames   int64
	produced int64
}

// pcmTracker keeps the monotonic device position and the client frames not yet heard by the device in the current generation. Position includes silence rendered for underruns; discarded frames on Flush exclude underrun silence.
type pcmTracker struct {
	base         int64
	frames       int64
	at           time.Time
	writtenInGen int64
	chunks       []pcmChunk
}

func (t *pcmTracker) push(frames, produced int64) {
	if frames <= 0 {
		return
	}
	produced = min(max(produced, 0), frames)
	t.chunks = append(t.chunks, pcmChunk{
		first:    t.writtenInGen,
		frames:   frames,
		produced: produced,
	})
	t.writtenInGen += frames
}

func (t *pcmTracker) advance(played int64, when time.Time) {
	played = min(max(played, 0), t.writtenInGen)
	if played > 0 {
		if total := t.base + played; total > t.frames {
			t.frames = total
			if !when.IsZero() && when.After(t.at) {
				t.at = when
			}
		}
	}
	playedInGen := max(t.frames-t.base, 0)
	drop := 0
	for drop < len(t.chunks) && t.chunks[drop].first+t.chunks[drop].frames <= playedInGen {
		drop++
	}
	if drop > 0 {
		copy(t.chunks, t.chunks[drop:])
		t.chunks = t.chunks[:len(t.chunks)-drop]
	}
}

func (t *pcmTracker) finishGeneration(played int64, when time.Time) int {
	t.advance(played, when)
	playedInGen := max(t.frames-t.base, 0)
	dropped := 0
	for _, c := range t.chunks {
		chunkPlayed := min(max(playedInGen-c.first, 0), c.frames)
		if rem := c.produced - chunkPlayed; rem > 0 {
			dropped += int(rem)
		}
	}
	t.base = t.frames
	t.writtenInGen = 0
	t.chunks = t.chunks[:0]
	return dropped
}

func (t *pcmTracker) position() (int64, time.Time) {
	return t.frames, t.at
}

type pcmControlReply struct {
	err          string
	bufferFrames uint32
	chunkFrames  uint32
	playedFrames int64
	offsetNanos  int64
	latencyNanos int64
}

// audioOutputManager owns the open pull PCM streams on a surface's pipe.
type audioOutputManager struct {
	send func(func(*writer)) error
	now  func() time.Time

	mu         sync.Mutex
	closed     bool
	nextStream uint32
	nextReq    uint32
	streams    map[uint32]*audioOutput
	waiting    map[uint32]chan pcmControlReply
}

func (m *audioOutputManager) init(send func(func(*writer)) error) {
	m.mu.Lock()
	m.send = send
	m.mu.Unlock()
}

func (m *audioOutputManager) clock() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

func (m *audioOutputManager) sendPayload(body func(*writer)) error {
	m.mu.Lock()
	send := m.send
	closed := m.closed
	m.mu.Unlock()
	if closed || send == nil {
		return errors.New("nsgo: audio: Android runtime is unavailable")
	}
	return send(body)
}

func (m *audioOutputManager) ask(body func(reqID uint32, w *writer)) (pcmControlReply, error) {
	ch := make(chan pcmControlReply, 1)
	m.mu.Lock()
	if m.closed || m.send == nil {
		m.mu.Unlock()
		return pcmControlReply{}, errors.New("nsgo: audio: Android runtime is unavailable")
	}
	m.nextReq++
	reqID := m.nextReq
	if m.waiting == nil {
		m.waiting = map[uint32]chan pcmControlReply{}
	}
	m.waiting[reqID] = ch
	send := m.send
	m.mu.Unlock()

	if err := send(func(w *writer) { body(reqID, w) }); err != nil {
		m.mu.Lock()
		delete(m.waiting, reqID)
		m.mu.Unlock()
		return pcmControlReply{}, err
	}

	reply := <-ch
	if reply.err != "" {
		return reply, errors.New("nsgo: audio: " + reply.err)
	}
	return reply, nil
}

func (m *audioOutputManager) open(conf driver.AudioOutputConfig) (driver.AudioOutput, error) {
	m.mu.Lock()
	if m.closed || m.send == nil {
		m.mu.Unlock()
		return nil, driver.ErrAudioOutputUnsupported
	}
	m.nextStream++
	id := m.nextStream
	m.mu.Unlock()

	reply, err := m.ask(func(reqID uint32, w *writer) {
		writeAudioOutputOpen(w, id, reqID, conf)
	})
	if err != nil {
		return nil, err
	}

	chunkFrames := max(int(reply.chunkFrames), 1)
	bufferFrames := max(int(reply.bufferFrames), chunkFrames)
	baseLatency := time.Duration(reply.latencyNanos)
	if baseLatency <= 0 && conf.SampleRate > 0 {
		baseLatency = time.Duration(int64(bufferFrames) * int64(time.Second) / int64(conf.SampleRate))
	}
	o := &audioOutput{
		mgr:          m,
		id:           id,
		conf:         conf,
		bufferFrames: bufferFrames,
		chunkFrames:  chunkFrames,
		baseLatency:  baseLatency,
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, errors.New("nsgo: audio: Android runtime is unavailable")
	}
	if m.streams == nil {
		m.streams = map[uint32]*audioOutput{}
	}
	m.streams[id] = o
	m.mu.Unlock()
	return o, nil
}

func (m *audioOutputManager) stream(id uint32) *audioOutput {
	m.mu.Lock()
	o := m.streams[id]
	m.mu.Unlock()
	return o
}

func (m *audioOutputManager) remove(id uint32) {
	m.mu.Lock()
	delete(m.streams, id)
	m.mu.Unlock()
}

// report handles one msgAudioOutputReport payload on the pipe reader goroutine without blocking.
func (m *audioOutputManager) report(r *reader) {
	switch r.byte() {
	case pcmReply:
		_ = r.u32() // streamID
		reqID := r.u32()
		reply := pcmControlReply{
			err:          r.str(),
			bufferFrames: r.u32(),
			chunkFrames:  r.u32(),
			playedFrames: int64(r.u64()),
			offsetNanos:  int64(r.u64()),
			latencyNanos: int64(r.u64()),
		}
		m.mu.Lock()
		ch := m.waiting[reqID]
		delete(m.waiting, reqID)
		m.mu.Unlock()
		if ch != nil {
			ch <- reply
		}
	case pcmPull:
		id, gen, requested := r.u32(), r.u32(), int(r.u32())
		played, offset, latency := int64(r.u64()), int64(r.u64()), int64(r.u64())
		if o := m.stream(id); o != nil {
			o.onPull(gen, requested, played, offset, latency)
		}
	case pcmInterrupted:
		id, gen := r.u32(), r.u32()
		played, offset := int64(r.u64()), int64(r.u64())
		reason := r.str()
		if o := m.stream(id); o != nil {
			go o.onInterrupted(gen, played, offset, reason)
		}
	}
}

// closeAll wakes pending control calls and detaches every open stream when the pipe closes.
func (m *audioOutputManager) closeAll() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	waiting := m.waiting
	m.waiting = nil
	streams := make([]*audioOutput, 0, len(m.streams))
	for _, o := range m.streams {
		streams = append(streams, o)
	}
	m.streams = nil
	m.mu.Unlock()

	for _, ch := range waiting {
		ch <- pcmControlReply{err: "Android runtime disconnected"}
	}
	for _, o := range streams {
		o.detach()
	}
}

func writeAudioOutputOpen(w *writer, id, reqID uint32, conf driver.AudioOutputConfig) {
	w.byte(pcmOpen)
	w.u32(id)
	w.u32(reqID)
	w.u32(uint32(conf.SampleRate))
	w.byte(byte(conf.Channels))
	w.u64(uint64(conf.Latency))
}

func writeAudioOutputStart(w *writer, id, reqID, gen uint32) {
	w.byte(pcmStart)
	w.u32(id)
	w.u32(reqID)
	w.u32(gen)
}

func writeAudioOutputPause(w *writer, id, reqID, gen uint32, keepFocus bool) {
	w.byte(pcmPause)
	w.u32(id)
	w.u32(reqID)
	w.u32(gen)
	w.flags(keepFocus)
}

func writeAudioOutputClose(w *writer, id, reqID uint32) {
	w.byte(pcmClose)
	w.u32(id)
	w.u32(reqID)
}

func writeAudioOutputData(w *writer, id, gen, frames uint32, pcm []byte) {
	w.byte(pcmData)
	w.u32(id)
	w.u32(gen)
	w.u32(frames)
	w.u32(uint32(len(pcm)))
	w.buf = append(w.buf, pcm...)
}

type audioOutput struct {
	mgr          *audioOutputManager
	id           uint32
	conf         driver.AudioOutputConfig
	bufferFrames int
	chunkFrames  int
	baseLatency  time.Duration

	mu         sync.Mutex
	closed     bool
	active     bool
	gen        uint32
	stopCh     chan struct{}
	renderDone chan struct{}

	posMu    sync.Mutex
	tracker  pcmTracker
	pullCh   chan int
	pullGen  uint32
	rendered bool

	latency   atomic.Int64
	underruns atomic.Uint64
}

func (o *audioOutput) hostTime(offsetNanos int64) time.Time {
	now := o.mgr.clock()
	when := now.Add(time.Duration(offsetNanos))
	if when.After(now) {
		return now
	}
	return when
}

func (o *audioOutput) Start() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.startLocked()
}

func (o *audioOutput) startLocked() error {
	if o.closed {
		return errors.New("nsgo: audio: PCM output is closed")
	}
	if o.active {
		return nil
	}
	gen := o.gen + 1
	pullCh := make(chan int, 32)
	stopCh := make(chan struct{})
	renderDone := make(chan struct{})

	o.posMu.Lock()
	o.pullCh = pullCh
	o.pullGen = gen
	o.posMu.Unlock()

	reply, err := o.mgr.ask(func(reqID uint32, w *writer) {
		writeAudioOutputStart(w, o.id, reqID, gen)
	})
	if err != nil {
		o.posMu.Lock()
		if o.pullGen == gen {
			o.pullCh = nil
			o.pullGen = 0
		}
		o.posMu.Unlock()
		return err
	}

	if reply.chunkFrames > 0 {
		o.chunkFrames = int(reply.chunkFrames)
	}
	if reply.bufferFrames > 0 {
		o.bufferFrames = int(reply.bufferFrames)
	}
	if reply.latencyNanos > 0 {
		o.baseLatency = time.Duration(reply.latencyNanos)
	}

	o.gen = gen
	o.active = true
	o.stopCh = stopCh
	o.renderDone = renderDone
	go o.renderLoop(gen, o.chunkFrames, pullCh, stopCh, renderDone)
	return nil
}

func (o *audioOutput) Pause() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.stopLocked(false)
}

func (o *audioOutput) stopLocked(keepFocus bool) int {
	if !o.active {
		return 0
	}
	gen := o.gen
	stopCh := o.stopCh
	renderDone := o.renderDone

	o.posMu.Lock()
	o.pullCh = nil
	o.pullGen = 0
	o.posMu.Unlock()

	close(stopCh)
	<-renderDone

	reply, err := o.mgr.ask(func(reqID uint32, w *writer) {
		writeAudioOutputPause(w, o.id, reqID, gen, keepFocus)
	})

	var played int64
	var when time.Time
	if err == nil {
		played = reply.playedFrames
		when = o.hostTime(reply.offsetNanos)
	}

	o.posMu.Lock()
	dropped := o.tracker.finishGeneration(played, when)
	o.posMu.Unlock()

	o.active = false
	o.stopCh = nil
	o.renderDone = nil
	return dropped
}

func (o *audioOutput) Flush() int {
	o.mu.Lock()
	wasRunning := o.active
	dropped := o.stopLocked(true)
	var err error
	if wasRunning {
		err = o.startLocked()
	}
	o.mu.Unlock()
	if err != nil && o.conf.Interrupt != nil {
		o.conf.Interrupt(err)
	}
	return dropped
}

func (o *audioOutput) Close() {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return
	}
	o.closed = true
	o.stopLocked(false)
	o.mu.Unlock()

	o.mgr.remove(o.id)
	_, _ = o.mgr.ask(func(reqID uint32, w *writer) {
		writeAudioOutputClose(w, o.id, reqID)
	})
}

func (o *audioOutput) detach() {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return
	}
	o.closed = true
	if o.active {
		o.posMu.Lock()
		o.pullCh = nil
		o.pullGen = 0
		o.posMu.Unlock()
		close(o.stopCh)
		<-o.renderDone
		o.active = false
		o.stopCh = nil
		o.renderDone = nil
	}
	o.mu.Unlock()
}

func (o *audioOutput) onPull(gen uint32, requested int, played, offsetNanos, latencyNanos int64) {
	when := o.hostTime(offsetNanos)
	o.posMu.Lock()
	if o.pullCh == nil || o.pullGen != gen {
		o.posMu.Unlock()
		return
	}
	o.tracker.advance(played, when)
	if o.rendered && latencyNanos > 0 {
		o.latency.Store(latencyNanos)
	}
	ch := o.pullCh
	o.posMu.Unlock()

	if requested <= 0 {
		return
	}
	select {
	case ch <- requested:
	default:
	}
}

func (o *audioOutput) onInterrupted(gen uint32, played, offsetNanos int64, reason string) {
	o.mu.Lock()
	if o.closed || !o.active || o.gen != gen {
		o.mu.Unlock()
		return
	}
	stopCh := o.stopCh
	renderDone := o.renderDone

	o.posMu.Lock()
	o.pullCh = nil
	o.pullGen = 0
	o.posMu.Unlock()

	close(stopCh)
	<-renderDone

	when := o.hostTime(offsetNanos)
	o.posMu.Lock()
	o.tracker.finishGeneration(played, when)
	o.posMu.Unlock()

	o.active = false
	o.stopCh = nil
	o.renderDone = nil
	o.mu.Unlock()

	if o.conf.Interrupt != nil {
		if reason == "" {
			reason = "audio output interrupted"
		}
		o.conf.Interrupt(errors.New("nsgo: audio: " + reason))
	}
}

func (o *audioOutput) renderLoop(gen uint32, defaultFrames int, pullCh <-chan int, stopCh <-chan struct{}, done chan struct{}) {
	defer close(done)
	channels := o.conf.Channels
	buf := make([]float32, max(defaultFrames, 1)*channels)
	wire := make([]byte, len(buf)*4)

	for {
		select {
		case <-stopCh:
			return
		case frames := <-pullCh:
			select {
			case <-stopCh:
				return
			default:
			}
			if frames <= 0 {
				frames = defaultFrames
			}
			samples := frames * channels
			if samples > cap(buf) {
				buf = make([]float32, samples)
				wire = make([]byte, samples*4)
			}
			pcm := buf[:samples]
			produced := min(max(o.conf.Render(pcm), 0), frames)
			if produced < frames {
				clear(pcm[produced*channels:])
				o.underruns.Add(1)
			}

			o.posMu.Lock()
			if o.pullGen != gen {
				o.posMu.Unlock()
				return
			}
			o.tracker.push(int64(frames), int64(produced))
			o.rendered = true
			if o.latency.Load() == 0 && o.baseLatency > 0 {
				o.latency.Store(int64(o.baseLatency))
			}
			o.posMu.Unlock()

			raw := wire[:samples*4]
			for i, s := range pcm {
				binary.LittleEndian.PutUint32(raw[i*4:], math.Float32bits(s))
			}
			_ = o.mgr.sendPayload(func(w *writer) {
				writeAudioOutputData(w, o.id, gen, uint32(frames), raw)
			})
		}
	}
}

func (o *audioOutput) Position() (int64, time.Time) {
	o.posMu.Lock()
	defer o.posMu.Unlock()
	return o.tracker.position()
}

func (o *audioOutput) Latency() time.Duration {
	return time.Duration(o.latency.Load())
}

func (o *audioOutput) Underruns() int {
	return int(o.underruns.Load())
}
