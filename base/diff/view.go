// Package diff is base's DiffView, a port of NSH's: the changes between
// two texts, or the files of a patch, side by side or one over the other,
// the words changed within lines marked, the unchanged lines between
// changes collapsed into separators that expand, and the text colored by
// the Editor's highlighting. It has no look of its own: Styles gives every
// part, the added and deleted tints among them, its look.
//
// It is a package of its own because its highlighting links chroma's
// lexers.
package diff

import (
	"fmt"
	"math"
	"path"
	"slices"
	"strings"

	"github.com/alecthomas/chroma/v2"

	"github.com/oneo-me/nsgo/base/internal/syntax"
	"github.com/oneo-me/nsgo/base/internal/window"
	"github.com/oneo-me/nsgo/core"
)

// Indicators is how changed lines are marked beside their numbers.
type Indicators uint8

const (
	// IndicatorBars is a bar at the gutter's edge.
	IndicatorBars Indicators = iota
	// IndicatorClassic is + and −.
	IndicatorClassic
	IndicatorNone
)

// Side is a side of a diff.
type Side uint8

const (
	Old Side = iota
	New
)

// Hunk is a change: its index, its file's index in a patch (0 for two
// texts), and its lines as 1-based numbers and counts on each side.
type Hunk struct {
	Index, File                            int
	OldStart, OldLines, NewStart, NewLines int
}

// Stats is the lines added and deleted and the number of changes.
type Stats struct{ Added, Deleted, Hunks int }

// File is a file of a patch: its paths (OldPath for renames and copies),
// status, counts, its changes from FirstHunk, and whether it is collapsed.
type File struct {
	Path, OldPath    string
	Status           FileStatus
	Binary           bool
	Added, Deleted   int
	Hunks, FirstHunk int
	Collapsed        bool
}

// Styles are the styles of a DiffView's parts, its whole look.
type Styles struct {
	// Side is each side (split) or the one column (unified); Gutter a
	// side's numbers and indicators; LineNumber every number, with
	// LineNumberAdded and LineNumberDeleted after it on changed lines.
	Side, Gutter, LineNumber, LineNumberAdded, LineNumberDeleted core.Style
	// Content is a side's text beside its gutter; Line every row, with
	// LineAdded and LineDeleted after it; Filler the empty half of a split
	// row beside a line the other side lacks.
	Content, Line, LineAdded, LineDeleted, Filler core.Style
	// WordAdded and WordDeleted mark the words changed within lines.
	WordAdded, WordDeleted core.Style
	// IndicatorAdded and IndicatorDeleted are the bars, or the + and −.
	IndicatorAdded, IndicatorDeleted core.Style
	// Separator is a separator of collapsed lines or a patch's hunk
	// header; HunkActions the box of the app's actions on a change;
	// FileHeader a patch's file's header.
	Separator, HunkActions, FileHeader core.Style
}

// DiffView shows the changes between OldValue and NewValue, or the files
// of Patch when they are nil.
type DiffView struct {
	OldValue, NewValue func() string
	// Patch is a unified diff of one file or several: git diff's, git
	// format-patch's, diff -u's. Each hunk shows under a separator of its
	// header, its lines numbered as in the file.
	Patch func() string
	// Language colors the text; a patch's files by their paths without
	// it.
	Language string
	// Unified shows one column, the deleted lines over the added ones,
	// rather than two sides.
	Unified    bool
	Inline     Inline
	Indicators Indicators
	// ContextLines is the unchanged lines shown around each change: 3 when
	// 0, none when negative. ExpandAll shows every one.
	ContextLines int
	ExpandAll    bool
	// NoLineNumbers hides the numbers.
	NoLineNumbers bool
	// FontFamily (the monospaced font when empty), FontSize (13) and
	// LineHeight (20).
	FontFamily           string
	FontSize, LineHeight core.Points
	// RenderHunkActions draws the app's actions on a change (revert,
	// keep), at its first row's right.
	RenderHunkActions func(cx *core.Scope, h Hunk) core.Node
	Styles            Styles
	Style             core.Style
	StyleFunc         func() core.Style
	Ref               *Ref
	TestID            string

	// OnChange receives the texts after a change was reverted or kept.
	OnChange          func(old, new string)
	OnLineNumberPress func(side Side, line int)
	// OnStatsChange receives the counts once and whenever a diff counts
	// otherwise.
	OnStatsChange func(Stats)
	OnFileToggle  func(File)
	// OnError receives a patch that cannot be read.
	OnError func(error)
}

// Ref is a DiffView's methods, bound once it mounts.
type Ref struct{ v *dv }

// rowKind is what a row shows.
type rowKind uint8

