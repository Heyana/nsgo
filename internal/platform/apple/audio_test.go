//go:build darwin && !ios

package apple

import (
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// init keeps the main goroutine, and so TestMain, on the main thread.
func init() { runtime.LockOSThread() }

// mainLoop says when TestMain runs the main thread's run loop: only while a test holds it. The package's other tests, the canvas cache's, stand in for the main thread themselves, calling runMain on their own goroutine, so a run loop serving the main queue meanwhile would run their OnMain work on the main thread at the same time as they read what it writes.
var mainLoop struct {
	// running is held by the main thread while it runs the loop, so that the last release waits for the loop to stop.
	running sync.Mutex
	holds   atomic.Int32
	wake    chan struct{}
}

// holdMainLoop has the main thread run its run loop until release.
func holdMainLoop() (release func()) {
	if mainLoop.holds.Add(1) == 1 {
		select {
		case mainLoop.wake <- struct{}{}:
		default:
		}
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			if mainLoop.holds.Add(-1) == 0 {
				mainLoop.running.Lock()
				mainLoop.running.Unlock()
			}
		})
	}
}

// TestMain runs the main thread's run loop while a test holds it, as an app does: OnMain, the players' time observers and their notifications run there.
func TestMain(m *testing.M) {
	mainLoop.wake = make(chan struct{}, 1)
	go func() { os.Exit(m.Run()) }()
	if _, err := purego.Dlopen("/System/Library/Frameworks/Foundation.framework/Foundation", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		panic(err)
	}
	loop := objc.ID(Class("NSRunLoop")).Send(sel("mainRunLoop"))
	// A run loop with no port to wait on returns at once.
	loop.Send(sel("addPort:forMode:"), objc.ID(Class("NSMachPort")).Send(sel("port")), NSString("kCFRunLoopDefaultMode"))
	for {
		if mainLoop.holds.Load() == 0 {
			<-mainLoop.wake
			continue
		}
		mainLoop.running.Lock()
		if mainLoop.holds.Load() > 0 {
			withPool(func() {
				until := objc.ID(Class("NSDate")).Send(sel("dateWithTimeIntervalSinceNow:"), 0.05)
				loop.Send(sel("runMode:beforeDate:"), NSString("kCFRunLoopDefaultMode"), until)
			})
		}
		mainLoop.running.Unlock()
	}
}

// wav is length of silence as a WAV: 16-bit mono at 8 kHz.
func wav(length time.Duration) []byte { return wavOf(length, 8000, 1) }

// wavOf is length of 16-bit silence at rate with channels.
func wavOf(length time.Duration, rate, channels int) []byte {
	frame := 2 * channels
	size := int(length.Seconds()*float64(rate)) * frame
	b := make([]byte, 44+size)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(36+size))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1) // PCM
	binary.LittleEndian.PutUint16(b[22:], uint16(channels))
	binary.LittleEndian.PutUint32(b[24:], uint32(rate))
	binary.LittleEndian.PutUint32(b[28:], uint32(rate*frame))
	binary.LittleEndian.PutUint16(b[32:], uint16(frame))
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(size))
	return b
}

type audioReport struct {
	id uint32
	e  driver.AudioEvent
}

// testAudio is an engine's Audio whose reports a test reads, its temporary files in a directory of the test's.
type testAudio struct {
	t       *testing.T
	audio   *Audio
	reports chan audioReport
	dir     string
}

func newTestAudio(t *testing.T) *testAudio {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	ta := &testAudio{t: t, reports: make(chan audioReport, 256), dir: dir}
	// Registered first, the hold goes last, after the audio closed.
	t.Cleanup(holdMainLoop())
	ta.audio = NewAudio(func(ev driver.Event) {
		ta.reports <- audioReport{ev.Count, *ev.Data.(*driver.AudioEvent)}
	})
	t.Cleanup(func() { onMain(ta.audio.Close) })
	return ta
}

// onMain runs fn on the main thread and waits for it.
func onMain(fn func()) {
	defer holdMainLoop()()
	done := make(chan struct{})
	OnMain(func() {
		fn()
		close(done)
	})
	<-done
}

