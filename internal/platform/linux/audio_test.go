//go:build linux && !android

package linux

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// The encoded fixtures contain 1.2 seconds of mono silence at 44.1 kHz.
//
//go:embed testdata/audio/silence.m4a
var silentAAC []byte

//go:embed testdata/audio/silence.mp3
var silentMP3 []byte

func audioTestWAV(duration time.Duration) []byte {
	const rate = 8000
	samples := int(duration.Seconds() * rate)
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

func audioTestSources(t *testing.T) map[string]core.AudioSource {
	t.Helper()
	mp3 := filepath.Join(t.TempDir(), "silence.mp3")
	if err := os.WriteFile(mp3, silentMP3, 0o600); err != nil {
		t.Fatal(err)
	}
	return map[string]core.AudioSource{
		"WAV bytes":    {Data: audioTestWAV(1200 * time.Millisecond)},
		"AAC data URL": {URI: "data:audio/mp4;base64," + base64.StdEncoding.EncodeToString(silentAAC)},
		"MP3 file":     {URI: mp3},
	}
}

func audioTestWindow(t *testing.T) (*window, *core.Scope) {
	t.Helper()
	scope := make(chan *core.Scope, 1)
	w := startTestWindow(t, func(cx *core.Scope) core.Node {
		scope <- cx
		return core.View{}
	}, Options{Title: t.Name(), Width: 200, Height: 100})
	cx := awaitAudio(t, scope)
	var available bool
	w.onLoop(t, func() {
		available = w.env.Audio
		if w.audio != nil {
			t.Error("the idle window opened audio")
		}
	})
	if !available {
		t.Skip("GStreamer audio is unavailable")
	}
	return w, cx
}

func awaitAudio[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(7 * time.Second):
		t.Fatal("audio did not report a result")
		var zero T
		return zero
	}
}

func TestLinuxAudioSounds(t *testing.T) {
	w, cx := audioTestWindow(t)
	type result struct {
		sound *core.Sound
		err   error
	}
	loaded := make(chan result, 1)
	load := func(source core.AudioSource) result {
		cx.Post(func() { cx.LoadSound(source, func(s *core.Sound, err error) { loaded <- result{s, err} }) })
		return awaitAudio(t, loaded)
	}
	for name, source := range audioTestSources(t) {
		got := load(source)
		if got.err != nil || got.sound.Duration() < time.Second {
			t.Fatalf("%s: sound=%v error=%v", name, got.sound, got.err)
		}
		w.onLoop(t, func() {
			if w.audio.mixer != nil {
				t.Error("loading a sound opened the output")
			}
		})
		cx.Post(func() {
			got.sound.Play(core.SoundVoice{Volume: 0.3})
			got.sound.Play(core.SoundVoice{Volume: 0.2, Rate: 1.5, Pan: -0.5})
		})
		w.engine.Sync()
		deadline := time.Now().Add(2 * time.Second)
		for {
			count := 0
			w.onLoop(t, func() {
				if m := w.audio.mixer; m != nil {
					count = len(m.voices)
				}
			})
			if count == 2 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("two concurrent playbacks did not reach the mixer")
			}
			time.Sleep(10 * time.Millisecond)
		}
		cx.Post(func() { got.sound.Stop(); got.sound.Close() })
		w.engine.Sync()
		w.onLoop(t, func() {
			if len(w.audio.sounds) != 0 || w.audio.mixer == nil || len(w.audio.mixer.voices) != 0 {
				t.Error("stopping and closing the sound left audio behind")
			}
		})
		// Exercise a fresh output for each format.
		w.onLoop(t, func() {
			if m := w.audio.mixer; m != nil {
				m.close()
				w.audio.mixer = nil
			}
		})
	}
	for _, tt := range []struct {
		data []byte
		want core.AudioError
	}{
		{[]byte("not audio"), core.AudioDecode},
		{audioTestWAV(6 * time.Second), core.AudioQuotaExceeded},
	} {
		if got := load(core.AudioSource{Data: tt.data}); !errors.Is(got.err, tt.want) {
			t.Fatalf("audio error %v, want %v", got.err, tt.want)
		}
	}
	large := filepath.Join(t.TempDir(), "oversized.wav")
	f, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	err = f.Truncate(driver.MaxSoundBytes + 1)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if got := load(core.AudioSource{URI: large}); !errors.Is(got.err, core.AudioQuotaExceeded) {
		t.Fatalf("oversized sound error %v, want %v", got.err, core.AudioQuotaExceeded)
	}
	set := make(chan error, 1)
	cx.Post(func() { set <- cx.SetSoundLimits(core.SoundLimits{MaxDuration: 10 * time.Second}) })
	if err := awaitAudio(t, set); err != nil {
		t.Fatal(err)
	}
	if got := load(core.AudioSource{Data: audioTestWAV(6 * time.Second)}); got.err != nil || got.sound.Duration() < 6*time.Second-time.Millisecond {
		t.Fatalf("sound within a raised duration limit: sound=%v error=%v", got.sound, got.err)
	}
}

