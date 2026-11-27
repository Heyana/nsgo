package watchos

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWatchPermissionsHostProtocol(t *testing.T) {
	cases := []struct {
		name string
		key  string
		cmd  driver.PermissionCommand
		kind byte
		ask  byte
	}{
		{"mic-check", driver.PermissionMicrophone, driver.PermissionCheck, permKindMicrophone, 0},
		{"mic-ask", driver.PermissionMicrophone, driver.PermissionAsk, permKindMicrophone, 1},
		{"mic-guide", driver.PermissionMicrophone, driver.PermissionGuide, permKindMicrophone, 0},
		{"cal-check", driver.PermissionCalendars, driver.PermissionCheck, permKindCalendars, 0},
		{"cal-ask", driver.PermissionCalendars, driver.PermissionAsk, permKindCalendars, 1},
		{"rem-check", driver.PermissionReminders, driver.PermissionCheck, permKindReminders, 0},
		{"rem-ask", driver.PermissionReminders, driver.PermissionAsk, permKindReminders, 1},
		{"bt-check", driver.PermissionBluetooth, driver.PermissionCheck, permKindBluetooth, 0},
		{"bt-ask", driver.PermissionBluetooth, driver.PermissionAsk, permKindBluetooth, 1},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var perms watchPermissions
			var w writer
			var events []driver.Event
			id := uint32(100 + i)
			dispatch := func(ev driver.Event) { events = append(events, ev) }

			perms.handle(id, driver.PermissionRequest{Command: tc.cmd, Name: tc.key}, &w, nil, dispatch)
			if len(events) != 0 {
				t.Fatalf("host-backed permission answered synchronously: %+v", events)
			}
			var want bytes.Buffer
			want.WriteByte(opPermission)
			_ = binary.Write(&want, binary.LittleEndian, id)
			want.WriteByte(tc.kind)
			want.WriteByte(tc.ask)
			if !bytes.Equal(w.buf, want.Bytes()) {
				t.Fatalf("wire bytes = %v, want %v", w.buf, want.Bytes())
			}

			perms.finished(id, uint8(driver.PermissionGranted), dispatch)
			// Duplicate completion is ignored.
			perms.finished(id, uint8(driver.PermissionDenied), dispatch)
			if len(events) != 1 || events[0].Kind != driver.EventPermission || events[0].Count != id || events[0].Index != driver.PermissionGranted {
				t.Fatalf("events = %+v", events)
			}
			if len(perms.pending) != 0 {
				t.Fatalf("pending not cleared: %+v", perms.pending)
			}
		})
	}
}

func TestWatchPermissionsHostStatuses(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  uint8
		want int
	}{
		{"prompt", uint8(driver.PermissionNotAsked), driver.PermissionNotAsked},
		{"granted", uint8(driver.PermissionGranted), driver.PermissionGranted},
		{"denied", uint8(driver.PermissionDenied), driver.PermissionDenied},
		{"unknown", uint8(driver.PermissionUnknown), driver.PermissionUnknown},
		{"not-gated", uint8(driver.PermissionNotGated), driver.PermissionNotGated},
		{"unavailable", uint8(driver.PermissionUnavailable), driver.PermissionUnavailable},
		{"limited", uint8(driver.PermissionLimited), driver.PermissionLimited},
		{"out-of-range", 99, driver.PermissionUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var perms watchPermissions
			var w writer
			var events []driver.Event
			dispatch := func(ev driver.Event) { events = append(events, ev) }
			perms.handle(7, driver.PermissionRequest{Command: driver.PermissionAsk, Name: driver.PermissionMicrophone}, &w, nil, dispatch)
			perms.finished(7, tc.raw, dispatch)
			if len(events) != 1 || events[0].Index != tc.want {
				t.Fatalf("events = %+v, want status %d", events, tc.want)
			}
		})
	}

	// Unknown ID is ignored.
	var perms watchPermissions
	var events []driver.Event
	perms.finished(999, uint8(driver.PermissionGranted), func(ev driver.Event) { events = append(events, ev) })
	if len(events) != 0 {
		t.Fatalf("unknown ID dispatched %+v", events)
	}
}

