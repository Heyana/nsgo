package android

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestSoundVoicesHoldTheEnginesPlaybackLimit(t *testing.T) {
	var s soundVoices
	playback := uint32(0)
	// play sends one AudioPlay of sound 1 through s and returns the playbacks the batch stops first.
	play := func(surface uint32, limit int, restricted bool) []uint32 {
		playback++
		b := &driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemText}, {Kind: driver.ItemAudio, Count: 1,
			Audio: &driver.AudioRequest{Command: driver.AudioPlay, Playback: playback, MaxPlaybacks: limit, Restricted: restricted}}}}
		out := s.limit(surface, b)
		if len(b.Items) != 2 || b.Items[1].Audio.Command != driver.AudioPlay {
			t.Fatal("limit changed the batch it was given")
		}
		last := out.Items[len(out.Items)-1]
		if out.Items[0].Kind != driver.ItemText || last.Audio.Command != driver.AudioPlay || last.Audio.Playback != playback {
			t.Fatalf("limit reordered the batch: %+v", out.Items)
		}
		var stopped []uint32
		for _, it := range out.Items[1 : len(out.Items)-1] {
			if it.Kind != driver.ItemAudio || it.Count != 1 || it.Audio.Command != driver.AudioPlaybackStop {
				t.Fatalf("limit added %+v", it)
			}
			stopped = append(stopped, it.Audio.Playback)
		}
		if stopped == nil && out != b {
			t.Fatal("a batch without stops was copied")
		}
		return stopped
	}

	for range driver.MaxSoundVoices {
		if stopped := play(0, 0, false); stopped != nil {
			t.Fatalf("a play within the default limit stopped %v", stopped)
		}
	}
	// SoundPool plays at most 32 streams, so a raised limit stops the oldest there.
	if stopped := play(0, driver.MaxSoundVoices*2, false); !slices.Equal(stopped, []uint32{1}) {
		t.Fatalf("the 33rd play under a raised limit stopped %v, want [1]", stopped)
	}
	// A lowered limit stops the oldest until the new playback fits.
	if stopped := play(0, 4, false); len(stopped) != driver.MaxSoundVoices-3 || stopped[0] != 2 {
		t.Fatalf("lowering the limit to 4 stopped %v", stopped)
	}
	// A reported end frees its place.
	s.ended(1, playback)
	if stopped := play(0, 4, false); stopped != nil {
		t.Fatalf("a play after an end stopped %v", stopped)
	}
	// Restricted playbacks stay within the default cap among themselves.
	s.limit(0, &driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemAudio, Count: 1, Audio: &driver.AudioRequest{Command: driver.AudioStop}}}})
	for range driver.MaxSoundVoices {
		play(0, 0, true)
	}
	first := playback - driver.MaxSoundVoices + 1
	if stopped := play(0, 0, true); !slices.Equal(stopped, []uint32{first}) {
		t.Fatalf("the 33rd restricted play stopped %v, want [%d]", stopped, first)
	}
	// A closed overlay's playbacks no longer count.
	s.closeSurface(0)
	play(7, 2, false)
	s.closeSurface(7)
	if stopped := play(0, 1, false); stopped != nil {
		t.Fatalf("a closed surface's playback was stopped: %v", stopped)
	}
}
