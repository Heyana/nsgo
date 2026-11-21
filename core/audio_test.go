package core

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// audioItems are b's ItemAudio requests.
func audioItems(b *driver.MountBatch) []driver.MountItem {
	var items []driver.MountItem
	for _, it := range b.Items {
		if it.Kind == driver.ItemAudio {
			items = append(items, it)
		}
	}
	return items
}

// audioCommands names b's audio requests as "id command".
func audioCommands(b *driver.MountBatch) []string {
	var out []string
	for _, it := range audioItems(b) {
		out = append(out, fmt.Sprintf("%d %s", it.Count, it.Audio.Command))
	}
	return out
}

func newAudioHost(t *testing.T, app func(cx *Scope) Node) *host {
	h := newHost(t, Size{Width: 100, Height: 100}, app)
	h.tree.SetEnvironment(Environment{Audio: true})
	h.commit()
	return h
}

func reportAudio(h *host, id uint32, e driver.AudioEvent) {
	h.tree.Dispatch(driver.Event{Kind: driver.EventAudio, Count: id, Data: &e})
}

func TestSoundsLoadOnceAndPlay(t *testing.T) {
	var cx *Scope
	h := newAudioHost(t, func(c *Scope) Node { cx = c; return View{} })
	var sounds []*Sound
	load := func(src AudioSource) {
		cx.LoadSound(src, func(s *Sound, err error) {
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			sounds = append(sounds, s)
		})
	}
	load(AudioSource{URI: "asset:sounds/click.wav"})
	load(AudioSource{URI: "asset:sounds/click.wav"})
	items := audioItems(h.commit())
	if len(items) != 1 {
		t.Fatalf("one load for two of one source, got %d items", len(items))
	}
	req := items[0].Audio
	if req.Command != driver.AudioLoad || req.Source.URI != "asset:sounds/click.wav" || req.MaxBytes != 8<<20 || req.MaxDuration != 5*time.Second {
		t.Fatalf("load = %+v", req)
	}
	id := items[0].Count
	if len(sounds) != 0 {
		t.Fatal("the load answered before the platform did")
	}
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioLoaded, Duration: 120 * time.Millisecond, Bytes: 46080})
	if len(sounds) != 2 || sounds[0].Duration() != 120*time.Millisecond {
		t.Fatalf("sounds %v", sounds)
	}

	// A third load of the source takes the decoded sound, in a later turn.
	load(AudioSource{URI: "asset:sounds/click.wav"})
	if len(sounds) != 2 {
		t.Fatal("a loaded sound answered in the same turn")
	}
	h.drain()
	if len(sounds) != 3 || len(audioItems(h.commit())) != 0 {
		t.Fatalf("third load: %d sounds", len(sounds))
	}

	// Playbacks overlap, each a command in the turn's batch, the voice clamped with 0 as 1 for Volume and Rate and 0 centered for Pan.
	sounds[0].Play(SoundVoice{})
	sounds[1].Play(SoundVoice{Volume: 0.5, Rate: 1.2, Pan: -0.5})
	sounds[2].Play(SoundVoice{Volume: 3, Rate: 9, Pan: 2})
	sounds[2].Play(SoundVoice{Volume: -1, Rate: 0.1, Pan: -2})
	sounds[2].Play(SoundVoice{Pan: float32(math.NaN())})
	sounds[2].Play(SoundVoice{Pan: float32(math.Copysign(0, -1))})
	sounds[2].Play(SoundVoice{Pan: float32(math.Inf(-1))})
	sounds[2].Play(SoundVoice{Pan: float32(math.Inf(1))})
	var voices []SoundVoice
	for _, it := range audioItems(h.commit()) {
		if it.Count != id || it.Audio.Command != driver.AudioPlay {
			t.Fatalf("play = %d %v", it.Count, it.Audio.Command)
		}
		voices = append(voices, SoundVoice{Volume: it.Audio.Volume, Rate: it.Audio.Rate, Pan: it.Audio.Pan})
	}
	want := []SoundVoice{
		{Volume: 1, Rate: 1, Pan: 0},
		{Volume: 0.5, Rate: 1.2, Pan: -0.5},
		{Volume: 1, Rate: 2, Pan: 1},
		{Volume: 0, Rate: 0.5, Pan: -1},
		{Volume: 1, Rate: 1, Pan: 0},
		{Volume: 1, Rate: 1, Pan: 0},
		{Volume: 1, Rate: 1, Pan: -1},
		{Volume: 1, Rate: 1, Pan: 1},
	}
	if !slices.Equal(voices, want) {
		t.Fatalf("voices %v, want %v", voices, want)
	}
	if math.Signbit(float64(voices[5].Pan)) {
		t.Fatal("-0 pan kept its sign bit")
	}

	// The decoded sound goes with the last Sound over it.
	sounds[0].Stop()
	sounds[0].Close()
	sounds[1].Close()
	sounds[0].Play(SoundVoice{})
	if got := audioCommands(h.commit()); !slices.Equal(got, []string{fmt.Sprint(id, " stop")}) {
		t.Fatalf("after two closes: %v", got)
	}
	sounds[2].Close()
	sounds[2].Close()
	if got := audioCommands(h.commit()); !slices.Equal(got, []string{fmt.Sprint(id, " unload")}) {
		t.Fatalf("after the last close: %v", got)
	}
	// A late report finds no one, and a new load decodes again.
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioLoaded, Duration: time.Second})
	load(AudioSource{URI: "asset:sounds/click.wav"})
	if items := audioItems(h.commit()); len(items) != 1 || items[0].Count == id || items[0].Audio.Command != driver.AudioLoad {
		t.Fatalf("load again: %v", items)
	}
}

