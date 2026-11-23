//go:build android && nsgotest

package android

import (
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// These fixtures encode 1.2 seconds of mono silence at 44.1 kHz.
//
//go:embed testdata/audio/silence.m4a
var silentAAC []byte

//go:embed testdata/audio/silence.mp3
var silentMP3 []byte

func silentAudioWAV(length time.Duration) []byte {
	const rate = 8000
	samples := int(length.Seconds() * rate)
	data := make([]byte, 44+2*samples)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], rate)
	binary.LittleEndian.PutUint32(data[28:], 2*rate)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(2*samples))
	return data
}

type androidAudioSource struct {
	name   string
	source core.AudioSource
}

func nativeAudioSources(t *testing.T) []androidAudioSource {
	t.Helper()
	mp3 := filepath.Join(t.TempDir(), "silence.mp3")
	if err := os.WriteFile(mp3, silentMP3, 0o600); err != nil {
		t.Fatal(err)
	}
	return []androidAudioSource{
		{"WAV bytes", core.AudioSource{Data: silentAudioWAV(1200 * time.Millisecond)}},
		{"AAC data URL", core.AudioSource{URI: "data:audio/mp4;base64," + base64.StdEncoding.EncodeToString(silentAAC)}},
		{"MP3 file", core.AudioSource{URI: mp3}},
	}
}

func startAudioTest(t *testing.T) (*nsgotest.App, *core.Scope) {
	t.Helper()
	if nsgotest.Target() != "android" {
		t.Skip("requires Android audio")
	}
	var cx *core.Scope
	app := nsgotest.Start(t, func(scope *core.Scope) core.Node {
		cx = scope
		return core.View{}
	})
	var available bool
	app.Post(func() { available = core.Untracked(cx, cx.Env().CanPlayAudio) })
	app.Settle()
	if !available {
		t.Fatal("the Android runtime did not report audio")
	}
	return app, cx
}

func awaitAudio[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(6 * time.Second):
		t.Fatal("Android audio did not report a result")
		var zero T
		return zero
	}
}

