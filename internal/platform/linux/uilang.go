package linux

import "strings"

// gnuLanguageList converts a BCP 47 tag into a colon-separated GNU gettext
// LANGUAGE priority list (as GTK 4 and GLib read from /usr/share/locale), or
// "" when lang is empty, invalid, or a pseudo locale.
func gnuLanguageList(lang string) string {
	language, script, region, variant, ok := parseBCP47(lang)
	if !ok {
		return ""
	}
	var out []string
	add := func(s string) {
		for _, prev := range out {
			if prev == s {
				return
			}
		}
		out = append(out, s)
	}

	switch language {
	case "zh":
		switch script {
		case "Hans":
			if region != "" {
				add("zh_" + region)
			}
			add("zh_CN")
			add("zh")
		case "Hant":
			if region != "" {
				add("zh_" + region)
			}
			if region == "MO" {
				add("zh_HK")
			}
			add("zh_TW")
			add("zh_HK")
			add("zh")
		default:
			switch region {
			case "HK":
				add("zh_HK")
				add("zh_TW")
				add("zh")
			case "MO":
				add("zh_MO")
				add("zh_HK")
				add("zh_TW")
				add("zh")
			case "TW":
				add("zh_TW")
				add("zh_HK")
				add("zh")
			case "":
				add("zh_CN")
				add("zh")
			default:
				add("zh_" + region)
				add("zh")
			}
		}
	default:
		mod := gnuModifier(language, script, variant)
		if mod != "" {
			if region != "" {
				add(language + "_" + region + "@" + mod)
			}
			add(language + "@" + mod)
		} else if region != "" {
			add(language + "_" + region)
		}
		add(language)
		// English is gettext's untranslated source language: ending with C
		// stops gettext at English even when the session's LC_MESSAGES is not.
		if language == "en" {
			add("C")
		}
	}
	return strings.Join(out, ":")
}

// gnuModifier returns the GNU gettext @modifier corresponding to a BCP 47
// script or variant where /usr/share/locale uses one.
func gnuModifier(language, script, variant string) string {
	if language == "ca" && variant == "valencia" {
		return "valencia"
	}
	switch script {
	case "Latn":
		switch language {
		case "sr", "be", "uz", "bs", "az":
			return "latin"
		}
	case "Cyrl":
		switch language {
		case "uz", "az":
			return "cyrillic"
		}
	}
	return ""
}

// lcMessagesCandidates returns UTF-8 POSIX locale names to try with
// setlocale(LC_MESSAGES, ...) when the process's current LC_MESSAGES is C or
// POSIX (under which glibc's gettext ignores LANGUAGE).
func lcMessagesCandidates(lang string) []string {
	gnu := gnuLanguageList(lang)
	var out []string
	add := func(s string) {
		for _, prev := range out {
			if prev == s {
				return
			}
		}
		out = append(out, s)
	}
	if gnu != "" {
		first, _, _ := strings.Cut(gnu, ":")
		base, mod, hasMod := strings.Cut(first, "@")
		if strings.Contains(base, "_") {
			if hasMod {
				add(base + ".UTF-8@" + mod)
				add(base + ".utf8@" + mod)
			} else {
				add(base + ".UTF-8")
				add(base + ".utf8")
			}
		}
	}
	add("C.UTF-8")
	add("C.utf8")
	add("en_US.UTF-8")
	return out
}

// isPOSIXLocale reports whether loc is the bare C/POSIX locale under which
// glibc's gettext ignores the LANGUAGE environment variable.
func isPOSIXLocale(loc string) bool {
	return loc == "" || loc == "C" || loc == "POSIX"
}

// parseBCP47 extracts lowercased language, titlecased 4-letter script,
// uppercased 2-letter alpha region, and optional variant from a BCP 47 tag.
// Returns ok=false for empty, malformed, or pseudo locales.
func parseBCP47(lang string) (language, script, region, variant string, ok bool) {
	s := strings.ReplaceAll(strings.TrimSpace(lang), "_", "-")
	if s == "" {
		return "", "", "", "", false
	}
	parts := strings.Split(s, "-")
	var clean []string
	for i, p := range parts {
		if p == "" {
			return "", "", "", "", false
		}
		// Drop BCP 47 singleton extensions (-u-..., -t-..., -x-...).
		if i > 0 && len(p) == 1 {
			break
		}
		if !isASCIIAlnum(p) {
			return "", "", "", "", false
		}
		clean = append(clean, p)
	}
	if len(clean) == 0 {
		return "", "", "", "", false
	}
	full := strings.Join(clean, "-")
	if strings.EqualFold(full, "en-XA") || strings.EqualFold(full, "ar-XB") {
		return "", "", "", "", false
	}
	if len(clean[0]) < 2 || len(clean[0]) > 3 || !isASCIILetters(clean[0]) {
		return "", "", "", "", false
	}
	language = strings.ToLower(clean[0])
	idx := 1
	if idx < len(clean) && len(clean[idx]) == 4 && isASCIILetters(clean[idx]) {
		p := clean[idx]
		script = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		idx++
	}
	if idx < len(clean) {
		p := clean[idx]
		if len(p) == 2 && isASCIILetters(p) {
			region = strings.ToUpper(p)
			idx++
		} else if len(p) == 3 && isASCIIDigits(p) {
			// UN M.49 numeric region (e.g. es-419): recognized as region,
			// omitted from GNU locale names.
			idx++
		}
	}
	if idx < len(clean) && len(clean[idx]) >= 5 {
		variant = strings.ToLower(clean[idx])
	}
	return language, script, region, variant, true
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

func isASCIILetters(s string) bool {
	for i := range len(s) {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return true
}

func isASCIIDigits(s string) bool {
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