func (ta *testAudio) send(id uint32, r driver.AudioRequest) {
	onMain(func() { ta.audio.Handle(id, &r) })
}

// wait returns the next report of id in phase, failing the test after timeout; seen gets the reports of id on the way.
func (ta *testAudio) wait(id uint32, phase driver.AudioPhase, timeout time.Duration, seen func(driver.AudioEvent)) driver.AudioEvent {
	ta.t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case r := <-ta.reports:
			if r.id != id {
				continue
			}
			if r.e.Phase == phase {
				return r.e
			}
			if r.e.Phase == driver.AudioFailure && phase != driver.AudioFailure {
				ta.t.Fatalf("audio %d failed: %s: %s", id, r.e.Error, r.e.Message)
			}
			if seen != nil {
				seen(r.e)
			}
		case <-deadline:
			ta.t.Fatalf("audio %d reported no phase %d within %v", id, phase, timeout)
		}
	}
}

// quiet fails the test if id reports anything for d, after what it reported before the call: the main thread reports into the channel as it goes, so a command sent has had its reports queued.
func (ta *testAudio) quiet(id uint32, d time.Duration) {
	ta.t.Helper()
	for len(ta.reports) > 0 {
		<-ta.reports
	}
	deadline := time.After(d)
	for {
		select {
		case r := <-ta.reports:
			if r.id == id {
				ta.t.Fatalf("audio %d reported %+v", id, r.e)
			}
		case <-deadline:
			return
		}
	}
}

func near(got, want time.Duration) bool {
	return got > want-20*time.Millisecond && got < want+20*time.Millisecond
}

func TestASoundDecodesOnceAndPlaysOverlapping(t *testing.T) {
	ta := newTestAudio(t)
	load := driver.AudioRequest{Command: driver.AudioLoad, Source: driver.ImageSource{Data: wav(200 * time.Millisecond)},
		MaxBytes: driver.MaxSoundBytes, MaxDuration: driver.MaxSoundDuration}
	ta.send(1, load)
	e := ta.wait(1, driver.AudioLoaded, 5*time.Second, nil)
	// 1,600 frames of one channel of 32-bit floats.
	if !near(e.Duration, 200*time.Millisecond) || e.Bytes != 1600*4 || e.Cut {
		t.Fatalf("loaded %+v", e)
	}
	// Another format reconnects the voices it takes, with the engine running.
	load.Source.Data = wavOf(100*time.Millisecond, 44100, 2)
	ta.send(2, load)
	if e := ta.wait(2, driver.AudioLoaded, 5*time.Second, nil); e.Bytes != 4410*2*4 {
		t.Fatalf("a stereo sound loaded %+v", e)
	}
	for i := range driver.MaxSoundVoices + 8 {
		ta.send(uint32(1+i%2), driver.AudioRequest{Command: driver.AudioPlay, Volume: 0.01, Rate: 2})
	}
	onMain(func() {
		if n := len(ta.audio.voices); n > driver.MaxSoundVoices {
			t.Errorf("%d voices, past the limit of %d", n, driver.MaxSoundVoices)
		}
	})
	ta.send(1, driver.AudioRequest{Command: driver.AudioStop})
	ta.send(1, driver.AudioRequest{Command: driver.AudioUnload})
	ta.send(2, driver.AudioRequest{Command: driver.AudioUnload})
	onMain(func() {
		if len(ta.audio.sounds) != 0 {
			t.Error("the unloaded sound is still held")
		}
	})
	ta.quiet(1, 100*time.Millisecond)
	if files, _ := os.ReadDir(ta.dir); len(files) != 0 {
		t.Errorf("%d temporary files left", len(files))
	}
}

