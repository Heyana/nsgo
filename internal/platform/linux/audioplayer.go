//go:build linux && !android

package linux

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

// linuxPlayer plays one source through GStreamer's classic playbin (decodebin2,
// avoiding playbin3/decodebin3 in GtkMediaFile which aborts on GStreamer 1.28)
// on the GTK thread, or a looping one's decoded track through the sound mixer
// (audiotrack.go).
type linuxPlayer struct {
	owner  *window
	audio  *linuxAudio
	id     uint32
	source driver.ImageSource
	stream uintptr // GStreamer playbin pipeline
	bus    uintptr // playbin's GstBus
	file   audioPlayerFile

	streamPlaying  bool
	streamSeekable bool
	streamEnded    bool

	// track is the decoded or streamed track, voice plays it and decoding cancels its decoding. noTrack says GStreamer could not read the track for the mixer, so playbin plays it; loadWanted that the player was asked to load.
	track                       trackFrames
	voice                       *trackVoice
	decoding                    context.CancelFunc
	noTrack, loadWanted, opened bool

	volume    float32
	loop      bool
	loopStart time.Duration
	interval  time.Duration

	loading, loaded, wanted, playing, ended, buffering, closed bool
	loadDeadline, lastProgress                                 time.Time
	position, duration                                         time.Duration
	pendingSeek, seeking                                       bool
	seeks, seekingNumber, appliedSeeks, laps                   uint32
}

// audioPlayerFile owns bytes staged on a worker for GStreamer playbin, which
// accepts files and URIs. Closing the player can race the write or the callback
// that opens the finished file.
type audioPlayerFile struct {
	mu     sync.Mutex
	path   string
	closed bool
}

func (f *audioPlayerFile) name() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.path
}

func (f *audioPlayerFile) hold(path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		os.Remove(path)
		return false
	}
	// Playbin and a track decoder can each stage the bytes; the later copy replaces the earlier.
	if f.path != "" && f.path != path {
		os.Remove(f.path)
	}
	f.path = path
	return true
}

func (f *audioPlayerFile) close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	if f.path != "" {
		os.Remove(f.path)
		f.path = ""
	}
}

var audioRateWarning sync.Once

func warnAudioRate(rate float32) {
	if rate != 1 {
		audioRateWarning.Do(func() { log.Print("nsgo: Linux audio players play at 1×") })
	}
}

func (p *linuxPlayer) command(req *driver.AudioRequest) {
	switch req.Command {
	case driver.AudioPreload:
		p.loadWanted = true
		p.prepare()
	case driver.AudioResume:
		if p.fromTrack() {
			p.resumeTrack()
			return
		}
		p.wanted = true
		if p.ended && !p.pendingSeek && !p.seeking {
			p.position, p.pendingSeek = 0, true
		}
		p.prepare()
		if !p.loaded && (p.loading || p.stream != 0) {
			p.buffer(true)
		}
		p.applySeek()
		p.play()
	case driver.AudioPause:
		p.pause()
	case driver.AudioSeek:
		if p.track != nil && p.stream == 0 {
			p.seeks++
			p.appliedSeeks, p.ended = p.seeks, false
			p.position = p.clampPosition(req.Position)
			if p.voice != nil {
				p.startTrack()
			}
			p.report(driver.AudioProgress)
			return
		}
		p.position, p.pendingSeek = req.Position, true
		p.seeks++
		p.applySeek()
	case driver.AudioSet:
		looped := p.loop != req.Loop
		if p.voice != nil && looped {
			p.trackPosition(time.Now())
		}
		p.volume, p.loop = req.Volume, req.Loop
		warnAudioRate(req.Rate)
		if v := p.voice; v != nil {
			v.setVolume(p.volume)
			// What the voice queued past now no longer follows the loop: a lap it should not play, or the end of a track it should repeat.
			if looped && (p.loop && v.eos || !p.loop && v.lapAhead()) {
				p.startTrack()
			}
		}
		if p.stream != 0 {
			p.setStreamVolume()
			if p.loop && p.loaded {
				p.decodeTrack()
			}
		}
	}
}

