package windows

import (
	"errors"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"
)

func putUTF16(dst []uint16, s string) {
	copy(dst, utf16.Encode([]rune(s)))
}

func makePacificInfo() winDynamicTimeZoneInfo {
	var info winDynamicTimeZoneInfo
	info.Bias = 8 * 60
	putUTF16(info.StandardName[:], "Pacific Standard Time")
	info.StandardDate = winSystemTime{Month: 11, DayOfWeek: 0, Day: 1, Hour: 2}
	putUTF16(info.DaylightName[:], "Pacific Daylight Time")
	info.DaylightDate = winSystemTime{Month: 3, DayOfWeek: 0, Day: 2, Hour: 2}
	info.DaylightBias = -60
	putUTF16(info.TimeZoneKeyName[:], "Pacific Standard Time")
	return info
}

func makeSydneyInfo() winDynamicTimeZoneInfo {
	var info winDynamicTimeZoneInfo
	info.Bias = -10 * 60
	putUTF16(info.StandardName[:], "AUS Eastern Standard Time")
	info.StandardDate = winSystemTime{Month: 4, DayOfWeek: 0, Day: 1, Hour: 3}
	putUTF16(info.DaylightName[:], "AUS Eastern Daylight Time")
	info.DaylightDate = winSystemTime{Month: 10, DayOfWeek: 0, Day: 1, Hour: 2}
	info.DaylightBias = -60
	putUTF16(info.TimeZoneKeyName[:], "AUS Eastern Standard Time")
	return info
}

func makeTokyoInfo() winDynamicTimeZoneInfo {
	var info winDynamicTimeZoneInfo
	info.Bias = -9 * 60
	putUTF16(info.StandardName[:], "Tokyo Standard Time")
	info.StandardBias = 60 // Must be ignored because StandardDate.Month == 0.
	putUTF16(info.TimeZoneKeyName[:], "Tokyo Standard Time")
	return info
}

func TestDynamicTimeZoneInfoLayoutSize(t *testing.T) {
	if got := unsafe.Sizeof(winDynamicTimeZoneInfo{}); got != 432 {
		t.Fatalf("sizeof(winDynamicTimeZoneInfo) = %d, want 432 (Win32 DYNAMIC_TIME_ZONE_INFORMATION)", got)
	}
}

func TestWindowsZoneIDToIANA(t *testing.T) {
	cases := []struct {
		winID string
		want  string
		ok    bool
	}{
		{"Pacific Standard Time", "America/Los_Angeles", true},
		{"Eastern Standard Time", "America/New_York", true},
		{"Tokyo Standard Time", "Asia/Tokyo", true},
		{"AUS Eastern Standard Time", "Australia/Sydney", true},
		{"GMT Standard Time", "Europe/London", true},
		{"UTC", "Etc/UTC", true},
		{"Unknown Custom Zone", "", false},
	}
	for _, tc := range cases {
		got, ok := windowsZoneIDToIANA(tc.winID)
		if got != tc.want || ok != tc.ok {
			t.Errorf("windowsZoneIDToIANA(%q) = (%q, %v), want (%q, %v)", tc.winID, got, ok, tc.want, tc.ok)
		}
	}
}