const (
	rowEqual rowKind = iota
	rowModified
	rowDeleted
	rowAdded
	rowSeparator
	rowFile
)

// row is a row of the view: lines of each side (-1 for none), or a
// separator of a gap or a hunk's header, or a file's header.
type row struct {
	kind     rowKind
	old, new int
	hunk     int // the change a line is in, -1 for none
	gap      int // a separator's gap
	hidden   int
	header   string
	file     int
}

// segment is a file's part of the joined texts.
type segment struct {
	file             File
	oldAt, newAt     int
	oldLen, newLen   int
	hunk0, hunk1     int
	oldNums, newNums []int
	starts           [][2]int // a patch's hunks' first lines, file-local
	headers          []string
	lexer            chroma.Lexer
	patch            bool
}

type dv struct {
	p            DiffView
	cx           *core.Scope
	m            *model
	segs         []segment
	rows         []row
	inl          map[[2]int][2][][2]int
	hlOld, hlNew [][]syntax.Tok
	expand       map[int][2]int
	collapsed    map[string]bool
	rev, hlRev   *core.Signal[int]
	scrollY      *core.Signal[core.Points]
	viewport     *core.Signal[core.Size]
	vs           *core.Ref
	win          *window.Rows
	font         core.Font
	charW, lh    float32
	widest       int
	stats        Stats
	reported     bool
	current      int
	job          int
}

// Render implements core.Node.
func (p DiffView) Render(cx *core.Scope) core.Node {
	v := &dv{p: p, cx: cx, rev: core.NewSignal(cx, 0), hlRev: core.NewSignal(cx, 0),
		scrollY: core.NewSignal(cx, core.Points(0)), viewport: core.NewSignal(cx, core.Size{}), vs: &core.Ref{},
		win: window.New(8, 16), expand: map[int][2]int{}, collapsed: map[string]bool{}, current: -1, m: &model{}}
	v.font = core.Font{Family: p.FontFamily, Size: p.FontSize}
	if v.font.Family == "" {
		v.font.Family = core.FontMonospace
	}
	if v.font.Size == 0 {
		v.font.Size = 13
	}
	v.lh = float32(p.LineHeight)
	if v.lh == 0 {
		v.lh = 20
	}
	v.charW = max(cx.MeasureText("0000000000", v.font).Width/10, 1)
	if p.Ref != nil {
		p.Ref.v = v
	}
	cx.Effect(func() {
		var oldText, newText, patch string
		if p.OldValue != nil || p.NewValue != nil {
			if p.OldValue != nil {
				oldText = p.OldValue()
			}
			if p.NewValue != nil {
				newText = p.NewValue()
			}
		} else if p.Patch != nil {
			patch = p.Patch()
		}
		cx.Untrack(func() { v.load(oldText, newText, patch) })
	})
	return core.View{
		Style:     merge(core.Style{Overflow: core.Hidden, FontFamily: v.font.Family, FontSize: v.font.Size}, p.Style),
		StyleFunc: p.StyleFunc, TestID: p.TestID,
		Children: []core.Node{core.Scroll{
			Ref: v.vs, TestID: "diff-scroll",
			Style:    core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)},
			OnScroll: func(e core.ScrollEvent) { v.scrollY.Set(e.Offset.Y) },
			OnLayout: func(b core.Bounds) { v.viewport.Set(b.Size) },
			Children: []core.Node{core.Scroll{Horizontal: true, Children: []core.Node{v.box()}}},
		}},
	}
}

func merge(styles ...core.Style) core.Style {
	var out core.Style
	for _, s := range styles {
		if !s.IsZero() {
			out = out.Merge(s)
		}
	}
	return out
}

