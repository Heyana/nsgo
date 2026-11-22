//go:build linux && !android

package linux

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

var gVariantGetBoolean func(v uintptr) int32

func init() {
	if openLibs() == nil {
		purego.RegisterLibFunc(&gVariantGetBoolean, glibLib, "g_variant_get_boolean")
	}
}

// TestTrayMenuNumbersEveryEntry gives separators, submenus and plain labels IDs of their own, which a later menu never reuses.
func TestTrayMenuNumbersEveryEntry(t *testing.T) {
	var m dbusMenu
	m.set([]driver.MenuEntry{
		{Label: "Open", ID: 7},
		{Separator: true},
		{Label: "More", Items: []driver.MenuEntry{{Label: "Inner", ID: 8}, {Separator: true}}},
		{Separator: true},
		{Label: "About"},
	})
	if len(m.root) != 5 || len(m.items) != 7 {
		t.Fatalf("root %v, %d items", m.root, len(m.items))
	}
	seen := map[int32]bool{0: true}
	for id := range m.items {
		if seen[id] {
			t.Fatalf("ID %d twice", id)
		}
		seen[id] = true
	}
	entry, children, ok := m.node(m.root[2])
	if !ok || entry.Label != "More" || len(children) != 2 || entry.Items != nil {
		t.Fatalf("submenu %+v %v %v", entry, children, ok)
	}
	if inner, _, _ := m.node(children[0]); inner.ID != 8 {
		t.Errorf("inner entry %+v", inner)
	}
	if entry, children, ok := m.node(0); !ok || entry != nil || len(children) != 5 {
		t.Errorf("root %+v %v %v", entry, children, ok)
	}
	old, revision := m.root[0], m.revision
	m.set([]driver.MenuEntry{{Label: "Open", ID: 9}})
	if _, _, ok := m.node(old); ok || m.root[0] <= old || m.revision != revision+1 {
		t.Errorf("a new menu kept ID %d (root %v, revision %d)", old, m.root, m.revision)
	}
}

func TestMenuLabelEscapesUnderscores(t *testing.T) {
	if got := menuLabel("Save_as_new"); got != "Save__as__new" {
		t.Errorf("label %q", got)
	}
}

// trayPNG is a w by h PNG of half-transparent red.
func trayPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 128})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// TestTrayIconIsStraightARGB keeps the icon's colors under partial alpha and its aspect as it scales down.
func TestTrayIconIsStraightARGB(t *testing.T) {
	pix, w, h := trayIcon(trayPNG(t, 128, 64))
	if w != 64 || h != 32 || len(pix) != w*h*4 {
		t.Fatalf("%dx%d, %d bytes", w, h, len(pix))
	}
	if got := pix[:4]; !bytes.Equal(got, []byte{128, 255, 0, 0}) {
		t.Errorf("first pixel ARGB %v, want [128 255 0 0]", got)
	}
	if pix, _, _ := trayIcon([]byte("not a png")); pix != nil {
		t.Error("an invalid PNG gave pixels")
	}
}