func TestWatchPermissionsNotificationsAndStatic(t *testing.T) {
	var perms watchPermissions
	var w writer
	var events []driver.Event
	var asked []bool
	dispatch := func(ev driver.Event) { events = append(events, ev) }

	notifyPerm := func(ask bool, answer func(int)) {
		asked = append(asked, ask)
		if ask {
			answer(driver.NotificationsGranted)
		} else {
			answer(driver.NotificationsNotAsked)
		}
	}

	// Notifications check, ask and guide route through UNUserNotificationCenter.
	perms.handle(1, driver.PermissionRequest{Command: driver.PermissionCheck, Name: driver.PermissionNotifications}, &w, notifyPerm, dispatch)
	perms.handle(2, driver.PermissionRequest{Command: driver.PermissionAsk, Name: driver.PermissionNotifications}, &w, notifyPerm, dispatch)
	perms.handle(3, driver.PermissionRequest{Command: driver.PermissionGuide, Name: driver.PermissionNotifications}, &w, notifyPerm, dispatch)
	if len(w.buf) != 0 {
		t.Fatalf("notifications wrote host ops: %v", w.buf)
	}
	if len(asked) != 3 || asked[0] || !asked[1] || asked[2] {
		t.Fatalf("asked = %v, want [false true false]", asked)
	}
	if len(events) != 3 ||
		events[0].Index != driver.PermissionNotAsked ||
		events[1].Index != driver.PermissionGranted ||
		events[2].Index != driver.PermissionNotAsked {
		t.Fatalf("notification permission events = %+v", events)
	}

	// Nil notification backend answers unavailable.
	events = nil
	perms.handle(4, driver.PermissionRequest{Command: driver.PermissionCheck, Name: driver.PermissionNotifications}, &w, nil, dispatch)
	if len(events) != 1 || events[0].Index != driver.PermissionUnavailable {
		t.Fatalf("nil notification backend = %+v", events)
	}

	// Denied notification permission maps to PermissionDenied.
	events = nil
	perms.handle(5, driver.PermissionRequest{Command: driver.PermissionCheck, Name: driver.PermissionNotifications}, &w, func(_ bool, answer func(int)) {
		answer(driver.NotificationsDenied)
	}, dispatch)
	if len(events) != 1 || events[0].Index != driver.PermissionDenied {
		t.Fatalf("denied notification permission = %+v", events)
	}

	// OpenPermissionSettings honestly answers false without writing host ops.
	events = nil
	for i, key := range []string{driver.PermissionMicrophone, driver.PermissionBluetooth, driver.PermissionNotifications, driver.PermissionLocation} {
		perms.handle(uint32(10+i), driver.PermissionRequest{Command: driver.PermissionOpenSettings, Name: key}, &w, notifyPerm, dispatch)
	}
	if len(w.buf) != 0 || len(events) != 4 {
		t.Fatalf("open settings = %d events, buf %v", len(events), w.buf)
	}
	for _, ev := range events {
		if ev.Kind != driver.EventPermission || ev.Finished {
			t.Fatalf("open settings reported opened: %+v", ev)
		}
	}

	// StopGuide sends nothing.
	events = nil
	perms.handle(20, driver.PermissionRequest{Command: driver.PermissionStopGuide}, &w, notifyPerm, dispatch)
	if len(events) != 0 || len(w.buf) != 0 {
		t.Fatalf("stop guide emitted %+v / %v", events, w.buf)
	}

	// Static unavailable and unknown permissions answer immediately.
	for _, tc := range []struct {
		key  string
		want int
	}{
		{driver.PermissionCamera, driver.PermissionUnavailable},
		{driver.PermissionPhotos, driver.PermissionUnavailable},
		{driver.PermissionMediaLibrary, driver.PermissionUnavailable},
		{driver.PermissionAccessibility, driver.PermissionUnavailable},
		{driver.PermissionScreenRecording, driver.PermissionUnavailable},
		{driver.PermissionAutomation + ":com.apple.finder", driver.PermissionUnavailable},
		{driver.PermissionContacts, driver.PermissionUnknown},
		{driver.PermissionLocalNetwork, driver.PermissionUnknown},
		{"no-such-permission", driver.PermissionUnavailable},
	} {
		events = nil
		perms.handle(30, driver.PermissionRequest{Command: driver.PermissionCheck, Name: tc.key}, &w, notifyPerm, dispatch)
		if len(w.buf) != 0 || len(events) != 1 || events[0].Index != tc.want {
			t.Fatalf("static %q: events=%+v buf=%v want=%d", tc.key, events, w.buf, tc.want)
		}
	}
}
