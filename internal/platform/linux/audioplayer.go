//go:build linux && !android

package linux

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

// linuxPlayer plays one source through GtkMediaFile on the GTK thread.
type linuxPlayer struct {
	owner  *window
	id     uint32
	source driver.ImageSource
	stream uintptr
	file   audioPlayerFile

	volume   float32
	loop     bool
	interval time.Duration

	loading, loaded, wanted, playing, ended, buffering, closed bool
	loadDeadline, lastProgress                                 time.Time
	position, duration                                         time.Duration
	pendingSeek, seeking                                       bool
	seeks, seekingNumber, appliedSeeks, laps                   uint32
}

// audioPlayerFile owns bytes staged on a worker for GtkMediaFile, whose
// GStreamer backend accepts files and URIs. Closing the player can race
// the write or the callback that opens the finished file.
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
		audioRateWarning.Do(func() { log.Print("nsgo: Linux audio players play at 1× with GtkMediaFile") })
	}
}

func (p *linuxPlayer) command(req *driver.AudioRequest) {
	switch req.Command {
	case driver.AudioPreload:
		p.prepare()
	case driver.AudioResume:
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
		p.position, p.pendingSeek = req.Position, true
		p.seeks++
		p.applySeek()
	case driver.AudioSet:
		p.volume, p.loop = req.Volume, req.Loop
		warnAudioRate(req.Rate)
		if p.stream != 0 {
			gtkMediaStreamSetVolume(p.stream, float64(p.volume))
			gtkMediaStreamSetLoop(p.stream, boolInt(p.loop))
		}
	}
}

func (p *linuxPlayer) prepare() {
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
	file := videoFile(source)
	stream := gtkMediaFileNewForFile(file)
	gObjectUnref(file)
	if stream == 0 {
		p.fail(driver.AudioUnavailable, "GTK did not create a media stream")
		return
	}
	p.stream = stream
	gtkMediaStreamSetVolume(stream, float64(p.volume))
	gtkMediaStreamSetLoop(stream, boolInt(p.loop))
}

func (p *linuxPlayer) tick(now time.Time) bool {
	if p.stream == 0 {
		return p.loading
	}
	if gerr := gtkMediaStreamGetError(p.stream); gerr != 0 {
		code := driver.AudioDecode
		if remoteAudio(p.source.URI) {
			code = driver.AudioNetwork
		} else if p.loaded {
			code = driver.AudioFailed
		}
		p.fail(code, mediaError(goString((*gError)(cPointer(gerr)).message)))
		return false
	}
	if !p.loaded {
		switch {
		case gtkMediaStreamIsPrepared(p.stream) != 0 && gtkMediaStreamHasAudio(p.stream) != 0:
			p.loaded = true
			p.duration = micros(max(0, gtkMediaStreamGetDuration(p.stream)))
			p.buffer(false)
			p.report(driver.AudioOpened)
			p.applySeek()
			p.play()
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
	}
	if p.seeking {
		if gtkMediaStreamIsSeeking(p.stream) != 0 {
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
		return false
	}
	if gtkMediaStreamGetEnded(p.stream) != 0 {
		p.ended, p.wanted, p.playing = true, false, false
		p.position = p.duration
		p.report(driver.AudioEnded)
		return false
	}
	if gtkMediaStreamGetPlaying(p.stream) == 0 {
		p.pause()
		return false
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
	gtkMediaStreamSetPlaying(p.stream, 1)
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
	p.wanted, p.playing = false, false
	if p.stream != 0 {
		gtkMediaStreamSetPlaying(p.stream, 0)
		if p.loaded && !p.pendingSeek && !p.seeking {
			p.position = p.timestamp()
		}
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
	if gtkMediaStreamIsSeekable(p.stream) != 0 {
		p.seeking = true
		gtkMediaStreamSeek(p.stream, p.position.Microseconds())
		return
	}
	p.appliedSeeks = p.seeks
	p.position = p.timestamp()
	p.report(driver.AudioProgress)
}

func (p *linuxPlayer) timestamp() time.Duration {
	return micros(max(0, gtkMediaStreamGetTimestamp(p.stream)))
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
	p.buffer(false)
	p.pendingSeek = p.pendingSeek || p.seeking || p.position > 0
	p.resetStream()
	p.owner.audioReport(p.id, driver.AudioEvent{Phase: driver.AudioFailure, Error: code, Message: message})
}

func (p *linuxPlayer) resetStream() {
	if p.stream != 0 {
		gtkMediaStreamSetPlaying(p.stream, 0)
		gtkMediaFileClear(p.stream)
		gObjectUnref(p.stream)
		p.stream = 0
	}
	p.loading, p.loaded, p.wanted, p.playing, p.ended, p.seeking = false, false, false, false, false, false
}

func (p *linuxPlayer) close() {
	p.closed = true
	p.resetStream()
	p.file.close()
}
