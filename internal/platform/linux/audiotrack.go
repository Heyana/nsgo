//go:build linux && !android

package linux

// GtkMediaFile loops with a seek at the end of the track, which drops the output for a moment. A looping player decodes its whole track instead, within driver.MaxGaplessLoopBytes and driver.MaxGaplessLoopDuration, and plays it through the sound mixer, queuing sample-contiguous pieces that wrap from the end of the track to the loop start; once it has the track it plays every pass from it. A track it cannot decode whole loops through GtkMediaFile.

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// trackPiece is the size of the pieces a track voice queues.
const trackPiece = soundRate / 20

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
	// eos says the voice queued the end of the track; gone that the mixer closed it.
	eos, gone bool
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

// fill queues the track from v.next until soundLoopAhead past now, wrapping to loopFrom at the end while loop, or queuing the end of the stream otherwise.
func (v *trackVoice) fill(now time.Time, pcm []byte, loop bool, loopFrom int) bool {
	frames := len(pcm) / soundFrameBytes
	for !v.eos && trackTime(v.queued) < now.Sub(v.started)+soundLoopAhead {
		lap := false
		if v.next >= frames {
			if !loop {
				v.eos = true
				return gstAppSrcEndOfStream(v.appsrc) == 0
			}
			v.next, lap = loopFrom, true
		}
		n := min(trackPiece, frames-v.next)
		start := trackTime(v.queued)
		if !pushSoundBuffer(v.appsrc, pcm[v.next*soundFrameBytes:(v.next+n)*soundFrameBytes], uint64(start), trackTime(v.queued+int64(n))-start) {
			return false
		}
		v.pieces = append(v.pieces, trackVoicePiece{at: v.queued, from: v.next, count: n, lap: lap})
		v.queued += int64(n)
		v.next += n
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

// decodeTrack decodes the player's whole track on a worker for a gapless loop, unless it has it, is decoding it or could not. A stream that plays meanwhile plays on.
func (p *linuxPlayer) decodeTrack() {
	if p.track != nil || p.decoding != nil || p.noTrack {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.decoding = cancel
	source := p.source
	go func() {
		b, code, _ := readSoundBytes(ctx, source, driver.MaxGaplessLoopBytes)
		var pcm []byte
		if code == "" {
			var cut bool
			var message string
			if pcm, cut, message = decodeSound(ctx, b, driver.MaxGaplessLoopDuration); cut || message != "" {
				pcm = nil
			}
		}
		if ctx.Err() != nil {
			return
		}
		p.owner.post(func() {
			// A closed player cancelled its decoding.
			if p.closed || ctx.Err() != nil {
				return
			}
			cancel()
			p.decoding = nil
			if pcm == nil {
				// GtkMediaFile plays the track, from a seek to where the player is.
				p.noTrack = true
				if p.stream == 0 && (p.wanted || p.loadWanted) {
					p.pendingSeek = p.pendingSeek || p.position > 0
					p.prepare()
				}
			} else {
				p.adopt(pcm)
			}
			if p.audio != nil {
				p.audio.ensureTick()
			}
		})
	}()
}

// adopt takes the decoded track. A stream that plays hands over to it at its end, the next loop start; otherwise the track takes over at once from the stream's position.
func (p *linuxPlayer) adopt(pcm []byte) {
	p.track = pcm
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
	p.loaded, p.duration = true, soundDuration(pcm)
	p.position = min(p.position, p.duration)
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
	from := min(p.position, p.duration)
	if p.loop && from >= p.duration {
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
		p.fail(driver.AudioFailed, "GStreamer did not take the player's audio")
		return false
	}
	return true
}

// trackPosition reads the voice's position and counts the laps it started.
func (p *linuxPlayer) trackPosition(now time.Time) (lapped, done bool) {
	frame, laps, done := p.voice.advance(now)
	p.position = min(trackTime(int64(frame)), p.duration)
	p.laps += laps
	return laps > 0, done
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
	if p.voice.gone {
		p.voice = nil
		p.fail(driver.AudioFailed, "the audio output closed")
		return false
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
