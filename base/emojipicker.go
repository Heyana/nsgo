package base

import (
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/base/internal/emoji"
	"github.com/nsgo-dev/nsgo/base/locales"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
)

// EmojiCategory is a Unicode emoji group, or EmojiRecent for recently used
// emoji when EmojiPicker.Recent supplies some.
type EmojiCategory uint8

const (
	EmojiSmileys EmojiCategory = iota
	EmojiPeople
	EmojiAnimals
	EmojiFood
	EmojiTravel
	EmojiActivities
	EmojiObjects
	EmojiSymbols
	EmojiFlags
	// EmojiRecent is the virtual category shown before the Unicode groups when
	// EmojiPicker.Recent is non-empty.
	EmojiRecent
)

// EmojiCategories returns the nine standard Unicode emoji categories in order.
func EmojiCategories() []EmojiCategory {
	return []EmojiCategory{
		EmojiSmileys, EmojiPeople, EmojiAnimals, EmojiFood, EmojiTravel,
		EmojiActivities, EmojiObjects, EmojiSymbols, EmojiFlags,
	}
}

// Label returns the category's localized name in the language the app shows
// under cx (base/locales).
func (c EmojiCategory) Label(cx *core.Scope) string {
	switch c {
	case EmojiRecent:
		return locales.EmojiCategoryRecent(cx)
	case EmojiSmileys:
		return locales.EmojiCategorySmileys(cx)
	case EmojiPeople:
		return locales.EmojiCategoryPeople(cx)
	case EmojiAnimals:
		return locales.EmojiCategoryAnimals(cx)
	case EmojiFood:
		return locales.EmojiCategoryFood(cx)
	case EmojiTravel:
		return locales.EmojiCategoryTravel(cx)
	case EmojiActivities:
		return locales.EmojiCategoryActivities(cx)
	case EmojiObjects:
		return locales.EmojiCategoryObjects(cx)
	case EmojiSymbols:
		return locales.EmojiCategorySymbols(cx)
	case EmojiFlags:
		return locales.EmojiCategoryFlags(cx)
	default:
		return ""
	}
}

// IconEmoji returns a representative emoji for the category.
func (c EmojiCategory) IconEmoji() string {
	switch c {
	case EmojiRecent:
		return "🕒"
	case EmojiSmileys:
		return "😀"
	case EmojiPeople:
		return "👋"
	case EmojiAnimals:
		return "🐻"
	case EmojiFood:
		return "🍔"
	case EmojiTravel:
		return "✈️"
	case EmojiActivities:
		return "⚽"
	case EmojiObjects:
		return "💡"
	case EmojiSymbols:
		return "❤️"
	case EmojiFlags:
		return "🏳️"
	default:
		return "😀"
	}
}

// EmojiSkinTone is a Fitzpatrick skin tone modifier applied to emoji that
// support skin tones. The zero value is the default (yellow/neutral) tone.
type EmojiSkinTone uint8

const (
	SkinToneDefault EmojiSkinTone = iota
	SkinToneLight
	SkinToneMediumLight
	SkinToneMedium
	SkinToneMediumDark
	SkinToneDark
)

// EmojiSkinTones returns all six skin tone choices in order.
func EmojiSkinTones() []EmojiSkinTone {
	return []EmojiSkinTone{
		SkinToneDefault, SkinToneLight, SkinToneMediumLight,
		SkinToneMedium, SkinToneMediumDark, SkinToneDark,
	}
}

// Label returns the skin tone's localized name in the language the app shows
// under cx (base/locales).
func (t EmojiSkinTone) Label(cx *core.Scope) string {
	switch t {
	case SkinToneLight:
		return locales.EmojiSkinToneLight(cx)
	case SkinToneMediumLight:
		return locales.EmojiSkinToneMediumLight(cx)
	case SkinToneMedium:
		return locales.EmojiSkinToneMedium(cx)
	case SkinToneMediumDark:
		return locales.EmojiSkinToneMediumDark(cx)
	case SkinToneDark:
		return locales.EmojiSkinToneDark(cx)
	default:
		return locales.EmojiSkinToneDefault(cx)
	}
}

// Emoji is one entry of the Unicode emoji catalog.
type Emoji struct {
	// Emoji is the Unicode sequence for this emoji, with any active skin tone
	// applied.
	Emoji string
	// Name is the emoji's localized CLDR short name in the active language.
	Name string
	// Keywords are the emoji's localized search keywords.
	Keywords []string
	// Category is the emoji's Unicode category.
	Category EmojiCategory
	// HasSkinTones reports whether skin tone variants exist for this emoji.
	HasSkinTones bool
	// Skins holds the supported skin tone variants, including mixed tones for
	// emoji with more than one person, when HasSkinTones is true.
	Skins []string
}

// WithSkinTone returns a copy of e with tone applied when e supports skin
// tones.
func (e Emoji) WithSkinTone(tone EmojiSkinTone) Emoji {
	if !e.HasSkinTones || tone > SkinToneDark {
		return e
	}
	if tone == SkinToneDefault {
		if it, ok := emoji.ByString(e.Emoji, nil); ok {
			e.Emoji = emoji.Base()[it.Index].Emoji
		}
		return e
	}
	want := rune(0x1f3fa) + rune(tone)
	for _, variant := range e.Skins {
		found, same := false, true
		for _, r := range variant {
			if r >= 0x1f3fb && r <= 0x1f3ff {
				found = true
				if r != want {
					same = false
				}
			}
		}
		if found && same {
			e.Emoji = variant
			break
		}
	}
	return e
}

