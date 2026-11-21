package nsgotest

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
)

func TestCloudStorePerAppIsolationAndSimulation(t *testing.T) {
	t.Parallel()
	var root1, root2 *core.Scope
	app1 := Start(t, func(cx *core.Scope) core.Node {
		root1 = cx
		return core.Text{
			TestID: "cloud",
			ContentFunc: func() string {
				if cx.Env().CanUseCloudStorage() {
					return "on"
				}
				return "off"
			},
		}
	})
	_ = Start(t, func(cx *core.Scope) core.Node {
		root2 = cx
		return core.View{TestID: "two"}
	}, CloudAvailable(true))

	if got := app1.Find("id:cloud").Text; got != "off" {
		t.Fatalf("app1 cloud = %q, want off", got)
	}
	if root1.CloudStore().Available() {
		t.Fatal("app1 cloud should start unavailable")
	}
	if !root2.CloudStore().Available() {
		t.Fatal("app2 cloud should start available")
	}

	app1.SetCloudAvailable(true)
	if got := app1.Find("id:cloud").Text; got != "on" {
		t.Fatalf("app1 cloud = %q, want on", got)
	}

	// Write 1 MB to app1 and verify app2 does not see it.
	payload := bytes.Repeat([]byte("nsgo-cloud-1241!"), (1<<20)/16)
	if err := root1.CloudStore().WriteFile("big.sav", payload); err != nil {
		t.Fatalf("WriteFile 1 MB: %v", err)
	}
	got, err := root1.CloudStore().ReadFile("big.sav")
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("ReadFile 1 MB mismatch: err=%v len=%d", err, len(got))
	}
	if _, err := root2.CloudStore().ReadFile("big.sav"); !errors.Is(err, core.ErrCloudNotFound) {
		t.Fatalf("app2 saw app1 file: %v", err)
	}

	// Simulate external change & conflict.
	var gotEvents []core.CloudChangeEvent
	stopWatch := root1.CloudStore().WatchChanges(func(ev core.CloudChangeEvent) {
		gotEvents = append(gotEvents, ev)
	})
	defer stopWatch()

	app1.SimulateCloudChange(
		map[string][]byte{"k1": []byte("v1")},
		map[string][]byte{"f1": []byte("d1")},
		core.CloudChangeServer,
	)
	if len(gotEvents) != 1 || !slices.Equal(gotEvents[0].Keys, []string{"k1"}) || !slices.Equal(gotEvents[0].Files, []string{"f1"}) {
		t.Fatalf("unexpected events: %+v", gotEvents)
	}

	var gotConflicts []core.CloudConflict
	stopConflict := root1.CloudStore().OnConflict(func(c core.CloudConflict) {
		gotConflicts = append(gotConflicts, c)
	})
	defer stopConflict()

	app1.SimulateCloudConflict("f1", []byte("remote-d2"))
	if len(gotConflicts) != 1 || string(gotConflicts[0].Local) != "d1" || string(gotConflicts[0].Remote) != "remote-d2" {
		t.Fatalf("unexpected conflicts: %+v", gotConflicts)
	}
	if err := gotConflicts[0].Resolve([]byte("resolved-d3")); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	after, err := root1.CloudStore().ReadFile("f1")
	if err != nil || string(after) != "resolved-d3" {
		t.Fatalf("after Resolve: %q, %v", after, err)
	}

	// Toggle off: operations fail with ErrCloudUnavailable.
	app1.SetCloudAvailable(false)
	if got := app1.Find("id:cloud").Text; got != "off" {
		t.Fatalf("app1 cloud = %q, want off", got)
	}
	if err := root1.CloudStore().Set("k1", []byte("drop")); !errors.Is(err, core.ErrCloudUnavailable) {
		t.Fatalf("Set while unavailable = %v, want ErrCloudUnavailable", err)
	}
}
