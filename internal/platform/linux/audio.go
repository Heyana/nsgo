//go:build linux && !android

package linux

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

type linuxSound struct {
	owner  *window
	cancel context.CancelFunc
	pcm    []byte
}

// linuxAudio is one engine's audio service, on the GTK thread.
type linuxAudio struct {
	root     *window
	sounds   map[uint32]*linuxSound
	players  map[uint32]*linuxPlayer
	mixer    *soundMixer
	lastPlay time.Time
	timer    uint32
	// latencyReads wait for the mixer to reach PLAYING, which configures its latency.
	latencyReads []latencyRead
	// retired holds closed players' streams that were seeking: GtkMediaFile frees its GStreamer pipeline at once, while a streaming thread may still hold the pipeline's locks for the seek's flush, which aborts the process, so a seeking stream goes once its seek ends.
	retired []retiredStream
}

type retiredStream struct {
	stream uintptr
	until  time.Time
}

// retiredStreamWait bounds how long a closed player's seek may hold its stream.
const retiredStreamWait = 2 * time.Second

// closePlayer closes p, keeping its stream until a seek under way ends.
func (a *linuxAudio) closePlayer(p *linuxPlayer) {
	if p.stream != 0 && gtkMediaStreamIsSeeking(p.stream) != 0 {
		gtkMediaStreamSetPlaying(p.stream, 0)
		a.retired = append(a.retired, retiredStream{p.stream, time.Now().Add(retiredStreamWait)})
		p.stream = 0
		a.ensureTick()
	}
	p.close()
}

// releaseRetired frees the retired streams whose seek ended or whose wait ran out, reporting whether any remain.
func (a *linuxAudio) releaseRetired(now time.Time) bool {
	a.retired = slices.DeleteFunc(a.retired, func(r retiredStream) bool {
		if gtkMediaStreamIsSeeking(r.stream) != 0 && now.Before(r.until) {
			return false
		}
		freeMediaStream(r.stream)
		return true
	})
	return len(a.retired) > 0
}

type latencyRead struct {
	owner *window
	id    uint32
	until time.Time
}

func (w *window) audioRoot() *window {
	for w.opener != nil {
		w = w.opener
	}
	return w
}

func (w *window) audioCommand(id uint32, req *driver.AudioRequest) {
	if w.closing || req == nil {
		return
	}
	if !loadSound() {
		switch req.Command {
		case driver.AudioLoad, driver.AudioOpen:
			w.audioReport(id, driver.AudioEvent{Phase: driver.AudioFailure, Error: driver.AudioUnavailable})
		case driver.AudioReadLatency:
			w.audioReport(id, driver.AudioEvent{Phase: driver.AudioLatency})
		case driver.AudioPlay:
			w.audioReport(id, driver.AudioEvent{Phase: driver.AudioPlaybackEnded, Playback: req.Playback})
		}
		return
	}
	root := w.audioRoot()
	if root.closing {
		return
	}
	if root.audio == nil {
		root.audio = &linuxAudio{root: root, sounds: map[uint32]*linuxSound{}, players: map[uint32]*linuxPlayer{}}
	}
	a := root.audio
	switch req.Command {
	case driver.AudioLoad:
		a.load(w, id, *req)
	case driver.AudioPlay:
		a.play(id, req)
	case driver.AudioPlaybackSet, driver.AudioPlaybackStop:
		if a.mixer != nil {
			a.mixer.control(id, req)
		}
	case driver.AudioReadLatency:
		a.readLatency(w, id)
	case driver.AudioStop:
		if a.mixer != nil {
			a.mixer.sweep(time.Now(), id)
		}
	case driver.AudioUnload:
		a.unload(id)
	case driver.AudioOpen:
		a.players[id] = &linuxPlayer{owner: w, audio: a, id: id, source: req.Source, interval: req.ProgressInterval,
			volume: req.Volume, loop: req.Loop, loopStart: req.LoopStart, position: req.Position, pendingSeek: req.Position > 0}
		warnAudioRate(req.Rate)
	case driver.AudioClose:
		if p := a.players[id]; p != nil {
			delete(a.players, id)
			a.closePlayer(p)
		}
	default:
		if p := a.players[id]; p != nil {
			p.command(req)
			a.ensureTick()
		}
	}
}

