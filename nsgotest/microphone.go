package nsgotest

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Microphone is a fake recorder for package microphone's RecordWAV, Authorize and Allowed, which MicrophoneInput gives an app: it writes the samples a test emits into real WAV files under the test's temporary directory, as Android's runtime writes them, so a test checks what the app recorded and how long.
type Microphone struct {
	// Denied refuses the app the microphone: Authorize and RecordWAV answer microphone.ErrDenied.
	Denied bool
	// Rate is the rate the fake records at whatever the app asks, as a device with only its own does; 0 records at the rate asked, or 48 kHz.
	Rate int

	mu       sync.Mutex
	dir      string
	running  *fakeRecording
	recorded []Recorded
}

// Recorded is a file the fake microphone recorded.
type Recorded struct {
	Path           string
	Rate, Channels int
	Duration       time.Duration
}

type fakeRecording struct {
	id             uint64
	path           string
	file           *os.File
	rate, channels int
	frames         int64
	levels, pause  bool
	silenced       bool
}

// MicrophoneInput records the app's RecordWAV with m. The recorder is one per process, as the runtime's is, so Start fails a test whose app would take the one another running test's app has. On a target the device records instead. A headless test in a browser skips, since the fake writes real WAV files.
func MicrophoneInput(m *Microphone) Option {
	return func(a *App) { a.microphone = m }
}

// microphones guards the process's recorder, the one fake connected.
var microphones struct {
	sync.Mutex
	connected *Microphone
}

// connect makes m the process's recorder for t's app.
func (m *Microphone) connect(t testing.TB) {
	t.Helper()
	if runtime.GOOS == "js" {
		t.Skip("nsgotest: MicrophoneInput needs a filesystem for its WAV files")
	}
	microphones.Lock()
	if microphones.connected != nil {
		microphones.Unlock()
		t.Fatal("nsgotest: another app's MicrophoneInput is in use; tests with a fake microphone cannot run in parallel")
		return
	}
	microphones.connected = m
	microphones.Unlock()
	m.mu.Lock()
	m.dir = filepath.Join(t.TempDir(), "recordings")
	m.mu.Unlock()
	driver.ConnectRecordings(m.command)
	m.report(m.state())
	t.Cleanup(func() {
		m.mu.Lock()
		var end map[string]any
		if m.running != nil {
			end = m.finish("unavailable")
		}
		m.mu.Unlock()
		if end != nil {
			m.report(end)
		}
		m.report(map[string]any{"type": "state"})
		driver.ConnectRecordings(nil)
		microphones.Lock()
		microphones.connected = nil
		microphones.Unlock()
	})
}

// Emit records samples, interleaved when the recording is stereo, into the running recording, reporting their level to an app that asked for levels and the duration written to one that did not; nothing records while none runs or while Interrupt silences it.
func (m *Microphone) Emit(samples []int16) {
	m.mu.Lock()
	r := m.running
	if r == nil || r.silenced || len(samples) < r.channels {
		m.mu.Unlock()
		return
	}
	samples = samples[:len(samples)/r.channels*r.channels]
	b := make([]byte, 2*len(samples))
	var peak, squares float64
	for i, s := range samples {
		binary.LittleEndian.PutUint16(b[2*i:], uint16(s))
		v := float64(s) / 32768
		peak, squares = max(peak, math.Abs(v)), squares+v*v
	}
	var level map[string]any
	if _, err := r.file.Write(b); err != nil {
		end := m.finish("failed")
		m.mu.Unlock()
		m.report(end)
		return
	}
	r.frames += int64(len(samples) / r.channels)
	m.header(r)
	if r.levels {
		level = map[string]any{"type": "level", "id": r.id, "frames": r.frames, "peak": min(peak, 1), "rms": math.Sqrt(squares / float64(len(samples)))}
	} else {
		// The runtime reports what it wrote about once a second; the fake, after each Emit.
		level = map[string]any{"type": "written", "id": r.id, "frames": r.frames}
	}
	m.mu.Unlock()
	if level != nil {
		m.report(level)
	}
}