// load diffs the texts, or the patch's files, joined into two texts.
func (v *dv) load(oldText, newText, patch string) {
	v.segs = nil
	v.expand = map[int][2]int{}
	v.current = -1
	var oldLines, newLines []string
	var hunks []hunk
	add := func(s segment, old, new []string, ops []op) {
		s.oldAt, s.newAt, s.oldLen, s.newLen = len(oldLines), len(newLines), len(old), len(new)
		oldLines, newLines = append(oldLines, old...), append(newLines, new...)
		s.hunk0 = len(hunks)
		i, j := 0, 0
		for k := 0; k < len(ops); {
			if ops[k] == opEqual {
				i, j, k = i+1, j+1, k+1
				continue
			}
			h := hunk{oldStart: i, newStart: j}
			for k < len(ops) && ops[k] != opEqual {
				if ops[k] == opDelete {
					i++
				} else {
					j++
				}
				k++
			}
			h.oldEnd, h.newEnd = i, j
			h.pairs = pair(old[h.oldStart:h.oldEnd], new[h.newStart:h.newEnd], s.oldAt+h.oldStart, s.newAt+h.newStart)
			h.oldStart, h.oldEnd, h.newStart, h.newEnd = h.oldStart+s.oldAt, h.oldEnd+s.oldAt, h.newStart+s.newAt, h.newEnd+s.newAt
			hunks = append(hunks, h)
			s.file.Added += h.newEnd - h.newStart
			s.file.Deleted += h.oldEnd - h.oldStart
		}
		s.hunk1 = len(hunks)
		s.file.Hunks, s.file.FirstHunk = s.hunk1-s.hunk0, s.hunk0
		v.segs = append(v.segs, s)
	}
	if patch == "" {
		old, new := splitLines(oldText), splitLines(newText)
		add(segment{lexer: syntax.Lexer(v.p.Language)}, old, new, myers(old, new, maxSteps))
	} else {
		files, err := parsePatch(patch)
		if err != nil && v.p.OnError != nil {
			v.p.OnError(err)
		}
		for i, f := range files {
			oldT, newT, on, nn := f.texts()
			var ops []op
			var starts [][2]int
			var headers []string
			oi, ni := 0, 0
			for _, h := range f.hunks {
				starts = append(starts, [2]int{oi, ni})
				headers = append(headers, h.header)
				for _, l := range h.lines {
					switch l[0] {
					case '-':
						ops = append(ops, opDelete)
						oi++
					case '+':
						ops = append(ops, opInsert)
						ni++
					default:
						ops = append(ops, opEqual)
						oi, ni = oi+1, ni+1
					}
				}
			}
			lang := v.p.Language
			if lang == "" {
				lang = path.Base(f.path)
			}
			file := File{Path: f.path, OldPath: f.oldPath, Status: f.status, Binary: f.binary, Collapsed: v.collapsed[f.path]}
			if file.OldPath == file.Path {
				file.OldPath = ""
			}
			_ = i
			add(segment{file: file, oldNums: on, newNums: nn, starts: starts, headers: headers, patch: true,
				lexer: syntax.Lexer(lang)}, splitLines(oldT), splitLines(newT), ops)
		}
		for path := range v.collapsed {
			if !slices.ContainsFunc(v.segs, func(s segment) bool { return s.file.Path == path }) {
				delete(v.collapsed, path)
			}
		}
	}
	v.m = &model{old: oldLines, new: newLines, hunks: hunks}
	v.inl = map[[2]int][2][][2]int{}
	v.widest = 0
	for _, l := range append(slices.Clone(oldLines), newLines...) {
		v.widest = max(v.widest, columns(l))
	}
	v.build()
	v.highlight()
	st := Stats{Hunks: len(hunks)}
	for _, s := range v.segs {
		st.Added += s.file.Added
		st.Deleted += s.file.Deleted
	}
	if (!v.reported || st != v.stats) && v.p.OnStatsChange != nil {
		v.p.OnStatsChange(st)
	}
	v.stats, v.reported = st, true
}

// columns is a line's width in columns, tabs as 4.
func columns(l string) int {
	n := 0
	for _, r := range strings.TrimSuffix(l, "\n") {
		if r == '\t' {
			n += 4
		} else {
			n++
		}
	}
	return n
}

// context is the unchanged lines shown around a change.
func (v *dv) context() int {
	switch {
	case v.p.ContextLines < 0:
		return 0
	case v.p.ContextLines == 0:
		return 3
	}
	return v.p.ContextLines
}

// build makes the rows: file headers, separators and lines.
func (v *dv) build() {
	var rows []row
	many := len(v.segs) > 1
	gap := 0
	for fi, s := range v.segs {
		if many || s.patch && s.file.Path != "" && len(v.segs) > 1 {
			rows = append(rows, row{kind: rowFile, file: fi, old: -1, new: -1, hunk: -1})
		}
		if s.file.Collapsed {
			continue
		}
		if s.patch {
			rows = v.patchRows(rows, s, &gap)
			continue
		}
		rows = v.textRows(rows, s, &gap)
	}
	v.rows = rows
	v.rev.Update(func(n int) int { return n + 1 })
}

