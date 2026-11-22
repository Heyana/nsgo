// Package microphone records the system's default input and builds on every target, reporting where it cannot record. Capture streams 16-bit mono PCM on macOS, iOS, Windows and Linux, which it reaches directly, as base/terminal does. RecordWAV writes 16-bit PCM WAV files on macOS, iOS, Windows, Linux and Android; on Android the runtime's foreground service records, which the package reaches through a channel of internal/driver outside any view, as a recording belongs to the process.
//
// Apps check and ask for the microphone with core.Scope.PermissionStatus and core.Scope.RequestPermission of core.PermissionMicrophone; Allowed and Authorize share their implementation for code that records without an engine.
package microphone

import (
	"errors"
	"io/fs"
	"strings"
	"sync"
	"time"
)

var (
	// ErrUnsupported says this target cannot record.
	ErrUnsupported = errors.New("microphone: not supported on this platform")
	// ErrDenied says the user or the system refused the app the microphone.
	ErrDenied = errors.New("microphone: access denied")
	// ErrNoUsageDescription says the process the system holds responsible
	// for this one, such as the app that started it, declares no reason for
	// recording, for which the system would abort this process.
	ErrNoUsageDescription = errors.New("microphone: the responsible app has no NSMicrophoneUsageDescription")
	// ErrUnavailable says the microphone cannot record now: on Android no activity of the app is visible to ask for it or start using it, the app declares no recording service (nsgo.yaml's android.recording), or the device refused to open it.
	ErrUnavailable = errors.New("microphone: unavailable")
	// ErrBusy says a recording runs already, as an app records one at a time, or another app's capture keeps this one from starting.
	ErrBusy = errors.New("microphone: busy")
	// ErrInterrupted ends a recording the system silenced, as a call does, unless RecordOptions.PauseOnInterruption keeps it.
	ErrInterrupted = errors.New("microphone: interrupted")
	// ErrFull ends a WAV recording that reached RIFF's 4 GiB, about 12 hours of 16-bit mono at 48 kHz.
	ErrFull = errors.New("microphone: the WAV file reached its 4 GiB limit")
	// ErrNoSpace ends a recording whose storage is about to run out.
	ErrNoSpace = errors.New("microphone: no space left for the recording")
	// ErrAbandoned is an earlier recording whose Go process ended while it ran: the runtime finished its file.
	ErrAbandoned = errors.New("microphone: the process that recorded ended")
	// ErrPartial is an earlier recording whose app was killed while it ran: the next RecordWAV repaired its file, which holds what was written until then.
	ErrPartial = errors.New("microphone: the recording was cut off when the app was killed")
)

// Stream is a recording in progress.
type Stream struct {
	stop  func()
	ended <-chan struct{}
}

// Stop ends the recording. Once it returns no more samples arrive, and
// calling it again does nothing.
func (s *Stream) Stop() { s.stop() }

// Ended closes once no more samples will arrive: after Stop, or once the
// system took the microphone away, as iOS does for a call or when the app
// leaves the screen. The recording still needs its Stop then.
func (s *Stream) Ended() <-chan struct{} { return s.ended }

// RecordOptions configures RecordWAV.
type RecordOptions struct {
	// Rate is the sample rate asked for, in hertz; 0 takes the device's. A device that cannot record at it records at its own, which File reports.
	Rate int
	// Channels is 1 for mono, the default, or 2 for stereo, which a device without it records as mono.
	Channels int
	// Level receives the peak and RMS of the frames written over each LevelInterval, on a goroutine of microphone's, never an engine's: an app that updates its views from it hops with Scope.Post. A level it has not taken yet is replaced by the next, so a slow callback never holds up the recording.
	Level func(Level)
	// LevelInterval is how often Level runs; 0 is a tenth of a second.
	LevelInterval time.Duration
	// PauseOnInterruption keeps a recording the system silenced, as a call does: it writes nothing while Interrupted says so and goes on after. Without it the interruption ends the recording with ErrInterrupted.
	PauseOnInterruption bool
}

// Level is the loudness of the frames a level covers, each in [0, 1].
type Level struct {
	Peak, RMS float32
}

// File is a recording's file: its path in the app's private storage and the format written, the rate the device gave rather than the one asked for, and the duration written so far.
type File struct {
	Path     string
	Rate     int
	Channels int
	Duration time.Duration
}

// Recording is a recording to a file, which belongs to the process rather than to a view.
type Recording struct {
	id      uint64
	level   func(Level)
	started chan struct{}
	ended   chan struct{}
	// levels wakes the goroutine delivering levels, for the one in pending.
	levels   chan struct{}
	stopOnce sync.Once
	// stopFn stops a local in-process recording; nil sends a stop command over internal/driver.
	stopFn func()

	mu          sync.Mutex
	file        File
	frames      int64
	err         error
	interrupted bool
	pending     Level
}

