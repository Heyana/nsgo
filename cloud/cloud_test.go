package cloud_test

import (
	"bytes"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/cloud"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestKeyValueAndFileRoundTripOneMegabyte(t *testing.T) {
	t.Parallel()
	var root *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		root = cx
		return core.View{TestID: "root"}
	}, nsgotest.CloudAvailable(true))

	// Build a deterministic 1 MiB payload for both KV and file round-trips.
	oneMB := make([]byte, 1<<20)
	for i := range oneMB {
		oneMB[i] = byte((i*31 + 17) & 0xff)
	}

	var setErr, getErr, delErr, keysErr error
	var gotVal []byte
	var gotKeys []string

	app.Post(func() {
		cloud.Set(root, "profile", oneMB, func(err error) { setErr = err })
		cloud.Set(root, "theme", []byte("dark"), func(err error) {
			if err != nil {
				t.Errorf("Set theme: %v", err)
			}
		})
	})
	app.Settle()
	if setErr != nil {
		t.Fatalf("Set 1 MB KV failed: %v", setErr)
	}

	app.Post(func() {
		cloud.Get(root, "profile", func(v []byte, err error) { gotVal, getErr = v, err })
		cloud.Keys(root, func(k []string, err error) { gotKeys, keysErr = k, err })
	})
	app.Settle()
	if getErr != nil {
		t.Fatalf("Get 1 MB KV failed: %v", getErr)
	}
	if !bytes.Equal(gotVal, oneMB) {
		t.Fatalf("Get 1 MB KV mismatch: got %d bytes", len(gotVal))
	}
	if keysErr != nil || !slices.Equal(gotKeys, []string{"profile", "theme"}) {
		t.Fatalf("Keys = %v, %v; want [profile theme]", gotKeys, keysErr)
	}

	app.Post(func() {
		cloud.Delete(root, "profile", func(err error) { delErr = err })
	})
	app.Settle()
	if delErr != nil {
		t.Fatalf("Delete failed: %v", delErr)
	}

	app.Post(func() {
		cloud.Get(root, "profile", func(v []byte, err error) { gotVal, getErr = v, err })
	})
	app.Settle()
	if !errors.Is(getErr, cloud.ErrNotFound) {
		t.Fatalf("Get deleted key = %v, want ErrNotFound", getErr)
	}

	// File / snapshot store: 1 MiB round-trip + nested path + listing + deletion.
	var writeErr, readErr, listErr, delFileErr error
	var gotFile []byte
	var gotFiles []string

	app.Post(func() {
		cloud.WriteFile(root, "saves/slot1.sav", oneMB, func(err error) { writeErr = err })
		cloud.WriteFile(root, "saves/meta.yaml", []byte("version: 1\n"), func(err error) {
			if err != nil {
				t.Errorf("WriteFile meta: %v", err)
			}
		})
	})
	app.Settle()
	if writeErr != nil {
		t.Fatalf("WriteFile 1 MB failed: %v", writeErr)
	}

	app.Post(func() {
		cloud.ReadFile(root, "saves/slot1.sav", func(d []byte, err error) { gotFile, readErr = d, err })
		cloud.ListFiles(root, func(names []string, err error) { gotFiles, listErr = names, err })
	})
	app.Settle()
	if readErr != nil {
		t.Fatalf("ReadFile 1 MB failed: %v", readErr)
	}
	if !bytes.Equal(gotFile, oneMB) {
		t.Fatalf("ReadFile 1 MB mismatch: got %d bytes", len(gotFile))
	}
	if listErr != nil || !slices.Equal(gotFiles, []string{"saves/meta.yaml", "saves/slot1.sav"}) {
		t.Fatalf("ListFiles = %v, %v", gotFiles, listErr)
	}

	app.Post(func() {
		cloud.DeleteFile(root, "saves/slot1.sav", func(err error) { delFileErr = err })
	})
	app.Settle()
	if delFileErr != nil {
		t.Fatalf("DeleteFile failed: %v", delFileErr)
	}

	app.Post(func() {
		cloud.ReadFile(root, "saves/slot1.sav", func(_ []byte, err error) { readErr = err })
	})
	app.Settle()
	if !errors.Is(readErr, cloud.ErrNotFound) {
		t.Fatalf("ReadFile deleted file = %v, want ErrNotFound", readErr)
	}
}