func TestASoundFailsAsTheDesignSays(t *testing.T) {
	ta := newTestAudio(t)
	path := filepath.Join(t.TempDir(), "click.wav")
	if err := os.WriteFile(path, wav(50*time.Millisecond), 0o644); err != nil {
		t.Fatal(err)
	}
	limits := func(src driver.ImageSource) driver.AudioRequest {
		return driver.AudioRequest{Command: driver.AudioLoad, Source: src, MaxBytes: driver.MaxSoundBytes, MaxDuration: driver.MaxSoundDuration}
	}
	ta.send(1, limits(driver.ImageSource{URI: path}))
	if e := ta.wait(1, driver.AudioLoaded, 5*time.Second, nil); !near(e.Duration, 50*time.Millisecond) {
		t.Errorf("a file path loaded %+v", e)
	}
	ta.send(2, limits(driver.ImageSource{Data: wav(6 * time.Second)}))
	if e := ta.wait(2, driver.AudioLoaded, 5*time.Second, nil); !e.Cut || e.Duration != driver.MaxSoundDuration {
		t.Errorf("a sound of 6 seconds loaded %+v, want it cut at 5", e)
	}
	for id, c := range map[uint32]struct {
		src  driver.ImageSource
		code string
	}{
		3: {driver.ImageSource{Data: []byte(strings.Repeat("not audio ", 100))}, driver.AudioDecode},
		4: {driver.ImageSource{Data: make([]byte, driver.MaxSoundBytes+1)}, driver.AudioQuotaExceeded},
		5: {driver.ImageSource{URI: filepath.Join(t.TempDir(), "missing.wav")}, driver.AudioNotSupported},
		6: {driver.ImageSource{URI: "/tmp/\xff\xfe.wav"}, driver.AudioNotSupported},
	} {
		ta.send(id, limits(c.src))
		if e := ta.wait(id, driver.AudioFailure, 5*time.Second, nil); e.Error != c.code {
			t.Errorf("sound %d failed with %q (%s), want %q", id, e.Error, e.Message, c.code)
		}
	}
	// A sound unloaded while it decodes is never reported.
	ta.send(7, limits(driver.ImageSource{Data: wav(time.Second)}))
	ta.send(7, driver.AudioRequest{Command: driver.AudioUnload})
	ta.quiet(7, 300*time.Millisecond)
}

func openPlayer(ta *testAudio, id uint32, src driver.ImageSource, loop bool) {
	ta.send(id, driver.AudioRequest{Command: driver.AudioOpen, Source: src, Volume: 0.01, Rate: 1, Loop: loop,
		ProgressInterval: 50 * time.Millisecond})
}

func TestAPlayerPlaysToItsEndAndAgain(t *testing.T) {
	ta := newTestAudio(t)
	openPlayer(ta, 1, driver.ImageSource{Data: wav(400 * time.Millisecond)}, false)
	ta.send(1, driver.AudioRequest{Command: driver.AudioResume})
	if e := ta.wait(1, driver.AudioOpened, 5*time.Second, nil); !near(e.Duration, 400*time.Millisecond) {
		t.Fatalf("opened %+v", e)
	}
	ta.wait(1, driver.AudioPlaying, time.Second, nil)
	var last time.Duration
	ta.wait(1, driver.AudioEnded, 5*time.Second, func(e driver.AudioEvent) {
		if e.Phase == driver.AudioProgress {
			if e.Position < last {
				t.Errorf("the position went back from %v to %v", last, e.Position)
			}
			last = e.Position
		}
	})
	onMain(func() {
		if p := ta.audio.players[1]; p.playing || p.release != nil {
			t.Error("an ended player plays on or holds the session")
		}
	})
	// Played again, it starts over.
	ta.send(1, driver.AudioRequest{Command: driver.AudioResume})
	ta.wait(1, driver.AudioPlaying, time.Second, nil)
	if e := ta.wait(1, driver.AudioProgress, 2*time.Second, nil); e.Position >= 300*time.Millisecond {
		t.Errorf("played again from %v", e.Position)
	}
	ta.send(1, driver.AudioRequest{Command: driver.AudioClose})
	ta.quiet(1, 300*time.Millisecond)
	if files, _ := os.ReadDir(ta.dir); len(files) != 0 {
		t.Errorf("%d temporary files left", len(files))
	}
}

