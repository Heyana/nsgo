package windows

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const (
	toastScheduleVersion = 1
	toastScheduleName    = "scheduled-notifications.json"
	defaultChainDelay    = 2 * time.Second
)

type toastScheduleFile struct {
	Version int              `json:"version"`
	Items   []persistedToast `json:"items"`
}

type persistedToast struct {
	ID      string                    `json:"id"`
	Title   string                    `json:"title"`
	Body    string                    `json:"body,omitempty"`
	Silent  bool                      `json:"silent,omitempty"`
	At      time.Time                 `json:"at"`
	Repeat  driver.NotificationRepeat `json:"repeat"`
	Hour    int                       `json:"hour"`
	Minute  int                       `json:"minute"`
	Second  int                       `json:"second"`
	Weekday time.Weekday              `json:"weekday"`

	seq uint64
}

func (p persistedToast) notification() driver.Notification {
	return driver.Notification{
		ID:     p.ID,
		Title:  p.Title,
		Body:   p.Body,
		Silent: p.Silent,
		At:     p.At,
		Repeat: p.Repeat,
	}
}

// toastRepeatManager chains Windows' single-shot ScheduledToastNotification into daily and weekly repeating schedules. The upcoming occurrence is always placed on Windows' ToastNotifier so it still fires after the app exits; while the app runs, a timer queues the next occurrence right after the current one fires, and Rearm restores persisted repeats on the next launch.
type toastRepeatManager struct {
	mu           sync.Mutex
	path         string
	entries      map[string]*persistedToast
	sched        driver.NotificationScheduler
	scheduleOnce func(driver.Notification) (time.Time, bool)
	unschedule   func(id string)
	dispatch     func(func())
	chainDelay   time.Duration
}

func newToastRepeatManager(path string, scheduleOnce func(driver.Notification) (time.Time, bool), unschedule func(id string), dispatch func(func())) *toastRepeatManager {
	m := &toastRepeatManager{
		path:         path,
		entries:      map[string]*persistedToast{},
		scheduleOnce: scheduleOnce,
		unschedule:   unschedule,
		dispatch:     dispatch,
		chainDelay:   defaultChainDelay,
	}
	m.sched.Fire = m.onChain
	return m
}

func (m *toastRepeatManager) now() time.Time {
	if m.sched.Now != nil {
		return m.sched.Now()
	}
	return time.Now()
}

// Schedule schedules n on Windows' ToastNotifier, replacing any existing notification with the same ID. For daily and weekly repeats, it also persists the schedule and arms the in-app chain timer.
func (m *toastRepeatManager) Schedule(n driver.Notification) bool {
	if n.ID == "" || n.At.IsZero() {
		return false
	}
	m.mu.Lock()
	_, had := m.entries[n.ID]
	if had {
		delete(m.entries, n.ID)
	}
	m.sched.Cancel(n.ID)
	if had && n.Repeat == driver.NotificationOnce {
		m.saveLocked()
	}
	m.mu.Unlock()

	if m.unschedule != nil {
		m.unschedule(n.ID)
	}
	if m.scheduleOnce == nil {
		return false
	}
	scheduledAt, ok := m.scheduleOnce(n)
	if !ok {
		if had {
			m.mu.Lock()
			m.saveLocked()
			m.mu.Unlock()
		}
		return false
	}
	if n.Repeat == driver.NotificationOnce {
		return true
	}

	h, min, sec := n.At.Clock()
	wd := n.At.Weekday()
	m.mu.Lock()
	entry := &persistedToast{
		ID:      n.ID,
		Title:   n.Title,
		Body:    n.Body,
		Silent:  n.Silent,
		At:      scheduledAt,
		Repeat:  n.Repeat,
		Hour:    h,
		Minute:  min,
		Second:  sec,
		Weekday: wd,
	}
	seq, armed := m.sched.Schedule(driver.Notification{
		ID:     n.ID,
		At:     scheduledAt.Add(m.chainDelay),
		Repeat: driver.NotificationOnce,
	})
	if armed {
		entry.seq = seq
		m.entries[n.ID] = entry
		m.saveLocked()
	}
	m.mu.Unlock()
	return true
}

// Cancel removes id (or every repeating schedule when id is empty) from memory and persisted storage, stopping its chain timer.
func (m *toastRepeatManager) Cancel(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sched.Cancel(id)
	if id == "" {
		if len(m.entries) > 0 {
			clear(m.entries)
			m.saveLocked()
		}
		return
	}
	if _, ok := m.entries[id]; ok {
		delete(m.entries, id)
		m.saveLocked()
	}
}

// Stop stops every in-process repeat timer without touching Windows' scheduled toasts or the persisted schedule file, so upcoming toasts still fire after the app exits and the next launch rearms future repeats.
func (m *toastRepeatManager) Stop() {
	m.mu.Lock()
	m.sched.Close()
	m.mu.Unlock()
}

