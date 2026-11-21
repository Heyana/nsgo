//go:build linux && !android

package linux

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
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
		if req.Command == driver.AudioLoad || req.Command == driver.AudioOpen {
			w.audioReport(id, driver.AudioEvent{Phase: driver.AudioFailure, Error: driver.AudioUnavailable})
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
		a.play(id, req.Volume, req.Rate, req.Pan)
	case driver.AudioStop:
		if a.mixer != nil {
			a.mixer.sweep(time.Now(), id)
		}
	case driver.AudioUnload:
		a.unload(id)
	case driver.AudioOpen:
		a.players[id] = &linuxPlayer{owner: w, id: id, source: req.Source, interval: req.ProgressInterval,
			volume: req.Volume, loop: req.Loop, position: req.Position, pendingSeek: req.Position > 0}
		warnAudioRate(req.Rate)
	case driver.AudioClose:
		if p := a.players[id]; p != nil {
			delete(a.players, id)
			p.close()
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
		var cut bool
		if code == "" {
			pcm, cut, message = decodeSound(ctx, b, req.MaxDuration)
			if message != "" {
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
		return nil, driver.AudioQuotaExceeded, "the sound has more than 8 MiB"
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

func (a *linuxAudio) play(id uint32, volume, rate, pan float32) {
	sound := a.sounds[id]
	if sound == nil || len(sound.pcm) == 0 {
		return
	}
	a.checkOutput()
	if a.mixer == nil {
		a.mixer = newSoundMixer()
	}
	if a.mixer != nil {
		a.mixer.play(id, sound.pcm, volume, rate, pan)
		a.lastPlay = time.Now()
		a.ensureTick()
	}
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

var audioTickCallback = sync.OnceValue(func() uintptr { return purego.NewCallback(audioTick) })

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
		if len(a.mixer.voices) == 0 && now.Sub(a.lastPlay) > 2*time.Second {
			a.mixer.close()
			a.mixer = nil
		}
	}
	active := a.mixer != nil
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
			p.close()
		}
	}
	if w == root {
		if a.timer != 0 {
			gSourceRemove(a.timer)
		}
		if a.mixer != nil {
			a.mixer.close()
		}
		root.audio = nil
	}
}
