// Package pri writes the Windows resource index used by MSIX package manifests. It implements the data-only MRT v2 format described by Microsoft's MIT-licensed MRT Core readers and writers, without invoking Windows tools at build time.
package pri

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/language"
)

// Candidate supplies one string or relative file path, selected by Language or Scale. An empty qualifier is neutral; combining qualifiers is not supported.
type Candidate struct {
	Value, Language string
	Scale           int
}

// Resource is a case-insensitive slash-separated MRT name. Path values refer to files in the package; other values are UTF-16 strings stored in the index.
type Resource struct {
	Name       string
	Path       bool
	Candidates []Candidate
}

type qualifier struct {
	language string
	scale    int
}
type entry struct {
	resource   Resource
	candidates []Candidate
	qualifiers []int
	decision   int
}

// Write builds one complete resources.pri with language and scale candidates. mapName is the package identity name; defaultLanguage marks the app's fallback. It never reads files or mutates the supplied resource and candidate slices.
func Write(mapName, defaultLanguage string, resources []Resource) ([]byte, error) {
	if !validText(mapName) || mapName == "" || len(wide(mapName)) > 1000 || strings.ContainsAny(mapName, "/\\:") {
		return nil, fmt.Errorf("pri: invalid resource map name")
	}
	if _, err := language.Parse(defaultLanguage); err != nil || defaultLanguage == "" {
		return nil, fmt.Errorf("pri: invalid default language")
	}
	if len(resources) == 0 || len(resources) > math.MaxInt16 {
		return nil, fmt.Errorf("pri: invalid resource count")
	}
	ordered := slices.Clone(resources)
	slices.SortFunc(ordered, func(a, b Resource) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	allQualifiers := map[qualifier]bool{}
	seenNames := map[string]bool{}
	for _, r := range ordered {
		if !validName(r.Name) || seenNames[strings.ToLower(r.Name)] || len(r.Candidates) == 0 {
			return nil, fmt.Errorf("pri: invalid or duplicate resource %q", r.Name)
		}
		seenNames[strings.ToLower(r.Name)] = true
		seen := map[qualifier]bool{}
		for _, c := range r.Candidates {
			q := qualifier{strings.ToLower(c.Language), c.Scale}
			if !validText(c.Value) || len(wide(c.Value)) > math.MaxUint16 {
				return nil, fmt.Errorf("pri: resource %q has an invalid or oversized value", r.Name)
			}
			if q.language != "" {
				if _, err := language.Parse(q.language); err != nil {
					return nil, fmt.Errorf("pri: invalid language %q", c.Language)
				}
			}
			if q.scale != 0 && (q.scale < 50 || q.scale > 1000) || q.scale != 0 && q.language != "" || seen[q] {
				return nil, fmt.Errorf("pri: invalid or duplicate qualifier for %q", r.Name)
			}
			if r.Path && (strings.HasPrefix(c.Value, "/") || strings.HasPrefix(c.Value, `\`) || strings.Contains(c.Value, ":") || strings.Contains(c.Value, "..") || c.Value == "") {
				return nil, fmt.Errorf("pri: path value must stay inside its package")
			}
			seen[q] = true
			if q != (qualifier{}) {
				allQualifiers[q] = true
			}
		}
	}
	qs := make([]qualifier, 0, len(allQualifiers))
	for q := range allQualifiers {
		qs = append(qs, q)
	}
	slices.SortFunc(qs, func(a, b qualifier) int {
		if n := strings.Compare(a.language, b.language); n != 0 {
			return n
		}
		return a.scale - b.scale
	})
	if len(qs) > math.MaxInt16 {
		return nil, fmt.Errorf("pri: too many qualifiers")
	}
	// Index zero is the always-true qualifier and neutral set in MRT.
	qs = append([]qualifier{{}}, qs...)
	qi := map[qualifier]int{}
	for i, q := range qs {
		qi[q] = i
	}
	entries := make([]entry, len(ordered))
	decisions := [][]int{nil, {0}}
	decisionIDs := map[string]int{"0": 1}
	values := 0
	for i, r := range ordered {
		e := entry{resource: r, candidates: slices.Clone(r.Candidates)}
		slices.SortFunc(e.candidates, func(a, b Candidate) int {
			return qi[qualifier{strings.ToLower(b.Language), b.Scale}] - qi[qualifier{strings.ToLower(a.Language), a.Scale}]
		})
		var key []string
		for _, c := range e.candidates {
			q := qi[qualifier{strings.ToLower(c.Language), c.Scale}]
			e.qualifiers = append(e.qualifiers, q)
			key = append(key, strconv.Itoa(q))
		}
		k := strings.Join(key, ",")
		id, ok := decisionIDs[k]
		if !ok {
			id = len(decisions)
			decisionIDs[k] = id
			decisions = append(decisions, e.qualifiers)
		}
		e.decision = id
		entries[i] = e
		values += len(e.candidates)
	}
	if values > math.MaxUint16 {
		return nil, fmt.Errorf("pri: too many resource candidates")
	}
	schema, err := schemaSection(mapName, entries)
	if err != nil {
		return nil, err
	}
	decisionsData, err := decisionSection(qs, decisions, defaultLanguage)
	if err != nil {
		return nil, err
	}
	resourceMap, err := mapSection(entries, values)
	if err != nil {
		return nil, err
	}
	descriptor := new(buffer)
	descriptor.u16(2, 0xffff, 0, 1, 1, 1, 3, 0, 0, 0) // deployment-mergeable, no included file
	descriptor.u16(2, 0, 3)                           // schema, decisions and primary map section indices
	return file([]section{{"[mrm_decn_info]", decisionsData}, {"[mrm_pridescex]", descriptor.data}, {"[mrm_hschema]  ", schema}, {"[mrm_res_map2_]", resourceMap}}), nil
}

func validText(s string) bool { return utf8.ValidString(s) && !strings.ContainsRune(s, 0) }
func validName(s string) bool {
	if !validText(s) || s == "" || strings.ContainsAny(s, "\\:") {
		return false
	}
	for _, p := range strings.Split(s, "/") {
		if p == "" || p == "." || p == ".." || len(utf16.Encode([]rune(p))) > 255 {
			return false
		}
	}
	return len(utf16.Encode([]rune(s))) < math.MaxUint16
}

// All lengths and offsets are little-endian, in bytes unless a field says cch.
type buffer struct{ data []byte }

func (b *buffer) u16(v ...int) {
	for _, n := range v {
		b.data = binary.LittleEndian.AppendUint16(b.data, uint16(n))
	}
}
func (b *buffer) u32(v ...int) {
	for _, n := range v {
		b.data = binary.LittleEndian.AppendUint32(b.data, uint32(n))
	}
}
func (b *buffer) pad(n int) {
	for len(b.data)%n != 0 {
		b.data = append(b.data, 0)
	}
}
func wide(s string) []byte {
	b := new(buffer)
	for _, c := range utf16.Encode([]rune(s)) {
		b.u16(int(c))
	}
	b.u16(0)
	return b.data
}

type section struct {
	name string
	data []byte
}

func file(sections []section) []byte {
	body, toc := new(buffer), new(buffer)
	for _, s := range sections {
		name := make([]byte, 16)
		copy(name, s.name)
		size := 32 + (len(s.data)+7)/8*8 + 8
		toc.data = append(toc.data, name...)
		toc.u16(0, 0)
		toc.u32(0, len(body.data), size)
		body.data = append(body.data, name...)
		body.u32(0)
		body.u16(0, 0)
		body.u32(size, 0)
		body.data = append(body.data, s.data...)
		body.pad(8)
		body.u32(0xdef5fade, size)
	}
	out := new(buffer)
	out.data = append(out.data, "mrm_pri2"...)
	out.u16(0, 1)
	total := 32 + len(toc.data) + len(body.data) + 16
	out.u32(total, 32, 32+len(toc.data))
	out.u16(len(sections), 0xffff)
	out.u32(0)
	out.data = append(out.data, toc.data...)
	out.data = append(out.data, body.data...)
	out.u32(0xdefffade, total)
	out.data = append(out.data, "mrm_pri2"...)
	return out.data
}

func decisionSection(qs []qualifier, decisions [][]int, defaultLanguage string) ([]byte, error) {
	refs := []int{0}
	sets := [][2]int{{0, 0}}
	literals := []byte{0, 0}
	offsets := make([]int, len(qs))
	for i, q := range qs[1:] {
		refs = append(refs, i+1)
		sets = append(sets, [2]int{len(refs) - 1, 1})
		value := q.language
		if q.scale != 0 {
			value = strconv.Itoa(q.scale)
		}
		offsets[i+1] = len(literals) / 2
		literals = append(literals, wide(value)...)
	}
	drefs := make([][2]int, len(decisions))
	for i, d := range decisions {
		if i == 0 {
			continue
		}
		if i == 1 {
			drefs[i] = [2]int{0, 1}
			continue
		}
		drefs[i] = [2]int{len(refs), len(d)}
		refs = append(refs, d...)
	}
	if len(literals)/2 > math.MaxUint16 || len(refs) > math.MaxUint16 {
		return nil, fmt.Errorf("pri: decision data exceeds format limits")
	}
	out := new(buffer)
	out.u16(len(qs), len(qs), len(sets), len(decisions), len(refs), len(literals)/2)
	for _, d := range drefs {
		out.u16(d[0], d[1])
	}
	for _, s := range sets {
		out.u16(s[0], s[1])
	}
	out.u16(0, 0, 0, 0)
	for i, q := range qs[1:] {
		priority, score := 700, 0
		if q.language == strings.ToLower(defaultLanguage) {
			score = 1000
		}
		if q.scale != 0 {
			priority = 200
			score = 750
			if q.scale == 100 {
				score = 1000
			}
		}
		out.u16(i+1, priority, score, 0)
	}
	out.u16(0, 0, 0, 1)
	out.u32(0) // always true
	for i, q := range qs[1:] {
		attribute := 0
		if q.scale != 0 {
			attribute = 2
		}
		out.u16(2, attribute, 0, 10)
		out.u32(offsets[i+1])
	}
	for _, r := range refs {
		out.u16(r)
	}
	out.data = append(out.data, literals...)
	return out.data, nil
}

func mapSection(entries []entry, values int) ([]byte, error) {
	out := new(buffer)
	out.u16(0, 0, 2, 0, 0, 2, 1, 1, len(entries), 0)
	out.u32(values, 0, 0)
	// WinCore's value-type pool is 4; UTF-16 String=0 and Path=1.
	out.u32(4, 0, 4, 1)
	out.u16(0, 0)
	out.u16(len(entries), 0)
	first := 0
	for _, e := range entries {
		out.u16(e.decision, first)
		first += len(e.candidates)
	}
	data := new(buffer)
	for _, e := range entries {
		kind := byte(0)
		if e.resource.Path {
			kind = 1
		}
		for _, c := range e.candidates {
			value := wide(c.Value)
			out.data = append(out.data, 0, kind)
			out.u16(len(value))
			out.u32(len(data.data))
			data.data = append(data.data, value...)
		}
	}
	if uint64(len(data.data)) > math.MaxUint32 {
		return nil, fmt.Errorf("pri: resource data too large")
	}
	binary.LittleEndian.PutUint32(out.data[24:], uint32(len(data.data)))
	out.data = append(out.data, data.data...)
	out.pad(4)
	return out.data, nil
}

type node struct {
	name, path             string
	parent, index, payload int
	scope                  bool
	children               []*node
}

func schemaSection(mapName string, entries []entry) ([]byte, error) {
	root := &node{scope: true}
	byPath := map[string]*node{"": root}
	items := make([]*node, len(entries))
	for i, e := range entries {
		parent := root
		parts := strings.Split(e.resource.Name, "/")
		for j, name := range parts {
			full := name
			if parent.path != "" {
				full = parent.path + "/" + name
			}
			key := strings.ToLower(full)
			n := byPath[key]
			isScope := j < len(parts)-1
			if n == nil {
				n = &node{name: name, path: full, scope: isScope}
				if !isScope {
					n.payload = i
					items[i] = n
				}
				byPath[key] = n
				parent.children = append(parent.children, n)
			} else if !n.scope || !isScope {
				return nil, fmt.Errorf("pri: resource and subtree overlap")
			}
			parent = n
		}
	}
	nodes := []*node{root}
	var scopes []*node
	for i := 0; i < len(nodes); i++ {
		n := nodes[i]
		n.index = i
		if n.scope {
			n.payload = len(scopes)
			scopes = append(scopes, n)
			slices.SortFunc(n.children, func(a, b *node) int { return strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name)) })
			for _, c := range n.children {
				c.parent = i
				nodes = append(nodes, c)
			}
		}
	}
	if len(nodes) > math.MaxUint16 {
		return nil, fmt.Errorf("pri: resource hierarchy too large")
	}
	names := []byte{0, 0}
	offsets := make([]int, len(nodes))
	longest := 0
	for i, n := range nodes {
		if i == 0 {
			continue
		}
		offsets[i] = len(names) / 2
		names = append(names, wide(n.name)...)
		longest = max(longest, len(utf16.Encode([]rune(n.path))))
	}
	if len(names)/2 > math.MaxUint16 {
		return nil, fmt.Errorf("pri: resource names too large")
	}
	hierarchy := new(buffer)
	hierarchy.u16(longest, 0)
	hierarchy.u32(len(nodes), len(scopes), len(items), len(names)/2, 0)
	for i, n := range nodes {
		flags := 0
		if n.scope {
			flags = 0x10
		}
		initial := 0
		if n.name != "" {
			initial = int(utf16.Encode([]rune{unicode.ToUpper([]rune(n.name)[0])})[0])
		}
		hierarchy.u16(n.parent, len(utf16.Encode([]rune(n.path))), initial)
		hierarchy.data = append(hierarchy.data, byte(len(utf16.Encode([]rune(n.name)))), byte(flags))
		hierarchy.u16(offsets[i], n.payload)
	}
	for _, n := range scopes {
		first := 0
		if len(n.children) > 0 {
			first = n.children[0].index
		}
		hierarchy.u16(n.index, len(n.children), first, 0)
	}
	for _, n := range items {
		hierarchy.u16(n.index)
	}
	hierarchy.data = append(hierarchy.data, names...)
	hierarchy.pad(8)
	binary.LittleEndian.PutUint32(hierarchy.data[20:], uint32(len(hierarchy.data)))
	unique := "ms-appx://" + mapName + "/"
	scopeNames := make([]string, len(scopes))
	itemNames := make([]string, len(items))
	for i, n := range scopes {
		scopeNames[i] = n.path
	}
	for i, n := range items {
		itemNames[i] = n.path
	}
	checksum := schemaChecksum(unique, mapName, scopeNames, itemNames)
	out := new(buffer)
	out.u16(1, len(wide(unique))/2, len(wide(mapName))/2, 0)
	out.u16(1, 0)
	out.u32(0, int(checksum), len(scopes), len(items))
	out.data = append(out.data, wide(unique)...)
	out.data = append(out.data, wide(mapName)...)
	out.pad(4)
	out.data = append(out.data, hierarchy.data...)
	return out.data, nil
}

func schemaChecksum(unique, simple string, scopes, items []string) uint32 {
	crc := uint32(0)
	u32 := func(n uint32) {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], n)
		crc = crc32.Update(crc, crc32.IEEETable, b[:])
	}
	str := func(s string) {
		b := wide(strings.ToLower(s))
		u32(uint32(len(b)))
		crc = crc32.Update(crc, crc32.IEEETable, b)
	}
	str(unique)
	str(simple)
	u32(1)
	for _, pool := range [][]string{scopes, items} {
		u32(0)
		u32(0)
		u32(1)
		u32(uint32(len(pool)))
		for _, s := range pool {
			str(s)
		}
	}
	return crc
}
