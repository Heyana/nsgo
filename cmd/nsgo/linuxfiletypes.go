package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/mimeinfo"
)

// linuxFileTypes returns the desktop entry's media types and the shared MIME-info definitions of unknown extensions the app owns. Explicit MIME entries refer to existing system types; the app's generated application/x-<id> may also be named explicitly.
func (p *project) linuxFileTypes() (types []string, definitions []byte, err error) {
	var xml strings.Builder
	add := func(m string) {
		if !slices.Contains(types, m) {
			types = append(types, m)
		}
	}
	for _, f := range p.FileTypes {
		custom := "application/x-" + strings.ToLower(f.ID)
		var extensions []string
		if len(f.MIME) > 0 {
			for _, m := range f.MIME {
				add(m)
			}
			if fileTypeOwned(f, p.ID) && slices.Contains(f.MIME, custom) {
				extensions = f.Extensions
			}
		} else {
			for _, ext := range f.Extensions {
				if m := mimeinfo.ForExtension(ext); m != "" {
					add(m)
				} else if !fileTypeOwned(f, p.ID) {
					return nil, nil, fmt.Errorf("file_types %q: Linux needs mime for the unknown extension %q of an imported type", f.Name, ext)
				} else {
					extensions = append(extensions, ext)
				}
			}
		}
		if len(extensions) == 0 {
			continue
		}
		add(custom)
		fmt.Fprintf(&xml, "  <mime-type type=\"%s\">\n    <comment>%s</comment>\n", xmlText(custom), xmlText(f.Name))
		for _, ext := range extensions {
			fmt.Fprintf(&xml, "    <glob pattern=\"*.%s\"/>\n", xmlText(strings.ToLower(ext)))
		}
		xml.WriteString("  </mime-type>\n")
	}
	if xml.Len() > 0 {
		definitions = []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<mime-info xmlns=\"http://www.freedesktop.org/standards/shared-mime-info\">\n" + xml.String() + "</mime-info>\n")
	}
	return types, definitions, nil
}

// linuxRegisters reports whether the app's desktop entry registers anything with the system, file types or URL schemes, whose caches the package's scripts refresh.
func (p *project) linuxRegisters() bool { return len(p.FileTypes) > 0 || len(p.URLSchemes) > 0 }

// linuxCacheScript refreshes desktop registration after install and uninstall. A machine without a desktop can install the package too.
const linuxCacheScript = `#!/bin/sh
set -e
if command -v update-mime-database >/dev/null 2>&1; then
  update-mime-database /usr/share/mime
fi
if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database -q /usr/share/applications
fi
`

// Arch normally refreshes these caches through pacman's desktop and MIME hooks. The install script refreshes them when the tools are available, without ever choosing the user's default handler.
const archCacheScript = `post_install() {
  if command -v update-mime-database >/dev/null 2>&1; then
    update-mime-database /usr/share/mime
  fi
  if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database -q /usr/share/applications
  fi
}
post_upgrade() { post_install; }
post_remove() { post_install; }
`