// Rearm loads persisted repeating schedules from disk, advances any whose scheduled occurrence passed while the app was closed, places each upcoming occurrence on Windows' ToastNotifier and arms its chain timer. If no schedule file exists or it has no items, Rearm returns without touching the notifier.
func (m *toastRepeatManager) Rearm() {
	items := m.load()
	if len(items) == 0 {
		return
	}
	now := m.now()
	var updated []*persistedToast
	for _, item := range items {
		if item.ID == "" || (item.Repeat != driver.NotificationDaily && item.Repeat != driver.NotificationWeekly) {
			continue
		}
		if !item.At.After(now) {
			item.At = driver.NextClockRepeat(item.Hour, item.Minute, item.Second, item.Weekday, item.Repeat, now)
		}
		if m.unschedule != nil {
			m.unschedule(item.ID)
		}
		cp := item
		if m.scheduleOnce != nil {
			if scheduledAt, ok := m.scheduleOnce(cp.notification()); ok {
				cp.At = scheduledAt
			}
		}
		updated = append(updated, &cp)
	}
	m.mu.Lock()
	for _, entry := range updated {
		seq, armed := m.sched.Schedule(driver.Notification{
			ID:     entry.ID,
			At:     entry.At.Add(m.chainDelay),
			Repeat: driver.NotificationOnce,
		})
		if armed {
			entry.seq = seq
			m.entries[entry.ID] = entry
		}
	}
	m.saveLocked()
	m.mu.Unlock()
}

func (m *toastRepeatManager) onChain(fired driver.Notification, _ time.Time, seq uint64) {
	m.mu.Lock()
	entry := m.entries[fired.ID]
	if entry == nil || entry.seq != seq || m.sched.Canceled(fired.ID, seq) {
		m.mu.Unlock()
		return
	}
	now := m.now()
	nextAt := driver.NextClockRepeat(entry.Hour, entry.Minute, entry.Second, entry.Weekday, entry.Repeat, now)
	entry.At = nextAt
	nextNotif := entry.notification()
	newSeq, armed := m.sched.Schedule(driver.Notification{
		ID:     entry.ID,
		At:     nextAt.Add(m.chainDelay),
		Repeat: driver.NotificationOnce,
	})
	if !armed {
		m.mu.Unlock()
		return
	}
	entry.seq = newSeq
	m.saveLocked()
	m.mu.Unlock()

	run := func() {
		if m.isStale(nextNotif.ID, newSeq) || m.scheduleOnce == nil {
			return
		}
		actualAt, ok := m.scheduleOnce(nextNotif)
		if !ok || actualAt.Equal(nextNotif.At) {
			return
		}
		m.mu.Lock()
		if cur := m.entries[nextNotif.ID]; cur != nil && cur.seq == newSeq {
			cur.At = actualAt
			if reSeq, ok := m.sched.Schedule(driver.Notification{
				ID:     cur.ID,
				At:     actualAt.Add(m.chainDelay),
				Repeat: driver.NotificationOnce,
			}); ok {
				cur.seq = reSeq
			}
			m.saveLocked()
		}
		m.mu.Unlock()
	}
	if m.dispatch != nil {
		m.dispatch(run)
	} else {
		run()
	}
}

func (m *toastRepeatManager) isStale(id string, seq uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.entries[id]
	return e == nil || e.seq != seq || m.sched.Canceled(id, seq)
}

func (m *toastRepeatManager) pending() []persistedToast {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshotLocked()
}

func (m *toastRepeatManager) snapshotLocked() []persistedToast {
	out := make([]persistedToast, 0, len(m.entries))
	for _, e := range m.entries {
		out = append(out, *e)
	}
	slices.SortFunc(out, func(a, b persistedToast) int {
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

func (m *toastRepeatManager) load() []persistedToast {
	if m.path == "" {
		return nil
	}
	data, err := os.ReadFile(m.path)
	if err != nil {
		return nil
	}
	var f toastScheduleFile
	if err := json.Unmarshal(data, &f); err != nil || f.Version != toastScheduleVersion {
		return nil
	}
	return f.Items
}

func (m *toastRepeatManager) saveLocked() {
	if m.path == "" {
		return
	}
	items := m.snapshotLocked()
	if len(items) == 0 {
		if err := os.Remove(m.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return
		}
		return
	}
	data, err := json.MarshalIndent(toastScheduleFile{Version: toastScheduleVersion, Items: items}, "", "  ")
	if err != nil {
		return
	}
	data = append(data, '\n')
	dir := filepath.Dir(m.path)
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	tmp := m.path + ".tmp"
	if os.WriteFile(tmp, data, 0o600) != nil {
		return
	}
	_ = os.Rename(tmp, m.path)
}