func TestUnavailableNeverSilentlyDropsWrites(t *testing.T) {
	t.Parallel()
	var root *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		root = cx
		return core.Text{
			TestID: "status",
			ContentFunc: func() string {
				if cx.Env().CanUseCloudStorage() {
					return "available"
				}
				return "unavailable"
			},
		}
	})

	if got := app.Find("id:status").Text; got != "unavailable" {
		t.Fatalf("status = %q, want unavailable", got)
	}

	checkAllUnavailable := func(stage string) {
		t.Helper()
		var errs [8]error
		app.Post(func() {
			cloud.Get(root, "k", func(_ []byte, err error) { errs[0] = err })
			cloud.Set(root, "k", []byte("v"), func(err error) { errs[1] = err })
			cloud.Delete(root, "k", func(err error) { errs[2] = err })
			cloud.Keys(root, func(_ []string, err error) { errs[3] = err })
			cloud.ReadFile(root, "save.bin", func(_ []byte, err error) { errs[4] = err })
			cloud.WriteFile(root, "save.bin", []byte("v"), func(err error) { errs[5] = err })
			cloud.DeleteFile(root, "save.bin", func(err error) { errs[6] = err })
			cloud.ListFiles(root, func(_ []string, err error) { errs[7] = err })
		})
		app.Settle()
		for i, err := range errs {
			if !errors.Is(err, cloud.ErrUnavailable) {
				t.Errorf("%s: op %d returned %v, want ErrUnavailable", stage, i, err)
			}
		}
	}

	checkAllUnavailable("initial")

	// Enable cloud storage, write data, then disable again and verify writes fail loudly.
	app.SetCloudAvailable(true)
	if got := app.Find("id:status").Text; got != "available" {
		t.Fatalf("status = %q, want available", got)
	}

	var writeErr error
	app.Post(func() {
		cloud.Set(root, "k", []byte("saved"), func(err error) { writeErr = err })
	})
	app.Settle()
	if writeErr != nil {
		t.Fatalf("Set while available: %v", writeErr)
	}

	app.SetCloudAvailable(false)
	if got := app.Find("id:status").Text; got != "unavailable" {
		t.Fatalf("status = %q, want unavailable", got)
	}
	checkAllUnavailable("after-disable")

	// Re-enable and confirm the earlier failed Set did not overwrite "saved".
	app.SetCloudAvailable(true)
	var got []byte
	var getErr error
	app.Post(func() {
		cloud.Get(root, "k", func(v []byte, err error) { got, getErr = v, err })
	})
	app.Settle()
	if getErr != nil || string(got) != "saved" {
		t.Fatalf("Get after re-enable = %q, %v; want %q, nil", got, getErr, "saved")
	}
}

func TestWatchChangesAndScopeDisposalCleanup(t *testing.T) {
	t.Parallel()
	var events []cloud.ChangeEvent
	var showChild *core.Signal[bool]

	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		showChild = core.NewSignal(cx, true)
		return core.Show{
			When: showChild.Get,
			Then: func(child *core.Scope) core.Node {
				cloud.WatchChanges(child, func(ev cloud.ChangeEvent) {
					events = append(events, ev)
				})
				return core.View{TestID: "child"}
			},
		}
	}, nsgotest.CloudAvailable(true))

	app.SimulateCloudChange(
		map[string][]byte{"level": []byte("42")},
		map[string][]byte{"slot1.sav": []byte("remote-save")},
		cloud.ChangeServer,
	)
	if len(events) != 1 {
		t.Fatalf("got %d change events, want 1", len(events))
	}
	if !slices.Equal(events[0].Keys, []string{"level"}) || !slices.Equal(events[0].Files, []string{"slot1.sav"}) || events[0].Reason != cloud.ChangeServer {
		t.Fatalf("unexpected change event: %+v", events[0])
	}

	// Dispose the child scope; subsequent changes must not reach the disposed watcher.
	app.Post(func() { showChild.Set(false) })
	app.Settle()

	app.SimulateCloudChange(
		map[string][]byte{"level": []byte("99")},
		nil,
		cloud.ChangeInitialSync,
	)
	if len(events) != 1 {
		t.Fatalf("disposed watcher still received event: %+v", events)
	}
}

