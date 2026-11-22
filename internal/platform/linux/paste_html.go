package linux

import (
	"bytes"
	"encoding/binary"
	"unicode/utf16"
)

// This file builds on every system, so unit tests check Linux's pasted HTML
// MIME matching, chunk capping and UTF-8/UTF-16 decoding on any host.

const (
	pasteHTMLChunk = 64 << 10 // 64 KiB per asynchronous GIO read
	pasteHTMLMax   = 16 << 20 // 16 MiB cap on pasted HTML bytes
)

// pasteHTMLMIMETypes are the MIME types requested from GDK when pasting HTML.
var pasteHTMLMIMETypes = []string{"text/html", "text/html;charset=utf-8"}

// hasPasteHTMLFormat reports whether the clipboard offers any HTML MIME type.
func hasPasteHTMLFormat(contains func(mime string) bool) bool {
	for _, mime := range pasteHTMLMIMETypes {
		if contains(mime) {
			return true
		}
	}
	return false
}

// appendPasteHTML appends chunk to dst up to pasteHTMLMax bytes and reports
// whether the cap has been reached.
func appendPasteHTML(dst, chunk []byte) ([]byte, bool) {
	if len(dst) >= pasteHTMLMax {
		return dst, true
	}
	if remain := pasteHTMLMax - len(dst); len(chunk) > remain {
		return append(dst, chunk[:remain]...), true
	}
	dst = append(dst, chunk...)
	return dst, len(dst) >= pasteHTMLMax
}

// decodeClipboardHTML decodes raw bytes read from a Linux text/html clipboard
// stream into UTF-8. Some X11 applications (notably Firefox) place UTF-16 with
// a byte-order mark in text/html, while GTK/Chromium/Wayland applications
// provide UTF-8 with an optional BOM and/or trailing NUL byte.
func decodeClipboardHTML(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	if len(raw) >= 2 && raw[0] == 0xFF && raw[1] == 0xFE {
		return decodeUTF16HTML(raw[2:], binary.LittleEndian)
	}
	if len(raw) >= 2 && raw[0] == 0xFE && raw[1] == 0xFF {
		return decodeUTF16HTML(raw[2:], binary.BigEndian)
	}
	if len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF {
		raw = raw[3:]
	}
	raw = bytes.TrimRight(raw, "\x00")
	return string(raw)
}

func decodeUTF16HTML(raw []byte, order binary.ByteOrder) string {
	n := len(raw) / 2
	if n == 0 {
		return ""
	}
	units := make([]uint16, n)
	for i := range n {
		units[i] = order.Uint16(raw[i*2:])
	}
	for len(units) > 0 && units[len(units)-1] == 0 {
		units = units[:len(units)-1]
	}
	if len(units) == 0 {
		return ""
	}
	return string(utf16.Decode(units))
}
