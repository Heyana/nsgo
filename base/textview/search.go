package textview

import (
	"cmp"
	"slices"

	"github.com/nsgo-dev/nsgo/core"
)

// Search is one search across the text views that join it: its query and options, whether it is open, and its matches in reading order with the one selected. A find bar owns one and shows its count; the views highlight its matches and reveal the selected one.
type Search struct {
	open    *core.Signal[bool]
	query   *core.Signal[string]
	options *core.Signal[FindOptions]
	// finder is the query's, nil while the search is closed or the query is empty.
	finder *core.Memo[*finder]
	views  *core.Signal[[]*searchView]
	// all are the views' matches in tree order.
	all *core.Memo[searchMatches]
	// sel is the selected match, which at keeps while the matches change and the search is closed; moved changes as Next and Previous move it.
	sel    *core.Memo[place]
	at     place
	moved  *core.Signal[int]
	margin core.Edges
}

// searchMatches are a search's matches, with each view's place in tree order.
type searchMatches struct {
	hits []hit
	rank map[*searchView]int
}

// hit is a match in a text of a view.
type hit struct {
	view  *searchView
	text  *searchText
	match Match
}

// place is where a selected match is: its view, its text's key and its range, and its index among all matches.
type place struct {
	set      bool
	view     *searchView
	key      string
	from, to int
	index    int
}

var searchKey = core.NewKey[*Search]("textview search", nil)

// NewSearch returns a closed search with no query, owned by cx.
func NewSearch(cx *core.Scope) *Search {
	s := &Search{open: core.NewSignal(cx, false), query: core.NewSignal(cx, ""), options: core.NewSignal(cx, FindOptions{}),
		views: core.NewSignalFunc[[]*searchView](cx, nil, nil), moved: core.NewSignal(cx, 0)}
	s.finder = core.NewMemoFunc(cx, func() *finder {
		if !s.open.Get() {
			return nil
		}
		return newFinder(s.query.Get(), s.options.Get())
	}, nil)
	s.all = core.NewMemoFunc(cx, func() searchMatches {
		views := slices.Clone(s.views.Get())
		slices.SortStableFunc(views, func(a, b *searchView) int {
			switch {
			case a.root.Before(b.root):
				return -1
			case b.root.Before(a.root):
				return 1
			}
			return 0
		})
		m := searchMatches{rank: make(map[*searchView]int, len(views))}
		for i, v := range views {
			m.rank[v] = i
			m.hits = append(m.hits, v.matches.Get().hits...)
		}
		return m
	}, nil)
	s.sel = core.NewMemo(cx, func() place {
		s.moved.Get()
		m := s.all.Get()
		if len(m.hits) == 0 {
			return place{}
		}
		s.at = m.place(s.locate(m))
		return s.at
	})
	// The view showing a newly selected match reveals it.
	var revealed place
	cx.Effect(func() {
		p := s.sel.Get()
		shown := p
		shown.index = 0
		if shown == revealed {
			return
		}
		revealed = shown
		if p.set {
			s.all.GetUntracked().hits[p.index].text.reveal(p.from, p.to, s.margin)
		}
	})
	return s
}

// ProvideSearch has every TextView below cx's owner join s, unless its Search field names another.
func ProvideSearch(cx *core.Scope, s *Search) { searchKey.Provide(cx, s) }

// Open opens the search: the views find and highlight its query again.
func (s *Search) Open() { s.open.Set(true) }

// Close closes the search: the views show no matches and Count is zero, while the query, the options and the selected match stay for when it opens again.
func (s *Search) Close() { s.open.Set(false) }

// IsOpen reports, tracked, whether the search is open.
func (s *Search) IsOpen() bool { return s.open.Get() }

// SetQuery sets what the search finds; an empty query finds nothing.
func (s *Search) SetQuery(q string) { s.query.Set(q) }

// Query returns the query, tracked.
func (s *Search) Query() string { return s.query.Get() }

// SetOptions sets how the search matches.
func (s *Search) SetOptions(o FindOptions) { s.options.Set(o) }

// Count returns, tracked, how many matches the open search has across its views, each view's capped.
func (s *Search) Count() int { return len(s.all.Get().hits) }