// hunkRows are a change's rows: side by side in pairs, or the deleted
// lines over the added ones.
func (v *dv) hunkRows(rows []row, h hunk, index int) []row {
	if v.p.Unified {
		for i := h.oldStart; i < h.oldEnd; i++ {
			rows = append(rows, row{kind: rowDeleted, old: i, new: -1, hunk: index})
		}
		for j := h.newStart; j < h.newEnd; j++ {
			rows = append(rows, row{kind: rowAdded, old: -1, new: j, hunk: index})
		}
		return rows
	}
	oi, ni := h.oldStart, h.newStart
	for _, pr := range append(h.pairs, [2]int{h.oldEnd, h.newEnd}) {
		for ; oi < pr[0]; oi++ {
			rows = append(rows, row{kind: rowDeleted, old: oi, new: -1, hunk: index})
		}
		for ; ni < pr[1]; ni++ {
			rows = append(rows, row{kind: rowAdded, old: -1, new: ni, hunk: index})
		}
		if pr[0] < h.oldEnd && pr[1] < h.newEnd {
			rows = append(rows, row{kind: rowModified, old: pr[0], new: pr[1], hunk: index})
			oi, ni = pr[0]+1, pr[1]+1
		}
	}
	return rows
}

// textRows are two texts' rows: the changes with their context, and
// separators for the unchanged lines between them, as the user expanded
// them.
func (v *dv) textRows(rows []row, s segment, gap *int) []row {
	ctx := v.context()
	equal := func(o, n int) row { return row{kind: rowEqual, old: o, new: n, hunk: -1} }
	oi, ni := s.oldAt, s.newAt
	for hi := s.hunk0; hi <= s.hunk1; hi++ {
		// The unchanged lines before change hi (or after the last).
		endO := s.oldAt + s.oldLen
		if hi < s.hunk1 {
			endO = v.m.hunks[hi].oldStart
		}
		count := endO - oi
		top, bottom := ctx, ctx
		if hi == s.hunk0 {
			top = 0
		}
		if hi == s.hunk1 {
			bottom = 0
		}
		if e, ok := v.expand[*gap]; ok {
			top, bottom = top+e[0], bottom+e[1]
		}
		if v.p.ExpandAll || top+bottom >= count-1 {
			top, bottom = count, 0
		}
		for k := 0; k < min(top, count); k++ {
			rows = append(rows, equal(oi+k, ni+k))
		}
		if hidden := count - top - bottom; hidden > 0 {
			rows = append(rows, row{kind: rowSeparator, gap: *gap, hidden: hidden, old: oi + top, new: ni + top, hunk: -1})
		}
		for k := max(count-bottom, top); k < count; k++ {
			rows = append(rows, equal(oi+k, ni+k))
		}
		*gap++
		oi, ni = oi+count, ni+count
		if hi < s.hunk1 {
			h := v.m.hunks[hi]
			rows = v.hunkRows(rows, h, hi)
			oi, ni = h.oldEnd, h.newEnd
		}
	}
	return rows
}

// patchRows are a patched file's rows: each hunk under a separator of its
// header, every line shown.
func (v *dv) patchRows(rows []row, s segment, gap *int) []row {
	hi := s.hunk0
	oi, ni := s.oldAt, s.newAt
	next := 0
	for oi < s.oldAt+s.oldLen || ni < s.newAt+s.newLen {
		if next < len(s.starts) && oi-s.oldAt == s.starts[next][0] && ni-s.newAt == s.starts[next][1] {
			rows = append(rows, row{kind: rowSeparator, gap: *gap, header: s.headers[next], old: oi, new: ni, hunk: -1})
			*gap++
			next++
		}
		if hi < s.hunk1 && v.m.hunks[hi].oldStart == oi && v.m.hunks[hi].newStart == ni {
			h := v.m.hunks[hi]
			rows = v.hunkRows(rows, h, hi)
			oi, ni, hi = h.oldEnd, h.newEnd, hi+1
			continue
		}
		rows = append(rows, row{kind: rowEqual, old: oi, new: ni, hunk: -1})
		oi, ni = oi+1, ni+1
	}
	return rows
}

// highlight lexes each file's sides on a goroutine of its own.
func (v *dv) highlight() {
	v.hlOld, v.hlNew = make([][]syntax.Tok, len(v.m.old)), make([][]syntax.Tok, len(v.m.new))
	v.job++
	job := v.job
	for _, s := range v.segs {
		if s.lexer == nil {
			continue
		}
		strip := func(lines []string) []string {
			out := make([]string, len(lines))
			for i, l := range lines {
				out[i] = strings.TrimSuffix(l, "\n")
			}
			return out
		}
		old := strip(v.m.old[s.oldAt : s.oldAt+s.oldLen])
		new := strip(v.m.new[s.newAt : s.newAt+s.newLen])
		lexer, at := s.lexer, [2]int{s.oldAt, s.newAt}
		go func() {
			o, n := syntax.LexLines(lexer, old), syntax.LexLines(lexer, new)
			v.cx.Post(func() {
				if v.job != job {
					return
				}
				copy(v.hlOld[at[0]:], o)
				copy(v.hlNew[at[1]:], n)
				v.hlRev.Update(func(n int) int { return n + 1 })
			})
		}()
	}
}

