package core

import (
	"encoding/json"
	"errors"
	"fmt"
	goruntime "runtime"
	"sync"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Android dataSync foreground service keep-alive: the core's part of
// mobile.StartDataSync, mobile.DataSyncSupported, mobile.DataSync,
// mobile.DataSyncOptions and mobile.DataSyncNotification, which apps use.

var (
	// ErrDataSyncNotAllowed reports that Android refused to start the dataSync
	// foreground service outside an eligible foreground state
	// (ForegroundServiceStartNotAllowedException). It is the core's part of
	// mobile.ErrDataSyncNotAllowed.
	ErrDataSyncNotAllowed = errors.New("data sync: foreground service start not allowed")

	// ErrDataSyncUnavailable reports that the Android app does not declare
	// DataSyncService (nsgo.yaml's android.data_sync: true) or the runtime does
	// not support it. It is the core's part of mobile.ErrDataSyncUnavailable.
	ErrDataSyncUnavailable = driver.ErrDataSyncUnavailable

	// ErrDataSyncTimedOut reports that Android ended the dataSync foreground
	// service because its system quota expired (API 35+ Service.onTimeout).
	// It is the core's part of mobile.ErrDataSyncTimedOut.
	ErrDataSyncTimedOut = errors.New("data sync: foreground service time limit expired")
)

// DataSyncNotification describes the ongoing notification shown by Android's
// dataSync foreground service. It is the core's part of
// mobile.DataSyncNotification.
type DataSyncNotification struct {
	// Title is the notification's content title. When empty, the app's name is used.
	Title string
	// Body is the secondary status text shown below Title.
	Body string
	// Current and Total report transfer progress mapped to Android's native
	// Notification.Builder.setProgress. When Total > 0 and Indeterminate is false,
	// determinate progress (Current/Total) is shown. When Indeterminate is true
	// or Total < 0, an indeterminate progress bar is shown. When Total == 0 and
	// Indeterminate is false, no progress bar is shown.
	Current int64
	Total   int64
	// Indeterminate forces an indeterminate progress bar regardless of Total.
	Indeterminate bool
}

func (n DataSyncNotification) hasContent() bool {
	return n.Title != "" || n.Body != "" || n.Current != 0 || n.Total != 0 || n.Indeterminate
}

// DataSyncOptions configures a dataSync foreground service session. It is the
// core's part of mobile.DataSyncOptions.
type DataSyncOptions struct {
	// Notification sets the initial ongoing foreground notification.
	Notification DataSyncNotification
	// WakeLock requests a PowerManager PARTIAL_WAKE_LOCK while active so the CPU
	// stays awake during screen-off transfers. Requires android.permission.WAKE_LOCK
	// in nsgo.yaml's android.permissions; ignored gracefully when undeclared.
	WakeLock bool
	// OnTimeout runs on its own goroutine if Android invokes Service.onTimeout
	// (API 35+ dataSync quota) so the app can flush partial files or checkpoints
	// before the runtime stops the foreground service cleanly.
	OnTimeout func()
}

// DataSync is a handle to a reference-counted Android dataSync foreground
// service session. Multiple concurrent transfers may each acquire a handle and
// share one foreground service and notification; the service stops once every
// active handle has called Stop. All methods are safe on a nil handle and from
// any goroutine. It is the core's part of mobile.DataSync.
type DataSync struct {
	id        uint64
	sessionID uint64
	noop      bool
	wakeLock  bool
	onTimeout func()

	ended   chan struct{}
	endOnce sync.Once

	mu     sync.Mutex
	active bool
	err    error
}

var closedDataSyncChan = func() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}()

// Update updates the ongoing foreground service notification. It is a safe
// no-op on a nil handle, a non-Android no-op handle, or an ended handle.
func (d *DataSync) Update(n DataSyncNotification) {
	if d == nil || d.noop {
		return
	}
	var cmd *dataSyncCommand
	dsManager.mu.Lock()
	if dsManager.state == dsRunning && dsManager.sessionID == d.sessionID && dsManager.handles[d.id] == d {
		dsManager.lastNotification = n
		c := dsNotificationCommand("update", d.sessionID, n, dsManager.wakeLock)
		cmd = &c
	}
	dsManager.mu.Unlock()
	if cmd != nil {
		_ = sendDataSyncCommand(*cmd)
	}
}