func TestNativeAndroidSound(t *testing.T) {
	app, cx := startAudioTest(t)
	type loadResult struct {
		sound *core.Sound
		err   error
	}
	result := make(chan loadResult, 1)
	load := func(source core.AudioSource) loadResult {
		app.Post(func() {
			cx.LoadSound(source, func(sound *core.Sound, err error) { result <- loadResult{sound, err} })
		})
		app.Settle()
		return awaitAudio(t, result)
	}
	for _, format := range nativeAudioSources(t) {
		got := load(format.source)
		if got.err != nil {
			t.Fatalf("%s: %v", format.name, got.err)
		}
		if got.sound.Duration() < time.Second {
			t.Fatalf("%s: short duration %v", format.name, got.sound.Duration())
		}
		app.Post(func() {
			muted := got.sound.Play(core.SoundVoice{Muted: true})
			muted.SetVolume(0.4)
			muted.Stop()
			got.sound.Play(core.SoundVoice{Volume: 0.2})
			got.sound.Play(core.SoundVoice{Volume: 0.3, Rate: 1.5})
			got.sound.Play(core.SoundVoice{Volume: 0.3, Pan: -1})
			got.sound.Play(core.SoundVoice{Volume: 0.3, Rate: 0.8, Pan: 0.6})
		})
		app.Settle()
		app.Post(func() { got.sound.Stop(); got.sound.Close() })
		app.Settle()
	}
	large := filepath.Join(t.TempDir(), "oversized.wav")
	if err := os.WriteFile(large, make([]byte, driver.MaxSoundBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		source core.AudioSource
		err    core.AudioError
	}{
		{core.AudioSource{Data: []byte("not media")}, core.AudioDecode},
		{core.AudioSource{Data: silentAudioWAV(6 * time.Second)}, core.AudioQuotaExceeded},
		{core.AudioSource{URI: large}, core.AudioQuotaExceeded},
	} {
		if got := load(tc.source); !errors.Is(got.err, tc.err) {
			t.Fatalf("sound error = %v, want %v", got.err, tc.err)
		}
	}
}

func TestNativeAndroidPlaybackControls(t *testing.T) {
	app, cx := startAudioTest(t)
	loaded := make(chan *core.Sound, 1)
	failed := make(chan error, 1)
	step := func(fn func()) { app.Post(fn); app.Settle() }
	step(func() {
		cx.LoadSound(core.AudioSource{Data: silentAudioWAV(1200 * time.Millisecond)}, func(sound *core.Sound, err error) {
			if err != nil {
				failed <- err
				return
			}
			loaded <- sound
		})
	})
	var sound *core.Sound
	select {
	case sound = <-loaded:
	case err := <-failed:
		t.Fatal(err)
	case <-time.After(6 * time.Second):
		t.Fatal("the sound did not load")
	}
	ended := func(playback *core.Playback) bool {
		var value bool
		step(func() { value = playback.Ended() })
		return value
	}
	var voice *core.Playback
	// No later Play should be needed to deliver natural completion. Changing
	// the initial slow rate must also move the original expiry earlier.
	step(func() {
		voice = sound.Play(core.SoundVoice{Muted: true, Pan: -1, Rate: .5})
		voice.SetVolume(.2)
		voice.SetRate(2)
	})
	app.WaitUntil(1500*time.Millisecond, func() bool { return ended(voice) })
	// Slowing an active stream must retain the source position and move its
	// expiry later than the 600 ms it would take at its initial rate.
	step(func() { voice = sound.Play(core.SoundVoice{Muted: true, Rate: 2}) })
	time.Sleep(100 * time.Millisecond)
	step(func() { voice.SetRate(.5) })
	time.Sleep(750 * time.Millisecond)
	if ended(voice) {
		t.Fatal("the slowed playback ended at its original expiry")
	}
	app.WaitUntil(3*time.Second, func() bool { return ended(voice) })
	// The 33rd live stream retires the oldest independently of sound.Stop.
	voices := make([]*core.Playback, driver.MaxSoundVoices+1)
	step(func() {
		for i := range voices {
			voices[i] = sound.Play(core.SoundVoice{Muted: true, Rate: .5})
		}
	})
	app.WaitUntil(time.Second, func() bool { return ended(voices[0]) })
	if ended(voices[1]) {
		t.Fatal("evicting the oldest stream also ended its successor")
	}
	step(voices[1].Stop)
	if !ended(voices[1]) || ended(voices[2]) {
		t.Fatal("stopping one handle changed a sibling playback")
	}
	// A lowered engine limit stops the oldest playbacks until the next one fits, in the batch that plays it.
	step(sound.Stop)
	var limitErr error
	step(func() { limitErr = cx.SetSoundLimits(core.SoundLimits{MaxPlaybacks: 2}) })
	if limitErr != nil {
		t.Fatal(limitErr)
	}
	limited := make([]*core.Playback, 3)
	step(func() {
		for i := range limited {
			limited[i] = sound.Play(core.SoundVoice{Muted: true, Rate: .5})
		}
	})
	app.WaitUntil(time.Second, func() bool { return ended(limited[0]) })
	if ended(limited[1]) || ended(limited[2]) {
		t.Fatal("a playback limit of 2 ended more than the oldest of 3 playbacks")
	}
	step(sound.Close)
}

func TestNativeAndroidPlayer(t *testing.T) {
	app, cx := startAudioTest(t)
	for _, format := range nativeAudioSources(t) {
		ends, played := make(chan struct{}, 4), make(chan error, 4)
		var p *core.AudioPlayer
		app.Post(func() {
			p = cx.NewAudioPlayer(format.source, core.AudioPlayerOptions{OnEnd: func() { ends <- struct{}{} }})
			p.SetRate(2)
			p.SetVolume(0.25)
			p.SetMuted(true)
			p.Play(func(err error) { played <- err })
		})
		app.Settle()
		if err := awaitAudio(t, played); err != nil {
			t.Fatalf("%s: %v", format.name, err)
		}
		awaitAudio(t, ends)
		app.Post(p.Close)
		app.Settle()
	}

	progress := make(chan time.Duration, 256)
	ready := make(chan time.Duration, 1)
	ended, played := make(chan struct{}, 4), make(chan error, 4)
	var p *core.AudioPlayer
	step := func(fn func()) { app.Post(fn); app.Settle() }
	step(func() {
		p = cx.NewAudioPlayer(core.AudioSource{Data: silentAudioWAV(1200 * time.Millisecond)}, core.AudioPlayerOptions{
			AudioSession:     core.AudioSessionPlayback,
			ProgressInterval: 25 * time.Millisecond,
			OnLoad:           func(d time.Duration) { ready <- d },
			OnProgress:       func(pos, _ time.Duration) { progress <- pos },
			OnEnd:            func() { ended <- struct{}{} },
		})
		p.Seek(300 * time.Millisecond)
		p.Preload()
	})
	if duration := awaitAudio(t, ready); duration != 1200*time.Millisecond {
		t.Fatalf("WAV duration %v, want 1.2s", duration)
	}
	if pos := awaitAudio(t, progress); pos != 300*time.Millisecond {
		t.Fatalf("preload seek position %v, want 300ms", pos)
	}
	step(func() {
		p.SetVolume(0.2)
		p.SetRate(1.5)
		p.SetLoop(true)
		p.Play(func(err error) { played <- err })
	})
	if err := awaitAudio(t, played); err != nil {
		t.Fatal(err)
	}
	previous := awaitAudio(t, progress)
	deadline := time.After(3 * time.Second)
looped:
	for {
		select {
		case pos := <-progress:
			if pos < previous {
				break looped
			}
			previous = pos
		case <-deadline:
			t.Fatal("the player did not report a loop")
		}
	}
	select {
	case <-ended:
		t.Fatal("an actively looping player reported an end")
	default:
	}
	step(p.Pause)
	app.WaitUntil(3*time.Second, func() bool {
		var state core.AudioPlayerState
		step(func() { state = p.State() })
		return state == core.AudioPlayerPaused
	})
	for len(progress) > 0 {
		<-progress
	}
	step(func() {
		p.Seek(100 * time.Millisecond)
		p.Seek(600 * time.Millisecond)
	})
	if pos := awaitAudio(t, progress); pos != 600*time.Millisecond {
		t.Fatalf("paused seek position %v, want the latest seek to 600ms", pos)
	}
	step(func() { p.SetLoop(false); p.Play(func(err error) { played <- err }) })
	if err := awaitAudio(t, played); err != nil {
		t.Fatal(err)
	}
	awaitAudio(t, ended)
	step(p.Close)

	bad := make(chan error, 1)
	step(func() {
		p = cx.NewAudioPlayer(core.AudioSource{Data: []byte("not media")}, core.AudioPlayerOptions{})
		p.Play(func(err error) { bad <- err })
	})
	if err := awaitAudio(t, bad); !errors.Is(err, core.AudioDecode) {
		t.Fatalf("unsupported player format: %v", err)
	}
	step(p.Close)
}
