//go:build windows

package windows

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var audioAvailable = sync.OnceValue(func() bool {
	return mfStartup.Find() == nil && mfCreateSourceReader.Find() == nil && mfCreateMediaType.Find() == nil &&
		mfCreateByteStream.Find() == nil && shCreateMemStream.Find() == nil &&
		(xaCreateWithVersion.Find() == nil || xaCreate.Find() == nil)
})

// windowAudio owns one surface's sounds and players, on the window thread.
type windowAudio struct {
	w               *window
	sounds          map[uint32]*winSound
	loading         map[uint32]context.CancelFunc
	players         map[uint32]*winAudioPlayer
	output          *soundOutput
	voiceTimer      *time.Timer
	voiceGeneration uint64
}

func (w *window) audioCommand(id uint32, r *driver.AudioRequest) {
	if r == nil {
		return
	}
	if w.audio == nil {
		w.audio = &windowAudio{w: w, sounds: map[uint32]*winSound{}, loading: map[uint32]context.CancelFunc{}, players: map[uint32]*winAudioPlayer{}}
	}
	a := w.audio
	switch r.Command {
	case driver.AudioLoad:
		a.load(id, r.Source, r.MaxBytes, r.MaxDuration)
	case driver.AudioPlay:
		a.play(id, r)
	case driver.AudioPlaybackSet, driver.AudioPlaybackStop:
		a.controlPlayback(id, r)
	case driver.AudioReadLatency:
		a.report(id, driver.AudioEvent{Phase: driver.AudioLatency})
	case driver.AudioStop:
		a.stop(id)
	case driver.AudioUnload:
		a.unload(id)
	case driver.AudioOpen:
		if p := a.players[id]; p != nil {
			p.close()
		}
		a.players[id] = &winAudioPlayer{a: a, id: id, source: r.Source, volume: r.Volume,
			rate: r.Rate, loop: r.Loop, loopStart: r.LoopStart, position: r.Position, interval: r.ProgressInterval}
	case driver.AudioClose:
		if p := a.players[id]; p != nil {
			delete(a.players, id)
			p.close()
		}
	default:
		if p := a.players[id]; p != nil {
			p.apply(r)
		}
	}
}

func (w *window) closeAudio() {
	a := w.audio
	if a == nil {
		return
	}
	w.audio = nil
	for _, cancel := range a.loading {
		cancel()
	}
	for _, p := range a.players {
		p.close()
	}
	a.closeOutput()
	for _, sound := range a.sounds {
		coTaskMemFree.Call(uintptr(sound.data))
	}
}

func (a *windowAudio) report(id uint32, ev driver.AudioEvent) {
	a.w.engine.Dispatch(driver.Event{Kind: driver.EventAudio, Count: id, Data: &ev})
}

func audioFailure(code, message string) driver.AudioEvent {
	return driver.AudioEvent{Phase: driver.AudioFailure, Error: code, Message: message}
}

func remoteAudio(source string) bool {
	scheme, _, ok := strings.Cut(source, "://")
	return ok && (strings.EqualFold(scheme, "http") || strings.EqualFold(scheme, "https"))
}

func audioExtension(data []byte, uri string) string {
	switch driver.AudioType(data) {
	case "audio/wav":
		return ".wav"
	case "audio/mpeg":
		return ".mp3"
	case "audio/mp4":
		return ".m4a"
	case "audio/aac":
		return ".aac"
	case "audio/flac":
		return ".flac"
	case "audio/ogg":
		return ".ogg"
	}
	name, _, _ := strings.Cut(strings.ToLower(uri), "?")
	name, _, _ = strings.Cut(name, "#")
	for _, ext := range []string{".wav", ".mp3", ".aac", ".m4a", ".mp4", ".flac", ".ogg", ".wma"} {
		if strings.HasSuffix(name, ext) {
			return ext
		}
	}
	return ".bin"
}