// Stop releases this handle's reference to the foreground service. When the
// last active handle stops, Android stops DataSyncService and removes its
// notification. Calling Stop more than once or on a nil handle is safe.
func (d *DataSync) Stop() {
	if d == nil {
		return
	}
	if d.noop {
		d.endOnce.Do(func() { close(d.ended) })
		return
	}
	var cmd *dataSyncCommand
	dsManager.mu.Lock()
	if dsManager.handles[d.id] == d {
		delete(dsManager.handles, d.id)
		d.mu.Lock()
		d.active = false
		d.mu.Unlock()
		if len(dsManager.handles) == 0 && dsManager.state != dsIdle {
			sid := dsManager.sessionID
			dsManager.state = dsIdle
			dsManager.wakeLock = false
			dsManager.start = nil
			c := dataSyncCommand{Op: "stop", ID: sid}
			cmd = &c
		} else if dsManager.state == dsRunning {
			wantWake := dsManager.anyWakeLockLocked()
			if wantWake != dsManager.wakeLock {
				dsManager.wakeLock = wantWake
				c := dsNotificationCommand("update", dsManager.sessionID, dsManager.lastNotification, wantWake)
				cmd = &c
			}
		}
	}
	dsManager.mu.Unlock()
	d.endOnce.Do(func() { close(d.ended) })
	if cmd != nil {
		_ = sendDataSyncCommand(*cmd)
	}
}

// Ended returns a channel closed once this handle ends, either via Stop or
// when Android stops or times out the foreground service.
func (d *DataSync) Ended() <-chan struct{} {
	if d == nil {
		return closedDataSyncChan
	}
	return d.ended
}

// Err returns why the session ended unexpectedly (such as ErrDataSyncTimedOut),
// or nil while running or after a normal Stop.
func (d *DataSync) Err() error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.err
}

