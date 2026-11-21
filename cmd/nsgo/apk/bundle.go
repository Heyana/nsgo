package apk

import (
	"encoding/binary"
	"fmt"
	"io"
	"slices"
	"strings"
)

// An Android App Bundle is what Google Play takes: a ZIP of the base
// module, whose manifest and resource table are protocol buffers, as
// aapt2's Resources.proto and Configuration.proto define them, rather
// than binary XML, and BundleConfig.pb, bundletool's config.proto, from
// which Play makes each device's APKs. The bundle is signed as a JAR, with
// the upload key.

// BundletoolVersion is the bundletool the bundle says it is made for.
const BundletoolVersion = "1.18.3"

// WriteBundle writes a signed Android App Bundle to w: template's manifest
// with m filled in, files as the base module's, by their paths in an APK,
// and the resource table, which holds the launcher icon, app names and
// network policy when m has them.
func WriteBundle(w io.Writer, template []byte, m Manifest, files []File, key *Key) error {
	root, err := appManifest(template, m)
	if err != nil {
		return err
	}
	manifest, err := protoElement(root, true, m.Icon != nil)
	if err != nil {
		return err
	}
	all := []File{{Name: "base/manifest/AndroidManifest.xml", Data: pbBytes(nil, 1, manifest)}}
	for _, f := range files {
		name, ok := bundlePath(f.Name)
		if !ok {
			return fmt.Errorf("bundle: %s has no place in a module", f.Name)
		}
		all = append(all, File{Name: name, Data: f.Data, Stored: f.Stored})
	}
	if m.Icon != nil {
		all = append(all, File{Name: "base/" + iconFile, Data: m.Icon, Stored: true})
	}
	if m.Adaptive != nil {
		// A bundle's XML resources are protobuf XmlNodes, as its manifest is; bundletool turns them into binary XML for the APKs it makes.
		icon, err := protoElement(adaptiveIcon(len(m.Adaptive.Monochrome) > 0), true, true)
		if err != nil {
			return err
		}
		all = append(all, File{Name: "base/" + adaptiveIconFile, Data: pbBytes(nil, 1, icon)})
		for _, file := range adaptiveLayers(m.Adaptive) {
			file.Name = "base/" + file.Name
			all = append(all, file)
		}
	}
	for _, resource := range append(m.xmlResources(), m.layoutResources()...) {
		root := resource.node()
		data, err := protoElement(root, len(attrNames(root)) > 0, false)
		if err != nil {
			return err
		}
		all = append(all, File{Name: "base/" + resource.file, Data: pbBytes(nil, 1, data)})
	}
	if native := bundleNativeLibraries(files); len(native) != 0 {
		all = append(all, File{Name: "base/native.pb", Data: native})
	}
	all = append(all,
		File{Name: "base/resources.pb", Data: resourcesProto(m)},
		File{Name: "BundleConfig.pb", Data: bundleConfig()})
	assets, err := bundleAssetEntries(m.Package, m.AssetPacks)
	if err != nil {
		return err
	}
	return writeBundleArchive(w, all, assets, key)
}

// bundlePath is where a file of an APK goes in the base module: the dex
// files under dex/, the rest, such as lib/ and assets/, as they are.
func bundlePath(name string) (string, bool) {
	switch {
	case strings.HasSuffix(name, ".dex") && !strings.Contains(name, "/"):
		return "base/dex/" + name, true
	case strings.HasPrefix(name, "lib/"), strings.HasPrefix(name, "assets/"), strings.HasPrefix(name, "res/"):
		return "base/" + name, true
	}
	return "", false
}

// bundleNativeLibraries encodes the base module's native.pb (bundletool's
// NativeLibraries), so each ABI's executable goes into its own split APK.
func bundleNativeLibraries(files []File) []byte {
	var abis []string
	for _, file := range files {
		rest, ok := strings.CutPrefix(file.Name, "lib/")
		if !ok {
			continue
		}
		abi, _, ok := strings.Cut(rest, "/")
		if ok && !slices.Contains(abis, abi) {
			abis = append(abis, abi)
		}
	}
	slices.Sort(abis)
	// Abi.AbiAlias values from bundletool's targeting.proto.
	aliases := map[string]uint64{
		"armeabi": 1, "armeabi-v7a": 2, "arm64-v8a": 3, "x86": 4,
		"x86_64": 5, "mips": 6, "mips64": 7, "riscv64": 8,
	}
	var out []byte
	for _, abi := range abis {
		alias, ok := aliases[abi]
		if !ok {
			continue
		}
		targeting := pbBytes(nil, 1, pbVarint(nil, 1, alias))
		dir := pbString(nil, 1, "lib/"+abi)
		out = pbBytes(out, 1, pbBytes(dir, 2, targeting))
	}
	return out
}

// bundleConfig is BundleConfig.pb: the bundletool version, and native
// libraries kept compressed, so the system extracts them, as the
// runtime starts the app's executable from the libraries' directory.
func bundleConfig() []byte {
	tool := pbString(nil, 2, BundletoolVersion)
	// Optimizations.uncompress_native_libraries, present with enabled
	// false.
	optimizations := pbBytes(nil, 2, nil)
	out := pbBytes(nil, 1, tool)
	return pbBytes(out, 2, optimizations)
}

