//go:build linux && !android

package linux

import (
	"encoding/xml"
	"net/url"
	"strings"
)

const (
	previewName = "org.gnome.NautilusPreviewer"
	previewPath = "/org/gnome/NautilusPreviewer"
	previewV2   = "org.gnome.NautilusPreviewer2"
)

// previewProtocol is the previewer's introspected contract. Current Sushi
// takes an activation token; earlier versions use three arguments, or an
// X11 window ID instead of a parent handle. Reject other signatures.
type previewProtocol struct {
	iface, signature string
	selection        bool
}

func previewProtocolOf(data string) (previewProtocol, bool) {
	type arg struct {
		Type      string `xml:"type,attr"`
		Direction string `xml:"direction,attr"`
	}
	type method struct {
		Name string `xml:"name,attr"`
		Args []arg  `xml:"arg"`
	}
	var node struct {
		Interfaces []struct {
			Name       string   `xml:"name,attr"`
			Methods    []method `xml:"method"`
			Properties []struct {
				Name string `xml:"name,attr"`
				Type string `xml:"type,attr"`
			} `xml:"property"`
		} `xml:"interface"`
	}
	if xml.Unmarshal([]byte(data), &node) != nil {
		return previewProtocol{}, false
	}
	for _, name := range []string{previewV2, previewName} {
		for _, iface := range node.Interfaces {
			if iface.Name != name {
				continue
			}
			for _, m := range iface.Methods {
				if m.Name != "ShowFile" {
					continue
				}
				var sig strings.Builder
				for _, a := range m.Args {
					if a.Direction != "out" {
						sig.WriteString(a.Type)
					}
				}
				p := previewProtocol{iface: name, signature: sig.String()}
				if name == previewV2 && (p.signature == "ssbs" || p.signature == "ssb") {
					parent, visible := false, false
					for _, prop := range iface.Properties {
						parent = parent || prop.Name == "ParentHandle" && prop.Type == "s"
						visible = visible || prop.Name == "Visible" && prop.Type == "b"
					}
					p.selection = parent && visible
					return p, true
				}
				if name == previewName && p.signature == "sub" {
					return p, true
				}
			}
		}
	}
	return previewProtocol{}, false
}

func previewFileURI(path string) string { return (&url.URL{Scheme: "file", Path: path}).String() }

// previewSelection maps GTK's directional events to the requested files,
// stopping at the ends. A tab move is not a file-selection event.
func previewSelection(index, count int, direction uint32) (int, bool) {
	if count <= 0 {
		return 0, false
	}
	next := index
	switch direction {
	case 2, 4: // GTK_DIR_UP, GTK_DIR_LEFT
		next--
	case 3, 5: // GTK_DIR_DOWN, GTK_DIR_RIGHT
		next++
	default:
		return index, false
	}
	next = min(max(next, 0), count-1)
	return next, next != index
}
