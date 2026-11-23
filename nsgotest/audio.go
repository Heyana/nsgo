package nsgotest

import (
	"slices"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Sound is a sound the platform decoded.
type Sound struct {
	// Source is what it decoded and Duration how long it plays.
	Source   core.AudioSource
	Duration time.Duration
	// Plays records effective initial volume and rate, Playbacks the individual plays oldest first, Stops how often the app stopped all plays, and Unloaded whether the app let it go.
	Plays     []core.SoundVoice
	Playbacks []*AudioPlayback
	Stops     int
	Unloaded  bool
	id        uint32
	tree      *core.Tree
}

// AudioPlayback is one play of a Sound in the headless audio model.
type AudioPlayback struct {
	// Volume and Rate are its current settings and Loop whether it repeats until stopped. Ended reports all ways it stopped. Restricted says a restricted scope's sound played it, which the platform holds to the restricted playbacks' share of the output's voices.
	Volume, Rate float32
	Loop         bool
	Ended        bool
	Restricted   bool
	id           uint32
	sound        *Sound
	remaining    time.Duration
	startAt      time.Time
}

// AudioPlayer is a player the app opened.
type AudioPlayer struct {
	Source  core.AudioSource
	Session core.AudioSession
	// Commands are what the app asked of it, by name: open, preload, resume, pause, seek, set and close.
	Commands []string
	// Position is where it is, and Duration how long it plays once Loaded; Volume, Rate and Loop are how it plays, Playing whether it does and Closed whether the app closed it.
	Position, Duration time.Duration
	Volume, Rate       float32
	Loop               bool
	// LoopStart is where playback returns after its first pass.
	LoopStart time.Duration
	Loaded    bool
	Playing   bool
	Closed    bool
	id        uint32
	// interval is the time between its progress reports and since the time since the last.
	interval, since time.Duration
	seeks, laps     uint32
	// ended says it played to its end with no seek since.
	ended bool
}

// decodedRate is the bytes a second of audio takes decoded, as 48 kHz stereo floats.
const decodedRate = 48000 * 2 * 4

// audio answers an ItemAudio as a platform would, decoding the durations AudioDurations gives.
func (a *App) audio(id uint32, r driver.AudioRequest) {
	report := func(e driver.AudioEvent) { a.post(driver.Event{Kind: driver.EventAudio, Count: id, Data: &e}) }
	fail := func(code, message string) {
		report(driver.AudioEvent{Phase: driver.AudioFailure, Error: code, Message: message})
	}
	switch r.Command {
	case driver.AudioReadLatency:
		report(driver.AudioEvent{Phase: driver.AudioLatency, Latency: a.AudioOutputLatency})
		return
	case driver.AudioLoad:
		if int64(len(r.Source.Data)) > r.MaxBytes {
			fail(driver.AudioQuotaExceeded, "nsgotest: the sound has more than its bytes")
			return
		}
		d, ok := a.audioDuration(core.AudioSource{URI: r.Source.URI, Data: r.Source.Data})
		if !ok {
			fail(driver.AudioDecode, "nsgotest: no audio at "+r.Source.URI)
			return
		}
		cut := d > r.MaxDuration
		if cut {
			d = r.MaxDuration
		}
		a.Sounds = append(a.Sounds, &Sound{Source: core.AudioSource{URI: r.Source.URI, Data: r.Source.Data}, Duration: d, id: id, tree: a.responseTree})
		report(driver.AudioEvent{Phase: driver.AudioLoaded, Duration: d, Bytes: int64(d.Seconds() * decodedRate), Cut: cut})
		return
	case driver.AudioPlay, driver.AudioStop, driver.AudioUnload, driver.AudioPlaybackSet, driver.AudioPlaybackStop:
		i := slices.IndexFunc(a.Sounds, func(s *Sound) bool { return s.id == id })
		if i < 0 {
			return
		}
		s := a.Sounds[i]
		switch r.Command {
		case driver.AudioPlay:
			s.Plays = append(s.Plays, core.SoundVoice{Volume: r.Volume, Rate: r.Rate, Pan: r.Pan, StartAt: r.StartAt, Loop: r.Loop})
			if r.Playback != 0 {
				a.startPlayback(s, r)
			}
		case driver.AudioStop:
			s.Stops++
			a.stopPlaybacks(s, 0)
		case driver.AudioUnload:
			s.Unloaded = true
			a.stopPlaybacks(s, 0)
		case driver.AudioPlaybackSet:
			for _, p := range a.audioPlaybacks {
				if p.id == r.Playback && p.sound == s {
					p.Volume, p.Rate = r.Volume, r.Rate
					break
				}
			}
		case driver.AudioPlaybackStop:
			a.stopPlaybacks(s, r.Playback)
		}
		return
	case driver.AudioOpen:
		a.Players = append(a.Players, &AudioPlayer{Source: core.AudioSource{URI: r.Source.URI, Data: r.Source.Data}, Session: r.Session,
			Position: r.Position, Volume: r.Volume, Rate: r.Rate, Loop: r.Loop, LoopStart: r.LoopStart, interval: r.ProgressInterval, id: id})
	}
	i := slices.IndexFunc(a.Players, func(p *AudioPlayer) bool { return p.id == id })
	if i < 0 {
		return
	}
	p := a.Players[i]
	p.Commands = append(p.Commands, r.Command.String())
	progress := func(phase driver.AudioPhase) {
		report(driver.AudioEvent{Phase: phase, Position: p.Position, Duration: p.Duration, Seeks: p.seeks, Laps: p.laps})
	}
	load := func() bool {
		if p.Loaded {
			return true
		}
		d, ok := a.audioDuration(p.Source)
		if !ok {
			fail(driver.AudioDecode, "nsgotest: no audio at "+p.Source.URI)
			return false
		}
		p.Loaded, p.Duration = true, d
		if d > 0 {
			p.Position = min(p.Position, d)
		}
		report(driver.AudioEvent{Phase: driver.AudioOpened, Duration: d})
		return true
	}
	switch r.Command {
	case driver.AudioPreload:
		load()
	case driver.AudioResume:
		if a.RefusePlayback {
			fail(driver.AudioNotAllowed, "nsgotest: RefusePlayback")
			return
		}
		if !load() {
			return
		}
		if p.ended {
			p.Position, p.ended = 0, false
		}
		p.Playing, p.since = true, 0
		report(driver.AudioEvent{Phase: driver.AudioPlaying})
	case driver.AudioPause:
		if p.Playing {
			p.Playing = false
			progress(driver.AudioPaused)
		}
	case driver.AudioSeek:
		p.seeks++
		p.Position, p.ended = r.Position, false
		if p.Loaded {
			if p.Duration > 0 {
				p.Position = min(p.Position, p.Duration)
			}
			progress(driver.AudioProgress)
		}
	case driver.AudioSet:
		p.Volume, p.Rate, p.Loop = r.Volume, r.Rate, r.Loop
	case driver.AudioClose:
		p.Closed, p.Playing = true, false
	}
}

func (a *App) startPlayback(s *Sound, r driver.AudioRequest) {
	oldest, active := -1, 0
	for i, p := range a.audioPlaybacks {
		if p.sound.tree == s.tree {
			if oldest < 0 {
				oldest = i
			}
			active++
		}
	}
	if active >= driver.MaxSoundVoices {
		a.endPlayback(a.audioPlaybacks[oldest])
		a.audioPlaybacks = slices.Delete(a.audioPlaybacks, oldest, oldest+1)
	}
	p := &AudioPlayback{Volume: r.Volume, Rate: r.Rate, Loop: r.Loop, Restricted: r.Restricted, id: r.Playback, sound: s, remaining: s.Duration, startAt: r.StartAt}
	s.Playbacks = append(s.Playbacks, p)
	a.audioPlaybacks = append(a.audioPlaybacks, p)
}

func (a *App) endPlayback(p *AudioPlayback) {
	p.Ended = true
	a.postTo(p.sound.tree, driver.Event{Kind: driver.EventAudio, Count: p.sound.id,
		Data: &driver.AudioEvent{Phase: driver.AudioPlaybackEnded, Playback: p.id}})
}

func (a *App) stopPlaybacks(s *Sound, id uint32) {
	a.audioPlaybacks = slices.DeleteFunc(a.audioPlaybacks, func(p *AudioPlayback) bool {
		if p.sound != s || id != 0 && p.id != id {
			return false
		}
		a.endPlayback(p)
		return true
	})
}

// audioDuration is what AudioDurations says src lasts.
func (a *App) audioDuration(src core.AudioSource) (time.Duration, bool) {
	key := src.URI
	if len(src.Data) > 0 {
		key = string(src.Data)
	}
	d, ok := a.AudioDurations[key]
	return d, ok
}

// playAudio moves the players playing on by d of the clock's time, reporting their progress and ends.
func (a *App) playAudio(d time.Duration) {
	a.audioPlaybacks = slices.DeleteFunc(a.audioPlaybacks, func(p *AudioPlayback) bool {
		elapsed := d
		if sinceStart := a.Clock.Now().Sub(p.startAt); sinceStart < elapsed {
			elapsed = max(0, sinceStart)
		}
		if p.Loop {
			return false
		}
		p.remaining -= time.Duration(float64(elapsed) * float64(p.Rate))
		if p.remaining > 0 {
			return false
		}
		a.endPlayback(p)
		return true
	})
	for _, p := range a.Players {
		if !p.Playing {
			continue
		}
		report := func(phase driver.AudioPhase) {
			a.post(driver.Event{Kind: driver.EventAudio, Count: p.id,
				Data: &driver.AudioEvent{Phase: phase, Position: p.Position, Duration: p.Duration, Seeks: p.seeks, Laps: p.laps}})
		}
		p.Position += time.Duration(float64(d) * float64(p.Rate))
		if p.Duration > 0 && p.Position >= p.Duration {
			if !p.Loop {
				p.Position, p.Playing, p.ended = p.Duration, false, true
				report(driver.AudioEnded)
				continue
			}
			start := driver.AudioLoopStart(p.LoopStart, p.Duration)
			repeat := p.Duration - start
			excess := p.Position - p.Duration
			p.laps += 1 + uint32(excess/repeat)
			p.Position = start + excess%repeat
		}
		if p.since += d; p.since >= p.interval {
			p.since %= p.interval
			report(driver.AudioProgress)
		}
	}
}