func TestAPlayerLoopsSeeksAndPauses(t *testing.T) {
	ta := newTestAudio(t)
	path := filepath.Join(t.TempDir(), "loop.wav")
	if err := os.WriteFile(path, wav(300*time.Millisecond), 0o644); err != nil {
		t.Fatal(err)
	}
	openPlayer(ta, 1, driver.ImageSource{URI: path}, true)
	ta.send(1, driver.AudioRequest{Command: driver.AudioResume})
	ta.wait(1, driver.AudioPlaying, 5*time.Second, nil)
	for {
		e := ta.wait(1, driver.AudioProgress, 3*time.Second, func(e driver.AudioEvent) {
			if e.Phase == driver.AudioEnded {
				t.Fatal("a looping player ended")
			}
		})
		if e.Laps > 0 {
			break
		}
	}
	ta.send(1, driver.AudioRequest{Command: driver.AudioSeek, Position: 100 * time.Millisecond})
	// The reports from before the seek count none.
	e := ta.wait(1, driver.AudioProgress, time.Second, nil)
	for e.Seeks == 0 {
		e = ta.wait(1, driver.AudioProgress, time.Second, nil)
	}
	if e.Seeks != 1 || e.Position != 100*time.Millisecond {
		t.Errorf("after the seek %+v", e)
	}
	ta.send(1, driver.AudioRequest{Command: driver.AudioPause})
	ta.wait(1, driver.AudioPaused, time.Second, nil)
	onMain(func() {
		if p := ta.audio.players[1]; p.playing || p.release != nil {
			t.Error("a paused player plays on or holds the session")
		}
	})
	ta.send(1, driver.AudioRequest{Command: driver.AudioClose})
	ta.quiet(1, 200*time.Millisecond)
}

func TestAPlayerSeeksBeforeItLoads(t *testing.T) {
	ta := newTestAudio(t)
	ta.send(1, driver.AudioRequest{Command: driver.AudioOpen, Source: driver.ImageSource{Data: wav(time.Second)}, Volume: 0.01, Rate: 1,
		ProgressInterval: 50 * time.Millisecond, Position: 600 * time.Millisecond})
	ta.send(1, driver.AudioRequest{Command: driver.AudioPreload})
	ta.wait(1, driver.AudioOpened, 5*time.Second, nil)
	ta.send(1, driver.AudioRequest{Command: driver.AudioResume})
	ta.wait(1, driver.AudioPlaying, time.Second, nil)
	if e := ta.wait(1, driver.AudioProgress, 2*time.Second, nil); e.Position < 600*time.Millisecond {
		t.Errorf("played from %v, want 600ms on", e.Position)
	}
}

func TestAPlayerFailsAsTheDesignSays(t *testing.T) {
	ta := newTestAudio(t)
	for id, c := range map[uint32]struct {
		src  driver.ImageSource
		code string
	}{
		1: {driver.ImageSource{URI: filepath.Join(t.TempDir(), "missing.mp3")}, driver.AudioNotSupported},
		2: {driver.ImageSource{Data: []byte(strings.Repeat("not audio ", 100))}, driver.AudioDecode},
		3: {driver.ImageSource{URI: "/tmp/a\x00b.wav"}, driver.AudioNotSupported},
		4: {driver.ImageSource{URI: "ftp://example.com/a.mp3"}, driver.AudioNotSupported},
	} {
		openPlayer(ta, id, c.src, false)
		ta.send(id, driver.AudioRequest{Command: driver.AudioResume})
		if e := ta.wait(id, driver.AudioFailure, 10*time.Second, nil); e.Error != c.code {
			t.Errorf("player %d failed with %q (%s), want %q", id, e.Error, e.Message, c.code)
		}
		// Played again, it loads again and fails again.
		ta.send(id, driver.AudioRequest{Command: driver.AudioResume})
		ta.wait(id, driver.AudioFailure, 10*time.Second, nil)
	}
}

