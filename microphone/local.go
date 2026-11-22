//go:build darwin || windows || (linux && !android)

package microphone

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const (
	hasLocalRecorder = true
	partSuffix       = ".part"
	journalName      = ".earlier"
	diskSpaceReserve = 8 << 20 // 8 MiB, matching Android's RecordingService.RESERVE
)

// captureSession is a live platform PCM input opened for RecordWAV.
type captureSession struct {
	rate     int
	channels int
	stop     func()
	resume   func()
}

type localRecorderHooks struct {
	authorize func() error
	open      func(rate, channels int, onSamples func([]int16), onInterrupt func(bool), onError func(error)) (*captureSession, error)
	freeSpace func(dir string) int64
	maxData   uint64
	reserve   int64
}

func defaultLocalHooks() localRecorderHooks {
	return localRecorderHooks{
		authorize: authorizeSystem,
		open:      openWAVCapture,
		freeSpace: availableDiskSpace,
		maxData:   uint64(driver.MaxWAVDataBytes),
		reserve:   diskSpaceReserve,
	}
}

var localRec struct {
	mu     sync.Mutex
	active *localRecording
	hooks  *localRecorderHooks // non-nil only in tests
}

func currentLocalHooks() localRecorderHooks {
	localRec.mu.Lock()
	h := localRec.hooks
	localRec.mu.Unlock()
	if h != nil {
		return *h
	}
	return defaultLocalHooks()
}

type localRecording struct {
	rec    *Recording
	dir    string
	target string
	part   string
	file   *os.File

	rate        int
	channels    int
	frameBytes  int
	levelFrames int64
	pause       bool
	maxData     uint64
	reserve     int64
	freeSpace   func(string) int64

	sess   *captureSession
	bufCh  chan []int16
	stopCh chan struct{}

	closing         atomic.Bool
	silenced        atomic.Bool
	stopCaptureOnce sync.Once

	reasonMu  sync.Mutex
	hasReason bool
	cause     error

	// Writer-goroutine-owned state.
	dataBytes uint64
	checked   uint64
	heard     int64
	peak      int
	squares   float64
	pcmBuf    []byte
}

