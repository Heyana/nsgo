package editor

import (
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

// maxUndo is the most steps undo keeps.
const maxUndo = 1000

// edit replaces the text from from to to with text.
type edit struct {
	from, to pos
	text     string
}

// change is an edit as it was applied: in the text as the changes before
// it in its list left it, with the text it replaced and where the new text
// ends.
type change struct {
	from, oldEnd, newEnd pos
	old, text            string
}

// mapPos carries p through c: before it it stays, after it it moves with
// the text, inside it it goes to the change's end (or start, with
// before).
func (c change) mapPos(p pos, before bool) pos {
	switch {
	case p.less(c.from) || p == c.from && before:
		return p
	case p.less(c.oldEnd):
		if before {
			return c.from
		}
		return c.newEnd
	case p.line == c.oldEnd.line:
		return pos{c.newEnd.line, c.newEnd.col + p.col - c.oldEnd.col}
	}
	return pos{p.line + c.newEnd.line - c.oldEnd.line, p.col}
}

// mapThrough carries p through changes, in the order they were applied.
func mapThrough(changes []change, p pos, before bool) pos {
	for _, c := range changes {
		p = c.mapPos(p, before)
	}
	return p
}

// editKind groups undo steps: runs of typing at one place, or of
// deleting, merge into one step.
type editKind uint8

const (
	kindOther editKind = iota
	kindTyping
	kindDeleting
)

// undoStep is what an undo takes back: the changes in the order they were
// applied and the selections before and after.
type undoStep struct {
	changes       []change
	before, after []selection
	kind          editKind
}

// buffer is the text, its version and its undo history.
type buffer struct {
	r       *rope
	version int
	undo    []undoStep
	redo    []undoStep
	// open lets the next step merge into the last one; a move, a press or
	// a selection set by the app closes it.
	open bool
}

func newBuffer(text string) *buffer { return &buffer{r: newRope(normalize(text))} }

// normalize ends lines with \n only.
func normalize(text string) string {
	if strings.IndexByte(text, '\r') < 0 {
		return text
	}
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}

// apply applies edits of the text as it is, sorted and not overlapping,
// and returns the changes, from the last edit to the first, so each is in
// the text the ones before it left.
func (b *buffer) apply(edits []edit) []change {
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].from.less(edits[j].from) })
	out := make([]change, 0, len(edits))
	for i := len(edits) - 1; i >= 0; i-- {
		e := edits[i]
		if i+1 < len(edits) && edits[i+1].from.less(e.to) {
			continue // overlaps the next: skipped
		}
		from, to := b.r.clamp(e.from), b.r.clamp(e.to)
		if to.less(from) {
			from, to = to, from
		}
		text := normalize(e.text)
		if from == to && text == "" {
			continue
		}
		out = append(out, b.replace(from, to, text))
	}
	if len(out) > 0 {
		b.version++
	}
	return out
}

// replace replaces from to to with text.
func (b *buffer) replace(from, to pos, text string) change {
	old := b.r.slice(from, to)
	first, last := b.r.line(from.line), b.r.line(to.line)
	parts := strings.Split(text, "\n")
	lines := make([]string, len(parts))
	copy(lines, parts)
	lines[0] = first[:from.col] + lines[0]
	end := pos{from.line + len(parts) - 1, len(lines[len(lines)-1])}
	lines[len(lines)-1] += last[to.col:]
	b.r.splice(from.line, to.line+1, lines)
	return change{from: from, oldEnd: to, newEnd: end, old: old, text: text}
}

// record keeps changes as an undo step, merged into the last one when
// both are typing, or deleting, at one place.
func (b *buffer) record(changes []change, before, after []selection, kind editKind) {
	if len(changes) == 0 {
		return
	}
	b.redo = b.redo[:0]
	if n := len(b.undo); n > 0 && b.open && kind != kindOther && b.undo[n-1].kind == kind && b.merges(b.undo[n-1], changes, kind) {
		last := &b.undo[n-1]
		last.changes = append(last.changes, changes...)
		last.after = after
		return
	}
	b.undo = append(b.undo, undoStep{changes: changes, before: before, after: after, kind: kind})
	if len(b.undo) > maxUndo {
		b.undo = b.undo[1:]
	}
	b.open = kind != kindOther
}

// invert returns the change that undoes c.
func (c change) invert() change {
	return change{
		from:   c.from,
		oldEnd: c.newEnd,
		newEnd: c.oldEnd,
		old:    c.text,
		text:   c.old,
	}
}

// changeAt builds a change at from replacing old with text, deriving
// oldEnd and newEnd from their contents so spans cannot drift across
// rebases.
func changeAt(from pos, old, text string) change {
	return change{
		from:   from,
		oldEnd: advance(from, old),
		newEnd: advance(from, text),
		old:    old,
		text:   text,
	}
}

// invertChanges returns the reverse sequence of inverse changes.
func invertChanges(changes []change) []change {
	out := make([]change, len(changes))
	for i, c := range changes {
		out[len(changes)-1-i] = c.invert()
	}
	return out
}

// transformChange commutes two changes a and b starting from the same
// document: aNext applies after b, bNext applies after a, and both
// paths reach the same document. It returns ok=false when a and b
// overlap.
func transformChange(a, b change) (aNext, bNext change, ok bool) {
	switch {
	case b.oldEnd.less(a.from) || b.oldEnd == a.from && (b.from.less(b.oldEnd) || a.from.less(a.oldEnd)):
		// b ends at or before a starts (and they are not both pure
		// insertions at the same position): b is before a.
		return changeAt(b.mapPos(a.from, false), a.old, a.text), b, true
	case a.oldEnd.less(b.from) || a.oldEnd == b.from:
		// a ends at or before b starts (including two pure insertions at
		// the same position, where a goes first): a is before b.
		return a, changeAt(a.mapPos(b.from, false), b.old, b.text), true
	default:
		return change{}, change{}, false
	}
}

