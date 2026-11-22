//go:build linux && !android

package linux

// GtkMediaFile loops with a seek at the end of the track, which drops the output for a moment. A looping player decodes its whole track instead, within driver.MaxGaplessLoopBytes and driver.MaxGaplessLoopDuration, and plays it through the sound mixer, queuing sample-contiguous pieces that wrap from the end of the track to the loop start; once it has the track it plays every pass from it. A track over those limits streams through a decoder of its own instead (audiostream.go), and one GStreamer cannot read at all loops through GtkMediaFile.

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// trackPiece is the size of the pieces a track voice queues.
const trackPiece = soundRate / 20

// trackRestartAfter is how long a track voice plays before a closed mixer restarts it in a new one rather than failing the player.
const trackRestartAfter = time.Second

// trackVoice plays a player's decoded track through the mixer, from where it started on.
type trackVoice struct {
	bin, src, pad, appsrc, volume uintptr
	// started is when, on the Go clock, the voice's first frame reaches the mixer; queued counts the frames queued since.
	started time.Time
	queued  int64
	// next is the track's frame the voice queues next.
	next int
	// pieces are the queued pieces whose start has not played yet, and the one playing first; played counts those whose start played.
	pieces []trackVoicePiece
	played int
	// eos says the voice queued the end of the track; gone that the mixer closed it; lap that the next piece it queues starts a lap.
	eos, gone, lap bool
}

// trackVoicePiece is a queued piece of a track: frames from the track's frame from, at frame at of the voice, the first of a lap when lap.
type trackVoicePiece struct {
	at          int64
	from, count int
	lap         bool
}

// startTrack adds a voice for a player's track at volume, its first frame shortly after the mixer's current block. It is nil when the mixer does not play yet or the voice cannot link.
func (m *soundMixer) startTrack(volume float32) *trackVoice {
	if !m.playing() {
		return nil
	}
	desc := "appsrc name=voice is-live=true format=time min-latency=0 max-latency=0 caps=\"" + soundCaps + "\" ! volume name=volume" +
		fmt.Sprintf(" volume=%g", volume)
	v := &trackVoice{}
	var gerr uintptr
	v.bin = gstParseBin(cstr(desc), 1, &gerr)
	if gerr != 0 || v.bin == 0 {
		if v.bin != 0 {
			gstObjectUnref(v.bin)
		}
		log.Print("nsgo: audio player: ", soundError(gerr))
		return nil
	}
	if gstBinAdd(m.pipeline, v.bin) == 0 {
		gstObjectUnref(v.bin)
		return nil
	}
	v.src, v.pad = gstElementGetStaticPad(v.bin, cstr("src")), gstElementGetRequestPad(m.mixer, cstr("sink_%u"))
	v.appsrc, v.volume = gstBinGetByName(v.bin, cstr("voice")), gstBinGetByName(v.bin, cstr("volume"))
	start := gstElementRunningTime(m.pipeline)
	const lead = 20 * time.Millisecond
	if v.src == 0 || v.pad == 0 || v.appsrc == 0 || v.volume == 0 || start == math.MaxUint64 {
		m.removeTrack(v)
		return nil
	}
	gstPadSetOffset(v.src, int64(start)+int64(lead))
	if gstPadLink(v.src, v.pad) != 0 || gstElementSyncState(v.bin) == 0 {
		m.removeTrack(v)
		return nil
	}
	v.started = time.Now().Add(lead)
	m.tracks = append(m.tracks, v)
	return v
}

func (m *soundMixer) removeTrack(v *trackVoice) {
	for i, t := range m.tracks {
		if t == v {
			m.tracks = append(m.tracks[:i], m.tracks[i+1:]...)
			break
		}
	}
	gstElementSetState(v.bin, gstStateNull)
	if v.src != 0 && v.pad != 0 {
		gstPadUnlink(v.src, v.pad)
	}
	for _, o := range []uintptr{v.src, v.appsrc, v.volume} {
		if o != 0 {
			gstObjectUnref(o)
		}
	}
	if v.pad != 0 {
		gstElementReleasePad(m.mixer, v.pad)
		gstObjectUnref(v.pad)
	}
	gstBinRemove(m.pipeline, v.bin)
	v.gone = true
}

func (v *trackVoice) setVolume(volume float32) {
	var value gValue
	gValueInit(uintptr(unsafe.Pointer(&value)), gTypeDouble)
	gValueSetDouble(&value, float64(volume))
	gObjectSetProperty(v.volume, cstr("volume"), &value)
	gValueUnset(uintptr(unsafe.Pointer(&value)))
}

// trackTime is the time of n frames at the mixer's rate.
func trackTime(n int64) time.Duration {
	return time.Duration(n * int64(time.Second) / soundRate)
}