func startLocalWAV(name string, o RecordOptions) (*Recording, error) {
	hooks := currentLocalHooks()
	if err := hooks.authorize(); err != nil {
		return nil, err
	}

	dir, err := recordingsDir()
	if err != nil {
		return nil, err
	}

	lr := &localRecording{
		dir:       dir,
		pause:     o.PauseOnInterruption,
		maxData:   hooks.maxData,
		reserve:   hooks.reserve,
		freeSpace: hooks.freeSpace,
		bufCh:     make(chan []int16, 64),
		stopCh:    make(chan struct{}),
	}

	localRec.mu.Lock()
	if localRec.active != nil {
		localRec.mu.Unlock()
		return nil, ErrBusy
	}
	localRec.active = lr
	localRec.mu.Unlock()

	releaseActive := func() {
		localRec.mu.Lock()
		if localRec.active == lr {
			localRec.active = nil
		}
		localRec.mu.Unlock()
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		releaseActive()
		return nil, fmt.Errorf("microphone: cannot make the recordings directory: %w", err)
	}

	repairPartials(dir)

	target := filepath.Join(dir, name)
	part := filepath.Join(dir, "."+name+partSuffix)
	if _, err := os.Stat(target); err == nil {
		releaseActive()
		return nil, &fs.PathError{Op: "record", Path: target, Err: fs.ErrExist}
	}
	if _, err := os.Stat(part); err == nil {
		releaseActive()
		return nil, &fs.PathError{Op: "record", Path: target, Err: fs.ErrExist}
	}
	if free := hooks.freeSpace(dir); free >= 0 && free < hooks.reserve {
		releaseActive()
		return nil, ErrNoSpace
	}

	f, err := os.OpenFile(part, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		releaseActive()
		if errors.Is(err, fs.ErrExist) {
			return nil, &fs.PathError{Op: "record", Path: target, Err: fs.ErrExist}
		}
		return nil, fmt.Errorf("microphone: %w", err)
	}

	r := &Recording{
		level:   o.Level,
		started: make(chan struct{}),
		ended:   make(chan struct{}),
	}
	if o.Level != nil {
		r.levels = make(chan struct{}, 1)
	}
	lr.rec = r
	lr.target = target
	lr.part = part
	lr.file = f

	onSamples := func(samples []int16) {
		if lr.closing.Load() || len(samples) == 0 {
			return
		}
		if lr.pause && lr.silenced.Load() {
			return
		}
		select {
		case lr.bufCh <- samples:
		default:
		}
	}
	onInterrupt := func(silenced bool) {
		if lr.closing.Load() {
			return
		}
		if !lr.pause {
			if silenced {
				go lr.finish(ErrInterrupted)
			}
			return
		}
		if prev := lr.silenced.Swap(silenced); prev != silenced {
			r.mu.Lock()
			r.interrupted = silenced
			r.mu.Unlock()
			if !silenced && lr.sess != nil && lr.sess.resume != nil {
				lr.sess.resume()
			}
		}
	}
	onError := func(err error) {
		if lr.closing.Load() || err == nil {
			return
		}
		go lr.finish(err)
	}

	sess, err := hooks.open(o.Rate, max(o.Channels, 1), onSamples, onInterrupt, onError)
	if err != nil {
		f.Close()
		os.Remove(part)
		releaseActive()
		return nil, err
	}
	lr.sess = sess
	lr.rate = sess.rate
	lr.channels = sess.channels
	lr.frameBytes = sess.channels * 2

	if o.Level != nil {
		interval := o.LevelInterval
		if interval <= 0 {
			interval = 100 * time.Millisecond
		}
		ms := max(interval.Milliseconds(), minLevelInterval)
		lr.levelFrames = max(int64(1), int64(lr.rate)*ms/1000)
	}

	hdr := driver.WAVHeader(lr.rate, lr.channels, 0)
	if _, err := f.WriteAt(hdr[:], 0); err != nil {
		lr.closing.Store(true)
		sess.stop()
		f.Close()
		os.Remove(part)
		releaseActive()
		if isNoSpaceError(err) {
			return nil, ErrNoSpace
		}
		return nil, fmt.Errorf("microphone: %w", err)
	}

	r.file = File{Path: target, Rate: lr.rate, Channels: lr.channels}
	r.stopFn = func() { lr.finish(nil) }
	close(r.started)

	go lr.writeLoop(releaseActive)
	if r.levels != nil {
		go r.deliver()
	}
	return r, nil
}

func (lr *localRecording) setReason(err error) {
	lr.reasonMu.Lock()
	if !lr.hasReason {
		lr.hasReason = true
		lr.cause = err
	}
	lr.reasonMu.Unlock()
}

func (lr *localRecording) reason() error {
	lr.reasonMu.Lock()
	defer lr.reasonMu.Unlock()
	return lr.cause
}

func (lr *localRecording) stopCapture() {
	lr.stopCaptureOnce.Do(func() {
		lr.closing.Store(true)
		if lr.sess != nil && lr.sess.stop != nil {
			lr.sess.stop()
		}
		close(lr.stopCh)
	})
}

func (lr *localRecording) finish(cause error) {
	lr.setReason(cause)
	lr.stopCapture()
}

func (lr *localRecording) writeLoop(releaseActive func()) {
	for {
		select {
		case s := <-lr.bufCh:
			if err := lr.writeSamples(s); err != nil {
				lr.setReason(err)
				goto finalize
			}
		case <-lr.stopCh:
			if lr.reason() == nil {
				for {
					select {
					case s := <-lr.bufCh:
						if err := lr.writeSamples(s); err != nil {
							lr.setReason(err)
							goto finalize
						}
					default:
						goto finalize
					}
				}
			}
			goto finalize
		}
	}

finalize:
	lr.stopCapture()

	var finalErr error
	hdr := driver.WAVHeader(lr.rate, lr.channels, uint32(lr.dataBytes))
	if _, err := lr.file.WriteAt(hdr[:], 0); err != nil {
		finalErr = err
	} else if err := lr.file.Sync(); err != nil {
		finalErr = err
	}
	if err := lr.file.Close(); err != nil && finalErr == nil {
		finalErr = err
	}

	finalPath := lr.part
	if err := os.Rename(lr.part, lr.target); err == nil {
		finalPath = lr.target
	} else if finalErr == nil {
		finalErr = err
	}

	if cause := lr.reason(); cause != nil {
		finalErr = cause
	} else if finalErr != nil && isNoSpaceError(finalErr) {
		finalErr = ErrNoSpace
	}

	frames := int64(lr.dataBytes) / int64(lr.frameBytes)
	lr.rec.mu.Lock()
	lr.rec.file = File{Path: finalPath, Rate: lr.rate, Channels: lr.channels}
	lr.rec.frames = frames
	lr.rec.err = finalErr
	lr.rec.interrupted = false
	lr.rec.mu.Unlock()

	releaseActive()
	close(lr.rec.ended)
}