// Interrupt silences the running recording, as a call does, or ends its silence: one that pauses on interruption writes nothing meanwhile, and another ends with microphone.ErrInterrupted.
func (m *Microphone) Interrupt(silenced bool) {
	m.mu.Lock()
	r := m.running
	if r == nil || r.silenced == silenced {
		m.mu.Unlock()
		return
	}
	var report map[string]any
	if !r.pause {
		report = m.finish("interrupted")
	} else {
		r.silenced = silenced
		report = map[string]any{"type": "interrupted", "id": r.id, "silenced": silenced}
	}
	m.mu.Unlock()
	m.report(report)
}

// Recording says a recording runs.
func (m *Microphone) Recording() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running != nil
}

// Recordings are the files the app finished recording, oldest first.
func (m *Microphone) Recordings() []Recorded {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Recorded(nil), m.recorded...)
}

// command takes one of package microphone's messages, as the runtime's RecordingService does.
func (m *Microphone) command(data []byte) {
	var c struct {
		Op       string `json:"op"`
		ID       uint64 `json:"id"`
		Name     string `json:"name"`
		Rate     int    `json:"rate"`
		Channels int    `json:"channels"`
		Level    int64  `json:"level"`
		Pause    bool   `json:"pause"`
	}
	if json.Unmarshal(data, &c) != nil {
		return
	}
	m.mu.Lock()
	var reports []any
	switch c.Op {
	case "authorize":
		code := ""
		if m.Denied {
			code = "denied"
		}
		reports = append(reports, m.state(), map[string]any{"type": "authorized", "id": c.ID, "error": code})
	case "stop":
		if m.running != nil && m.running.id == c.ID {
			reports = append(reports, m.finish(""))
		}
	case "start":
		reports = append(reports, m.state(), m.start(c.ID, c.Name, c.Rate, c.Channels, c.Level > 0, c.Pause))
	}
	m.mu.Unlock()
	for _, r := range reports {
		m.report(r)
	}
}

// start starts a recording, or reports why it cannot; m.mu is held.
func (m *Microphone) start(id uint64, name string, rate, channels int, levels, pause bool) map[string]any {
	refuse := func(code string) map[string]any { return map[string]any{"type": "ended", "id": id, "error": code} }
	if m.Denied {
		return refuse("denied")
	}
	if m.running != nil {
		return refuse("busy")
	}
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return refuse("failed")
	}
	path := filepath.Join(m.dir, name)
	if _, err := os.Stat(path); err == nil {
		return map[string]any{"type": "ended", "id": id, "error": "exists", "path": path}
	}
	if m.Rate > 0 {
		rate = m.Rate
	} else if rate <= 0 {
		rate = 48000
	}
	f, err := os.Create(filepath.Join(m.dir, "."+name+".part"))
	if err != nil {
		return refuse("failed")
	}
	r := &fakeRecording{id: id, path: path, file: f, rate: rate, channels: max(channels, 1), levels: levels, pause: pause}
	m.header(r)
	f.Seek(44, io.SeekStart)
	m.running = r
	return map[string]any{"type": "started", "id": id, "path": path, "rate": rate, "channels": r.channels, "frames": 0}
}

// finish ends the running recording with code, renaming its file; m.mu is held.
func (m *Microphone) finish(code string) map[string]any {
	r := m.running
	m.running = nil
	m.header(r)
	r.file.Close()
	os.Rename(r.file.Name(), r.path)
	m.recorded = append(m.recorded, Recorded{Path: r.path, Rate: r.rate, Channels: r.channels,
		Duration: time.Duration(r.frames) * time.Second / time.Duration(r.rate)})
	return map[string]any{"type": "ended", "id": r.id, "error": code, "path": r.path, "rate": r.rate, "channels": r.channels, "frames": r.frames}
}

// header writes r's WAV header for what it holds; m.mu is held.
func (m *Microphone) header(r *fakeRecording) {
	h := driver.WAVHeader(r.rate, r.channels, uint32(r.frames)*uint32(r.channels)*2)
	r.file.WriteAt(h[:], 0)
}

func (m *Microphone) state() map[string]any {
	return map[string]any{"type": "state", "supported": true, "allowed": !m.Denied}
}

func (m *Microphone) report(r any) {
	data, err := json.Marshal(r)
	if err != nil {
		panic(err)
	}
	driver.ReportRecording(data)
}
