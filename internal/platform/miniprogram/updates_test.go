package miniprogram

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestMiniUpdaterWaitsForReadyAndAnswersEveryCheck(t *testing.T) {
	var events []driver.Event
	u := newMiniUpdater("Fixture", "1.0", updateSnapshot{supported: true}, func(ev driver.Event) { events = append(events, ev) })
	defer u.close()
	u.check(1)
	u.check(2)
	u.publish(updateSnapshot{supported: true, checked: true, available: true})
	for _, ev := range events {
		if ev.Count != 0 {
			t.Fatal("answered before download readiness")
		}
	}
	events = nil
	when := time.Unix(1234, 0)
	u.publish(updateSnapshot{supported: true, checked: true, available: true, ready: true, checkedAt: when})
	seen := map[uint32]bool{}
	for _, ev := range events {
		if ev.Count == 0 {
			continue
		}
		found := ev.Data.(*driver.Update)
		if !found.Restart || found.Version != "" || found.App != "Fixture" || found.Current != "1.0" {
			t.Fatalf("answer = %+v", found)
		}
		seen[ev.Count] = true
	}
	if !seen[1] || !seen[2] || len(u.pending) != 0 {
		t.Fatalf("checks=%v pending=%d", seen, len(u.pending))
	}
	if !u.state().LastCheck.Equal(when) {
		t.Fatal("lost platform's actual check time")
	}
}

func TestMiniUpdaterFailureTimeoutAndClose(t *testing.T) {
	var events []driver.Event
	u := newMiniUpdater("Fixture", "1", updateSnapshot{supported: true}, func(ev driver.Event) { events = append(events, ev) })
	u.check(1)
	u.expire(1)
	u.publish(updateSnapshot{supported: true, ready: true})
	answers := 0
	for _, ev := range events {
		if ev.Count == 1 {
			answers++
			if ev.Data.(*driver.Update).Err == "" {
				t.Fatal("timeout not reported")
			}
		}
	}
	if answers != 1 {
		t.Fatalf("timed-out check answered %d times", answers)
	}
	u.publish(updateSnapshot{supported: true, err: "vendor failure"})
	u.check(2)
	if got := events[len(events)-1].Data.(*driver.Update); got.Err != "vendor failure" || got.Restart {
		t.Fatalf("failure=%+v", got)
	}
	u.publish(updateSnapshot{supported: true})
	u.check(3)
	u.close()
	before := len(events)
	u.expire(3)
	u.publish(updateSnapshot{supported: true, ready: true})
	u.check(4)
	if len(events) != before || len(u.pending) != 0 {
		t.Fatal("closed controller dispatched or retained callbacks")
	}
}

func TestMiniUpdaterCachedNoUpdateAndUnavailable(t *testing.T) {
	for _, snapshot := range []updateSnapshot{{supported: true, checked: true}, {}} {
		var found *driver.Update
		u := newMiniUpdater("Fixture", "1", snapshot, func(ev driver.Event) {
			if ev.Count != 0 {
				found = ev.Data.(*driver.Update)
			}
		})
		u.check(1)
		if found == nil || found.Restart || found.Version != "" {
			t.Fatalf("answer=%+v", found)
		}
		if snapshot.supported {
			if found.Err != "" {
				t.Fatal(found.Err)
			}
		} else if found.Err == "" || found.Managed != driver.UpdateManagedUnavailable {
			t.Fatalf("unsupported=%+v", found)
		}
		u.close()
	}
}

func TestMiniUpdaterRetainsDisabledBuildPolicy(t *testing.T) {
	for _, reason := range []driver.UpdateManaged{driver.UpdateManagedChannel, driver.UpdateManagedDevelopment} {
		var found *driver.Update
		u := newMiniUpdater("Fixture", "1", updateSnapshot{supported: true, ready: true}, func(ev driver.Event) { found = ev.Data.(*driver.Update) })
		u.blocked = reason
		u.check(1)
		if found.Managed != reason || found.Restart || found.Err != "" || u.state().Managed != reason {
			t.Fatalf("disabled answer=%+v", found)
		}
		u.close()
	}
}
