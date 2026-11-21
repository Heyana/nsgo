package apk

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unicode/utf16"

	androidruntime "github.com/nsgo-dev/nsgo/internal/platform/android/runtime"
)

// TestEncodeManifest reads back the binary XML the system parses, above
// all that each element's attributes ascend by framework ID: the system
// binary-searches them, and an out of order theme attribute is ignored.
func TestEncodeManifest(t *testing.T) {
	data, err := EncodeManifest(androidruntime.Manifest, Manifest{
		Package:     "dev.oneo.nsgo.test",
		Label:       "Test",
		VersionName: "1.2.3",
		VersionCode: 10203,
		Icon:        []byte("png"),
	})
	if err != nil {
		t.Fatal(err)
	}
	doc := decodeXML(t, data)

	root := doc.root
	if root.name != "manifest" {
		t.Fatalf("root = %q, want manifest", root.name)
	}
	if got := root.plain(t, "package"); got != "dev.oneo.nsgo.test" {
		t.Fatalf("package = %q", got)
	}
	if got := root.value(t, "versionCode"); got != 10203 {
		t.Fatalf("versionCode = %d", got)
	}
	if got := root.string(t, "versionName"); got != "1.2.3" {
		t.Fatalf("versionName = %q", got)
	}

	app := root.child(t, "application")
	if got := app.string(t, "label"); got != "Test" {
		t.Fatalf("label = %q", got)
	}
	if got, want := app.value(t, "theme"), uint32(0x01030129); got != want {
		t.Fatalf("theme = %#x, want %#x", got, want)
	}
	if got, want := app.value(t, "icon"), uint32(iconResourceID); got != want {
		t.Fatalf("icon = %#x, want %#x", got, want)
	}
	provider := app.child(t, "provider")
	if got := provider.string(t, "authorities"); got != "dev.oneo.nsgo.test.nsgo.files" {
		t.Fatalf("the file provider's authority = %q", got)
	}
	if got := provider.value(t, "grantUriPermissions"); got == 0 {
		t.Fatal("the file provider grants no URI permissions")
	}

	doc.walk(func(e *xmlNode) {
		last := uint32(0)
		for _, a := range e.attrs {
			if a.id == 0 {
				continue
			}
			if a.id < last {
				t.Fatalf("%s: %s (%#x) follows %#x", e.name, doc.strings[a.name], a.id, last)
			}
			last = a.id
		}
	})
	if len(doc.ids) > len(doc.strings) {
		t.Fatalf("resource map has %d IDs for %d strings", len(doc.ids), len(doc.strings))
	}
	for i, id := range doc.ids {
		if id == 0 {
			t.Fatalf("resource map entry %d is zero", i)
		}
		if i > 0 && id < doc.ids[i-1] {
			t.Fatalf("resource map does not ascend: %#x after %#x", id, doc.ids[i-1])
		}
	}
}

