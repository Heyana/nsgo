//go:build js

package web

import (
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