func (p *linuxPlayer) setStreamVolume() {
	var value gValue
	gValueInit(uintptr(unsafe.Pointer(&value)), gTypeDouble)
	gValueSetDouble(&value, float64(p.volume))
	gObjectSetProperty(p.stream, cstr("volume"), &value)
	gValueUnset(uintptr(unsafe.Pointer(&value)))
}

// setStreamLoop is a no-op for playbin because playbin loops by seeking in tick
// once EOS arrives (or hands over to a decoded track when ready).
func (p *linuxPlayer) setStreamLoop() {}

// loopFrom is where a looping player starts over: loopStart inside a known duration, else 0.
func (p *linuxPlayer) loopFrom() time.Duration {
	return driver.AudioLoopStart(p.loopStart, p.duration)
}

// loopAgain seeks an ended player to its loop start and plays on, counting the lap, as tick plays again once the seek lands; a stream that cannot seek ends instead.
func (p *linuxPlayer) loopAgain() bool {
	if !p.streamSeekable {
		return false
	}
	if p.duration == 0 {
		p.duration = p.queryDuration()
	}
	p.streamEnded = false
	p.laps++
	p.position, p.pendingSeek = p.loopFrom(), true
	p.lastProgress = time.Now()
	p.report(driver.AudioProgress)
	p.applySeek()
	return true
}

func (p *linuxPlayer) prepare() {
	if p.fromTrack() {
		p.decodeTrack()
		return
	}
	if p.loading || p.stream != 0 {
		return
	}
	p.loadDeadline = time.Now().Add(time.Minute)
	if path := p.file.name(); path != "" {
		p.openFile(path)
		return
	}
	if len(p.source.Data) == 0 && !strings.HasPrefix(p.source.URI, "data:") {
		p.openFile(p.source.URI)
		return
	}

	p.loading = true
	go func() {
		path, code, message := stageAudioPlayer(p.source)
		if path != "" && !p.file.hold(path) {
			return
		}
		p.owner.post(func() {
			if p.closed {
				return
			}
			p.loading = false
			if code != "" {
				p.fail(code, message)
				return
			}
			p.openFile(path)
		})
	}()
}

func stageAudioPlayer(source driver.ImageSource) (path, code, message string) {
	data, err := imageload.Load(context.Background(), source, nil)
	if err != nil {
		return "", driver.AudioNotSupported, err.Error()
	}
	f, err := os.CreateTemp("", "nsgo-audio-*")
	if err != nil {
		return "", driver.AudioFailed, err.Error()
	}
	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", driver.AudioFailed, err.Error()
	}
	return f.Name(), "", ""
}

func (p *linuxPlayer) openFile(source string) {
	pipeline, message := soundPipeline("playbin flags=audio+soft-volume")
	if pipeline == 0 {
		p.fail(driver.AudioUnavailable, message)
		return
	}
	bus := gstElementGetBus(pipeline)
	if bus == 0 {
		gstObjectUnref(pipeline)
		p.fail(driver.AudioUnavailable, "GStreamer did not create a message bus")
		return
	}
	var value gValue
	gValueInit(uintptr(unsafe.Pointer(&value)), gTypeString)
	gValueSetString(&value, cstr(trackURI(source)))
	gObjectSetProperty(pipeline, cstr("uri"), &value)
	gValueUnset(uintptr(unsafe.Pointer(&value)))

	p.stream, p.bus = pipeline, bus
	p.streamPlaying, p.streamSeekable, p.streamEnded = false, false, false
	p.setStreamVolume()
	if gstElementSetState(pipeline, gstStatePaused) == gstStateChangeFail {
		msg := soundBusError(bus)
		if msg == "" {
			msg = "GStreamer could not open the audio"
		}
		code := driver.AudioDecode
		if remoteAudio(p.source.URI) {
			code = driver.AudioNetwork
		}
		p.fail(code, msg)
	}
}

func (p *linuxPlayer) queryDuration() time.Duration {
	if p.stream == 0 {
		return 0
	}
	var dur int64
	if gstElementQueryDuration(p.stream, gstFormatTime, &dur) != 0 && dur > 0 {
		return time.Duration(dur)
	}
	return 0
}

