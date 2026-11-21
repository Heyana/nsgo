// Package emoji holds the Unicode emoji catalog and multi-language CLDR names
// and keywords for base.EmojiPicker, generated from emojibase-data by go
// generate (internal/gen).
//
// Everything is loaded lazily on first call with no package init, so Go's
// linker drops the embedded data completely from apps that never reach
// EmojiPicker or its lookup helpers.
package emoji

//go:generate go run ./internal/gen -o .

import (
	"cmp"
	"compress/flate"
	_ "embed"
	"encoding/binary"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
)

//go:embed emoji.bin
var packedData string

// Category is one of the nine Unicode emoji groups (excluding Component).
type Category uint8

const (
	Smileys Category = iota
	People
	Animals
	Food
	Travel
	Activities
	Objects
	Symbols
	Flags
)

// Entry is one emoji's language-independent data.
type Entry struct {
	Index    int
	Emoji    string
	Category Category
	Skins    []string
}

// LocaleEntry is one emoji's name and keywords in a single language.
type LocaleEntry struct {
	Name           string
	FoldedName     string
	Keywords       []string
	FoldedKeywords []string
}

// Item is a localized emoji returned by lookup and search.
type Item struct {
	Index    int
	Emoji    string
	Name     string
	Keywords []string
	Category Category
	Skins    []string
}

type localeMeta struct {
	name   string
	parent string
	off    uint32
	length uint32
}

type binHeader struct {
	baseOff uint32
	baseLen uint32
	locales []localeMeta
	byName  map[string]localeMeta
}

var (
	headerOnce sync.Once
	headerVal  *binHeader

	baseOnce sync.Once
	baseVal  []Entry

	lookupOnce sync.Once
	lookupVal  map[string]lookupTarget

	localeCache sync.Map // string -> []LocaleEntry
)

type lookupTarget struct {
	index int
	emoji string
}

func header() *binHeader {
	headerOnce.Do(func() {
		d := packedData
		if len(d) < 15 || d[:4] != "NSEM" || d[4] != 1 {
			panic("emoji: corrupt emoji.bin header")
		}
		h := &binHeader{
			baseOff: binary.LittleEndian.Uint32([]byte(d[5:9])),
			baseLen: binary.LittleEndian.Uint32([]byte(d[9:13])),
		}
		count := int(binary.LittleEndian.Uint16([]byte(d[13:15])))
		h.locales = make([]localeMeta, count)
		h.byName = make(map[string]localeMeta, count)
		pos := 15
		for i := range count {
			nLen := int(d[pos])
			pos++
			name := d[pos : pos+nLen]
			pos += nLen
			pLen := int(d[pos])
			pos++
			parent := d[pos : pos+pLen]
			pos += pLen
			off := binary.LittleEndian.Uint32([]byte(d[pos : pos+4]))
			pos += 4
			length := binary.LittleEndian.Uint32([]byte(d[pos : pos+4]))
			pos += 4
			m := localeMeta{name: name, parent: parent, off: off, length: length}
			h.locales[i] = m
			h.byName[name] = m
		}
		headerVal = h
	})
	return headerVal
}

func inflate(off, length uint32) string {
	r := flate.NewReader(strings.NewReader(packedData[off : off+length]))
	raw, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		panic("emoji: inflate failed: " + err.Error())
	}
	return string(raw)
}

// Base returns all Unicode emoji entries in standard order.
func Base() []Entry {
	baseOnce.Do(func() {
		h := header()
		raw := inflate(h.baseOff, h.baseLen)
		lines := strings.Split(raw, "\n")
		entries := make([]Entry, len(lines))
		for i, line := range lines {
			u, rest, _ := strings.Cut(line, "\t")
			gStr, skinsStr, _ := strings.Cut(rest, "\t")
			g, _ := strconv.Atoi(gStr)
			var skins []string
			if skinsStr != "" {
				skins = strings.Split(skinsStr, ",")
			}
			entries[i] = Entry{
				Index:    i,
				Emoji:    u,
				Category: Category(g),
				Skins:    skins,
			}
		}
		baseVal = entries
	})
	return baseVal
}

// SupportedLocales returns the sorted list of locale codes bundled in emoji.bin.
func SupportedLocales() []string {
	h := header()
	out := make([]string, len(h.locales))
	for i, l := range h.locales {
		out[i] = l.name
	}
	return out
}

