//go:build windows

package windows

import (
	_ "embed"
	"encoding/binary"
	"errors"
	"math"
	"runtime"
	"slices"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/audio"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Short locally generated sine tones, for the native AAC and MP3 decoders.
//
//go:embed testdata/audio.m4a
var testAAC []byte

//go:embed testdata/audio.mp3
var testMP3 []byte

const testAudioRate = 24000

func audioTestWAV(duration time.Duration) []byte {
	frames := int(duration * testAudioRate / time.Second)
	b := make([]byte, 44+2*frames)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16) // PCM format size
	binary.LittleEndian.PutUint16(b[20:], 1)  // PCM
	binary.LittleEndian.PutUint16(b[22:], 1)  // mono
	binary.LittleEndian.PutUint32(b[24:], testAudioRate)
	binary.LittleEndian.PutUint32(b[28:], 2*testAudioRate)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(2*frames))
	for i := range frames {
		v := int16(2048 * math.Sin(float64(i)*2*math.Pi*440/testAudioRate))
		binary.LittleEndian.PutUint16(b[44+2*i:], uint16(v))
	}
	return b
}

func waitAudioTest[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(10 * time.Second):
		t.Fatalf("waiting for %s", what)
		var zero T
		return zero
	}
}

func TestWindowsAudioDecodesAndCapsSounds(t *testing.T) {
	if mfCreateSourceReader.Find() != nil {
		t.Skip("Media Foundation is unavailable")
	}
	for _, tc := range []struct {
		name string
		data []byte
		cap  time.Duration
		cut  bool
		bad  bool
	}{
		{"tone.wav", audioTestWAV(250 * time.Millisecond), time.Second, false, false},
		{"tone.m4a", testAAC, time.Second, false, false},
		{"tone.mp3", testMP3, time.Second, false, false},
		{"cut.wav", audioTestWAV(250 * time.Millisecond), 100 * time.Millisecond, true, false},
		{"invalid.bin", []byte("this is not audio"), time.Second, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type decodeResult struct {
				pcm decodedSound
				ev  driver.AudioEvent
				err error
			}
			done := make(chan decodeResult, 1)
			select {
			case soundDecoder() <- func(startErr error) {
				if startErr != nil {
					done <- decodeResult{err: startErr}
					return
				}
				pcm, ev, err := decodeWindowsSound(tc.data, tc.name, tc.cap)
				done <- decodeResult{pcm, ev, err}
			}:
			case <-time.After(10 * time.Second):
				t.Fatal("the Media Foundation decoder did not take the job")
			}
			r := waitAudioTest(t, done, "the decoded sound")
			if tc.bad {
				if r.err == nil {
					t.Fatal("invalid audio decoded")
				}
				return
			}
			if r.err != nil || r.ev.Cut != tc.cut {
				t.Fatalf("decode = %+v, %v", r.ev, r.err)
			}
			if tc.cut {
				if len(r.pcm.data) != 0 || r.ev.Duration != tc.cap {
					t.Fatalf("cut sound kept %d bytes over %v", len(r.pcm.data), r.ev.Duration)
				}
				return
			}
			if r.ev.Bytes != int64(len(r.pcm.data)) || r.ev.Duration < 200*time.Millisecond || r.ev.Duration > 400*time.Millisecond {
				t.Fatalf("incorrect decoded duration or size: %+v", r.ev)
			}
			loudest := float32(0)
			for at := 0; at < len(r.pcm.data); at += 4 {
				v := math.Float32frombits(binary.LittleEndian.Uint32(r.pcm.data[at:]))
				loudest = max(loudest, v)
			}
			if loudest < 0.01 || loudest > 1 {
				t.Fatalf("decoded samples do not contain a tone: peak %g", loudest)
			}
		})
	}
}