func emojiFromItem(it emoji.Item) Emoji {
	return Emoji{
		Emoji:        it.Emoji,
		Name:         it.Name,
		Keywords:     slices.Clone(it.Keywords),
		Category:     EmojiCategory(it.Category),
		HasSkinTones: len(it.Skins) > 0,
		Skins:        slices.Clone(it.Skins),
	}
}

// SearchEmojis searches every Unicode emoji by its names, keywords and
// shortcodes across languages (BCP 47 tags such as "zh-Hans", "ja", "de",
// falling back to English). An empty query returns every emoji in Unicode
// order.
func SearchEmojis(query string, languages ...string) []Emoji {
	items := emoji.Search(query, languages)
	out := make([]Emoji, len(items))
	for i, it := range items {
		out[i] = emojiFromItem(it)
	}
	return out
}

// EmojisInCategory returns every emoji in category with its name and keywords
// localized in the first supported language of languages (defaulting to
// English).
func EmojisInCategory(category EmojiCategory, languages ...string) []Emoji {
	if category > EmojiFlags {
		return nil
	}
	items := emoji.AllInCategory(emoji.Category(category), languages)
	out := make([]Emoji, len(items))
	for i, it := range items {
		out[i] = emojiFromItem(it)
	}
	return out
}

// EmojiByString looks up an emoji (base or skin-toned) by its Unicode string
// and returns its localized metadata.
func EmojiByString(s string, languages ...string) (Emoji, bool) {
	it, ok := emoji.ByString(s, languages)
	if !ok {
		return Emoji{}, false
	}
	return emojiFromItem(it), true
}

// EmojiPickerRef is an EmojiPicker's imperative handle.
type EmojiPickerRef struct {
	focus       func()
	query       func() string
	setQuery    func(string)
	setCategory func(EmojiCategory)
	count       func() int
}

// Query returns the current search query, or an empty string while unmounted.
func (r *EmojiPickerRef) Query() string {
	if r.query == nil {
		return ""
	}
	return r.query()
}

// SetQuery requests a new search query, as typing in the field does.
func (r *EmojiPickerRef) SetQuery(query string) {
	if r.setQuery != nil {
		r.setQuery(query)
	}
}

// Focus moves keyboard focus to the search field, or the picker root when
// NoSearch is set.
func (r *EmojiPickerRef) Focus() {
	if r.focus != nil {
		r.focus()
	}
}

// SetCategory jumps to category, clearing any active search query.
func (r *EmojiPickerRef) SetCategory(c EmojiCategory) {
	if r.setCategory != nil {
		r.setCategory(c)
	}
}

// MatchedCount reports how many emoji are currently shown.
func (r *EmojiPickerRef) MatchedCount() int {
	if r.count == nil {
		return 0
	}
	return r.count()
}

// EmojiPicker is an unstyled emoji picker: a search field matching every
// Unicode emoji by its localized CLDR names, keywords and shortcodes in the
// app's languages, a category tab bar, a virtualized grid of emoji rows, and
// optional skin tone and preview parts. All default UI words and category
// labels come from base/locales. Emoji data is loaded lazily on first render
// so apps that never use EmojiPicker link none of it.
type EmojiPicker struct {
	// Query makes the search query controlled, nil for internal state seeded
	// from DefaultQuery.
	Query         func() *string
	DefaultQuery  string
	OnQueryChange func(query string)
	// NoSearch hides the search field.
	NoSearch bool
	// Placeholder and SearchLabel override base/locales defaults for the
	// search field.
	Placeholder     string
	PlaceholderFunc func() string
	SearchLabel     string
	SearchLabelFunc func() string
	AutoFocus       bool
	InputSource     string
	KeepInputSource bool

	// Category makes the active category controlled, nil for internal state
	// seeded from DefaultCategory.
	Category         func() EmojiCategory
	DefaultCategory  EmojiCategory
	OnCategoryChange func(EmojiCategory)
	// Categories restricts and orders the Unicode categories shown; nil shows
	// all nine standard categories (plus EmojiRecent first when Recent is
	// non-empty).
	Categories []EmojiCategory
	// NoCategories hides the category tab bar.
	NoCategories bool

	// SkinTone makes the active skin tone controlled, nil for internal state
	// seeded from DefaultSkinTone.
	SkinTone         func() EmojiSkinTone
	DefaultSkinTone  EmojiSkinTone
	OnSkinToneChange func(EmojiSkinTone)
	// NoSkinTone hides the skin tone selector bar.
	NoSkinTone bool

	// Recent supplies recently used emoji strings (e.g. "🔥", "👍"). When
	// non-empty and no search query is active, they appear first under
	// EmojiRecent.
	Recent func() []string

	// Languages explicitly chooses the BCP 47 languages searched for emoji
	// names and keywords (with English fallback). When empty, the picker
	// follows the app's language (i18n.Language) and locale (Env.Locale).
	Languages     []string
	LanguagesFunc func() []string

	// Columns is how many emoji sit in each virtualized grid row; 8 when <= 0.
	Columns int

	Disabled          func() bool
	Invalid           func() bool
	OnSelect          func(Emoji)
	OnHighlightChange func(*Emoji)
	// OnCancel runs on Escape when the search query is empty.
	OnCancel func()
	Ref      *EmojiPickerRef

	Class              string
	Style              core.Style
	StyleFunc          func() core.Style
	SearchBarClass     string
	SearchBarStyle     core.Style
	SearchBarStyleFunc func() core.Style
	SearchClass        string
	SearchStyle        core.Style
	SearchStyleFunc    func() core.Style
	// SearchField wraps or styles the search core.Input.
	SearchField          func(core.Input) core.Node
	CategoriesClass      string
	CategoriesStyle      core.Style
	CategoriesStyleFunc  func() core.Style
	CategoryClass        string
	CategoryStyle        core.Style
	CategoryStyleFunc    func() core.Style
	ListClass            string
	ListStyle            core.Style
	ListStyleFunc        func() core.Style
	SectionClass         string
	SectionStyle         core.Style
	SectionStyleFunc     func() core.Style
	RowClass             string
	RowStyle             core.Style
	RowStyleFunc         func() core.Style
	ItemClass            string
	ItemStyle            core.Style
	ItemStyleFunc        func() core.Style
	EmptyClass           string
	EmptyStyle           core.Style
	EmptyStyleFunc       func() core.Style
	SkinToneBarClass     string
	SkinToneBarStyle     core.Style
	SkinToneBarStyleFunc func() core.Style
	SkinToneClass        string
	SkinToneStyle        core.Style
	SkinToneStyleFunc    func() core.Style
	PreviewClass         string
	PreviewStyle         core.Style
	PreviewStyleFunc     func() core.Style
	FooterBarClass       string
	FooterBarStyle       core.Style
	FooterBarStyleFunc   func() core.Style

	Header, Footer, Empty      core.Node
	SearchPrefix, SearchSuffix core.Node
	RenderItem                 func(Emoji) core.Node
	RenderSection              func(category EmojiCategory, label string) core.Node
	RenderCategory             func(category EmojiCategory, label string) core.Node
	RenderSkinTone             func(tone EmojiSkinTone, label string) core.Node
	RenderPreview              func(emoji *Emoji) core.Node

	Label     string
	LabelFunc func() string
	TestID    string
	// OnLayout and Tooltip are its root view's, as a core.View's.
	OnLayout func(core.Bounds)
	Tooltip  core.Node
}

