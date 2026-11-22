package apk

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Widget declares one kind of Android App Widget. Localized names and
// descriptions share the app's string table; "" is the default language.
type Widget struct {
	Kind                string
	Names, Descriptions map[string]string
	Families            []string
}

const widgetComponentPrefix = "dev.oneo.nsgo.widgets.K"

func widgetComponent(kind string) string {
	sum := sha256.Sum256([]byte(kind))
	return widgetComponentPrefix + hex.EncodeToString(sum[:16])
}

func widgetResourceName(index int) string { return fmt.Sprintf("nsgo_widget_%d", index) }

func (m Manifest) widgetXMLResources() []xmlResource {
	var out []xmlResource
	for i, w := range m.Widgets {
		name := widgetResourceName(i)
		out = append(out, xmlResource{name, "res/xml/" + name + ".xml", func() *node { return widgetProviderInfo(m, w, i) }})
	}
	return out
}

func layoutResourceID(m Manifest) uint32 {
	id := xmlResourceID(m)
	if m.networkPolicy() || m.Game || len(m.Widgets) > 0 {
		id += 1 << 16
	}
	return id
}

func widgetProviderInfo(m Manifest, w Widget, index int) *node {
	n := &node{name: xml.Name{Local: "appwidget-provider"}}
	for _, attr := range []struct{ key, value string }{
		{"minWidth", "158dp"}, {"minHeight", "158dp"}, {"updatePeriodMillis", "1800000"},
		{"initialLayout", fmt.Sprintf("@0x%08x", layoutResourceID(m))}, {"resizeMode", "3"},
		{"widgetCategory", "1"}, {"targetCellWidth", "2"}, {"targetCellHeight", "2"},
		{"description", fmt.Sprintf("@0x%08x", nameResourceID(m.Icon != nil)+uint32(index*2+2))},
	} {
		setAttr(n, androidNS, attr.key, attr.value)
	}
	return n
}

func (m Manifest) layoutResources() []xmlResource {
	if len(m.Widgets) == 0 {
		return nil
	}
	return []xmlResource{{"nsgo_widget", "res/layout/nsgo_widget.xml", widgetLayout}, {"nsgo_widget_timer", "res/layout/nsgo_widget_timer.xml", widgetTimerLayout}}
}

// The same layout draws the complete bitmap and transparent link overlays.
// Framework IDs avoid a separate app ID resource type.
func widgetLayout() *node {
	root := &node{name: xml.Name{Local: "FrameLayout"}}
	image := &node{name: xml.Name{Local: "ImageView"}}
	for _, n := range []*node{root, image} {
		setAttr(n, androidNS, "layout_width", "-1")
		setAttr(n, androidNS, "layout_height", "-1")
	}
	setAttr(root, androidNS, "id", "@0x01020002")  // android.R.id.content
	setAttr(image, androidNS, "id", "@0x01020006") // android.R.id.icon
	setAttr(image, androidNS, "scaleType", "1")    // FIT_XY
	overlay := &node{name: xml.Name{Local: "FrameLayout"}}
	setAttr(overlay, androidNS, "id", "@0x01020004") // android.R.id.empty
	setAttr(overlay, androidNS, "layout_width", "-1")
	setAttr(overlay, androidNS, "layout_height", "-1")
	root.kids = []*node{image, overlay}
	return root
}

func widgetTimerLayout() *node {
	n := widgetLayout()
	n.kids[0].name.Local = "Chronometer"
	setAttr(n.kids[0], androidNS, "layout_height", "-2")
	setAttr(n.kids[0], androidNS, "includeFontPadding", "false")
	setAttr(n.kids[0], androidNS, "id", "@0x01020014") // android.R.id.text1
	n.kids[0].attrs = slices.DeleteFunc(n.kids[0].attrs, func(a xml.Attr) bool { return a.Name.Local == "scaleType" })
	return n
}

func addWidgets(app *node, m Manifest) error {
	if len(m.Widgets) == 0 {
		return nil
	}
	setAttr(app, androidNS, "appComponentFactory", "dev.oneo.nsgo.NsgoComponents")
	first := xmlResourceID(m)
	if m.networkPolicy() {
		first++
	}
	if m.Game {
		first++
	}
	for i, w := range m.Widgets {
		receiver := &node{name: xml.Name{Local: "receiver"}}
		setAttr(receiver, androidNS, "name", widgetComponent(w.Kind))
		setAttr(receiver, androidNS, "exported", "false")
		setAttr(receiver, androidNS, "label", fmt.Sprintf("@0x%08x", nameResourceID(m.Icon != nil)+uint32(i*2+1)))
		filter := &node{name: xml.Name{Local: "intent-filter"}}
		for _, action := range []string{"android.appwidget.action.APPWIDGET_UPDATE", "android.intent.action.BOOT_COMPLETED", "android.intent.action.MY_PACKAGE_REPLACED", "android.intent.action.TIME_SET", "android.intent.action.TIMEZONE_CHANGED", "android.intent.action.LOCALE_CHANGED"} {
			a := &node{name: xml.Name{Local: "action"}}
			setAttr(a, androidNS, "name", action)
			filter.kids = append(filter.kids, a)
		}
		receiver.kids = append(receiver.kids, filter)
		for _, meta := range []struct {
			name, value string
			resource    bool
		}{
			{"android.appwidget.provider", fmt.Sprintf("@0x%08x", first+uint32(i)), true},
			{"dev.oneo.nsgo.widget.kind", w.Kind, false},
			{"dev.oneo.nsgo.widget.description", fmt.Sprintf("@0x%08x", nameResourceID(m.Icon != nil)+uint32(i*2+2)), true},
			{"dev.oneo.nsgo.widget.families", strings.Join(w.Families, ","), false},
		} {
			n := &node{name: xml.Name{Local: "meta-data"}}
			setAttr(n, androidNS, "name", meta.name)
			key := "value"
			if meta.resource {
				key = "resource"
			}
			setAttr(n, androidNS, key, meta.value)
			receiver.kids = append(receiver.kids, n)
		}
		app.kids = append(app.kids, receiver)
	}
	return nil
}

type stringResource struct {
	name   string
	values map[string]string
}

func (m Manifest) stringResources() []stringResource {
	names := m.Names
	if len(names) == 0 && len(m.Widgets) > 0 {
		names = map[string]string{"": m.Label}
	}
	if len(names) == 0 {
		return nil
	}
	out := []stringResource{{"app_name", names}}
	for i, w := range m.Widgets {
		names, descriptions := w.Names, w.Descriptions
		if len(names) == 0 {
			names = map[string]string{"": w.Kind}
		}
		if len(descriptions) == 0 {
			descriptions = map[string]string{"": ""}
		}
		prefix := widgetResourceName(i)
		out = append(out, stringResource{prefix + "_name", names}, stringResource{prefix + "_description", descriptions})
	}
	return out
}

func parseLayoutSize(s string) (uint32, error) {
	n, err := strconv.ParseInt(s, 10, 32)
	return uint32(n), err
}

// parseDP encodes the integral dp dimensions used by provider metadata as an
// Android complex value, with radix 23.0 and unit DIP.
func parseDP(s string) (uint32, error) {
	if !strings.HasSuffix(s, "dp") {
		return 0, fmt.Errorf("%q is not an integral dp dimension", s)
	}
	n, err := strconv.ParseUint(strings.TrimSuffix(s, "dp"), 10, 23)
	if err != nil {
		return 0, err
	}
	return uint32(n)<<8 | 1, nil
}