func (w *window) audioReport(id uint32, r driver.AudioEvent) {
	w.engine.Dispatch(driver.Event{Kind: driver.EventAudio, Count: id, Data: &r})
}

func (a *linuxAudio) load(w *window, id uint32, req driver.AudioRequest) {
	ctx, cancel := context.WithCancel(context.Background())
	sound := &linuxSound{owner: w, cancel: cancel}
	a.sounds[id] = sound
	go func() {
		defer cancel()
		b, code, message := readSoundBytes(ctx, req.Source, req.MaxBytes)
		var pcm []byte
		var cut, large bool
		if code == "" {
			pcm, cut, large, message = decodeSound(ctx, b, req.MaxDuration, cmp.Or(req.MaxDecodedBytes, driver.MaxDecodedBytes))
			switch {
			case large:
				code, message = driver.AudioQuotaExceeded, "the sound exceeds its decoded size limit"
			case message != "":
				code = driver.AudioDecode
			}
		}
		if ctx.Err() != nil {
			return
		}
		w.post(func() {
			if a.sounds[id] != sound {
				return
			}
			sound.cancel = nil
			if code != "" {
				delete(a.sounds, id)
				w.audioReport(id, driver.AudioEvent{Phase: driver.AudioFailure, Error: code, Message: message})
				return
			}
			if !cut {
				sound.pcm = pcm
			}
			w.audioReport(id, driver.AudioEvent{Phase: driver.AudioLoaded, Duration: soundDuration(pcm),
				Bytes: int64(len(pcm)), Cut: cut})
		})
	}()
}

func readSoundBytes(owner context.Context, src driver.ImageSource, maxBytes int64) ([]byte, string, string) {
	ctx, cancel := context.WithCancel(owner)
	defer cancel()
	var large atomic.Bool
	progress := func(loaded, total int64) {
		if loaded > maxBytes || total > maxBytes {
			large.Store(true)
			cancel()
		}
	}
	data, err := soundSourceBytes(ctx, src, maxBytes, progress)
	if large.Load() || int64(len(data)) > maxBytes {
		return nil, driver.AudioQuotaExceeded, fmt.Sprintf("the sound exceeds its limit of %d encoded bytes", maxBytes)
	}
	if err != nil {
		code := driver.AudioNotSupported
		if remoteAudio(src.URI) {
			code = driver.AudioNetwork
		}
		return nil, code, err.Error()
	}
	return data, "", ""
}

func soundSourceBytes(ctx context.Context, src driver.ImageSource, limit int64, progress imageload.Progress) ([]byte, error) {
	if len(src.Data) != 0 || remoteAudio(src.URI) || strings.HasPrefix(src.URI, "data:") {
		return imageload.Load(ctx, src, progress)
	}
	if strings.ContainsRune(src.URI, 0) {
		return nil, errors.New("the audio source contains a NUL")
	}
	file := videoFile(src.URI)
	path := filePath(file)
	gObjectUnref(file)
	if path == "" {
		return nil, errors.New("the audio source has no local file")
	}
	in, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	return io.ReadAll(io.LimitReader(in, limit+1))
}

func remoteAudio(uri string) bool {
	scheme, _, ok := strings.Cut(uri, "://")
	return ok && (strings.EqualFold(scheme, "http") || strings.EqualFold(scheme, "https"))
}

func (a *linuxAudio) play(id uint32, req *driver.AudioRequest) {
	sound := a.sounds[id]
	if sound == nil || len(sound.pcm) == 0 {
		a.playbackEnded(id, req.Playback)
		return
	}
	a.checkOutput()
	if a.mixer == nil {
		a.mixer = newSoundMixer(a.playbackEnded)
	}
	if a.mixer != nil {
		a.mixer.play(soundPlayback{id: id, playback: req.Playback, pcm: sound.pcm, volume: req.Volume, rate: req.Rate, pan: req.Pan, startAt: req.StartAt, loop: req.Loop,
			limit: req.MaxPlaybacks, restricted: req.Restricted})
		a.lastPlay = time.Now()
		a.ensureTick()
	} else {
		a.playbackEnded(id, req.Playback)
	}
}

func (a *linuxAudio) playbackEnded(sound, playback uint32) {
	if playback != 0 {
		a.root.audioReport(sound, driver.AudioEvent{Phase: driver.AudioPlaybackEnded, Playback: playback})
	}
}

