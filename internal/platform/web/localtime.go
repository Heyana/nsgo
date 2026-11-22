//go:build js

package web

import (
	"encoding/binary"
	"strconv"
	"sync"
	"syscall/js"
	"time"
)

// Go's js runtime sets time.Local once to the offset the browser's zone has
// at startup, so times across a daylight saving change show an hour off and
// a change of the system's zone while the page stays open goes unseen.
// refreshTimeZone checks Intl.DateTimeFormat().resolvedOptions().timeZone and
// Date's offsets, loads the IANA zone when available (for apps with tzdata),
// else synthesizes TZif transitions from the browser's Date from 1970 to 50
// years ahead, else falls back to a fixed zone, and updates time.Local only
// when the browser's zone changed. Times read after a change are in the new
// zone; the app is not rendered again for it.

const timeZoneCheckIntervalMS = 30_000

type browserZoneSignature struct {
	iana      string
	nowOffset int
	janOffset int
	julOffset int
}

type webZoneTracker struct {
	mu           sync.Mutex
	hasSig       bool
	lastSig      browserZoneSignature
	readSig      func(time.Time) browserZoneSignature
	loadLocation func(string) (*time.Location, error)
	synthTZif    func(int) []byte
	apply        func(*time.Location)
	now          func() time.Time
}

var webTimeZone = webZoneTracker{
	readSig:      readBrowserZoneSignature,
	loadLocation: time.LoadLocation,
	synthTZif:    browserZone,
	apply: func(loc *time.Location) {
		// Go has no other way to change the zone: the store races with readers of time.Local, but it is one pointer, which none sees torn.
		time.Local = loc
	},
	now: time.Now,
}

func init() {
	refreshTimeZone()
}

func refreshTimeZone() bool {
	return webTimeZone.refresh()
}

func (t *webZoneTracker) refresh() bool {
	now := time.Now()
	if t.now != nil {
		now = t.now()
	}
	sig := t.readSig(now)

	t.mu.Lock()
	if t.hasSig && sig == t.lastSig {
		t.mu.Unlock()
		return false
	}
	t.lastSig = sig
	t.hasSig = true
	t.mu.Unlock()

	loc := loadBrowserLocation(sig, now.UTC().Year()+50, t.loadLocation, t.synthTZif)
	t.apply(loc)
	return true
}

func loadBrowserLocation(sig browserZoneSignature, endYear int, loadLocation func(string) (*time.Location, error), synthTZif func(int) []byte) *time.Location {
	if sig.iana != "" && loadLocation != nil {
		if loc, err := loadLocation(sig.iana); err == nil {
			return loc
		}
	}
	name := sig.iana
	if name == "" {
		name = "Local"
	}
	if synthTZif != nil {
		if data := synthTZif(endYear); len(data) > 0 {
			if loc, err := time.LoadLocationFromTZData(name, data); err == nil {
				return loc
			}
		}
	}
	return time.FixedZone(name, sig.nowOffset)
}

func browserTimeZoneID() string {
	intl := js.Global().Get("Intl")
	if !intl.Truthy() {
		return ""
	}
	dtf := intl.Get("DateTimeFormat")
	if dtf.Type() != js.TypeFunction {
		return ""
	}
	inst := dtf.New()
	if !inst.Truthy() || inst.Get("resolvedOptions").Type() != js.TypeFunction {
		return ""
	}
	opts := inst.Call("resolvedOptions")
	if !opts.Truthy() {
		return ""
	}
	tz := opts.Get("timeZone")
	if tz.Type() != js.TypeString {
		return ""
	}
	return tz.String()
}

func readBrowserZoneSignature(now time.Time) browserZoneSignature {
	date := dateClass()
	year := now.UTC().Year()
	return browserZoneSignature{
		iana:      browserTimeZoneID(),
		nowOffset: offsetAt(date, float64(now.UnixMilli())),
		janOffset: offsetAt(date, float64(time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli())),
		julOffset: offsetAt(date, float64(time.Date(year, 7, 1, 0, 0, 0, 0, time.UTC).UnixMilli())),
	}
}

func dateClass() js.Value { return js.Global().Get("Date") }

// offsetAt returns the browser zone's offset east of UTC at unix ms, in
// seconds.
func offsetAt(date js.Value, ms float64) int {
	return -date.New(ms).Call("getTimezoneOffset").Int() * 60
}

// browserZone returns TZif data of the browser zone's changes up to the
// start of year end.
func browserZone(end int) []byte {
	date := dateClass()
	const week = 7 * 24 * 3600 * 1000.0
	stop := float64(time.Date(end, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli())
	type change struct {
		at     int64 // unix seconds
		offset int
	}
	first := offsetAt(date, 0)
	var changes []change
	prev, off := 0.0, first
	for t := week; t < stop; t += week {
		o := offsetAt(date, t)
		if o == off {
			prev = t
			continue
		}
		// The change is in (prev, t]: find its minute.
		lo, hi := prev, t
		for hi-lo > 60000 {
			mid := float64(int64((lo+hi)/2/60000) * 60000)
			if mid <= lo || mid >= hi {
				break
			}
			if offsetAt(date, mid) == off {
				lo = mid
			} else {
				hi = mid
			}
		}
		changes = append(changes, change{int64(hi / 1000), o})
		prev, off = t, o
	}
	// A zone type per offset; daylight saving is any offset above this
	// year's smaller one.
	year := time.Now().UTC().Year()
	std := min(offsetAt(date, float64(time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli())),
		offsetAt(date, float64(time.Date(year, 7, 1, 0, 0, 0, 0, time.UTC).UnixMilli())))
	var types []int
	typeOf := func(offset int) byte {
		for i, o := range types {
			if o == offset {
				return byte(i)
			}
		}
		types = append(types, offset)
		return byte(len(types) - 1)
	}
	typeOf(first)
	idx := make([]byte, len(changes))
	for i, c := range changes {
		idx[i] = typeOf(c.offset)
	}
	var names []byte
	var infos []byte
	for _, o := range types {
		info := binary.BigEndian.AppendUint32(nil, uint32(int32(o)))
		dst := byte(0)
		if o != std {
			dst = 1
		}
		info = append(info, dst, byte(len(names)))
		infos = append(infos, info...)
		names = append(names, zoneName(o)...)
		names = append(names, 0)
	}
	// Version 2: an empty version 1 block, then the data with 64-bit
	// times, which reach past 2038, and an empty footer.
	header := func(version byte, counts ...int) []byte {
		h := append([]byte("TZif"), version)
		h = append(h, make([]byte, 15)...)
		for _, n := range counts {
			h = binary.BigEndian.AppendUint32(h, uint32(n))
		}
		return h
	}
	b := header('2', 0, 0, 0, 0, 1, 1)
	b = append(b, 0, 0, 0, 0, 0, 0, 0) // one empty type and name
	b = append(b, header('2', 0, 0, 0, len(changes), len(types), len(names))...)
	for _, c := range changes {
		b = binary.BigEndian.AppendUint64(b, uint64(c.at))
	}
	b = append(b, idx...)
	b = append(b, infos...)
	b = append(b, names...)
	return append(b, '\n', '\n')
}

// zoneName names an offset as Go's js runtime does: UTC+8, UTC-3:30.
func zoneName(offset int) string {
	s := "UTC+"
	if offset < 0 {
		s, offset = "UTC-", -offset
	}
	s += strconv.Itoa(offset / 3600)
	if m := offset % 3600 / 60; m != 0 {
		s += ":" + strconv.Itoa(m)
	}
	return s
}