// fill queues the track from v.next until soundLoopAhead past now, wrapping to loopFrom at the end while loop, or queuing the end of the stream otherwise. A streamed track may have no frames ready yet, as after its seek back to the loop start: the voice queues them on a later tick, from what it already queued.
func (v *trackVoice) fill(now time.Time, track trackFrames, loop bool, loopFrom int) bool {
	for !v.eos && trackTime(v.queued) < now.Sub(v.started)+soundLoopAhead {
		pcm, end, ok := track.frames(v.next, trackPiece)
		if !ok {
			return false
		}
		if end {
			if !loop {
				v.eos = true
				return gstAppSrcEndOfStream(v.appsrc) == 0
			}
			if v.lap {
				// The loop has no frames past its start.
				return true
			}
			v.next, v.lap = loopFrom, true
			continue
		}
		n := len(pcm) / soundFrameBytes
		if n == 0 {
			return true
		}
		start := trackTime(v.queued)
		if !pushSoundBuffer(v.appsrc, pcm, uint64(start), trackTime(v.queued+int64(n))-start) {
			return false
		}
		v.pieces = append(v.pieces, trackVoicePiece{at: v.queued, from: v.next, count: n, lap: v.lap})
		v.queued += int64(n)
		v.next += n
		v.lap = false
	}
	return true
}

// advance is the track's frame the voice plays at now, the laps it started since the last call, and whether it played the end of a track it does not loop.
func (v *trackVoice) advance(now time.Time) (frame int, laps uint32, done bool) {
	elapsed := max(0, int64(now.Sub(v.started))*soundRate/int64(time.Second))
	for v.played < len(v.pieces) && v.pieces[v.played].at <= elapsed {
		if v.pieces[v.played].lap {
			laps++
		}
		v.played++
	}
	if v.played > 1 {
		v.pieces = v.pieces[v.played-1:]
		v.played = 1
	}
	if v.played == 0 {
		if len(v.pieces) == 0 {
			return v.next, laps, v.eos
		}
		return v.pieces[0].from, laps, false
	}
	p := v.pieces[0]
	return p.from + int(min(elapsed-p.at, int64(p.count))), laps, v.eos && elapsed >= v.queued
}

// lapAhead says the voice queued a lap that has not started playing.
func (v *trackVoice) lapAhead() bool {
	for _, p := range v.pieces[v.played:] {
		if p.lap {
			return true
		}
	}
	return false
}

// fromTrack says the player plays its decoded track through the mixer, or waits for the track it decodes instead of opening GtkMediaFile.
func (p *linuxPlayer) fromTrack() bool {
	return p.track != nil || p.stream == 0 && !p.loading && (p.decoding != nil || p.loop && !p.noTrack)
}

// trackDecodeLimit is the longest track a looping player decodes whole; a longer one streams.
var trackDecodeLimit = driver.MaxGaplessLoopDuration