// Selected returns, tracked, the selected match's place among them, one-based, and zero when there is none.
func (s *Search) Selected() int {
	p := s.sel.Get()
	if !p.set {
		return 0
	}
	return p.index + 1
}

// SelectedMatch returns, tracked, the selected match, and false when there is none.
func (s *Search) SelectedMatch() (Match, bool) {
	p := s.sel.Get()
	if !p.set {
		return Match{}, false
	}
	m := s.all.Get().hits[p.index].match
	m.Text.Path = slices.Clone(m.Text.Path)
	return m, true
}

// Next selects the match after the selected one, the first after the last.
func (s *Search) Next() { s.step(1) }

// Previous selects the match before the selected one, the last before the first.
func (s *Search) Previous() { s.step(-1) }

func (s *Search) step(by int) {
	p := s.sel.GetUntracked()
	m := s.all.GetUntracked()
	if !p.set {
		return
	}
	n := len(m.hits)
	s.at = m.place(((p.index+by)%n + n) % n)
	s.moved.Set(s.moved.GetUntracked() + 1)
}

// SelectedBounds returns the selected match's first box in surface points after the last commit, from its text's line boxes rather than its block's frame, and false while there is none or its text is not laid out. It is not tracked.
func (s *Search) SelectedBounds() (core.Bounds, bool) {
	p := s.sel.GetUntracked()
	if !p.set {
		return core.Bounds{}, false
	}
	ref := s.all.GetUntracked().hits[p.index].text.ref
	frame, ok := ref.Bounds()
	boxes := ref.TextRangeBoxes(p.from, p.to)
	if !ok || len(boxes) == 0 {
		return core.Bounds{}, false
	}
	b := boxes[0]
	b.Origin.X += frame.Origin.X
	b.Origin.Y += frame.Origin.Y
	return b, true
}

// SetRevealMargin sets the margin a view keeps clear at the outermost scroll view's edges as it reveals the selected match, for the bars over them, such as the find bar.
func (s *Search) SetRevealMargin(e core.Edges) { s.margin = e }

// place is the place of the i'th match.
func (m searchMatches) place(i int) place {
	h := m.hits[i]
	return place{set: true, view: h.view, key: h.text.key, from: h.match.From, to: h.match.To, index: i}
}

// locate returns the index of the match at s.at, else of the first after it, wrapping to the first, else, where its view is gone, of the match at its index.
func (s *Search) locate(m searchMatches) int {
	at := s.at
	if !at.set {
		return 0
	}
	rank, ok := m.rank[at.view]
	if !ok {
		return min(at.index, len(m.hits)-1)
	}
	for i, h := range m.hits {
		if h.view == at.view && h.text.key == at.key && h.match.From == at.from && h.match.To == at.to {
			return i
		}
	}
	for i, h := range m.hits {
		if c := cmp.Or(cmp.Compare(m.rank[h.view], rank), cmp.Compare(h.text.key, at.key), cmp.Compare(h.match.From, at.from)); c >= 0 {
			return i
		}
	}
	return 0
}

// searchView is a TextView in a search: its root, which orders it among the others, and the texts it shows, in their places' order.
type searchView struct {
	s       *Search
	root    *core.Ref
	texts   []*searchText
	version *core.Signal[int]
	matches *core.Memo[viewMatches]
}

// viewMatches are a view's matches, at most maxMatches, in its texts' order, and each text's.
type viewMatches struct {
	hits   []hit
	byText map[*searchText][]textRange
}

