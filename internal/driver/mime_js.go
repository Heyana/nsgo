//go:build js

package driver

import (
	"strings"

	"github.com/nsgo-dev/nsgo/internal/mimeinfo"
)

// On js/wasm there is no host /etc/mime.types file, so use mimeinfo's fixed
// table instead of importing stdlib mime (whose init and RFC 2231 formatters
// add ~40 KB of wasm code to every web and mini-program binary).
func sysTypeByExtension(ext string) string {
	t := mimeinfo.ForExtension(strings.TrimPrefix(ext, "."))
	if strings.HasPrefix(t, "text/") {
		return t + "; charset=utf-8"
	}
	return t
}

func sysExtensionsByType(string) []string { return nil }
