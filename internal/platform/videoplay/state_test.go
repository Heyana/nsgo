package videoplay

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// step is one thing that happens to a video's player, and what the backend reports for it.
type step struct {
	name string
	do   func(p *State) (driver.Event, bool)
	want []driver.Event
	// apply is whether a Configure that follows gives the player the app's play state.
	apply bool
}

func pausedFor(reason int) driver.Event {
	return driver.Event{Kind: driver.EventVideoPause, Index: reason}
}

var played = driver.Event{Kind: driver.EventVideoPlay}

func runSteps(t *testing.T, steps []step) {
	t.Helper()
	var p State
	for _, s := range steps {
		var got []driver.Event
		if ev, ok := s.do(&p); ok {
			got = append(got, ev)
		}
		if len(got) != len(s.want) || len(got) == 1 && (got[0].Kind != s.want[0].Kind || got[0].Index != s.want[0].Index) {
			t.Fatalf("%s: reported %v, want %v", s.name, got, s.want)
		}
		if apply := p.Configure(false); apply != s.apply {
			t.Fatalf("%s: a Configure applies Paused %v, want %v", s.name, apply, s.apply)
		}
	}
}

// configure is a Configure of the app's paused: it gives the player that state when the backend's State lets it.
func configure(paused, changed bool) func(p *State) (driver.Event, bool) {
	return func(p *State) (driver.Event, bool) {
		if p.Configure(changed) {
			p.Applied(!paused)
		}
		return driver.Event{}, false
	}
}

// reached is the player reaching playing on its own while the app's Paused is paused.
func reached(playing, paused bool, reason int) func(p *State) (driver.Event, bool) {
	return func(p *State) (driver.Event, bool) { return p.Changed(playing, paused, reason) }
}

func TestVideoControlsPauseHoldsUntilPausedChanges(t *testing.T) {
	runSteps(t, []step{
		{name: "opened playing", do: configure(false, true), apply: true},
		// The app's own changes come back as rate notifications that change nothing.
		{name: "own rate", do: reached(true, false, driver.VideoPauseControls), apply: true},
		{name: "controls pause", do: reached(false, false, driver.VideoPauseControls), want: []driver.Event{pausedFor(driver.VideoPauseControls)}},
		{name: "the same rate again", do: reached(false, false, driver.VideoPauseControls)},
		// Muted changes: the pause holds.
		{name: "unrelated config", do: configure(false, false)},
		{name: "app follows", do: configure(true, true), apply: true},
		{name: "app plays", do: configure(false, true), apply: true},
	})
}

func TestVideoControlsPlayWhileTheAppIsPausedIsReported(t *testing.T) {
	runSteps(t, []step{
		{name: "opened paused", do: configure(true, true), apply: true},
		{name: "controls play", do: reached(true, true, driver.VideoPauseControls), want: []driver.Event{played}},
		{name: "unrelated config", do: configure(true, false)},
		{name: "controls pause", do: reached(false, true, driver.VideoPauseControls), want: []driver.Event{pausedFor(driver.VideoPauseControls)}, apply: true},
		{name: "controls play again", do: reached(true, true, driver.VideoPauseControls), want: []driver.Event{played}},
		{name: "app follows", do: configure(false, true), apply: true},
	})
}

func TestVideoSystemPauseKeepsTheReasonItGaveFirst(t *testing.T) {
	runSteps(t, []step{
		{name: "opened playing", do: configure(false, true), apply: true},
		{name: "route lost", do: func(p *State) (driver.Event, bool) {
			p.Expect(driver.VideoPauseRouteLost)
			return p.Changed(false, false, driver.VideoPauseControls)
		}, want: []driver.Event{pausedFor(driver.VideoPauseRouteLost)}},
		// A lost route does not play on by itself.
		{name: "no resume", do: func(p *State) (driver.Event, bool) {
			if p.Resume() {
				t.Error("a lost route resumed")
			}
			return driver.Event{}, false
		}},
		{name: "controls play", do: reached(true, false, driver.VideoPauseControls), want: []driver.Event{played}, apply: true},
	})
}

func TestVideoInterruptionResumesUnlessTheAppTookIt(t *testing.T) {
	interrupted := reached(false, false, driver.VideoPauseInterruption)
	resume := func(want bool) func(p *State) (driver.Event, bool) {
		return func(p *State) (driver.Event, bool) {
			if got := p.Resume(); got != want {
				t.Errorf("resume() = %v, want %v", got, want)
			}
			if want {
				return p.Changed(true, false, driver.VideoPauseInterruption)
			}
			return driver.Event{}, false
		}
	}
	runSteps(t, []step{
		{name: "opened playing", do: configure(false, true), apply: true},
		{name: "interrupted", do: interrupted, want: []driver.Event{pausedFor(driver.VideoPauseInterruption)}},
		{name: "unrelated config", do: configure(false, false)},
		{name: "interruption ends", do: resume(true), want: []driver.Event{played}, apply: true},
		{name: "interrupted again", do: interrupted, want: []driver.Event{pausedFor(driver.VideoPauseInterruption)}},
		{name: "app follows", do: configure(true, true), apply: true},
		{name: "interruption ends after the app paused", do: resume(false), apply: true},
	})
}

