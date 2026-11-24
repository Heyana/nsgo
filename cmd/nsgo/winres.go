package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"slices"
	"strings"
	"unicode/utf16"
)

// The COFF and resource constants windowsResource writes, from the PE format specification.
const (
	rtIcon      = 3
	rtGroupIcon = 14
	rtVersion   = 16
	// iconGroupID is the group platform/windows loads the window's icon
	// from; Explorer shows the first group.
	iconGroupID = 1
	langEnUS    = 0x0409
	// langUserDefault is MAKELANGID(LANG_NEUTRAL, SUBLANG_DEFAULT), the user's default language.
	langUserDefault = 0x0400
)

// windowsVersion is what an executable's version resource says.
type windowsVersion struct {
	// Fixed is the version as four numbers, which Windows compares.
	Fixed [4]uint16
	// Version is the version as people read it, FileVersion and ProductVersion.
	Version string
	// Exe is the executable's file name, InternalName and OriginalFilename.
	Exe string
	// Company and Copyright are CompanyName and LegalCopyright, left out when empty.
	Company, Copyright string
	// Tables name the app in each language, the source's first, each in a version resource of its own.
	Tables []windowsVersionTable
}

// windowsVersionTable is the app's name in one language, its ProductName and FileDescription.
type windowsVersionTable struct {
	Language uint16
	Name     string
}

// windowsResource writes a COFF object with a .rsrc section holding an icon group of PNG images, when there are images, and the version resource, when there is one, which go build links into a Windows executable. Both share the one object, as the linker takes .rsrc from one object alone.
func windowsResource(goarch string, images [][]byte, sizes []int, version *windowsVersion) ([]byte, error) {
	return windowsResourceWithManifest(goarch, images, sizes, version, "", nil)
}

// windowsIconEntry preserves an ICO's dimensions, palette and bit depth. App PNG icons use the default 32-bit entries; installer ICOs may still hold DIBs.
type windowsIconEntry struct {
	Shape  [4]uint8
	Format [2]uint16
}