func TestEncodeManifestURLSchemes(t *testing.T) {
	manifest := Manifest{Package: "dev.oneo.nsgo.test"}
	for _, schemes := range [][]string{nil, {"myapp", "myapp-dev"}} {
		manifest.URLSchemes = schemes
		data, err := EncodeManifest(androidruntime.Manifest, manifest)
		if err != nil {
			t.Fatal(err)
		}
		activity := decodeXML(t, data).root.child(t, "application").child(t, "activity")
		if got := activity.string(t, "name"); got != "dev.oneo.nsgo.NsgoActivity" {
			t.Fatalf("activity = %q", got)
		}
		if got := len(activity.kids); got != 1+len(schemes) {
			t.Fatalf("activity has %d children for %d schemes, want %d", got, len(schemes), 1+len(schemes))
		}
		launcher := activity.kids[0]
		if launcher.name != "intent-filter" || launcher.child(t, "action").string(t, "name") != "android.intent.action.MAIN" ||
			launcher.child(t, "category").string(t, "name") != "android.intent.category.LAUNCHER" {
			t.Fatal("the launcher filter changed")
		}
		if len(schemes) == 0 {
			for _, attr := range activity.attrs {
				if attr.id == androidAttrs["launchMode"].id {
					t.Fatal("an app without URL schemes changes the default launch mode")
				}
			}
			continue
		}
		if attr := activity.attr(t, "launchMode"); attr.kind != typeInt || attr.data != 2 {
			t.Fatalf("launchMode = %+v, want singleTask as an integer", attr)
		}
		for i, scheme := range schemes {
			filter := activity.kids[i+1]
			if filter.name != "intent-filter" || len(filter.kids) != 4 ||
				filter.child(t, "action").string(t, "name") != "android.intent.action.VIEW" ||
				filter.kids[1].name != "category" || filter.kids[1].string(t, "name") != "android.intent.category.DEFAULT" ||
				filter.kids[2].name != "category" || filter.kids[2].string(t, "name") != "android.intent.category.BROWSABLE" ||
				filter.child(t, "data").string(t, "scheme") != scheme {
				t.Fatalf("bad intent filter for %q: %+v", scheme, filter)
			}
		}
	}
}