func TestConflictDetectionAndResolution(t *testing.T) {
	t.Parallel()
	var root *core.Scope
	var received []cloud.Conflict
	var showWatcher *core.Signal[bool]

	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		root = cx
		showWatcher = core.NewSignal(cx, true)
		return core.Show{
			When: showWatcher.Get,
			Then: func(child *core.Scope) core.Node {
				cloud.OnConflict(child, func(c cloud.Conflict) {
					received = append(received, c)
				})
				return core.View{TestID: "watcher"}
			},
		}
	}, nsgotest.CloudAvailable(true))

	app.Post(func() {
		cloud.WriteFile(root, "save.sav", []byte("local-v1"), func(err error) {
			if err != nil {
				t.Errorf("WriteFile: %v", err)
			}
		})
	})
	app.Settle()

	app.Advance(5 * time.Second)
	app.SimulateCloudConflict("save.sav", []byte("remote-v2"))

	if len(received) != 1 {
		t.Fatalf("got %d conflicts, want 1", len(received))
	}
	c := received[0]
	if c.Name != "save.sav" || string(c.Local) != "local-v1" || string(c.Remote) != "remote-v2" {
		t.Fatalf("unexpected conflict: %+v", c)
	}
	if !c.RemoteTime.After(c.LocalTime) {
		t.Errorf("expected RemoteTime (%v) > LocalTime (%v)", c.RemoteTime, c.LocalTime)
	}

	// Reading before resolution returns *cloud.ConflictError unwrapping to cloud.ErrConflict.
	var readErr error
	app.Post(func() {
		cloud.ReadFile(root, "save.sav", func(_ []byte, err error) { readErr = err })
	})
	app.Settle()
	if !errors.Is(readErr, cloud.ErrConflict) {
		t.Fatalf("ReadFile during conflict = %v, want ErrConflict", readErr)
	}
	var ce *cloud.ConflictError
	if !errors.As(readErr, &ce) || ce.Key != "save.sav" || string(ce.Local) != "local-v1" || string(ce.Remote) != "remote-v2" {
		t.Fatalf("ConflictError = %+v", ce)
	}

	// Resolve with merged data; ReadFile now succeeds with the winning bytes.
	if err := c.Resolve([]byte("merged-v3")); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	var resolved []byte
	app.Post(func() {
		cloud.ReadFile(root, "save.sav", func(d []byte, err error) { resolved, readErr = d, err })
	})
	app.Settle()
	if readErr != nil || string(resolved) != "merged-v3" {
		t.Fatalf("ReadFile after Resolve = %q, %v; want %q, nil", resolved, readErr, "merged-v3")
	}

	// Disposing the watcher's scope stops further conflict notifications.
	app.Post(func() { showWatcher.Set(false) })
	app.Settle()
	app.SimulateCloudConflict("save.sav", []byte("remote-v4"))
	if len(received) != 1 {
		t.Fatalf("disposed conflict handler still fired: %d", len(received))
	}
}

func TestPerAppIsolationAndValidation(t *testing.T) {
	t.Parallel()
	var rootA, rootB *core.Scope
	appA := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		rootA = cx
		return core.View{TestID: "a"}
	}, nsgotest.CloudAvailable(true))
	appB := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		rootB = cx
		return core.View{TestID: "b"}
	}, nsgotest.CloudAvailable(true))

	appA.Post(func() {
		cloud.Set(rootA, "shared-key", []byte("from-A"), nil)
		cloud.WriteFile(rootA, "slot.sav", []byte("file-A"), nil)
	})
	appA.Settle()

	var errKey, errFile error
	appB.Post(func() {
		cloud.Get(rootB, "shared-key", func(_ []byte, err error) { errKey = err })
		cloud.ReadFile(rootB, "slot.sav", func(_ []byte, err error) { errFile = err })
	})
	appB.Settle()
	if !errors.Is(errKey, cloud.ErrNotFound) || !errors.Is(errFile, cloud.ErrNotFound) {
		t.Fatalf("apps leaked cloud state: key=%v file=%v", errKey, errFile)
	}

	// Validate invalid keys and traversal file names are refused.
	for _, bad := range []string{"", "/abs", "../escape", "a/../..", `bad\sep`} {
		var err error
		appA.Post(func() {
			cloud.WriteFile(rootA, bad, []byte("x"), func(e error) { err = e })
		})
		appA.Settle()
		if err == nil {
			t.Errorf("WriteFile(%q) succeeded, want error", bad)
		}
	}
	var emptyKeyErr error
	appA.Post(func() {
		cloud.Set(rootA, "", []byte("x"), func(e error) { emptyKeyErr = e })
	})
	appA.Settle()
	if emptyKeyErr == nil {
		t.Error("Set empty key succeeded, want error")
	}
}
