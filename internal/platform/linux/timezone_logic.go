package linux

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultLocaltimePath = "/etc/localtime"
	defaultTimezonePath  = "/etc/timezone"

	inotifyEventHeaderSize = 16
	inotifyQOverflow       = 0x4000
)

// followSystemTimeZone reports whether this process follows the system's /etc/localtime rather than an explicit TZ environment variable, matching Go's time/zoneinfo_unix.go.
func followSystemTimeZone(lookupEnv func(string) (string, bool)) bool {
	tz, ok := lookupEnv("TZ")
	if !ok {
		return true
	}
	return tz == defaultLocaltimePath || tz == ":"+defaultLocaltimePath
}

// zoneNameFromSymlink extracts an IANA zone ID (such as "America/New_York") from a symlink target pointing under a zoneinfo directory.
func zoneNameFromSymlink(target string) string {
	if target == "" {
		return ""
	}
	const marker = "zoneinfo/"
	idx := strings.LastIndex(target, marker)
	if idx < 0 {
		return ""
	}
	name := strings.TrimPrefix(target[idx+len(marker):], "/")
	for _, prefix := range []string{"posix/", "right/"} {
		name = strings.TrimPrefix(name, prefix)
	}
	name = strings.TrimSuffix(name, "/")
	if name == "" || name == "." || name == ".." || strings.Contains(name, "..") {
		return ""
	}
	return name
}

// parseInotifyEvents parses raw Linux inotify_event records and reports whether any event targets one of names or indicates a queue overflow.
func parseInotifyEvents(buf []byte, names ...string) bool {
	for len(buf) >= inotifyEventHeaderSize {
		mask := binary.NativeEndian.Uint32(buf[4:8])
		nameLen := int(binary.NativeEndian.Uint32(buf[12:16]))
		if nameLen < 0 || inotifyEventHeaderSize+nameLen > len(buf) {
			break
		}
		if mask&inotifyQOverflow != 0 {
			return true
		}
		raw := buf[inotifyEventHeaderSize : inotifyEventHeaderSize+nameLen]
		entry, _, _ := bytes.Cut(raw, []byte{0})
		if len(entry) > 0 {
			s := string(entry)
			for _, want := range names {
				if s == want {
					return true
				}
			}
		}
		buf = buf[inotifyEventHeaderSize+nameLen:]
	}
	return false
}

func readTimezoneFile(path string) string {
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(b))
	if line == "" || strings.ContainsAny(line, "\r\n") || strings.Contains(line, "..") {
		return ""
	}
	return line
}

func locationsAgree(a, b *time.Location, year int) bool {
	for _, m := range []time.Month{time.January, time.July} {
		at := time.Date(year, m, 15, 12, 0, 0, 0, time.UTC)
		_, oa := at.In(a).Zone()
		_, ob := at.In(b).Zone()
		if oa != ob {
			return false
		}
	}
	return true
}

// readLinuxTimeZone reads the system's current time zone from localtimePath (and timezonePath as a name hint or fallback) and returns its name, raw TZif bytes, and parsed *time.Location.
func readLinuxTimeZone(localtimePath, timezonePath string, loadLocation func(string) (*time.Location, error), nowYear int) (name string, data []byte, loc *time.Location, ok bool) {
	if data, err := os.ReadFile(localtimePath); err == nil && len(data) > 0 {
		if link, err := os.Readlink(localtimePath); err == nil {
			name = zoneNameFromSymlink(link)
		}
		if name == "" {
			if resolved, err := filepath.EvalSymlinks(localtimePath); err == nil {
				name = zoneNameFromSymlink(resolved)
			}
		}
		rawLoc, err := time.LoadLocationFromTZData("Local", data)
		if err == nil {
			if name == "" {
				if tz := readTimezoneFile(timezonePath); tz != "" && loadLocation != nil {
					if tzLoc, err := loadLocation(tz); err == nil && locationsAgree(rawLoc, tzLoc, nowYear) {
						name = tz
					}
				}
			}
			if name == "" || name == "Local" {
				return "Local", data, rawLoc, true
			}
			if namedLoc, err := time.LoadLocationFromTZData(name, data); err == nil {
				return name, data, namedLoc, true
			}
			return "Local", data, rawLoc, true
		}
	}

	if tz := readTimezoneFile(timezonePath); tz != "" && loadLocation != nil {
		if loc, err := loadLocation(tz); err == nil {
			return tz, nil, loc, true
		}
	}
	return "", nil, nil, false
}

type linuxZoneTracker struct {
	mu            sync.Mutex
	seeded        bool
	lastName      string
	lastDiskName  string
	lastDiskData  []byte
	localtimePath string
	timezonePath  string
	lookupEnv     func(string) (string, bool)
	loadLocation  func(string) (*time.Location, error)
	apply         func(*time.Location)
	nowYear       func() int
}

func (t *linuxZoneTracker) year() int {
	if t.nowYear != nil {
		return t.nowYear()
	}
	return time.Now().UTC().Year()
}

// seed records the system's initial /etc/localtime without overwriting Go's startup time.Local.
func (t *linuxZoneTracker) seed() {
	if !followSystemTimeZone(t.lookupEnv) {
		return
	}
	name, data, _, ok := readLinuxTimeZone(t.localtimePath, t.timezonePath, t.loadLocation, t.year())
	if !ok {
		return
	}
	t.mu.Lock()
	if !t.seeded {
		t.lastName = name
		t.lastDiskName = name
		t.lastDiskData = append([]byte(nil), data...)
		t.seeded = true
	}
	t.mu.Unlock()
}

// refresh reloads /etc/localtime and updates time.Local only when the on-disk zone changed.
func (t *linuxZoneTracker) refresh() bool {
	if !followSystemTimeZone(t.lookupEnv) {
		return false
	}
	name, data, loc, ok := readLinuxTimeZone(t.localtimePath, t.timezonePath, t.loadLocation, t.year())
	if !ok {
		return false
	}
	t.mu.Lock()
	if t.seeded && name == t.lastDiskName && bytes.Equal(data, t.lastDiskData) {
		t.mu.Unlock()
		return false
	}
	t.lastName = name
	t.lastDiskName = name
	t.lastDiskData = append([]byte(nil), data...)
	t.seeded = true
	t.mu.Unlock()

	t.apply(loc)
	return true
}

// applyNamed handles a D-Bus timedate1 Timezone property change. It prefers reloading /etc/localtime when the file changed on disk so TZif data stays authoritative, falling back to loadLocation(name) when /etc/localtime has not changed on disk.
func (t *linuxZoneTracker) applyNamed(name string) bool {
	if !followSystemTimeZone(t.lookupEnv) {
		return false
	}
	if t.refresh() {
		return true
	}
	name = strings.TrimSpace(name)
	if name == "" || t.loadLocation == nil {
		return false
	}
	t.mu.Lock()
	unchanged := t.seeded && t.lastName == name
	t.mu.Unlock()
	if unchanged {
		return false
	}
	loc, err := t.loadLocation(name)
	if err != nil {
		return false
	}
	t.mu.Lock()
	if t.seeded && t.lastName == name {
		t.mu.Unlock()
		return false
	}
	t.lastName = name
	t.seeded = true
	t.mu.Unlock()

	t.apply(loc)
	return true
}
