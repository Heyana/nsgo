package core

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestUpdaterStateReportsAreTrackedAcrossScopes(t *testing.T) {
	root := NewRoot(RootOptions{Updater: UpdaterState{Managed: UpdaterSelf}})
	defer root.Dispose()
	tree := NewTree(root, ApproxTextEngine{})
	child := root.Child()
	var got UpdaterState
	child.Effect(func() { got = UpdaterStatus(child) })
	state := driver.UpdaterState{Managed: driver.UpdateManagedSelf, AutomaticChecks: true, Available: "2", Notes: "notes", LastCheck: time.Unix(123, 0)}
	tree.Dispatch(driver.Event{Kind: driver.EventUpdate, Data: &state})
	if got.Available != "2" || got.Notes != "notes" || !got.AutomaticChecks || got.LastCheck.Unix() != 123 {
		t.Fatalf("state=%+v", got)
	}
	child.Dispose()
	if UpdaterStatus(root).Available != "2" {
		t.Fatal("disposing reader discarded application state")
	}
}
