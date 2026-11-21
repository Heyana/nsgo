package linux

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestScreenshotPortalSupportsColor(t *testing.T) {
	cases := []struct {
		version uint32
		ok      bool
		want    bool
	}{
		{0, false, false},
		{0, true, false},
		{1, true, false},
		{2, false, false},
		{2, true, true},
		{3, true, true},
	}
	for _, tc := range cases {
		if got := screenshotPortalSupportsColor(tc.version, tc.ok); got != tc.want {
			t.Errorf("screenshotPortalSupportsColor(%d, %v) = %v, want %v", tc.version, tc.ok, got, tc.want)
		}
	}
}

func TestPortalColorFromRGB(t *testing.T) {
	nan := math.NaN()
	cases := []struct {
		name    string
		code    uint32
		r, g, b float64
		want    driver.Color
		ok      bool
	}{
		{name: "black", code: portalResponseSuccess, r: 0, g: 0, b: 0, want: 0x000000ff, ok: true},
		{name: "white", code: portalResponseSuccess, r: 1, g: 1, b: 1, want: 0xffffffff, ok: true},
		{name: "red", code: portalResponseSuccess, r: 1, g: 0, b: 0, want: 0xff0000ff, ok: true},
		{name: "green", code: portalResponseSuccess, r: 0, g: 1, b: 0, want: 0x00ff00ff, ok: true},
		{name: "blue", code: portalResponseSuccess, r: 0, g: 0, b: 1, want: 0x0000ffff, ok: true},
		{name: "rounded", code: portalResponseSuccess, r: 0.5, g: 0.25, b: 0.75, want: 0x8040bfff, ok: true},
		{name: "exact bytes", code: portalResponseSuccess, r: 0x12 / 255.0, g: 0x34 / 255.0, b: 0x56 / 255.0, want: 0x123456ff, ok: true},
		{name: "clamped", code: portalResponseSuccess, r: -0.25, g: 1.5, b: 0.5, want: 0x00ff80ff, ok: true},
		{name: "cancelled", code: portalResponseCancelled, r: 1, g: 0, b: 0, want: 0, ok: false},
		{name: "other failure", code: portalResponseOther, r: 1, g: 0, b: 0, want: 0, ok: false},
		{name: "nan red", code: portalResponseSuccess, r: nan, g: 0.5, b: 0.5, want: 0, ok: false},
		{name: "nan green", code: portalResponseSuccess, r: 0.5, g: nan, b: 0.5, want: 0, ok: false},
		{name: "nan blue", code: portalResponseSuccess, r: 0.5, g: 0.5, b: nan, want: 0, ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := portalColorFromRGB(tc.code, tc.r, tc.g, tc.b)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("portalColorFromRGB(%d, %v, %v, %v) = (0x%08x, %v), want (0x%08x, %v)",
					tc.code, tc.r, tc.g, tc.b, uint32(got), ok, uint32(tc.want), tc.ok)
			}
		})
	}
}

func TestFormatX11ParentWindow(t *testing.T) {
	if got := formatX11ParentWindow(0); got != "" {
		t.Fatalf("formatX11ParentWindow(0) = %q, want empty", got)
	}
	if got := formatX11ParentWindow(0x1a2b3c); got != "x11:1a2b3c" {
		t.Fatalf("formatX11ParentWindow(0x1a2b3c) = %q, want x11:1a2b3c", got)
	}
}

func TestPortalRequestPathForSender(t *testing.T) {
	if got := portalRequestPathForSender(":1.42", "nsgo_color_1"); got != "/org/freedesktop/portal/desktop/request/1_42/nsgo_color_1" {
		t.Fatalf("got %q", got)
	}
	if got := portalRequestPathForSender("", "nsgo_color_1"); got != "" {
		t.Fatalf("empty sender = %q, want empty", got)
	}
	if got := portalRequestPathForSender(":1.42", ""); got != "" {
		t.Fatalf("empty token = %q, want empty", got)
	}
}

func TestPortalColorResultEvent(t *testing.T) {
	picked := portalColorResultEvent(4, 0x224466ff, true)
	if picked.Kind != driver.EventFiles || picked.Count != 4 || !picked.Finished || picked.Data != driver.Color(0x224466ff) {
		t.Fatalf("picked event = %+v", picked)
	}
	cancelled := portalColorResultEvent(5, 0x224466ff, false)
	if cancelled.Kind != driver.EventFiles || cancelled.Count != 5 || cancelled.Finished || cancelled.Data != nil {
		t.Fatalf("cancelled event = %+v", cancelled)
	}
}