// A pause because the app went to the background is reported as an interruption, but no audio-session interruption that ends later plays the video: not one that begins while it is paused, and not one that paused it before the background's notification came.
func TestVideoBackgroundPauseDoesNotResume(t *testing.T) {
	interruptionBegins := func(p *State) (driver.Event, bool) {
		p.Expect(driver.VideoPauseInterruption)
		return driver.Event{}, false
	}
	background := reached(false, false, Background)
	interrupted := reached(false, false, driver.VideoPauseInterruption)
	// after is what follows the background pause, failing t.
	after := func(t *testing.T) []step {
		return []step{
			{name: "unrelated config", do: configure(false, false)},
			{name: "interruption begins while paused", do: interruptionBegins},
			{name: "interruption ends", do: func(p *State) (driver.Event, bool) {
				if p.Resume() {
					t.Error("an interruption's end played a video the background paused")
				}
				return driver.Event{}, false
			}},
			{name: "controls play", do: reached(true, false, driver.VideoPauseControls), want: []driver.Event{played}, apply: true},
			{name: "interrupted", do: interrupted, want: []driver.Event{pausedFor(driver.VideoPauseInterruption)}},
			{name: "interruption resumes", do: func(p *State) (driver.Event, bool) {
				if !p.Resume() {
					t.Error("an interruption after the controls played did not resume")
				}
				return p.Changed(true, false, driver.VideoPauseInterruption)
			}, want: []driver.Event{played}, apply: true},
		}
	}
	opened := step{name: "opened playing", do: configure(false, true), apply: true}
	for _, c := range []struct {
		name   string
		before []step
	}{
		{"background", []step{
			{name: "backgrounded", do: background, want: []driver.Event{pausedFor(driver.VideoPauseInterruption)}},
		}},
		{"background while an interruption is expected", []step{
			{name: "interruption begins", do: interruptionBegins, apply: true},
			{name: "backgrounded", do: background, want: []driver.Event{pausedFor(driver.VideoPauseInterruption)}},
		}},
		// The reason the system gave before the player stopped is the one reported.
		{"background while a lost route is expected", []step{
			{name: "route lost begins", do: func(p *State) (driver.Event, bool) {
				p.Expect(driver.VideoPauseRouteLost)
				return driver.Event{}, false
			}, apply: true},
			{name: "backgrounded", do: background, want: []driver.Event{pausedFor(driver.VideoPauseRouteLost)}},
		}},
		{"background after an interruption paused", []step{
			{name: "interrupted", do: interrupted, want: []driver.Event{pausedFor(driver.VideoPauseInterruption)}},
			{name: "backgrounded", do: background},
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			runSteps(t, append(append([]step{opened}, c.before...), after(t)...))
		})
	}
}

func TestVideoEndHoldsUntilExplicitReplay(t *testing.T) {
	runSteps(t, []step{
		{name: "opened playing", do: configure(false, true), apply: true},
		{name: "controls pause", do: reached(false, false, driver.VideoPauseControls), want: []driver.Event{pausedFor(driver.VideoPauseControls)}},
		{name: "controls play", do: reached(true, false, driver.VideoPauseControls), want: []driver.Event{played}, apply: true},
		{name: "ended", do: ended(false)},
		// The rate notification of the end comes after it.
		{name: "end's rate", do: reached(false, false, driver.VideoPauseControls)},
		{name: "unrelated config", do: configure(false, false)},
		{name: "app pauses", do: configure(true, true)},
		{name: "app replays", do: configure(false, true), apply: true},
	})
	runSteps(t, []step{
		{name: "opened paused", do: configure(true, true), apply: true},
		{name: "controls play", do: reached(true, true, driver.VideoPauseControls), want: []driver.Event{played}},
		{name: "ended", do: ended(false)},
		{name: "unrelated config", do: configure(true, false)},
		{name: "controls replay", do: reached(true, true, driver.VideoPauseControls), want: []driver.Event{played}},
	})
}

// A repeating video plays on from its start, and a play from the controls while the app's Paused is true holds through the end.
func TestVideoRepeatKeepsTheControlsPlay(t *testing.T) {
	runSteps(t, []step{
		{name: "opened paused", do: configure(true, true), apply: true},
		{name: "controls play", do: reached(true, true, driver.VideoPauseControls), want: []driver.Event{played}},
		{name: "ended and repeats", do: ended(true)},
		// The play from its start comes back as a rate notification that changes nothing.
		{name: "repeat's rate", do: reached(true, true, driver.VideoPauseControls)},
		{name: "unrelated config", do: configure(true, false)},
		{name: "app follows", do: configure(false, true), apply: true},
	})
}

// ended is the player reaching its end, repeating or not.
func ended(repeat bool) func(p *State) (driver.Event, bool) {
	return func(p *State) (driver.Event, bool) {
		p.Ended(repeat, time.Second)
		return driver.Event{}, false
	}
}

// A completed video starts over only while the player is still where it ended: the controls' scrubber moves it back without telling the backend, and the app's next play resumes there. The end the player reached is the mark, even short of the item's duration.
func TestVideoReplayResumesWhereTheControlsScrubbed(t *testing.T) {
	var p State
	if p.Rewinds(0) {
		t.Fatal("a video that never ended starts over")
	}
	p.Ended(false, 990*time.Millisecond)
	for _, c := range []struct {
		at   time.Duration
		want bool
	}{{990 * time.Millisecond, true}, {time.Second, true}, {400 * time.Millisecond, false}, {0, false}} {
		if got := p.Rewinds(c.at); got != c.want {
			t.Fatalf("completed at 990ms, at %v: rewinds %v, want %v", c.at, got, c.want)
		}
	}
	p.Applied(true)
	if p.Rewinds(time.Second) {
		t.Fatal("a replayed video starts over again")
	}
	p.Ended(true, time.Second)
	if p.Rewinds(time.Second) {
		t.Fatal("a repeating end starts over through the app")
	}
}