// inlineOf is a modified row's changed ranges, computed once.
func (v *dv) inlineOf(o, n int) [2][][2]int {
	k := [2]int{o, n}
	if r, ok := v.inl[k]; ok {
		return r
	}
	a, b := inline(v.m.old[o], v.m.new[n], v.p.Inline)
	v.inl[k] = [2][][2]int{a, b}
	return v.inl[k]
}

// windowRows is the rows made (tracked).
func (v *dv) windowRows() (int, int) {
	v.rev.Get()
	vp := v.viewport.Get()
	y := float32(v.scrollY.Get())
	n := len(v.rows)
	first := min(max(int(y/v.lh), 0), n)
	last := min(int((y+max(float32(vp.Height), 40*v.lh))/v.lh)+1, n)
	return v.win.Span(first, max(last, first), n)
}

// numberWidth is a number column's width.
func (v *dv) numberWidth() float32 {
	if v.p.NoLineNumbers {
		return 0
	}
	most := max(len(v.m.old), len(v.m.new))
	for _, s := range v.segs {
		if len(s.oldNums) > 0 {
			most = max(most, slices.Max(append(s.oldNums, 0)))
		}
		if len(s.newNums) > 0 {
			most = max(most, slices.Max(append(s.newNums, 0)))
		}
	}
	return float32(max(len(fmt.Sprint(most)), 2))*v.charW + 8
}

// gutterWidth is a side's gutter: its numbers (two unified) and the
// indicator.
func (v *dv) gutterWidth() float32 {
	w := v.numberWidth()
	if v.p.Unified {
		w *= 2
	}
	if v.p.Indicators != IndicatorNone {
		w += v.charW + 4
	}
	return w
}

// sideWidth is a side's width.
func (v *dv) sideWidth() float32 {
	v.rev.Get()
	vp := v.viewport.Get()
	text := float32(v.widest)*v.charW + 16
	room := float32(vp.Width)
	if !v.p.Unified {
		room = float32(math.Floor(float64(room / 2)))
	}
	return max(v.gutterWidth()+text, room)
}

// boxWidth is the rows' width: a side, or two.
func (v *dv) boxWidth() float32 {
	w := v.sideWidth()
	if !v.p.Unified {
		w *= 2
	}
	return w
}

func (v *dv) box() core.Node {
	return core.View{
		StyleFunc: func() core.Style {
			v.rev.Get()
			return core.Style{Width: core.Pt(core.Points(v.boxWidth())), Height: core.Pt(core.Points(float32(len(v.rows)) * v.lh))}
		},
		Children: []core.Node{
			core.Each(func() []int {
				s, e := v.windowRows()
				out := make([]int, e-s)
				for i := range out {
					out[i] = s + i
				}
				return out
			}, func(i int) int { return i }, func(cx *core.Scope, i func() int, _ func() int) core.Node {
				return core.Dynamic(func(cx *core.Scope) core.Node {
					v.rev.Get()
					if i() >= len(v.rows) {
						return core.Group{}
					}
					return v.rowView(i(), v.rows[i()])
				})
			}),
			v.actions(),
		},
	}
}

// rowView is row r at index i.
func (v *dv) rowView(i int, r row) core.Node {
	s := &v.p.Styles
	place := core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Top: core.Pt(core.Points(float32(i) * v.lh)),
		Height: core.Pt(core.Points(v.lh)), Direction: core.Row, AlignItems: core.AlignCenter}
	switch r.kind {
	case rowFile:
		seg := v.segs[r.file]
		f := seg.file
		name := f.Path
		if f.OldPath != "" && f.OldPath != f.Path {
			name = f.OldPath + " → " + f.Path
		}
		mark := "▾ "
		if f.Collapsed {
			mark = "▸ "
		}
		return core.View{Style: merge(place, s.FileHeader), Role: core.RoleButton, Label: name,
			OnPress: func(core.PressEvent) { v.toggleFile(f.Path) },
			Children: []core.Node{core.Text{MaxLines: 1, Spans: []core.Span{{Content: mark + name + "  "},
				{Content: fmt.Sprintf("+%d", f.Added), Style: core.Style{Color: s.IndicatorAdded.Color}}, {Content: " "},
				{Content: fmt.Sprintf("−%d", f.Deleted), Style: core.Style{Color: s.IndicatorDeleted.Color}}}}}}
	case rowSeparator:
		label := r.header
		if label == "" {
			label = fmt.Sprintf("⋯ %d unchanged lines", r.hidden)
			if r.hidden == 1 {
				label = "⋯ 1 unchanged line"
			}
		}
		gap := r.gap
		return core.View{Style: merge(place, s.Separator), Role: core.RoleButton, Label: label,
			OnPress: func(core.PressEvent) {
				if r.header == "" {
					v.expandGap(gap, math.MaxInt32, math.MaxInt32)
				}
			},
			Children: []core.Node{core.Text{Content: label, MaxLines: 1}}}
	}
	if v.p.Unified {
		return core.View{Style: merge(place, s.Side), Children: v.sideView(r, -1)}
	}
	half := core.Style{Width: core.Pt(core.Points(v.sideWidth())), Height: core.Pt(core.Points(v.lh)), Direction: core.Row,
		AlignItems: core.AlignCenter}
	return core.View{Style: place, Children: []core.Node{
		core.View{Style: merge(half, s.Side), Children: v.sideView(r, int(Old))},
		core.View{Style: merge(half, s.Side), Children: v.sideView(r, int(New))},
	}}
}