func (p *linuxPlayer) querySeekable() bool {
	if p.stream == 0 {
		return false
	}
	q := gstQueryNewSeeking(gstFormatTime)
	if q == 0 {
		return false
	}
	defer gstMiniObjectUnref(q)
	if gstElementQuery(p.stream, q) == 0 {
		return false
	}
	var format, seekable int32
	var start, end int64
	gstQueryParseSeeking(q, &format, &seekable, &start, &end)
	return seekable != 0
}

func (p *linuxPlayer) tick(now time.Time) bool {
	if p.stream == 0 && p.track != nil {
		return p.tickTrack(now)
	}
	if p.stream == 0 {
		return p.loading
	}
	var asyncDone bool
	for p.bus != 0 {
		msg := gstBusPopFiltered(p.bus, gstMessageAny)
		if msg == 0 {
			break
		}
		kind := (*gstMessageHeader)(cPointer(msg)).kind
		switch kind {
		case gstMessageError:
			text := gstErrorText(msg)
			gstMiniObjectUnref(msg)
			code := driver.AudioDecode
			if remoteAudio(p.source.URI) {
				code = driver.AudioNetwork
			} else if p.loaded {
				code = driver.AudioFailed
			}
			p.fail(code, text)
			return false
		case gstMessageAsyncDone:
			asyncDone = true
		case gstMessageEOS:
			p.streamEnded = true
		}
		gstMiniObjectUnref(msg)
	}
	if !p.loaded {
		switch {
		case asyncDone:
			var nAudio int32
			gObjectGet(p.stream, uintptr(unsafe.Pointer(cstr("n-audio"))), uintptr(unsafe.Pointer(&nAudio)), 0)
			if nAudio <= 0 {
				p.fail(driver.AudioDecode, "the audio source has no audio stream")
				return false
			}
			p.loaded = true
			p.duration = p.queryDuration()
			p.streamSeekable = p.querySeekable()
			p.buffer(false)
			p.opened = true
			p.report(driver.AudioOpened)
			// A loop turned on while the stream loaded decodes its track now, as AudioSet does on a loaded stream.
			if p.loop {
				p.decodeTrack()
			}
			p.applySeek()
			p.play()
			if p.seeking {
				return true
			}
		case !now.Before(p.loadDeadline):
			code := driver.AudioDecode
			if remoteAudio(p.source.URI) {
				code = driver.AudioNetwork
			}
			p.fail(code, "the audio did not prepare within a minute")
			return false
		default:
			return true
		}
	} else if p.seeking {
		if !asyncDone {
			return true
		}
		p.seeking = false
		p.appliedSeeks = p.seekingNumber
		if p.pendingSeek {
			p.applySeek()
			return true
		}
		p.position = p.timestamp()
		p.report(driver.AudioProgress)
		p.play()
	}
	if !p.playing {
		return p.seeking
	}
	if p.streamEnded {
		if p.loop && p.track != nil {
			// The track decoded while the stream played: it takes over at the loop start.
			p.dropStream()
			p.laps++
			p.duration = p.trackDuration()
			p.position = p.loopFrom()
			p.report(driver.AudioProgress)
			p.startTrack()
			return true
		}
		if p.loop && p.loopAgain() {
			return true
		}
		p.ended, p.wanted, p.playing, p.streamPlaying, p.streamEnded = true, false, false, false, false
		gstElementSetState(p.stream, gstStatePaused)
		if p.duration == 0 {
			p.duration = p.timestamp()
		}
		p.position = p.duration
		p.report(driver.AudioEnded)
		return false
	}
	if p.duration == 0 {
		p.duration = p.queryDuration()
	}
	position := p.timestamp()
	wrapped := p.loop && position < p.position
	if wrapped {
		p.laps++
	}
	p.position = position
	if wrapped || now.Sub(p.lastProgress) >= max(16*time.Millisecond, p.interval) {
		p.lastProgress = now
		p.report(driver.AudioProgress)
	}
	return true
}

func (p *linuxPlayer) play() {
	if !p.wanted || !p.loaded || p.pendingSeek || p.seeking {
		return
	}
	p.streamEnded = false
	gstElementSetState(p.stream, gstStatePlaying)
	p.streamPlaying = true
	if !p.playing {
		p.ended = false
		p.playing = true
		p.lastProgress = time.Now()
		p.report(driver.AudioPlaying)
	}
}

