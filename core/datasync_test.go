package core

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestDataSyncNoOpWithoutHost(t *testing.T) {
	driver.ConnectDataSync(nil)
	if DataSyncSupported() {
		t.Fatal("DataSyncSupported = true without a host, want false")
	}
	ds, err := StartDataSync(DataSyncOptions{
		Notification: DataSyncNotification{Title: "Syncing", Body: "file.bin", Current: 1, Total: 10},
	})
	if err != nil {
		t.Fatalf("StartDataSync without host returned error: %v", err)
	}
	if ds == nil {
		t.Fatal("StartDataSync without host returned nil handle")
	}
	if ds.Active() {
		t.Error("no-op handle reports Active() == true")
	}
	if ds.Err() != nil {
		t.Errorf("no-op handle Err() = %v, want nil", ds.Err())
	}
	ds.Update(DataSyncNotification{Title: "Updated", Current: 5, Total: 10})
	select {
	case <-ds.Ended():
		t.Error("Ended closed before Stop")
	default:
	}
	ds.Stop()
	ds.Stop() // idempotent
	select {
	case <-ds.Ended():
	default:
		t.Error("Ended not closed after Stop")
	}

	// Nil handle methods are safe no-ops.
	var nilDS *DataSync
	nilDS.Update(DataSyncNotification{Title: "ignored"})
	nilDS.Stop()
	if nilDS.Active() || nilDS.Err() != nil {
		t.Error("nil handle must report inactive and nil error")
	}
	select {
	case <-nilDS.Ended():
	default:
		t.Error("nil handle Ended() must already be closed")
	}
}