// transformChanges commutes two sequential change lists a and b
// starting from the same document: aNext applies after b, bNext applies
// after a, and both paths reach the same document. It returns ok=false
// when any pair of changes overlaps.
func transformChanges(a, b []change) (aNext, bNext []change, ok bool) {
	aNext = make([]change, len(a))
	bNext = slices.Clone(b)
	for i, ca := range a {
		for j := range bNext {
			var nextA, nextB change
			nextA, nextB, ok = transformChange(ca, bNext[j])
			if !ok {
				return nil, nil, false
			}
			ca = nextA
			bNext[j] = nextB
		}
		aNext[i] = ca
	}
	return aNext, bNext, true
}

// rebase carries the undo and redo history through changes no step
// records, such as a merge from a sync server, so undo still takes back
// the edits around them and restores selections where the user left
// them. Each stack is walked from the step adjacent to the current
// document; a step that overlaps the carried changes drops itself and
// every deeper step whose coordinates depend on it.
func (b *buffer) rebase(changes []change) {
	if len(changes) == 0 {
		return
	}
	b.undo = rebaseSteps(b.undo, changes, true)
	b.redo = rebaseSteps(b.redo, changes, false)
	b.open = false
}

// rebaseSteps transforms an undo or redo stack step by step from top
// (adjacent to the current document) to bottom, carrying changes across
// each step's coordinate space.
func rebaseSteps(steps []undoStep, changes []change, undo bool) []undoStep {
	carried := changes
	keep := 0
	for i := len(steps) - 1; i >= 0; i-- {
		step := &steps[i]
		if undo {
			invNext, prevCarried, ok := transformChanges(invertChanges(step.changes), carried)
			if !ok {
				keep = i + 1
				break
			}
			step.changes = invertChanges(invNext)
			step.after = rebaseSels(carried, step.after)
			step.before = rebaseSels(prevCarried, step.before)
			carried = prevCarried
		} else {
			nextChanges, nextCarried, ok := transformChanges(step.changes, carried)
			if !ok {
				keep = i + 1
				break
			}
			step.changes = nextChanges
			step.before = rebaseSels(carried, step.before)
			step.after = rebaseSels(nextCarried, step.after)
			carried = nextCarried
		}
	}
	if keep > 0 {
		n := copy(steps, steps[keep:])
		clear(steps[n:])
		steps = steps[:n]
	}
	return steps
}

// rebaseSels carries selections through changes.
func rebaseSels(changes []change, sels []selection) []selection {
	if len(sels) == 0 {
		return sels
	}
	out := make([]selection, len(sels))
	for i, sel := range sels {
		out[i] = selection{anchor: mapThrough(changes, sel.anchor, false), head: mapThrough(changes, sel.head, false), goal: -1}
	}
	return out
}

// merges reports whether changes go on where step left off: one change
// each, typing on from its end (a white space after a word starts a new
// step), or deleting back into it.
func (b *buffer) merges(step undoStep, changes []change, kind editKind) bool {
	if len(changes) != 1 {
		return false
	}
	last, c := step.changes[len(step.changes)-1], changes[0]
	if kind == kindTyping {
		if c.from != last.newEnd {
			return false
		}
		prev, _ := utf8.DecodeLastRuneInString(last.text)
		next, _ := utf8.DecodeRuneInString(c.text)
		return !(isSpace(next) && !isSpace(prev))
	}
	return c.oldEnd == last.from || c.from == last.from
}

func isSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\n' }

// clearHistory drops every step of the undo and redo stacks.
func (b *buffer) clearHistory() {
	b.undo = nil
	b.redo = nil
	b.open = false
}

// takeUndo undoes the last step: the inverse changes, applied, and the
// selections it had.
func (b *buffer) takeUndo() ([]change, []selection, bool) {
	n := len(b.undo)
	if n == 0 {
		return nil, nil, false
	}
	step := b.undo[n-1]
	b.undo = b.undo[:n-1]
	out := b.invert(step.changes)
	b.redo = append(b.redo, step)
	b.open = false
	return out, step.before, true
}

// takeRedo does the last undone step again.
func (b *buffer) takeRedo() ([]change, []selection, bool) {
	n := len(b.redo)
	if n == 0 {
		return nil, nil, false
	}
	step := b.redo[n-1]
	b.redo = b.redo[:n-1]
	out := make([]change, 0, len(step.changes))
	for _, c := range step.changes {
		out = append(out, b.replace(c.from, c.oldEnd, c.text))
	}
	b.version++
	b.undo = append(b.undo, step)
	b.open = false
	return out, step.after, true
}

// invert takes changes back, the last applied first.
func (b *buffer) invert(changes []change) []change {
	out := make([]change, 0, len(changes))
	for i := len(changes) - 1; i >= 0; i-- {
		c := changes[i]
		out = append(out, b.replace(c.from, c.newEnd, c.old))
	}
	b.version++
	return out
}

// utf16Col is byte column col of line as UTF-16 units.
func utf16Col(line string, col int) int {
	n := 0
	for _, r := range line[:min(col, len(line))] {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// byteCol is UTF-16 column u of line as bytes, clamped to the line.
func byteCol(line string, u int) int {
	n := 0
	for i, r := range line {
		if n >= u {
			return i
		}
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return len(line)
}