type emojiRowKind uint8

const (
	emojiRowSection emojiRowKind = iota
	emojiRowCells
)

type emojiGridRow struct {
	kind       emojiRowKind
	category   EmojiCategory
	firstFlat  int
	items      []Emoji
	rowInGrid  int
	sectionRow int
}

type emojiShown struct {
	rows         []emojiGridRow
	keys         []any
	flat         []Emoji
	flatRow      []int // flat emoji index -> list row index
	flatCol      []int // flat emoji index -> column within its row
	cellRows     []int // list row indices that hold emoji cells
	sectionRow   map[EmojiCategory]int
	sectionFirst map[EmojiCategory]int
	categories   []EmojiCategory
	searching    bool
}

type emojiSectionKey struct{ cat EmojiCategory }
type emojiCellsKey struct {
	searching bool
	cat       EmojiCategory
	row       int
}

const emojiLitBuckets = 32

type emojiState struct {
	cx          *core.Scope
	p           EmojiPicker
	shown       *core.Memo[*emojiShown]
	highlight   *core.Signal[int]
	lit         [emojiLitBuckets]*core.Signal[int]
	hoveredItem *core.Signal[*Emoji]
	list        *core.Ref
	first, last int
	known       bool
	pointer     *core.Point
}

func (s *emojiState) setHighlight(idx int, scroll bool) {
	sh := s.shown.GetUntracked()
	if len(sh.flat) == 0 {
		idx = -1
	} else {
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sh.flat) {
			idx = len(sh.flat) - 1
		}
	}
	prev := s.highlight.GetUntracked()
	if prev == idx {
		s.updatePreview(sh, idx)
		if scroll && idx >= 0 {
			s.reveal(idx)
		}
		return
	}
	s.highlight.Set(idx)
	if prev >= 0 {
		s.lit[prev%emojiLitBuckets].Update(func(n int) int { return n + 1 })
	}
	if idx >= 0 {
		s.lit[idx%emojiLitBuckets].Update(func(n int) int { return n + 1 })
		if scroll {
			s.reveal(idx)
		}
	}
	s.updatePreview(sh, idx)
}

func (s *emojiState) updatePreview(sh *emojiShown, idx int) {
	var next *Emoji
	if idx >= 0 && idx < len(sh.flat) {
		e := sh.flat[idx]
		next = &e
	}
	prev := s.hoveredItem.GetUntracked()
	if prev == nil && next == nil {
		return
	}
	if prev != nil && next != nil && prev.Emoji == next.Emoji && prev.Name == next.Name && prev.Category == next.Category &&
		prev.HasSkinTones == next.HasSkinTones && slices.Equal(prev.Keywords, next.Keywords) && slices.Equal(prev.Skins, next.Skins) {
		return
	}
	s.hoveredItem.Set(next)
	if s.p.OnHighlightChange != nil {
		if next == nil {
			s.p.OnHighlightChange(nil)
		} else {
			copy := *next
			s.p.OnHighlightChange(&copy)
		}
	}
}

func (s *emojiState) isHighlighted(idx int) bool {
	if idx < 0 {
		return false
	}
	s.lit[idx%emojiLitBuckets].Get()
	return s.highlight.GetUntracked() == idx
}

