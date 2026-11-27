// Package audiosession holds the one AVAudioSession on iOS, tvOS and watchOS for every part of NSGo that sets it: videos, the audio package's players and sounds, speech and microphone. Each holder asks for a level, and the highest held sets the session; once none is held it goes back to the category from before the first and is deactivated, telling other apps they may play. It imports nothing of the module, so microphone and the backends both import it, and guards its holders with a mutex, so the main thread, audio workers and microphone's caller may all call it. Outside GOOS=ios it holds nothing.
//
// Deactivating waits for the system, the more so while a player's output still runs down, which an AVPlayer's does for a while after its pause: called on the main thread at once it blocked an iPhone's for about 600 ms at every pause. So the last release only arranges the deactivation, which runs deactivateAfter later on a goroutine of its own, and a hold that comes first keeps the session active instead, at no cost when its level is the one the session has. A hold that comes while the deactivation runs waits for it, then activates the session again.
package audiosession

import (
	"sync"
	"time"
)

// Level is what a holder needs of the session, from Kept to Dictation; a higher one wins.
type Level uint8

const (
	// Kept is a video playing with the default session: the category from before the first holder, the app's own, kept active, so that the session is not deactivated under the playing video, which would stop it.
	Kept Level = iota + 1
	// Ambient is every sound and a player with the default session: the Ambient category, which mixes with other apps' audio and which the Ring/Silent switch and the screen lock silence.
	Ambient
	// Mixing is a video or a player with AudioSessionMixWithOthers: Playback in the SpokenAudio mode, mixing with other apps' audio.
	Mixing
	// Playback is a video or a player with AudioSessionPlayback: Playback in the SpokenAudio mode, pausing other apps' audio.
	Playback
	// Recording is a recording, microphone.Capture: PlayAndRecord with AllowBluetoothHFP and DefaultToSpeaker in the Default mode, which the other levels' sounds play through and the Ring/Silent switch does not silence.
	Recording
	// Dictation is a speech session listening: Recording in the Measurement mode.
	Dictation
)

// deactivateAfter is how long the session stays active after its last holder let go: a player's output has run down by then, so the deactivation does not wait on it, and a hold within it, as a play right after a pause, finds the session as it was.
var deactivateAfter = time.Second

// apply moves the session from level from to level to, 0 being none held; a variable so tests see the moves.
var apply = applySession

var holders struct {
	sync.Mutex
	// held counts the holders of each level.
	held [Dictation + 1]int
	// applied is the level the session is at, 0 for none; it stays at the last level held until the deactivation runs.
	applied Level
	// deactivation is the deactivation arranged, nil for none: a hold stops its timer, and one whose timer already fired finds it is no longer the one arranged.
	deactivation *deactivation
}

type deactivation struct{ timer *time.Timer }

// Hold sets the session to level, or keeps the higher one held, until release, which does nothing called again. When the session cannot be set and activated, Hold holds nothing and returns why.
func Hold(level Level) (release func(), err error) {
	holders.Lock()
	defer holders.Unlock()
	holders.held[level]++
	if err := settle(); err != nil {
		holders.held[level]--
		settle()
		return func() {}, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			holders.Lock()
			defer holders.Unlock()
			holders.held[level]--
			settle()
		})
	}, nil
}

// settle sets the session to the highest level held, or arranges its deactivation when none is.
func settle() error {
	want := wanted()
	if want == 0 {
		if holders.applied != 0 && holders.deactivation == nil {
			d := &deactivation{}
			d.timer = time.AfterFunc(deactivateAfter, func() { deactivate(d) })
			holders.deactivation = d
		}
		return nil
	}
	if holders.deactivation != nil {
		holders.deactivation.timer.Stop()
		holders.deactivation = nil
	}
	if want == holders.applied {
		return nil
	}
	if err := apply(holders.applied, want); err != nil {
		return err
	}
	holders.applied = want
	return nil
}

// wanted is the highest level held, 0 for none.
func wanted() Level {
	for l := Dictation; l > 0; l-- {
		if holders.held[l] > 0 {
			return l
		}
	}
	return 0
}

// deactivate runs deactivation d, unless a hold came since it was arranged.
func deactivate(d *deactivation) {
	holders.Lock()
	defer holders.Unlock()
	if holders.deactivation != d {
		return
	}
	holders.deactivation = nil
	if wanted() != 0 || holders.applied == 0 {
		return
	}
	// Going to none cannot fail: restore logs what it cannot give back.
	apply(holders.applied, 0)
	holders.applied = 0
}
