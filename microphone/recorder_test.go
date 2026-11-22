package microphone

import (
	"encoding/json"
	"errors"
	"io/fs"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// fakeRecorder stands in for Android's RecordingService: it hears package microphone's commands and reports what answer says to each.
type fakeRecorder struct {
	mu       sync.Mutex
	commands []command
}

func connect(t *testing.T, answer func(c command) []report) *fakeRecorder {
	t.Helper()
	f := &fakeRecorder{}
	driver.ConnectRecordings(func(msg []byte) {
		var c command
		if err := json.Unmarshal(msg, &c); err != nil {
			t.Errorf("invalid command %s", msg)
			return
		}
		f.mu.Lock()
		f.commands = append(f.commands, c)
		f.mu.Unlock()
		for _, r := range answer(c) {
			reportJSON(r)
		}
	})
	t.Cleanup(func() {
		reportJSON(report{Type: "state"})
		driver.ConnectRecordings(nil)
	})
	return f
}

func reportJSON(r report) {
	data, err := json.Marshal(r)
	if err != nil {
		panic(err)
	}
	driver.ReportRecording(data)
}

// recordAt answers a start with a recording at rate and a stop with its end after frames.
func recordAt(rate int, frames int64) func(c command) []report {
	return func(c command) []report {
		switch c.Op {
		case "start":
			return []report{{Type: "started", ID: c.ID, Path: "/data/recordings/" + c.Name, Rate: rate, Channels: c.Channels}}
		case "stop":
			return []report{{Type: "ended", ID: c.ID, Path: "/data/recordings/take.wav", Rate: rate, Channels: 1, Frames: frames}}
		}
		return nil
	}
}

func TestRecordWAVReportsTheFileWritten(t *testing.T) {
	f := connect(t, recordAt(48000, 96000))
	r, err := RecordWAV("take.wav", RecordOptions{Rate: 44100})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.File(); got.Path != "/data/recordings/take.wav" || got.Rate != 48000 || got.Channels != 1 {
		t.Errorf("File = %+v, want the device's 48 kHz mono", got)
	}
	r.Stop()
	r.Stop()
	select {
	case <-r.Ended():
	default:
		t.Fatal("Ended is open after Stop")
	}
	if d := r.File().Duration; d != 2*time.Second {
		t.Errorf("Duration = %v, want 2s", d)
	}
	if err := r.Err(); err != nil {
		t.Errorf("Err after Stop = %v", err)
	}
	if len(f.commands) != 2 || f.commands[0].Rate != 44100 || f.commands[0].Channels != 1 || f.commands[0].Level != 0 || f.commands[1].Op != "stop" {
		t.Errorf("commands = %+v, want a start at 44.1 kHz mono without levels and one stop", f.commands)
	}
}

func TestRecordWAVRefusals(t *testing.T) {
	code := ""
	connect(t, func(c command) []report {
		return []report{{Type: "ended", ID: c.ID, Error: code, Path: "/data/recordings/" + c.Name}}
	})
	for _, tc := range []struct {
		code string
		want error
	}{{"denied", ErrDenied}, {"busy", ErrBusy}, {"unavailable", ErrUnavailable}, {"space", ErrNoSpace}, {"exists", fs.ErrExist}} {
		code = tc.code
		if _, err := RecordWAV("take.wav", RecordOptions{}); !errors.Is(err, tc.want) {
			t.Errorf("%s: RecordWAV = %v, want %v", tc.code, err, tc.want)
		}
	}
	for _, bad := range []string{"", ".hidden.wav", "dir/take.wav", `dir\take.wav`} {
		if _, err := RecordWAV(bad, RecordOptions{}); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("RecordWAV(%q) = %v, want fs.ErrInvalid", bad, err)
		}
	}
	if _, err := RecordWAV("take.wav", RecordOptions{Channels: 3}); err == nil {
		t.Error("three channels were accepted")
	}
}