func TestEncodeManifestShareTypes(t *testing.T) {
	manifest := Manifest{
		Package:    "dev.oneo.nsgo.test",
		URLSchemes: []string{"myapp"},
		ShareTypes: []string{"text/plain", "image/*"},
	}
	data, err := EncodeManifest(androidruntime.Manifest, manifest)
	if err != nil {
		t.Fatal(err)
	}
	activity := decodeXML(t, data).root.child(t, "application").child(t, "activity")
	if attr := activity.attr(t, "launchMode"); attr.kind != typeInt || attr.data != 2 {
		t.Fatalf("launchMode = %+v, want singleTask as an integer", attr)
	}
	if len(activity.kids) != 3 {
		t.Fatalf("activity has %d filters, want launcher, link and share", len(activity.kids))
	}
	filter := activity.kids[2]
	if filter.name != "intent-filter" || len(filter.kids) != 5 {
		t.Fatalf("bad share intent filter: %+v", filter)
	}
	for i, want := range []struct{ tag, attr, value string }{
		{"action", "name", "android.intent.action.SEND"},
		{"action", "name", "android.intent.action.SEND_MULTIPLE"},
		{"category", "name", "android.intent.category.DEFAULT"},
		{"data", "mimeType", "text/plain"},
		{"data", "mimeType", "image/*"},
	} {
		kid := filter.kids[i]
		if kid.name != want.tag || kid.string(t, want.attr) != want.value {
			t.Errorf("share filter child %d = %+v, want %+v", i, kid, want)
		}
		if want.attr == "mimeType" && kid.attr(t, want.attr).id != 0x01010026 {
			t.Error("mimeType has the wrong framework resource ID")
		}
	}

	root, err := appManifest(androidruntime.Manifest, manifest)
	if err != nil {
		t.Fatal(err)
	}
	proto, err := protoElement(root, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(proto, pbVarint(nil, 5, 0x01010026)) {
		t.Error("the App Bundle manifest lacks the mimeType resource ID")
	}
	for _, value := range []string{"android.intent.action.SEND", "android.intent.action.SEND_MULTIPLE", "text/plain", "image/*"} {
		if !bytes.Contains(proto, []byte(value)) {
			t.Errorf("the App Bundle manifest lacks %q", value)
		}
	}
}

// TestEncodeManifestPermissions checks that the app's permissions join
// the runtime's once each, and that a name that is none fails.
func TestEncodeManifestPermissions(t *testing.T) {
	perms := []string{"android.permission.REQUEST_INSTALL_PACKAGES", "android.permission.INTERNET"}
	data, err := EncodeManifest(androidruntime.Manifest, Manifest{Package: "dev.oneo.nsgo.test", Permissions: perms})
	if err != nil {
		t.Fatal(err)
	}
	asked := map[string]int{}
	for _, kid := range decodeXML(t, data).root.kids {
		if kid.name == "uses-permission" {
			asked[kid.string(t, "name")]++
		}
	}
	for _, p := range perms {
		if asked[p] != 1 {
			t.Errorf("the manifest asks for %s %d times: %v", p, asked[p], asked)
		}
	}
	if _, err := EncodeManifest(androidruntime.Manifest, Manifest{Package: "dev.oneo.nsgo.test", Permissions: []string{"not a permission"}}); err == nil {
		t.Error("a bad permission was accepted")
	}
}

// TestEncodeManifestLibraries checks that the device's libraries the app uses go into its application as uses-library elements that do not require them, and none without.
func TestEncodeManifestLibraries(t *testing.T) {
	data, err := EncodeManifest(androidruntime.Manifest, Manifest{Package: "dev.oneo.nsgo.test", Libraries: []string{"androidx.window.extensions"}})
	if err != nil {
		t.Fatal(err)
	}
	var libs []*xmlNode
	for _, kid := range decodeXML(t, data).root.child(t, "application").kids {
		if kid.name == "uses-library" {
			libs = append(libs, kid)
		}
	}
	if len(libs) != 1 || libs[0].string(t, "name") != "androidx.window.extensions" || libs[0].value(t, "required") != 0 {
		t.Fatalf("uses-library elements %+v, want one that does not require the window extensions", libs)
	}
	data, err = EncodeManifest(androidruntime.Manifest, Manifest{Package: "dev.oneo.nsgo.test"})
	if err != nil {
		t.Fatal(err)
	}
	for _, kid := range decodeXML(t, data).root.child(t, "application").kids {
		if kid.name == "uses-library" {
			t.Fatal("an app that uses no library declares one")
		}
	}
	if _, err := EncodeManifest(androidruntime.Manifest, Manifest{Package: "dev.oneo.nsgo.test", Libraries: []string{"not a library"}}); err == nil {
		t.Error("a bad library name was accepted")
	}
}

func TestEncodeManifestRejectsBadIDs(t *testing.T) {
	for _, id := range []string{"", "hello", "dev..nsgo", "dev.nsgo.hello world", "1dev.nsgo.hello"} {
		if _, err := EncodeManifest(androidruntime.Manifest, Manifest{Package: id}); err == nil {
			t.Errorf("id %q was accepted", id)
		}
	}
}

func TestVersionCode(t *testing.T) {
	for _, test := range []struct {
		version string
		want    int
	}{
		{"1.2.3", 10203},
		{"0.1.0", 100},
		{"0.1", 100},
		{"2", 20000},
		{"1.2.3.4", 10203},
		{"dev", 1},
		{"0.0.0", 1},
		{"1.2.-3", 1},
	} {
		if got := VersionCode(test.version); got != test.want {
			t.Errorf("VersionCode(%q) = %d, want %d", test.version, got, test.want)
		}
	}
}

// decoded is what the tests read out of the binary XML.

type xmlNode struct {
	name  string
	attrs []xmlAttribute
	kids  []*xmlNode
	doc   *xmlDocument
	text  string
}

type xmlAttribute struct {
	ns   int32
	id   uint32 // the framework ID from the resource map
	name uint32
	kind uint8
	data uint32
}

type xmlDocument struct {
	strings []string
	ids     []uint32
	root    *xmlNode
}

// string returns the android attribute's string value.
func (n *xmlNode) string(t *testing.T, name string) string {
	t.Helper()
	return stringAt(t, n, n.attr(t, name).data)
}

// plain returns an attribute's string value, namespaced or not.
func (n *xmlNode) plain(t *testing.T, name string) string {
	t.Helper()
	for _, a := range n.attrs {
		if a.id == 0 && n.doc.strings[a.name] == name {
			return n.doc.strings[a.data]
		}
	}
	t.Fatalf("%s: no attribute %q", n.name, name)
	return ""
}

// value returns the android attribute's typed value.
func (n *xmlNode) value(t *testing.T, name string) uint32 {
	t.Helper()
	return n.attr(t, name).data
}

func (n *xmlNode) attr(t *testing.T, name string) xmlAttribute {
	t.Helper()
	for _, a := range n.attrs {
		if a.id == androidAttrs[name].id {
			return a
		}
	}
	t.Fatalf("%s: no android:%s", n.name, name)
	return xmlAttribute{}
}

func (n *xmlNode) child(t *testing.T, name string) *xmlNode {
	t.Helper()
	for _, kid := range n.kids {
		if kid.name == name {
			return kid
		}
	}
	t.Fatalf("%s: no %s child", n.name, name)
	return nil
}

func stringAt(t *testing.T, n *xmlNode, i uint32) string {
	t.Helper()
	if int(i) >= len(n.doc.strings) {
		t.Fatalf("%s: string %d of %d", n.name, i, len(n.doc.strings))
	}
	return n.doc.strings[i]
}

// decodeXML parses what EncodeManifest writes, with a reader of its own so
// the test does not trust the writer.
func decodeXML(t *testing.T, data []byte) xmlDocument {
	t.Helper()
	u16 := func(off int) uint16 { return binary.LittleEndian.Uint16(data[off:]) }
	u32 := func(off int) uint32 { return binary.LittleEndian.Uint32(data[off:]) }
	if got := u16(0); got != xmlHeader {
		t.Fatalf("header type = %#x", got)
	}
	if got := u32(4); got != uint32(len(data)) {
		t.Fatalf("file size = %d, want %d", got, len(data))
	}
	doc := xmlDocument{}
	var stack []*xmlNode
	off := 0
	// The header is a chunk like any other; skip its own fields.
	off = 8
	for off < len(data) {
		kind, size := u16(off), int(u32(off+4))
		if size < 8 || off+size > len(data) {
			t.Fatalf("chunk %#x at %d has size %d", kind, off, size)
		}
		switch kind {
		case stringPool:
			if u32(off+16)&0x100 != 0 {
				t.Fatal("string pool is not the UTF-16 the writer makes")
			}
			count := int(u32(off + 8))
			start := off + int(u32(off+20))
			for i := 0; i < count; i++ {
				p := start + int(u32(off+28+4*i))
				n := int(u16(p))
				p += 2
				if n&0x8000 != 0 {
					n = n&0x7fff<<16 | int(u16(p))
					p += 2
				}
				units := make([]uint16, n)
				for j := range units {
					units[j] = u16(p + 2*j)
				}
				doc.strings = append(doc.strings, string(utf16.Decode(units)))
			}
		case resourceMap:
			for p := 8; p+4 <= size; p += 4 {
				doc.ids = append(doc.ids, u32(off+p))
			}
		case xmlStartElement:
			n := &xmlNode{name: doc.strings[u32(off+20)], doc: &doc}
			count := int(u16(off + 28))
			for i := 0; i < count; i++ {
				a := off + 36 + 20*i
				attr := xmlAttribute{
					ns:   int32(u32(a)),
					name: u32(a + 4),
					kind: data[a+15],
					data: u32(a + 16),
				}
				if attr.ns >= 0 && int(attr.name) < len(doc.ids) {
					attr.id = doc.ids[attr.name]
				}
				n.attrs = append(n.attrs, attr)
			}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.kids = append(parent.kids, n)
			} else if doc.root == nil {
				doc.root = n
			}
			stack = append(stack, n)
		case xmlText:
			stack[len(stack)-1].text = doc.strings[u32(off+16)]
		case xmlEndElement:
			stack = stack[:len(stack)-1]
		}
		off += size
	}
	if doc.root == nil {
		t.Fatal("no root element")
	}
	return doc
}

