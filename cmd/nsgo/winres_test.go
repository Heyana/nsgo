package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"fmt"
	projectcfg "github.com/nsgo-dev/nsgo/project"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"unicode/utf16"
)

// resourceKey is a resource's type, ID and language.
type resourceKey struct{ typ, id, lang uint32 }

// decodeResources decodes the resource tree of f's .rsrc section, whose data entries hold addresses less base: 0 in an object, the section's address in an executable. It returns the keys in the tree's order and each resource's data.
func decodeResources(t *testing.T, f *pe.File, base uint32) ([]resourceKey, map[resourceKey][]byte) {
	t.Helper()
	sec := f.Section(".rsrc")
	if sec == nil {
		t.Fatal("no .rsrc section")
	}
	data, err := sec.Data()
	if err != nil {
		t.Fatal(err)
	}
	le := binary.LittleEndian
	const subdir = 0x80000000
	entries := func(at uint32) [][2]uint32 {
		n := int(le.Uint16(data[at+12:])) + int(le.Uint16(data[at+14:]))
		var out [][2]uint32
		for i := range n {
			e := at + 16 + uint32(i)*8
			out = append(out, [2]uint32{le.Uint32(data[e:]), le.Uint32(data[e+4:])})
		}
		return out
	}
	var keys []resourceKey
	res := map[resourceKey][]byte{}
	for _, typ := range entries(0) {
		if typ[1]&subdir == 0 {
			t.Fatalf("type %d points at data, not a directory", typ[0])
		}
		for _, id := range entries(typ[1] &^ subdir) {
			for _, lang := range entries(id[1] &^ subdir) {
				if lang[1]&subdir != 0 {
					t.Fatalf("resource %d/%d/%d is a directory", typ[0], id[0], lang[0])
				}
				at, size := le.Uint32(data[lang[1]:])-base, le.Uint32(data[lang[1]+4:])
				k := resourceKey{typ[0], id[0], lang[0]}
				keys = append(keys, k)
				res[k] = data[at : at+size]
			}
		}
	}
	return keys, res
}

// versionEntry is a decoded node of a version resource.
type versionEntry struct {
	key      string
	text     bool
	value    []byte
	children []versionEntry
}

// decodeVersion decodes a version resource's node from the start of b, checking its lengths.
func decodeVersion(t *testing.T, b []byte) versionEntry {
	t.Helper()
	le := binary.LittleEndian
	length, valueLength, typ := int(le.Uint16(b)), int(le.Uint16(b[2:])), le.Uint16(b[4:])
	if length > len(b) {
		t.Fatalf("a node of %d bytes in %d", length, len(b))
	}
	var key []uint16
	at := 6
	for ; le.Uint16(b[at:]) != 0; at += 2 {
		key = append(key, le.Uint16(b[at:]))
	}
	at = align4(at + 2)
	n := versionEntry{key: string(utf16.Decode(key)), text: typ == 1}
	size := valueLength
	if n.text {
		size *= 2
	}
	n.value = b[at : at+size]
	for at = align4(at + size); at < length; {
		child := decodeVersion(t, b[at:length])
		n.children = append(n.children, child)
		at = align4(at + int(le.Uint16(b[at:])))
	}
	return n
}

func align4(n int) int { return (n + 3) &^ 3 }

// child returns n's child named key.
func (n versionEntry) child(t *testing.T, key string) versionEntry {
	t.Helper()
	for _, c := range n.children {
		if c.key == key {
			return c
		}
	}
	t.Fatalf("%s has no %s", n.key, key)
	return versionEntry{}
}

// versionStrings returns a decoded VS_VERSIONINFO's string tables, by their keys.
func versionStrings(t *testing.T, root versionEntry) map[string]map[string]string {
	t.Helper()
	out := map[string]map[string]string{}
	for _, table := range root.child(t, "StringFileInfo").children {
		strs := map[string]string{}
		for _, s := range table.children {
			if !s.text {
				t.Errorf("%s is not text", s.key)
			}
			u := make([]uint16, len(s.value)/2)
			for i := range u {
				u[i] = binary.LittleEndian.Uint16(s.value[2*i:])
			}
			if len(u) == 0 || u[len(u)-1] != 0 {
				t.Errorf("%s is not terminated", s.key)
				continue
			}
			strs[s.key] = string(utf16.Decode(u[:len(u)-1]))
		}
		out[table.key] = strs
	}
	return out
}

func testVersion() *windowsVersion {
	return &windowsVersion{
		Fixed: [4]uint16{1, 2, 3, 7}, Version: "1.2.3", Exe: "example.exe", Company: "Acme",
		Tables: []windowsVersionTable{{0x0804, "示例"}, {0x0409, "Example"}, {0x0407, "Beispiel"}},
	}
}

