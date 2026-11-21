package audio_test

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/audio"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

var playsAudio = nsgotest.Environment(core.Environment{Audio: true})

// A tap's sound is loaded ahead and played in the batch of the tap's turn.
func TestASoundPlaysOnATap(t *testing.T) {
	var click *audio.Sound
	var loadErr error
	var app *nsgotest.App
	app = nsgotest.Start(t, func(cx *core.Scope) core.Node {
		audio.LoadSound(cx, audio.Source{URI: "asset:sounds/click.wav"}, func(s *audio.Sound, err error) { click, loadErr = s, err })
		audio.LoadSound(cx, audio.Source{URI: "asset:sounds/long.wav"}, func(_ *audio.Sound, err error) {
			if !errors.Is(err, audio.QuotaExceeded) {
				t.Errorf("a sound of a minute: %v", err)
			}
		})
		audio.LoadSound(cx, audio.Source{URI: "asset:sounds/missing.flac"}, func(_ *audio.Sound, err error) {
			if !errors.Is(err, audio.Decode) {
				t.Errorf("a sound the platform cannot read: %v", err)
			}
		})
		return core.View{TestID: "pad", Class: "size-12", OnPress: func(core.PressEvent) {
			click.Play(audio.Voice{})
			click.Play(audio.Voice{Volume: 0.5, Rate: 1.2, Loop: true})
		}}
	}, playsAudio, func(a *nsgotest.App) {
		a.AudioDurations = map[string]time.Duration{"asset:sounds/click.wav": 80 * time.Millisecond, "asset:sounds/long.wav": time.Minute}
	})
	if loadErr != nil || click == nil || click.Duration() != 80*time.Millisecond {
		t.Fatalf("click %v, %v", click, loadErr)
	}
	app.Press("id:pad")
	if len(app.Sounds) != 2 {
		t.Fatalf("%d sounds decoded", len(app.Sounds))
	}
	s := app.Sounds[0]
	if s.Source.URI != "asset:sounds/click.wav" || !slices.Equal(s.Plays, []audio.Voice{{Volume: 1, Rate: 1}, {Volume: 0.5, Rate: 1.2, Loop: true}}) {
		t.Fatalf("plays %+v", s)
	}
	if !app.Sounds[1].Unloaded {
		t.Fatal("the cut sound stays loaded")
	}
}

func TestSoundSchedulesOnTheTestClockAndReadsLatency(t *testing.T) {
	var sound *audio.Sound
	var starts []time.Time
	var latency time.Duration
	var known, schedule bool
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		schedule = audio.CanSchedule(cx)
		audio.OutputLatency(cx, func(d time.Duration, ok bool) { latency, known = d, ok })
		audio.LoadSound(cx, audio.Source{URI: "asset:beat.wav"}, func(s *audio.Sound, err error) {
			if err != nil {
				t.Errorf("load the beat: %v", err)
			}
			sound = s
		})
		return core.View{TestID: "play", Class: "size-12", OnPress: func(core.PressEvent) {
			first := audio.Now(cx).Add(time.Second)
			starts = []time.Time{first, first.Add(500 * time.Millisecond)}
			for _, at := range starts {
				sound.Play(audio.Voice{StartAt: at})
			}
		}}
	}, nsgotest.Environment(core.Environment{Audio: true, AudioSchedule: true}), func(a *nsgotest.App) {
		a.AudioDurations = map[string]time.Duration{"asset:beat.wav": 20 * time.Millisecond}
		a.AudioOutputLatency = 12 * time.Millisecond
	})
	if !schedule || !known || latency != 12*time.Millisecond || sound == nil {
		t.Fatalf("schedule %v, latency %s, known %v, sound %v", schedule, latency, known, sound)
	}
	app.Press("id:play")
	plays := app.Sounds[0].Plays
	if len(plays) != 2 || !plays[0].StartAt.Equal(starts[0]) || !plays[1].StartAt.Equal(starts[1]) {
		t.Fatalf("scheduled plays = %+v, want %v", plays, starts)
	}
}