// ResolveLocale maps a BCP 47 locale tag to the closest supported emoji locale,
// or "" when none matches.
func ResolveLocale(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(tag, "_", "-")))
	if tag == "" {
		return ""
	}
	// Pseudo locales use English emoji data.
	if tag == "en-xa" || tag == "ar-xb" {
		return "en"
	}
	h := header()
	if _, ok := h.byName[tag]; ok {
		return tag
	}
	// Chinese script and region rules.
	if tag == "zh" || strings.HasPrefix(tag, "zh-") {
		parts := strings.Split(tag, "-")
		for _, p := range parts[1:] {
			switch p {
			case "hant", "tw", "hk", "mo":
				return "zh-hant"
			case "hans", "cn", "sg":
				return "zh"
			}
		}
		return "zh"
	}
	// Norwegian Bokmal alias.
	if tag == "no" || strings.HasPrefix(tag, "no-") {
		return "nb"
	}
	// Walk parent subtags (e.g. en-au -> en, de-at -> de, es-419 -> es).
	for cur := tag; ; {
		i := strings.LastIndexByte(cur, '-')
		if i <= 0 {
			break
		}
		cur = cur[:i]
		if _, ok := h.byName[cur]; ok {
			return cur
		}
	}
	return ""
}

// NormalizeLocales resolves langs to deduplicated supported locales, appending
// English as a fallback when it was not already requested.
func NormalizeLocales(langs []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range langs {
		if r := ResolveLocale(l); r != "" && !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	if !seen["en"] {
		out = append(out, "en")
	}
	return out
}

// LocaleTable returns the localized names and keywords for a resolved locale
// code (e.g. "en", "zh", "ja"), inflating it once on demand.
func LocaleTable(loc string) []LocaleEntry {
	if v, ok := localeCache.Load(loc); ok {
		return v.([]LocaleEntry)
	}
	h := header()
	meta, ok := h.byName[loc]
	if !ok {
		if loc == "en" {
			return nil
		}
		return LocaleTable("en")
	}
	n := len(Base())
	var entries []LocaleEntry
	if meta.parent != "" {
		parent := LocaleTable(meta.parent)
		entries = make([]LocaleEntry, len(parent))
		copy(entries, parent)
		raw := inflate(meta.off, meta.length)
		if raw != "" {
			for line := range strings.SplitSeq(raw, "\n") {
				idxStr, rest, ok := strings.Cut(line, "\t")
				if !ok {
					continue
				}
				idx, err := strconv.Atoi(idxStr)
				if err != nil || idx < 0 || idx >= len(entries) {
					continue
				}
				lbl, tags, _ := strings.Cut(rest, "\t")
				entries[idx] = parseLocaleEntry(lbl, tags)
			}
		}
	} else {
		entries = make([]LocaleEntry, n)
		raw := inflate(meta.off, meta.length)
		i := 0
		for line := range strings.SplitSeq(raw, "\n") {
			if i >= n {
				break
			}
			lbl, tags, _ := strings.Cut(line, "\t")
			entries[i] = parseLocaleEntry(lbl, tags)
			i++
		}
	}
	actual, _ := localeCache.LoadOrStore(loc, entries)
	return actual.([]LocaleEntry)
}

func parseLocaleEntry(label, tagsStr string) LocaleEntry {
	var keywords, foldedKeywords []string
	if tagsStr != "" {
		keywords = strings.Split(tagsStr, "|")
		foldedKeywords = make([]string, len(keywords))
		for i, k := range keywords {
			foldedKeywords[i] = strings.ToLower(k)
		}
	}
	return LocaleEntry{
		Name:           label,
		FoldedName:     strings.ToLower(label),
		Keywords:       keywords,
		FoldedKeywords: foldedKeywords,
	}
}

func localizedItem(e Entry, primary, fallback []LocaleEntry) Item {
	le := primary[e.Index]
	if le.Name == "" && fallback != nil {
		le = fallback[e.Index]
	}
	return Item{
		Index:    e.Index,
		Emoji:    e.Emoji,
		Name:     le.Name,
		Keywords: le.Keywords,
		Category: e.Category,
		Skins:    e.Skins,
	}
}

// ItemAt returns the localized Item at index in Base().
func ItemAt(index int, langs []string) Item {
	base := Base()
	if index < 0 || index >= len(base) {
		return Item{}
	}
	locs := NormalizeLocales(langs)
	primary := LocaleTable(locs[0])
	var fallback []LocaleEntry
	if locs[0] != "en" {
		fallback = LocaleTable("en")
	}
	return localizedItem(base[index], primary, fallback)
}

// AllInCategory returns every emoji in cat localized in langs.
func AllInCategory(cat Category, langs []string) []Item {
	base := Base()
	locs := NormalizeLocales(langs)
	primary := LocaleTable(locs[0])
	var fallback []LocaleEntry
	if locs[0] != "en" {
		fallback = LocaleTable("en")
	}
	var out []Item
	for _, e := range base {
		if e.Category == cat {
			out = append(out, localizedItem(e, primary, fallback))
		}
	}
	return out
}

func stripVS16(s string) string {
	return strings.ReplaceAll(s, "\uFE0F", "")
}

func lookupMap() map[string]lookupTarget {
	lookupOnce.Do(func() {
		base := Base()
		m := make(map[string]lookupTarget, len(base)*2)
		add := func(key string, t lookupTarget) {
			if _, exists := m[key]; !exists {
				m[key] = t
			}
		}
		for _, e := range base {
			add(e.Emoji, lookupTarget{index: e.Index, emoji: e.Emoji})
			if stripped := stripVS16(e.Emoji); stripped != e.Emoji {
				add(stripped, lookupTarget{index: e.Index, emoji: e.Emoji})
			}
			for _, sk := range e.Skins {
				add(sk, lookupTarget{index: e.Index, emoji: sk})
				if stripped := stripVS16(sk); stripped != sk {
					add(stripped, lookupTarget{index: e.Index, emoji: sk})
				}
			}
		}
		lookupVal = m
	})
	return lookupVal
}

// ByString looks up an emoji (base or skin-toned, with or without U+FE0F) and
// returns its localized Item, preserving any skin tone in Item.Emoji.
func ByString(s string, langs []string) (Item, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Item{}, false
	}
	m := lookupMap()
	t, ok := m[s]
	if !ok {
		t, ok = m[stripVS16(s)]
		if !ok {
			return Item{}, false
		}
	}
	item := ItemAt(t.index, langs)
	item.Emoji = t.emoji
	return item, true
}

