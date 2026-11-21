package linux

import (
	"math"
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const (
	screenshotPortalIface      = "org.freedesktop.portal.Screenshot"
	screenshotPortalMinVersion = uint32(2)

	portalResponseSuccess   = uint32(0)
	portalResponseCancelled = uint32(1)
	portalResponseOther     = uint32(2)
)

// screenshotPortalSupportsColor reports whether the XDG Desktop Portal
// Screenshot interface advertises PickColor (introduced in version 2).
func screenshotPortalSupportsColor(version uint32, ok bool) bool {
	return ok && version >= screenshotPortalMinVersion
}

// portalColorResponseOK reports whether a Request::Response code indicates a
// completed pick (0 = success, 1 = cancelled, 2 = failed).
func portalColorResponseOK(code uint32) bool {
	return code == portalResponseSuccess
}

// portalColorFromRGB converts an XDG Screenshot.PickColor response code and
// its (ddd) sRGB float64 channels in [0, 1] into an opaque driver.Color
// (0xRRGGBBAA). Cancelled/failed responses or NaN channels return (0, false).
func portalColorFromRGB(code uint32, r, g, b float64) (driver.Color, bool) {
	if !portalColorResponseOK(code) {
		return 0, false
	}
	if math.IsNaN(r) || math.IsNaN(g) || math.IsNaN(b) {
		return 0, false
	}
	part := func(v float64) uint32 {
		return uint32(max(min(v, 1), 0)*255 + 0.5)
	}
	packed := part(r)<<24 | part(g)<<16 | part(b)<<8 | 0xff
	return driver.Color(packed), true
}

// formatX11ParentWindow formats an X11 window ID for XDG Desktop Portal's
// parent_window argument ("x11:<hex-xid>"), or "" when xid is 0.
func formatX11ParentWindow(xid uint64) string {
	if xid == 0 {
		return ""
	}
	return "x11:" + strconv.FormatUint(xid, 16)
}

// portalRequestPathForSender derives the predictable Request object path
// xdg-desktop-portal uses for a D-Bus connection unique name and handle_token.
func portalRequestPathForSender(uniqueName, token string) string {
	if uniqueName == "" || token == "" {
		return ""
	}
	sanitized := strings.ReplaceAll(strings.TrimPrefix(uniqueName, ":"), ".", "_")
	return "/org/freedesktop/portal/desktop/request/" + sanitized + "/" + token
}

func portalColorResultEvent(id uint32, c driver.Color, ok bool) driver.Event {
	if !ok {
		return driver.Event{Kind: driver.EventFiles, Count: id}
	}
	return driver.Event{Kind: driver.EventFiles, Count: id, Finished: true, Data: c}
}