func TestLinuxAudioMixerOutput(t *testing.T) {
	w, _ := audioTestWindow(t)
	var mixer *soundMixer
	var sink uintptr
	w.onLoop(t, func() {
		mixer = soundMixerTo("appsink name=output max-buffers=4 drop=true")
		if mixer != nil {
			sink = gstBinGetByName(mixer.pipeline, cstr("output"))
		}
	})
	if mixer == nil || sink == 0 {
		t.Fatal("the native mixer did not open its test output")
	}
	defer w.onLoop(t, func() { gstObjectUnref(sink); mixer.close() })

	pcm := make([]byte, soundRate*soundFrameBytes/10)
	for at := 0; at < len(pcm); at += 4 {
		binary.LittleEndian.PutUint32(pcm[at:], math.Float32bits(0.25))
	}
	hear := func(left, right float32) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			w.onLoop(t, func() {
				if message := mixer.checkOutput(); message != "" {
					t.Error(message)
				}
				mixer.sweep(time.Now(), 0)
				mixer.startPending()
				mixer.feed(time.Now())
			})
			sample := gstAppSinkTryPullSample(sink, uint64(20*time.Millisecond))
			if sample == 0 {
				continue
			}
			b := gstSampleGetBuffer(sample)
			var data [8]byte
			copied := gstBufferExtract(b, 0, unsafe.Pointer(&data[0]), 8)
			gstMiniObjectUnref(sample)
			l := math.Float32frombits(binary.LittleEndian.Uint32(data[:4]))
			r := math.Float32frombits(binary.LittleEndian.Uint32(data[4:]))
			if copied == 8 && math.Abs(float64(l-left)) < 0.02 && math.Abs(float64(r-right)) < 0.02 {
				return
			}
		}
		t.Fatalf("mixer did not output the expected samples (%f, %f)", left, right)
	}
	for range 3 {
		w.onLoop(t, func() {
			mixer.play(soundPlayback{id: 1, pcm: pcm, volume: 1, rate: 1})
			mixer.play(soundPlayback{id: 2, pcm: pcm, volume: 1, rate: 1})
		})
		hear(0.5, 0.5)
		hear(0, 0)
	}
	// Full pans silence the far channel and leave the near one at its level.
	w.onLoop(t, func() { mixer.play(soundPlayback{id: 1, pcm: pcm, volume: 1, rate: 1, pan: 1}) })
	hear(0, 0.25)
	hear(0, 0)
	w.onLoop(t, func() { mixer.play(soundPlayback{id: 1, pcm: pcm, volume: 1, rate: 1, pan: -1}) })
	hear(0.25, 0)
	hear(0, 0)

	// A looping 100 ms voice still sounds after a second, past what its first fill queued, until its sound stops.
	w.onLoop(t, func() { mixer.play(soundPlayback{id: 3, playback: 200, pcm: pcm, volume: 1, rate: 1, loop: true}) })
	hear(0.25, 0.25)
	for until := time.Now().Add(time.Second); time.Now().Before(until); time.Sleep(20 * time.Millisecond) {
		w.onLoop(t, func() { mixer.feed(time.Now()) })
	}
	hear(0.25, 0.25)
	// A rate change reconnects the looping voice mid-copy, and it keeps repeating.
	w.onLoop(t, func() {
		mixer.control(3, &driver.AudioRequest{Command: driver.AudioPlaybackSet, Playback: 200, Volume: 1, Rate: 2})
		if len(mixer.voices) != 1 || !mixer.voices[0].play.loop || mixer.voices[0].play.rate != 2 {
			t.Error("a rate change dropped the looping voice")
		}
	})
	for until := time.Now().Add(time.Second); time.Now().Before(until); time.Sleep(20 * time.Millisecond) {
		w.onLoop(t, func() { mixer.feed(time.Now()) })
	}
	hear(0.25, 0.25)
	w.onLoop(t, func() {
		mixer.sweep(time.Now(), 3)
		if len(mixer.voices) != 0 {
			t.Error("stopping the sound kept its looping voice")
		}
	})
	hear(0, 0)

	// Distinct halves reveal a rate change that restarts the source.
	pcm = make([]byte, 4*soundRate*soundFrameBytes)
	for at := 0; at < len(pcm); at += 4 {
		level := float32(0.125)
		if at >= len(pcm)/2 {
			level = 0.25
		}
		binary.LittleEndian.PutUint32(pcm[at:], math.Float32bits(level))
	}
	var ended []uint32
	w.onLoop(t, func() {
		mixer.ended = func(_, playback uint32) { ended = append(ended, playback) }
		mixer.play(soundPlayback{id: 3, playback: 201, pcm: pcm, volume: 1, rate: 1})
	})
	hear(0.125, 0.125)
	w.onLoop(t, func() {
		mixer.play(soundPlayback{id: 3, playback: 202, pcm: pcm, rate: 1, startAt: time.Now().Add(time.Minute)})
		mixer.control(3, &driver.AudioRequest{Command: driver.AudioPlaybackStop, Playback: 202})
		if !slices.Contains(ended, uint32(202)) || !slices.ContainsFunc(mixer.voices, func(voice soundVoice) bool {
			return voice.play.id == 3 && voice.play.playback == 201
		}) {
			t.Errorf("stopping a pending playback affected its sibling: ended=%v voices=%d", ended, len(mixer.voices))
		}
		mixer.control(3, &driver.AudioRequest{Command: driver.AudioPlaybackSet, Playback: 201, Rate: 1})
	})
	hear(0, 0)
	w.onLoop(t, func() {
		mixer.control(3, &driver.AudioRequest{Command: driver.AudioPlaybackSet, Playback: 201, Volume: 0.5, Rate: 1})
	})
	hear(0.125, 0.125)
	w.onLoop(t, func() {
		mixer.control(3, &driver.AudioRequest{Command: driver.AudioPlaybackSet, Playback: 201, Volume: 0.5, Rate: 2})
	})
	deadline := time.Now().Add(1800 * time.Millisecond)
	finished, restartedFrames := false, 0
	for !finished && time.Now().Before(deadline) {
		w.onLoop(t, func() {
			if message := mixer.checkOutput(); message != "" {
				t.Error(message)
			}
			mixer.sweep(time.Now(), 0)
			finished = slices.Contains(ended, uint32(201))
		})
		sample := gstAppSinkTryPullSample(sink, uint64(20*time.Millisecond))
		if sample == 0 {
			continue
		}
		buffer := gstSampleGetBuffer(sample)
		data := make([]byte, gstBufferGetSize(buffer))
		if len(data) > 0 {
			gstBufferExtract(buffer, 0, unsafe.Pointer(&data[0]), uintptr(len(data)))
		}
		gstMiniObjectUnref(sample)
		for frame := 0; frame+soundFrameBytes <= len(data); frame += soundFrameBytes {
			level := math.Float32frombits(binary.LittleEndian.Uint32(data[frame:]))
			if math.Abs(float64(level-0.0625)) < 0.005 {
				restartedFrames++
			}
		}
	}
	if !finished || restartedFrames > soundRate/20 {
		t.Fatalf("rate change completion=%v, replayed prefix frames=%d", finished, restartedFrames)
	}
	w.onLoop(t, func() {
		for playback := uint32(300); playback < 300+driver.MaxSoundVoices+1; playback++ {
			mixer.play(soundPlayback{id: 4, playback: playback, pcm: pcm, rate: 1})
		}
		if !slices.Contains(ended, uint32(300)) || len(mixer.voices) != driver.MaxSoundVoices {
			t.Error("the oldest playback was not retired at the native limit")
		}
		mixer.sweep(time.Now(), 4)
	})

	// A raised limit keeps more voices, restricted playbacks stay within the default cap among themselves, and a lowered limit stops the oldest.
	const raised = driver.MaxSoundVoices + 8
	w.onLoop(t, func() {
		defer mixer.sweep(time.Now(), 3)
		for range raised {
			mixer.play(soundPlayback{id: 3, pcm: pcm, volume: 0.01, rate: 1, limit: raised})
		}
		if len(mixer.voices) != raised {
			t.Errorf("voices at a raised limit = %d, want %d", len(mixer.voices), raised)
			return
		}
		for range driver.MaxSoundVoices + 1 {
			mixer.play(soundPlayback{id: 3, pcm: pcm, volume: 0.01, rate: 1, limit: raised * 2, restricted: true})
		}
		restricted := 0
		for _, v := range mixer.voices {
			if v.play.restricted {
				restricted++
			}
		}
		if restricted != driver.MaxSoundVoices || len(mixer.voices) != raised+driver.MaxSoundVoices {
			t.Errorf("restricted voices = %d of %d, want %d of %d", restricted, len(mixer.voices), driver.MaxSoundVoices, raised+driver.MaxSoundVoices)
			return
		}
		mixer.play(soundPlayback{id: 3, pcm: pcm, volume: 0.01, rate: 1, limit: 4})
		if len(mixer.voices) != 4 {
			t.Errorf("voices after lowering the limit = %d, want 4", len(mixer.voices))
			return
		}
	})
}