func TestAPreloadLoadsAFailedPlayerAgain(t *testing.T) {
	ta := newTestAudio(t)
	path := filepath.Join(t.TempDir(), "later.wav")
	openPlayer(ta, 1, driver.ImageSource{URI: path}, false)
	ta.send(1, driver.AudioRequest{Command: driver.AudioPreload})
	if e := ta.wait(1, driver.AudioFailure, 5*time.Second, nil); e.Error != driver.AudioNotSupported {
		t.Fatalf("a missing file failed with %q (%s)", e.Error, e.Message)
	}
	if err := os.WriteFile(path, wav(300*time.Millisecond), 0o644); err != nil {
		t.Fatal(err)
	}
	// The core waits in loading for the preload's answer: the failed player loads again.
	ta.send(1, driver.AudioRequest{Command: driver.AudioPreload})
	if e := ta.wait(1, driver.AudioOpened, 5*time.Second, nil); !near(e.Duration, 300*time.Millisecond) {
		t.Errorf("loaded again %+v", e)
	}
}

func TestAPlayerPlaysOnFromWhereItFailed(t *testing.T) {
	ta := newTestAudio(t)
	openPlayer(ta, 1, driver.ImageSource{Data: wav(3 * time.Second)}, false)
	ta.send(1, driver.AudioRequest{Command: driver.AudioResume})
	ta.wait(1, driver.AudioPlaying, 5*time.Second, nil)
	var last time.Duration
	for last < 400*time.Millisecond {
		last = ta.wait(1, driver.AudioProgress, 2*time.Second, nil).Position
	}
	// The output lost while it plays, as AVPlayerItemFailedToPlayToEndTimeNotification reports it.
	onMain(func() { ta.audio.players[1].fail(driver.AudioFailed, "the output went") })
	ta.wait(1, driver.AudioFailure, time.Second, nil)
	// The core keeps the position it had and takes none lower without a seek, so the player plays on from there.
	ta.send(1, driver.AudioRequest{Command: driver.AudioResume})
	ta.wait(1, driver.AudioOpened, 5*time.Second, nil)
	ta.wait(1, driver.AudioPlaying, time.Second, nil)
	if e := ta.wait(1, driver.AudioProgress, 2*time.Second, nil); e.Position < last || e.Seeks != 0 {
		t.Errorf("played again from %v (%d seeks), want %v on", e.Position, e.Seeks, last)
	}
}

// playing opens player id over 3 seconds of silence and plays it until it reported 200 ms played.
func playing(ta *testAudio, id uint32) {
	ta.t.Helper()
	openPlayer(ta, id, driver.ImageSource{Data: wav(3 * time.Second)}, false)
	ta.send(id, driver.AudioRequest{Command: driver.AudioResume})
	ta.wait(id, driver.AudioPlaying, 5*time.Second, nil)
	for ta.wait(id, driver.AudioProgress, 2*time.Second, nil).Position < 200*time.Millisecond {
	}
}

// checkSystemPaused fails the test unless player id is paused, its AVPlayer too, holding no session.
func checkSystemPaused(ta *testAudio, id uint32) {
	ta.t.Helper()
	onMain(func() {
		p := ta.audio.players[id]
		if p.playing || p.release != nil || objc.Send[float32](p.player, sel("rate")) != 0 {
			ta.t.Error("a player the system paused plays on or holds the session")
		}
	})
}

// interrupt posts an AVAudioSessionInterruptionNotification of kind, 1 for began and 0 for ended, with options, as iOS posts it.
func interrupt(kind, options uint) {
	number := func(n uint) objc.ID { return objc.ID(Class("NSNumber")).Send(sel("numberWithUnsignedInteger:"), n) }
	info := objc.ID(Class("NSMutableDictionary")).Send(sel("dictionary"))
	info.Send(sel("setObject:forKey:"), number(kind), avName("AVAudioSessionInterruptionTypeKey"))
	info.Send(sel("setObject:forKey:"), number(options), avName("AVAudioSessionInterruptionOptionKey"))
	center := objc.ID(Class("NSNotificationCenter")).Send(sel("defaultCenter"))
	center.Send(sel("postNotificationName:object:userInfo:"), avName("AVAudioSessionInterruptionNotification"), objc.ID(0), info)
}

