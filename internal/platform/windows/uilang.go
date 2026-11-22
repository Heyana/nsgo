package windows

import (
	"strings"
	"unicode/utf16"
)

// windowsLocaleNameMax is Windows' LOCALE_NAME_MAX_LENGTH minus its NUL.
const windowsLocaleNameMax = 84

// windowsUILanguageCandidates returns the ordered BCP 47 tags to try with
// SetThreadPreferredUILanguages (MUI_LANGUAGE_NAME), from most specific to
// primary language, or nil when lang is empty, invalid, or a pseudo locale.
func windowsUILanguageCandidates(lang string) []string {
	s := strings.ReplaceAll(strings.TrimSpace(lang), "_", "-")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "-")
	var clean []string
	for i, p := range parts {
		if p == "" {
			return nil
		}
		// Drop BCP 47 singleton extensions (-u-..., -t-..., -x-...).
		if i > 0 && len(p) == 1 {
			break
		}
		if !isASCIIAlnum(p) {
			return nil
		}
		clean = append(clean, p)
	}
	if len(clean) == 0 {
		return nil
	}
	full := strings.Join(clean, "-")
	if len(full) > windowsLocaleNameMax {
		return nil
	}
	if strings.EqualFold(full, "en-XA") || strings.EqualFold(full, "ar-XB") {
		return nil
	}
	// Strip trailing subtags one by one as MUI fallbacks if Windows rejects
	// a specific variant or region tag.
	out := make([]string, 0, len(clean))
	for n := len(clean); n >= 1; n-- {
		out = append(out, strings.Join(clean[:n], "-"))
	}
	return out
}

func isASCIIAlnum(s string) bool {
	for i := range len(s) {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// utf16MultiString encodes tags as a Windows PCZZWSTR double-null-terminated
// UTF-16 list, returning nil if tags is empty.
func utf16MultiString(tags ...string) []uint16 {
	var out []uint16
	for _, t := range tags {
		if t == "" {
			continue
		}
		out = append(out, utf16.Encode([]rune(t))...)
		out = append(out, 0)
	}
	if len(out) == 0 {
		return nil
	}
	return append(out, 0)
}
