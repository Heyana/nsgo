package android

import (
	"cmp"
	"slices"
	"sync"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// soundVoices follows the sound playbacks the runtime's one SoundPool plays for the activity and its overlays, so that Go holds them to the engine's playback limit without a change to the wire: a batch stops the oldest playback before a play that would pass the limit. SoundPool plays at most driver.MaxSoundVoices streams, as AOSP's StreamManager clamps maxStreams to 32 (kMaxStreams, under the per-UID AudioTrack limit), so a raised limit holds at 32 and Audio.java retires its own oldest stream there. A playback the runtime ended but has not reported yet counts until it does; its stop finds nothing, so the runtime plays fewer than the limit, never more.
type soundVoices struct {
	mu     sync.Mutex
	voices []soundVoice // oldest first
}

type soundVoice struct {
	surface, sound, playback uint32
	// restricted marks a playback of a scope held to core.Access, which counts against driver.MaxSoundVoices among the restricted playbacks whatever the engine's limit.
	restricted bool
}

// limit returns surface's batch b with an AudioPlaybackStop of the oldest playback before each AudioPlay that would pass its MaxPlaybacks (driver.MaxSoundVoices when 0 or above it), or, for a restricted one, driver.MaxSoundVoices restricted playbacks. It returns b itself when it adds no stop; a test's observer may hold b, so a changed batch is a copy.
func (s *soundVoices) limit(surface uint32, b *driver.MountBatch) *driver.MountBatch {
	if !slices.ContainsFunc(b.Items, func(it driver.MountItem) bool { return it.Kind == driver.ItemAudio }) {
		return b
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []driver.MountItem // nil until a stop goes in
	for i, it := range b.Items {
		if r := it.Audio; it.Kind == driver.ItemAudio && r != nil {
			switch r.Command {
			case driver.AudioPlay:
				if r.Playback == 0 {
					break
				}
				var stops []driver.MountItem
				for j := s.toStop(r); j >= 0; j = s.toStop(r) {
					v := s.voices[j]
					s.voices = slices.Delete(s.voices, j, j+1)
					stops = append(stops, driver.MountItem{Kind: driver.ItemAudio, Count: v.sound, Audio: &driver.AudioRequest{Command: driver.AudioPlaybackStop, Playback: v.playback}})
				}
				if len(stops) > 0 && out == nil {
					out = slices.Clone(b.Items[:i])
				}
				out = append(out, stops...)
				s.voices = append(s.voices, soundVoice{surface: surface, sound: it.Count, playback: r.Playback, restricted: r.Restricted})
			case driver.AudioPlaybackStop:
				s.forget(it.Count, r.Playback)
			case driver.AudioStop, driver.AudioUnload:
				s.voices = slices.DeleteFunc(s.voices, func(v soundVoice) bool { return v.sound == it.Count })
			}
		}
		if out != nil {
			out = append(out, it)
		}
	}
	if out == nil {
		return b
	}
	c := *b
	c.Items = out
	return &c
}

// toStop is the index of the playback r stops before it plays, -1 for none. The caller holds s.mu.
func (s *soundVoices) toStop(r *driver.AudioRequest) int {
	if r.Restricted {
		oldest, n := -1, 0
		for i, v := range s.voices {
			if v.restricted {
				if n == 0 {
					oldest = i
				}
				n++
			}
		}
		if n >= driver.MaxSoundVoices {
			return oldest
		}
	}
	if len(s.voices) >= max(1, min(cmp.Or(r.MaxPlaybacks, driver.MaxSoundVoices), driver.MaxSoundVoices)) {
		return 0
	}
	return -1
}

// ended forgets a playback the runtime reported ended; sound ids are unique across the engine's surfaces.
func (s *soundVoices) ended(sound, playback uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.forget(sound, playback)
}

// forget drops a playback. The caller holds s.mu.
func (s *soundVoices) forget(sound, playback uint32) {
	if i := slices.IndexFunc(s.voices, func(v soundVoice) bool { return v.sound == sound && v.playback == playback }); i >= 0 {
		s.voices = slices.Delete(s.voices, i, i+1)
	}
}

// closeSurface forgets the playbacks of a surface that closed, whose sounds the runtime unloads.
func (s *soundVoices) closeSurface(surface uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.voices = slices.DeleteFunc(s.voices, func(v soundVoice) bool { return v.surface == surface })
}