// The object holds the icon and a version resource per language, sorted, the source's as the user's default language, which the loader falls back to.
func TestWindowsVersionResource(t *testing.T) {
	images := [][]byte{scaledPNG(testIcon(64), 16)}
	syso, err := windowsResource("amd64", images, []int{16}, testVersion())
	if err != nil {
		t.Fatal(err)
	}
	f, err := pe.NewFile(bytes.NewReader(syso))
	if err != nil {
		t.Fatal(err)
	}
	keys, res := decodeResources(t, f, 0)
	want := []resourceKey{{rtIcon, 1, langEnUS}, {rtGroupIcon, iconGroupID, langEnUS}, {rtVersion, 1, langUserDefault}, {rtVersion, 1, 0x0407}, {rtVersion, 1, 0x0409}}
	if !slices.Equal(keys, want) {
		t.Fatalf("resources %v, want %v", keys, want)
	}
	if len(f.Section(".rsrc").Relocs) != len(keys) {
		t.Errorf("%d relocations for %d resources", len(f.Section(".rsrc").Relocs), len(keys))
	}
	for lang, name := range map[uint32]string{langUserDefault: "示例", 0x0407: "Beispiel", 0x0409: "Example"} {
		data := res[resourceKey{rtVersion, 1, lang}]
		root := decodeVersion(t, data)
		if root.key != "VS_VERSION_INFO" || root.text || len(root.value) != 52 || int(binary.LittleEndian.Uint16(data)) != len(data) {
			t.Fatalf("root %q, text %v, value %d bytes, length %d of %d", root.key, root.text, len(root.value), binary.LittleEndian.Uint16(data), len(data))
		}
		var fixed [13]uint32
		binary.Read(bytes.NewReader(root.value), binary.LittleEndian, &fixed)
		if fixed[0] != 0xFEEF04BD || fixed[2] != 1<<16|2 || fixed[3] != 3<<16|7 || fixed[4] != fixed[2] || fixed[5] != fixed[3] || fixed[8] != 0x00040004 || fixed[9] != 1 {
			t.Errorf("VS_FIXEDFILEINFO % x", fixed)
		}
		table := uint16(lang)
		if lang == langUserDefault {
			table = 0x0804
		}
		key := fmt.Sprintf("%04x04b0", table)
		want := map[string]map[string]string{key: {
			"CompanyName": "Acme", "FileDescription": name, "FileVersion": "1.2.3", "InternalName": "example.exe",
			"OriginalFilename": "example.exe", "ProductName": name, "ProductVersion": "1.2.3",
		}}
		if got := versionStrings(t, root); !reflect.DeepEqual(got, want) {
			t.Errorf("%#04x: strings %v, want %v", lang, got, want)
		}
		translation := root.child(t, "VarFileInfo").child(t, "Translation")
		if want := []byte{byte(table), byte(table >> 8), 0xb0, 0x04}; translation.text || !bytes.Equal(translation.value, want) {
			t.Errorf("%#04x: Translation % x, want % x", lang, translation.value, want)
		}
	}

	// Without an icon the object holds the version alone; with neither it is refused.
	syso, err = windowsResource("arm64", nil, nil, testVersion())
	if err != nil {
		t.Fatal(err)
	}
	if f, err = pe.NewFile(bytes.NewReader(syso)); err != nil {
		t.Fatal(err)
	}
	if keys, _ := decodeResources(t, f, 0); !slices.Equal(keys, want[2:]) {
		t.Errorf("without an icon: resources %v", keys)
	}
	if _, err := windowsResource("amd64", nil, nil, nil); err == nil {
		t.Error("an object with no resources was written")
	}
}