func TestLocaleRegion(t *testing.T) {
	cases := []struct {
		locale string
		want   string
	}{
		{"en-US", "US"},
		{"zh-Hans-CN", "CN"},
		{"fr_ca", "CA"},
		{"es-419", "419"},
		{"en-x-private", ""},
		{"ja", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := localeRegion(tc.locale); got != tc.want {
			t.Errorf("localeRegion(%q) = %q, want %q", tc.locale, got, tc.want)
		}
	}
}

func TestWindowsLocationSynthesizesNorthernAndSouthernDST(t *testing.T) {
	noTZData := func(string) (*time.Location, error) { return nil, errors.New("no tzdata") }

	pacific := makePacificInfo()
	loc := windowsLocation(&pacific, "", noTZData, 2026)
	if loc.String() != "America/Los_Angeles" {
		t.Fatalf("Pacific location name = %q, want America/Los_Angeles", loc)
	}
	winter := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC).In(loc)
	if name, off := winter.Zone(); name != "PST" || off != -8*3600 || winter.IsDST() {
		t.Errorf("Pacific winter = (%q, %d, isDST=%v), want (PST, -28800, false)", name, off, winter.IsDST())
	}
	summer := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC).In(loc)
	if name, off := summer.Zone(); name != "PDT" || off != -7*3600 || !summer.IsDST() {
		t.Errorf("Pacific summer = (%q, %d, isDST=%v), want (PDT, -25200, true)", name, off, summer.IsDST())
	}
	// Exact spring-forward boundary in 2026: 2nd Sunday in March is 2026-03-08 02:00 PST (10:00 UTC).
	beforeSpring := time.Date(2026, 3, 8, 9, 59, 59, 0, time.UTC).In(loc)
	afterSpring := time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC).In(loc)
	if _, off := beforeSpring.Zone(); off != -8*3600 {
		t.Errorf("before spring forward offset = %d, want -28800", off)
	}
	if _, off := afterSpring.Zone(); off != -7*3600 {
		t.Errorf("at spring forward offset = %d, want -25200", off)
	}

	sydney := makeSydneyInfo()
	sydLoc := windowsLocation(&sydney, "", noTZData, 2026)
	if sydLoc.String() != "Australia/Sydney" {
		t.Fatalf("Sydney location name = %q, want Australia/Sydney", sydLoc)
	}
	sydSummer := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC).In(sydLoc)
	if name, off := sydSummer.Zone(); name != "AEDT" || off != 11*3600 || !sydSummer.IsDST() {
		t.Errorf("Sydney Jan = (%q, %d, isDST=%v), want (AEDT, 39600, true)", name, off, sydSummer.IsDST())
	}
	sydWinter := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC).In(sydLoc)
	if name, off := sydWinter.Zone(); name != "AEST" || off != 10*3600 || sydWinter.IsDST() {
		t.Errorf("Sydney Jul = (%q, %d, isDST=%v), want (AEST, 36000, false)", name, off, sydWinter.IsDST())
	}
}

func TestWindowsLocationRespectsDisabledDSTAndNoDSTZones(t *testing.T) {
	noTZData := func(string) (*time.Location, error) { return nil, errors.New("no tzdata") }

	tokyo := makeTokyoInfo()
	tokLoc := windowsLocation(&tokyo, "", noTZData, 2026)
	if tokLoc.String() != "Asia/Tokyo" {
		t.Fatalf("Tokyo location name = %q, want Asia/Tokyo", tokLoc)
	}
	for _, at := range []time.Time{
		time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
	} {
		if name, off := at.In(tokLoc).Zone(); name != "JST" || off != 9*3600 || at.In(tokLoc).IsDST() {
			t.Errorf("Tokyo at %v = (%q, %d, isDST=%v), want (JST, 32400, false)", at, name, off, at.In(tokLoc).IsDST())
		}
	}

	// When DynamicDaylightTimeDisabled != 0, IANA tzdata with DST must not override the user's disabled-DST setting.
	pacificNoDST := makePacificInfo()
	pacificNoDST.DynamicDaylightTimeDisabled = 1
	calledLoad := false
	loc := windowsLocation(&pacificNoDST, "America/Los_Angeles", func(string) (*time.Location, error) {
		calledLoad = true
		return time.FixedZone("WRONG", -7*3600), nil
	}, 2026)
	if calledLoad {
		t.Errorf("loadLocation was called despite DynamicDaylightTimeDisabled != 0")
	}
	summer := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC).In(loc)
	if _, off := summer.Zone(); off != -8*3600 || summer.IsDST() {
		t.Errorf("DST-disabled Pacific summer = (%d, isDST=%v), want (-28800, false)", off, summer.IsDST())
	}

	// Non-ASCII localized StandardName falls back cleanly to a UTC offset abbreviation.
	var localized winDynamicTimeZoneInfo
	localized.Bias = -9*60 - 30
	putUTF16(localized.StandardName[:], "カスタム標準時")
	putUTF16(localized.TimeZoneKeyName[:], "Custom Localized Zone")
	locLocalized := windowsLocation(&localized, "", noTZData, 2026)
	if name, off := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC).In(locLocalized).Zone(); name != "UTC+9:30" || off != 9*3600+30*60 {
		t.Errorf("localized zone = (%q, %d), want (UTC+9:30, 34200)", name, off)
	}
}