func (lr *localRecording) writeSamples(samples []int16) error {
	usable := len(samples) / lr.channels * lr.channels
	if usable == 0 {
		return nil
	}
	samples = samples[:usable]
	n := uint64(usable * 2)
	if lr.dataBytes+n > lr.maxData {
		return ErrFull
	}

	if cap(lr.pcmBuf) < int(n) {
		lr.pcmBuf = make([]byte, n)
	}
	b := lr.pcmBuf[:n]
	for i, s := range samples {
		binary.LittleEndian.PutUint16(b[2*i:], uint16(s))
	}
	if _, err := lr.file.WriteAt(b, int64(driver.WAVHeaderSize)+int64(lr.dataBytes)); err != nil {
		if isNoSpaceError(err) {
			return ErrNoSpace
		}
		if free := lr.freeSpace(lr.dir); free >= 0 && free < lr.reserve {
			return ErrNoSpace
		}
		return fmt.Errorf("microphone: %w", err)
	}
	lr.dataBytes += n
	totalFrames := int64(lr.dataBytes) / int64(lr.frameBytes)

	if lr.levelFrames > 0 {
		for _, s := range samples {
			v := int(s)
			if v < 0 {
				v = -v
			}
			if v > lr.peak {
				lr.peak = v
			}
			lr.squares += float64(s) * float64(s)
		}
		lr.heard += int64(usable / lr.channels)
		if lr.heard >= lr.levelFrames {
			lvl := Level{
				Peak: float32(min(1.0, float64(lr.peak)/32768.0)),
				RMS:  float32(math.Sqrt(lr.squares/float64(lr.heard*int64(lr.channels))) / 32768.0),
			}
			lr.heard = 0
			lr.peak = 0
			lr.squares = 0

			lr.rec.mu.Lock()
			lr.rec.frames = max(lr.rec.frames, totalFrames)
			lr.rec.pending = lvl
			lr.rec.mu.Unlock()
			if lr.rec.levels != nil {
				select {
				case lr.rec.levels <- struct{}{}:
				default:
				}
			}
		}
	}

	// Patch header and check disk space roughly once a second.
	if lr.dataBytes-lr.checked >= uint64(lr.rate*lr.frameBytes) {
		lr.checked = lr.dataBytes
		hdr := driver.WAVHeader(lr.rate, lr.channels, uint32(lr.dataBytes))
		if _, err := lr.file.WriteAt(hdr[:], 0); err != nil {
			if isNoSpaceError(err) {
				return ErrNoSpace
			}
			return fmt.Errorf("microphone: %w", err)
		}
		if free := lr.freeSpace(lr.dir); free >= 0 && free < lr.reserve {
			return ErrNoSpace
		}
		if lr.levelFrames == 0 {
			lr.rec.mu.Lock()
			lr.rec.frames = max(lr.rec.frames, totalFrames)
			lr.rec.mu.Unlock()
		}
	}
	return nil
}

// journalEntry is one persisted terminal record of an earlier partial recording.
type journalEntry struct {
	Path     string `json:"path"`
	Rate     int    `json:"rate"`
	Channels int    `json:"channels"`
	Frames   int64  `json:"frames"`
	Error    string `json:"error"`
}

