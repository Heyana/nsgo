package apk

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	androidruntime "github.com/nsgo-dev/nsgo/internal/platform/android/runtime"
)

func TestWidgetProvidersAndResources(t *testing.T) {
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []struct {
		name, prefix string
		write        func(io.Writer, []byte, Manifest, []File, *Key) error
	}{
		{"apk", "", Write}, {"aab", "base/", WriteBundle},
	} {
		t.Run(format.name, func(t *testing.T) {
			m := Manifest{Package: "dev.nsgo.widgets", Label: "Widgets", VersionCode: 1, LoopbackCleartext: true, Game: true, Names: map[string]string{"": "Widgets"}, Widgets: []Widget{
				{Kind: "counter", Names: map[string]string{"": "Counter", "zh": "计数"}, Descriptions: map[string]string{"": "Count of items"}, Families: []string{"small", "medium"}},
				{Kind: "weather", Names: map[string]string{"": "Weather"}},
			}}
			root, err := appManifest(androidruntime.Manifest, m)
			if err != nil {
				t.Fatal(err)
			}
			app := child(root, "application")
			// Components remain stable when authors reorder or add kinds.
			found := map[string]bool{}
			for _, n := range app.kids {
				if n.name.Local != "receiver" {
					continue
				}
				for _, a := range n.attrs {
					if a.Name.Local == "name" && strings.HasPrefix(a.Value, widgetComponentPrefix) {
						found[a.Value] = true
					}
				}
			}
			if len(found) != 2 || !found[widgetComponent("counter")] || !found[widgetComponent("weather")] {
				t.Fatalf("providers %v", found)
			}
			var out bytes.Buffer
			if err := format.write(&out, androidruntime.Manifest, m, nil, key); err != nil {
				t.Fatal(err)
			}
			archive, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
			if err != nil {
				t.Fatal(err)
			}
			for _, file := range []string{"res/xml/nsgo_widget_0.xml", "res/xml/nsgo_widget_1.xml", "res/layout/nsgo_widget.xml", "res/layout/nsgo_widget_timer.xml"} {
				r, err := archive.Open(format.prefix + file)
				if err != nil {
					t.Fatal(err)
				}
				data, _ := io.ReadAll(r)
				r.Close()
				if len(data) < 16 {
					t.Fatalf("empty resource %s", file)
				}
			}
			if got := layoutResourceID(m); got != 0x7f030000 {
				t.Fatalf("layout ID %#x, want %#x", got, uint32(0x7f030000))
			}
			for i, res := range m.widgetXMLResources() {
				encoded, err := encodeXML(res.node(), false)
				if err != nil {
					t.Fatal(err)
				}
				n := decodeXML(t, encoded)
				if n.root.value(t, "initialLayout") != layoutResourceID(m) || n.root.value(t, "minWidth") != 158<<8|1 {
					t.Fatalf("provider%d geometry %+v", i, n)
				}
			}
		})
	}
}

func TestWidgetDimensionAndLayoutValues(t *testing.T) {
	for value, want := range map[string]uint32{"0dp": 1, "158dp": 158<<8 | 1, "2048dp": 2048<<8 | 1} {
		got, err := parseDP(value)
		if err != nil || got != want {
			t.Errorf("parseDP(%s)=%#x,%v, want %#x", value, got, err, want)
		}
	}
	for _, value := range []string{"158px", "-1dp", "1.5dp", "16777216dp"} {
		if _, err := parseDP(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
	for _, n := range []int{-1, -2, 0, 120} {
		got, err := parseLayoutSize(fmt.Sprint(n))
		if err != nil || int32(got) != int32(n) {
			t.Errorf("layout %d=%d,%v", n, got, err)
		}
	}
}
