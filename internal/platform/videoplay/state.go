// Package videoplay tracks app and platform ownership of a video’s playback.
package videoplay

import (
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Background is the backend's own reason for a pause because the app went to the background, which Changed reports as VideoPauseInterruption. No audio-session interruption ends it, so it never lets the video resume, and it takes back the resume of an interruption that paused the video before.
const Background = -1

// State follows who decides whether a video's player plays. The app's Paused applies when it changes and when a source opens; between changes the platform's controls and the system may pause or play the player, and the player then keeps their state, not Paused, until Paused changes. Each such change is reported once, as EventVideoPause with its reason or EventVideoPlay.
type State struct {
	// Playing is what the player did last, as far as the backend knows.
	Playing bool
	// Completed holds a non-repeating end until an explicit replay or seek.
	Completed bool
	// end is where that end left the player.
	end time.Duration
	// held says the player's play state is the platform's, not the app's Paused.
	held bool
	// reason is why the system is about to pause the player, given before the player says it stopped; ok when there is one.
	reason    int
	ok        bool
	resumable bool
}

// Configure reports whether to apply the app’s Paused: when it changed, or no
// native control or completed frame holds the play state.
func (p *State) Configure(pausedChanged bool) bool {
	if pausedChanged {
		p.held, p.ok, p.resumable = false, false, false
	}
	return !p.held && (!p.Completed || pausedChanged)
}

// Applied records the play state the backend is about to give the player, which is not reported.
func (p *State) Applied(playing bool) {
	p.Playing = playing
	if playing {
		p.Completed = false
	}
}

// Ended records that the player reached its end, which EventEnd reports. A video that repeats plays on from its start, and whoever held its play state keeps it, so a play from the controls while Paused is true goes on; one that does not stops, leaves the play state to Paused again, and stays at its end through unrelated configuration until Paused changes, a seek or the controls play it; at is where it stopped.
func (p *State) Ended(repeat bool, at time.Duration) {
	if repeat {
		p.Playing = true
		return
	}
	p.Playing, p.held, p.ok, p.resumable = false, false, false, false
	p.Completed, p.end = true, at
}

// Rewinds says whether the app's play of a completed video starts over from the beginning, with at the player's position: it does while the player is still where it ended, and resumes where the platform's controls scrubbed it back to otherwise. The end the player reached is the mark, not the item's duration, which a player's end may fall short of.
func (p *State) Rewinds(at time.Duration) bool {
	return p.Completed && at >= p.end
}

// Expect records why the system is about to pause the player, before the player says it stopped.
func (p *State) Expect(reason int) {
	if p.Playing {
		p.reason, p.ok = reason, true
	}
}

// Changed takes a play state the player reached on its own, from its controls or the system, with paused the app's Paused, and returns the event that reports it. reason is the pause's, unless the system gave one before. A pause for an audio-session interruption may resume when the interruption ends.
func (p *State) Changed(playing, paused bool, reason int) (driver.Event, bool) {
	background := !playing && reason == Background
	if background {
		// The background's rate notification may come after an interruption already paused the player: it reports nothing, but the video stays paused.
		p.resumable = false
	}
	if playing == p.Playing {
		return driver.Event{}, false
	}
	p.Playing = playing
	p.held = playing == paused
	if playing {
		p.Completed = false
		p.ok, p.resumable = false, false
		return driver.Event{Kind: driver.EventVideoPlay}, true
	}
	if p.ok {
		reason, p.ok = p.reason, false
	}
	// A reason the system gave before stays the one reported, but the background still keeps the pause from resuming.
	p.resumable = p.held && reason == driver.VideoPauseInterruption && !background
	if reason == Background {
		reason = driver.VideoPauseInterruption
	}
	return driver.Event{Kind: driver.EventVideoPause, Index: reason}, true
}

// Resume says whether the player plays again as an interruption that paused it ends: when the system lets it, and the app has not changed Paused since.
func (p *State) Resume() bool {
	ok := p.resumable && p.held && !p.Playing
	p.resumable = false
	return ok
}