func repairPartials(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, de := range entries {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		if !strings.HasPrefix(name, ".") || !strings.HasSuffix(name, partSuffix) || len(name) <= 1+len(partSuffix) {
			continue
		}
		partPath := filepath.Join(dir, name)
		f, err := os.OpenFile(partPath, os.O_RDWR, 0)
		if err != nil {
			continue
		}
		var h [driver.WAVHeaderSize]byte
		n, err := f.ReadAt(h[:], 0)
		rate, ch, _, ok := driver.ParseWAVHeader(h[:n])
		if err != nil || n != driver.WAVHeaderSize || !ok {
			f.Close()
			os.Remove(partPath)
			continue
		}
		info, err := f.Stat()
		if err != nil {
			f.Close()
			continue
		}
		frame := int64(ch * 2)
		rawData := max(int64(0), info.Size()-driver.WAVHeaderSize)
		data := min(rawData, int64(driver.MaxWAVDataBytes)) / frame * frame
		_ = f.Truncate(driver.WAVHeaderSize + data)
		repaired := driver.WAVHeader(rate, ch, uint32(data))
		_, _ = f.WriteAt(repaired[:], 0)
		_ = f.Sync()
		_ = f.Close()

		targetName := name[1 : len(name)-len(partSuffix)]
		targetPath := filepath.Join(dir, targetName)
		at := partPath
		if _, err := os.Stat(targetPath); errors.Is(err, fs.ErrNotExist) {
			if os.Rename(partPath, targetPath) == nil {
				at = targetPath
			}
		}
		journalPut(dir, journalEntry{
			Path:     at,
			Rate:     rate,
			Channels: ch,
			Frames:   data / frame,
			Error:    "partial",
		})
	}
}

func journalPut(dir string, entry journalEntry) {
	items := readJournal(dir)
	items = slices.DeleteFunc(items, func(e journalEntry) bool { return e.Path == entry.Path })
	items = append(items, entry)
	_ = writeJournal(dir, items)
}

func readJournal(dir string) []journalEntry {
	data, err := os.ReadFile(filepath.Join(dir, journalName))
	if err != nil {
		return nil
	}
	var items []journalEntry
	if json.Unmarshal(data, &items) != nil {
		return nil
	}
	kept := items[:0]
	dropped := false
	for _, e := range items {
		if _, err := os.Stat(e.Path); err == nil {
			kept = append(kept, e)
		} else {
			dropped = true
		}
	}
	if dropped {
		_ = writeJournal(dir, kept)
	}
	return kept
}

func writeJournal(dir string, items []journalEntry) error {
	path := filepath.Join(dir, journalName)
	if len(items) == 0 {
		err := os.Remove(path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}
	next := path + ".next"
	f, err := os.Create(next)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(next)
		return err
	}
	return os.Rename(next, path)
}

func localEarlier() []*Recording {
	localRec.mu.Lock()
	defer localRec.mu.Unlock()
	dir, err := recordingsDir()
	if err != nil {
		return nil
	}
	items := readJournal(dir)
	if len(items) == 0 {
		return nil
	}
	_ = writeJournal(dir, nil)
	out := make([]*Recording, 0, len(items))
	for _, e := range items {
		r := &Recording{
			started: make(chan struct{}),
			ended:   make(chan struct{}),
			file:    File{Path: e.Path, Rate: e.Rate, Channels: e.Channels},
			frames:  e.Frames,
			err:     reportError(report{Error: e.Error}),
		}
		close(r.started)
		close(r.ended)
		out = append(out, r)
	}
	return out
}

func recordingsDir() (string, error) {
	if dir := driver.RecordingsDir(); dir != "" {
		return dir, nil
	}
	base := defaultAppDataDir()
	if base == "" {
		return "", errors.New("microphone: cannot locate the app's data directory")
	}
	return filepath.Join(base, "default", "recordings"), nil
}

func defaultAppDataDir() string {
	switch runtime.GOOS {
	case "darwin", "ios":
		home, _ := os.UserHomeDir()
		if home == "" {
			home = os.Getenv("HOME")
		}
		if home == "" {
			return ""
		}
		return filepath.Join(home, "Library", "Application Support")
	case "windows":
		if dir := os.Getenv("APPDATA"); dir != "" {
			return dir
		}
		dir, _ := os.UserConfigDir()
		return dir
	default:
		if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
			return dir
		}
		home, _ := os.UserHomeDir()
		if home == "" {
			home = os.Getenv("HOME")
		}
		if home == "" {
			return ""
		}
		return filepath.Join(home, ".local", "share")
	}
}