func TestSoundBytesShareByDigest(t *testing.T) {
	var cx *Scope
	h := newAudioHost(t, func(c *Scope) Node { cx = c; return View{} })
	wav := []byte("RIFF\x24\x00\x00\x00WAVEfmt ")
	cx.LoadSound(AudioSource{Data: wav}, nil)
	cx.LoadSound(AudioSource{Data: slices.Clone(wav)}, nil)
	cx.LoadSound(AudioSource{Data: append(slices.Clone(wav), 0)}, nil)
	items := audioItems(h.commit())
	if len(items) != 2 || string(items[0].Audio.Source.Data) != string(wav) {
		t.Fatalf("loads %v", audioCommands(&driver.MountBatch{Items: items}))
	}
}

func TestSoundFailures(t *testing.T) {
	var cx *Scope
	h := newAudioHost(t, func(c *Scope) Node { cx = c; return View{} })
	var errs []error
	load := func(src AudioSource) uint32 {
		cx.LoadSound(src, func(s *Sound, err error) {
			if s != nil {
				t.Fatalf("%v loaded", src.URI)
			}
			errs = append(errs, err)
		})
		items := audioItems(h.commit())
		if len(items) == 0 {
			return 0
		}
		return items[0].Count
	}
	expect := func(code AudioError, unload uint32) {
		t.Helper()
		h.drain()
		if len(errs) != 1 || !errors.Is(errs[0], code) {
			t.Fatalf("errors %v, want %v", errs, code)
		}
		errs = nil
		got := audioCommands(h.commit())
		if unload != 0 && !slices.Equal(got, []string{fmt.Sprint(unload, " unload")}) || unload == 0 && len(got) != 0 {
			t.Fatalf("after the failure: %v", got)
		}
	}

	// The platform's failure, with its words.
	id := load(AudioSource{URI: "asset:a.flac"})
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioFailure, Error: driver.AudioDecode, Message: "not audio"})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "not audio") {
		t.Fatalf("errors %v", errs)
	}
	expect(AudioDecode, id)

	// A sound the platform cut at 5 seconds, or that would pass 16 MiB decoded, is freed and fails with QuotaExceeded.
	id = load(AudioSource{URI: "asset:long.wav"})
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioLoaded, Duration: 5 * time.Second, Bytes: 1 << 20, Cut: true})
	expect(AudioQuotaExceeded, id)
	cx.LoadSound(AudioSource{URI: "asset:a.wav"}, nil)
	first := audioItems(h.commit())[0].Count
	reportAudio(h, first, driver.AudioEvent{Phase: driver.AudioLoaded, Duration: 4 * time.Second, Bytes: 12 << 20})
	id = load(AudioSource{URI: "asset:b.wav"})
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioLoaded, Duration: 4 * time.Second, Bytes: 5 << 20})
	expect(AudioQuotaExceeded, id)

	// More than 8 MiB of bytes fails before the platform is asked.
	load(AudioSource{Data: make([]byte, 8<<20+1)})
	expect(AudioQuotaExceeded, 0)
	load(AudioSource{})
	expect(AudioNotSupported, 0)

	// An engine holds at most 64 sounds, those loading among them.
	for i := range 63 {
		cx.LoadSound(AudioSource{URI: fmt.Sprintf("asset:%d.wav", i)}, nil)
	}
	if n := len(audioItems(h.commit())); n != 63 {
		t.Fatalf("%d loads", n)
	}
	load(AudioSource{URI: "asset:65.wav"})
	expect(AudioQuotaExceeded, 0)
}