func TestOutputLatencyIsUnknownWithoutAudio(t *testing.T) {
	var answered, known, schedule bool
	nsgotest.Start(t, func(cx *core.Scope) core.Node {
		schedule = audio.CanSchedule(cx)
		audio.OutputLatency(cx, func(_ time.Duration, ok bool) { answered, known = true, ok })
		return core.View{}
	})
	if !answered || known || schedule {
		t.Fatalf("answered %v, latency known %v, can schedule %v", answered, known, schedule)
	}
}

func TestAPlayerPlaysOnTheClock(t *testing.T) {
	var player *audio.Player
	var log []string
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		player = audio.NewPlayer(cx, audio.Source{URI: "https://example.com/voice.m4a"}, audio.PlayerOptions{
			OnLoad:     func(d time.Duration) { log = append(log, fmt.Sprint("load ", d)) },
			OnProgress: func(p, _ time.Duration) { log = append(log, fmt.Sprint(p.Round(time.Millisecond))) },
			OnEnd:      func() { log = append(log, "end") },
		})
		return core.View{}
	}, playsAudio, func(a *nsgotest.App) {
		a.AudioDurations = map[string]time.Duration{"https://example.com/voice.m4a": time.Second}
	})
	if len(app.Players) != 0 {
		t.Fatal("a new player opened on the platform")
	}
	var played []error
	player.Play(func(err error) { played = append(played, err) })
	app.Settle()
	if len(played) != 1 || played[0] != nil || player.State() != audio.PlayerPlaying {
		t.Fatalf("played %v, state %v", played, player.State())
	}
	app.Advance(500 * time.Millisecond)
	if player.Position() != 500*time.Millisecond {
		t.Fatalf("position %v", player.Position())
	}
	player.Pause()
	app.Settle()
	player.Seek(800 * time.Millisecond)
	app.Settle()
	app.Advance(time.Second)
	if player.Position() != 800*time.Millisecond || player.State() != audio.PlayerPaused {
		t.Fatalf("paused at %v, %v", player.Position(), player.State())
	}
	player.Play(nil)
	app.Advance(time.Second)
	want := []string{"load 1s", "250ms", "500ms", "500ms", "800ms", "1s", "end"}
	if !slices.Equal(log, want) {
		t.Fatalf("log %q, want %q", log, want)
	}
	p := app.Players[0]
	if want := []string{"open", "resume", "pause", "seek", "resume"}; !slices.Equal(p.Commands, want) || player.State() != audio.PlayerEnded {
		t.Fatalf("commands %q, state %v", p.Commands, player.State())
	}

	// Looping, it plays on past its end without ending, its position starting over.
	log = nil
	player.SetLoop(true)
	player.Play(nil)
	app.Advance(1100 * time.Millisecond)
	if want := []string{"250ms", "500ms", "750ms", "0s"}; player.State() != audio.PlayerPlaying || !slices.Equal(log, want) {
		t.Fatalf("looping: %v, %q, want %q", player.State(), log, want)
	}
	player.Close()
	app.Settle()
	if !p.Closed || p.Playing {
		t.Fatalf("closed player %+v", p)
	}
}

func TestAPlayerLoopsFromAfterItsIntro(t *testing.T) {
	const start = 400 * time.Millisecond
	var player *audio.Player
	ends := 0
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		player = audio.NewPlayer(cx, audio.Source{URI: "asset:music.wav"}, audio.PlayerOptions{
			LoopStart: start,
			OnEnd:     func() { ends++ },
		})
		player.SetLoop(true)
		player.Play(nil)
		return core.View{}
	}, playsAudio, func(a *nsgotest.App) {
		a.AudioDurations = map[string]time.Duration{"asset:music.wav": time.Second}
	})
	if p := app.Players[0]; !p.Loop || p.LoopStart != start || p.Position != 0 {
		t.Fatalf("initial player = %+v", p)
	}
	app.Advance(300 * time.Millisecond)
	if got := player.Position(); got <= 0 || got >= start {
		t.Fatalf("the first pass skipped its intro, progress %s", got)
	}
	app.Advance(2 * time.Second)
	const repeatedPosition = 500 * time.Millisecond
	if got := app.Players[0].Position; got < repeatedPosition-time.Millisecond || got > repeatedPosition+time.Millisecond || ends != 0 {
		t.Fatalf("after two loops, position %s and %d ends", got, ends)
	}
	player.SetLoop(false)
	app.Advance(510 * time.Millisecond)
	if player.State() != audio.PlayerEnded || ends != 1 {
		t.Fatalf("after disabling loop, state %v and %d ends", player.State(), ends)
	}
}