func TestRecordWAVEndsBySelf(t *testing.T) {
	var id uint64
	connect(t, func(c command) []report {
		id = c.ID
		return []report{{Type: "started", ID: c.ID, Path: "/data/recordings/take.wav", Rate: 16000, Channels: 1}}
	})
	for code, want := range map[string]error{"interrupted": ErrInterrupted, "full": ErrFull, "space": ErrNoSpace} {
		r, err := RecordWAV("take.wav", RecordOptions{})
		if err != nil {
			t.Fatal(err)
		}
		reportJSON(report{Type: "ended", ID: id, Error: code, Path: "/data/recordings/take.wav", Rate: 16000, Channels: 1, Frames: 8000})
		<-r.Ended()
		if err := r.Err(); !errors.Is(err, want) {
			t.Errorf("%s: Err = %v, want %v", code, err, want)
		}
		if d := r.File().Duration; d != time.Second/2 {
			t.Errorf("%s: Duration = %v", code, d)
		}
		// Stop after an end sends nothing and returns.
		r.Stop()
	}
}

// TestDurationWithoutLevels checks that a recording without Level reports its duration while it runs, from the frames the recorder says it wrote.
func TestDurationWithoutLevels(t *testing.T) {
	f := connect(t, recordAt(16000, 48000))
	r, err := RecordWAV("take.wav", RecordOptions{})
	if err != nil {
		t.Fatal(err)
	}
	id := f.commands[0].ID
	reportJSON(report{Type: "written", ID: id, Frames: 16000})
	if d := r.File().Duration; d != time.Second {
		t.Errorf("Duration after a second written = %v, want 1s", d)
	}
	reportJSON(report{Type: "written", ID: id, Frames: 32000})
	if d := r.File().Duration; d != 2*time.Second {
		t.Errorf("Duration after two seconds written = %v, want 2s", d)
	}
	r.Stop()
	if d := r.File().Duration; d != 3*time.Second {
		t.Errorf("Duration after Stop = %v, want 3s", d)
	}
}