func (p *linuxPlayer) pause() {
	if !p.playing && !p.wanted {
		return
	}
	if p.voice != nil {
		p.trackPosition(time.Now())
		p.stopTrack()
	}
	p.wanted, p.playing = false, false
	if p.stream != 0 {
		if p.loaded && !p.pendingSeek && !p.seeking {
			p.position = p.timestamp()
		}
		gstElementSetState(p.stream, gstStatePaused)
		p.streamPlaying = false
	}
	p.buffer(false)
	p.report(driver.AudioPaused)
}

func (p *linuxPlayer) applySeek() {
	if !p.loaded || p.seeking || !p.pendingSeek {
		return
	}
	if p.duration > 0 {
		p.position = min(p.position, p.duration)
	}
	p.pendingSeek = false
	p.seekingNumber = p.seeks
	if p.streamSeekable {
		p.seeking = true
		p.streamEnded = false
		if gstElementSeekSimple(p.stream, gstFormatTime, gstSeekFlush|gstSeekAccurate, p.position.Nanoseconds()) != 0 {
			return
		}
		p.seeking = false
	}
	p.appliedSeeks = p.seeks
	p.position = p.timestamp()
	p.report(driver.AudioProgress)
}

func (p *linuxPlayer) timestamp() time.Duration {
	if p.stream != 0 {
		var pos int64
		if gstElementQueryPosition(p.stream, gstFormatTime, &pos) != 0 && pos >= 0 {
			return time.Duration(pos)
		}
	}
	return p.position
}

func (p *linuxPlayer) report(phase driver.AudioPhase) {
	p.owner.audioReport(p.id, driver.AudioEvent{Phase: phase, Position: p.position, Duration: p.duration,
		Seeks: p.appliedSeeks, Laps: p.laps, Buffering: p.buffering})
}

func (p *linuxPlayer) buffer(buffering bool) {
	if p.buffering != buffering {
		p.buffering = buffering
		p.report(driver.AudioBuffer)
	}
}

func (p *linuxPlayer) fail(code, message string) {
	// A decoding that finishes later would bring back the player the app was told failed.
	p.cancelDecoding()
	p.stopTrack()
	p.buffer(false)
	p.pendingSeek = p.pendingSeek || p.seeking || p.position > 0
	p.resetStream()
	p.owner.audioReport(p.id, driver.AudioEvent{Phase: driver.AudioFailure, Error: code, Message: message})
}

func (p *linuxPlayer) cancelDecoding() {
	if p.decoding != nil {
		p.decoding()
		p.decoding = nil
	}
}

func (p *linuxPlayer) resetStream() {
	p.dropStream()
	p.loading, p.loaded, p.wanted, p.playing, p.ended, p.seeking = false, false, false, false, false, false
}

// dropStream frees the GStreamer playbin pipeline and leaves the player's state as it is.
func (p *linuxPlayer) dropStream() {
	if p.stream != 0 {
		freePlayerPipeline(p.stream, p.bus)
		p.stream, p.bus = 0, 0
	}
	p.seeking, p.streamPlaying, p.streamSeekable, p.streamEnded = false, false, false, false
}

// freePlayerPipeline stops a GStreamer playbin pipeline and releases its bus and pipeline references.
func freePlayerPipeline(stream, bus uintptr) {
	if stream != 0 {
		gstElementSetState(stream, gstStateNull)
	}
	if bus != 0 {
		gstObjectUnref(bus)
	}
	if stream != 0 {
		gstObjectUnref(stream)
	}
}

// freeMediaStream stops a GtkMediaFile, closes its file and lets it go.
func freeMediaStream(stream uintptr) {
	gtkMediaStreamSetPlaying(stream, 0)
	gtkMediaFileClear(stream)
	gObjectUnref(stream)
}

func (p *linuxPlayer) close() {
	p.closed = true
	p.cancelDecoding()
	p.stopTrack()
	if p.track != nil {
		p.track.close()
	}
	p.resetStream()
	p.file.close()
}