func (s *emojiState) reveal(flatIdx int) {
	sh := s.shown.GetUntracked()
	if flatIdx < 0 || flatIdx >= len(sh.flatRow) {
		return
	}
	row := sh.flatRow[flatIdx]
	if !s.known {
		s.list.ScrollToIndexAt(row, 0, 0, false)
		return
	}
	if row < s.first {
		s.list.ScrollToIndexAt(row, 0, 0, false)
	} else if row > s.last {
		s.list.ScrollToIndexAt(row, 1, 0, false)
	}
}

func (s *emojiState) moveHoriz(delta int) {
	sh := s.shown.GetUntracked()
	if len(sh.flat) == 0 {
		return
	}
	cur := s.highlight.GetUntracked()
	if cur < 0 {
		s.setHighlight(0, true)
		return
	}
	next := cur + delta
	if next < 0 {
		next = 0
	} else if next >= len(sh.flat) {
		next = len(sh.flat) - 1
	}
	s.setHighlight(next, true)
}

func (s *emojiState) moveVert(rowDelta int) {
	sh := s.shown.GetUntracked()
	if len(sh.flat) == 0 || len(sh.cellRows) == 0 {
		return
	}
	cur := s.highlight.GetUntracked()
	if cur < 0 {
		s.setHighlight(0, true)
		return
	}
	curRow := sh.flatRow[cur]
	curCol := sh.flatCol[cur]
	pos := -1
	for i, r := range sh.cellRows {
		if r == curRow {
			pos = i
			break
		}
	}
	if pos < 0 {
		s.setHighlight(0, true)
		return
	}
	targetPos := pos + rowDelta
	if targetPos < 0 {
		targetPos = 0
	} else if targetPos >= len(sh.cellRows) {
		targetPos = len(sh.cellRows) - 1
	}
	targetRow := sh.rows[sh.cellRows[targetPos]]
	col := min(curCol, len(targetRow.items)-1)
	s.setHighlight(targetRow.firstFlat+col, true)
}

func (s *emojiState) choose(flatIdx int) {
	if is(s.p.Disabled) {
		return
	}
	sh := s.shown.GetUntracked()
	if flatIdx < 0 || flatIdx >= len(sh.flat) {
		return
	}
	if s.p.OnSelect != nil {
		s.p.OnSelect(sh.flat[flatIdx])
	}
}

func (s *emojiState) accept() {
	if idx := s.highlight.GetUntracked(); idx >= 0 {
		s.choose(idx)
	}
}