func TestAnInterruptionPausesThePlayersUntilTheyPlayAgain(t *testing.T) {
	ta := newTestAudio(t)
	playing(ta, 1)
	onMain(func() { interrupt(1, 0) })
	ta.wait(1, driver.AudioPaused, 500*time.Millisecond, nil)
	checkSystemPaused(ta, 1)
	ta.quiet(1, 300*time.Millisecond)
	// Ended with AVAudioSessionInterruptionOptionShouldResume, it leaves them paused: the core has them paused.
	onMain(func() { interrupt(0, 1) })
	ta.quiet(1, 400*time.Millisecond)
	checkSystemPaused(ta, 1)
	ta.send(1, driver.AudioRequest{Command: driver.AudioResume})
	ta.wait(1, driver.AudioPlaying, time.Second, nil)
	ta.wait(1, driver.AudioProgress, time.Second, nil)
}

func TestAStopNoCommandAskedForIsASystemPause(t *testing.T) {
	ta := newTestAudio(t)
	playing(ta, 1)
	onMain(func() {
		p := ta.audio.players[1]
		// Without its time observer, as when its call as the player stops comes while the rate is not yet 0, only the rate's change reports the stop.
		p.player.Send(sel("removeTimeObserver:"), p.timeToken)
		p.timeToken.Send(sel("release"))
		p.timeToken = 0
		p.player.Send(sel("pause"))
	})
	e := ta.wait(1, driver.AudioPaused, 500*time.Millisecond, nil)
	if e.Position <= 0 || e.Position >= 3*time.Second {
		t.Errorf("paused at %v", e.Position)
	}
	checkSystemPaused(ta, 1)
	ta.quiet(1, 300*time.Millisecond)
}

// encoded is a file of name holding length of silence that afconvert encoded with args, skipping the test where the Mac has no afconvert.
func encoded(t *testing.T, length time.Duration, name string, args ...string) string {
	t.Helper()
	afconvert, err := exec.LookPath("afconvert")
	if err != nil {
		t.Skip("no afconvert to encode the audio with")
	}
	dir := t.TempDir()
	source, out := filepath.Join(dir, "source.wav"), filepath.Join(dir, name)
	if err := os.WriteFile(source, wavOf(length, 44100, 1), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, err := exec.Command(afconvert, append(args, source, out)...).CombinedOutput(); err != nil {
		t.Fatalf("afconvert %v: %v: %s", args, err, b)
	}
	return out
}

// TestCompressedAudioDecodesAndPlays plays the formats besides WAV that the design names: AAC in an M4A and FLAC, which AVFoundation decodes. MP3 stays a check by hand, as no tool on a Mac encodes it.
func TestCompressedAudioDecodesAndPlays(t *testing.T) {
	for _, c := range []struct {
		name string
		args []string
	}{
		{"silence.m4a", []string{"-f", "m4af", "-d", "aac"}},
		{"silence.flac", []string{"-f", "flac", "-d", "flac"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := encoded(t, 400*time.Millisecond, c.name, c.args...)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			ta := newTestAudio(t)
			limits := func(src driver.ImageSource) driver.AudioRequest {
				return driver.AudioRequest{Command: driver.AudioLoad, Source: src, MaxBytes: driver.MaxSoundBytes, MaxDuration: driver.MaxSoundDuration}
			}
			// From its file and from its bytes, which the temporary file is named for.
			for id, src := range []driver.ImageSource{{URI: path}, {Data: data}} {
				ta.send(uint32(id+1), limits(src))
				if e := ta.wait(uint32(id+1), driver.AudioLoaded, 5*time.Second, nil); !near(e.Duration, 400*time.Millisecond) || e.Bytes == 0 || e.Cut {
					t.Errorf("sound %d loaded %+v", id+1, e)
				}
			}
			ta.send(1, driver.AudioRequest{Command: driver.AudioPlay, Volume: 0.01, Rate: 1})
			openPlayer(ta, 3, driver.ImageSource{URI: path}, false)
			ta.send(3, driver.AudioRequest{Command: driver.AudioResume})
			if e := ta.wait(3, driver.AudioOpened, 5*time.Second, nil); !near(e.Duration, 400*time.Millisecond) {
				t.Errorf("player opened %+v", e)
			}
			ta.wait(3, driver.AudioPlaying, time.Second, nil)
			ta.wait(3, driver.AudioEnded, 5*time.Second, nil)
		})
	}
}

func TestAudioThatCannotBeDecodedFailsWithDecode(t *testing.T) {
	ta := newTestAudio(t)
	dir := t.TempDir()
	id := uint32(0)
	for _, name := range []string{"broken.m4a", "broken.flac", "broken.mp3"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(strings.Repeat("not audio ", 100)), 0o644); err != nil {
			t.Fatal(err)
		}
		id++
		ta.send(id, driver.AudioRequest{Command: driver.AudioLoad, Source: driver.ImageSource{URI: path}, MaxBytes: driver.MaxSoundBytes, MaxDuration: driver.MaxSoundDuration})
		if e := ta.wait(id, driver.AudioFailure, 5*time.Second, nil); e.Error != driver.AudioDecode {
			t.Errorf("sound of %s failed with %q (%s), want %q", name, e.Error, e.Message, driver.AudioDecode)
		}
		id++
		openPlayer(ta, id, driver.ImageSource{URI: path}, false)
		ta.send(id, driver.AudioRequest{Command: driver.AudioResume})
		if e := ta.wait(id, driver.AudioFailure, 10*time.Second, nil); e.Error != driver.AudioDecode {
			t.Errorf("player of %s failed with %q (%s), want %q", name, e.Error, e.Message, driver.AudioDecode)
		}
	}
}

