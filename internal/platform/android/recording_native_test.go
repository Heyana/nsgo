//go:build android && nsgotest

package android

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/testhost"
	"github.com/nsgo-dev/nsgo/microphone"
)

// This opt-in test records only into its installed test application's private directory and deletes the WAV. Its host controller grants that test APK's microphone permission, backgrounds it, turns the screen off and resumes it.
func TestNativeAndroidRecordingAcceptance(t *testing.T) {
	duration, err := time.ParseDuration(os.Getenv("NSGO_ANDROID_RECORDING_ACCEPTANCE"))
	if err != nil || duration < 15*time.Second {
		t.Skip("set NSGO_ANDROID_RECORDING_ACCEPTANCE=30m with the recording device controller")
	}
	var cx *core.Scope
	live, err := testhost.Current().Open(func(scope *core.Scope) core.Node { cx = scope; return core.View{} }, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	live.Sync()
	target := live.(*testSurface)
	step := func(fn func()) { cx.Post(fn); live.Sync() }
	mark := func(phase string) { fmt.Fprintln(os.Stderr, "   recording acceptance "+phase) }
	ready := filepath.Join(os.TempDir(), "nsgo-recording-test-ready")
	defer os.Remove(ready)
	mark("waiting-for-permission")
	for deadline := time.Now().Add(time.Minute); ; {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the device controller did not grant the test permission")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := microphone.Authorize(); err != nil {
		t.Fatal(err)
	}
	type probe struct {
		Notification bool `json:"notification"`
		Descriptors  int  `json:"descriptors"`
		PSS          int  `json:"pssKB"`
	}
	inspect := func() probe {
		t.Helper()
		data, err := target.ask(map[string]any{"op": "recording-probe"})
		if err != nil {
			t.Fatal(err)
		}
		var value probe
		if err := json.Unmarshal([]byte(data), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	var levels atomic.Int64
	var player *core.AudioPlayer
	var audio bool
	step(func() { audio = cx.Env().CanPlayAudio() })
	waitPlay := func(ch <-chan error) {
		t.Helper()
		select {
		case err := <-ch:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(6 * time.Second):
			t.Fatal("audio playback did not start")
		}
	}
	if audio {
		playing := make(chan error, 1)
		step(func() {
			player = cx.NewAudioPlayer(core.AudioSource{Data: recordingSilenceWAV(time.Second)}, core.AudioPlayerOptions{})
			player.SetLoop(true)
			player.Play(func(err error) { playing <- err })
		})
		waitPlay(playing)
		defer func() { step(player.Close) }()
	}
	name := fmt.Sprintf("acceptance-%d.wav", time.Now().UnixNano())
	recording, err := microphone.RecordWAV(name, microphone.RecordOptions{
		Rate: 44100, LevelInterval: 50 * time.Millisecond, Level: func(microphone.Level) { levels.Add(1) },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		recording.Stop()
		if path := recording.File().Path; path != "" {
			_ = os.Remove(path)
			_ = os.Remove(filepath.Join(filepath.Dir(path), "."+name+".part"))
		}
	}()
	started := time.Now()
	if audio {
		loaded := make(chan error, 1)
		step(func() {
			if player.State() != core.AudioPlayerPlaying {
				t.Error("starting the recorder interrupted playback")
			}
			cx.LoadSound(core.AudioSource{Data: recordingSilenceWAV(100 * time.Millisecond)}, func(sound *core.Sound, err error) {
				if err == nil {
					sound.Play(core.SoundVoice{})
					cx.AfterFunc(200*time.Millisecond, sound.Close)
				}
				loaded <- err
			})
		})
		waitPlay(loaded)
		time.Sleep(300 * time.Millisecond)
		step(func() {
			if player.State() != core.AudioPlayerPlaying {
				t.Error("a cue interrupted the background player")
			}
			player.Close()
		})
		mark("playback-coexistence-passed")
	}
	if err := target.do(map[string]any{"op": "recreate-activity"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	initial := inspect()
	if !initial.Notification || initial.Descriptors != 1 {
		t.Fatalf("recording after recreation: notification %t, descriptors %d", initial.Notification, initial.Descriptors)
	}
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)
	mark("ready-for-screen-off")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	previous := time.Duration(0)
	for time.Since(started) < duration {
		select {
		case <-recording.Ended():
			t.Fatalf("recording ended after %s: %v", time.Since(started).Round(time.Second), recording.Err())
		case <-ticker.C:
			file := recording.File()
			if file.Duration < previous {
				t.Fatal("recording duration moved backwards")
			}
			previous = file.Duration
			if elapsed := int(time.Since(started).Seconds()); elapsed%60 == 0 {
				var memory runtime.MemStats
				runtime.ReadMemStats(&memory)
				if memory.HeapAlloc > baseline.HeapAlloc+64<<20 {
					t.Fatal("Go heap grew with the streamed recording")
				}
				fmt.Fprintf(os.Stderr, "   recording acceptance elapsed=%ds duration=%s levels=%d\n", elapsed, file.Duration.Round(time.Second), levels.Load())
			}
		}
	}
	mark("ready-to-stop")
	for deadline := time.Now().Add(time.Minute); ; {
		var foreground bool
		step(func() { foreground = cx.Env().Lifecycle() == core.LifecycleActive })
		if foreground {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the device controller did not resume the recording app")
		}
		time.Sleep(100 * time.Millisecond)
	}
	recording.Stop()
	if err := recording.Err(); err != nil {
		t.Fatal(err)
	}
	file := recording.File()
	if file.Duration < duration-time.Second || float64(levels.Load())/file.Duration.Seconds() < 10 {
		t.Fatalf("short recording or slow levels: duration %s, callbacks %d", file.Duration, levels.Load())
	}
	f, err := os.Open(file.Path)
	if err != nil {
		t.Fatal(err)
	}
	var header [44]byte
	_, readErr := io.ReadFull(f, header[:])
	stat, statErr := f.Stat()
	f.Close()
	if readErr != nil || statErr != nil || string(header[:4]) != "RIFF" || string(header[8:12]) != "WAVE" ||
		int64(binary.LittleEndian.Uint32(header[4:])) != stat.Size()-8 ||
		int64(binary.LittleEndian.Uint32(header[40:])) != stat.Size()-44 ||
		binary.LittleEndian.Uint16(header[20:]) != 1 || binary.LittleEndian.Uint16(header[34:]) != 16 ||
		int(binary.LittleEndian.Uint16(header[22:])) != file.Channels ||
		int(binary.LittleEndian.Uint32(header[24:])) != file.Rate ||
		int(binary.LittleEndian.Uint16(header[32:])) != 2*file.Channels ||
		int(binary.LittleEndian.Uint32(header[28:])) != 2*file.Channels*file.Rate {
		t.Fatal("the completed WAV header does not describe its file")
	}
	if audio {
		playing, ended := make(chan error, 1), make(chan struct{}, 1)
		step(func() {
			player = cx.NewAudioPlayer(core.AudioSource{URI: file.Path}, core.AudioPlayerOptions{OnEnd: func() { ended <- struct{}{} }})
			player.Play(func(err error) { playing <- err })
		})
		waitPlay(playing)
		step(func() {
			if delta := player.Duration() - file.Duration; delta < -50*time.Millisecond || delta > 50*time.Millisecond {
				t.Errorf("replay duration %s differs from recording %s", player.Duration(), file.Duration)
			}
			player.Seek(max(0, file.Duration-200*time.Millisecond))
		})
		select {
		case <-ended:
		case <-time.After(6 * time.Second):
			t.Fatal("the completed WAV did not replay to its end")
		}
		step(player.Close)
		mark("wav-replay-passed")
	}
	final := inspect()
	for deadline := time.Now().Add(3 * time.Second); final.Notification && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
		final = inspect()
	}
	if final.Notification || final.Descriptors != 0 || final.PSS > initial.PSS+64*1024 {
		t.Fatalf("recording leaked native resources: notification %t, descriptors %d, PSS growth %d KiB",
			final.Notification, final.Descriptors, final.PSS-initial.PSS)
	}
	fmt.Fprintf(os.Stderr, "   recording acceptance complete duration=%s rate=%d channels=%d levels=%d bytes=%d\n",
		file.Duration.Round(time.Millisecond), file.Rate, file.Channels, levels.Load(), stat.Size())
}

func recordingSilenceWAV(duration time.Duration) []byte {
	const rate = 8000
	data := make([]byte, 44+2*int(duration.Seconds()*rate))
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
	binary.LittleEndian.PutUint32(data[40:], uint32(len(data)-44))
	return data
}

func TestNativeAndroidRecordingBackgroundRefusal(t *testing.T) {
	if os.Getenv("NSGO_ANDROID_RECORDING_ACCEPTANCE") == "" {
		t.Skip("run the recording device controller with background-refusal")
	}
	live, err := testhost.Current().Open(func(*core.Scope) core.Node { return core.View{} }, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	live.Sync()
	target := live.(*testSurface)
	ready := filepath.Join(os.TempDir(), "nsgo-recording-test-ready")
	defer os.Remove(ready)
	fmt.Fprintln(os.Stderr, "   recording acceptance waiting-for-permission")
	for deadline := time.Now().Add(time.Minute); ; {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the device controller did not grant the test permission")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := microphone.Authorize(); err != nil {
		t.Fatal(err)
	}
	visible := func() bool {
		t.Helper()
		data, err := target.ask(map[string]any{"op": "recording-probe"})
		if err != nil {
			t.Fatal(err)
		}
		var value struct{ Visible bool }
		if err := json.Unmarshal([]byte(data), &value); err != nil {
			t.Fatal(err)
		}
		return value.Visible
	}
	fmt.Fprintln(os.Stderr, "   recording acceptance ready-for-screen-off")
	for deadline := time.Now().Add(15 * time.Second); visible(); {
		if time.Now().After(deadline) {
			t.Fatal("the device controller did not background the test")
		}
		time.Sleep(100 * time.Millisecond)
	}
	answer := make(chan error, 1)
	go func() {
		recording, err := microphone.RecordWAV("background-refused.wav", microphone.RecordOptions{})
		if recording != nil {
			recording.Stop()
			_ = os.Remove(recording.File().Path)
		}
		answer <- err
	}()
	select {
	case err := <-answer:
		if !errors.Is(err, microphone.ErrUnavailable) {
			t.Fatalf("background start returned %v, want ErrUnavailable", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("the background start did not answer")
	}
	fmt.Fprintln(os.Stderr, "   recording acceptance ready-to-stop")
	for deadline := time.Now().Add(15 * time.Second); !visible(); {
		if time.Now().After(deadline) {
			t.Fatal("the device controller did not resume the test")
		}
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Fprintln(os.Stderr, "   recording acceptance complete background-refusal")
}
