package textview

import "strings"

// parseContainerOpen checks whether line opens a :::name container registered
// in names (container keyword → Custom.Name). Returns the Custom name, parsed
// attributes, colon count and true on match.
func parseContainerOpen(line string, names map[string]string) (name string, attrs map[string]string, colons int, ok bool) {
	if len(names) == 0 {
		return "", nil, 0, false
	}
	trimmed := strings.TrimRight(line, " \t\r\n")
	// Up to 3 leading spaces.
	lead := 0
	for lead < len(trimmed) && lead < 4 && trimmed[lead] == ' ' {
		lead++
	}
	if lead >= 4 {
		return "", nil, 0, false
	}
	rest := trimmed[lead:]
	colons = len(rest) - len(strings.TrimLeft(rest, ":"))
	if colons < 3 {
		return "", nil, 0, false
	}
	rest = rest[colons:]
	end := strings.IndexFunc(rest, func(c rune) bool {
		return !(c == '-' || c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z')
	})
	if end < 0 {
		end = len(rest)
	}
	name, ok = names[rest[:end]]
	if !ok {
		return "", nil, 0, false
	}
	set := func(k, v string) {
		if attrs == nil {
			attrs = map[string]string{}
		}
		attrs[k] = v
	}
	rest = rest[end:]
	if strings.HasPrefix(rest, "[") {
		if close := strings.IndexByte(rest, ']'); close > 0 {
			set("label", rest[1:close])
			rest = rest[close+1:]
		}
	}
	if strings.HasPrefix(rest, "{") {
		if close := strings.IndexByte(rest, '}'); close > 0 {
			parseContainerAttrs(rest[1:close], set, func(k string) string { return attrs[k] })
		}
	}
	return name, attrs, colons, true
}

// isContainerClose reports whether line closes a container opened with colons
// colons (a line of at least colons colons and optional surrounding spaces).
func isContainerClose(line string, colons int) bool {
	trimmed := strings.TrimSpace(line)
	return len(trimmed) >= colons && len(strings.Trim(trimmed, ":")) == 0
}

// parseContainerAttrs reads key=value, key="quoted value", .class and #id.
func parseContainerAttrs(s string, set func(k, v string), get func(k string) string) {
	for {
		if s = strings.TrimLeft(s, " \t"); s == "" {
			return
		}
		end := strings.IndexAny(s, " \t=")
		if end >= 0 && s[end] == '=' {
			key, value := s[:end], s[end+1:]
			if strings.HasPrefix(value, `"`) {
				value, s, _ = strings.Cut(value[1:], `"`)
			} else if i := strings.IndexAny(value, " \t"); i >= 0 {
				value, s = value[:i], value[i:]
			} else {
				s = ""
			}
			set(key, value)
			continue
		}
		if end < 0 {
			end = len(s)
		}
		word := s[:end]
		s = s[end:]
		switch {
		case strings.HasPrefix(word, "."):
			set("class", strings.TrimSpace(get("class")+" "+word[1:]))
		case strings.HasPrefix(word, "#"):
			set("id", word[1:])
		default:
			set(word, "")
		}
	}
}