// Render implements core.Node.
func (p EmojiPicker) Render(cx *core.Scope) core.Node {
	cols := p.Columns
	if cols <= 0 {
		cols = 8
	}

	ownQuery := core.NewSignal(cx, p.DefaultQuery)
	query := func() string {
		if p.Query != nil {
			if q := p.Query(); q != nil {
				return *q
			}
		}
		return ownQuery.Get()
	}
	setQuery := func(text string) {
		if p.Query == nil || p.Query() == nil {
			ownQuery.Set(text)
		}
		if p.OnQueryChange != nil {
			p.OnQueryChange(text)
		}
	}

	ownCategory := core.NewSignal(cx, p.DefaultCategory)
	activeCategory := func() EmojiCategory {
		if p.Category != nil {
			return p.Category()
		}
		return ownCategory.Get()
	}
	setCategoryVal := func(c EmojiCategory) {
		if p.Category == nil {
			ownCategory.Set(c)
		}
		if p.OnCategoryChange != nil {
			p.OnCategoryChange(c)
		}
	}

	ownSkin := core.NewSignal(cx, p.DefaultSkinTone)
	skinTone := func() EmojiSkinTone {
		if p.SkinTone != nil {
			return p.SkinTone()
		}
		return ownSkin.Get()
	}
	setSkinTone := func(t EmojiSkinTone) {
		if is(p.Disabled) {
			return
		}
		if p.SkinTone == nil {
			ownSkin.Set(t)
		}
		if p.OnSkinToneChange != nil {
			p.OnSkinToneChange(t)
		}
	}

	searchLanguages := func() []string {
		if p.LanguagesFunc != nil {
			if l := p.LanguagesFunc(); len(l) > 0 {
				return l
			}
		}
		if len(p.Languages) > 0 {
			return p.Languages
		}
		var out []string
		if lang := i18n.Language(cx); lang != "" {
			out = append(out, lang)
		}
		if loc := cx.Env().Locale(); loc != "" {
			out = append(out, loc)
		}
		return out
	}

	st := &emojiState{
		cx:          cx,
		p:           p,
		highlight:   core.NewSignal(cx, -1),
		hoveredItem: core.NewSignal[*Emoji](cx, nil),
		list:        new(core.Ref),
	}
	for i := range st.lit {
		st.lit[i] = core.NewSignal(cx, 0)
	}

	st.shown = core.NewMemoFunc(cx, func() *emojiShown {
		langs := searchLanguages()
		tone := skinTone()
		q := ""
		if !p.NoSearch {
			q = strings.TrimSpace(query())
		}
		_, directEmoji := emoji.ByString(q, langs)

		// Determine categories list (including EmojiRecent when Recent is non-empty).
		baseCats := p.Categories
		if len(baseCats) == 0 {
			baseCats = EmojiCategories()
		}
		var recentItems []Emoji
		if p.Recent != nil {
			seen := map[string]bool{}
			for _, raw := range p.Recent() {
				if it, ok := emoji.ByString(raw, langs); ok && !seen[it.Emoji] {
					seen[it.Emoji] = true
					recentItems = append(recentItems, emojiFromItem(it))
				}
			}
		}
		cats := make([]EmojiCategory, 0, len(baseCats)+1)
		if len(recentItems) > 0 {
			cats = append(cats, EmojiRecent)
		}
		for _, c := range baseCats {
			if c != EmojiRecent {
				cats = append(cats, c)
			}
		}

		sh := &emojiShown{
			sectionRow:   map[EmojiCategory]int{},
			sectionFirst: map[EmojiCategory]int{},
			categories:   cats,
			searching:    q != "",
		}

		appendItems := func(cat EmojiCategory, items []Emoji, withHeader bool) {
			if len(items) == 0 {
				return
			}
			secRow := len(sh.rows)
			if withHeader {
				sh.sectionRow[cat] = secRow
				sh.sectionFirst[cat] = len(sh.flat)
				sh.rows = append(sh.rows, emojiGridRow{
					kind:       emojiRowSection,
					category:   cat,
					firstFlat:  len(sh.flat),
					sectionRow: secRow,
				})
				sh.keys = append(sh.keys, emojiSectionKey{cat})
			}
			rowNum := 0
			for start := 0; start < len(items); start += cols {
				end := min(start+cols, len(items))
				chunk := make([]Emoji, end-start)
				firstFlat := len(sh.flat)
				rowIdx := len(sh.rows)
				for j, e := range items[start:end] {
					if cat != EmojiRecent && !directEmoji {
						e = e.WithSkinTone(tone)
					}
					chunk[j] = e
					sh.flat = append(sh.flat, e)
					sh.flatRow = append(sh.flatRow, rowIdx)
					sh.flatCol = append(sh.flatCol, j)
				}
				sh.rows = append(sh.rows, emojiGridRow{
					kind:       emojiRowCells,
					category:   cat,
					firstFlat:  firstFlat,
					items:      chunk,
					rowInGrid:  rowNum,
					sectionRow: secRow,
				})
				sh.keys = append(sh.keys, emojiCellsKey{searching: sh.searching, cat: cat, row: rowNum})
				sh.cellRows = append(sh.cellRows, rowIdx)
				rowNum++
			}
		}

		if q != "" {
			allowed := map[EmojiCategory]bool{}
			for _, c := range baseCats {
				allowed[c] = true
			}
			matches := emoji.Search(q, langs)
			filtered := make([]Emoji, 0, len(matches))
			for _, it := range matches {
				e := emojiFromItem(it)
				if allowed[e.Category] {
					filtered = append(filtered, e)
				}
			}
			appendItems(EmojiSmileys, filtered, false)
			return sh
		}

		// Browse mode: group emojis by category in Unicode order.
		if len(recentItems) > 0 {
			appendItems(EmojiRecent, recentItems, true)
		}
		all := emoji.Search("", langs)
		byCat := map[EmojiCategory][]Emoji{}
		for _, it := range all {
			e := emojiFromItem(it)
			byCat[e.Category] = append(byCat[e.Category], e)
		}
		for _, c := range baseCats {
			if c == EmojiRecent {
				continue
			}
			appendItems(c, byCat[c], true)
		}
		return sh
	}, func(a, b *emojiShown) bool { return a == b })

	// Reset or clamp highlight when shown rows change.
	var lastQuery *string
	var pendingCat *EmojiCategory
	var lastCategory *EmojiCategory
	if p.DefaultCategory != EmojiSmileys {
		c := p.DefaultCategory
		pendingCat = &c
	}
	cx.Effect(func() {
		q := ""
		if !p.NoSearch {
			q = query()
		}
		sh := st.shown.Get()
		if p.Category != nil {
			c := p.Category()
			clearedQuery := !sh.searching && lastQuery != nil && strings.TrimSpace(*lastQuery) != ""
			if lastCategory == nil || *lastCategory != c || clearedQuery {
				pendingCat = &c
				lastCategory = &c
			}
		}
		cx.Untrack(func() {
			reset := lastQuery == nil || *lastQuery != q
			lastQuery = &q
			if pendingCat != nil && !sh.searching {
				c := *pendingCat
				pendingCat = nil
				if row, ok := sh.sectionRow[c]; ok {
					st.list.ScrollToIndexAt(row, 0, 0, false)
				}
				if first, ok := sh.sectionFirst[c]; ok {
					st.setHighlight(first, false)
					return
				}
			}
			if len(sh.flat) == 0 {
				st.setHighlight(-1, false)
			} else if reset || st.highlight.GetUntracked() < 0 || st.highlight.GetUntracked() >= len(sh.flat) {
				st.setHighlight(0, false)
			} else {
				st.updatePreview(sh, st.highlight.GetUntracked())
			}
		})
	})

	jumpToCategory := func(c EmojiCategory) {
		if is(p.Disabled) {
			return
		}
		setCategoryVal(c)
		if !p.NoSearch && core.Untracked(cx, query) != "" {
			cc := c
			pendingCat = &cc
			setQuery("")
			return
		}
		sh := core.Untracked(cx, st.shown.Get)
		if row, ok := sh.sectionRow[c]; ok {
			st.list.ScrollToIndexAt(row, 0, 0, false)
		}
		if first, ok := sh.sectionFirst[c]; ok {
			st.setHighlight(first, false)
		}
	}

	root, field := new(core.Ref), new(core.Ref)
	fieldFocused := core.NewSignal(cx, false)
	rootFocused := core.NewSignal(cx, false)
	composing := core.NewSignal(cx, false)

	keysActive := func() bool {
		if is(p.Disabled) {
			return false
		}
		if p.NoSearch {
			return rootFocused.Get()
		}
		return (fieldFocused.Get() && !composing.Get()) || rootFocused.Get()
	}

	keys := core.Show{When: keysActive, Then: func(cx *core.Scope) core.Node {
		when := core.KeyAlways
		if p.NoSearch || !core.Untracked(cx, fieldFocused.Get) {
			when = core.KeyNotEditing
		}
		bind := func(key string, w core.KeyWhen, repeat bool, f func()) {
			cx.OnKeyWith(key, core.KeyOptions{When: w, Repeat: repeat}, f)
		}
		bind("up", when, true, func() { st.moveVert(-1) })
		bind("down", when, true, func() { st.moveVert(1) })
		bind("pageup", when, true, func() { st.moveVert(-4) })
		bind("pagedown", when, true, func() { st.moveVert(4) })
		bind("enter", when, false, st.accept)
		bind("home", core.KeyNotEditing, false, func() { st.setHighlight(0, true) })
		bind("end", core.KeyNotEditing, false, func() {
			if n := len(st.shown.GetUntracked().flat); n > 0 {
				st.setHighlight(n-1, true)
			}
		})
		bind("space", core.KeyNotEditing, false, st.accept)
		bind("escape", core.KeyAlways, false, func() {
			if !p.NoSearch && core.Untracked(cx, query) != "" {
				setQuery("")
			} else if p.OnCancel != nil {
				p.OnCancel()
			}
		})
		// Left/right move across the grid when not editing text.
		bind("left", core.KeyNotEditing, true, func() {
			delta := -1
			if core.Untracked(cx, cx.Env().RTL) {
				delta = 1
			}
			st.moveHoriz(delta)
		})
		bind("right", core.KeyNotEditing, true, func() {
			delta := 1
			if core.Untracked(cx, cx.Env().RTL) {
				delta = -1
			}
			st.moveHoriz(delta)
		})
		return nil
	}}

	// Bind left/right with KeyAlways while search field is focused and empty.
	emptyFieldNav := core.Show{When: func() bool {
		return !p.NoSearch && !is(p.Disabled) && fieldFocused.Get() && !composing.Get() && query() == ""
	}, Then: func(cx *core.Scope) core.Node {
		cx.OnKeyWith("left", core.KeyOptions{When: core.KeyAlways, Repeat: true}, func() {
			delta := -1
			if core.Untracked(cx, cx.Env().RTL) {
				delta = 1
			}
			st.moveHoriz(delta)
		})
		cx.OnKeyWith("right", core.KeyOptions{When: core.KeyAlways, Repeat: true}, func() {
			delta := 1
			if core.Untracked(cx, cx.Env().RTL) {
				delta = -1
			}
			st.moveHoriz(delta)
		})
		return nil
	}}

	if p.Ref != nil {
		p.Ref.query = query
		p.Ref.setQuery = func(text string) {
			if !is(p.Disabled) {
				setQuery(text)
			}
		}
		p.Ref.focus = func() {
			if p.NoSearch {
				root.Focus()
			} else {
				field.Focus()
			}
		}
		p.Ref.setCategory = jumpToCategory
		p.Ref.count = func() int { return len(st.shown.GetUntracked().flat) }
		cx.Cleanup(func() { *p.Ref = EmojiPickerRef{} })
	}

	var children []core.Node
	if p.Header != nil {
		children = append(children, p.Header)
	}

	// Search bar.
	if !p.NoSearch {
		var bar []core.Node
		if p.SearchPrefix != nil {
			bar = append(bar, p.SearchPrefix)
		}
		searchInput := core.Input{
			Ref:             field,
			Value:           query,
			AutoFocus:       p.AutoFocus,
			Disabled:        p.Disabled,
			Invalid:         p.Invalid,
			SubmitBehavior:  core.SubmitBehaviorSubmit,
			Class:           p.SearchClass,
			Style:           p.SearchStyle,
			StyleFunc:       p.SearchStyleFunc,
			InputSource:     p.InputSource,
			KeepInputSource: p.KeepInputSource,
			PlaceholderFunc: func() string {
				if p.PlaceholderFunc != nil {
					return p.PlaceholderFunc()
				}
				if p.Placeholder != "" {
					return p.Placeholder
				}
				return locales.EmojiSearchPlaceholder(cx)
			},
			LabelFunc: func() string {
				if p.SearchLabelFunc != nil {
					return p.SearchLabelFunc()
				}
				if p.SearchLabel != "" {
					return p.SearchLabel
				}
				return locales.EmojiSearch(cx)
			},
			OnFocus:             func() { fieldFocused.Set(true) },
			OnBlur:              func() { fieldFocused.Set(false) },
			OnCompositionChange: func(e core.Composition) { composing.Set(e.Composing) },
			OnChange:            setQuery,
			OnSubmit:            func(string) { st.accept() },
		}
		bar = append(bar, drawnField(p.SearchField, searchInput))
		if p.SearchSuffix != nil {
			bar = append(bar, p.SearchSuffix)
		}
		children = append(children, core.View{
			Class:     p.SearchBarClass,
			Style:     p.SearchBarStyle,
			StyleFunc: p.SearchBarStyleFunc,
			Children:  bar,
		})
	}

	// Category tab bar.
	if !p.NoCategories {
		categories := core.NewMemoFunc(cx, func() []EmojiCategory {
			return st.shown.Get().categories
		}, slices.Equal[[]EmojiCategory])
		children = append(children, Tabs{
			LabelFunc: func() string { return locales.EmojiCategories(cx) },
			Class:     p.CategoriesClass,
			Style:     core.Style{Direction: core.Row}.Merge(p.CategoriesStyle),
			StyleFunc: p.CategoriesStyleFunc,
			Children: []core.Node{
				core.Dynamic(func(cx *core.Scope) core.Node {
					cats := categories.Get()
					tabs := make([]core.Node, len(cats))
					for i, cat := range cats {
						c := cat
						var face core.Node
						if p.RenderCategory != nil {
							face = core.Dynamic(func(cx *core.Scope) core.Node {
								return p.RenderCategory(c, c.Label(cx))
							})
						} else {
							face = core.Text{Content: c.IconEmoji()}
						}
						tabs[i] = Tab{
							Disabled:  p.Disabled,
							Selected:  func() bool { return !st.shown.Get().searching && activeCategory() == c },
							LabelFunc: func() string { return c.Label(cx) },
							Class:     p.CategoryClass,
							Style:     p.CategoryStyle,
							StyleFunc: p.CategoryStyleFunc,
							OnPress:   func(core.PressEvent) { jumpToCategory(c) },
							Children:  []core.Node{face},
						}
					}
					return core.View{
						Style:    core.Style{Direction: core.Row, FlexGrow: 1, AlignItems: core.AlignCenter},
						Children: tabs,
					}
				}),
			},
		})
	}

	// Virtualized grid list.
	list := core.List{
		Ref:           st.list,
		EstimatedSize: 36,
		Count:         func() int { return len(st.shown.Get().rows) },
		Key: func(i int) any {
			if keys := st.shown.GetUntracked().keys; i < len(keys) {
				return keys[i]
			}
			return i
		},
		Row: func(cx *core.Scope, r core.ListRow) core.Node {
			return st.renderRow(cx, r, cols)
		},
		Class: p.ListClass,
		Style: core.Style{FlexGrow: 1, MinHeight: core.Pt(0)}.Merge(p.ListStyle),
		StyleFunc: func() core.Style {
			var s core.Style
			if p.ListStyleFunc != nil {
				s = p.ListStyleFunc()
			}
			if len(st.shown.Get().rows) == 0 {
				s.Display = core.DisplayNone
			}
			return s
		},
		ViewabilityConfig: core.ViewabilityConfig{ItemVisiblePercentThreshold: 1},
		OnViewableItemsChange: func(v core.ViewableItems) {
			st.known = len(v.Viewable) > 0
			if !st.known {
				return
			}
			st.first, st.last = v.Viewable[0].Index, v.Viewable[len(v.Viewable)-1].Index
			sh := st.shown.GetUntracked()
			if !sh.searching && st.first >= 0 && st.first < len(sh.rows) {
				topCat := sh.rows[st.first].category
				if activeCategory() != topCat {
					setCategoryVal(topCat)
					// An app adopting a scroll report is not asking us to jump
					// back to the group's header while the native scroll glides.
					if p.Category != nil && p.Category() == topCat {
						lastCategory = &topCat
					}
				}
			}
		},
	}
	children = append(children, list)

	// Empty state when search finds no emoji.
	emptyNode := p.Empty
	if emptyNode == nil {
		emptyNode = core.View{
			Class:     p.EmptyClass,
			Style:     p.EmptyStyle,
			StyleFunc: p.EmptyStyleFunc,
			Children: []core.Node{
				core.Text{
					LiveRegion:  core.LiveRegionPolite,
					ContentFunc: func() string { return locales.EmojiNoMatches(cx) },
				},
			},
		}
	}
	children = append(children, core.Show{
		When: func() bool { return len(st.shown.Get().rows) == 0 },
		Then: func(*core.Scope) core.Node { return emptyNode },
	})

	// Optional preview + skin tone footer bar.
	var footerBar []core.Node
	if p.RenderPreview != nil {
		footerBar = append(footerBar, core.View{
			Class:     p.PreviewClass,
			Style:     p.PreviewStyle,
			StyleFunc: p.PreviewStyleFunc,
			Children: []core.Node{
				core.Dynamic(func(*core.Scope) core.Node {
					return p.RenderPreview(st.hoveredItem.Get())
				}),
			},
		})
	}
	if !p.NoSkinTone {
		tones := EmojiSkinTones()
		swatches := make([]core.Node, len(tones))
		for i, t := range tones {
			tone := t
			var content core.Node
			if p.RenderSkinTone != nil {
				content = core.Dynamic(func(cx *core.Scope) core.Node {
					return p.RenderSkinTone(tone, tone.Label(cx))
				})
			} else {
				content = core.Text{Content: Emoji{Emoji: "✋", HasSkinTones: true, Skins: skinToneHandSamples}.WithSkinTone(tone).Emoji}
			}
			checked := func() bool { return skinTone() == tone }
			check := func() {
				if !checked() {
					setSkinTone(tone)
				}
			}
			swatches[i] = core.Func(func(cx *core.Scope) core.Node {
				return rovingView(cx, core.View{
					Role:      core.RoleRadio,
					Disabled:  p.Disabled,
					Checked:   checked,
					Selected:  checked,
					LabelFunc: func() string { return tone.Label(cx) },
					Class:     p.SkinToneClass,
					Style:     p.SkinToneStyle,
					StyleFunc: p.SkinToneStyleFunc,
					OnPress:   func(core.PressEvent) { check() },
					Children:  []core.Node{content},
				}, checked, check)
			})
		}
		footerBar = append(footerBar, core.Func(func(cx *core.Scope) core.Node {
			group := newRoving(cx, false)
			return core.View{
				Role:      core.RoleRadioGroup,
				LabelFunc: func() string { return locales.EmojiSkinTone(cx) },
				Class:     p.SkinToneBarClass,
				Style:     core.Style{Direction: core.Row}.Merge(p.SkinToneBarStyle),
				StyleFunc: p.SkinToneBarStyleFunc,
				Children:  append(swatches, group.keys()),
			}
		}))
	}
	if len(footerBar) > 0 {
		children = append(children, core.View{
			Class:     p.FooterBarClass,
			Style:     core.Style{Direction: core.Row, AlignItems: core.AlignCenter}.Merge(p.FooterBarStyle),
			StyleFunc: p.FooterBarStyleFunc,
			Children:  footerBar,
		})
	}

	if p.Footer != nil {
		children = append(children, p.Footer)
	}
	children = append(children, keys, emptyFieldNav)

	return core.View{
		Ref:  root,
		Role: core.RoleGrid,
		LabelFunc: func() string {
			if p.LabelFunc != nil {
				return p.LabelFunc()
			}
			if p.Label != "" {
				return p.Label
			}
			return locales.EmojiPicker(cx)
		},
		Disabled:  p.Disabled,
		Invalid:   p.Invalid,
		Focusable: func() bool { return p.NoSearch && !is(p.Disabled) },
		OnFocus:   func() { rootFocused.Set(true) },
		OnBlur:    func() { rootFocused.Set(false) },
		Class:     p.Class,
		Style:     p.Style,
		StyleFunc: p.StyleFunc,
		TestID:    p.TestID,
		OnLayout:  p.OnLayout,
		Tooltip:   p.Tooltip,
		Children:  children,
	}
}