func TestSoundsUnavailable(t *testing.T) {
	var cx *Scope
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node { cx = c; return View{} })
	var err error
	cx.LoadSound(AudioSource{URI: "asset:a.wav"}, func(_ *Sound, e error) { err = e })
	if len(audioItems(h.commit())) != 0 {
		t.Fatal("asked the platform")
	}
	h.drain()
	if !errors.Is(err, AudioUnavailable) {
		t.Fatalf("err %v", err)
	}
}

// audioAccess grants audio when audio is set, and the network for hosts.
type audioAccess struct {
	audio bool
	hosts []string
}

func (a audioAccess) Allows(req AccessRequest) bool {
	switch req.Permission {
	case AccessAudio:
		return a.audio
	case AccessNetwork:
		return slices.ContainsFunc(a.hosts, func(h string) bool { return strings.HasPrefix(req.Resource, "https://"+h+"/") })
	}
	return false
}

func (audioAccess) WebHosts() []string { return nil }

func TestAudioHeldToAccess(t *testing.T) {
	var cx *Scope
	h := newAudioHost(t, func(c *Scope) Node { cx = c; return View{} })
	silent, held := cx.Child(), cx.Child()
	silent.ProvideAccess(audioAccess{})
	held.ProvideAccess(audioAccess{audio: true, hosts: []string{"ok.com"}})
	var errs []error
	record := func(_ *Sound, err error) { errs = append(errs, err) }
	silent.LoadSound(AudioSource{Data: []byte("RIFF")}, record)
	held.LoadSound(AudioSource{URI: "https://evil.io/a.mp3"}, record)
	held.LoadSound(AudioSource{URI: "/etc/a.wav"}, record)
	held.LoadSound(AudioSource{URI: "https://ok.com/a.mp3"}, record)
	held.LoadSound(AudioSource{URI: "asset:a.wav"}, record)
	held.LoadSound(AudioSource{Data: []byte("RIFF")}, record)
	p := silent.NewAudioPlayer(AudioSource{URI: "asset:a.m4a"}, AudioPlayerOptions{OnError: func(err error) { errs = append(errs, err) }})
	p.Play(func(err error) { errs = append(errs, err) })
	if got := len(audioItems(h.commit())); got != 3 {
		t.Fatalf("%d loads asked of the platform, want the three granted", got)
	}
	h.drain()
	if len(errs) != 5 {
		t.Fatalf("errors %v", errs)
	}
	for _, err := range errs {
		var refusal *Error
		if !errors.Is(err, AudioNotAllowed) || !errors.As(err, &refusal) || refusal.Code != CodeAccessDenied {
			t.Fatalf("%v is no refusal", err)
		}
	}
	if p.State() != AudioPlayerFailed {
		t.Fatalf("refused player is %v", p.State())
	}
}