func TestWindowsAudioOverlapsAndStopsVoices(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &windowAudio{sounds: map[uint32]*winSound{}}
	const frames = testAudioRate * 3
	const n = frames * 4
	buf := coAlloc(n)
	if buf == nil {
		t.Fatal("cannot allocate audio fixture")
	}
	defer coTaskMemFree.Call(uintptr(buf))
	defer a.closeOutput()
	pcm := unsafe.Slice((*float32)(buf), frames)
	for i := range pcm {
		pcm[i] = float32(0.03 * math.Sin(float64(i)*2*math.Pi*440/testAudioRate))
	}
	sound := &winSound{format: soundFormat{channels: 1, rate: testAudioRate}, data: buf, size: n}
	a.sounds[1], a.sounds[2] = sound, sound
	var err error
	a.output, err = openSoundOutput()
	if err != nil {
		t.Skip(err)
	}
	started := time.Now()
	a.play(1, &driver.AudioRequest{Volume: 0.2, Rate: 1, Pan: -1})
	if a.output == nil || len(a.output.voices) != 1 {
		t.Fatal("XAudio2 did not start a voice")
	}
	v := a.output.voices[0]
	if a.output.channels >= 2 {
		// A mono voice has one level per output: full left keeps the front left's default and silences the front right.
		levels := make([]float32, a.output.channels)
		v.voice.call(xaVoiceGetMatrix, 0, 1, uintptr(a.output.channels), uintptr(unsafe.Pointer(&levels[0])))
		if v.matrix == nil || levels[0] != v.matrix[0] || levels[1] != 0 {
			t.Fatalf("panned full left, the output levels are %v over the default %v", levels, v.matrix)
		}
	} else {
		t.Logf("the default output has %d channel(s), which take no pan", a.output.channels)
	}
	const pointerBytes = unsafe.Sizeof(uintptr(0))
	for {
		var state [pointerBytes + 12]byte
		v.voice.call(xaVoiceState, uintptr(unsafe.Pointer(&state)), 0)
		if binary.LittleEndian.Uint64(state[pointerBytes+4:]) > 0 {
			t.Logf("first XAudio2 samples processed after %s (warm output)", time.Since(started))
			break
		}
		if time.Since(started) > 2*time.Second {
			t.Fatal("XAudio2 did not process playback samples")
		}
		time.Sleep(time.Millisecond)
	}
	for range driver.MaxSoundVoices {
		a.play(2, &driver.AudioRequest{Volume: 0.05, Rate: 1.4, Pan: 0.5})
	}
	if a.output == nil || len(a.output.voices) != driver.MaxSoundVoices {
		t.Fatal("overlapping playbacks did not stay at the voice cap")
	}
	a.stop(1)
	if len(a.output.voices) != driver.MaxSoundVoices {
		t.Fatal("the oldest voice survived the cap or stopping one sound interrupted another")
	}
	a.stop(2)
	if len(a.output.voices) != 0 {
		t.Fatal("stopping left source voices alive")
	}
}

