package linux

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func makeTestTZif(abbr string, offset int) []byte {
	var b []byte
	header := func(version byte, counts ...int) []byte {
		h := append([]byte("TZif"), version)
		h = append(h, make([]byte, 15)...)
		for _, n := range counts {
			h = binary.BigEndian.AppendUint32(h, uint32(n))
		}
		return h
	}
	names := append([]byte(abbr), 0)
	b = append(b, header('2', 0, 0, 0, 0, 1, 1)...)
	b = append(b, 0, 0, 0, 0, 0, 0, 0)
	b = append(b, header('2', 0, 0, 0, 0, 1, len(names))...)
	b = binary.BigEndian.AppendUint32(b, uint32(int32(offset)))
	b = append(b, 0, 0)
	b = append(b, names...)
	return append(b, '\n', '\n')
}

func makeInotifyRecord(mask uint32, name string) []byte {
	var nameBytes []byte
	if name != "" {
		nameBytes = append([]byte(name), 0)
		for len(nameBytes)%4 != 0 {
			nameBytes = append(nameBytes, 0)
		}
	}
	rec := make([]byte, inotifyEventHeaderSize+len(nameBytes))
	binary.NativeEndian.PutUint32(rec[0:4], 1)
	binary.NativeEndian.PutUint32(rec[4:8], mask)
	binary.NativeEndian.PutUint32(rec[8:12], 0)
	binary.NativeEndian.PutUint32(rec[12:16], uint32(len(nameBytes)))
	copy(rec[inotifyEventHeaderSize:], nameBytes)
	return rec
}

func TestFollowSystemTimeZone(t *testing.T) {
	cases := []struct {
		name string
		val  string
		set  bool
		want bool
	}{
		{"unset", "", false, true},
		{"localtime", "/etc/localtime", true, true},
		{"colon localtime", ":/etc/localtime", true, true},
		{"empty UTC", "", true, false},
		{"explicit UTC", "UTC", true, false},
		{"explicit IANA", "America/New_York", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := followSystemTimeZone(func(k string) (string, bool) {
				if k != "TZ" {
					return "", false
				}
				return tc.val, tc.set
			})
			if got != tc.want {
				t.Errorf("followSystemTimeZone(%q, set=%v) = %v, want %v", tc.val, tc.set, got, tc.want)
			}
		})
	}
}