// Stop ends the recording and waits for its file to be complete under its final name; calling it again does nothing. No level starts after it returns.
func (r *Recording) Stop() {
	select {
	case <-r.ended:
		return
	default:
	}
	r.stopOnce.Do(func() {
		if r.stopFn != nil {
			r.stopFn()
		} else {
			send(command{Op: "stop", ID: r.id})
		}
	})
	<-r.ended
}

// Ended closes once the recording is over and its file complete: after Stop, or once it ended by itself, which Err says why.
func (r *Recording) Ended() <-chan struct{} { return r.ended }

// File is the recording's file, its duration so far until Ended closes: as of the last level, or about once a second for a recording without Level.
func (r *Recording) File() File {
	r.mu.Lock()
	defer r.mu.Unlock()
	f := r.file
	if f.Rate > 0 {
		f.Duration = time.Duration(r.frames) * time.Second / time.Duration(f.Rate)
	}
	return f
}

// Err says why the recording ended by itself once Ended closed, such as ErrInterrupted, ErrFull or ErrNoSpace: nil while it runs and after Stop.
func (r *Recording) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

// Interrupted says the system silences the recording now, which writes nothing meanwhile; only a recording that pauses on interruption runs on through one.
func (r *Recording) Interrupted() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.interrupted
}

// RecordWAV records 16-bit PCM WAV to the file name in the app's recordings directory, private storage under the app's data directory, until it stops or ends by itself. The file goes under a temporary name until then, and its header follows what was written about once a second, so a file whose app was killed plays up to its last second; the next RecordWAV repairs it under its name and Earlier reports it, in that process or a later one, until the app takes it. It asks for the microphone as Authorize does and waits for the answer, so call it off the UI. It records directly on macOS, iOS, Windows and Linux; on Android it records in the runtime's foreground service, through a hidden activity and the screen going off, and starts only while an activity of the app is visible, in an app whose nsgo.yaml sets android.recording; it reports ErrUnsupported elsewhere. From Android 13 the service's notification shows only once the user lets the app post notifications, so the first RecordWAV asks for POST_NOTIFICATIONS with RECORD_AUDIO, or alone where RECORD_AUDIO is granted; a user who refuses notifications still records, without a visible notification.
func RecordWAV(name string, o RecordOptions) (*Recording, error) {
	if name == "" || strings.ContainsAny(name, "/\\\x00") || strings.HasPrefix(name, ".") {
		return nil, &fs.PathError{Op: "record", Path: name, Err: fs.ErrInvalid}
	}
	if o.Rate < 0 || o.Channels < 0 || o.Channels > 2 {
		return nil, errors.New("microphone: a WAV recording takes a rate of 0 or more and one or two channels")
	}
	if !reachable() {
		if hasLocalRecorder {
			return startLocalWAV(name, o)
		}
		return nil, ErrUnsupported
	}
	listen()
	r := &Recording{level: o.Level, started: make(chan struct{}), ended: make(chan struct{})}
	cmd := command{Op: "start", Name: name, Rate: o.Rate, Channels: max(o.Channels, 1), Pause: o.PauseOnInterruption}
	if o.Level != nil {
		interval := o.LevelInterval
		if interval <= 0 {
			interval = 100 * time.Millisecond
		}
		cmd.Level = max(interval.Milliseconds(), minLevelInterval)
		r.levels = make(chan struct{}, 1)
	}
	recorder.mu.Lock()
	recorder.nextID++
	r.id = recorder.nextID
	cmd.ID = r.id
	recorder.recordings[r.id] = r
	recorder.mu.Unlock()
	send(cmd)
	select {
	case <-r.started:
	case <-r.ended:
		select {
		case <-r.started:
		default:
			return nil, r.Err()
		}
	}
	if r.levels != nil {
		go r.deliver()
	}
	return r, nil
}

// minLevelInterval is the shortest level interval in milliseconds, about one read of the recorder's.
const minLevelInterval = 20

// Earlier returns, once each, the recordings of earlier runs that ended unseen: ErrAbandoned's, whose Go process ended while the app's lived on, and ErrPartial's, whose app was killed, which a RecordWAV repaired. Their Ended is closed. The runtime keeps each one's record until Earlier returns it, so a recording a process heard of but did not take, as it ended first, is reported to the next process; one whose file the app deleted is not.
func Earlier() []*Recording {
	if !reachable() {
		if hasLocalRecorder {
			return localEarlier()
		}
		return nil
	}
	listen()
	recorder.mu.Lock()
	earlier := recorder.earlier
	recorder.earlier = nil
	recorder.mu.Unlock()
	for _, r := range earlier {
		send(command{Op: "taken", Path: r.file.Path})
	}
	return earlier
}

// deliver hands the recording's levels to its callback until it ends.
func (r *Recording) deliver() {
	for {
		select {
		case <-r.levels:
		case <-r.ended:
			return
		}
		r.mu.Lock()
		l := r.pending
		r.mu.Unlock()
		select {
		case <-r.ended:
			return
		default:
			r.level(l)
		}
	}
}