func TestAudioExtension(t *testing.T) {
	for _, c := range []struct {
		data      []byte
		uri, want string
	}{
		{wav(0), "", ".wav"},
		{[]byte("ID3\x04"), "", ".mp3"},
		{[]byte("????ftypM4A "), "https://example.com/a.mp3", ".m4a"},
		{nil, "https://example.com/voice.MP3?v=2", ".mp3"},
		{nil, "asset:sounds/click.caf", ".caf"},
		{nil, "data:audio/mpeg;base64,AA==", ""},
		{nil, "/tmp/name.with spaces", ""},
	} {
		if got := audioExtension(c.data, c.uri); got != c.want {
			t.Errorf("audioExtension(%q) = %q, want %q", c.uri, got, c.want)
		}
	}
}

func TestAudioFileURLRefusesWhatNoURLNames(t *testing.T) {
	onMain(func() {
		for _, path := range []string{"", "/tmp/\xff.wav", "/tmp/a\x00.wav"} {
			if audioFileURL(path) != 0 {
				t.Errorf("audioFileURL(%q) named a URL", path)
			}
		}
		if audioFileURL("/tmp/a.wav") == 0 {
			t.Error("audioFileURL named no URL for a plain path")
		}
	})
}

// toneWAV is length of a 440 Hz sine wave as 16-bit PCM WAV at rate with channels.
func toneWAV(length time.Duration, rate, channels int) []byte {
	b := wavOf(length, rate, channels)
	frames := (len(b) - 44) / (2 * channels)
	for i := range frames {
		sample := int16(math.Sin(2*math.Pi*440*float64(i)/float64(rate)) * 16000)
		for ch := range channels {
			off := 44 + (i*channels+ch)*2
			binary.LittleEndian.PutUint16(b[off:], uint16(sample))
		}
	}
	return b
}

