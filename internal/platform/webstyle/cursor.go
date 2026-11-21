package webstyle

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var cursorURLEscape = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\a `)

func cursorCSSURL(u string) string { return `url("` + cursorURLEscape.Replace(u) + `")` }

// DataURL encodes image bytes into a data: URL with the sniffed media type
// (SVG, PNG, JPEG, GIF, WebP, defaulting to image/png).
func DataURL(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	mime := driver.SniffImage(data)
	if mime == "" {
		if head := data[:min(len(data), 512)]; bytes.Contains(head, []byte("<svg")) {
			mime = "image/svg+xml"
		} else {
			mime = "image/png"
		}
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// CustomCursorDeclarations returns the CSS cursor property values for a view's
// named and custom cursors, in progressive-enhancement order: a universal
// url() declaration followed by -webkit-image-set() and image-set() when a 2x
// representation is present so HiDPI displays select it automatically.
// Returns nil when no cursor property should be emitted.
func CustomCursorDeclarations(named driver.Cursor, custom *driver.CustomCursor, url1x, url2x string, dpr float32) []string {
	if custom == nil {
		if named == driver.CursorAuto {
			return nil
		}
		return []string{named.Name()}
	}
	fb := custom.EffectiveFallback(named)
	if url1x == "" && url2x == "" {
		if fb == driver.CursorAuto {
			return nil
		}
		return []string{fb.Name()}
	}
	kw := fb.Name()
	if fb == driver.CursorAuto {
		kw = driver.CursorDefault.Name()
	}
	hot := driver.ClampHotspot(custom.Hotspot, custom.LogicalSize())
	hx := int(math.Round(float64(hot.X)))
	hy := int(math.Round(float64(hot.Y)))

	switch {
	case url1x != "" && url2x != "":
		u1, u2 := cursorCSSURL(url1x), cursorCSSURL(url2x)
		// Emit url() first for browsers without image-set in cursor, then
		// -webkit-image-set and standard image-set so HiDPI displays pick 2x.
		primary := u1
		if dpr >= driver.CustomCursorHiDPIScale {
			// Still keep 1x in plain url() so browsers without image-set don't
			// draw the 2x bitmap at double logical size; image-set overrides it
			// on supporting browsers.
			primary = u1
		}
		return []string{
			fmt.Sprintf("%s %d %d, %s", primary, hx, hy, kw),
			fmt.Sprintf("-webkit-image-set(%s 1x, %s 2x) %d %d, %s", u1, u2, hx, hy, kw),
			fmt.Sprintf("image-set(%s 1x, %s 2x) %d %d, %s", u1, u2, hx, hy, kw),
		}
	case url1x != "":
		return []string{
			fmt.Sprintf("%s %d %d, %s", cursorCSSURL(url1x), hx, hy, kw),
		}
	default:
		u2 := cursorCSSURL(url2x)
		return []string{
			fmt.Sprintf("%s %d %d, %s", u2, hx, hy, kw),
			fmt.Sprintf("-webkit-image-set(%s 2x) %d %d, %s", u2, hx, hy, kw),
			fmt.Sprintf("image-set(%s 2x) %d %d, %s", u2, hx, hy, kw),
		}
	}
}
