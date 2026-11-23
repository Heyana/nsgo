//go:build android

package android

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

// writeAudio carries the engine's audio commands in the batch as Android
// applies them. Byte sources follow as paths so a large player source never
// fills the pipe or holds up the batch's mount.
func writeAudio(w *writer, id uint32, r *driver.AudioRequest) {
	w.u32(id)
	w.byte(byte(r.Command))
	w.u32(r.Playback)
	switch r.Command {
	case driver.AudioLoad:
		writeAudioSource(w, r.Source)
		w.u64(uint64(r.MaxBytes))
		w.u64(uint64(r.MaxDuration.Microseconds()))
	case driver.AudioPlay:
		// SoundPool takes a volume per channel, so the pan splits the voice's volume between them.
		left, right := driver.AudioPanGains(r.Pan)
		// Keep the gains separate so changing a muted voice's volume preserves its pan.
		w.float(r.Volume)
		w.float(left)
		w.float(right)
		w.float(r.Rate)
		w.flags(r.Loop)
	case driver.AudioPlaybackSet:
		w.float(r.Volume)
		w.float(r.Rate)
	case driver.AudioPlaybackStop:
	case driver.AudioOpen:
		writeAudioSource(w, r.Source)
		w.float(r.Volume)
		w.float(r.Rate)
		w.flags(r.Loop)
		w.byte(byte(r.Session))
		w.u64(uint64(r.ProgressInterval.Milliseconds()))
		w.u64(uint64(r.Position.Milliseconds()))
		w.u64(uint64(r.LoopStart.Milliseconds()))
	case driver.AudioSeek:
		w.u64(uint64(r.Position.Milliseconds()))
	case driver.AudioSet:
		w.float(r.Volume)
		w.float(r.Rate)
		w.flags(r.Loop)
	}
}

func audioNeedsFile(src driver.ImageSource) bool {
	return len(src.Data) > 0 || strings.HasPrefix(src.URI, "data:")
}

func writeAudioSource(w *writer, src driver.ImageSource) {
	staged := audioNeedsFile(src)
	w.flags(staged)
	if !staged {
		w.str(src.URI)
	}
}

type audioKey struct{ surface, id uint32 }

type audioFile struct {
	sound bool
	path  string
}

// audioFiles owns the files until a sound decodes or its tree or player
// closes. The worker can finish a write after the owner went away.
type audioFiles struct {
	mu     sync.Mutex
	open   map[audioKey]*audioFile
	closed bool
}

func (s *surface) stageAudio(from uint32, b *driver.MountBatch) {
	for _, it := range b.Items {
		if it.Kind != driver.ItemAudio {
			continue
		}
		key := audioKey{from, it.Count}
		r := it.Audio
		switch r.Command {
		case driver.AudioLoad, driver.AudioOpen:
			if !audioNeedsFile(r.Source) {
				continue
			}
			file := &audioFile{sound: r.Command == driver.AudioLoad}
			files := &s.audioFiles
			files.mu.Lock()
			if !files.closed {
				if files.open == nil {
					files.open = map[audioKey]*audioFile{}
				}
				files.open[key] = file
				go s.stageAudioFile(key, file, r.Source, r.MaxBytes)
			}
			files.mu.Unlock()
		case driver.AudioUnload, driver.AudioClose:
			s.audioFiles.drop(key, false)
		}
	}
}

func (s *surface) stageAudioFile(key audioKey, file *audioFile, src driver.ImageSource, limit int64) {
	path, code, message := audioTempFile(src, limit)
	files := &s.audioFiles
	files.mu.Lock()
	pending := files.open[key] == file
	if pending {
		file.path = path
	}
	files.mu.Unlock()
	if !pending {
		if path != "" {
			os.Remove(path)
		}
		return
	}
	err := s.sendExtended(extAudioFile, func(w *writer) {
		w.u32(key.surface)
		w.u32(key.id)
		w.str(path)
		w.str(code)
		w.str(message)
	})
	if errors.Is(err, errNoExtendedMessages) {
		// A runtime that reads no extended messages cannot take the file, so the sound or player fails as audio unavailable; the pipe itself is fine.
		files.drop(key, false)
		s.dispatchTo(key.surface, driver.Event{Kind: driver.EventAudio, Count: key.id, Data: &driver.AudioEvent{
			Phase: driver.AudioFailure, Error: driver.AudioUnavailable, Message: "the Android runtime does not read staged audio files"}})
	}
}

func audioTempFile(src driver.ImageSource, limit int64) (path, code, message string) {
	if limit > 0 && len(src.Data) == 0 {
		_, payload, _ := strings.Cut(src.URI, ",")
		if int64(len(payload)) > 3*limit {
			return "", driver.AudioQuotaExceeded, fmt.Sprintf("the sound exceeds its limit of %d encoded bytes", limit)
		}
	}
	data, err := imageload.Load(context.Background(), src, nil)
	if err != nil {
		return "", driver.AudioNotSupported, err.Error()
	}
	if limit > 0 && int64(len(data)) > limit {
		return "", driver.AudioQuotaExceeded, fmt.Sprintf("the sound exceeds its limit of %d encoded bytes", limit)
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

func (f *audioFiles) drop(key audioKey, soundOnly bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if file := f.open[key]; file != nil && (!soundOnly || file.sound) {
		f.remove(key, file)
	}
}

func (f *audioFiles) closeSurface(id uint32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for key, file := range f.open {
		if key.surface == id {
			f.remove(key, file)
		}
	}
}

func (f *audioFiles) close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	for key, file := range f.open {
		f.remove(key, file)
	}
}

func (f *audioFiles) remove(key audioKey, file *audioFile) {
	delete(f.open, key)
	if file.path != "" {
		os.Remove(file.path)
	}
}

func (s *surface) audioReport(r *reader) {
	key := audioKey{r.u32(), r.u32()}
	report := &driver.AudioEvent{
		Playback:  r.u32(),
		Phase:     driver.AudioPhase(r.byte()),
		Duration:  time.Duration(r.u64()) * time.Microsecond,
		Position:  time.Duration(r.u64()) * time.Microsecond,
		Bytes:     int64(r.u64()),
		Seeks:     r.u32(),
		Laps:      r.u32(),
		Buffering: r.byte() != 0,
		Error:     r.str(),
		Message:   r.str(),
	}
	switch report.Phase {
	case driver.AudioLoaded, driver.AudioFailure:
		s.audioFiles.drop(key, true)
	case driver.AudioPlaybackEnded:
		s.soundVoices.ended(key.id, report.Playback)
	}
	s.dispatchTo(key.surface, driver.Event{Kind: driver.EventAudio, Count: key.id, Data: report})
}