func TestWindowsAudioPlayerState(t *testing.T) {
	// No output is opened by the app until an operation needs it. Probe
	// the runner here so an endpoint-less machine can still test decoding.
	runtime.LockOSThread()
	output, err := openSoundOutput()
	probe := &windowAudio{output: output}
	probe.closeOutput()
	runtime.UnlockOSThread()
	if err != nil {
		t.Skip(err)
	}
	type handle struct {
		cx *core.Scope
		p  *audio.Player
	}
	ready := make(chan handle, 1)
	loaded := make(chan time.Duration, 2)
	ended := make(chan struct{}, 4)
	play := make(chan error, 2)
	failure := make(chan error, 2)
	progress := make(chan time.Duration, 128)
	hwnd := startWith(t, func(cx *core.Scope) core.Node {
		if !cx.Env().CanPlayAudio() {
			ready <- handle{cx: cx}
			return core.View{}
		}
		p := audio.NewPlayer(cx, audio.Source{Data: audioTestWAV(2 * time.Second)}, audio.PlayerOptions{
			OnLoad: func(d time.Duration) { loaded <- d },
			OnEnd:  func() { ended <- struct{}{} },
			OnProgress: func(at, _ time.Duration) {
				select {
				case progress <- at:
				default:
				}
			},
			OnError:          func(err error) { failure <- err },
			ProgressInterval: 25 * time.Millisecond,
		})
		p.SetVolume(0.35)
		p.SetMuted(true)
		ready <- handle{cx, p}
		return core.View{}
	}, Options{Title: "windowsaudio|test"})
	h := waitAudioTest(t, ready, "the audio test window")
	if h.p == nil {
		t.Fatal("Windows did not advertise audio")
	}
	h.cx.Post(func() { h.p.Preload() })
	select {
	case d := <-loaded:
		if d < 1990*time.Millisecond || d > 2010*time.Millisecond {
			t.Fatalf("loaded duration = %v", d)
		}
	case err := <-failure:
		t.Fatal(err)
	case <-time.After(10 * time.Second):
		t.Fatal("audio player did not preload")
	}
	checkVolume := func(want float64) {
		t.Helper()
		onMain(t, hwnd, func(w *window) {
			if len(w.audio.players) != 1 {
				t.Error("the audio player was not retained")
				return
			}
			for _, p := range w.audio.players {
				const meGetVolume = 36
				if got := p.media.getDouble(meGetVolume); math.Abs(got-want) > 0.001 {
					t.Errorf("native player volume = %g, want %g", got, want)
				}
			}
		})
	}
	checkVolume(0)
	h.cx.Post(func() {
		h.p.Seek(500 * time.Millisecond)
		h.p.SetVolume(0.1) // Changing volume while muted must remain silent.
		h.p.Play(func(err error) { play <- err })
	})
	if err := waitAudioTest(t, play, "audio playback"); err != nil {
		t.Fatal(err)
	}
	checkVolume(0)
	h.cx.Post(func() {
		h.p.SetMuted(false)
		h.p.Play(func(err error) { play <- err })
	})
	if err := waitAudioTest(t, play, "unmuting the playing player"); err != nil {
		t.Fatal(err)
	}
	checkVolume(0.1)
	if at := waitAudioTest(t, progress, "audio progress after a seek"); at < 450*time.Millisecond {
		t.Fatalf("playback started at %v after seeking to 500 ms", at)
	}
	h.cx.Post(func() { h.p.Pause() })
	for deadline := time.Now().Add(2 * time.Second); ; {
		state := make(chan audio.PlayerState, 1)
		h.cx.Post(func() { state <- h.p.State() })
		if waitAudioTest(t, state, "the player state") == audio.PlayerPaused {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("audio player did not pause")
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.cx.Post(func() {
		h.p.SetLoop(true)
		h.p.SetRate(2)
		h.p.Play(func(err error) { play <- err })
	})
	if err := waitAudioTest(t, play, "audio resuming"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for looped := false; !looped; {
		select {
		case <-ended:
			t.Fatal("a looping player reported its end")
		case at := <-progress:
			looped = at < 150*time.Millisecond
		case <-deadline:
			t.Fatal("audio player did not loop")
		}
	}
	h.cx.Post(func() { h.p.SetLoop(false) })
	waitAudioTest(t, ended, "audio playback reaching its end")
	h.cx.Post(func() {
		h.p.Close()
		bad := audio.NewPlayer(h.cx, audio.Source{Data: []byte("not audio")}, audio.PlayerOptions{})
		bad.Play(func(err error) { play <- err })
	})
	if err := waitAudioTest(t, play, "invalid audio failure"); !errors.Is(err, audio.Decode) {
		t.Fatalf("invalid audio failed with %v, want Decode", err)
	}
}

func TestWindowsAudioControlsOneNativeVoiceAndReportsEachEnd(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	output, err := openSoundOutput()
	if err != nil {
		t.Skip(err)
	}
	a := &windowAudio{output: output}
	defer a.closeOutput()
	const frames = testAudioRate * 3
	buffer := coAlloc(frames * 4)
	if buffer == nil {
		t.Fatal("cannot allocate audio fixture")
	}
	// Native source voices must be destroyed before their sample storage.
	defer func() { a.closeOutput(); coTaskMemFree.Call(uintptr(buffer)) }()
	clear(unsafe.Slice((*float32)(buffer), frames))
	sound := &winSound{format: soundFormat{channels: 1, rate: testAudioRate}, data: buffer, size: frames * 4}
	var ended []uint32
	output.ended = func(_ uint32, playback uint32) { ended = append(ended, playback) }
	play := func(id, playback uint32, clip *winSound) {
		t.Helper()
		if !output.play(id, clip, &driver.AudioRequest{Playback: playback, Rate: 1}) {
			t.Fatal("native voice did not start")
		}
	}
	find := func(playback uint32) *soundVoice {
		for i := range output.voices {
			if output.voices[i].playback == playback {
				return &output.voices[i]
			}
		}
		return nil
	}
	play(1, 1, sound)
	play(2, 2, sound)
	a.controlPlayback(1, &driver.AudioRequest{Command: driver.AudioPlaybackSet, Playback: 1, Volume: 0.25, Rate: 0.5})
	const getVolume, getRate = 13, 27
	voice := find(1)
	if a.output != output || voice == nil {
		t.Fatal("native live controls discarded the voice")
	}
	var volume, rate float32
	voice.voice.call(getVolume, uintptr(unsafe.Pointer(&volume)))
	if volume != 0.25 {
		t.Fatalf("live voice volume = %g", volume)
	}
	for deadline := time.Now().Add(time.Second); ; {
		voice.voice.call(getRate, uintptr(unsafe.Pointer(&rate)))
		if rate == 0.5 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("live voice rate = %g", rate)
		}
		time.Sleep(time.Millisecond)
	}
	a.controlPlayback(1, &driver.AudioRequest{Command: driver.AudioPlaybackStop, Playback: 1})
	if !slices.Equal(ended, []uint32{1}) || find(2) == nil {
		t.Fatal("stopping one voice ended its overlapping sibling")
	}
	for playback := uint32(3); playback <= driver.MaxSoundVoices+2; playback++ {
		play(2, playback, sound)
	}
	if len(output.voices) != driver.MaxSoundVoices || !slices.Equal(ended, []uint32{1, 2}) {
		t.Fatalf("voice eviction ended %v", ended)
	}
	a.stop(2)
	if len(output.voices) != 0 || len(ended) != driver.MaxSoundVoices+2 {
		t.Fatalf("stopping all voices ended %v", ended)
	}
	// Reusing a voice must report its old handle once, then retain only the new one.
	short := *sound
	short.size = testAudioRate / 20 * 4
	play(1, 100, &short)
	for deadline := time.Now().Add(time.Second); !slices.Contains(ended, uint32(100)); {
		output.reap()
		if time.Now().After(deadline) {
			t.Fatal("a completed native buffer did not end its handle")
		}
		time.Sleep(time.Millisecond)
	}
	play(1, 101, sound)
	a.controlPlayback(1, &driver.AudioRequest{Command: driver.AudioPlaybackStop, Playback: 100})
	output.reap()
	count := 0
	for _, playback := range ended {
		if playback == 100 {
			count++
		}
	}
	if find(101) == nil || count != 1 {
		t.Fatal("a stale handle stopped the reused voice or ended twice")
	}
}

func TestWindowsAudioPlaybackEndsWhileItsWindowIsHidden(t *testing.T) {
	runtime.LockOSThread()
	output, err := openSoundOutput()
	probe := &windowAudio{output: output}
	probe.closeOutput()
	runtime.UnlockOSThread()
	if err != nil {
		t.Logf("no default audio endpoint; verifying failed-start completion: %v", err)
	}
	type handle struct {
		cx    *core.Scope
		sound *audio.Sound
		err   error
	}
	ready := make(chan handle, 1)
	startWith(t, func(cx *core.Scope) core.Node {
		audio.LoadSound(cx, audio.Source{Data: audioTestWAV(300 * time.Millisecond)}, func(sound *audio.Sound, err error) {
			ready <- handle{cx, sound, err}
		})
		return core.View{}
	}, Options{Title: "windowsaudiohidden|test", Hidden: true})
	h := waitAudioTest(t, ready, "the hidden window's sound load")
	if h.err != nil {
		t.Fatal(h.err)
	}
	type start struct {
		playback *audio.Playback
		ended    bool
	}
	started := make(chan start, 1)
	h.cx.Post(func() {
		playback := h.sound.Play(audio.Voice{Muted: true})
		started <- start{playback, playback.Ended()}
	})
	playback := waitAudioTest(t, started, "the tracked sound")
	state := make(chan bool, 1)
	if playback.ended {
		t.Fatal("Windows returned an already ended handle")
	}
	for deadline := time.Now().Add(3 * time.Second); ; {
		h.cx.Post(func() { state <- playback.playback.Ended() })
		if waitAudioTest(t, state, "the playback end state") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the hidden window did not receive its playback's native end")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