// protoElement encodes n as an XmlNode's element, with the android
// namespace declared on the root, and the values the attributes compile
// to as the binary manifest does.
func protoElement(n *node, root, icon bool) ([]byte, error) {
	var el []byte
	if root {
		ns := pbString(nil, 1, "android")
		ns = pbString(ns, 2, androidNS)
		el = pbBytes(el, 1, ns)
		for _, a := range n.attrs {
			if a.Name.Space == "xmlns" && a.Value == distNS {
				el = pbBytes(el, 1, pbString(pbString(nil, 1, a.Name.Local), 2, distNS))
			}
		}
	}
	if n.name.Space != "" {
		el = pbString(el, 2, n.name.Space)
	}
	el = pbString(el, 3, n.name.Local)
	for _, a := range n.attrs {
		if a.Name.Local == "xmlns" || a.Name.Space == "xmlns" {
			continue
		}
		var attr []byte
		switch a.Name.Space {
		case "":
			attr = pbString(attr, 2, a.Name.Local)
			attr = pbString(attr, 3, a.Value)
			if a.Name.Local == cleartextAttribute {
				item, err := protoItem(a.Name.Local, a.Value, attrSpec{kind: typeBool, parse: parseBool}, icon)
				if err != nil {
					return nil, err
				}
				attr = pbBytes(attr, 6, item)
			}
		case androidNS:
			spec, ok := androidAttrs[a.Name.Local]
			if !ok {
				return nil, fmt.Errorf("manifest: packaging does not know android:%s", a.Name.Local)
			}
			attr = pbString(attr, 1, androidNS)
			attr = pbString(attr, 2, a.Name.Local)
			attr = pbString(attr, 3, a.Value)
			attr = pbVarint(attr, 5, uint64(spec.id))
			item, err := protoItem(a.Name.Local, a.Value, spec, icon)
			if err != nil {
				return nil, err
			}
			if item != nil {
				attr = pbBytes(attr, 6, item)
			}
		case distNS:
			attr = pbString(attr, 1, distNS)
			attr = pbString(attr, 2, a.Name.Local)
			attr = pbString(attr, 3, a.Value)
			if a.Name.Local == "include" {
				item, err := protoItem(a.Name.Local, a.Value, attrSpec{kind: typeBool, parse: parseBool}, icon)
				if err != nil {
					return nil, err
				}
				attr = pbBytes(attr, 6, item)
			}
		default:
			return nil, fmt.Errorf("manifest: unknown namespace %q", a.Name.Space)
		}
		el = pbBytes(el, 4, attr)
	}
	if n.text != "" {
		el = pbBytes(el, 5, pbString(nil, 2, n.text))
	}
	for _, kid := range n.kids {
		child, err := protoElement(kid, false, icon)
		if err != nil {
			return nil, err
		}
		el = pbBytes(el, 5, pbBytes(nil, 1, child))
	}
	return el, nil
}

// protoItem is the Item an android:* attribute compiles to; nil for a
// string, which stays its value alone.
func protoItem(name, value string, spec attrSpec, icon bool) ([]byte, error) {
	if name == "label" && strings.HasPrefix(value, "@0x") {
		id, err := parseResourceID(value)
		if err != nil {
			return nil, err
		}
		return pbBytes(nil, 1, pbVarint(nil, 2, uint64(id))), nil
	}
	if name == "label" && value == nameResource {
		return pbBytes(nil, 1, pbVarint(nil, 2, uint64(nameResourceID(icon)))), nil
	}
	if spec.kind == typeString {
		return nil, nil
	}
	v, err := spec.parse(value)
	if err != nil {
		return nil, fmt.Errorf("manifest: android:%s: %w", name, err)
	}
	var prim []byte
	switch spec.kind {
	case typeReference:
		return pbBytes(nil, 1, pbVarint(nil, 2, uint64(v))), nil
	case typeDimension:
		prim = pbVarint(nil, 13, uint64(v))
	case typeInt:
		prim = pbVarint(nil, 6, uint64(int64(int32(v))))
	case typeHex:
		prim = pbVarint(nil, 7, uint64(v))
	case typeBool:
		b := uint64(0)
		if v != 0 {
			b = 1
		}
		prim = pbVarint(nil, 8, b)
	}
	return pbBytes(nil, 7, prim), nil
}