func TestSoundLoadGoesWithItsOwner(t *testing.T) {
	var cx *Scope
	h := newAudioHost(t, func(c *Scope) Node { cx = c; return View{} })
	owner := cx.Child()
	called := false
	owner.LoadSound(AudioSource{URI: "asset:a.wav"}, func(*Sound, error) { called = true })
	id := audioItems(h.commit())[0].Count
	owner.Dispose()
	if got := audioCommands(h.commit()); !slices.Equal(got, []string{fmt.Sprint(id, " unload")}) {
		t.Fatalf("after the owner went: %v", got)
	}
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioLoaded, Duration: time.Second})
	h.drain()
	if called {
		t.Fatal("the load called back after its owner went")
	}

	// A loaded sound goes with the owner that loaded it.
	owner = cx.Child()
	var sound *Sound
	owner.LoadSound(AudioSource{URI: "asset:b.wav"}, func(s *Sound, _ error) { sound = s })
	id = audioItems(h.commit())[0].Count
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioLoaded, Duration: time.Second})
	owner.Dispose()
	sound.Play(SoundVoice{})
	if got := audioCommands(h.commit()); !slices.Equal(got, []string{fmt.Sprint(id, " unload")}) {
		t.Fatalf("after the owner went: %v", got)
	}
}