// TestLinuxAudioMixerSchedulesStarts measures two voices scheduled 500 ms apart in the mixed output's timestamps.
func TestLinuxAudioMixerSchedulesStarts(t *testing.T) {
	w, _ := audioTestWindow(t)
	var mixer *soundMixer
	var sink uintptr
	w.onLoop(t, func() {
		mixer = soundMixerTo("appsink name=output max-buffers=0")
		if mixer != nil {
			sink = gstBinGetByName(mixer.pipeline, cstr("output"))
		}
	})
	if mixer == nil || sink == 0 {
		t.Fatal("the native mixer did not open its test output")
	}
	defer w.onLoop(t, func() { gstObjectUnref(sink); mixer.close() })
	pcm := make([]byte, soundRate*soundFrameBytes/20)
	for at := 0; at < len(pcm); at += 4 {
		binary.LittleEndian.PutUint32(pcm[at:], math.Float32bits(0.25))
	}
	scheduled := false
	var onsets []time.Duration
	silent := true
	deadline := time.Now().Add(5 * time.Second)
	for len(onsets) < 2 && time.Now().Before(deadline) {
		w.onLoop(t, func() {
			if message := mixer.checkOutput(); message != "" {
				t.Error(message)
			}
			if !scheduled && mixer.playing() {
				scheduled = true
				first := time.Now().Add(300 * time.Millisecond)
				mixer.play(soundPlayback{id: 1, pcm: pcm, volume: 1, rate: 1, startAt: first})
				mixer.play(soundPlayback{id: 2, pcm: pcm, volume: 1, rate: 1, startAt: first.Add(500 * time.Millisecond)})
			}
		})
		sample := gstAppSinkTryPullSample(sink, uint64(20*time.Millisecond))
		if sample == 0 {
			continue
		}
		b := gstSampleGetBuffer(sample)
		pts := (*gstSoundBuffer)(cPointer(b)).pts
		data := make([]byte, gstBufferGetSize(b))
		if len(data) > 0 {
			gstBufferExtract(b, 0, unsafe.Pointer(&data[0]), uintptr(len(data)))
		}
		gstMiniObjectUnref(sample)
		for frame := 0; frame+soundFrameBytes <= len(data); frame += soundFrameBytes {
			loud := math.Float32frombits(binary.LittleEndian.Uint32(data[frame:])) > 0.1
			if loud && silent && scheduled {
				onsets = append(onsets, time.Duration(pts)+time.Duration(frame/soundFrameBytes)*time.Second/soundRate)
			}
			silent = !loud
		}
	}
	if len(onsets) < 2 {
		t.Fatalf("heard %d scheduled starts, want 2", len(onsets))
	}
	if gap := onsets[1] - onsets[0]; gap < 499*time.Millisecond || gap > 501*time.Millisecond {
		t.Fatalf("scheduled starts %v apart in the output, want 500ms within 1ms", gap)
	}
	var latency time.Duration
	w.onLoop(t, func() { latency = mixer.latency() })
	if latency <= 0 {
		t.Fatalf("live mixer latency %v, want a reported latency", latency)
	}
}

func TestLinuxAudioOutputLatencyAnswers(t *testing.T) {
	_, cx := audioTestWindow(t)
	type reading struct {
		latency time.Duration
		known   bool
	}
	got := make(chan reading, 1)
	cx.Post(func() {
		cx.AudioOutputLatency(func(latency time.Duration, known bool) { got <- reading{latency, known} })
	})
	r := awaitAudio(t, got)
	if r.known != (r.latency > 0) {
		t.Fatalf("latency %v known %v", r.latency, r.known)
	}
}