// sideView is a row's gutter and text on side (-1: unified).
func (v *dv) sideView(r row, side int) []core.Node {
	s := &v.p.Styles
	line, isOld := r.new, false
	switch {
	case side == int(Old):
		line, isOld = r.old, true
	case side < 0 && r.kind == rowDeleted:
		line, isOld = r.old, true
	}
	added := r.kind == rowAdded || r.kind == rowModified && !isOld
	deleted := r.kind == rowDeleted || r.kind == rowModified && isOld
	if side < 0 && r.kind == rowModified {
		added, deleted = false, false
	}
	lineStyle := s.Line
	switch {
	case line < 0:
		lineStyle = merge(lineStyle, s.Filler)
	case added:
		lineStyle = merge(lineStyle, s.LineAdded)
	case deleted:
		lineStyle = merge(lineStyle, s.LineDeleted)
	}
	numStyle := merge(core.Style{Width: core.Pt(core.Points(v.numberWidth())), TextAlign: core.TextAlignEnd,
		Padding: core.Edges{Right: 8}}, s.LineNumber)
	if added {
		numStyle = merge(numStyle, s.LineNumberAdded)
	} else if deleted {
		numStyle = merge(numStyle, s.LineNumberDeleted)
	}
	number := func(sd Side, at int) core.Node {
		if v.p.NoLineNumbers {
			return core.Group{}
		}
		text := ""
		if at >= 0 {
			text = fmt.Sprint(v.number(sd, at))
		}
		return core.Text{Content: text, MaxLines: 1, Style: numStyle,
			OnPress: func(core.PressEvent) {
				if at >= 0 && v.p.OnLineNumberPress != nil {
					v.p.OnLineNumberPress(sd, v.number(sd, at))
				}
			}}
	}
	var gutter []core.Node
	if side < 0 {
		gutter = append(gutter, number(Old, r.old), number(New, r.new))
	} else if isOld {
		gutter = append(gutter, number(Old, line))
	} else {
		gutter = append(gutter, number(New, line))
	}
	switch v.p.Indicators {
	case IndicatorBars:
		var st core.Style
		if added {
			st = s.IndicatorAdded
		} else if deleted {
			st = s.IndicatorDeleted
		}
		gutter = append(gutter, core.View{Style: core.Style{Width: core.Pt(core.Points(v.charW + 4)), Height: core.Pt(core.Points(v.lh))},
			Children: []core.Node{core.View{Style: merge(core.Style{Width: core.Pt(3), Height: core.Pt(core.Points(v.lh))}, st)}}})
	case IndicatorClassic:
		mark, st := " ", core.Style{}
		if added {
			mark, st = "+", s.IndicatorAdded
		} else if deleted {
			mark, st = "−", s.IndicatorDeleted
		}
		gutter = append(gutter, core.Text{Content: mark, Style: merge(core.Style{Width: core.Pt(core.Points(v.charW + 4))}, st)})
	}
	var text core.Node = core.Group{}
	if line >= 0 {
		text = core.Text{MaxLines: 1, Style: core.Style{WhiteSpace: core.WhiteSpaceNoWrap, LineHeight: core.Points(v.lh)},
			SpansFunc: func() []core.Span { return v.spans(r, isOld, line) }}
	}
	return []core.Node{
		core.View{Style: merge(core.Style{Direction: core.Row, Height: core.Pt(core.Points(v.lh)), AlignItems: core.AlignCenter},
			s.Gutter), Children: gutter},
		core.View{Style: merge(core.Style{FlexGrow: 1, Height: core.Pt(core.Points(v.lh)), Direction: core.Row,
			AlignItems: core.AlignCenter, Padding: core.Edges{Left: 8, Right: 8}}, lineStyle, s.Content), Children: []core.Node{text}},
	}
}