func TestPlayerFollowsThePlatform(t *testing.T) {
	var cx *Scope
	h := newAudioHost(t, func(c *Scope) Node { cx = c; return View{} })
	var log []string
	p := cx.NewAudioPlayer(AudioSource{URI: "https://example.com/voice.m4a"}, AudioPlayerOptions{
		OnLoad:       func(d time.Duration) { log = append(log, fmt.Sprint("load ", d)) },
		OnProgress:   func(pos, d time.Duration) { log = append(log, fmt.Sprint("progress ", pos)) },
		OnBuffer:     func(b bool) { log = append(log, fmt.Sprint("buffer ", b)) },
		OnEnd:        func() { log = append(log, "end") },
		OnError:      func(err error) { log = append(log, "error "+err.Error()) },
		AudioSession: AudioSessionPlayback,
	})
	p.SetVolume(0.5)
	p.Seek(2 * time.Second)
	if items := audioItems(h.commit()); len(items) != 0 || p.State() != AudioPlayerIdle || p.Position() != 2*time.Second {
		t.Fatalf("a new player asked the platform: %v", items)
	}
	p.Play(func(err error) { log = append(log, fmt.Sprint("played ", err)) })
	items := audioItems(h.commit())
	if len(items) != 2 || items[0].Audio.Command != driver.AudioOpen || items[1].Audio.Command != driver.AudioResume {
		t.Fatalf("play = %v", audioCommands(&driver.MountBatch{Items: items}))
	}
	open, id := items[0].Audio, items[0].Count
	if open.Source.URI != "https://example.com/voice.m4a" || open.Volume != 0.5 || open.Rate != 1 || open.Position != 2*time.Second ||
		open.Session != AudioSessionPlayback || open.ProgressInterval != 250*time.Millisecond {
		t.Fatalf("open = %+v", open)
	}
	if p.State() != AudioPlayerLoading {
		t.Fatalf("state %v", p.State())
	}
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioOpened, Duration: 10 * time.Second})
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioPlaying})
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioProgress, Position: 2250 * time.Millisecond, Duration: 10 * time.Second})
	// A report that goes back without a seek is held at the last.
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioProgress, Position: 2200 * time.Millisecond, Duration: 10 * time.Second})
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioBuffer, Buffering: true})
	if p.State() != AudioPlayerLoading {
		t.Fatalf("buffering: %v", p.State())
	}
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioBuffer, Buffering: false})
	if p.State() != AudioPlayerPlaying || p.Duration() != 10*time.Second {
		t.Fatalf("state %v, duration %v", p.State(), p.Duration())
	}
	want := []string{"load 10s", "played <nil>", "progress 2.25s", "progress 2.25s", "buffer true", "buffer false"}
	if !slices.Equal(log, want) {
		t.Fatalf("log %q, want %q", log, want)
	}

	// A seek takes the position at once; a report from before it is dropped, and the first after it may go back.
	log = nil
	p.Seek(time.Second)
	if got := audioItems(h.commit()); len(got) != 1 || got[0].Audio.Command != driver.AudioSeek || got[0].Audio.Position != time.Second {
		t.Fatalf("seek = %v", got)
	}
	if p.Position() != time.Second {
		t.Fatalf("position %v", p.Position())
	}
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioProgress, Position: 2500 * time.Millisecond})
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioProgress, Position: 990 * time.Millisecond, Seeks: 1})
	p.Seek(time.Minute)
	if p.Position() != 10*time.Second {
		t.Fatalf("seek past the end: %v", p.Position())
	}
	// A loop starting over goes back too.
	p.SetLoop(true)
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioProgress, Position: 9 * time.Second, Seeks: 2})
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioProgress, Position: 500 * time.Millisecond, Seeks: 2, Laps: 1})
	if got := audioCommands(h.commit()); !slices.Equal(got, []string{fmt.Sprint(id, " seek"), fmt.Sprint(id, " set")}) {
		t.Fatalf("commands %v", got)
	}
	// The system pausing it pauses it, reporting where it is.
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioPaused, Position: 600 * time.Millisecond, Seeks: 2, Laps: 1})
	if p.State() != AudioPlayerPaused {
		t.Fatalf("paused by the system: %v", p.State())
	}
	want = []string{"progress 990ms", "progress 9s", "progress 500ms", "progress 600ms"}
	if !slices.Equal(log, want) {
		t.Fatalf("log %q, want %q", log, want)
	}

	// It ends with its duration as its position, and plays from the start again.
	log = nil
	p.SetLoop(false)
	p.Play(nil)
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioPlaying})
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioEnded})
	if p.State() != AudioPlayerEnded || p.Position() != 10*time.Second {
		t.Fatalf("ended: %v at %v", p.State(), p.Position())
	}
	p.Play(nil)
	if p.Position() != 0 {
		t.Fatalf("replay from %v", p.Position())
	}
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioProgress, Position: 250 * time.Millisecond, Seeks: 2, Laps: 1})
	want = []string{"progress 10s", "end", "progress 250ms"}
	if !slices.Equal(log, want) {
		t.Fatalf("log %q, want %q", log, want)
	}

	// A failure goes through OnError and Play's callback.
	log = nil
	h.commit()
	p.Play(func(err error) { log = append(log, fmt.Sprint("played ", errors.Is(err, AudioNetwork))) })
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioFailure, Error: driver.AudioNetwork, Message: "offline"})
	if p.State() != AudioPlayerFailed {
		t.Fatalf("failed: %v", p.State())
	}
	want = []string{"error nsgo: audio: network: offline", "played true"}
	if !slices.Equal(log, want) {
		t.Fatalf("log %q, want %q", log, want)
	}

	// Closed, it tells the platform and hears nothing more.
	log = nil
	h.commit()
	p.Play(func(error) { log = append(log, "played") })
	p.Close()
	p.Close()
	got := audioCommands(h.commit())
	if !slices.Equal(got, []string{fmt.Sprint(id, " resume"), fmt.Sprint(id, " close")}) {
		t.Fatalf("close: %v", got)
	}
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioPlaying})
	if len(log) != 0 {
		t.Fatalf("after close: %q", log)
	}
}

