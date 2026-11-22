//go:build windows

package windows

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

const (
	mfMediaAudioOnly = 1
	meSetPreload     = 10
	meIsSeeking      = 15
	meHasAudio       = 39
	meSetByteStream  = 45 // IMFMediaEngineEx::SetSourceFromByteStream
	mePreloadAuto    = 4
)

var iidMediaEngineEx = parseGUID("83015ead-b1e6-40d0-a98a-37145ffe1ad1")

// winAudioPlayer owns an audio-only Media Foundation engine. Its events
// and timers take one hop to the window thread before reading native state.
type winAudioPlayer struct {
	a                   *windowAudio
	id                  uint32
	source              driver.ImageSource
	media               *comObject
	serial              uint64
	cancel              context.CancelFunc
	loadTimer, ticker   *time.Timer
	volume, rate        float32
	loop, opened, ended bool
	wanted, playing     bool
	playSent, buffering bool
	seekQueued, seeking bool
	seeks, laps         uint32
	position, duration  time.Duration
	interval            time.Duration
	// loopStart is where a loop starts over once the first pass ends.
	loopStart time.Duration
}

func (p *winAudioPlayer) apply(r *driver.AudioRequest) {
	switch r.Command {
	case driver.AudioPreload:
		p.preload()
	case driver.AudioResume:
		p.wanted = true
		if p.opened {
			p.start()
		} else {
			p.preload()
		}
	case driver.AudioPause:
		p.pause()
	case driver.AudioSeek:
		p.seeks++
		p.seekQueued, p.ended, p.position = true, false, r.Position
		p.seek()
	case driver.AudioSet:
		p.volume, p.rate, p.loop = r.Volume, r.Rate, r.Loop
		p.applySettings()
	}
}

func (p *winAudioPlayer) preload() {
	if p.media != nil || p.cancel != nil {
		return
	}
	p.serial++
	serial := p.serial
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.loadTimer = time.AfterFunc(time.Minute, func() {
		p.a.w.post(func() {
			if p.serial == serial && !p.opened {
				p.fail(p.sourceError(), "timed out opening the audio source")
			}
		})
	})
	go func(src driver.ImageSource) {
		uri, data, err := prepareAudioInput(ctx, src)
		if ctx.Err() != nil {
			return
		}
		p.a.w.post(func() {
			if p.serial != serial {
				return
			}
			if err != nil {
				p.fail(driver.AudioNotSupported, err.Error())
				return
			}
			p.open(uri, data, serial)
		})
	}(p.source)
}

// prepareAudioInput resolves local paths like video and leaves remote
// sources to Media Foundation so it can stream them. In-memory sources
// get a file name for Media Foundation's byte-stream handlers.
func prepareAudioInput(ctx context.Context, src driver.ImageSource) (string, []byte, error) {
	if len(src.Data) > 0 || strings.HasPrefix(src.URI, "data:") {
		data, err := imageload.Load(ctx, src, nil)
		if err != nil {
			return "", nil, err
		}
		if len(data) == 0 {
			return "", nil, errors.New("the audio source is empty")
		}
		return "file:///nsgo-audio" + audioExtension(data, src.URI), data, nil
	}
	if src.URI == "" || !utf8.ValidString(src.URI) || strings.ContainsRune(src.URI, 0) {
		return "", nil, errors.New("the audio location is invalid")
	}
	if remoteAudio(src.URI) {
		return src.URI, nil, nil
	}
	uri := src.URI
	if !strings.HasPrefix(strings.ToLower(uri), "file:") {
		var ok bool
		uri, ok = videoURL(uri, imageload.ResourceRoot)
		if !ok {
			return "", nil, errors.New("cannot resolve the audio location")
		}
	}
	u, err := url.Parse(uri)
	if err != nil {
		return "", nil, err
	}
	path, err := driver.FileURLPath(u, true)
	if err != nil {
		return "", nil, err
	}
	if _, err := os.Stat(path); err != nil {
		return "", nil, err
	}
	return uri, nil, nil
}