// resourcesProto is the module's resource table, resources.pb, holding
// what resources' binary table does.
func resourcesProto(m Manifest) []byte {
	adaptive := m.Adaptive
	var types [][]byte
	id := uint64(1)
	if m.Icon != nil {
		config := pbVarint(nil, 18, densityXXXHigh)
		config = pbVarint(config, 24, 4)
		file := pbString(nil, 1, iconFile)
		file = pbVarint(file, 2, 1) // PNG
		values := [][2][]byte{{config, pbBytes(nil, 5, file)}}
		entries := []protoEntry{{"ic_launcher", values}}
		if adaptive != nil {
			anydpi := pbVarint(nil, 18, 0xfffe)
			anydpi = pbVarint(anydpi, 24, 26)
			xml := pbString(nil, 1, adaptiveIconFile)
			xml = pbVarint(xml, 2, 3) // PROTO_XML
			entries[0].values = append(entries[0].values, [2][]byte{anydpi, pbBytes(nil, 5, xml)})
			for i, name := range []string{"ic_launcher_foreground", "ic_launcher_background", "ic_launcher_monochrome"} {
				if i == 2 && len(adaptive.Monochrome) == 0 {
					break
				}
				path := []string{foregroundFile, backgroundFile, monochromeFile}[i]
				png := pbString(nil, 1, path)
				png = pbVarint(png, 2, 1)
				entries = append(entries, protoEntry{name, [][2][]byte{{config, pbBytes(nil, 5, png)}}})
			}
		}
		types = append(types, protoTypeEntries(id, "mipmap", entries))
		id++
	}
	if resources := m.stringResources(); len(resources) > 0 {
		var entries []protoEntry
		for _, resource := range resources {
			var langs []string
			for lang := range resource.values {
				if lang == "" || localeConfig(lang) != nil {
					langs = append(langs, lang)
				}
			}
			slices.Sort(langs)
			var values [][2][]byte
			for _, lang := range langs {
				var config []byte
				if lang != "" {
					config = pbString(nil, 3, lang)
				}
				values = append(values, [2][]byte{config, pbBytes(nil, 2, pbString(nil, 1, resource.values[lang]))})
			}
			entries = append(entries, protoEntry{resource.name, values})
		}
		types = append(types, protoTypeEntries(id, "string", entries))
	}
	var xmlEntries []protoEntry
	for _, resource := range m.xmlResources() {
		file := pbString(nil, 1, resource.file)
		file = pbVarint(file, 2, 3) // PROTO_XML
		values := [][2][]byte{{nil, pbBytes(nil, 5, file)}}
		xmlEntries = append(xmlEntries, protoEntry{resource.name, values})
	}
	xmlType := uint64(xmlResourceID(m) >> 16 & 0xff)
	switch len(xmlEntries) {
	case 0:
	case 1:
		entry := xmlEntries[0]
		types = append(types, protoType(xmlType, "xml", entry.key, entry.values))
	default:
		types = append(types, protoTypeEntries(xmlType, "xml", xmlEntries))
	}
	if layouts := m.layoutResources(); len(layouts) > 0 {
		var entries []protoEntry
		for _, res := range layouts {
			file := pbString(nil, 1, res.file)
			file = pbVarint(file, 2, 3)
			entries = append(entries, protoEntry{res.name, [][2][]byte{{nil, pbBytes(nil, 5, file)}}})
		}
		types = append(types, protoTypeEntries(uint64(layoutResourceID(m)>>16&0xff), "layout", entries))
	}
	p := pbBytes(nil, 1, pbVarint(nil, 1, appPackageID))
	p = pbString(p, 2, m.Package)
	for _, t := range types {
		p = pbBytes(p, 3, t)
	}
	return pbBytes(nil, 2, p)
}

type protoEntry struct {
	key    string
	values [][2][]byte
}

func protoTypeEntries(id uint64, name string, entries []protoEntry) []byte {
	out := pbBytes(nil, 1, pbVarint(nil, 1, id))
	out = pbString(out, 2, name)
	for i, entry := range entries {
		e := pbBytes(nil, 1, pbVarint(nil, 1, uint64(i)))
		e = pbString(e, 2, entry.key)
		for _, value := range entry.values {
			cv := pbBytes(nil, 1, value[0])
			cv = pbBytes(cv, 2, pbBytes(nil, 4, value[1]))
			e = pbBytes(e, 6, cv)
		}
		out = pbBytes(out, 3, e)
	}
	return out
}

// protoType is a Type of one entry, key, with an item in each of its
// configurations.
func protoType(id uint64, name, key string, values [][2][]byte) []byte {
	entry := pbBytes(nil, 1, nil) // entry 0
	entry = pbString(entry, 2, key)
	for _, v := range values {
		cv := pbBytes(nil, 1, v[0])
		cv = pbBytes(cv, 2, pbBytes(nil, 4, v[1]))
		entry = pbBytes(entry, 6, cv)
	}
	t := pbBytes(nil, 1, pbVarint(nil, 1, id))
	t = pbString(t, 2, name)
	return pbBytes(t, 3, entry)
}

// The protocol buffer wire format: each field a key, its number and wire
// type, then a varint or a length and bytes.

func pbKey(b []byte, field, wire int) []byte {
	return binary.AppendUvarint(b, uint64(field)<<3|uint64(wire))
}

func pbVarint(b []byte, field int, v uint64) []byte {
	return binary.AppendUvarint(pbKey(b, field, 0), v)
}

func pbBytes(b []byte, field int, v []byte) []byte {
	b = binary.AppendUvarint(pbKey(b, field, 2), uint64(len(v)))
	return append(b, v...)
}

func pbString(b []byte, field int, s string) []byte {
	return pbBytes(b, field, []byte(s))
}
