//go:build windows

package windows

import (
	_ "embed"
	"encoding/binary"
	"errors"
	"math"
	"runtime"
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
	a.play(1, 0.2, 1, -1)
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
		a.play(2, 0.05, 1.4, 0.5)
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
	startWith(t, func(cx *core.Scope) core.Node {
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
	h.cx.Post(func() {
		h.p.Seek(500 * time.Millisecond)
		h.p.SetVolume(0.1)
		h.p.Play(func(err error) { play <- err })
	})
	if err := waitAudioTest(t, play, "audio playback"); err != nil {
		t.Fatal(err)
	}
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
