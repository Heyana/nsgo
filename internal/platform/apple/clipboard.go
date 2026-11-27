//go:build darwin

package apple

import (
	"slices"
	"strings"
)

// PasteboardTypes maps a clipboard format name to native Apple pasteboard UTIs.
// Native UTIs (without '/') pass through unchanged; MIME media types expand to
// standard UTIs and their UniformTypeIdentifiers equivalent so NSPasteboardItem
// and UIPasteboard accept them.
func PasteboardTypes(name string) []string {
	if !strings.ContainsRune(name, '/') {
		if name == "" {
			return nil
		}
		return []string{name}
	}
	base := strings.TrimSpace(strings.ToLower(name))
	if semi := strings.IndexByte(base, ';'); semi >= 0 {
		base = strings.TrimSpace(base[:semi])
	}
	var out []string
	add := func(uti string) {
		if uti != "" && !slices.Contains(out, uti) {
			out = append(out, uti)
		}
	}
	switch base {
	case "text/plain":
		add("public.utf8-plain-text")
		return out
	case "application/mathml+xml":
		add("public.mathml")
		add("public.mathml.presentation")
	case "text/x-tex", "application/x-tex":
		add("org.tug.tex")
	}
	if ut := UTType().Send(sel("typeWithMIMEType:"), NSString(base)); ut != 0 {
		add(GoString(ut.Send(sel("identifier"))))
	}
	return out
}