// The version comes from nsgo.yaml and the catalogs' appName, in each language Windows has an ID for.
func TestProjectWindowsVersion(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"locales/en.yaml":      "appName: My App\nhello: Hello\n",
		"locales/zh-Hans.yaml": "appName: 应用\nhello: 你好\n",
		"locales/zh-TW.yaml":   "appName: 應用\n",
		"locales/eo.yaml":      "appName: Apo\nhello: Saluton\n",
		"locales/fr.yaml":      "hello: Bonjour\n",
	})
	p := &project{Config: projectcfg.Config{Dir: dir, Name: "MyApp", Version: "1.2.0-beta.1", BuildNumber: 7, Copyright: "© 2026 Example"}}
	p.I18n.Dir, p.I18n.Source = filepath.Join(dir, "locales"), "en"
	p.Windows.Store.PublisherName = "Example Store"
	v, err := p.windowsVersion()
	if err != nil {
		t.Fatal(err)
	}
	want := &windowsVersion{
		Fixed: [4]uint16{1, 2, 0, 7}, Version: "1.2.0-beta.1", Exe: "MyApp.exe", Company: "Example Store", Copyright: "© 2026 Example",
		Tables: []windowsVersionTable{{0x0409, "My App"}, {0x0804, "应用"}, {0x0404, "應用"}},
	}
	if !reflect.DeepEqual(v, want) {
		t.Errorf("got %+v\nwant %+v", v, want)
	}

	// windows.publisher comes before the Store's, and a build number too large for a part is left out.
	p.Windows.Publisher, p.BuildNumber = "Example Ltd", 70000
	if v, err = p.windowsVersion(); err != nil {
		t.Fatal(err)
	}
	if v.Company != "Example Ltd" || v.Fixed != [4]uint16{1, 2, 0, 0} {
		t.Errorf("company %q, fixed %v", v.Company, v.Fixed)
	}

	// A source language Windows has no ID for is neutral.
	p.I18n.Source = "eo"
	if v, err = p.windowsVersion(); err != nil {
		t.Fatal(err)
	}
	if v.Tables[0] != (windowsVersionTable{0, "Apo"}) || len(v.Tables) != 4 {
		t.Errorf("tables %v", v.Tables)
	}

	// Without catalogs, the source language names the app by name.
	p.I18n.Dir, p.I18n.Source = filepath.Join(dir, "none"), "en"
	if v, err = p.windowsVersion(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(v.Tables, []windowsVersionTable{{0x0409, "MyApp"}}) {
		t.Errorf("without catalogs: tables %v", v.Tables)
	}
}

func TestWindowsLanguageID(t *testing.T) {
	for tag, want := range map[string]uint16{
		"en": 0x0409, "en-GB": 0x0809, "en-AU": 0x0c09, "en-XA": 0x0409, "pt": 0x0416, "pt-PT": 0x0816,
		"zh": 0x0804, "zh-Hans": 0x0804, "zh-Hant": 0x0404, "zh-TW": 0x0404, "zh-HK": 0x0c04, "zh-Hant-HK": 0x0c04,
		"sr-Latn": 0x241a, "es-419": 0x580a, "ja": 0x0411,
	} {
		if got, ok := windowsLanguageID(tag); !ok || got != want {
			t.Errorf("%s: %#04x, %v; want %#04x", tag, got, ok, want)
		}
	}
	if _, ok := windowsLanguageID("eo"); ok {
		t.Error("eo has an ID")
	}
}

// An object of the app's own holding resources for the architecture takes the place of NSGo's, which the linker could not link beside it.
func TestWindowsAppResources(t *testing.T) {
	dir := t.TempDir()
	syso, err := windowsResource("amd64", nil, nil, testVersion())
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"rsrc_windows_amd64.syso":      syso,
		windowsResourceObject("arm64"): syso,
		"code_windows_arm64.syso":      []byte("not an object"),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for arch, want := range map[string]string{"amd64": "rsrc_windows_amd64.syso", "arm64": "", "386": ""} {
		if got, err := windowsAppResources(dir, arch); err != nil || got != want {
			t.Errorf("%s: %q, %v; want %q", arch, got, err, want)
		}
	}
	if err := os.Rename(filepath.Join(dir, "rsrc_windows_amd64.syso"), filepath.Join(dir, "rsrc.syso")); err != nil {
		t.Fatal(err)
	}
	if got, _ := windowsAppResources(dir, "386"); got != "rsrc.syso" {
		t.Errorf("an object for every architecture: %q", got)
	}
}

// The command companion links the app's resource object, so it shows the same version.
func TestBuildWindowsCommandResources(t *testing.T) {
	dir := t.TempDir()
	syso, err := windowsResource("amd64", nil, nil, testVersion())
	if err != nil {
		t.Fatal(err)
	}
	if err := buildWindowsCommand(t.Context(), &project{Config: projectcfg.Config{Name: "example"}}, dir, "amd64", syso); err != nil {
		t.Fatal(err)
	}
	f, err := pe.Open(filepath.Join(dir, "example.com"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	keys, res := decodeResources(t, f, f.Section(".rsrc").VirtualAddress)
	if len(keys) != 3 || keys[0] != (resourceKey{rtVersion, 1, langUserDefault}) {
		t.Fatalf("resources %v", keys)
	}
	source := versionStrings(t, decodeVersion(t, res[keys[0]]))
	english := versionStrings(t, decodeVersion(t, res[keys[2]]))
	if got := source["080404b0"]["FileDescription"] + "," + english["040904b0"]["ProductName"]; got != "示例,Example" {
		t.Errorf("names %s", got)
	}
}