func TestDataSyncRefCountUpdateAndWakeLock(t *testing.T) {
	var mu sync.Mutex
	var cmds []dataSyncCommand
	driver.ConnectDataSync(func(msg []byte) error {
		var c dataSyncCommand
		if err := json.Unmarshal(msg, &c); err != nil {
			t.Errorf("unmarshal command: %v", err)
			return err
		}
		mu.Lock()
		cmds = append(cmds, c)
		mu.Unlock()
		if c.Op == "start" {
			go func(id uint64) {
				b, _ := json.Marshal(dataSyncReport{Type: "started", ID: id})
				driver.ReportDataSync(b)
			}(c.ID)
		}
		return nil
	})
	t.Cleanup(func() { driver.ConnectDataSync(nil) })

	stateMsg, _ := json.Marshal(dataSyncReport{Type: "state", Supported: true})
	driver.ReportDataSync(stateMsg)
	if !DataSyncSupported() {
		t.Fatal("DataSyncSupported = false after state supported=true")
	}

	h1, err := StartDataSync(DataSyncOptions{
		Notification: DataSyncNotification{Title: "Syncing", Body: "a.bin", Current: 0, Total: 100},
		WakeLock:     true,
	})
	if err != nil {
		t.Fatalf("first StartDataSync: %v", err)
	}
	if !h1.Active() {
		t.Fatal("h1.Active() = false, want true")
	}

	// Second concurrent transfer shares the same foreground service without sending a second "start".
	h2, err := StartDataSync(DataSyncOptions{
		Notification: DataSyncNotification{Title: "Syncing 2 files", Body: "b.bin", Indeterminate: true},
	})
	if err != nil {
		t.Fatalf("second StartDataSync: %v", err)
	}
	if !h2.Active() {
		t.Fatal("h2.Active() = false, want true")
	}

	h2.Update(DataSyncNotification{Title: "Syncing 2 files", Body: "50%", Current: 50, Total: 100})

	// Stopping h1 drops WakeLock (only h1 requested it) while keeping the service running for h2.
	h1.Stop()
	if h1.Active() {
		t.Error("h1 still active after Stop")
	}
	if !h2.Active() {
		t.Error("h2 became inactive when h1 stopped")
	}

	mu.Lock()
	for _, c := range cmds {
		if c.Op == "stop" {
			t.Fatalf("service stopped prematurely while h2 still active: %+v", cmds)
		}
	}
	mu.Unlock()

	// Stopping h2 releases the last reference and stops the service.
	h2.Stop()
	if h2.Active() {
		t.Error("h2 still active after Stop")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(cmds) != 5 {
		t.Fatalf("got %d commands (%+v), want 5 (start, update, update, update wakelock=false, stop)", len(cmds), cmds)
	}
	if cmds[0].Op != "start" || !cmds[0].WakeLock || cmds[0].Title != "Syncing" {
		t.Errorf("cmd[0] = %+v", cmds[0])
	}
	if cmds[1].Op != "update" || !cmds[1].Indeterminate || !cmds[1].WakeLock {
		t.Errorf("cmd[1] = %+v", cmds[1])
	}
	if cmds[2].Op != "update" || cmds[2].Current != 50 || cmds[2].Total != 100 || !cmds[2].WakeLock {
		t.Errorf("cmd[2] = %+v", cmds[2])
	}
	if cmds[3].Op != "update" || cmds[3].WakeLock {
		t.Errorf("cmd[3] = %+v, want wakeLock=false update", cmds[3])
	}
	if cmds[4].Op != "stop" || cmds[4].ID != cmds[0].ID {
		t.Errorf("cmd[4] = %+v, want stop for session %d", cmds[4], cmds[0].ID)
	}
}

func TestDataSyncStartErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		code    string
		detail  string
		wantErr error
	}{
		{"not_allowed", "not_allowed", "app is in background", ErrDataSyncNotAllowed},
		{"unavailable", "unavailable", "android.data_sync not set", ErrDataSyncUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			driver.ConnectDataSync(func(msg []byte) error {
				var c dataSyncCommand
				_ = json.Unmarshal(msg, &c)
				if c.Op == "start" {
					go func(id uint64) {
						b, _ := json.Marshal(dataSyncReport{Type: "ended", ID: id, Error: tc.code, Detail: tc.detail})
						driver.ReportDataSync(b)
					}(c.ID)
				}
				return nil
			})
			t.Cleanup(func() { driver.ConnectDataSync(nil) })

			h, err := StartDataSync(DataSyncOptions{Notification: DataSyncNotification{Title: "Upload"}})
			if h != nil {
				t.Errorf("StartDataSync returned non-nil handle on failure")
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestDataSyncTimeoutNotifiesAndStops(t *testing.T) {
	stopped := make(chan uint64, 1)
	var sessionID uint64
	driver.ConnectDataSync(func(msg []byte) error {
		var c dataSyncCommand
		_ = json.Unmarshal(msg, &c)
		switch c.Op {
		case "start":
			sessionID = c.ID
			go func(id uint64) {
				b, _ := json.Marshal(dataSyncReport{Type: "started", ID: id})
				driver.ReportDataSync(b)
			}(c.ID)
		case "stop":
			stopped <- c.ID
		}
		return nil
	})
	t.Cleanup(func() { driver.ConnectDataSync(nil) })

	flushed := make(chan struct{})
	h, err := StartDataSync(DataSyncOptions{
		Notification: DataSyncNotification{Title: "Syncing"},
		OnTimeout: func() {
			close(flushed)
		},
	})
	if err != nil {
		t.Fatalf("StartDataSync: %v", err)
	}

	timeoutMsg, _ := json.Marshal(dataSyncReport{Type: "timeout", ID: sessionID})
	driver.ReportDataSync(timeoutMsg)

	select {
	case <-flushed:
	case <-time.After(2 * time.Second):
		t.Fatal("OnTimeout callback did not run")
	}
	select {
	case <-h.Ended():
	case <-time.After(2 * time.Second):
		t.Fatal("handle Ended() did not close on timeout")
	}
	select {
	case got := <-stopped:
		if got != sessionID {
			t.Errorf("stop sent for session %d, want %d", got, sessionID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stop command was not sent after OnTimeout completed")
	}
	if !errors.Is(h.Err(), ErrDataSyncTimedOut) {
		t.Errorf("h.Err() = %v, want ErrDataSyncTimedOut", h.Err())
	}
	if h.Active() {
		t.Error("h.Active() = true after timeout")
	}
}
