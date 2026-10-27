//go:build js

package web

import (
	"encoding/binary"
	"strconv"
	"syscall/js"
	"time"
)

// Go's js runtime sets time.Local to the offset the browser's zone has
// now, so times across a daylight saving change show an hour off. As
// NSH's web runtime asks the page for the offset at each time, this asks
// the browser's Date for the zone's changes, from 1970 to 50 years ahead,
// and makes time.Local of them.
func init() {
	if loc, err := time.LoadLocationFromTZData("Local", browserZone(time.Now().Year()+50)); err == nil {
		time.Local = loc
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
	year := time.Now().Year()
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
