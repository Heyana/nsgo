//go:build js

package web

import (
	"encoding/binary"
	"errors"
	"testing"
	"time"
)

// Run under a zone with daylight saving, as in
//
//	TZ=America/New_York GOOS=js GOARCH=wasm go test -exec="bun $(go env GOROOT)/lib/wasm/wasm_exec_node.js" ./internal/platform/web
func TestLocalTimeFollowsDaylightSaving(t *testing.T) {
	winter := time.Date(2026, 1, 15, 12, 0, 0, 0, time.Local)
	summer := time.Date(2026, 7, 15, 12, 0, 0, 0, time.Local)
	_, w := winter.Zone()
	_, s := summer.Zone()
	if w == s {
		t.Skipf("the zone has one offset (%d): run under one with daylight saving", w)
	}
	for _, tm := range []time.Time{winter, summer, time.Date(2060, 7, 15, 12, 0, 0, 0, time.Local)} {
		_, off := tm.Zone()
		if want := -offsetAt(dateClass(), float64(tm.UnixMilli())); off != -want {
			t.Errorf("%v: offset %d, the browser's %d", tm, off, -want)
		}
		if tm.IsDST() != (off != min(w, s)) {
			t.Errorf("%v: IsDST = %v", tm, tm.IsDST())
		}
	}
}

func TestBrowserTimeZoneIDReadsIntl(t *testing.T) {
	id := browserTimeZoneID()
	if id == "" {
		t.Fatalf("browserTimeZoneID() = %q, want non-empty Intl timeZone", id)
	}
	if time.Local.String() != id {
		t.Errorf("time.Local = %q, want Intl zone %q", time.Local, id)
	}
}

func makeFixedTZif(abbr string, offset int) []byte {
	header := func(version byte, counts ...int) []byte {
		h := append([]byte("TZif"), version)
		h = append(h, make([]byte, 15)...)
		for _, n := range counts {
			h = binary.BigEndian.AppendUint32(h, uint32(n))
		}
		return h
	}
	names := append([]byte(abbr), 0)
	b := header('2', 0, 0, 0, 0, 1, 1)
	b = append(b, 0, 0, 0, 0, 0, 0, 0)
	b = append(b, header('2', 0, 0, 0, 0, 1, len(names))...)
	b = binary.BigEndian.AppendUint32(b, uint32(int32(offset)))
	b = append(b, 0, 0)
	b = append(b, names...)
	return append(b, '\n', '\n')
}

func TestLoadBrowserLocationFallbacks(t *testing.T) {
	noTZData := func(string) (*time.Location, error) { return nil, errors.New("no tzdata") }

	// 1. Prefers loadLocation when tzdata is available.
	want := time.FixedZone("Asia/Tokyo", 9*3600)
	loc := loadBrowserLocation(browserZoneSignature{iana: "Asia/Tokyo", nowOffset: 9 * 3600}, 2076, func(name string) (*time.Location, error) {
		if name != "Asia/Tokyo" {
			t.Fatalf("loadLocation(%q), want Asia/Tokyo", name)
		}
		return want, nil
	}, func(int) []byte {
		t.Fatalf("synthTZif should not run when loadLocation succeeds")
		return nil
	})
	if loc != want {
		t.Fatalf("got %v, want %v", loc, want)
	}

	// 2. Falls back to synthesized TZif under the IANA name when loadLocation fails.
	loc = loadBrowserLocation(browserZoneSignature{iana: "Europe/Berlin", nowOffset: 3600}, 2076, noTZData, func(int) []byte {
		return makeFixedTZif("UTC+1", 3600)
	})
	if loc.String() != "Europe/Berlin" {
		t.Fatalf("synthesized location name = %q, want Europe/Berlin", loc)
	}
	if _, off := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC).In(loc).Zone(); off != 3600 {
		t.Fatalf("synthesized offset = %d, want 3600", off)
	}

	// 3. Falls back to FixedZone when synthesized TZif is unavailable or invalid.
	loc = loadBrowserLocation(browserZoneSignature{iana: "Nowhere/Atlantis", nowOffset: -12600}, 2076, noTZData, func(int) []byte {
		return []byte("not TZif")
	})
	if loc.String() != "Nowhere/Atlantis" {
		t.Fatalf("fallback location name = %q, want Nowhere/Atlantis", loc)
	}
	if _, off := time.Now().In(loc).Zone(); off != -12600 {
		t.Fatalf("fallback offset = %d, want -12600", off)
	}

	// 4. Empty IANA name defaults to "Local".
	loc = loadBrowserLocation(browserZoneSignature{iana: "", nowOffset: 5400}, 2076, noTZData, nil)
	if loc.String() != "Local" {
		t.Fatalf("empty IANA location name = %q, want Local", loc)
	}
	if _, off := time.Now().In(loc).Zone(); off != 5400 {
		t.Fatalf("empty IANA offset = %d, want 5400", off)
	}
}