// decodeTrack decodes the player's whole track on a worker for a gapless loop, or opens a decoder that streams a track too large or too long to decode whole, unless it has the track, is decoding it or could not. A stream that plays meanwhile plays on.
func (p *linuxPlayer) decodeTrack() {
	if p.track != nil || p.decoding != nil || p.noTrack {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.decoding = cancel
	source, limit := p.source, trackDecodeLimit
	// A stream known to be longer than the limit skips decoding it whole.
	whole := p.stream == 0 || !p.loaded || p.duration <= limit
	go func() {
		var track trackFrames
		tooLarge := !whole
		if whole {
			b, code, _ := readSoundBytes(ctx, source, driver.MaxGaplessLoopBytes)
			tooLarge = code == driver.AudioQuotaExceeded
			if code == "" {
				pcm, cut, message := decodeSound(ctx, b, limit)
				tooLarge = cut
				if !cut && message == "" {
					track = pcmTrack(pcm)
				}
			}
		}
		if tooLarge && ctx.Err() == nil {
			if uri := p.trackSourceURI(source); uri != "" {
				if d, _ := openTrackDecoder(uri); d != nil {
					track = d
				}
			}
		}
		if ctx.Err() != nil {
			if track != nil {
				track.close()
			}
			return
		}
		p.owner.post(func() {
			// A closed player cancelled its decoding.
			if p.closed || ctx.Err() != nil {
				if track != nil {
					track.close()
				}
				return
			}
			cancel()
			p.decoding = nil
			if track == nil {
				// GtkMediaFile plays the track, from a seek to where the player is.
				p.noTrack = true
				if p.stream == 0 && (p.wanted || p.loadWanted) {
					p.pendingSeek = p.pendingSeek || p.position > 0
					p.prepare()
				}
			} else {
				p.adopt(track)
			}
			if p.audio != nil {
				p.audio.ensureTick()
			}
		})
	}()
}

// trackSourceURI is the URI a track decoder streams source from, staging bytes and data URLs in the player's file as GtkMediaFile does; it runs on the decoding worker.
func (p *linuxPlayer) trackSourceURI(source driver.ImageSource) string {
	if len(source.Data) == 0 && !strings.HasPrefix(source.URI, "data:") {
		return trackURI(source.URI)
	}
	path := p.file.name()
	if path == "" {
		var code string
		if path, code, _ = stageAudioPlayer(source); code != "" || !p.file.hold(path) {
			return ""
		}
	}
	return trackURI(path)
}

// adopt takes the decoded track. A stream that plays hands over to it at its end, the next loop start; otherwise the track takes over at once from the stream's position.
func (p *linuxPlayer) adopt(track trackFrames) {
	p.track = track
	if p.stream != 0 && p.wanted {
		p.setStreamLoop()
		return
	}
	if p.stream != 0 {
		if p.loaded && !p.pendingSeek && !p.seeking {
			p.position = p.timestamp()
		}
		p.dropStream()
	}
	p.loaded, p.duration = true, track.length()
	p.position = p.clampPosition(p.position)
	sought := p.appliedSeeks != p.seeks
	p.pendingSeek, p.seeking, p.appliedSeeks = false, false, p.seeks
	if !p.opened {
		p.opened = true
		p.report(driver.AudioOpened)
	}
	if sought {
		p.report(driver.AudioProgress)
	}
	if p.wanted {
		p.buffer(true)
		p.startTrack()
	}
}

// resumeTrack plays the decoded track, or waits for it to decode.
func (p *linuxPlayer) resumeTrack() {
	if p.ended {
		p.ended, p.position = false, 0
	}
	if p.stream != 0 {
		if p.loaded && !p.pendingSeek && !p.seeking {
			p.position = p.timestamp()
		}
		p.dropStream()
	}
	p.wanted = true
	if p.track == nil {
		p.decodeTrack()
		p.buffer(true)
		return
	}
	p.loaded = true
	p.startTrack()
}

// startTrack plays the track from the player's position in a new mixer voice, which replaces a playing one. The mixer may not play yet: tick starts the voice once it does.
func (p *linuxPlayer) startTrack() {
	p.stopTrack()
	a := p.audio
	a.checkOutput()
	if a.mixer == nil {
		a.mixer = newSoundMixer(a.playbackEnded)
	}
	if a.mixer == nil {
		p.fail(driver.AudioUnavailable, "GStreamer has no audio output")
		return
	}
	a.lastPlay = time.Now()
	v := a.mixer.startTrack(p.volume)
	if v == nil {
		p.buffer(true)
		return
	}
	// The track's length can differ from the stream's that played before it; the voice wraps at the track's.
	p.duration = p.track.length()
	from := p.clampPosition(p.position)
	if p.loop && p.duration > 0 && from >= p.duration {
		from = p.loopFrom()
	}
	v.next = int(int64(from) * soundRate / int64(time.Second))
	p.voice, p.position = v, from
	if !p.feed(time.Now()) {
		return
	}
	p.buffer(false)
	p.lastProgress = time.Now()
	if !p.playing {
		p.playing = true
		p.report(driver.AudioPlaying)
	}
}

// feed queues the voice ahead of its playback.
func (p *linuxPlayer) feed(now time.Time) bool {
	loopFrom := int(int64(p.loopFrom()) * soundRate / int64(time.Second))
	if !p.voice.fill(now, p.track, p.loop, loopFrom) {
		p.fail(driver.AudioFailed, "GStreamer did not play the player's audio")
		return false
	}
	return true
}

// trackPosition reads the voice's position and counts the laps it started.
func (p *linuxPlayer) trackPosition(now time.Time) (lapped, done bool) {
	frame, laps, done := p.voice.advance(now)
	p.position = p.clampPosition(trackTime(int64(frame)))
	p.laps += laps
	return laps > 0, done
}

// clampPosition keeps a position within a known duration; a streamed track may not know its own.
func (p *linuxPlayer) clampPosition(position time.Duration) time.Duration {
	if p.duration > 0 {
		return min(position, p.duration)
	}
	return position
}

func (p *linuxPlayer) stopTrack() {
	if p.voice == nil {
		return
	}
	if !p.voice.gone && p.audio != nil && p.audio.mixer != nil {
		p.audio.mixer.removeTrack(p.voice)
	}
	p.voice = nil
}

// restartTrack moves a playing voice to the player's position.
func (p *linuxPlayer) restartTrack() {
	if p.voice != nil || p.wanted {
		p.startTrack()
	}
}

// tickTrack plays the decoded track on the audio tick: it starts the voice once the mixer plays, keeps it queued, and reports progress, laps and the end.
func (p *linuxPlayer) tickTrack(now time.Time) bool {
	if p.voice == nil {
		if p.wanted && p.track != nil {
			p.startTrack()
		}
		return p.wanted
	}
	if v := p.voice; v.gone {
		// The mixer closed under the voice, as when the system's output device changes. A new mixer plays on from where the voice was; a voice the output closed within its first second fails, so a broken output does not restart it on every tick.
		p.trackPosition(now)
		p.voice = nil
		if now.Sub(v.started) < trackRestartAfter {
			p.fail(driver.AudioFailed, "the audio output closed")
			return false
		}
		if p.wanted {
			p.startTrack()
		}
		return p.wanted
	}
	lapped, done := p.trackPosition(now)
	if done {
		p.stopTrack()
		p.ended, p.wanted, p.playing = true, false, false
		p.position = p.duration
		p.report(driver.AudioEnded)
		return false
	}
	if !p.feed(now) {
		return false
	}
	if lapped || now.Sub(p.lastProgress) >= max(16*time.Millisecond, p.interval) {
		p.lastProgress = now
		p.report(driver.AudioProgress)
	}
	return true
}