func windowsResourceWithManifest(goarch string, images [][]byte, sizes []int, version *windowsVersion, manifest string, iconEntries []windowsIconEntry) ([]byte, error) {
	machine, reloc := uint16(0), uint16(0)
	switch goarch {
	case "amd64":
		machine, reloc = 0x8664, 0x0003 // IMAGE_REL_AMD64_ADDR32NB
	case "arm64":
		machine, reloc = 0xaa64, 0x0002 // IMAGE_REL_ARM64_ADDR32NB
	case "386":
		machine, reloc = 0x014c, 0x0007 // IMAGE_REL_I386_DIR32NB
	default:
		return nil, fmt.Errorf("no Windows resources for %s", goarch)
	}
	if len(images) == 0 && version == nil && manifest == "" {
		return nil, errors.New("a Windows resource object needs an icon or a version")
	}

	// The resource tree has three levels: type, ID and language, each sorted by number, as the loader searches them. Every directory comes first, then the data entries, then the data.
	type leaf struct {
		lang uint32
		data []byte
	}
	type resource struct {
		id    uint32
		langs []leaf
	}
	type resourceType struct {
		id        uint32
		resources []resource
	}
	var types []resourceType
	put := func(b *bytes.Buffer, v any) { binary.Write(b, binary.LittleEndian, v) }
	if len(images) > 0 {
		// The group lists each image with the ID of its RT_ICON resource.
		var group bytes.Buffer
		put(&group, [3]uint16{0, 1, uint16(len(images))})
		icons := resourceType{id: rtIcon}
		for i, img := range images {
			side := uint8(sizes[i])
			if sizes[i] >= 256 {
				side = 0
			}
			shape, format := [4]uint8{side, side, 0, 0}, [2]uint16{1, 32}
			if i < len(iconEntries) {
				shape, format = iconEntries[i].Shape, iconEntries[i].Format
			}
			put(&group, shape)
			put(&group, format)
			put(&group, uint32(len(img)))
			put(&group, uint16(i+1))
			icons.resources = append(icons.resources, resource{uint32(i + 1), []leaf{{langEnUS, img}}})
		}
		types = append(types, icons, resourceType{rtGroupIcon, []resource{{iconGroupID, []leaf{{langEnUS, group.Bytes()}}}}})
	}
	if version != nil {
		// A version resource per language: the loader picks the one in the user's language, else the source's, filed as the user's default language (LANG_NEUTRAL, SUBLANG_DEFAULT), which it falls back to. Filed as LANG_NEUTRAL alone it would come before the user's language, and under its own ID after the lowest ID present.
		v := resource{id: 1}
		for i, t := range version.Tables {
			lang := uint32(t.Language)
			if i == 0 {
				lang = langUserDefault
			}
			v.langs = append(v.langs, leaf{lang, versionInfo(version, t)})
		}
		slices.SortFunc(v.langs, func(a, b leaf) int { return int(a.lang) - int(b.lang) })
		types = append(types, resourceType{rtVersion, []resource{v}})
	}
	if manifest != "" {
		types = append(types, resourceType{24, []resource{{1, []leaf{{0, []byte(manifest)}}}}})
	}
	const dirSize, entrySize, dataEntrySize = 16, 8, 16
	leaves, ids := 0, 0
	for _, t := range types {
		ids += len(t.resources)
		for _, r := range t.resources {
			leaves += len(r.langs)
		}
	}
	idDirs := dirSize + entrySize*len(types)
	langDirs := idDirs
	for _, t := range types {
		langDirs += dirSize + entrySize*len(t.resources)
	}
	dataEntries := langDirs + ids*dirSize + leaves*entrySize
	data := dataEntries + leaves*dataEntrySize

	var sec bytes.Buffer
	dir := func(entries int) { put(&sec, [4]uint32{0, 0, 0, uint32(entries) << 16}) }
	const subdir = 0x80000000
	dir(len(types))
	at := idDirs
	for _, t := range types {
		put(&sec, [2]uint32{t.id, subdir | uint32(at)})
		at += dirSize + entrySize*len(t.resources)
	}
	at = langDirs
	for _, t := range types {
		dir(len(t.resources))
		for _, r := range t.resources {
			put(&sec, [2]uint32{r.id, subdir | uint32(at)})
			at += dirSize + entrySize*len(r.langs)
		}
	}
	at = dataEntries
	for _, t := range types {
		for _, r := range t.resources {
			dir(len(r.langs))
			for _, l := range r.langs {
				put(&sec, [2]uint32{l.lang, uint32(at)})
				at += dataEntrySize
			}
		}
	}
	// Each data entry's address is relative to the section; the linker
	// relocates it to an image address.
	var relocs []uint32
	at = data
	for _, t := range types {
		for _, r := range t.resources {
			for _, l := range r.langs {
				relocs = append(relocs, uint32(sec.Len()))
				put(&sec, [4]uint32{uint32(at), uint32(len(l.data)), 0, 0})
				at += align8(len(l.data))
			}
		}
	}
	for _, t := range types {
		for _, r := range t.resources {
			for _, l := range r.langs {
				sec.Write(l.data)
				sec.Write(make([]byte, align8(len(l.data))-len(l.data)))
			}
		}
	}

	const fileHeader, sectionHeader, relocSize = 20, 40, 10
	rawData := fileHeader + sectionHeader
	relocations := rawData + sec.Len()
	symbols := relocations + relocSize*len(relocs)
	var out bytes.Buffer
	put(&out, machine)
	put(&out, uint16(1))                                           // sections
	put(&out, [3]uint32{0, uint32(symbols), 1})                    // time, symbol table, symbols
	put(&out, [2]uint16{0, 0})                                     // optional header, characteristics
	out.WriteString(".rsrc\x00\x00\x00")                           // name
	put(&out, [4]uint32{0, 0, uint32(sec.Len()), uint32(rawData)}) // virtual size and address, raw size and offset
	put(&out, [2]uint32{uint32(relocations), 0})                   // relocations, line numbers
	put(&out, [2]uint16{uint16(len(relocs)), 0})
	put(&out, uint32(0x40000040)) // initialized data, readable
	out.Write(sec.Bytes())
	for _, r := range relocs {
		put(&out, [2]uint32{r, 0})
		put(&out, reloc)
	}
	// One symbol, the section, which every relocation refers to.
	out.WriteString(".rsrc\x00\x00\x00")
	put(&out, uint32(0))
	put(&out, int16(1))
	put(&out, uint16(0))
	put(&out, [2]uint8{3, 0}) // static, no auxiliary records
	put(&out, uint32(4))      // an empty string table
	return out.Bytes(), nil
}