func TestWindowsLocationPrefersLoadLocationWhenAvailable(t *testing.T) {
	pacific := makePacificInfo()
	want := time.FixedZone("America/Vancouver", -8*3600)
	var loadedName string
	loc := windowsLocation(&pacific, "America/Vancouver", func(name string) (*time.Location, error) {
		loadedName = name
		return want, nil
	}, 2026)
	if loadedName != "America/Vancouver" || loc != want {
		t.Fatalf("loaded (%q, %v), want (America/Vancouver, %v)", loadedName, loc, want)
	}
}

func TestWindowsZoneTrackerUpdatesAndIsIdempotent(t *testing.T) {
	saved := time.Local
	t.Cleanup(func() { time.Local = saved })

	current := makePacificInfo()
	readable := true
	applyCount := 0

	tracker := &windowsZoneTracker{
		read: func() (winDynamicTimeZoneInfo, bool) {
			return current, readable
		},
		resolveIANA: func(winID string) string {
			id, _ := windowsZoneIDToIANA(winID)
			return id
		},
		loadLocation: func(string) (*time.Location, error) {
			return nil, errors.New("no tzdata")
		},
		apply: func(loc *time.Location) {
			applyCount++
			time.Local = loc
		},
		nowYear: func() int { return 2026 },
	}

	// Seed records startup state without clobbering time.Local.
	tracker.seed()
	if applyCount != 0 {
		t.Fatalf("seed applied %d times, want 0", applyCount)
	}

	// Refresh with unchanged zone is a no-op.
	if tracker.refresh() {
		t.Fatalf("refresh reported change when zone was unchanged")
	}
	if applyCount != 0 {
		t.Fatalf("unchanged refresh applied %d times, want 0", applyCount)
	}

	// Changing zone updates time.Local once.
	current = makeTokyoInfo()
	if !tracker.refresh() {
		t.Fatalf("refresh did not report change after switching to Tokyo")
	}
	if applyCount != 1 {
		t.Fatalf("applyCount = %d, want 1", applyCount)
	}
	if time.Local.String() != "Asia/Tokyo" {
		t.Fatalf("time.Local = %q, want Asia/Tokyo", time.Local)
	}
	if _, off := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC).In(time.Local).Zone(); off != 9*3600 {
		t.Fatalf("Tokyo offset = %d, want 32400", off)
	}

	// Repeated refresh with same zone is idempotent.
	if tracker.refresh() {
		t.Fatalf("repeated refresh reported change for identical Tokyo zone")
	}
	if applyCount != 1 {
		t.Fatalf("applyCount = %d after idempotent refresh, want 1", applyCount)
	}

	// Unreadable system call leaves time.Local intact.
	readable = false
	if tracker.refresh() {
		t.Fatalf("refresh reported change when system call failed")
	}
	if time.Local.String() != "Asia/Tokyo" {
		t.Fatalf("time.Local = %q after failed read, want Asia/Tokyo", time.Local)
	}

	// Subsequent switch to Sydney updates time.Local again.
	readable = true
	current = makeSydneyInfo()
	if !tracker.refresh() {
		t.Fatalf("refresh did not report change after switching to Sydney")
	}
	if applyCount != 2 || time.Local.String() != "Australia/Sydney" {
		t.Fatalf("after Sydney switch: applyCount=%d, time.Local=%q", applyCount, time.Local)
	}
}
