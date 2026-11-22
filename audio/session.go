package audio

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/audiosession"
)

// HoldSession holds iOS's one audio session for audio NSGo does not play, such as a library's own AudioQueue, as a playing Player with session would, until release, which does nothing called again: core.AudioSessionDefault plays ambient, beside other apps' audio and silenced by the Ring/Silent switch; core.AudioSessionMixWithOthers plays in silent mode beside other apps' audio; and core.AudioSessionPlayback plays in silent mode, pausing it. Videos, players, sounds, speech and recordings hold the session too, the highest level held setting it, and once the last holder lets go it goes back to the category from before and is deactivated. Any goroutine may call it. When the session cannot be set and activated, it holds nothing and returns Failed. Elsewhere it holds nothing.
func HoldSession(session core.AudioSession) (release func(), err error) {
	release, err = audiosession.Hold(sessionLevel(session))
	if err != nil {
		return release, Failed
	}
	return release, nil
}

// sessionLevel is the level a player with session holds.
func sessionLevel(session core.AudioSession) audiosession.Level {
	switch session {
	case core.AudioSessionPlayback:
		return audiosession.Playback
	case core.AudioSessionMixWithOthers:
		return audiosession.Mixing
	}
	return audiosession.Ambient
}