func align8(n int) int { return (n + 7) &^ 7 }

// versionInfo encodes a VS_VERSIONINFO in t's language: the fixed version, the string table keyed by the language and code page 1200, UTF-16, and VarFileInfo's Translation naming that pair, which tells readers the table to read.
func versionInfo(v *windowsVersion, t windowsVersionTable) []byte {
	le := binary.LittleEndian
	fixed := make([]byte, 52)                                           // VS_FIXEDFILEINFO
	le.PutUint32(fixed[0:], 0xFEEF04BD)                                 // dwSignature
	le.PutUint32(fixed[4:], 0x00010000)                                 // dwStrucVersion
	le.PutUint32(fixed[8:], uint32(v.Fixed[0])<<16|uint32(v.Fixed[1]))  // dwFileVersionMS
	le.PutUint32(fixed[12:], uint32(v.Fixed[2])<<16|uint32(v.Fixed[3])) // dwFileVersionLS
	copy(fixed[16:24], fixed[8:16])                                     // dwProductVersionMS and LS
	le.PutUint32(fixed[24:], 0x3F)                                      // dwFileFlagsMask: VS_FFI_FILEFLAGSMASK
	le.PutUint32(fixed[32:], 0x00040004)                                // dwFileOS: VOS_NT_WINDOWS32
	le.PutUint32(fixed[36:], 1)                                         // dwFileType: VFT_APP

	var strs []versionNode
	for _, s := range [][2]string{
		{"CompanyName", v.Company},
		{"FileDescription", t.Name},
		{"FileVersion", v.Version},
		{"InternalName", v.Exe},
		{"LegalCopyright", v.Copyright},
		{"OriginalFilename", v.Exe},
		{"ProductName", t.Name},
		{"ProductVersion", v.Version},
	} {
		if s[1] != "" {
			strs = append(strs, versionNode{key: s[0], text: true, value: utf16z(s[1])})
		}
	}
	table := versionNode{key: fmt.Sprintf("%04x04b0", t.Language), text: true, children: strs}
	translation := le.AppendUint16(le.AppendUint16(nil, t.Language), 1200)
	return versionNode{key: "VS_VERSION_INFO", value: fixed, children: []versionNode{
		{key: "StringFileInfo", text: true, children: []versionNode{table}},
		{key: "VarFileInfo", text: true, children: []versionNode{{key: "Translation", value: translation}}},
	}}.encode()
}

// versionNode is a node of a version resource: its length, its value's length, its type, a UTF-16 key, the value and the children, each on a 32-bit boundary.
type versionNode struct {
	key string
	// text is a text value, whose length counts UTF-16 characters, else a binary one, counted in bytes.
	text     bool
	value    []byte
	children []versionNode
}

func (n versionNode) encode() []byte {
	b := append(make([]byte, 6), utf16z(n.key)...)
	pad := func() {
		for len(b)%4 != 0 {
			b = append(b, 0)
		}
	}
	pad()
	b = append(b, n.value...)
	for _, child := range n.children {
		pad()
		b = append(b, child.encode()...)
	}
	valueLength, typ := len(n.value), uint16(0)
	if n.text {
		valueLength, typ = len(n.value)/2, 1
	}
	binary.LittleEndian.PutUint16(b[0:], uint16(len(b)))
	binary.LittleEndian.PutUint16(b[2:], uint16(valueLength))
	binary.LittleEndian.PutUint16(b[4:], typ)
	return b
}

// utf16z is s in UTF-16, little-endian, with a terminating zero.
func utf16z(s string) []byte {
	var b []byte
	for _, u := range utf16.Encode([]rune(s + "\x00")) {
		b = binary.LittleEndian.AppendUint16(b, u)
	}
	return b
}