// readLatency answers with the output pipeline's latency once it plays. A read opens the output as a play does, and the idle sweep closes it again.
func (a *linuxAudio) readLatency(w *window, id uint32) {
	a.checkOutput()
	if a.mixer == nil {
		a.mixer = newSoundMixer(a.playbackEnded)
	}
	if a.mixer == nil {
		w.audioReport(id, driver.AudioEvent{Phase: driver.AudioLatency})
		return
	}
	now := time.Now()
	a.lastPlay = now
	a.latencyReads = append(a.latencyReads, latencyRead{owner: w, id: id, until: now.Add(time.Second)})
	a.answerLatency(now)
	a.ensureTick()
}

// answerLatency answers the reads the mixer can answer: all of them once it plays, the late ones unknown, and all of them unknown without a mixer.
func (a *linuxAudio) answerLatency(now time.Time) {
	if len(a.latencyReads) == 0 {
		return
	}
	var latency time.Duration
	playing := a.mixer != nil && a.mixer.playing()
	if playing {
		latency = a.mixer.latency()
	}
	a.latencyReads = slices.DeleteFunc(a.latencyReads, func(r latencyRead) bool {
		if playing || a.mixer == nil || now.After(r.until) {
			r.owner.audioReport(r.id, driver.AudioEvent{Phase: driver.AudioLatency, Latency: latency})
			return true
		}
		return false
	})
}

func (a *linuxAudio) checkOutput() {
	if a.mixer != nil {
		if message := a.mixer.checkOutput(); message != "" {
			log.Print("nsgo: sound output: ", message)
			a.mixer.close()
			a.mixer = nil
		}
	}
}

func (a *linuxAudio) unload(id uint32) {
	if sound := a.sounds[id]; sound != nil {
		delete(a.sounds, id)
		if sound.cancel != nil {
			sound.cancel()
		}
	}
	if a.mixer != nil {
		a.mixer.sweep(time.Now(), id)
	}
}

// audioTickCallback is set in init because audioTick reaches ensureTick, through a player decoding its track, which a package-level initializer cannot refer back to.
var audioTickCallback func() uintptr

func init() {
	audioTickCallback = sync.OnceValue(func() uintptr { return purego.NewCallback(audioTick) })
}

func (a *linuxAudio) ensureTick() {
	if a.timer == 0 {
		a.timer = gTimeoutAdd(16, audioTickCallback(), a.root.handle)
	}
}

func audioTick(handle uintptr) int32 {
	root := windowOf(handle)
	if root == nil || root.closing || root.audio == nil {
		return 0
	}
	a := root.audio
	now := time.Now()
	a.checkOutput()
	if a.mixer != nil {
		a.mixer.sweep(now, 0)
		a.mixer.startPending()
		a.answerLatency(now)
		a.mixer.feed(now)
		if len(a.mixer.voices) == 0 && len(a.mixer.tracks) == 0 && len(a.latencyReads) == 0 && now.Sub(a.lastPlay) > 2*time.Second {
			a.mixer.close()
			a.mixer = nil
		}
	}
	a.answerLatency(now)
	active := a.releaseRetired(now) || a.mixer != nil
	for _, p := range a.players {
		if p.tick(now) {
			active = true
		}
	}
	if active {
		return 1
	}
	a.timer = 0
	return 0
}

func (w *window) closeAudio() {
	root := w.audioRoot()
	a := root.audio
	if a == nil {
		return
	}
	for id, sound := range a.sounds {
		if w == root || sound.owner == w {
			a.unload(id)
		}
	}
	for id, p := range a.players {
		if w == root || p.owner == w {
			delete(a.players, id)
			a.closePlayer(p)
		}
	}
	a.latencyReads = slices.DeleteFunc(a.latencyReads, func(r latencyRead) bool { return w == root || r.owner == w })
	if w == root {
		// The tick stops with the window, so the seeks end here: GtkMediaFile learns of them through the main context.
		for deadline := time.Now().Add(retiredStreamWait); a.releaseRetired(time.Now()) && time.Now().Before(deadline); {
			gMainContextIteration(0, 0)
			time.Sleep(time.Millisecond)
		}
		for _, r := range a.retired {
			freeMediaStream(r.stream)
		}
		a.retired = nil
		if a.timer != 0 {
			gSourceRemove(a.timer)
		}
		if a.mixer != nil {
			a.mixer.close()
		}
		root.audio = nil
	}
}