// walk visits the nodes of the tree in order.
func (d xmlDocument) walk(f func(*xmlNode)) {
	var visit func(*xmlNode)
	visit = func(n *xmlNode) {
		f(n)
		for _, kid := range n.kids {
			visit(kid)
		}
	}
	visit(d.root)
}

// TestEncodeManifestOverlay checks that only an app showing overlays declares the overlay's foreground service, of the specialUse type with its subtype.
func TestEncodeManifestOverlay(t *testing.T) {
	services := func(m Manifest) []*xmlNode {
		data, err := EncodeManifest(androidruntime.Manifest, m)
		if err != nil {
			t.Fatal(err)
		}
		var out []*xmlNode
		for _, kid := range decodeXML(t, data).root.child(t, "application").kids {
			if kid.name == "service" {
				out = append(out, kid)
			}
		}
		return out
	}
	if got := services(Manifest{Package: "dev.oneo.nsgo.test"}); len(got) != 0 {
		t.Fatalf("an app without overlays declares %d services", len(got))
	}
	got := services(Manifest{Package: "dev.oneo.nsgo.test", Overlay: true})
	if len(got) != 1 {
		t.Fatalf("an app with overlays declares %d services", len(got))
	}
	s := got[0]
	if s.string(t, "name") != OverlayService || s.value(t, "exported") != 0 || s.value(t, "foregroundServiceType") != 0x40000000 {
		t.Errorf("service %s, exported %d, type %#x", s.string(t, "name"), s.value(t, "exported"), s.value(t, "foregroundServiceType"))
	}
	p := s.child(t, "property")
	if p.string(t, "name") != "android.app.PROPERTY_SPECIAL_USE_FGS_SUBTYPE" || p.string(t, "value") != OverlaySubtype {
		t.Errorf("property %s = %s", p.string(t, "name"), p.string(t, "value"))
	}
}

