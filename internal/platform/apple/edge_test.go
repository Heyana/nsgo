//go:build darwin

package apple

import (
	"math"
	"runtime"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestBackdropFilters(t *testing.T) {
	// A pool belongs to the thread that made it, so the test stays on it until the drain.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(Class("NSAutoreleasePool")).Send(selNew)
	defer pool.Send(sel("drain"))

	count := func(arr objc.ID) uint {
		return objc.Send[uint](arr, sel("count"))
	}
	at := func(arr objc.ID, i uint) objc.ID {
		return arr.Send(sel("objectAtIndex:"), i)
	}
	filterType := func(f objc.ID) string {
		return GoString(f.Send(sel("type")))
	}
	readMatrix := func(f objc.ID) driver.ColorMatrix {
		v := f.Send(selValueForKey, NSString("inputColorMatrix"))
		if v == 0 {
			t.Fatal("colorMatrix CAFilter missing inputColorMatrix value")
		}
		var got driver.ColorMatrix
		v.Send(sel("getValue:"), unsafe.Pointer(&got))
		return got
	}
	assertMatrixEqual := func(t *testing.T, got, want driver.ColorMatrix) {
		t.Helper()
		for i := range got {
			if math.Abs(float64(got[i]-want[i])) > 1e-6 {
				t.Fatalf("matrix[%d] = %v, want %v (got=%v want=%v)", i, got[i], want[i], got, want)
			}
		}
	}

	// Empty, zero blur, and drop-shadow-only produce no CAFilters.
	for _, tc := range []struct {
		name    string
		filters []driver.Filter
	}{
		{"nil", nil},
		{"empty", []driver.Filter{}},
		{"zero blur", []driver.Filter{{Kind: driver.FilterBlur, Amount: 0}}},
		{"drop shadow only", []driver.Filter{{Kind: driver.FilterDropShadow, Shadow: driver.BoxShadow{Blur: 8, Color: driver.RGB(0)}}}},
	} {
		if got := BackdropFilters(tc.filters); got != 0 {
			t.Errorf("%s: BackdropFilters = %v, want 0", tc.name, got)
		}
	}

	// Color filter alone produces one colorMatrix CAFilter matching driver.SplitFilters.
	colorCases := []struct {
		name   string
		filter driver.Filter
	}{
		{"brightness", driver.Filter{Kind: driver.FilterBrightness, Amount: 0.5}},
		{"contrast", driver.Filter{Kind: driver.FilterContrast, Amount: 1.4}},
		{"saturate", driver.Filter{Kind: driver.FilterSaturate, Amount: 0}},
		{"grayscale", driver.Filter{Kind: driver.FilterGrayscale, Amount: 1}},
		{"sepia", driver.Filter{Kind: driver.FilterSepia, Amount: 0.75}},
		{"invert", driver.Filter{Kind: driver.FilterInvert, Amount: 1}},
		{"hue-rotate", driver.Filter{Kind: driver.FilterHueRotate, Amount: 120}},
		{"opacity", driver.Filter{Kind: driver.FilterOpacity, Amount: 0.5}},
	}
	for _, tc := range colorCases {
		t.Run("color/"+tc.name, func(t *testing.T) {
			filters := []driver.Filter{tc.filter}
			wantMat, _, _, _ := driver.SplitFilters(filters)
			arr := BackdropFilters(filters)
			if arr == 0 || count(arr) != 1 {
				t.Fatalf("got %d filters, want 1", count(arr))
			}
			f := at(arr, 0)
			if got := filterType(f); got != "colorMatrix" {
				t.Fatalf("filter type = %q, want colorMatrix", got)
			}
			assertMatrixEqual(t, readMatrix(f), wantMat)
		})
	}

	// Blur alone produces one gaussianBlur CAFilter with normalized edges.
	t.Run("blur-only", func(t *testing.T) {
		arr := BackdropFilters([]driver.Filter{{Kind: driver.FilterBlur, Amount: 6}, {Kind: driver.FilterBlur, Amount: 14}})
		if arr == 0 || count(arr) != 1 {
			t.Fatalf("got %d filters, want 1", count(arr))
		}
		f := at(arr, 0)
		if got := filterType(f); got != "gaussianBlur" {
			t.Fatalf("filter type = %q, want gaussianBlur", got)
		}
		if r := objc.Send[float64](f.Send(selValueForKey, NSString("inputRadius")), sel("doubleValue")); r != 14 {
			t.Fatalf("inputRadius = %v, want 14", r)
		}
		if norm := objc.Send[bool](f.Send(selValueForKey, NSString("inputNormalizeEdges")), sel("boolValue")); !norm {
			t.Fatal("inputNormalizeEdges = false, want true")
		}
	})

	// Blur combined with multiple color filters (and an ignored drop shadow) produces
	// [gaussianBlur(widest), colorMatrix(composed)].
	t.Run("blur-and-color", func(t *testing.T) {
		filters := []driver.Filter{
			{Kind: driver.FilterBrightness, Amount: 1.2},
			{Kind: driver.FilterBlur, Amount: 4},
			{Kind: driver.FilterDropShadow, Shadow: driver.BoxShadow{Blur: 10}},
			{Kind: driver.FilterSaturate, Amount: 0.3},
			{Kind: driver.FilterBlur, Amount: 12},
			{Kind: driver.FilterContrast, Amount: 1.1},
		}
		wantMat, _, wantBlur, _ := driver.SplitFilters(filters)
		arr := BackdropFilters(filters)
		if arr == 0 || count(arr) != 2 {
			t.Fatalf("got %d filters, want 2", count(arr))
		}
		blurFilter, colorFilter := at(arr, 0), at(arr, 1)
		if got := filterType(blurFilter); got != "gaussianBlur" {
			t.Fatalf("first filter type = %q, want gaussianBlur", got)
		}
		if r := objc.Send[float64](blurFilter.Send(selValueForKey, NSString("inputRadius")), sel("doubleValue")); r != float64(wantBlur) {
			t.Fatalf("inputRadius = %v, want %v", r, wantBlur)
		}
		if got := filterType(colorFilter); got != "colorMatrix" {
			t.Fatalf("second filter type = %q, want colorMatrix", got)
		}
		assertMatrixEqual(t, readMatrix(colorFilter), wantMat)
	})
}