// join has the view rendering under cx join s while cx lasts.
func (s *Search) join(cx *core.Scope, ref *core.Ref) *searchView {
	if ref == nil {
		ref = new(core.Ref)
	}
	v := &searchView{s: s, root: ref, version: core.NewSignal(cx, 0)}
	v.matches = core.NewMemoFunc(cx, func() viewMatches {
		v.version.Get()
		var m viewMatches
		if s.finder.Get() == nil {
			return m
		}
		m.byText = map[*searchText][]textRange{}
		for _, t := range v.texts {
			found := t.found.Get()
			if len(found) > maxMatches-len(m.hits) {
				found = found[:maxMatches-len(m.hits)]
			}
			if len(found) == 0 {
				continue
			}
			m.byText[t] = found
			for _, r := range found {
				m.hits = append(m.hits, hit{view: v, text: t, match: Match{Text: t.place, From: r.from, To: r.to}})
			}
			if len(m.hits) == maxMatches {
				break
			}
		}
		return m
	}, nil)
	// The view takes its place in tree order once it is mounted.
	cx.Effect(func() { s.views.Set(append(slices.Clip(s.views.GetUntracked()), v)) })
	cx.Cleanup(func() {
		views := s.views.GetUntracked()
		if i := slices.Index(views, v); i >= 0 {
			s.views.Set(slices.Delete(slices.Clone(views), i, i+1))
		}
	})
	return v
}

// searchText is a text a view shows in a search: its place, its view, and its matches.
type searchText struct {
	view  *searchView
	place TextRef
	key   string
	ref   *core.Ref
	// found are its own matches; shown those the view keeps, with the selected one's index.
	found *core.Memo[[]textRange]
	shown *core.Memo[shownRanges]
}

type shownRanges struct {
	ranges   []textRange
	selected int
}

// text has the text at place, whose characters text gives, join the view while cx lasts.
func (v *searchView) text(cx *core.Scope, place TextRef, text func() string) *searchText {
	t := &searchText{view: v, place: place, key: place.key(), ref: new(core.Ref)}
	t.found = core.NewMemoFunc(cx, func() []textRange { return v.s.finder.Get().find(text(), maxMatches) }, slices.Equal[[]textRange])
	i, _ := slices.BinarySearchFunc(v.texts, t.key, func(e *searchText, k string) int { return cmp.Compare(e.key, k) })
	for i < len(v.texts) && v.texts[i].key == t.key {
		i++
	}
	v.texts = slices.Insert(v.texts, i, t)
	v.version.Set(v.version.GetUntracked() + 1)
	cx.Cleanup(func() {
		if i := slices.Index(v.texts, t); i >= 0 {
			v.texts = slices.Delete(v.texts, i, i+1)
			v.version.Set(v.version.GetUntracked() + 1)
		}
	})
	t.shown = core.NewMemoFunc(cx, func() shownRanges {
		sh := shownRanges{ranges: v.matches.Get().byText[t], selected: -1}
		if p := v.s.sel.Get(); p.set && p.view == v && p.key == t.key {
			for i, r := range sh.ranges {
				if r.from == p.from && r.to == p.to {
					sh.selected = i
				}
			}
		}
		return sh
	}, func(a, b shownRanges) bool { return a.selected == b.selected && slices.Equal(a.ranges, b.ranges) })
	return t
}

// reveal reveals the text's characters from from to to through every scroll view around it, keeping margin clear at the outermost one's edges, once the text is laid out.
func (t *searchText) reveal(from, to int, margin core.Edges) {
	ref := t.ref
	ref.Measure(func(_, _ core.Bounds, ok bool) {
		if boxes := ref.TextRangeBoxes(from, to); ok && len(boxes) > 0 {
			ref.RevealBox(boxes[0], core.RevealOptions{Vertical: core.ScrollAlignNearest, Horizontal: core.ScrollAlignNearest, Margin: margin})
		}
	})
}

// searched is t, a text at place showing spans, which a view in a search builds again as the search's matches in it change: split at their ends, with the search's parts on them.
func (r *renderer) searched(cx *core.Scope, place TextRef, t core.Text) core.Text {
	t = r.selectable(t)
	if r.search == nil {
		return t
	}
	var spans func() []core.Span
	switch {
	case t.SpansFunc != nil:
		spans = t.SpansFunc
	case t.Spans != nil:
		static := pinned(t.Spans)
		spans = func() []core.Span { return resolvedSpans(static) }
	default:
		static := []core.Span{{Content: t.Content}}
		spans = func() []core.Span { return static }
	}
	st := r.search.text(cx, place, func() string { return spansText(spans()) })
	t.Ref, t.Content, t.Spans = st.ref, "", nil
	t.SpansFunc = func() []core.Span {
		sh := st.shown.Get()
		return r.searchSpans(spans(), sh.ranges, sh.selected)
	}
	return t
}