func TestPlayerPreloadsAndPauses(t *testing.T) {
	var cx *Scope
	h := newAudioHost(t, func(c *Scope) Node { cx = c; return View{} })
	var played []error
	p := cx.NewAudioPlayer(AudioSource{URI: "asset:book.mp3"}, AudioPlayerOptions{ProgressInterval: time.Millisecond})
	p.Preload()
	items := audioItems(h.commit())
	if got := audioCommands(&driver.MountBatch{Items: items}); len(got) != 2 || !strings.HasSuffix(got[1], "preload") ||
		items[0].Audio.ProgressInterval != 16*time.Millisecond {
		t.Fatalf("preload: %v", got)
	}
	id := items[0].Count
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioOpened, Duration: time.Hour})
	if p.State() != AudioPlayerPaused || p.Duration() != time.Hour {
		t.Fatalf("preloaded: %v, %v", p.State(), p.Duration())
	}
	// A seek while paused shows at once, and Play starts from there.
	p.Seek(30 * time.Minute)
	p.Play(func(err error) { played = append(played, err) })
	p.Pause()
	if got := audioCommands(h.commit()); !slices.Equal(got, []string{fmt.Sprint(id, " seek"), fmt.Sprint(id, " resume"), fmt.Sprint(id, " pause")}) {
		t.Fatalf("commands %v", got)
	}
	if p.Position() != 30*time.Minute {
		t.Fatalf("position %v", p.Position())
	}
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioPlaying})
	if len(played) != 1 || played[0] != nil {
		t.Fatalf("played %v", played)
	}
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioPaused, Position: 30 * time.Minute})
	if p.State() != AudioPlayerPaused {
		t.Fatalf("paused: %v", p.State())
	}
	// Play while playing calls back in a later turn.
	p.Play(nil)
	h.commit()
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioPlaying})
	p.Play(func(err error) { played = append(played, err) })
	h.drain()
	if len(played) != 2 || len(audioItems(h.commit())) != 0 {
		t.Fatalf("played %v", played)
	}
}

func TestPlayerPlaysAfterPause(t *testing.T) {
	var cx *Scope
	h := newAudioHost(t, func(c *Scope) Node { cx = c; return View{} })
	var played []error
	p := cx.NewAudioPlayer(AudioSource{URI: "asset:book.mp3"}, AudioPlayerOptions{})
	p.Play(nil)
	id := audioItems(h.commit())[0].Count
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioOpened, Duration: time.Minute})
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioPlaying})

	// A restart in one turn: the platform hears the pause, the seek and then the play.
	p.Pause()
	p.Seek(0)
	p.Play(func(err error) { played = append(played, err) })
	if got := audioCommands(h.commit()); !slices.Equal(got, []string{fmt.Sprint(id, " pause"), fmt.Sprint(id, " seek"), fmt.Sprint(id, " resume")}) {
		t.Fatalf("restart: %v", got)
	}
	h.drain()
	if len(played) != 0 {
		t.Fatalf("called back before the platform played: %v", played)
	}
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioPaused, Seeks: 1})
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioPlaying})
	if p.State() != AudioPlayerPlaying || len(played) != 1 || played[0] != nil {
		t.Fatalf("restarted: %v, played %v", p.State(), played)
	}

	// A toggle pressed twice before the platform reports the pause plays again.
	p.Pause()
	h.commit()
	p.Play(func(err error) { played = append(played, err) })
	if got := audioCommands(h.commit()); !slices.Equal(got, []string{fmt.Sprint(id, " resume")}) {
		t.Fatalf("toggle: %v", got)
	}
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioPaused, Seeks: 1})
	reportAudio(h, id, driver.AudioEvent{Phase: driver.AudioPlaying})
	if p.State() != AudioPlayerPlaying || len(played) != 2 || played[1] != nil {
		t.Fatalf("toggled: %v, played %v", p.State(), played)
	}
}

func TestPlayerUnavailable(t *testing.T) {
	var cx *Scope
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node { cx = c; return View{} })
	var errs []error
	p := cx.NewAudioPlayer(AudioSource{URI: "asset:a.mp3"}, AudioPlayerOptions{OnError: func(err error) { errs = append(errs, err) }})
	p.Play(func(err error) { errs = append(errs, err) })
	if len(audioItems(h.commit())) != 0 || len(errs) != 0 {
		t.Fatal("asked the platform, or failed in the same turn")
	}
	h.drain()
	if len(errs) != 2 || !errors.Is(errs[0], AudioUnavailable) || !errors.Is(errs[1], AudioUnavailable) || p.State() != AudioPlayerFailed {
		t.Fatalf("errors %v, state %v", errs, p.State())
	}
}