func (p *winAudioPlayer) open(uri string, data []byte, serial uint64) {
	media, err := p.makeEngine(serial)
	if err != nil {
		p.fail(driver.AudioUnavailable, err.Error())
		return
	}
	p.media = media
	media.call(meSetPreload, mePreloadAuto)
	p.applySettings()
	if err := setAudioSource(media, uri, data); err != nil {
		p.fail(p.sourceError(), err.Error())
	}
}

func (p *winAudioPlayer) makeEngine(serial uint64) (*comObject, error) {
	audioOutputOpens.Add(1)
	if err := initCOM(); err != nil {
		return nil, err
	}
	if err := mfStarted(); err != nil {
		return nil, err
	}
	notify := newNotify(&iidMediaEngineNotify, func(event, param1, param2 uintptr) {
		p.a.w.post(func() {
			if p.serial == serial && p.media != nil {
				p.notified(uint32(event), param1, param2)
			}
		})
	})
	defer notify.release()
	var attributes *comObject
	if hr, _, _ := mfCreateAttributes.Call(uintptr(unsafe.Pointer(&attributes)), 1); failed(hr) || attributes == nil {
		return nil, hresultError("no audio media attributes", hr)
	}
	defer attributes.release()
	if hr := attributes.call(attributesSetUnknown, uintptr(unsafe.Pointer(&mfMediaEngineCallback)), notify.ptr()); failed(hr) {
		return nil, hresultError("cannot observe the audio player", hr)
	}
	return createMediaEngine(mfMediaAudioOnly, attributes)
}

func setAudioSource(media *comObject, uri string, data []byte) error {
	name, _, _ := sysAllocString.Call(uintptr(unsafe.Pointer(wide(uri))))
	if name == 0 {
		return errors.New("cannot allocate the audio source name")
	}
	defer sysFreeString.Call(name)
	var hr uintptr
	if len(data) == 0 {
		hr = media.call(meSetSource, name)
	} else {
		stream, err := mediaByteStream(data, uri)
		if err != nil {
			return err
		}
		defer stream.release()
		ext := media.query(&iidMediaEngineEx)
		if ext == nil {
			return errors.New("the media engine cannot read audio from a byte stream")
		}
		defer ext.release()
		hr = ext.call(meSetByteStream, stream.ptr(), name)
	}
	if failed(hr) {
		return hresultError("cannot open the audio source", hr)
	}
	return nil
}

func (p *winAudioPlayer) applySettings() {
	if p.media != nil {
		p.media.callDouble(meSetVolume, float64(p.volume))
		p.media.callDouble(meSetDefaultPlaybackRate, float64(p.rate))
		p.media.callDouble(meSetPlaybackRate, float64(p.rate))
	}
}

func (p *winAudioPlayer) start() {
	if p.ended {
		p.ended, p.seekQueued, p.position = false, true, 0
		p.seek()
	}
	if p.playing && !p.buffering {
		p.report(driver.AudioEvent{Phase: driver.AudioPlaying})
		return
	}
	if p.playSent {
		return
	}
	p.applySettings()
	if hr := p.media.call(mePlay); failed(hr) {
		p.fail(driver.AudioFailed, hresultError("cannot start audio playback", hr).Error())
		return
	}
	p.playSent = true
}

func (p *winAudioPlayer) pause() {
	p.wanted, p.playSent, p.playing = false, false, false
	if p.media != nil {
		p.media.call(mePause)
	}
	p.stopTicker()
	p.setBuffering(false)
	p.reportPosition(driver.AudioPaused)
}

func (p *winAudioPlayer) seek() {
	if !p.opened || !p.seekQueued {
		return
	}
	if p.duration > 0 {
		p.position = min(p.position, p.duration)
	}
	p.seekQueued = false
	hr := p.media.callDouble(meSetCurrentTime, p.position.Seconds())
	p.seeking = !failed(hr) && p.media.call(meIsSeeking) != 0
	if !p.seeking {
		p.reportPosition(driver.AudioProgress)
	}
}

