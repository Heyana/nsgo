package audio

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/audiosession"
)

// A session holds the level a player with it holds.
func TestSessionLevel(t *testing.T) {
	for session, want := range map[core.AudioSession]audiosession.Level{
		core.AudioSessionDefault:       audiosession.Ambient,
		core.AudioSessionPlayback:      audiosession.Playback,
		core.AudioSessionMixWithOthers: audiosession.Mixing,
	} {
		if got := sessionLevel(session); got != want {
			t.Errorf("session %v holds %v, want %v", session, got, want)
		}
	}
}

// A hold succeeds, on iOS setting the session, and its release may be called twice.
func TestHoldSession(t *testing.T) {
	release, err := HoldSession(core.AudioSessionPlayback)
	if err != nil {
		t.Fatal(err)
	}
	release()
	release()
}