func TestAPlayerTheBrowserRefuses(t *testing.T) {
	var player *audio.Player
	var errs []error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		player = audio.NewPlayer(cx, audio.Source{URI: "asset:a.mp3"}, audio.PlayerOptions{OnError: func(err error) { errs = append(errs, err) }})
		return core.View{}
	}, playsAudio, func(a *nsgotest.App) {
		a.AudioDurations = map[string]time.Duration{"asset:a.mp3": time.Minute}
		a.RefusePlayback = true
	})
	player.Play(func(err error) { errs = append(errs, err) })
	app.Settle()
	if len(errs) != 2 || !errors.Is(errs[0], audio.NotAllowed) || !errors.Is(errs[1], audio.NotAllowed) || player.State() != audio.PlayerFailed {
		t.Fatalf("errors %v, state %v", errs, player.State())
	}
	app.RefusePlayback = false
	player.Play(nil)
	app.Settle()
	if player.State() != audio.PlayerPlaying {
		t.Fatalf("after a gesture: %v", player.State())
	}
}

func TestAudioUnavailable(t *testing.T) {
	var err error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		audio.LoadSound(cx, audio.Source{URI: "asset:a.wav"}, func(_ *audio.Sound, e error) { err = e })
		return core.View{}
	})
	if !errors.Is(err, audio.Unavailable) || len(app.Sounds) != 0 {
		t.Fatalf("err %v, %d sounds", err, len(app.Sounds))
	}
}

func TestASoundPlaysWithStereoPan(t *testing.T) {
	var click *audio.Sound
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		audio.LoadSound(cx, audio.Source{URI: "asset:sounds/step.wav"}, func(s *audio.Sound, err error) {
			if err != nil {
				t.Errorf("load: %v", err)
				return
			}
			click = s
		})
		return core.View{TestID: "trigger", Class: "size-12", OnPress: func(core.PressEvent) {
			click.Play(audio.Voice{})
			click.Play(audio.Voice{Pan: -1})
			click.Play(audio.Voice{Pan: 1})
			click.Play(audio.Voice{Volume: 0.6, Rate: 1.1, Pan: -0.4})
			click.Play(audio.Voice{Pan: -2})
			click.Play(audio.Voice{Pan: 2})
			click.Play(audio.Voice{Pan: float32(math.NaN())})
		}}
	}, playsAudio, func(a *nsgotest.App) {
		a.AudioDurations = map[string]time.Duration{"asset:sounds/step.wav": 60 * time.Millisecond}
	})
	if click == nil {
		t.Fatal("sound did not load")
	}
	app.Press("id:trigger")
	want := []audio.Voice{
		{Volume: 1, Rate: 1, Pan: 0},
		{Volume: 1, Rate: 1, Pan: -1},
		{Volume: 1, Rate: 1, Pan: 1},
		{Volume: 0.6, Rate: 1.1, Pan: -0.4},
		{Volume: 1, Rate: 1, Pan: -1},
		{Volume: 1, Rate: 1, Pan: 1},
		{Volume: 1, Rate: 1, Pan: 0},
	}
	if !slices.Equal(app.Sounds[0].Plays, want) {
		t.Fatalf("plays = %+v, want %+v", app.Sounds[0].Plays, want)
	}
}