// TestLevelsNeverWaitForTheApp checks that the recorder's reports go on while a level callback is slow, and that the callback then takes the latest level.
func TestLevelsNeverWaitForTheApp(t *testing.T) {
	var id uint64
	f := connect(t, func(c command) []report {
		if c.Op == "start" {
			id = c.ID
			return []report{{Type: "started", ID: c.ID, Path: "/data/recordings/take.wav", Rate: 48000, Channels: 1}}
		}
		return []report{{Type: "ended", ID: c.ID}}
	})
	release := make(chan struct{})
	var mu sync.Mutex
	var heard []Level
	r, err := RecordWAV("take.wav", RecordOptions{Level: func(l Level) {
		<-release
		mu.Lock()
		heard = append(heard, l)
		mu.Unlock()
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.commands[0].Level; got != 100 {
		t.Errorf("level interval = %dms, want a tenth of a second", got)
	}
	done := make(chan struct{})
	go func() {
		for i := 1; i <= 100; i++ {
			reportJSON(report{Type: "level", ID: id, Frames: int64(i) * 4800, Peak: float32(i) / 100, RMS: 0.1})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("a slow level callback held up the recorder's reports")
	}
	close(release)
	deadline := time.Now().Add(10 * time.Second)
	for {
		mu.Lock()
		n := len(heard)
		last := Level{}
		if n > 0 {
			last = heard[n-1]
		}
		mu.Unlock()
		if last.Peak == 1 {
			if n > 3 {
				t.Errorf("the callback took %d of 100 levels, want the waiting ones replaced", n)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the callback took %d levels, the last %v, want the latest", n, last)
		}
		time.Sleep(time.Millisecond)
	}
	if d := r.File().Duration; d != 10*time.Second {
		t.Errorf("Duration = %v, want the frames the last level counted", d)
	}
	r.Stop()
}

func TestPauseOnInterruption(t *testing.T) {
	var id uint64
	f := connect(t, func(c command) []report {
		if c.Op == "start" {
			id = c.ID
			return []report{{Type: "started", ID: c.ID, Path: "/data/recordings/take.wav", Rate: 48000, Channels: 1}}
		}
		return []report{{Type: "ended", ID: c.ID}}
	})
	r, err := RecordWAV("take.wav", RecordOptions{PauseOnInterruption: true})
	if err != nil {
		t.Fatal(err)
	}
	if !f.commands[0].Pause {
		t.Error("the start does not ask to pause")
	}
	reportJSON(report{Type: "interrupted", ID: id, Silenced: true})
	if !r.Interrupted() {
		t.Error("not interrupted while silenced")
	}
	reportJSON(report{Type: "interrupted", ID: id})
	if r.Interrupted() {
		t.Error("interrupted after the silence")
	}
	r.Stop()
}

func TestEarlierRecordings(t *testing.T) {
	connect(t, func(command) []report { return nil })
	reportJSON(report{Type: "earlier", Path: "/data/recordings/a.wav", Rate: 48000, Channels: 1, Frames: 48000, Error: "partial"})
	reportJSON(report{Type: "earlier", Path: "/data/recordings/b.wav", Rate: 48000, Channels: 2, Frames: 96000, Error: "abandoned"})
	earlier := Earlier()
	if len(earlier) != 2 || !errors.Is(earlier[0].Err(), ErrPartial) || !errors.Is(earlier[1].Err(), ErrAbandoned) {
		t.Fatalf("Earlier = %v", earlier)
	}
	if f := earlier[1].File(); f.Path != "/data/recordings/b.wav" || f.Channels != 2 || f.Duration != 2*time.Second {
		t.Errorf("File = %+v", f)
	}
	earlier[0].Stop()
	if len(Earlier()) != 0 {
		t.Error("Earlier reported a recording twice")
	}
}

// TestEarlierTellsTheRecorderWhatItTook holds the runtime's side of an earlier recording: it keeps the record until Earlier takes it, reporting it to each process that connects meanwhile, so Earlier names each file it returned, and a record heard again, from the runtime's journal or a repair after an abandonment, replaces the one not taken yet.
func TestEarlierTellsTheRecorderWhatItTook(t *testing.T) {
	f := connect(t, func(command) []report { return nil })
	reportJSON(report{Type: "earlier", Path: "/data/recordings/a.wav", Rate: 48000, Channels: 1, Frames: 48000, Error: "abandoned"})
	reportJSON(report{Type: "earlier", Path: "/data/recordings/b.wav", Rate: 48000, Channels: 1, Frames: 48000, Error: "partial"})
	reportJSON(report{Type: "earlier", Path: "/data/recordings/a.wav", Rate: 48000, Channels: 1, Frames: 96000, Error: "partial"})
	earlier := Earlier()
	if len(earlier) != 2 || earlier[0].File().Path != "/data/recordings/b.wav" || earlier[1].File().Path != "/data/recordings/a.wav" ||
		!errors.Is(earlier[1].Err(), ErrPartial) || earlier[1].File().Duration != 2*time.Second {
		t.Fatalf("Earlier = %v, want b.wav and a.wav's newer record", earlier)
	}
	f.mu.Lock()
	var taken []string
	for _, c := range f.commands {
		if c.Op == "taken" {
			taken = append(taken, c.Path)
		}
	}
	f.mu.Unlock()
	if len(taken) != 2 || taken[0] != "/data/recordings/b.wav" || taken[1] != "/data/recordings/a.wav" {
		t.Errorf("taken = %q, want both files", taken)
	}
	if len(Earlier()) != 0 {
		t.Error("Earlier reported a recording twice")
	}
}

func TestAuthorizeThroughTheRecorder(t *testing.T) {
	denied := true
	connect(t, func(c command) []report {
		if denied {
			return []report{{Type: "state", Supported: true}, {Type: "authorized", ID: c.ID, Error: "denied"}}
		}
		return []report{{Type: "state", Supported: true, Allowed: true}, {Type: "authorized", ID: c.ID}}
	})
	if err := Authorize(); !errors.Is(err, ErrDenied) {
		t.Errorf("Authorize = %v, want ErrDenied", err)
	}
	if !Supported() || Allowed() {
		t.Errorf("Supported %v and Allowed %v, want a recorder that may not record", Supported(), Allowed())
	}
	denied = false
	if err := Authorize(); err != nil || !Allowed() {
		t.Errorf("Authorize = %v and Allowed %v once granted", err, Allowed())
	}
}

func TestRecordWAVWithoutARecorder(t *testing.T) {
	if runtime.GOOS == "android" || hasLocalRecorder {
		t.Skip("this target records directly without an external recorder")
	}
	if _, err := RecordWAV("take.wav", RecordOptions{}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("RecordWAV = %v, want ErrUnsupported", err)
	}
}
