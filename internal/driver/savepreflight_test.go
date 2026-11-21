package driver

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSavePreflightTimesOutWithoutWriting(t *testing.T) {
	var pending SavePreflights
	path := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	events := make(chan Event, 2)
	go pending.Save(SaveRequest{Preflight: true, Data: []byte("new")}, 1, path, func(ev Event) { events <- ev })
	next := func() Event {
		t.Helper()
		select {
		case ev := <-events:
			return ev
		case <-time.After(2 * SavePreflightTimeout):
			t.Fatal("the save did not answer")
			return Event{}
		}
	}
	if ev := next(); ev.Kind != EventSavePreflight || ev.Text != path {
		t.Fatalf("preflight = %+v", ev)
	}
	if ev := next(); ev.Kind != EventFiles || ev.Finished || ev.Text != SavePreflightTimedOut || ev.Data != path {
		t.Fatalf("timeout = %+v", ev)
	}
	pending.Answer(MountItem{Kind: ItemSavePreflightAnswer, Count: 1, On: true})
	if data, err := os.ReadFile(path); err != nil || string(data) != "old" {
		t.Fatalf("late approval changed the file to %q, %v", data, err)
	}
}