// TestASoundPansStereo renders a mono and a stereo sound offline through AVAudioEngine at left (-1), center (0) and right (+1) pan and verifies the rendered stereo channels.
func TestASoundPansStereo(t *testing.T) {
	for _, tc := range []struct {
		name     string
		channels int
	}{
		{"mono", 1},
		{"stereo", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ta := newTestAudio(t)
			ta.send(1, driver.AudioRequest{
				Command:     driver.AudioLoad,
				Source:      driver.ImageSource{Data: toneWAV(100*time.Millisecond, 44100, tc.channels)},
				MaxBytes:    driver.MaxSoundBytes,
				MaxDuration: driver.MaxSoundDuration,
			})
			ta.wait(1, driver.AudioLoaded, 5*time.Second, nil)

			renderRMS := func(pan float32) (left, right float64) {
				onMain(func() {
					// Put the engine into offline manual rendering mode once.
					if ta.audio.engine == 0 {
						ta.audio.engine = objc.ID(Class("AVAudioEngine")).Send(sel("alloc")).Send(sel("init"))
						outFmt := objc.ID(Class("AVAudioFormat")).Send(sel("alloc")).Send(sel("initStandardFormatWithSampleRate:channels:"), float64(44100), uint32(2))
						defer outFmt.Send(sel("release"))
						var nsErr objc.ID
						const offline = 0 // AVAudioEngineManualRenderingModeOffline
						if !objc.Send[bool](ta.audio.engine, sel("enableManualRenderingMode:format:maximumFrameCount:error:"), int(offline), outFmt, uint32(4096), unsafe.Pointer(&nsErr)) {
							t.Fatalf("enableManualRenderingMode: %s", describeError(nsErr, "failed"))
						}
						ta.audio.engine.Send(sel("mainMixerNode"))
					}
					ta.audio.play(1, 1, 1, pan)
					if len(ta.audio.voices) == 0 {
						t.Fatal("no voice")
					}
					gotPan := objc.Send[float32](ta.audio.voices[0].mixer, sel("pan"))
					if gotPan != pan {
						t.Errorf("mixer pan = %v, want %v", gotPan, pan)
					}
					outFmt := ta.audio.engine.Send(sel("manualRenderingFormat"))
					buf := objc.ID(Class("AVAudioPCMBuffer")).Send(sel("alloc")).Send(sel("initWithPCMFormat:frameCapacity:"), outFmt, uint32(4096))
					defer buf.Send(sel("release"))
					var nsErr objc.ID
					// Render 2048 frames (about 46 ms); measure the second half once the mixer's click-free parameter ramp has settled.
					status := objc.Send[int](ta.audio.engine, sel("renderOffline:toBuffer:error:"), uint32(2048), buf, unsafe.Pointer(&nsErr))
					if status != 0 { // AVAudioEngineManualRenderingStatusSuccess
						t.Fatalf("renderOffline status %d: %s", status, describeError(nsErr, "failed"))
					}
					n := int(objc.Send[uint32](buf, sel("frameLength")))
					chPtrs := objc.Send[*[2]*float32](buf, sel("floatChannelData"))
					lSamples := unsafe.Slice(chPtrs[0], n)
					rSamples := unsafe.Slice(chPtrs[1], n)
					var lSum, rSum float64
					start := n / 2
					for i := start; i < n; i++ {
						lSum += float64(lSamples[i]) * float64(lSamples[i])
						rSum += float64(rSamples[i]) * float64(rSamples[i])
					}
					count := float64(n - start)
					left = math.Sqrt(lSum / count)
					right = math.Sqrt(rSum / count)
					ta.audio.stop(1)
				})
				return left, right
			}

			lLeft, rLeft := renderRMS(-1)
			lCenter, rCenter := renderRMS(0)
			lRight, rRight := renderRMS(1)
			t.Logf("%s RMS: pan=-1 (L=%.4f R=%.4f), pan=0 (L=%.4f R=%.4f), pan=+1 (L=%.4f R=%.4f)",
				tc.name, lLeft, rLeft, lCenter, rCenter, lRight, rRight)
			if lLeft <= 0.1 || rLeft >= 0.01 {
				t.Errorf("%s pan=-1: got L=%.4f R=%.4f, want left audible and right silent", tc.name, lLeft, rLeft)
			}
			if lCenter <= 0.1 || rCenter <= 0.1 || math.Abs(lCenter-rCenter) > 0.02 {
				t.Errorf("%s pan=0: got L=%.4f R=%.4f, want balanced stereo", tc.name, lCenter, rCenter)
			}
			if lRight >= 0.01 || rRight <= 0.1 {
				t.Errorf("%s pan=+1: got L=%.4f R=%.4f, want left silent and right audible", tc.name, lRight, rRight)
			}
		})
	}
}