// Active reports whether this handle is backed by a currently running Android
// dataSync foreground service. It is false on non-Android no-op handles, on nil
// handles, and after the session ends.
func (d *DataSync) Active() bool {
	if d == nil || d.noop {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.active
}

type dataSyncState uint8

const (
	dsIdle dataSyncState = iota
	dsStarting
	dsRunning
)

type dataSyncStart struct {
	done chan struct{}
	err  error
}

type dataSyncCommand struct {
	Op            string `json:"op"`
	ID            uint64 `json:"id"`
	Title         string `json:"title,omitempty"`
	Body          string `json:"body,omitempty"`
	Current       int64  `json:"current,omitempty"`
	Total         int64  `json:"total,omitempty"`
	Indeterminate bool   `json:"indeterminate,omitempty"`
	WakeLock      bool   `json:"wakeLock,omitempty"`
}

type dataSyncReport struct {
	Type      string `json:"type"` // state, started, timeout or ended
	ID        uint64 `json:"id"`
	Supported bool   `json:"supported"`
	Error     string `json:"error"`
	Detail    string `json:"detail"`
}

type dataSyncManager struct {
	once sync.Once

	mu               sync.Mutex
	supported        bool
	state            dataSyncState
	sessionID        uint64
	nextHandle       uint64
	start            *dataSyncStart
	handles          map[uint64]*DataSync
	lastNotification DataSyncNotification
	wakeLock         bool
}

var dsManager dataSyncManager

func (m *dataSyncManager) anyWakeLockLocked() bool {
	for _, h := range m.handles {
		if h.wakeLock {
			return true
		}
	}
	return false
}

func dataSyncListen() {
	dsManager.once.Do(func() {
		dsManager.mu.Lock()
		dsManager.handles = map[uint64]*DataSync{}
		dsManager.mu.Unlock()
		driver.HearDataSync(hearDataSync)
	})
}

func dataSyncReachable() bool {
	return goruntime.GOOS == "android" || driver.DataSyncConnected()
}

func dsNotificationCommand(op string, id uint64, n DataSyncNotification, wakeLock bool) dataSyncCommand {
	return dataSyncCommand{
		Op:            op,
		ID:            id,
		Title:         n.Title,
		Body:          n.Body,
		Current:       n.Current,
		Total:         n.Total,
		Indeterminate: n.Indeterminate,
		WakeLock:      wakeLock,
	}
}

func sendDataSyncCommand(c dataSyncCommand) error {
	msg, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return driver.SendDataSync(msg)
}

// DataSyncSupported is the core's part of mobile.DataSyncSupported: it reports
// whether Android's dataSync foreground service is declared in the app's
// manifest and supported by the runtime.
func DataSyncSupported() bool {
	dataSyncListen()
	if !dataSyncReachable() {
		return false
	}
	dsManager.mu.Lock()
	defer dsManager.mu.Unlock()
	return dsManager.supported
}

// StartDataSync is the core's part of mobile.StartDataSync: it acquires a
// reference-counted Android dataSync foreground service session. On non-Android
// platforms (when no test host is connected) it returns a safe no-op handle and
// a nil error.
func StartDataSync(opts DataSyncOptions) (*DataSync, error) {
	dataSyncListen()
	if !dataSyncReachable() {
		return &DataSync{noop: true, ended: make(chan struct{})}, nil
	}

	dsManager.mu.Lock()
	dsManager.nextHandle++
	hid := dsManager.nextHandle

	switch dsManager.state {
	case dsRunning:
		h := &DataSync{
			id:        hid,
			sessionID: dsManager.sessionID,
			wakeLock:  opts.WakeLock,
			onTimeout: opts.OnTimeout,
			ended:     make(chan struct{}),
			active:    true,
		}
		dsManager.handles[hid] = h
		needUpdate := false
		if opts.Notification.hasContent() {
			dsManager.lastNotification = opts.Notification
			needUpdate = true
		}
		if opts.WakeLock && !dsManager.wakeLock {
			dsManager.wakeLock = true
			needUpdate = true
		}
		var cmd dataSyncCommand
		if needUpdate {
			cmd = dsNotificationCommand("update", dsManager.sessionID, dsManager.lastNotification, dsManager.wakeLock)
		}
		dsManager.mu.Unlock()
		if needUpdate {
			_ = sendDataSyncCommand(cmd)
		}
		return h, nil

	case dsStarting:
		sid := dsManager.sessionID
		attempt := dsManager.start
		h := &DataSync{
			id:        hid,
			sessionID: sid,
			wakeLock:  opts.WakeLock,
			onTimeout: opts.OnTimeout,
			ended:     make(chan struct{}),
		}
		dsManager.handles[hid] = h
		dsManager.mu.Unlock()

		<-attempt.done
		if attempt.err != nil {
			return nil, attempt.err
		}
		var cmd *dataSyncCommand
		dsManager.mu.Lock()
		if dsManager.state == dsRunning && dsManager.sessionID == sid && dsManager.handles[hid] == h {
			needUpdate := false
			if opts.Notification.hasContent() {
				dsManager.lastNotification = opts.Notification
				needUpdate = true
			}
			if opts.WakeLock && !dsManager.wakeLock {
				dsManager.wakeLock = true
				needUpdate = true
			}
			if needUpdate {
				c := dsNotificationCommand("update", sid, dsManager.lastNotification, dsManager.wakeLock)
				cmd = &c
			}
		}
		dsManager.mu.Unlock()
		if cmd != nil {
			_ = sendDataSyncCommand(*cmd)
		}
		return h, nil

	default: // dsIdle
		dsManager.sessionID++
		sid := dsManager.sessionID
		attempt := &dataSyncStart{done: make(chan struct{})}
		h := &DataSync{
			id:        hid,
			sessionID: sid,
			wakeLock:  opts.WakeLock,
			onTimeout: opts.OnTimeout,
			ended:     make(chan struct{}),
		}
		dsManager.state = dsStarting
		dsManager.start = attempt
		dsManager.lastNotification = opts.Notification
		dsManager.wakeLock = opts.WakeLock
		dsManager.handles[hid] = h
		cmd := dsNotificationCommand("start", sid, opts.Notification, opts.WakeLock)
		dsManager.mu.Unlock()

		if err := sendDataSyncCommand(cmd); err != nil {
			failDataSyncStart(sid, attempt, err)
			return nil, err
		}
		<-attempt.done
		if attempt.err != nil {
			return nil, attempt.err
		}
		return h, nil
	}
}

func failDataSyncStart(sid uint64, attempt *dataSyncStart, err error) {
	var orphaned []*DataSync
	dsManager.mu.Lock()
	if dsManager.start == attempt && dsManager.sessionID == sid {
		attempt.err = err
		dsManager.state = dsIdle
		dsManager.start = nil
		dsManager.wakeLock = false
		for id, h := range dsManager.handles {
			delete(dsManager.handles, id)
			orphaned = append(orphaned, h)
		}
		close(attempt.done)
	}
	dsManager.mu.Unlock()
	for _, h := range orphaned {
		h.mu.Lock()
		h.active = false
		h.err = err
		h.mu.Unlock()
		h.endOnce.Do(func() { close(h.ended) })
	}
}

func hearDataSync(data []byte) {
	// A nil payload signals host disconnect (ConnectDataSync(nil)).
	if data == nil {
		var attempt *dataSyncStart
		var active []*DataSync
		dsManager.mu.Lock()
		dsManager.supported = false
		dsManager.state = dsIdle
		dsManager.wakeLock = false
		attempt = dsManager.start
		dsManager.start = nil
		for id, h := range dsManager.handles {
			delete(dsManager.handles, id)
			active = append(active, h)
		}
		dsManager.mu.Unlock()
		if attempt != nil {
			attempt.err = ErrDataSyncUnavailable
			close(attempt.done)
		}
		for _, h := range active {
			h.mu.Lock()
			h.active = false
			h.err = ErrDataSyncUnavailable
			h.mu.Unlock()
			h.endOnce.Do(func() { close(h.ended) })
		}
		return
	}

	var m dataSyncReport
	if json.Unmarshal(data, &m) != nil {
		return
	}

	switch m.Type {
	case "state":
		dsManager.mu.Lock()
		dsManager.supported = m.Supported
		dsManager.mu.Unlock()

	case "started":
		var attempt *dataSyncStart
		dsManager.mu.Lock()
		if dsManager.state == dsStarting && dsManager.sessionID == m.ID {
			dsManager.state = dsRunning
			attempt = dsManager.start
			dsManager.start = nil
			for _, h := range dsManager.handles {
				h.mu.Lock()
				h.active = true
				h.mu.Unlock()
			}
		}
		dsManager.mu.Unlock()
		if attempt != nil {
			close(attempt.done)
		}

	case "timeout":
		var handles []*DataSync
		var attempt *dataSyncStart
		dsManager.mu.Lock()
		if dsManager.sessionID == m.ID && dsManager.state != dsIdle {
			dsManager.state = dsIdle
			dsManager.wakeLock = false
			attempt = dsManager.start
			dsManager.start = nil
			for id, h := range dsManager.handles {
				delete(dsManager.handles, id)
				handles = append(handles, h)
			}
		}
		dsManager.mu.Unlock()
		if attempt != nil {
			attempt.err = ErrDataSyncTimedOut
			close(attempt.done)
		}
		if len(handles) == 0 {
			_ = sendDataSyncCommand(dataSyncCommand{Op: "stop", ID: m.ID})
			return
		}
		for _, h := range handles {
			h.mu.Lock()
			h.active = false
			h.err = ErrDataSyncTimedOut
			h.mu.Unlock()
		}
		go func(sid uint64, list []*DataSync) {
			for _, h := range list {
				if h.onTimeout != nil {
					h.onTimeout()
				}
				h.endOnce.Do(func() { close(h.ended) })
			}
			_ = sendDataSyncCommand(dataSyncCommand{Op: "stop", ID: sid})
		}(m.ID, handles)

	case "ended":
		err := dataSyncReportError(m)
		var attempt *dataSyncStart
		var handles []*DataSync
		dsManager.mu.Lock()
		if dsManager.sessionID == m.ID && dsManager.state != dsIdle {
			dsManager.state = dsIdle
			dsManager.wakeLock = false
			attempt = dsManager.start
			dsManager.start = nil
			for id, h := range dsManager.handles {
				delete(dsManager.handles, id)
				handles = append(handles, h)
			}
		}
		dsManager.mu.Unlock()
		if attempt != nil {
			if err == nil {
				err = ErrDataSyncUnavailable
			}
			attempt.err = err
			close(attempt.done)
		}
		for _, h := range handles {
			h.mu.Lock()
			h.active = false
			if h.err == nil {
				h.err = err
			}
			h.mu.Unlock()
			h.endOnce.Do(func() { close(h.ended) })
		}
	}
}

func dataSyncReportError(m dataSyncReport) error {
	var base error
	switch m.Error {
	case "":
		return nil
	case "not_allowed":
		base = ErrDataSyncNotAllowed
	case "unavailable":
		base = ErrDataSyncUnavailable
	case "timeout":
		base = ErrDataSyncTimedOut
	case "stopped":
		if m.Detail != "" {
			return errors.New("data sync: " + m.Detail)
		}
		return errors.New("data sync: foreground service stopped")
	case "busy":
		return errors.New("data sync: foreground service is owned by another engine")
	default:
		if m.Detail != "" {
			return errors.New("data sync: " + m.Detail)
		}
		return errors.New("data sync: foreground service failed")
	}
	if m.Detail != "" {
		return fmt.Errorf("%w: %s", base, m.Detail)
	}
	return base
}