func (p *winAudioPlayer) notified(event uint32, param1, param2 uintptr) {
	switch event {
	case mediaLoadedMetadata:
		if p.opened {
			return
		}
		if p.media.call(meHasAudio) == 0 {
			p.fail(driver.AudioDecode, "the source has no audio")
			return
		}
		p.opened = true
		p.stopLoading()
		p.duration = max(0, seconds(finite(p.media.getDouble(meGetDuration))))
		p.report(driver.AudioEvent{Phase: driver.AudioOpened, Duration: p.duration})
		p.seekQueued = p.seekQueued || p.position > 0
		p.seek()
		if p.wanted {
			p.start()
		}
	case mediaPlaying:
		if p.wanted && p.playSent {
			if !p.playing {
				p.tickLater()
			}
			p.playing = true
			p.report(driver.AudioEvent{Phase: driver.AudioPlaying})
		}
		p.setBuffering(false)
	case mediaCanPlay, mediaBufferEnded:
		p.setBuffering(false)
	case mediaWaiting, mediaBufferStarted:
		if p.wanted {
			p.setBuffering(true)
		}
	case mediaSeeked:
		if p.seeking && p.media.call(meIsSeeking) == 0 {
			p.seeking = false
			p.reportPosition(driver.AudioProgress)
		}
	case mediaEnded:
		if p.wanted && p.media.call(meIsEnded) != 0 {
			p.finished()
		}
	case mediaError:
		code := driver.AudioFailed
		if !p.opened {
			code = p.sourceError()
		}
		switch param1 {
		case 2:
			code = driver.AudioNetwork
		case 3, 4:
			code = driver.AudioDecode
		case 5:
			code = driver.AudioNotSupported
		}
		p.fail(code, fmt.Sprintf("audio playback failed (Media Foundation error %d, HRESULT 0x%08X)", param1, uint32(param2)))
	}
}

func (p *winAudioPlayer) finished() {
	p.playing, p.playSent, p.ended = false, false, true
	p.stopTicker()
	p.setBuffering(false)
	if p.loop {
		// A loop starts over at its LoopStart, with the short gap of a seek.
		p.laps++
		p.ended, p.seekQueued, p.position = false, true, driver.AudioLoopStart(p.loopStart, p.duration)
		p.seek()
		p.start()
		return
	}
	p.wanted = false
	p.report(driver.AudioEvent{Phase: driver.AudioEnded})
}

func (p *winAudioPlayer) tickLater() {
	interval := p.interval
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	var timer *time.Timer
	timer = time.AfterFunc(max(interval, 16*time.Millisecond), func() {
		p.a.w.post(func() {
			if p.ticker != timer {
				return
			}
			p.reportPosition(driver.AudioProgress)
			p.tickLater()
		})
	})
	p.ticker = timer
}

func (p *winAudioPlayer) reportPosition(phase driver.AudioPhase) {
	if p.opened && !p.seekQueued && !p.seeking {
		p.position = max(0, seconds(finite(p.media.getDouble(meGetCurrentTime))))
	}
	p.report(driver.AudioEvent{Phase: phase, Position: p.position, Duration: p.duration, Seeks: p.seeks, Laps: p.laps})
}

func (p *winAudioPlayer) report(ev driver.AudioEvent) { p.a.report(p.id, ev) }

func (p *winAudioPlayer) setBuffering(waiting bool) {
	if p.buffering != waiting {
		p.buffering = waiting
		p.report(driver.AudioEvent{Phase: driver.AudioBuffer, Buffering: waiting})
	}
}

func (p *winAudioPlayer) sourceError() string {
	if remoteAudio(p.source.URI) {
		return driver.AudioNetwork
	}
	return driver.AudioDecode
}

func (p *winAudioPlayer) fail(code, message string) {
	p.close()
	p.report(audioFailure(code, message))
}

func (p *winAudioPlayer) stopLoading() {
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	if p.loadTimer != nil {
		p.loadTimer.Stop()
		p.loadTimer = nil
	}
}

func (p *winAudioPlayer) stopTicker() {
	if p.ticker != nil {
		p.ticker.Stop()
		p.ticker = nil
	}
}

func (p *winAudioPlayer) close() {
	p.serial++
	p.stopLoading()
	p.stopTicker()
	if p.media != nil {
		p.media.call(meShutdown)
		p.media.release()
		p.media = nil
	}
	p.opened, p.playing, p.playSent, p.wanted = false, false, false, false
	p.buffering, p.seeking = false, false
}