// number is line at's number on side: as in the file for a patch,
// counted in its file otherwise.
func (v *dv) number(side Side, at int) int {
	for _, s := range v.segs {
		if side == Old && at >= s.oldAt && at < s.oldAt+s.oldLen {
			if s.oldNums != nil {
				return s.oldNums[at-s.oldAt]
			}
			return at - s.oldAt + 1
		}
		if side == New && at >= s.newAt && at < s.newAt+s.newLen {
			if s.newNums != nil {
				return s.newNums[at-s.newAt]
			}
			return at - s.newAt + 1
		}
	}
	return at + 1
}

// spans is a line's text in runs of one token and one inline state, tabs
// as four spaces.
func (v *dv) spans(r row, isOld bool, line int) []core.Span {
	v.hlRev.Get()
	dark := v.cx.Env().Dark()
	palette := &syntax.Light
	if dark {
		palette = &syntax.Dark
	}
	text := v.m.new[line]
	toks := v.hlNew[line]
	word := v.p.Styles.WordAdded
	if isOld {
		text, toks, word = v.m.old[line], v.hlOld[line], v.p.Styles.WordDeleted
	}
	text = strings.TrimSuffix(text, "\n")
	var marks [][2]int
	if r.kind == rowModified {
		in := v.inlineOf(r.old, r.new)
		marks = in[1]
		if isOld {
			marks = in[0]
		}
	}
	cuts := []int{0, len(text)}
	for _, t := range toks {
		cuts = append(cuts, t.Start, t.End)
	}
	for _, m := range marks {
		cuts = append(cuts, m[0], m[1])
	}
	slices.Sort(cuts)
	cuts = slices.Compact(cuts)
	var out []core.Span
	for i := 0; i+1 < len(cuts); i++ {
		a, b := cuts[i], cuts[i+1]
		if b > len(text) || a >= b {
			continue
		}
		var st core.Style
		for _, t := range toks {
			if t.Start <= a && b <= t.End {
				st = palette[t.Kind]
			}
		}
		for _, m := range marks {
			if m[0] <= a && b <= m[1] {
				st = merge(st, word)
			}
		}
		out = append(out, core.Span{Content: strings.ReplaceAll(text[a:b], "\t", "    "), Style: st})
	}
	if len(out) == 0 {
		out = []core.Span{{Content: ""}}
	}
	return out
}

// actions are the app's actions on each change in view, at its first
// row's right.
func (v *dv) actions() core.Node {
	if v.p.RenderHunkActions == nil {
		return core.Group{}
	}
	return core.Each(func() []int {
		v.rev.Get()
		s, e := v.windowRows()
		var out []int
		for i := s; i < e && i < len(v.rows); i++ {
			r := v.rows[i]
			if r.hunk >= 0 && (i == 0 || v.rows[i-1].hunk != r.hunk) {
				out = append(out, r.hunk)
			}
		}
		return out
	}, func(h int) int { return h }, func(cx *core.Scope, h func() int, _ func() int) core.Node {
		hk := v.publicHunk(core.Untracked(cx, h))
		return core.View{
			PointerEvents: core.PointerEventsBoxNone,
			StyleFunc: func() core.Style {
				v.rev.Get()
				top := float32(0)
				for i, r := range v.rows {
					if r.hunk == h() {
						top = float32(i) * v.lh
						break
					}
				}
				// At the right of the view, not of the box, which is as wide
				// as the longest line.
				right := min(v.boxWidth(), float32(v.viewport.Get().Width)) - 8
				return merge(core.Style{Position: core.Absolute, Left: core.Pt(0), Width: core.Pt(core.Points(right)),
					Top: core.Pt(core.Points(top)), Direction: core.Row, JustifyContent: core.JustifyEnd}, v.p.Styles.HunkActions)
			},
			Children: []core.Node{v.p.RenderHunkActions(cx, hk)},
		}
	})
}

func (v *dv) publicHunk(i int) Hunk {
	h := v.m.hunks[i]
	file := 0
	for fi, s := range v.segs {
		if i >= s.hunk0 && i < s.hunk1 {
			file = fi
		}
	}
	return Hunk{Index: i, File: file, OldStart: v.number(Old, min(h.oldStart, max(len(v.m.old)-1, 0))), OldLines: h.oldEnd - h.oldStart,
		NewStart: v.number(New, min(h.newStart, max(len(v.m.new)-1, 0))), NewLines: h.newEnd - h.newStart}
}

