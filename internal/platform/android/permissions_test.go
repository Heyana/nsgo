package android

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// TestPermissionProtocolMatchesJava holds Protocol.java's PERMISSION_* status
// and command constants to driver's, which writePermission sends and
// Permissions.java switches on.
func TestPermissionProtocolMatchesJava(t *testing.T) {
	testhost.RequireSource(t)
	java, err := os.ReadFile(filepath.Join("java", "dev", "oneo", "nsgo", "Protocol.java"))
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]int{}
	for _, m := range regexp.MustCompile(`static final int (PERMISSION_\w+) = (\d+);`).FindAllStringSubmatch(string(java), -1) {
		values[m[1]], _ = strconv.Atoi(m[2])
	}
	for name, want := range map[string]int{
		"PERMISSION_NOT_ASKED":     driver.PermissionNotAsked,
		"PERMISSION_GRANTED":       driver.PermissionGranted,
		"PERMISSION_DENIED":        driver.PermissionDenied,
		"PERMISSION_UNKNOWN":       driver.PermissionUnknown,
		"PERMISSION_NOT_GATED":     driver.PermissionNotGated,
		"PERMISSION_UNAVAILABLE":   driver.PermissionUnavailable,
		"PERMISSION_CHECK":         int(driver.PermissionCheck),
		"PERMISSION_ASK":           int(driver.PermissionAsk),
		"PERMISSION_OPEN_SETTINGS": int(driver.PermissionOpenSettings),
		"PERMISSION_GUIDE":         int(driver.PermissionGuide),
		"PERMISSION_STOP_GUIDE":    int(driver.PermissionStopGuide),
	} {
		if got, ok := values[name]; !ok || got != want {
			t.Errorf("Protocol.java's %s is %d (found %v), driver's %d", name, got, ok, want)
		}
	}
}

// TestPermissionItemEncode checks the fields writePermission sends in the order
// NsgoView.apply reads them.
func TestPermissionItemEncode(t *testing.T) {
	for _, c := range []struct {
		count uint32
		req   driver.PermissionRequest
	}{
		{1, driver.PermissionRequest{Command: driver.PermissionCheck, Name: driver.PermissionMicrophone}},
		{2, driver.PermissionRequest{Command: driver.PermissionAsk, Name: driver.PermissionCamera}},
		{3, driver.PermissionRequest{Command: driver.PermissionAsk, Name: driver.PermissionCalendars}},
		{4, driver.PermissionRequest{Command: driver.PermissionAsk, Name: driver.PermissionBluetooth}},
		{5, driver.PermissionRequest{Command: driver.PermissionAsk, Name: driver.PermissionMediaLibrary}},
		{6, driver.PermissionRequest{Command: driver.PermissionOpenSettings, Name: driver.PermissionLocation}},
		{7, driver.PermissionRequest{Command: driver.PermissionGuide, Name: driver.PermissionCamera}},
		{8, driver.PermissionRequest{Command: driver.PermissionStopGuide}},
	} {
		var w writer
		writePermission(&w, c.count, &c.req)
		r := reader{w.buf}
		if count, cmd, name := r.u32(), driver.PermissionCommand(r.byte()), r.str(); count != c.count || cmd != c.req.Command || name != c.req.Name || len(r.buf) != 0 {
			t.Errorf("%+v: encoded (%d, %d, %q), %d bytes left", c.req, count, cmd, name, len(r.buf))
		}
	}
}

// TestPermissionsJava exercises Permissions.java on the JVM across API levels,
// check/ask/persistence, calendar multi-permissions, queuing and settings, and
// round-trips wire payloads between Go's writePermission and readEvent.
func TestPermissionsJava(t *testing.T) {
	javac, java := androidJDK(t)
	dir := t.TempDir()
	args := []string{
		"-d", dir,
		filepath.Join("java", "dev", "oneo", "nsgo", "Protocol.java"),
		filepath.Join("java", "dev", "oneo", "nsgo", "Permissions.java"),
		filepath.Join("testdata", "PermissionsCheck.java"),
	}
	if out, err := exec.CommandContext(t.Context(), javac, args...).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(t.Context(), java, "-cp", dir, "dev.oneo.nsgo.PermissionsCheck").CombinedOutput(); err != nil {
		t.Fatalf("PermissionsCheck: %v\n%s", err, out)
	}

	for _, tc := range []struct {
		name     string
		count    uint32
		req      driver.PermissionRequest
		wantIdx  int
		wantOpen bool
	}{
		{"microphone granted", 11, driver.PermissionRequest{Command: driver.PermissionCheck, Name: driver.PermissionMicrophone}, driver.PermissionGranted, false},
		{"camera prompt", 12, driver.PermissionRequest{Command: driver.PermissionCheck, Name: driver.PermissionCamera}, driver.PermissionNotAsked, false},
		{"reminders unavailable", 13, driver.PermissionRequest{Command: driver.PermissionCheck, Name: driver.PermissionReminders}, driver.PermissionUnavailable, false},
		{"open settings camera", 14, driver.PermissionRequest{Command: driver.PermissionOpenSettings, Name: driver.PermissionCamera}, 0, true},
		{"open settings location", 15, driver.PermissionRequest{Command: driver.PermissionOpenSettings, Name: driver.PermissionLocation}, 0, true},
		{"open settings unsupported", 16, driver.PermissionRequest{Command: driver.PermissionOpenSettings, Name: driver.PermissionAccessibility}, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var w writer
			writePermission(&w, tc.count, &tc.req)
			cmd := exec.CommandContext(t.Context(), java, "-cp", dir, "dev.oneo.nsgo.PermissionsCheck", "roundtrip")
			cmd.Stdin = bytes.NewReader(w.buf)
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("roundtrip: %v", err)
			}
			r := reader{out}
			from, ev, _ := readEvent(&r)
			if len(r.buf) != 0 {
				t.Errorf("%d bytes left unread", len(r.buf))
			}
			if from != 0 || ev.Kind != driver.EventPermission || ev.Count != tc.count || ev.Index != tc.wantIdx || ev.Finished != tc.wantOpen {
				t.Errorf("got surface=%d event=%+v, want kind=%v count=%d index=%d finished=%v",
					from, ev, driver.EventPermission, tc.count, tc.wantIdx, tc.wantOpen)
			}
		})
	}
}
