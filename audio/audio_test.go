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
	var playbacks []*audio.Playback
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
				playbacks = append(playbacks, sound.Play(audio.Voice{StartAt: at}))
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
	app.FastForward(starts[0].Sub(app.Clock.Now()) - time.Millisecond)
	if playbacks[0].Ended() || playbacks[1].Ended() {
		t.Fatal("a pending playback ended before its scheduled start")
	}
	app.Advance(22 * time.Millisecond)
	if !playbacks[0].Ended() || playbacks[1].Ended() {
		t.Fatal("scheduled playback completion did not follow its start")
	}
	playbacks[1].Stop()
	app.Settle()
	if !playbacks[1].Ended() || !app.Sounds[0].Playbacks[1].Ended {
		t.Fatal("stopping a pending playback left it active")
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

func TestSoundPlaybackControlsAndEnds(t *testing.T) {
	const uri = "asset:effect.wav"
	var sound *audio.Sound
	var changed *core.Signal[*audio.Playback]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		changed = core.NewSignal[*audio.Playback](cx, nil)
		audio.LoadSound(cx, audio.Source{URI: uri}, func(s *audio.Sound, err error) {
			if err != nil {
				t.Error(err)
			}
			sound = s
		})
		return core.Text{TestID: "status", ContentFunc: func() string {
			if changed.Get().Ended() {
				return "Ended"
			}
			return "Playing"
		}}
	}, playsAudio, func(a *nsgotest.App) { a.AudioDurations = map[string]time.Duration{uri: time.Second} })
	first := sound.Play(audio.Voice{})
	second := sound.Play(audio.Voice{Muted: true})
	changed.Set(second)
	app.Settle()
	if first.Ended() || second.Ended() || app.Find("id:status").Text != "Playing" {
		t.Fatal("two independent playbacks should be active")
	}
	p := app.Sounds[0].Playbacks
	if p[1].Volume != 0 {
		t.Fatalf("the muted playback started with volume %v", p[1].Volume)
	}
	first.Stop()
	second.SetVolume(0.25)
	second.SetRate(2)
	app.Settle()
	if !first.Ended() || second.Ended() || !p[0].Ended || p[1].Ended || p[1].Volume != 0.25 || p[1].Rate != 2 {
		t.Fatalf("independent stop and settings gave %+v, %+v", p[0], p[1])
	}
	second.SetVolume(0)
	app.Settle()
	if p[1].Volume != 0 || second.Ended() {
		t.Fatal("zero volume must silence a playback without ending it")
	}
	app.Advance(499 * time.Millisecond)
	if second.Ended() {
		t.Fatal("playback ended before the source duration at its new rate")
	}
	app.Advance(2 * time.Millisecond)
	if !second.Ended() || !p[1].Ended || app.Find("id:status").Text != "Ended" {
		t.Fatal("the reactive playback state did not observe completion")
	}

	plays := make([]*audio.Playback, 33)
	for i := range plays {
		plays[i] = sound.Play(audio.Voice{})
	}
	app.Settle()
	for i, play := range plays {
		if play.Ended() != (i == 0) {
			t.Fatalf("playback %d: ended = %v", i, play.Ended())
		}
	}
	sound.Stop()
	app.Settle()
	if !plays[len(plays)-1].Ended() {
		t.Fatal("Sound.Stop left a playback active")
	}
	last := sound.Play(audio.Voice{})
	app.Settle()
	sound.Close()
	app.Settle()
	if !last.Ended() || !sound.Play(audio.Voice{}).Ended() {
		t.Fatal("closing the buffer left an active playback")
	}
}

func TestPlayerMuteKeepsItsTimelineAndVolume(t *testing.T) {
	var player *audio.Player
	const uri = "asset:music.wav"
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		player = audio.NewPlayer(cx, audio.Source{URI: uri}, audio.PlayerOptions{})
		return core.View{}
	}, playsAudio, func(a *nsgotest.App) { a.AudioDurations = map[string]time.Duration{uri: time.Second} })

	player.SetMuted(true)
	player.Play(nil)
	app.Settle()
	app.Advance(250 * time.Millisecond)
	model := app.Players[0]
	if !model.Playing || model.Volume != 0 || player.Position() != 250*time.Millisecond {
		t.Fatalf("muted play = %+v, position = %v", model, player.Position())
	}
	player.SetVolume(0.5)
	app.Settle()
	if model.Volume != 0 {
		t.Fatalf("setting volume removed mute: %+v", model)
	}
	player.SetMuted(false)
	app.Advance(250 * time.Millisecond)
	if !model.Playing || model.Volume != 0.5 || player.Position() != 500*time.Millisecond {
		t.Fatalf("unmuted play = %+v, position = %v", model, player.Position())
	}
	if want := []string{"open", "resume", "set", "set"}; !slices.Equal(model.Commands, want) {
		t.Fatalf("commands = %v, want %v", model.Commands, want)
	}
}

// A looping playback outlasts its sound until its own Stop, keeping its controls, and a muted player still loops from its LoopStart.
func TestLoopsKeepPlaybackControls(t *testing.T) {
	const effect, music, start = "asset:effect.wav", "asset:music.wav", 400 * time.Millisecond
	var sound *audio.Sound
	var player *audio.Player
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		audio.LoadSound(cx, audio.Source{URI: effect}, func(s *audio.Sound, err error) {
			if err != nil {
				t.Error(err)
			}
			sound = s
		})
		player = audio.NewPlayer(cx, audio.Source{URI: music}, audio.PlayerOptions{LoopStart: start})
		return core.View{}
	}, playsAudio, func(a *nsgotest.App) {
		a.AudioDurations = map[string]time.Duration{effect: 100 * time.Millisecond, music: time.Second}
	})
	loop := sound.Play(audio.Voice{Loop: true, Muted: true})
	once := sound.Play(audio.Voice{})
	player.SetLoop(true)
	player.SetMuted(true)
	player.Play(nil)
	app.Settle()
	app.Advance(1250 * time.Millisecond)
	p := app.Sounds[0].Playbacks
	if loop.Ended() || !once.Ended() || !p[0].Loop || p[0].Volume != 0 || p[1].Loop {
		t.Fatalf("after many lengths of the sound, loop ended %v, once ended %v, playbacks %+v %+v", loop.Ended(), once.Ended(), p[0], p[1])
	}
	loop.SetVolume(0.5)
	loop.SetRate(2)
	app.Settle()
	if loop.Ended() || p[0].Volume != 0.5 || p[0].Rate != 2 {
		t.Fatalf("the looping playback's settings = %+v", p[0])
	}
	loop.Stop()
	app.Settle()
	if !loop.Ended() || !p[0].Ended {
		t.Fatal("Stop left the looping playback active")
	}
	const repeated = start + 250*time.Millisecond
	if m := app.Players[0]; !m.Playing || !m.Loop || m.LoopStart != start || m.Volume != 0 || m.Position < repeated-time.Millisecond || m.Position > repeated+time.Millisecond {
		t.Fatalf("muted looping player = %+v, want it at %s", m, repeated)
	}
}