func TestPlayerGoesWithItsOwner(t *testing.T) {
	var cx *Scope
	h := newAudioHost(t, func(c *Scope) Node { cx = c; return View{} })
	owner := cx.Child()
	p := owner.NewAudioPlayer(AudioSource{URI: "asset:a.mp3"}, AudioPlayerOptions{})
	p.Play(nil)
	id := audioItems(h.commit())[0].Count
	owner.Dispose()
	if got := audioCommands(h.commit()); !slices.Equal(got, []string{fmt.Sprint(id, " close")}) {
		t.Fatalf("after the owner went: %v", got)
	}
	if !strings.Contains(AudioPlayerPaused.String()+AudioPlayerFailed.String(), "pausedfailed") {
		t.Fatal("state names")
	}
}

func TestExplicitScopeOwnsAudioPlayerAndSound(t *testing.T) {
	var appScope *Scope
	var shown *Signal[bool]
	var player *AudioPlayer
	var sound *Sound
	h := newAudioHost(t, func(app *Scope) Node {
		appScope = app
		shown = NewSignal(app, true)
		return Show{When: shown.Get, Then: func(*Scope) Node {
			return View{
				TestID: "play",
				Style:  Style{Width: Pt(40), Height: Pt(20)},
				OnPress: func(PressEvent) {
					player = app.NewAudioPlayer(AudioSource{URI: "asset:song.mp3"}, AudioPlayerOptions{})
					player.Play(nil)
					app.LoadSound(AudioSource{URI: "asset:click.wav"}, func(s *Sound, err error) {
						if err == nil {
							sound = s
						}
					})
				},
			}
		}}
	})
	h.tree.Dispatch(driver.Event{Kind: driver.EventPress, ID: h.find("play").id})
	items := audioItems(h.commit())
	if player == nil || len(items) < 2 {
		t.Fatalf("player=%v items=%v", player, audioCommands(&driver.MountBatch{Items: items}))
	}
	var playerID, soundID uint32
	for _, it := range items {
		switch it.Audio.Command {
		case driver.AudioOpen:
			playerID = it.Count
		case driver.AudioLoad:
			soundID = it.Count
		}
	}
	reportAudio(h, soundID, driver.AudioEvent{Phase: driver.AudioLoaded, Duration: 100 * time.Millisecond, Bytes: 1024})
	if sound == nil {
		t.Fatal("sound did not load")
	}

	// Disposing the button's Show branch leaves app-owned player and sound alive.
	shown.Set(false)
	if cmds := audioCommands(h.commit()); len(cmds) != 0 {
		t.Fatalf("button branch disposal closed app-owned audio: %v", cmds)
	}
	if player.closed || sound.closed {
		t.Fatalf("player.closed=%v sound.closed=%v after branch disposal", player.closed, sound.closed)
	}

	// Access restrictions still apply from both the running owner and the explicit scope.
	restricted := appScope.Child()
	restricted.ProvideAccess(audioAccess{})
	var deniedFromRunning, deniedFromTarget error
	restricted.Run(func() {
		p := appScope.NewAudioPlayer(AudioSource{URI: "asset:a.mp3"}, AudioPlayerOptions{})
		p.Play(func(err error) { deniedFromRunning = err })
	})
	appScope.Run(func() {
		p := restricted.NewAudioPlayer(AudioSource{URI: "asset:a.mp3"}, AudioPlayerOptions{})
		p.Play(func(err error) { deniedFromTarget = err })
	})
	h.drain()
	if !errors.Is(deniedFromRunning, AudioNotAllowed) || !errors.Is(deniedFromTarget, AudioNotAllowed) {
		t.Fatalf("access checks: running=%v target=%v", deniedFromRunning, deniedFromTarget)
	}

	// Disposing app closes both the player and the sound.
	appScope.Dispose()
	cmds := audioCommands(h.commit())
	if !slices.Contains(cmds, fmt.Sprint(playerID, " close")) || !slices.Contains(cmds, fmt.Sprint(soundID, " unload")) {
		t.Fatalf("app disposal commands = %v, want close %d and unload %d", cmds, playerID, soundID)
	}

	// LoadSound on a disposed scope emits nothing.
	appScope.LoadSound(AudioSource{URI: "asset:dead.wav"}, nil)
	if cmds := audioCommands(h.commit()); len(cmds) != 0 {
		t.Fatalf("LoadSound on disposed scope emitted %v", cmds)
	}
}