// windowsVersionParts is a version as Windows' file versions take it, four numbers: 1.2.0-beta.1 is 1.2.0.0.
func windowsVersionParts(v string) [4]uint16 {
	var out [4]uint16
	for i, part := range strings.Split(strings.TrimPrefix(v, "v"), ".") {
		n := 0
		for _, r := range part {
			if r < '0' || r > '9' {
				break
			}
			n = n*10 + int(r-'0')
		}
		out[i] = uint16(n % 65536)
		if i == 3 || strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			break
		}
	}
	return out
}

// windowsVersion is the version resource of the app's executables: version, with build_number as the fourth number where it fits in one, the app's name in each language its catalogs name it in, else name, and the publisher and copyright nsgo.yaml gives.
func (p *project) windowsVersion() (*windowsVersion, error) {
	v := &windowsVersion{
		Fixed:     windowsVersionParts(p.Version),
		Version:   p.Version,
		Exe:       p.Name + ".exe",
		Company:   p.windowsPublisher(),
		Copyright: p.Copyright,
	}
	if p.BuildNumber > 0 && p.BuildNumber < 65536 {
		v.Fixed[3] = uint16(p.BuildNumber)
	}
	n, err := p.appNames()
	if err != nil {
		return nil, err
	}
	if n == nil {
		id, _ := windowsLanguageID(p.I18n.Source)
		v.Tables = []windowsVersionTable{{id, p.Name}}
		return v, nil
	}
	for i, lang := range n.languages() {
		id, ok := windowsLanguageID(lang)
		if !ok && i > 0 {
			continue
		}
		if !ok {
			// A source language Windows has no ID for names its table language neutral.
			id = 0
		}
		if slices.ContainsFunc(v.Tables, func(t windowsVersionTable) bool { return t.Language == id }) {
			continue
		}
		v.Tables = append(v.Tables, windowsVersionTable{id, n.names[lang]})
	}
	return v, nil
}

// windowsPublisher is who publishes the app on Windows, which its executables' version resource and its installer's uninstall entry name: windows.publisher, else the Microsoft Store's publisher_name, else "".
func (p *project) windowsPublisher() string {
	if p.Windows.Publisher != "" {
		return p.Windows.Publisher
	}
	return p.Windows.Store.PublisherName
}

// windowsLanguageID is Windows' ID of a language tag, as 0x0409 for en-US: of the language and region, else of the language and script, else of the language alone, with Chinese by its script or region.
func windowsLanguageID(tag string) (uint16, bool) {
	parts := strings.Split(strings.ToLower(strings.ReplaceAll(tag, "_", "-")), "-")
	lang, script, region := parts[0], "", ""
	for _, part := range parts[1:] {
		switch {
		case len(part) == 4 && script == "" && region == "":
			script = part
		case region == "" && (len(part) == 2 || len(part) == 3 && part[0] >= '0' && part[0] <= '9'):
			region = part
		}
	}
	if lang == "zh" && script == "" {
		script = "hans"
		if region == "tw" || region == "hk" || region == "mo" {
			script = "hant"
		}
	}
	var keys []string
	if region != "" {
		keys = append(keys, lang+"-"+region)
	}
	if script != "" {
		keys = append(keys, lang+"-"+script)
	}
	for _, key := range append(keys, lang) {
		if id, ok := windowsLanguageIDs[key]; ok {
			return id, true
		}
	}
	return 0, false
}