// TestFrameworkAttributeIDs holds androidAttrs to the IDs of the SDK's public-final.xml, as the system reads a binary manifest's attributes by ID and ignores one under another's.
func TestFrameworkAttributeIDs(t *testing.T) {
	want := map[string]uint32{
		"versionCode":                 0x0101021b,
		"versionName":                 0x0101021c,
		"minSdkVersion":               0x0101020c,
		"targetSdkVersion":            0x01010270,
		"name":                        0x01010003,
		"label":                       0x01010001,
		"launchMode":                  0x0101001d,
		"mimeType":                    0x01010026,
		"scheme":                      0x01010027,
		"exported":                    0x01010010,
		"authorities":                 0x01010018,
		"grantUriPermissions":         0x0101001b,
		"theme":                       0x01010000,
		"icon":                        0x01010002,
		"debuggable":                  0x0101000f,
		"required":                    0x0101028e,
		"extractNativeLibs":           0x010104ea,
		"enableOnBackInvokedCallback": 0x0101066c,
		"foregroundServiceType":       0x01010599,
		"configChanges":               0x0101001f,
		"windowSoftInputMode":         0x0101022b,
		"value":                       0x01010024,
		"drawable":                    0x01010199,
		"networkSecurityConfig":       0x01010527,
	}
	for name, spec := range androidAttrs {
		id, ok := want[name]
		if !ok {
			t.Errorf("android:%s has no ID to check it against", name)
			continue
		}
		if spec.id != id {
			t.Errorf("android:%s = %#x, want %#x", name, spec.id, id)
		}
	}
}

// TestForegroundServiceTypes holds the flags to attrs_manifest.xml's.
func TestForegroundServiceTypes(t *testing.T) {
	for s, want := range map[string]uint32{
		"microphone":              0x80,
		"camera|microphone":       0xc0,
		"fileManagement":          0x1000,
		"mediaProcessing":         0x2000,
		"specialUse":              0x40000000,
		"dataSync | shortService": 0x801,
	} {
		got, err := parseForegroundServiceType(s)
		if err != nil || got != want {
			t.Errorf("parseForegroundServiceType(%q) = %#x, %v, want %#x", s, got, err, want)
		}
	}
	if _, err := parseForegroundServiceType("speaker"); err == nil {
		t.Error("an unknown type was accepted")
	}
}

