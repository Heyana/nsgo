//go:build linux && !android

package linux

import (
	"fmt"
	"testing"
)

func TestPreviewProtocolsAreIntrospected(t *testing.T) {
	for _, tc := range []struct {
		iface, sig string
		ok         bool
	}{{previewV2, "ssbs", true}, {previewV2, "ssb", true}, {previewName, "sub", true},
		{previewV2, "sub", false}, {previewName, "ssb", false}, {previewV2, "ss", false}, {"other", "ssbs", false}} {
		t.Run(tc.iface+"/"+tc.sig, func(t *testing.T) {
			args := ""
			for _, typ := range tc.sig {
				args += fmt.Sprintf(`<arg type="%c" direction="in"/>`, typ)
			}
			data := fmt.Sprintf(`<node><interface name="%s"><method name="ShowFile">%s</method><property name="ParentHandle" type="s"/><property name="Visible" type="b"/></interface></node>`, tc.iface, args)
			p, ok := previewProtocolOf(data)
			if ok != tc.ok || ok && (p.iface != tc.iface || p.signature != tc.sig || p.selection != (tc.iface == previewV2)) {
				t.Fatalf("protocol = %+v, %v", p, ok)
			}
		})
	}
	if _, ok := previewProtocolOf("not XML"); ok {
		t.Fatal("an invalid introspection reply accepted")
	}
}

func TestPreviewSelectionAndFileURI(t *testing.T) {
	for _, tc := range []struct {
		index, count int
		direction    uint32
		next         int
		changed      bool
	}{{1, 3, 2, 0, true}, {1, 3, 4, 0, true}, {1, 3, 3, 2, true}, {1, 3, 5, 2, true},
		{0, 3, 4, 0, false}, {2, 3, 5, 2, false}, {1, 3, 0, 1, false}, {0, 0, 5, 0, false}} {
		if next, changed := previewSelection(tc.index, tc.count, tc.direction); next != tc.next || changed != tc.changed {
			t.Errorf("selection %+v = %d, %v", tc, next, changed)
		}
	}
	if got := previewFileURI("/tmp/a #? 日本語.png"); got != "file:///tmp/a%20%23%3F%20%E6%97%A5%E6%9C%AC%E8%AA%9E.png" {
		t.Fatalf("preview URI = %q", got)
	}
}
