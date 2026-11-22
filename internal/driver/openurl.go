package driver

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

// MaxURLLength is the longest URL OpenURL opens, in bytes: a mailto: link with a body fits.
const MaxURLLength = 16 << 10

// The schemes OpenURL never opens: they run code or reach the app's own files and data rather than another app.
var refusedSchemes = []string{"javascript", "vbscript", "data", "blob", "file", "about", "asset"}

// CheckURL reports why OpenURL would refuse u: it must be absolute, at most MaxURLLength bytes, without spaces or control characters, a web address with a host, and of a scheme that opens another app, not javascript:, data:, blob:, file:, about: or asset:.
func CheckURL(u string) error {
	if len(u) > MaxURLLength {
		return fmt.Errorf("core: a URL has at most %d bytes, this one %d", MaxURLLength, len(u))
	}
	if strings.IndexFunc(u, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return errors.New("core: a URL has no spaces or control characters")
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("core: not a URL: %w", err)
	}
	if parsed.Scheme == "" {
		return fmt.Errorf("core: %q has no scheme", u)
	}
	scheme := strings.ToLower(parsed.Scheme)
	for _, s := range refusedSchemes {
		if scheme == s {
			return fmt.Errorf("core: %s: URLs are not opened", scheme)
		}
	}
	if (scheme == "http" || scheme == "https") && parsed.Host == "" {
		return fmt.Errorf("core: a web address has a host (%s)", u)
	}
	return nil
}