func (v *dv) expandGap(gap, up, down int) {
	e := v.expand[gap]
	v.expand[gap] = [2]int{min(e[0]+up, math.MaxInt32), min(e[1]+down, math.MaxInt32)}
	v.build()
}

func (v *dv) toggleFile(path string) {
	v.collapsed[path] = !v.collapsed[path]
	for i := range v.segs {
		if v.segs[i].file.Path == path {
			v.segs[i].file.Collapsed = v.collapsed[path]
			if v.p.OnFileToggle != nil {
				v.p.OnFileToggle(v.segs[i].file)
			}
		}
	}
	v.build()
}

// goTo scrolls row i to the top.
func (v *dv) goTo(i int) {
	y := float32(i) * v.lh
	v.vs.ScrollTo(core.Point{Y: core.Points(y)}, false)
	v.scrollY.Set(core.Points(y))
}

// Hunks returns the changes, in order.
func (r *Ref) Hunks() []Hunk {
	if r.v == nil {
		return nil
	}
	out := make([]Hunk, len(r.v.m.hunks))
	for i := range out {
		out[i] = r.v.publicHunk(i)
	}
	return out
}

// Stats returns the lines added and deleted and the number of changes.
func (r *Ref) Stats() Stats {
	if r.v == nil {
		return Stats{}
	}
	return r.v.stats
}

// GoToHunk scrolls change i's first row to the top, expanding a collapsed
// file first.
func (r *Ref) GoToHunk(i int) {
	v := r.v
	if v == nil || i < 0 || i >= len(v.m.hunks) {
		return
	}
	for _, s := range v.segs {
		if i >= s.hunk0 && i < s.hunk1 && s.file.Collapsed {
			v.toggleFile(s.file.Path)
		}
	}
	for row, x := range v.rows {
		if x.hunk == i {
			v.current = i
			v.goTo(row)
			return
		}
	}
}

// NextHunk scrolls the next change to the top and returns its index,
// after the last the first; -1 for none. PreviousHunk goes back.
func (r *Ref) NextHunk() int { return r.step(1) }

// PreviousHunk scrolls the previous change to the top.
func (r *Ref) PreviousHunk() int { return r.step(-1) }

func (r *Ref) step(d int) int {
	v := r.v
	if v == nil || len(v.m.hunks) == 0 {
		return -1
	}
	n := len(v.m.hunks)
	next := 0
	if v.current >= 0 {
		next = ((v.current+d)%n + n) % n
	} else if d < 0 {
		next = n - 1
	}
	r.GoToHunk(next)
	return next
}

// RevertHunk puts change i's old lines back into the new text and reports
// the texts.
func (r *Ref) RevertHunk(i int) {
	if v := r.v; v != nil && i >= 0 && i < len(v.m.hunks) && v.p.OnChange != nil {
		o, n := v.m.revert(v.m.hunks[i])
		v.load(o, n, "")
		v.p.OnChange(o, n)
	}
}

// AcceptHunk makes change i's new lines the old text's and reports the
// texts.
func (r *Ref) AcceptHunk(i int) {
	if v := r.v; v != nil && i >= 0 && i < len(v.m.hunks) && v.p.OnChange != nil {
		o, n := v.m.accept(v.m.hunks[i])
		v.load(o, n, "")
		v.p.OnChange(o, n)
	}
}

// ExpandAll shows every unchanged line; CollapseAll collapses them again.
func (r *Ref) ExpandAll() {
	if v := r.v; v != nil {
		for g := 0; g <= len(v.m.hunks)+len(v.segs); g++ {
			v.expand[g] = [2]int{math.MaxInt32, math.MaxInt32}
		}
		v.build()
	}
}

// CollapseAll collapses the unchanged lines again.
func (r *Ref) CollapseAll() {
	if v := r.v; v != nil {
		v.expand = map[int][2]int{}
		v.build()
	}
}

// Files returns a patch's files, in order.
func (r *Ref) Files() []File {
	if r.v == nil {
		return nil
	}
	var out []File
	for _, s := range r.v.segs {
		if s.patch {
			out = append(out, s.file)
		}
	}
	return out
}

// ToggleFile collapses the file at path, or expands it.
func (r *Ref) ToggleFile(path string) {
	if r.v != nil {
		r.v.toggleFile(path)
	}
}

// ScrollToFile scrolls the header of the file at path to the top.
func (r *Ref) ScrollToFile(path string) {
	if v := r.v; v != nil {
		for i, x := range v.rows {
			if x.kind == rowFile && v.segs[x.file].file.Path == path {
				v.goTo(i)
				return
			}
		}
	}
}