var skinToneHandSamples = []string{"✋🏻", "✋🏼", "✋🏽", "✋🏾", "✋🏿"}

func (s *emojiState) renderRow(cx *core.Scope, r core.ListRow, cols int) core.Node {
	p := s.p
	rowMemo := core.NewMemoFunc(cx, func() emojiGridRow {
		rows := s.shown.Get().rows
		if i := r.Index(); i >= 0 && i < len(rows) {
			return rows[i]
		}
		return emojiGridRow{}
	}, func(a, b emojiGridRow) bool { return false })
	kind := core.NewMemo(cx, func() emojiRowKind { return rowMemo.Get().kind })

	return core.Dynamic(func(cx *core.Scope) core.Node {
		if kind.Get() == emojiRowSection {
			var content core.Node
			if p.RenderSection != nil {
				content = core.Dynamic(func(cx *core.Scope) core.Node {
					row := rowMemo.Get()
					return p.RenderSection(row.category, row.category.Label(cx))
				})
			} else {
				content = core.Text{ContentFunc: func() string {
					return rowMemo.Get().category.Label(cx)
				}}
			}
			return core.View{
				Role:      core.RoleHeader,
				NoFocus:   true,
				Class:     p.SectionClass,
				Style:     p.SectionStyle,
				StyleFunc: p.SectionStyleFunc,
				Children:  []core.Node{content},
			}
		}

		cells := make([]core.Node, cols)
		for col := range cols {
			c := col
			cells[c] = core.Dynamic(func(cx *core.Scope) core.Node {
				row := rowMemo.Get()
				if row.kind != emojiRowCells || c >= len(row.items) {
					return core.View{
						NoFocus:                     true,
						AccessibilityElementsHidden: func() bool { return true },
						Style:                       core.Style{FlexGrow: 1, FlexBasis: core.Pt(0)},
					}
				}
				e := row.items[c]
				flatIdx := row.firstFlat + c
				var face core.Node
				if p.RenderItem != nil {
					face = p.RenderItem(e)
				} else {
					face = core.Text{Content: e.Emoji}
				}
				return core.View{
					Role:     core.RoleButton,
					NoFocus:  true,
					Label:    e.Name,
					Disabled: p.Disabled,
					Selected: func() bool { return s.isHighlighted(flatIdx) },
					PartStates: func() core.PartStates {
						return core.PartStates{Focused: s.isHighlighted(flatIdx)}
					},
					Class:     p.ItemClass,
					Style:     core.Style{FlexGrow: 1, FlexBasis: core.Pt(0)}.Merge(p.ItemStyle),
					StyleFunc: p.ItemStyleFunc,
					OnPress:   func(core.PressEvent) { s.choose(flatIdx) },
					OnMouseMove: func(ev core.MouseEvent) {
						if ev.PressedButton != core.MouseButtonNone || is(p.Disabled) {
							return
						}
						if s.pointer != nil && *s.pointer == ev.PagePosition {
							return
						}
						at := ev.PagePosition
						s.pointer = &at
						s.setHighlight(flatIdx, false)
					},
					Children: []core.Node{face},
				}
			})
		}
		return core.View{
			Role:      core.RoleRow,
			Class:     p.RowClass,
			Style:     core.Style{Direction: core.Row}.Merge(p.RowStyle),
			StyleFunc: p.RowStyleFunc,
			Children:  cells,
		}
	})
}