// Search matches every emoji against query across all languages in langs
// (resolved and deduplicated, with English fallback). Empty queries return all
// emojis in Unicode order.
func Search(query string, langs []string) []Item {
	base := Base()
	locs := NormalizeLocales(langs)
	tables := make([][]LocaleEntry, len(locs))
	for i, loc := range locs {
		tables[i] = LocaleTable(loc)
	}
	primary := tables[0]
	var fallback []LocaleEntry
	if locs[0] != "en" {
		fallback = LocaleTable("en")
	}

	raw := strings.TrimSpace(query)
	if raw == "" {
		out := make([]Item, len(base))
		for i, e := range base {
			out[i] = localizedItem(e, primary, fallback)
		}
		return out
	}

	// Direct emoji glyph lookup (e.g. pasting "🔥" or "👋🏽").
	if item, ok := ByString(raw, locs); ok {
		return []Item{item}
	}

	// Strip optional shortcode colons (":fire:" -> "fire").
	folded := strings.ToLower(strings.Trim(raw, ":"))
	if folded == "" {
		return nil
	}
	// Support both space-separated and underscore-separated queries.
	terms := strings.Fields(folded)
	if len(terms) == 0 {
		return nil
	}

	type scored struct {
		idx   int
		score int
	}
	var matches []scored
	for i := range base {
		total := 0
		ok := true
		for _, term := range terms {
			best := -1
			alt := strings.ReplaceAll(term, "_", " ")
			for langIdx, tbl := range tables {
				s := scoreEntry(tbl[i], term)
				if s < 0 && alt != term {
					s = scoreEntry(tbl[i], alt)
				}
				if s >= 0 {
					s += langIdx * 10
					if best < 0 || s < best {
						best = s
					}
				}
			}
			if best < 0 {
				ok = false
				break
			}
			total += best
		}
		if ok {
			matches = append(matches, scored{idx: i, score: total})
		}
	}
	slices.SortFunc(matches, func(a, b scored) int {
		return cmp.Or(cmp.Compare(a.score, b.score), cmp.Compare(a.idx, b.idx))
	})
	out := make([]Item, len(matches))
	for i, m := range matches {
		out[i] = localizedItem(base[m.idx], primary, fallback)
	}
	return out
}

// scoreEntry scores how well term matches e (lower is better, -1 for no match).
func scoreEntry(e LocaleEntry, term string) int {
	name := e.FoldedName
	if name == term && name != "" {
		return 0
	}
	kwBest := -1
	for _, kw := range e.FoldedKeywords {
		var s int
		switch {
		case kw == term:
			s = 3
		case strings.HasPrefix(kw, term):
			s = 6
		case strings.Contains(kw, term):
			s = 7
		default:
			continue
		}
		if kwBest < 0 || s < kwBest {
			kwBest = s
		}
	}
	nameBest := -1
	if name != "" {
		switch {
		case strings.HasPrefix(name, term):
			if kwBest == 3 {
				return 1
			}
			nameBest = 2
		case hasWordPrefix(name, term):
			nameBest = 4
		case strings.Contains(name, term):
			nameBest = 5
		}
	}
	if nameBest >= 0 && (kwBest < 0 || nameBest < kwBest) {
		return nameBest
	}
	return kwBest
}

func hasWordPrefix(s, prefix string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '-' || s[i] == '_' || s[i] == ':' {
			if strings.HasPrefix(s[i+1:], prefix) {
				return true
			}
		}
	}
	return false
}