// TestTrayServesItsItemAndMenuOverDBus registers two trays and reads them as a StatusNotifierItem host does.
func TestTrayServesItsItemAndMenuOverDBus(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Skip("no session bus")
	}
	var inner, quits atomic.Int32
	icon := trayPNG(t, 128, 64)
	app := core.AppRoot(func(cx *core.Scope) {
		cx.SetShutdownMode(core.ShutdownOnExplicit)
		cx.OnQuitRequest(func(q *core.QuitRequest) {
			quits.Add(1)
			q.Cancel()
		})
		cx.NewTray(core.TrayOptions{Icon: icon, Tooltip: "first", Menu: func() []core.MenuItem {
			return []core.MenuItem{
				{Label: "Count_1", OnSelect: func() {}},
				{Separator: true},
				{Label: "More", Items: []core.MenuItem{{Label: "Inner", OnSelect: func() { inner.Add(1) }}}},
				{Separator: true},
				{Label: "Quit", Role: core.MenuQuit},
			}
		}})
		cx.NewTray(core.TrayOptions{Tooltip: "second"})
	})
	w := startTestWindow(t, app, Options{Title: "nsgo-tray", ID: "dev.nsgo.traytest", Width: 120, Height: 80, Hidden: true})
	defer w.onLoop(t, w.close)
	var names []string
	for deadline := time.Now().Add(10 * time.Second); len(names) < 2; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("trays %v", names)
		}
		w.onLoop(t, func() {
			names = names[:0]
			for id := uint32(1); id <= 2; id++ {
				if tr := w.trays[id]; tr != nil {
					names = append(names, tr.name)
				}
			}
		})
	}
	trayBind()
	bus := sessionBus()
	str := func(s string) uintptr { return gVariantNewString(cstr(s)) }
	get := func(name, path, iface, property string) uintptr {
		t.Helper()
		start := time.Now()
		reply := dbusCall(bus, name, path, "org.freedesktop.DBus.Properties", "Get", str(iface), str(property))
		if time.Since(start) > 500*time.Millisecond {
			t.Errorf("Get %s took %v", property, time.Since(start))
		}
		if reply == 0 {
			return 0
		}
		defer gVariantUnref(reply)
		child := gVariantGetChildValue(reply, 0)
		defer gVariantUnref(child)
		return gVariantGetVariant(child)
	}
	stringOf := func(v uintptr) string {
		defer gVariantUnrefNonZero(v)
		if v == 0 {
			return "<none>"
		}
		return goString(gVariantGetString(v, 0))
	}
	if got := stringOf(get(names[0], trayItemPath, trayItemInterface, "IconName")); got != "" {
		t.Errorf("the first tray's IconName %q beside its pixmap", got)
	}
	if got := stringOf(get(names[1], trayItemPath, trayItemInterface, "IconName")); got != "dev.nsgo.traytest" {
		t.Errorf("the second tray's IconName %q", got)
	}
	if pixmap := get(names[0], trayItemPath, trayItemInterface, "IconPixmap"); pixmap == 0 || gVariantNChildren(pixmap) != 1 {
		t.Errorf("IconPixmap %v", pixmap)
	} else {
		item := gVariantGetChildValue(pixmap, 0)
		if w, h := variantInt32(item, 0), variantInt32(item, 1); w != 64 || h != 32 {
			t.Errorf("pixmap %dx%d", w, h)
		}
		gVariantUnref(item)
		gVariantUnref(pixmap)
	}
	tooltip := get(names[1], trayItemPath, trayItemInterface, "ToolTip")
	if got := variantString(tooltip, 2); got != "second" {
		t.Errorf("the second tray's tooltip %q", got)
	}
	gVariantUnrefNonZero(tooltip)
	// A property the interface lacks fails at once instead of going unanswered.
	if v := get(names[0], trayItemPath, trayItemInterface, "AttentionIconName"); v != 0 {
		gVariantUnref(v)
		t.Error("an undeclared property had a value")
	}

	call := func(method string, args ...uintptr) uintptr {
		t.Helper()
		reply := dbusCall(bus, names[0], trayMenuPath, trayMenuInterface, method, args...)
		if reply == 0 {
			t.Fatalf("%s failed", method)
		}
		return reply
	}
	reply := call("AboutToShow", gVariantNewInt32(0))
	if child := gVariantGetChildValue(reply, 0); gVariantGetBoolean(child) == 0 {
		t.Error("AboutToShow asked for no update")
	} else {
		gVariantUnref(child)
	}
	gVariantUnref(reply)
	type node struct {
		id       int32
		props    map[string]string
		children []node
	}
	var read func(v uintptr) node
	read = func(v uintptr) node {
		n := node{id: variantInt32(v, 0), props: map[string]string{}}
		props := gVariantGetChildValue(v, 1)
		for i := range gVariantNChildren(props) {
			entry := gVariantGetChildValue(props, i)
			value := gVariantGetChildValue(entry, 1)
			inner := gVariantGetVariant(value)
			if goString(gVariantGetTypeString(inner)) == "s" {
				n.props[variantString(entry, 0)] = goString(gVariantGetString(inner, 0))
			} else {
				n.props[variantString(entry, 0)] = goString(gVariantGetTypeString(inner))
			}
			gVariantUnref(inner)
			gVariantUnref(value)
			gVariantUnref(entry)
		}
		gVariantUnref(props)
		children := gVariantGetChildValue(v, 2)
		for i := range gVariantNChildren(children) {
			boxed := gVariantGetChildValue(children, i)
			child := gVariantGetVariant(boxed)
			n.children = append(n.children, read(child))
			gVariantUnref(child)
			gVariantUnref(boxed)
		}
		gVariantUnref(children)
		return n
	}
	layout := func(parent, depth int32) node {
		reply := call("GetLayout", gVariantNewInt32(parent), gVariantNewInt32(depth), menuArray("s", nil))
		defer gVariantUnref(reply)
		v := gVariantGetChildValue(reply, 1)
		defer gVariantUnref(v)
		return read(v)
	}
	root := layout(0, -1)
	if len(root.children) != 5 {
		t.Fatalf("layout %+v", root)
	}
	ids := map[int32]bool{0: true}
	for _, c := range root.children {
		if ids[c.id] {
			t.Errorf("ID %d twice in %+v", c.id, root)
		}
		ids[c.id] = true
	}
	if got := root.children[0].props["label"]; got != "Count__1" {
		t.Errorf("label %q", got)
	}
	if root.children[1].props["type"] != "separator" || root.children[3].props["type"] != "separator" {
		t.Errorf("separators %+v", root.children)
	}
	more := root.children[2]
	if more.props["children-display"] != "submenu" || len(more.children) != 1 {
		t.Fatalf("submenu %+v", more)
	}
	sub := layout(more.id, 1)
	if sub.id != more.id || len(sub.children) != 1 || sub.children[0].props["label"] != "Inner" {
		t.Fatalf("the submenu's layout %+v", sub)
	}
	if shallow := layout(0, 1); len(shallow.children) != 5 || len(shallow.children[2].children) != 0 {
		t.Errorf("a depth of 1 gave %+v", shallow)
	}
	gVariantUnref(call("Event", gVariantNewInt32(sub.children[0].id), str("clicked"), gVariantNewVariant(gVariantNewInt32(0)), gVariantNewUint32(0)))
	for deadline := time.Now().Add(5 * time.Second); inner.Load() == 0; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the submenu's item was not chosen")
		}
	}
	gVariantUnref(call("AboutToShow", gVariantNewInt32(0)))
	root = layout(0, -1)
	gVariantUnref(call("Event", gVariantNewInt32(root.children[4].id), str("clicked"), gVariantNewVariant(gVariantNewInt32(0)), gVariantNewUint32(0)))
	w.engine.Sync()
	if quits.Load() != 1 {
		t.Fatalf("the tray Quit ran %d quit handlers", quits.Load())
	}
	w.onLoop(t, func() {
		if w.closing {
			t.Error("a canceled tray Quit ended the app")
		}
	})
}