// windowsLanguageIDs are Windows' language IDs, by language and region, language and script, or language alone, lowercase.
var windowsLanguageIDs = map[string]uint16{
	"af": 0x0436, "am": 0x045e, "ar": 0x0401, "ar-eg": 0x0c01, "ar-sa": 0x0401,
	"az": 0x042c, "az-latn": 0x042c, "ba": 0x046d, "be": 0x0423, "bg": 0x0402,
	"bn": 0x0445, "bs": 0x141a, "ca": 0x0403, "co": 0x0483, "cs": 0x0405,
	"cy": 0x0452, "da": 0x0406, "de": 0x0407, "de-at": 0x0c07, "de-ch": 0x0807,
	"de-de": 0x0407, "el": 0x0408, "en": 0x0409, "en-au": 0x0c09, "en-ca": 0x1009,
	"en-gb": 0x0809, "en-ie": 0x1809, "en-in": 0x4009, "en-nz": 0x1409, "en-sg": 0x4809,
	"en-us": 0x0409, "en-za": 0x1c09, "es": 0x0c0a, "es-ar": 0x2c0a,
	"es-cl": 0x340a, "es-co": 0x240a, "es-es": 0x0c0a, "es-mx": 0x080a, "es-us": 0x540a,
	"es-419": 0x580a, "et": 0x0425, "eu": 0x042d, "fa": 0x0429, "fi": 0x040b,
	"fil": 0x0464, "fo": 0x0438, "fr": 0x040c, "fr-be": 0x080c, "fr-ca": 0x0c0c,
	"fr-ch": 0x100c, "fr-fr": 0x040c, "ga": 0x083c, "gd": 0x0491, "gl": 0x0456,
	"gsw": 0x0484, "gu": 0x0447, "ha": 0x0468, "he": 0x040d, "hi": 0x0439,
	"hr": 0x041a, "hsb": 0x042e, "hu": 0x040e, "hy": 0x042b, "id": 0x0421,
	"ig": 0x0470, "ii": 0x0478, "is": 0x040f, "it": 0x0410, "it-ch": 0x0810,
	"it-it": 0x0410, "iu": 0x045d, "ja": 0x0411, "ka": 0x0437, "kk": 0x043f,
	"kl": 0x046f, "km": 0x0453, "kn": 0x044b, "ko": 0x0412, "kok": 0x0457,
	"ky": 0x0440, "lb": 0x046e, "lo": 0x0454, "lt": 0x0427, "lv": 0x0426,
	"mi": 0x0481, "mk": 0x042f, "ml": 0x044c, "mn": 0x0450, "moh": 0x047c,
	"mr": 0x044e, "ms": 0x043e, "mt": 0x043a, "nb": 0x0414, "ne": 0x0461,
	"nl": 0x0413, "nl-be": 0x0813, "nl-nl": 0x0413, "nn": 0x0814, "no": 0x0414,
	"nso": 0x046c, "oc": 0x0482, "or": 0x0448, "pa": 0x0446, "pl": 0x0415,
	"prs": 0x048c, "ps": 0x0463, "pt": 0x0416, "pt-br": 0x0416, "pt-pt": 0x0816,
	"qut": 0x0486, "quz": 0x046b, "rm": 0x0417, "ro": 0x0418, "ru": 0x0419,
	"rw": 0x0487, "sa": 0x044f, "sah": 0x0485, "se": 0x043b, "si": 0x045b,
	"sk": 0x041b, "sl": 0x0424, "sq": 0x041c, "sr": 0x281a, "sr-cyrl": 0x281a,
	"sr-latn": 0x241a, "sv": 0x041d, "sw": 0x0441, "ta": 0x0449, "te": 0x044a,
	"tg": 0x0428, "th": 0x041e, "tn": 0x0432, "tr": 0x041f, "tt": 0x0444,
	"ug": 0x0480, "uk": 0x0422, "ur": 0x0420, "uz": 0x0443, "vi": 0x042a,
	"wo": 0x0488, "xh": 0x0434, "yo": 0x046a, "zh": 0x0804, "zh-cn": 0x0804,
	"zh-hans": 0x0804, "zh-hant": 0x0404, "zh-hk": 0x0c04, "zh-mo": 0x1404, "zh-sg": 0x1004,
	"zh-tw": 0x0404, "zu": 0x0435,
}

// icoFile is an .ico of img in the sizes Windows picks from, each a PNG.
func icoFile(img image.Image) []byte {
	var out bytes.Buffer
	put := func(v any) { binary.Write(&out, binary.LittleEndian, v) }
	put([3]uint16{0, 1, uint16(len(windowsIconSizes))})
	offset := 6 + 16*len(windowsIconSizes)
	var images [][]byte
	for _, size := range windowsIconSizes {
		data := scaledPNG(img, size)
		images = append(images, data)
		side := uint8(size)
		if size >= 256 {
			side = 0
		}
		put([4]uint8{side, side, 0, 0})
		put([2]uint16{1, 32})
		put([2]uint32{uint32(len(data)), uint32(offset)})
		offset += len(data)
	}
	for _, data := range images {
		out.Write(data)
	}
	return out.Bytes()
}