func TestWebZoneTrackerUpdatesAndIsIdempotent(t *testing.T) {
	saved := time.Local
	t.Cleanup(func() { time.Local = saved })

	sig := browserZoneSignature{iana: "America/New_York", nowOffset: -5 * 3600, janOffset: -5 * 3600, julOffset: -4 * 3600}
	synthCount := 0
	applyCount := 0

	tracker := &webZoneTracker{
		readSig: func(time.Time) browserZoneSignature { return sig },
		loadLocation: func(string) (*time.Location, error) {
			return nil, errors.New("no tzdata")
		},
		synthTZif: func(int) []byte {
			synthCount++
			return makeFixedTZif(zoneName(sig.nowOffset), sig.nowOffset)
		},
		apply: func(loc *time.Location) {
			applyCount++
			time.Local = loc
		},
		now: func() time.Time { return time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC) },
	}

	// Initial refresh loads the zone once.
	if !tracker.refresh() {
		t.Fatalf("initial refresh did not update time.Local")
	}
	if applyCount != 1 || synthCount != 1 || time.Local.String() != "America/New_York" {
		t.Fatalf("after init: apply=%d synth=%d local=%q", applyCount, synthCount, time.Local)
	}

	// Repeated refresh with unchanged signature is a no-op (no TZif scan, no write).
	if tracker.refresh() {
		t.Fatalf("unchanged refresh reported a change")
	}
	if applyCount != 1 || synthCount != 1 {
		t.Fatalf("unchanged refresh ran work: apply=%d synth=%d", applyCount, synthCount)
	}

	// Switching Intl time zone updates time.Local.
	sig = browserZoneSignature{iana: "Asia/Tokyo", nowOffset: 9 * 3600, janOffset: 9 * 3600, julOffset: 9 * 3600}
	if !tracker.refresh() {
		t.Fatalf("refresh did not detect switch to Asia/Tokyo")
	}
	if applyCount != 2 || synthCount != 2 || time.Local.String() != "Asia/Tokyo" {
		t.Fatalf("after Tokyo switch: apply=%d synth=%d local=%q", applyCount, synthCount, time.Local)
	}
	if _, off := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC).In(time.Local).Zone(); off != 9*3600 {
		t.Fatalf("Tokyo offset = %d, want 32400", off)
	}

	// Repeated refresh after change is idempotent again.
	if tracker.refresh() {
		t.Fatalf("repeated Tokyo refresh reported a change")
	}
	if applyCount != 2 || synthCount != 2 {
		t.Fatalf("repeated Tokyo refresh ran work: apply=%d synth=%d", applyCount, synthCount)
	}

	// Offset-only change with empty Intl zone still triggers an update.
	sig = browserZoneSignature{iana: "", nowOffset: -3*3600 - 1800, janOffset: -3*3600 - 1800, julOffset: -3*3600 - 1800}
	if !tracker.refresh() {
		t.Fatalf("refresh did not detect offset-only zone change")
	}
	if applyCount != 3 || time.Local.String() != "Local" {
		t.Fatalf("after offset-only switch: apply=%d local=%q", applyCount, time.Local)
	}
	if _, off := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC).In(time.Local).Zone(); off != -3*3600-1800 {
		t.Fatalf("offset-only zone offset = %d, want -12600", off)
	}
}