func TestZoneNameFromSymlink(t *testing.T) {
	cases := []struct {
		target string
		want   string
	}{
		{"/usr/share/zoneinfo/America/New_York", "America/New_York"},
		{"../usr/share/zoneinfo/Asia/Tokyo", "Asia/Tokyo"},
		{"/etc/zoneinfo/Europe/Berlin", "Europe/Berlin"},
		{"/usr/share/zoneinfo/posix/America/Chicago", "America/Chicago"},
		{"/usr/share/zoneinfo/right/UTC", "UTC"},
		{"/usr/share/zoneinfo/America/Argentina/Buenos_Aires", "America/Argentina/Buenos_Aires"},
		{"/tmp/custom-zone", ""},
		{"/usr/share/zoneinfo/../etc/passwd", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := zoneNameFromSymlink(tc.target); got != tc.want {
			t.Errorf("zoneNameFromSymlink(%q) = %q, want %q", tc.target, got, tc.want)
		}
	}
}

func TestParseInotifyEvents(t *testing.T) {
	unrelated := makeInotifyRecord(0x80, "hosts")
	localtime := makeInotifyRecord(0x80, "localtime")
	timezone := makeInotifyRecord(0x08, "timezone")
	overflow := makeInotifyRecord(inotifyQOverflow, "")

	if parseInotifyEvents(unrelated, "localtime", "timezone") {
		t.Errorf("unrelated event matched")
	}
	if !parseInotifyEvents(append(append([]byte(nil), unrelated...), localtime...), "localtime", "timezone") {
		t.Errorf("localtime event did not match")
	}
	if !parseInotifyEvents(timezone, "localtime", "timezone") {
		t.Errorf("timezone event did not match")
	}
	if !parseInotifyEvents(overflow, "localtime", "timezone") {
		t.Errorf("IN_Q_OVERFLOW did not trigger reload")
	}
	if parseInotifyEvents([]byte{1, 2, 3}, "localtime") {
		t.Errorf("truncated buffer matched")
	}
	// Corrupt length larger than remaining buffer must not panic.
	bad := make([]byte, inotifyEventHeaderSize)
	binary.NativeEndian.PutUint32(bad[12:16], 9999)
	if parseInotifyEvents(bad, "localtime") {
		t.Errorf("corrupt buffer matched")
	}
}

func TestLinuxZoneTrackerFollowsSymlinkAndFileChanges(t *testing.T) {
	saved := time.Local
	t.Cleanup(func() { time.Local = saved })

	root := t.TempDir()
	zoneinfo := filepath.Join(root, "usr", "share", "zoneinfo")
	tokyoFile := filepath.Join(zoneinfo, "Asia", "Tokyo")
	parisFile := filepath.Join(zoneinfo, "Europe", "Paris")
	if err := os.MkdirAll(filepath.Dir(tokyoFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(parisFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokyoFile, makeTestTZif("JST", 9*3600), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(parisFile, makeTestTZif("CET", 3600), 0o644); err != nil {
		t.Fatal(err)
	}

	etcDir := filepath.Join(root, "etc")
	if err := os.MkdirAll(etcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	localtime := filepath.Join(etcDir, "localtime")
	timezone := filepath.Join(etcDir, "timezone")
	if err := os.Symlink(tokyoFile, localtime); err != nil {
		t.Fatal(err)
	}

	applyCount := 0
	tracker := &linuxZoneTracker{
		localtimePath: localtime,
		timezonePath:  timezone,
		lookupEnv:     func(string) (string, bool) { return "", false },
		loadLocation: func(name string) (*time.Location, error) {
			switch name {
			case "Europe/Paris":
				return time.FixedZone("Europe/Paris", 3600), nil
			case "Asia/Kolkata":
				return time.FixedZone("Asia/Kolkata", 5*3600+1800), nil
			default:
				return nil, errors.New("unknown zone")
			}
		},
		apply: func(loc *time.Location) {
			applyCount++
			time.Local = loc
		},
		nowYear: func() int { return 2026 },
	}

	// Seed records initial state without clobbering time.Local.
	tracker.seed()
	if applyCount != 0 {
		t.Fatalf("seed applied %d times, want 0", applyCount)
	}

	// Unchanged refresh is idempotent.
	if tracker.refresh() {
		t.Fatalf("refresh reported change when localtime was unchanged")
	}
	if applyCount != 0 {
		t.Fatalf("applyCount = %d after unchanged refresh, want 0", applyCount)
	}

	// Atomically replace /etc/localtime symlink (as timedatectl does).
	tmpLink := filepath.Join(etcDir, ".#localtime1234")
	if err := os.Symlink(parisFile, tmpLink); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmpLink, localtime); err != nil {
		t.Fatal(err)
	}
	if !tracker.refresh() {
		t.Fatalf("refresh did not detect atomic symlink replacement")
	}
	if applyCount != 1 || time.Local.String() != "Europe/Paris" {
		t.Fatalf("after rename: applyCount=%d, time.Local=%q", applyCount, time.Local)
	}
	if name, off := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC).In(time.Local).Zone(); name != "CET" || off != 3600 {
		t.Fatalf("Europe/Paris zone = (%q, %d), want (CET, 3600)", name, off)
	}

	// Repeated refresh is idempotent.
	if tracker.refresh() {
		t.Fatalf("repeated refresh reported change for unchanged Europe/Paris")
	}
	if applyCount != 1 {
		t.Fatalf("applyCount = %d after repeated refresh, want 1", applyCount)
	}

	// Replace with regular TZif file + stale /etc/timezone: TZif data wins and stale name is not applied.
	if err := os.Remove(localtime); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(timezone, []byte("Europe/Paris\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localtime, makeTestTZif("IST", 5*3600+1800), 0o644); err != nil {
		t.Fatal(err)
	}
	if !tracker.refresh() {
		t.Fatalf("refresh did not detect regular file replacement")
	}
	if time.Local.String() != "Local" {
		t.Fatalf("stale /etc/timezone mislabeled time.Local as %q, want Local", time.Local)
	}
	if _, off := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC).In(time.Local).Zone(); off != 5*3600+1800 {
		t.Fatalf("copied TZif offset = %d, want 19800", off)
	}

	// Matching /etc/timezone gives the copied TZif file its IANA name.
	if err := os.WriteFile(timezone, []byte("Asia/Kolkata\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !tracker.refresh() {
		t.Fatalf("refresh did not pick up matching /etc/timezone name")
	}
	if time.Local.String() != "Asia/Kolkata" {
		t.Fatalf("time.Local = %q, want Asia/Kolkata", time.Local)
	}

	// D-Bus named change updates time.Local even if /etc/localtime didn't change on disk.
	if !tracker.applyNamed("Europe/Paris") {
		t.Fatalf("applyNamed did not apply Europe/Paris")
	}
	if time.Local.String() != "Europe/Paris" {
		t.Fatalf("after applyNamed: time.Local = %q, want Europe/Paris", time.Local)
	}
	if tracker.applyNamed("Europe/Paris") {
		t.Fatalf("repeated applyNamed was not idempotent")
	}
}

func TestLinuxZoneTrackerRespectsExplicitTZ(t *testing.T) {
	root := t.TempDir()
	localtime := filepath.Join(root, "localtime")
	if err := os.WriteFile(localtime, makeTestTZif("JST", 9*3600), 0o644); err != nil {
		t.Fatal(err)
	}
	applied := false
	tracker := &linuxZoneTracker{
		localtimePath: localtime,
		lookupEnv:     func(k string) (string, bool) { return "UTC", k == "TZ" },
		loadLocation:  func(string) (*time.Location, error) { return time.UTC, nil },
		apply:         func(*time.Location) { applied = true },
	}
	if tracker.refresh() || tracker.applyNamed("Asia/Tokyo") || applied {
		t.Fatalf("tracker modified time.Local despite explicit TZ=UTC")
	}
}
