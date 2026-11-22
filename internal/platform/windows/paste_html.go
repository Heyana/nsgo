package windows

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"
)

// This file builds on every system, so unit tests check Windows' CF_HTML parsing on any host.

const (
	cfHTMLStartFragmentMarker = "<!--StartFragment-->"
	cfHTMLEndFragmentMarker   = "<!--EndFragment-->"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// parseCFHTML extracts the pasted HTML from a Windows CF_HTML ("HTML Format") clipboard buffer. CF_HTML wraps UTF-8 HTML with an ASCII header whose byte offsets name the HTML document (StartHTML..EndHTML) and the pasted fragment (StartFragment..EndFragment). Trailing NUL padding from GlobalSize is trimmed before slicing, malformed or non-UTF-8-aligned offsets fall back to the fragment comment markers, StartHTML..EndHTML, or the body after the header, and raw HTML without a CF_HTML header is returned as-is.
func parseCFHTML(raw []byte) string {
	if i := bytes.IndexByte(raw, 0); i >= 0 {
		raw = raw[:i]
	}
	if len(raw) == 0 {
		return ""
	}

	var (
		hasHeader                bool
		startHTML, endHTML       int
		hasStartHTML, hasEndHTML bool
		startFrag, endFrag       int
		hasStartFrag, hasEndFrag bool
		headerEnd                int
	)

	pos := 0
	limit := len(raw)
	for pos < limit {
		nl := bytes.IndexByte(raw[pos:limit], '\n')
		var lineEnd, nextPos int
		if nl < 0 {
			lineEnd = limit
			nextPos = limit
		} else {
			lineEnd = pos + nl
			nextPos = lineEnd + 1
		}
		line := raw[pos:lineEnd]
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if pos == 0 {
			line = bytes.TrimPrefix(line, utf8BOM)
		}
		if len(line) == 0 || line[0] == '<' {
			headerEnd = pos
			break
		}
		colon := bytes.IndexByte(line, ':')
		if colon <= 0 {
			headerEnd = pos
			break
		}
		key := strings.TrimSpace(string(line[:colon]))
		if !isCFHTMLHeaderKey(key) {
			headerEnd = pos
			break
		}
		val := strings.TrimSpace(string(line[colon+1:]))
		hasHeader = true
		headerEnd = nextPos
		if n, err := strconv.Atoi(val); err == nil {
			switch {
			case strings.EqualFold(key, "StartHTML"):
				startHTML, hasStartHTML = n, true
				if n > pos && n < limit {
					limit = n
				}
			case strings.EqualFold(key, "EndHTML"):
				endHTML, hasEndHTML = n, true
			case strings.EqualFold(key, "StartFragment"):
				startFrag, hasStartFrag = n, true
				if n > pos && n < limit {
					limit = n
				}
			case strings.EqualFold(key, "EndFragment"):
				endFrag, hasEndFrag = n, true
			}
		}
		pos = nextPos
	}

	// 1. Prefer StartFragment..EndFragment when valid and UTF-8 aligned.
	if hasStartFrag && hasEndFrag && startFrag >= 0 && endFrag >= startFrag && startFrag <= len(raw) {
		if startFrag == endFrag {
			return ""
		}
		end := min(endFrag, len(raw))
		if isUTF8Boundaries(raw, startFrag, end) {
			return trimUTF8BOM(string(raw[startFrag:end]))
		}
	}

	// 2. Fall back to <!--StartFragment-->..<!--EndFragment--> markers.
	var htmlRegion []byte
	validHTML := hasStartHTML && hasEndHTML && startHTML >= 0 && endHTML > startHTML && startHTML < len(raw)
	if validHTML {
		end := min(endHTML, len(raw))
		if isUTF8Boundaries(raw, startHTML, end) {
			htmlRegion = raw[startHTML:end]
		}
	}
	searchRegion := htmlRegion
	if searchRegion == nil {
		searchRegion = raw[headerEnd:]
	}
	if frag, ok := extractCFHTMLMarkers(searchRegion); ok {
		return frag
	}

	// 3. Fall back to StartHTML..EndHTML.
	if len(htmlRegion) > 0 {
		return trimUTF8BOM(string(htmlRegion))
	}

	// 4. Fall back to the payload after the header, or raw HTML if no header existed.
	if hasHeader {
		body := bytes.TrimLeft(raw[headerEnd:], "\r\n")
		return trimUTF8BOM(string(body))
	}
	return trimUTF8BOM(string(raw))
}

func isCFHTMLHeaderKey(key string) bool {
	switch {
	case strings.EqualFold(key, "Version"),
		strings.EqualFold(key, "StartHTML"),
		strings.EqualFold(key, "EndHTML"),
		strings.EqualFold(key, "StartFragment"),
		strings.EqualFold(key, "EndFragment"),
		strings.EqualFold(key, "StartSelection"),
		strings.EqualFold(key, "EndSelection"),
		strings.EqualFold(key, "SourceURL"),
		strings.EqualFold(key, "Description"):
		return true
	default:
		return false
	}
}

func isUTF8Boundaries(b []byte, start, end int) bool {
	if start < 0 || end < start || end > len(b) {
		return false
	}
	if start < len(b) && !utf8.RuneStart(b[start]) {
		return false
	}
	if end < len(b) && !utf8.RuneStart(b[end]) {
		return false
	}
	return true
}

func extractCFHTMLMarkers(region []byte) (string, bool) {
	s := bytes.Index(region, []byte(cfHTMLStartFragmentMarker))
	if s < 0 {
		return "", false
	}
	s += len(cfHTMLStartFragmentMarker)
	e := bytes.LastIndex(region[s:], []byte(cfHTMLEndFragmentMarker))
	if e < 0 {
		return "", false
	}
	return trimUTF8BOM(string(region[s : s+e])), true
}

func trimUTF8BOM(s string) string {
	return strings.TrimPrefix(s, "\uFEFF")
}
