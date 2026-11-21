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
	// Plays are its playbacks, oldest first, Stops how often the app stopped them, and Unloaded whether the app let it go.
	Plays    []core.SoundVoice
	Stops    int
	Unloaded bool
	id       uint32
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
	Loaded             bool
	Playing            bool
	Closed             bool
	id                 uint32
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
		a.Sounds = append(a.Sounds, &Sound{Source: core.AudioSource{URI: r.Source.URI, Data: r.Source.Data}, Duration: d, id: id})
		report(driver.AudioEvent{Phase: driver.AudioLoaded, Duration: d, Bytes: int64(d.Seconds() * decodedRate), Cut: cut})
		return
	case driver.AudioPlay, driver.AudioStop, driver.AudioUnload:
		i := slices.IndexFunc(a.Sounds, func(s *Sound) bool { return s.id == id })
		if i < 0 {
			return
		}
		s := a.Sounds[i]
		switch r.Command {
		case driver.AudioPlay:
			s.Plays = append(s.Plays, core.SoundVoice{Volume: r.Volume, Rate: r.Rate, Pan: r.Pan, StartAt: r.StartAt})
		case driver.AudioStop:
			s.Stops++
		default:
			s.Unloaded = true
		}
		return
	case driver.AudioOpen:
		a.Players = append(a.Players, &AudioPlayer{Source: core.AudioSource{URI: r.Source.URI, Data: r.Source.Data}, Session: r.Session,
			Position: r.Position, Volume: r.Volume, Rate: r.Rate, Loop: r.Loop, interval: r.ProgressInterval, id: id})
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
			p.laps += uint32(p.Position / p.Duration)
			p.Position %= p.Duration
		}
		if p.since += d; p.since >= p.interval {
			p.since %= p.interval
			report(driver.AudioProgress)
		}
	}
}
