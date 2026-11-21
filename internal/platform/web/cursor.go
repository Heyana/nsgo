//go:build js && wasm

package web

import (
	"strings"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/webstyle"
)

const maxWebCursorURLs = 64

// cursorURL resolves src into a browser URL suitable for CSS cursor: url(...),
// using a data: URL for inline bytes or Pixels so nothing needs object URL
// revocation. Results are cached on s when non-nil.
func (s *surface) cursorURL(src driver.ImageSource) string {
	if src.Empty() {
		return ""
	}
	var key string
	if s != nil {
		key = src.Key()
		if u, ok := s.cursorURLs[key]; ok {
			return u
		}
	}
	var u string
	switch {
	case src.Pixels != nil && !src.Pixels.Rect.Empty():
		canvas := js.Global().Get("document").Call("createElement", "canvas")
		newPixelCanvas(canvas).put(driver.CopyPixels(src.Pixels), 0)
		u = canvas.Call("toDataURL", "image/png").String()
	case len(src.Data) > 0:
		u = webstyle.DataURL(src.Data)
	case strings.HasPrefix(src.URI, "asset:"):
		u = resourceURL(strings.TrimPrefix(src.URI, "asset:"))
	default:
		u = src.URI
	}
	if s != nil && key != "" {
		if s.cursorURLs == nil {
			s.cursorURLs = map[string]string{}
		}
		if len(s.cursorURLs) >= maxWebCursorURLs {
			clear(s.cursorURLs)
		}
		s.cursorURLs[key] = u
	}
	return u
}
