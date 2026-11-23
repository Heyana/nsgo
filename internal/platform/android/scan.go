//go:build android || nsgoandroidhost

package android

import (
	"slices"
	"sync"
	"sync/atomic"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/codescan"
)

// Scanning a code: the runtime shows Camera2's preview full screen and sends the luminance of its frames, a few a second, as msgScanReport; Go reads them with codescan, Android having no reader of its own without Google Play services, and stops the scanner with msgScan once one holds a code.

// What a msgScan asks and a msgScanReport reports, as Protocol.java's SCAN_*.
const (
	scanStart  = 0
	scanStop   = 1
	scanFrame  = 0
	scanClosed = 1
	scanFailed = 2
)

// scanning is a scan the runtime shows: what it reads, and whether a frame is being read, which the frames that come meanwhile skip.
type scanning struct {
	r       driver.ScanRequest
	reading atomic.Bool
	done    atomic.Bool
}

// scans are the scans under way, by answerKey.
var scans sync.Map

// withoutScans hands the scans of surface id's batch b to the runtime as msgScan and returns b without them.
func (s *surface) withoutScans(id uint32, b *driver.MountBatch) *driver.MountBatch {
	isScan := func(it driver.MountItem) bool { return it.Kind == driver.ItemScanCode }
	if !slices.ContainsFunc(b.Items, isScan) {
		return b
	}
	for _, it := range b.Items {
		if !isScan(it) {
			continue
		}
		// A scanner that can read none of the kinds asked for, such as PDF417 alone, would show the camera until the user closes it.
		if !slices.ContainsFunc(scanKinds(*it.Scan), codescan.Reads) {
			s.dispatchTo(id, driver.Event{Kind: driver.EventPick, Count: it.Count, Text: "the camera reads none of the codes asked for"})
			continue
		}
		scans.Store(answerKey(id, it.Count), &scanning{r: *it.Scan})
		s.send(msgScan, func(w *writer) {
			w.u32(id)
			w.u32(it.Count)
			w.byte(scanStart)
		})
	}
	out := *b
	out.Items = slices.DeleteFunc(slices.Clone(b.Items), isScan)
	return &out
}

// scanKinds are the kinds r asks for: a QR code alone when it names none.
func scanKinds(r driver.ScanRequest) []driver.CodeKind {
	if len(r.Kinds) == 0 {
		return []driver.CodeKind{driver.CodeQR}
	}
	return r.Kinds
}

// scanReport takes a msgScanReport on the reader goroutine: its surface and count, what it reports, then a frame's width, height and luminance, or a failure's reason.
func (s *surface) scanReport(r *reader) {
	surface, count, what := r.u32(), r.u32(), r.byte()
	key := answerKey(surface, count)
	v, ok := scans.Load(key)
	if !ok {
		return
	}
	sc := v.(*scanning)
	switch what {
	case scanFrame:
		width, height := int(r.u32()), int(r.u32())
		lum := r.bytes()
		if sc.done.Load() || !sc.reading.CompareAndSwap(false, true) {
			return
		}
		// lum is the payload's, which the reader reuses.
		lum = slices.Clone(lum)
		go func() {
			defer sc.reading.Store(false)
			code := codescan.Decode(lum, width, height, sc.r)
			if code == nil || !sc.done.CompareAndSwap(false, true) {
				return
			}
			scans.Delete(key)
			s.send(msgScan, func(w *writer) {
				w.u32(surface)
				w.u32(count)
				w.byte(scanStop)
			})
			s.dispatchTo(surface, driver.Event{Kind: driver.EventPick, Count: count, Data: code})
		}()
	case scanClosed, scanFailed:
		if !sc.done.CompareAndSwap(false, true) {
			return
		}
		scans.Delete(key)
		ev := driver.Event{Kind: driver.EventPick, Count: count}
		if what == scanFailed {
			ev.Text = r.str()
		}
		s.dispatchTo(surface, ev)
	}
}