// TestEncodeManifestRecording checks that only an app that records declares the recording service, with the microphone type and its permissions, in the APK's manifest and the bundle's.
func TestEncodeManifestRecording(t *testing.T) {
	declared := func(recording bool) (map[string]bool, *xmlNode) {
		data, err := EncodeManifest(androidruntime.Manifest, Manifest{Package: "dev.oneo.nsgo.test", Recording: recording})
		if err != nil {
			t.Fatal(err)
		}
		root := decodeXML(t, data).root
		asked := map[string]bool{}
		for _, kid := range root.kids {
			if kid.name == "uses-permission" {
				asked[kid.string(t, "name")] = true
			}
		}
		for _, kid := range root.child(t, "application").kids {
			if kid.name == "service" && kid.string(t, "name") == RecordingService {
				return asked, kid
			}
		}
		return asked, nil
	}
	asked, service := declared(false)
	if service != nil {
		t.Error("an app that does not record declares the recording service")
	}
	for _, p := range recordingPermissions {
		if asked[p] {
			t.Errorf("an app that does not record asks for %s", p)
		}
	}
	asked, service = declared(true)
	if service == nil {
		t.Fatal("an app that records declares no recording service")
	}
	if got := service.value(t, "foregroundServiceType"); got != 0x80 {
		t.Errorf("the recording service's type = %#x, want microphone (0x80)", got)
	}
	for _, p := range recordingPermissions {
		if !asked[p] {
			t.Errorf("an app that records does not ask for %s", p)
		}
	}

	root, err := appManifest(androidruntime.Manifest, Manifest{Package: "dev.oneo.nsgo.test", Recording: true})
	if err != nil {
		t.Fatal(err)
	}
	proto, err := protoElement(root, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(proto, pbVarint(nil, 5, 0x01010599)) || !bytes.Contains(proto, []byte(RecordingService)) {
		t.Error("the bundle's manifest lacks the recording service's type")
	}
}

// TestEncodeManifestServices checks that an app that records and shows overlays declares both services, each of its own type, and asks once for the foreground-service permission they share.
func TestEncodeManifestServices(t *testing.T) {
	data, err := EncodeManifest(androidruntime.Manifest, Manifest{
		Package:     "dev.oneo.nsgo.test",
		Permissions: []string{"android.permission.SYSTEM_ALERT_WINDOW", "android.permission.FOREGROUND_SERVICE", "android.permission.FOREGROUND_SERVICE_SPECIAL_USE", "android.permission.FOREGROUND_SERVICE_LOCATION"},
		Overlay:     true,
		Recording:   true,
		Location:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	root := decodeXML(t, data).root
	asked := map[string]int{}
	for _, kid := range root.kids {
		if kid.name == "uses-permission" {
			asked[kid.string(t, "name")]++
		}
	}
	for _, p := range []string{"android.permission.FOREGROUND_SERVICE", "android.permission.FOREGROUND_SERVICE_MICROPHONE", "android.permission.FOREGROUND_SERVICE_SPECIAL_USE", "android.permission.RECORD_AUDIO"} {
		if asked[p] != 1 {
			t.Errorf("the app asks for %s %d times", p, asked[p])
		}
	}
	types := map[string]uint32{}
	for _, kid := range root.child(t, "application").kids {
		if kid.name == "service" {
			types[kid.string(t, "name")] = kid.value(t, "foregroundServiceType")
		}
	}
	if len(types) != 3 || types[RecordingService] != 0x80 || types[OverlayService] != 0x40000000 || types[LocationService] != 0x8 {
		t.Errorf("services %#x, want %s of microphone (0x80), %s of specialUse (0x40000000) and %s of location (0x8)", types, RecordingService, OverlayService, LocationService)
	}
}
