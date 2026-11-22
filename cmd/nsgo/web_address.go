package main

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

// webBase fixes relative assets to the deployed app directory when the page
// itself may be reloaded at a deeper application address.
func (p *project) webBase() (string, error) {
	raw := p.Web.Base
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.ContainsAny(raw, "?#\\") || strings.IndexFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 || u.IsAbs() || u.Host != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("web.base must be an absolute path on this origin, such as / or /my-app/")
	}
	return strings.TrimSuffix(u.String(), "/") + "/", nil
}