func TestLinuxAudioPlayer(t *testing.T) {
	w, cx := audioTestWindow(t)
	for name, source := range audioTestSources(t) {
		played, ended := make(chan error, 1), make(chan struct{}, 1)
		cx.Post(func() {
			var player *core.AudioPlayer
			player = cx.NewAudioPlayer(source, core.AudioPlayerOptions{OnEnd: func() {
				player.Close()
				ended <- struct{}{}
			}})
			player.Play(func(err error) { played <- err })
		})
		if err := awaitAudio(t, played); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		awaitAudio(t, ended)
	}
	load, progress := make(chan time.Duration, 1), make(chan time.Duration, 256)
	played, ended := make(chan error, 2), make(chan struct{}, 1)
	var p *core.AudioPlayer
	cx.Post(func() {
		p = cx.NewAudioPlayer(core.AudioSource{Data: audioTestWAV(1200 * time.Millisecond)}, core.AudioPlayerOptions{
			OnLoad: func(d time.Duration) { load <- d }, ProgressInterval: 50 * time.Millisecond,
			OnProgress: func(pos, _ time.Duration) { progress <- pos }, OnEnd: func() { ended <- struct{}{} },
		})
		p.Seek(300 * time.Millisecond)
		p.Preload()
	})
	if d := awaitAudio(t, load); d != 1200*time.Millisecond {
		t.Fatalf("duration %v, want 1.2s", d)
	}
	if pos := awaitAudio(t, progress); pos < 280*time.Millisecond || pos > 320*time.Millisecond {
		t.Fatalf("preloaded position %v, want 300 ms", pos)
	}
	cx.Post(func() {
		p.SetLoop(true)
		p.SetVolume(0.4)
		p.Play(func(err error) { played <- err })
	})
	if err := awaitAudio(t, played); err != nil {
		t.Fatal(err)
	}
	cx.Post(func() { p.SetMuted(true) })
	w.engine.Sync()
	var player *linuxPlayer
	var before time.Duration
	w.onLoop(t, func() {
		for _, pl := range w.audio.players {
			player = pl
		}
		var volume float64
		gObjectGet(player.stream, uintptr(unsafe.Pointer(cstr("volume"))), uintptr(unsafe.Pointer(&volume)), 0)
		before = player.timestamp()
		if volume != 0 || !player.streamPlaying {
			t.Error("muting the player did not preserve silent playback")
		}
	})
	muteDeadline := time.Now().Add(2 * time.Second)
	for {
		var advanced bool
		w.onLoop(t, func() {
			advanced = player.timestamp() != before
			if !player.streamPlaying {
				t.Error("the muted player paused")
			}
		})
		if advanced {
			break
		}
		if time.Now().After(muteDeadline) {
			t.Fatal("the muted player's timeline stopped")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cx.Post(func() { p.SetMuted(false) })
	w.engine.Sync()
	w.onLoop(t, func() {
		var volume float64
		gObjectGet(player.stream, uintptr(unsafe.Pointer(cstr("volume"))), uintptr(unsafe.Pointer(&volume)), 0)
		if math.Abs(volume-0.4) > 1e-6 || !player.streamPlaying {
			t.Error("unmuting did not restore the playing volume")
		}
	})
	last := awaitAudio(t, progress)
	deadline := time.After(4 * time.Second)
looped:
	for {
		select {
		case pos := <-progress:
			if pos < last {
				break looped
			}
			last = pos
		case <-ended:
			t.Fatal("a looping player reported completion")
		case <-deadline:
			t.Fatal("the player did not report its next loop")
		}
	}
	cx.Post(p.Pause)
	paused := make(chan bool, 1)
	deadline = time.After(3 * time.Second)
	for {
		cx.Post(func() { paused <- p.State() == core.AudioPlayerPaused })
		if awaitAudio(t, paused) {
			break
		}
		select {
		case <-deadline:
			t.Fatal("the player did not pause")
		case <-time.After(10 * time.Millisecond):
		}
	}
	for len(progress) != 0 {
		<-progress
	}
	cx.Post(func() { p.Seek(100 * time.Millisecond); p.Seek(600 * time.Millisecond) })
	if pos := awaitAudio(t, progress); pos < 580*time.Millisecond || pos > 620*time.Millisecond {
		t.Fatalf("paused position %v, want the latest seek to 600 ms", pos)
	}
	cx.Post(func() { p.SetLoop(false); p.Play(func(err error) { played <- err }) })
	if err := awaitAudio(t, played); err != nil {
		t.Fatal(err)
	}
	awaitAudio(t, ended)
	cx.Post(p.Close)

	bad := make(chan error, 1)
	cx.Post(func() {
		player := cx.NewAudioPlayer(core.AudioSource{Data: []byte("not audio")}, core.AudioPlayerOptions{})
		player.Play(func(err error) { bad <- err })
	})
	if err := awaitAudio(t, bad); !errors.Is(err, core.AudioDecode) {
		t.Fatalf("unsupported player format: %v", err)
	}
}

// A player closed while its seek runs keeps its stream until the seek ends: freed at once, GStreamer's streaming thread may still hold the pipeline's locks and abort the process.
func TestLinuxAudioPlayerClosesWhileSeeking(t *testing.T) {
	w, cx := audioTestWindow(t)
	for range 20 {
		played := make(chan error, 1)
		var p *core.AudioPlayer
		cx.Post(func() {
			p = cx.NewAudioPlayer(core.AudioSource{Data: audioTestWAV(1200 * time.Millisecond)}, core.AudioPlayerOptions{})
			p.Play(func(err error) { played <- err })
		})
		if err := awaitAudio(t, played); err != nil {
			t.Fatal(err)
		}
		cx.Post(func() {
			p.Seek(600 * time.Millisecond)
			p.Close()
		})
	}
	// The retired streams go once their seeks end.
	deadline := time.Now().Add(5 * time.Second)
	for {
		var left int
		w.onLoop(t, func() { left = len(w.audioRoot().audio.retired) })
		if left == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d closed streams still held", left)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestLinuxAudioPlayerLoopsFromItsStart(t *testing.T) {
	_, cx := audioTestWindow(t)
	const loopStart = 600 * time.Millisecond
	played, ended := make(chan error, 1), make(chan struct{}, 1)
	progress := make(chan time.Duration, 256)
	var p *core.AudioPlayer
	cx.Post(func() {
		p = cx.NewAudioPlayer(core.AudioSource{Data: audioTestWAV(1200 * time.Millisecond)}, core.AudioPlayerOptions{
			LoopStart: loopStart, ProgressInterval: 50 * time.Millisecond,
			OnProgress: func(pos, _ time.Duration) { progress <- pos }, OnEnd: func() { ended <- struct{}{} },
		})
		p.SetLoop(true)
		p.Play(func(err error) { played <- err })
	})
	defer cx.Post(func() { p.Close() })
	if err := awaitAudio(t, played); err != nil {
		t.Fatal(err)
	}
	var last time.Duration
	intro, laps := false, 0
	deadline := time.After(6 * time.Second)
	for laps < 2 {
		select {
		case pos := <-progress:
			intro = intro || pos > 0 && pos < loopStart-50*time.Millisecond
			if pos < last {
				if !intro {
					t.Fatal("the player looped before playing its intro")
				}
				if pos < loopStart-20*time.Millisecond {
					t.Fatalf("a repeat started at %v, before the loop start %v", pos, loopStart)
				}
				laps++
			}
			last = pos
		case <-ended:
			t.Fatal("a player looping from its start reported completion")
		case <-deadline:
			t.Fatalf("the player repeated %d times before the deadline", laps)
		}
	}
}

// audioTestPlayer is the window's only native player.
func audioTestPlayer(t *testing.T, w *window) (p *linuxPlayer) {
	t.Helper()
	w.onLoop(t, func() {
		if w.audio != nil {
			for _, player := range w.audio.players {
				p = player
			}
		}
	})
	if p == nil {
		t.Fatal("the window has no native player")
	}
	return p
}

// awaitLoopsFrom reads progress until the player repeated twice, each repeat starting at loopStart.
func awaitLoopsFrom(t *testing.T, progress <-chan time.Duration, ended <-chan struct{}, loopStart time.Duration) {
	t.Helper()
	var last time.Duration
	laps := 0
	deadline := time.After(8 * time.Second)
	for laps < 2 {
		select {
		case pos := <-progress:
			if pos < last {
				if pos < loopStart-20*time.Millisecond {
					t.Fatalf("a repeat started at %v, before the loop start %v", pos, loopStart)
				}
				laps++
			}
			last = pos
		case <-ended:
			t.Fatal("a looping player reported completion")
		case <-deadline:
			t.Fatalf("the player repeated %d times before the deadline", laps)
		}
	}
}

// TestLinuxAudioPlayerStreamsLongTrack plays a track longer than a looping player decodes whole: its decoder streams it into the mixer and seeks back to the loop start at its end.
func TestLinuxAudioPlayerStreamsLongTrack(t *testing.T) {
	w, cx := audioTestWindow(t)
	loopStart := driver.MaxGaplessLoopDuration
	played, ended := make(chan error, 1), make(chan struct{}, 1)
	progress := make(chan time.Duration, 256)
	var player *core.AudioPlayer
	cx.Post(func() {
		player = cx.NewAudioPlayer(core.AudioSource{Data: audioTestWAV(loopStart + 1200*time.Millisecond)}, core.AudioPlayerOptions{
			LoopStart: loopStart, ProgressInterval: 50 * time.Millisecond,
			OnProgress: func(pos, _ time.Duration) { progress <- pos }, OnEnd: func() { ended <- struct{}{} },
		})
		player.Seek(loopStart + 900*time.Millisecond)
		player.SetLoop(true)
		player.Play(func(err error) { played <- err })
	})
	defer cx.Post(func() { player.Close() })
	if err := awaitAudio(t, played); err != nil {
		t.Fatal(err)
	}
	p := audioTestPlayer(t, w)
	awaitLoopsFrom(t, progress, ended, loopStart)
	w.onLoop(t, func() {
		if _, streamed := p.track.(*trackDecoder); !streamed || p.voice == nil || p.stream != 0 {
			t.Error("a track too long to decode whole did not stream into the mixer")
		}
	})
}

// TestLinuxAudioPlayerLoopsBySeekingWithoutTrack loops a player whose track GStreamer could not read for the mixer: GtkMediaFile plays it and seeks back to the loop start at its end.
func TestLinuxAudioPlayerLoopsBySeekingWithoutTrack(t *testing.T) {
	w, _ := audioTestWindow(t)
	const loopStart = 600 * time.Millisecond
	var p *linuxPlayer
	w.onLoop(t, func() {
		w.audio = &linuxAudio{root: w, sounds: map[uint32]*linuxSound{}, players: map[uint32]*linuxPlayer{}}
		p = &linuxPlayer{owner: w, audio: w.audio, id: 1, source: driver.ImageSource{Data: audioTestWAV(1200 * time.Millisecond)}, volume: 1,
			loop: true, loopStart: loopStart, interval: 20 * time.Millisecond, noTrack: true}
		w.audio.players[1] = p
		p.command(&driver.AudioRequest{Command: driver.AudioResume})
		w.audio.ensureTick()
	})
	defer w.onLoop(t, func() {
		delete(w.audio.players, 1)
		p.close()
	})
	var last time.Duration
	var laps uint32
	for deadline := time.Now().Add(8 * time.Second); laps < 2; time.Sleep(10 * time.Millisecond) {
		var position time.Duration
		var playing, ended bool
		w.onLoop(t, func() { position, laps, playing, ended = p.position, p.laps, p.playing, p.ended })
		if ended {
			t.Fatal("a looping player ended")
		}
		if playing && position < last && position < loopStart-20*time.Millisecond {
			t.Fatalf("a repeat started at %v, before the loop start %v", position, loopStart)
		}
		last = position
		if time.Now().After(deadline) {
			t.Fatalf("the player repeated %d times before the deadline", laps)
		}
	}
	w.onLoop(t, func() {
		if p.track != nil || p.voice != nil || p.stream == 0 {
			t.Error("a player without a track did not loop through GtkMediaFile")
		}
	})
}

// TestLinuxAudioPlayerOutlivesItsMixer closes the mixer under a looping player's decoded track, as an output error does when the system's audio device changes: the track plays on in a new mixer.
func TestLinuxAudioPlayerOutlivesItsMixer(t *testing.T) {
	w, cx := audioTestWindow(t)
	const loopStart = 600 * time.Millisecond
	played, ended, failed := make(chan error, 1), make(chan struct{}, 1), make(chan error, 1)
	progress := make(chan time.Duration, 256)
	var player *core.AudioPlayer
	cx.Post(func() {
		player = cx.NewAudioPlayer(core.AudioSource{Data: audioTestWAV(1200 * time.Millisecond)}, core.AudioPlayerOptions{
			LoopStart: loopStart, ProgressInterval: 50 * time.Millisecond,
			OnProgress: func(pos, _ time.Duration) { progress <- pos }, OnEnd: func() { ended <- struct{}{} },
			OnError: func(err error) { failed <- err },
		})
		player.SetLoop(true)
		player.Play(func(err error) { played <- err })
	})
	defer cx.Post(func() { player.Close() })
	if err := awaitAudio(t, played); err != nil {
		t.Fatal(err)
	}
	p := audioTestPlayer(t, w)
	// A voice the output closes within trackRestartAfter fails the player, so the mixer closes after it.
	var started time.Time
	for deadline := time.Now().Add(4 * time.Second); started.IsZero(); time.Sleep(10 * time.Millisecond) {
		w.onLoop(t, func() {
			if p.voice != nil {
				started = p.voice.started
			}
		})
		if time.Now().After(deadline) {
			t.Fatal("the looping player did not play its decoded track")
		}
	}
	time.Sleep(time.Until(started.Add(trackRestartAfter + 100*time.Millisecond)))
	var old *trackVoice
	w.onLoop(t, func() {
		old = p.voice
		if old == nil || w.audio.mixer == nil {
			t.Fatal("the looping player did not play its decoded track")
		}
		w.audio.mixer.close()
		w.audio.mixer = nil
	})
	// Laps counted before the mixer closed do not count.
	for len(progress) > 0 {
		<-progress
	}
	awaitLoopsFrom(t, progress, ended, loopStart)
	select {
	case err := <-failed:
		t.Fatalf("the player failed when its mixer closed: %v", err)
	default:
	}
	w.onLoop(t, func() {
		if p.voice == nil || p.voice == old || p.voice.gone || !p.playing {
			t.Error("the player did not play on in a new mixer")
		}
	})
}

// TestLinuxAudioPlayerHandsOverToTrack turns the loop on while GtkMediaFile loads and plays: the track decodes while the stream plays, and takes over at the stream's end.
func TestLinuxAudioPlayerHandsOverToTrack(t *testing.T) {
	w, cx := audioTestWindow(t)
	const loopStart = 600 * time.Millisecond
	played, ended := make(chan error, 1), make(chan struct{}, 1)
	progress := make(chan time.Duration, 256)
	var player *core.AudioPlayer
	cx.Post(func() {
		player = cx.NewAudioPlayer(core.AudioSource{Data: audioTestWAV(1200 * time.Millisecond)}, core.AudioPlayerOptions{
			LoopStart: loopStart, ProgressInterval: 50 * time.Millisecond,
			OnProgress: func(pos, _ time.Duration) { progress <- pos }, OnEnd: func() { ended <- struct{}{} },
		})
		player.Play(func(err error) { played <- err })
		player.SetLoop(true)
	})
	defer cx.Post(func() { player.Close() })
	if err := awaitAudio(t, played); err != nil {
		t.Fatal(err)
	}
	p := audioTestPlayer(t, w)
	var streamed bool
	w.onLoop(t, func() { streamed = p.stream != 0 })
	if !streamed {
		t.Fatal("the player did not start on GtkMediaFile")
	}
	for deadline := time.Now().Add(4 * time.Second); ; {
		var decoded bool
		w.onLoop(t, func() { decoded, streamed = p.track != nil, p.stream != 0 })
		if decoded {
			if !streamed {
				t.Fatal("the decoded track took over before the stream's end")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the looping player did not decode its track")
		}
		time.Sleep(10 * time.Millisecond)
	}
	awaitLoopsFrom(t, progress, ended, loopStart)
	w.onLoop(t, func() {
		if p.stream != 0 || p.voice == nil || p.duration != p.track.length() {
			t.Error("the decoded track did not take over the loop")
		}
	})
}

// levelTestWAV is 16-bit mono PCM at 48 kHz holding each level, from -1 to 1, for its duration in turn.
func levelTestWAV(stretches ...struct {
	d     time.Duration
	level float64
}) []byte {
	const rate = 48000
	var samples []int16
	for _, s := range stretches {
		for range int(s.d.Seconds() * rate) {
			samples = append(samples, int16(s.level*32767))
		}
	}
	data := audioTestWAV(0)
	binary.LittleEndian.PutUint32(data[24:], rate)
	binary.LittleEndian.PutUint32(data[28:], 2*rate)
	for _, s := range samples {
		data = binary.LittleEndian.AppendUint16(data, uint16(s))
	}
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	binary.LittleEndian.PutUint32(data[40:], uint32(2*len(samples)))
	return data
}

// TestLinuxAudioPlayerLoopsWithoutGap records a looping player's decoded track in the mixed output: a 300 ms intro at one level, then a 300 ms body at another that repeats from the loop start, with no silence at the repeats and no intro after the first pass.
func TestLinuxAudioPlayerLoopsWithoutGap(t *testing.T) {
	testLoopGap(t, nil, func(p *linuxPlayer) {
		if _, whole := p.track.(pcmTrack); !whole {
			t.Error("the looping player did not decode its track whole")
		}
	})
}

// TestLinuxAudioPlayerStreamsLoopWithoutGap records the same loop from a track over the limit a looping player decodes whole, which its decoder streams.
func TestLinuxAudioPlayerStreamsLoopWithoutGap(t *testing.T) {
	limit := trackDecodeLimit
	trackDecodeLimit = 400 * time.Millisecond
	t.Cleanup(func() { trackDecodeLimit = limit })
	testLoopGap(t, nil, func(p *linuxPlayer) {
		if _, streamed := p.track.(*trackDecoder); !streamed {
			t.Error("the looping player did not stream its track")
		}
	})
}

// testLoopGap records the loop's output; prepare, when set, readies the player on the GTK thread before it plays.
func testLoopGap(t *testing.T, prepare func(p *linuxPlayer, track []byte), check func(p *linuxPlayer)) {
	w, _ := audioTestWindow(t)
	const loopStart = 300 * time.Millisecond
	type stretch = struct {
		d     time.Duration
		level float64
	}
	track := levelTestWAV(stretch{loopStart, 0.5}, stretch{300 * time.Millisecond, 0.25})
	var p *linuxPlayer
	var sink uintptr
	w.onLoop(t, func() {
		mixer := soundMixerTo("appsink name=output max-buffers=0")
		if mixer == nil {
			return
		}
		sink = gstBinGetByName(mixer.pipeline, cstr("output"))
		w.audio = &linuxAudio{root: w, sounds: map[uint32]*linuxSound{}, players: map[uint32]*linuxPlayer{}, mixer: mixer, lastPlay: time.Now()}
		p = &linuxPlayer{owner: w, audio: w.audio, id: 1, source: driver.ImageSource{Data: track}, volume: 1,
			loop: true, loopStart: loopStart, interval: 20 * time.Millisecond}
		w.audio.players[1] = p
		if prepare != nil {
			prepare(p, track)
		}
		p.command(&driver.AudioRequest{Command: driver.AudioResume})
		w.audio.ensureTick()
	})
	if p == nil || sink == 0 {
		t.Fatal("the native mixer did not open its test output")
	}
	defer w.onLoop(t, func() {
		delete(w.audio.players, 1)
		p.close()
		gstObjectUnref(sink)
		if w.audio.mixer != nil {
			w.audio.mixer.close()
			w.audio.mixer = nil
		}
	})

	// Each run is a stretch of contiguous output buffers, from the first sound on.
	var runs [][]float32
	var next uint64
	var recorded int
	data := make([]byte, 0, 4096)
	for deadline := time.Now().Add(8 * time.Second); recorded < 2*soundRate && time.Now().Before(deadline); {
		sample := gstAppSinkTryPullSample(sink, uint64(20*time.Millisecond))
		if sample == 0 {
			continue
		}
		b := gstSampleGetBuffer(sample)
		timing := (*gstSoundBuffer)(cPointer(b))
		data = data[:gstBufferGetSize(b)]
		if len(data) > 0 {
			gstBufferExtract(b, 0, unsafe.Pointer(&data[0]), uintptr(len(data)))
		}
		pts, duration := timing.pts, timing.duration
		gstMiniObjectUnref(sample)
		if len(runs) == 0 || pts != next {
			runs = append(runs, nil)
		}
		next = pts + duration
		for frame := 0; frame+soundFrameBytes <= len(data); frame += soundFrameBytes {
			v := math.Float32frombits(binary.LittleEndian.Uint32(data[frame:]))
			run := &runs[len(runs)-1]
			if len(*run) > 0 || math.Abs(float64(v)) > 0.05 {
				*run = append(*run, v)
				recorded++
			}
		}
	}
	var laps uint32
	w.onLoop(t, func() {
		laps = p.laps
		check(p)
	})

	longest := 0
	introAgain := false
	for _, run := range runs {
		quiet, body := 0, 0
		for _, v := range run {
			level := math.Abs(float64(v))
			switch {
			case level <= 0.05:
				quiet++
				continue
			case level < 0.35:
				body++
			case body > soundRate/100:
				// 10 ms of the body went by, so this is the intro again.
				introAgain = true
			}
			// Silence counts only between sounds, not after the last one.
			longest = max(longest, quiet)
			quiet = 0
		}
	}
	t.Logf("recorded %v of the loop over %d laps, longest silence %v", trackTime(int64(recorded)), laps, trackTime(int64(longest)))
	if recorded < 2*soundRate || laps < 4 {
		t.Fatalf("recorded %v of the loop over %d laps, want 2s and at least 4 laps", trackTime(int64(recorded)), laps)
	}
	if gap := trackTime(int64(longest)); gap >= 5*time.Millisecond {
		t.Fatalf("the loop fell silent for %v, want under 5 ms", gap)
	}
	if introAgain {
		t.Fatal("a repeat played the intro again instead of starting at the loop start")
	}
}

// lateLengthTrack is a decoded track that, like a stream GStreamer cannot tell the length of, knows its length only once it played to its end.
type lateLengthTrack struct {
	pcmTrack
	ended bool
}

func (t *lateLengthTrack) frames(from, n int) ([]byte, bool, bool) {
	pcm, end, ok := t.pcmTrack.frames(from, n)
	t.ended = t.ended || end
	return pcm, end, ok
}

func (t *lateLengthTrack) length() time.Duration {
	if t.ended {
		return t.pcmTrack.length()
	}
	return 0
}

// TestLinuxAudioPlayerLoopsTrackOfLateLength loops a track whose length is unknown until its end from the loop start, not from the intro.
func TestLinuxAudioPlayerLoopsTrackOfLateLength(t *testing.T) {
	testLoopGap(t, func(p *linuxPlayer, track []byte) {
		pcm, _, _, message := decodeSound(context.Background(), track, time.Minute, trackDecodedBytes(time.Minute))
		if message != "" {
			t.Fatal(message)
		}
		p.track = &lateLengthTrack{pcmTrack: pcm}
	}, func(p *linuxPlayer) {
		if off := p.duration - 600*time.Millisecond; off > 5*time.Millisecond || off < -5*time.Millisecond {
			t.Errorf("the player reports a duration of %v, want the 600ms the track's end told", p.duration)
		}
	})
}

// TestLinuxAudioPlayerKeepsKnownDuration keeps the duration a player knows when its track cannot tell its own yet.
func TestLinuxAudioPlayerKeepsKnownDuration(t *testing.T) {
	p := &linuxPlayer{duration: 2 * time.Second, track: &lateLengthTrack{pcmTrack: make(pcmTrack, soundFrameBytes)}}
	if d := p.trackDuration(); d != 2*time.Second {
		t.Fatalf("trackDuration = %v, want the known 2s", d)
	}
}

// TestLinuxAudioTrackVoiceFollowsMixerClock moves a track voice's start on the Go clock to where the mixer's running time puts it, so that a voice whose Go clock drifted from the audio device's keeps its lead.
func TestLinuxAudioTrackVoiceFollowsMixerClock(t *testing.T) {
	w, _ := audioTestWindow(t)
	var p *linuxPlayer
	w.onLoop(t, func() {
		mixer := soundMixerTo("fakesink sync=true")
		if mixer == nil {
			return
		}
		w.audio = &linuxAudio{root: w, sounds: map[uint32]*linuxSound{}, players: map[uint32]*linuxPlayer{}, mixer: mixer, lastPlay: time.Now()}
		p = &linuxPlayer{owner: w, audio: w.audio, id: 1, source: driver.ImageSource{Data: audioTestWAV(time.Second)}, volume: 1,
			loop: true, interval: 20 * time.Millisecond}
		pcm, _, _, message := decodeSound(context.Background(), p.source.Data, time.Minute, trackDecodedBytes(time.Minute))
		if message != "" {
			t.Error(message)
			return
		}
		p.track = pcmTrack(pcm)
		w.audio.players[1] = p
		p.command(&driver.AudioRequest{Command: driver.AudioResume})
		w.audio.ensureTick()
	})
	if p == nil {
		t.Fatal("the native mixer did not open its test output")
	}
	defer w.onLoop(t, func() {
		delete(w.audio.players, 1)
		p.close()
		if w.audio.mixer != nil {
			w.audio.mixer.close()
			w.audio.mixer = nil
		}
	})
	// The voice starts on the first tick after the mixer plays, which a busy machine reaches several loop turns after resume.
	const drift = 300 * time.Millisecond
	for deadline, moved := time.Now().Add(4*time.Second), false; !moved; time.Sleep(10 * time.Millisecond) {
		w.onLoop(t, func() {
			if p.voice != nil {
				p.voice.started = p.voice.started.Add(drift)
				moved = true
			}
		})
		if !moved && time.Now().After(deadline) {
			t.Fatal("the player did not start a track voice")
		}
	}
	time.Sleep(200 * time.Millisecond)
	w.onLoop(t, func() {
		running := gstElementRunningTime(w.audio.mixer.pipeline)
		if p.voice == nil || running == math.MaxUint64 {
			t.Fatal("the track voice stopped")
		}
		want := time.Now().Add(p.voice.base - time.Duration(running))
		if off := p.voice.started.Sub(want); off > 50*time.Millisecond || off < -50*time.Millisecond {
			t.Fatalf("the voice's start is %v off the mixer's clock", off)
		}
	})
}

// TestLinuxAudioPlayerStreamsTrackThatDecodesSlowly streams a looping player's track that did not decode whole in time instead of looping it through GtkMediaFile.
func TestLinuxAudioPlayerStreamsTrackThatDecodesSlowly(t *testing.T) {
	deadline := soundDecodeDeadline
	soundDecodeDeadline = 0
	t.Cleanup(func() { soundDecodeDeadline = deadline })
	w, _ := audioTestWindow(t)
	var p *linuxPlayer
	w.onLoop(t, func() {
		w.audio = &linuxAudio{root: w, sounds: map[uint32]*linuxSound{}, players: map[uint32]*linuxPlayer{}}
		p = &linuxPlayer{owner: w, audio: w.audio, id: 1, source: driver.ImageSource{Data: audioTestWAV(time.Second)}, volume: 1, loop: true}
		w.audio.players[1] = p
		p.decodeTrack()
	})
	defer w.onLoop(t, func() {
		delete(w.audio.players, 1)
		p.close()
	})
	for until := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		var decoding, noTrack bool
		var track trackFrames
		w.onLoop(t, func() { decoding, noTrack, track = p.decoding != nil, p.noTrack, p.track })
		if !decoding {
			if _, streamed := track.(*trackDecoder); !streamed || noTrack {
				t.Fatalf("a track that did not decode in time got %T, noTrack %v, want a streaming decoder", track, noTrack)
			}
			return
		}
		if time.Now().After(until) {
			t.Fatal("the track did not finish decoding")
		}
	}
}

// TestLinuxAudioTrackDecoderDrainsItsBus seeks a streaming decoder back and forth, as a loop does at each repeat, and leaves no messages on its bus.
func TestLinuxAudioTrackDecoderDrainsItsBus(t *testing.T) {
	if !loadSound() {
		t.Skip("GStreamer is unavailable")
	}
	path := filepath.Join(t.TempDir(), "track.wav")
	if err := os.WriteFile(path, audioTestWAV(time.Second), 0o600); err != nil {
		t.Fatal(err)
	}
	d, message := openTrackDecoder(trackURI(path))
	if d == nil {
		t.Fatal(message)
	}
	defer d.close()
	for i := range 8 {
		if _, _, ok := d.frames(i%2*soundRate/2, trackPiece); !ok {
			t.Fatal("the decoder failed")
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)
	if _, _, ok := d.frames(d.at, trackPiece); !ok {
		t.Fatal("the decoder failed")
	}
	if message := gstBusPopFiltered(d.bus, gstMessageAny); message != 0 {
		kind := (*gstMessageHeader)(cPointer(message)).kind
		gstMiniObjectUnref(message)
		t.Fatalf("the decoder left a message of type %#x on its bus", kind)
	}
}
