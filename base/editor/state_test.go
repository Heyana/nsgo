package editor

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRopeSplicesAcrossChunks(t *testing.T) {
	var lines []string
	for i := range 2000 {
		lines = append(lines, fmt.Sprint("line ", i))
	}
	r := newRope(strings.Join(lines, "\n"))
	if r.lines() != 2000 || r.line(1500) != "line 1500" {
		t.Fatalf("lines %d, line 1500 %q", r.lines(), r.line(1500))
	}
	r.splice(500, 1600, []string{"a", "b"})
	if r.lines() != 902 || r.line(500) != "a" || r.line(502) != "line 1600" || r.line(499) != "line 499" {
		t.Fatalf("after splice: %d lines, %q %q", r.lines(), r.line(500), r.line(502))
	}
	r.splice(0, 902, []string{""})
	if r.lines() != 1 || r.text() != "" {
		t.Fatalf("emptied: %d lines %q", r.lines(), r.text())
	}
}

func goState(text string) *state {
	s := newState(text)
	s.lang = languageOf("go")
	s.tab, s.hard = 4, true
	return s
}

func TestTypingClosesAndIndents(t *testing.T) {
	s := goState("func f() ")
	s.setSels([]selection{caret(pos{0, 9})})
	s.insert("{", true)
	if got := s.r().text(); got != "func f() {}" || s.primary().head != (pos{0, 10}) {
		t.Fatalf("typed { : %q, caret %v", got, s.primary().head)
	}
	s.newline()
	if got := s.r().text(); got != "func f() {\n\t\n}" || s.primary().head != (pos{1, 1}) {
		t.Fatalf("newline in the pair: %q, caret %v", got, s.primary().head)
	}
	s.insert("x", true)
	s.insert("(", true)
	s.insert(")", true)
	if got := s.line(1); got != "\tx()" {
		t.Fatalf("stepped over ): %q", got)
	}
	// Typing merged into one step.
	s.buf.open = true
	if _, _, ok := s.buf.takeUndo(); !ok {
		t.Fatal("nothing to undo")
	}
}

func TestUndoGroupsTypingAndRestoresSelections(t *testing.T) {
	s := goState("")
	for _, c := range "hello world" {
		s.insert(string(c), true)
	}
	if n := len(s.buf.undo); n != 2 {
		t.Fatalf("%d undo steps, want the word and the space with the next word", n)
	}
	changes, sels, _ := s.buf.takeUndo()
	s.afterEdit(changes)
	s.setSels(sels)
	if s.r().text() != "hello" || s.primary().head != (pos{0, 5}) {
		t.Fatalf("undo: %q at %v", s.r().text(), s.primary().head)
	}
	changes, sels, _ = s.buf.takeRedo()
	s.afterEdit(changes)
	if s.r().text() != "hello world" {
		t.Fatalf("redo: %q", s.r().text())
	}
}

func TestBackspaceDeletesPairsAndIndentation(t *testing.T) {
	s := newState("    x()")
	s.lang = languageOf("ts")
	s.tab = 4
	s.setSels([]selection{caret(pos{0, 6})})
	s.deleteBy(s.prevChar, true)
	if got := s.r().text(); got != "    x" {
		t.Fatalf("backspace in a pair: %q", got)
	}
	s.setSels([]selection{caret(pos{0, 4})})
	s.deleteBy(s.prevChar, true)
	if got := s.r().text(); got != "x" {
		t.Fatalf("backspace in the indentation: %q", got)
	}
}

func TestSeveralCaretsEditTogether(t *testing.T) {
	s := goState("a\nb\nc")
	s.setSels([]selection{caret(pos{0, 1}), caret(pos{1, 1}), caret(pos{2, 1})})
	s.insert("!", false)
	if got := s.r().text(); got != "a!\nb!\nc!" || len(s.sels) != 3 {
		t.Fatalf("%q with %d carets", got, len(s.sels))
	}
	s.toggleComment()
	if got := s.r().text(); got != "// a!\n// b!\n// c!" {
		t.Fatalf("commented: %q", got)
	}
	s.toggleComment()
	s.setSels([]selection{caret(pos{0, 0})})
	s.moveLines(1)
	if got := s.r().text(); got != "b!\na!\nc!" || s.primary().head.line != 1 {
		t.Fatalf("moved down: %q", got)
	}
	s.selectAllOccurrences()
	if len(s.sels) != 1 {
		t.Fatalf("occurrences of b!: %d", len(s.sels))
	}
}

func TestFoldsByIndentationHideRows(t *testing.T) {
	s := goState("func a() {\n\tx\n\n\ty\n}\nfunc b() {\n\tz\n}")
	folds := s.foldCandidates()
	if len(folds) != 2 || folds[0] != (foldRange{0, 3}) || folds[1] != (foldRange{6, 6}) && folds[1] != (foldRange{5, 6}) {
		t.Fatalf("folds %v", folds)
	}
	s.folded = []foldRange{folds[0]}
	s.relayout()
	if s.disp.count() != 5 || s.disp.row(1, s.r()).line != 4 {
		t.Fatalf("%d rows, row 1 is line %d", s.disp.count(), s.disp.row(1, s.r()).line)
	}
	// A caret moved into it unfolds it.
	s.setSels([]selection{caret(pos{2, 0})})
	if len(s.folded) != 0 {
		t.Fatalf("still folded: %v", s.folded)
	}
}

func TestSoftWrapBreaksAfterSpaces(t *testing.T) {
	rows := wrapLine("aaaa bbbb cccc", 10, 4)
	if len(rows) != 2 || rows[0] != [2]int{0, 10} || rows[1] != [2]int{10, 14} {
		t.Fatalf("rows %v", rows)
	}
	if got := colAt("\tx中", len("\tx中"), 4); got != 7 {
		t.Fatalf("columns %d, want a tab's 4, x's 1 and 中's 2", got)
	}
}

func TestReplaceTextKeepsWhatStays(t *testing.T) {
	s := goState("one\ntwo\nthree")
	s.setSels([]selection{caret(pos{2, 3})})
	s.replaceText("one\n2\nthree", true)
	if s.primary().head != (pos{2, 3}) || s.r().text() != "one\n2\nthree" {
		t.Fatalf("%q, caret %v", s.r().text(), s.primary().head)
	}
}

func TestExpandGrowsByUnits(t *testing.T) {
	s := goState("func f() {\n\tcall(\"a word\", b)\n}")
	s.setSels([]selection{caret(pos{1, 9})}) // in "word"
	sel := func() string { p := s.primary(); return s.r().slice(p.start(), p.end()) }
	want := []string{"word", "a word", `"a word"`, `"a word", b`, `("a word", b)`, `call("a word", b)`,
		"\tcall(\"a word\", b)\n", "\n\tcall(\"a word\", b)\n", "{\n\tcall(\"a word\", b)\n}", "func f() {\n\tcall(\"a word\", b)\n}"}
	for i, w := range want {
		s.expand()
		if got := sel(); got != w {
			t.Fatalf("step %d: %q, want %q", i, got, w)
		}
	}
}

func TestSplitSelectionIntoLines(t *testing.T) {
	s := goState("one\ntwo\nthree")
	s.setSels([]selection{{anchor: pos{0, 1}, head: pos{2, 2}, goal: -1}})
	s.splitLines()
	if len(s.sels) != 3 || s.sels[0].span() != (span{pos{0, 1}, pos{0, 3}}) || s.sels[2].span() != (span{pos{2, 0}, pos{2, 2}}) {
		t.Fatalf("split: %+v", s.sels)
	}
}

func TestEdgeSpeedRamps(t *testing.T) {
	if edgeSpeed(200, 400) != 0 {
		t.Error("the middle scrolls")
	}
	near, far := edgeSpeed(390, 400), edgeSpeed(450, 400)
	if near < edgeMin || far <= near || edgeSpeed(2000, 400) != edgeMax || edgeSpeed(-2000, 400) != -edgeMax {
		t.Errorf("speeds %v %v", near, far)
	}
}

func undoState(s *state) bool {
	changes, sels, ok := s.buf.takeUndo()
	if !ok {
		return false
	}
	s.afterEdit(changes)
	s.setSels(slices.Clone(sels))
	return true
}

func redoState(s *state) bool {
	changes, sels, ok := s.buf.takeRedo()
	if !ok {
		return false
	}
	s.afterEdit(changes)
	s.setSels(slices.Clone(sels))
	return true
}

func TestRebaseInsertionAtDeletionPoint(t *testing.T) {
	s := newState("hello")
	s.setSels([]selection{{anchor: pos{0, 2}, head: pos{0, 5}, goal: -1}})
	s.insert("", false)
	if got := s.r().text(); got != "he" {
		t.Fatalf("after delete: %q", got)
	}
	s.replaceText("he12345", false)
	if got := s.r().text(); got != "he12345" {
		t.Fatalf("after merge: %q", got)
	}
	if !undoState(s) {
		t.Fatal("undo returned false")
	}
	if got := s.r().text(); got != "hello12345" {
		t.Fatalf("after undo: %q, want %q", got, "hello12345")
	}
	if !redoState(s) {
		t.Fatal("redo returned false")
	}
	if got := s.r().text(); got != "he12345" {
		t.Fatalf("after redo: %q, want %q", got, "he12345")
	}
}

func TestRebaseMultiStepMultilineEdits(t *testing.T) {
	s := newState("alpha\nbeta\ngamma\ndelta")
	// Step 1: edit line 1 ("beta" -> "BETA").
	s.setSels([]selection{{anchor: pos{1, 0}, head: pos{1, 4}, goal: -1}})
	s.insert("BETA", false)
	s.buf.open = false
	// Step 2: insert 3 lines above line 1 (at line 0 start).
	s.setSels([]selection{caret(pos{0, 0})})
	s.insert("x1\nx2\nx3\n", false)
	s.buf.open = false
	if got := s.r().text(); got != "x1\nx2\nx3\nalpha\nBETA\ngamma\ndelta" {
		t.Fatalf("after step 2: %q", got)
	}

	// External merge adds 2 lines at the top and edits the last line.
	s.replaceText("m1\nm2\nx1\nx2\nx3\nalpha\nBETA\ngamma\ndelta", false)
	s.replaceText("m1\nm2\nx1\nx2\nx3\nalpha\nBETA\ngamma\nDELTA!", false)

	// Undo Step 2 (removes x1..x3).
	if !undoState(s) {
		t.Fatal("undo step 2 returned false")
	}
	if got := s.r().text(); got != "m1\nm2\nalpha\nBETA\ngamma\nDELTA!" {
		t.Fatalf("after undo step 2: %q", got)
	}
	// Undo Step 1 (restores beta).
	if !undoState(s) {
		t.Fatal("undo step 1 returned false")
	}
	if got := s.r().text(); got != "m1\nm2\nalpha\nbeta\ngamma\nDELTA!" {
		t.Fatalf("after undo step 1: %q", got)
	}
	// Redo Step 1 (restores BETA).
	if !redoState(s) {
		t.Fatal("redo step 1 returned false")
	}
	if got := s.r().text(); got != "m1\nm2\nalpha\nBETA\ngamma\nDELTA!" {
		t.Fatalf("after redo step 1: %q", got)
	}
	// Redo Step 2 (restores x1..x3).
	if !redoState(s) {
		t.Fatal("redo step 2 returned false")
	}
	if got := s.r().text(); got != "m1\nm2\nx1\nx2\nx3\nalpha\nBETA\ngamma\nDELTA!" {
		t.Fatalf("after redo step 2: %q", got)
	}

	// Now test deleting lines above in Step 2 followed by an external line deletion.
	s2 := newState("l0\nl1\nl2\nl3\nl4\nl5")
	s2.setSels([]selection{caret(pos{4, 2})})
	s2.insert("!", false) // line 4 -> "l4!"
	s2.buf.open = false
	s2.setSels([]selection{{anchor: pos{0, 0}, head: pos{3, 0}, goal: -1}})
	s2.insert("", false) // deletes l0\nl1\nl2\n -> "l3\nl4!\nl5"
	s2.buf.open = false
	// External merge deletes "l5" line.
	s2.replaceText("l3\nl4!", false)
	if !undoState(s2) {
		t.Fatal("s2 undo step 2 failed")
	}
	if got := s2.r().text(); got != "l0\nl1\nl2\nl3\nl4!" {
		t.Fatalf("s2 after undo step 2: %q", got)
	}
	if !undoState(s2) {
		t.Fatal("s2 undo step 1 failed")
	}
	if got := s2.r().text(); got != "l0\nl1\nl2\nl3\nl4" {
		t.Fatalf("s2 after undo step 1: %q", got)
	}
	if !redoState(s2) || s2.r().text() != "l0\nl1\nl2\nl3\nl4!" {
		t.Fatalf("s2 after redo step 1: %q", s2.r().text())
	}
	if !redoState(s2) || s2.r().text() != "l3\nl4!" {
		t.Fatalf("s2 after redo step 2: %q", s2.r().text())
	}
}

func TestRebaseOverlappingAndFullReplacement(t *testing.T) {
	// Partial overlap: earlier step overlaps external edit, later step is disjoint.
	s := newState("one\ntwo\nthree")
	s.setSels([]selection{{anchor: pos{0, 0}, head: pos{0, 3}, goal: -1}})
	s.insert("ONE", false)
	s.buf.open = false
	s.setSels([]selection{{anchor: pos{2, 0}, head: pos{2, 5}, goal: -1}})
	s.insert("THREE", false)
	s.buf.open = false

	// External overwrites line 0 ("ONE" -> "1"): overlaps Step 1, disjoint from Step 2.
	s.replaceText("1\ntwo\nTHREE", false)
	if !undoState(s) {
		t.Fatal("expected disjoint top step to remain undoable")
	}
	if got := s.r().text(); got != "1\ntwo\nthree" {
		t.Fatalf("after undo top step: %q", got)
	}
	if undoState(s) {
		t.Fatalf("expected overlapping earlier step to be dropped, left %q", s.r().text())
	}
	if !redoState(s) || s.r().text() != "1\ntwo\nTHREE" {
		t.Fatalf("redo after partial truncation: %q", s.r().text())
	}

	// Full document replacement drops all conflicting history cleanly.
	s.replaceText("brand new content", false)
	for range 5 {
		undoState(s)
	}
	if got := s.r().text(); got != "brand new content" {
		t.Fatalf("undo after full replacement altered text: %q", got)
	}
	for range 5 {
		redoState(s)
	}
	if got := s.r().text(); got != "brand new content" {
		t.Fatalf("redo after full replacement altered text: %q", got)
	}
}

func TestRebasePreservesAndTruncatesRedo(t *testing.T) {
	s := newState("one\ntwo\nthree")
	s.setSels([]selection{caret(pos{0, 3})})
	s.insert("!", false)
	s.buf.open = false
	s.setSels([]selection{{anchor: pos{2, 1}, head: pos{2, 3}, goal: -1}})
	s.insert("HR", false)
	s.buf.open = false

	// Undo both steps so redo has 2 steps: [line2 "hr"->"HR", line0 "!"].
	if !undoState(s) || !undoState(s) {
		t.Fatal("undo setup failed")
	}
	if got := s.r().text(); got != "one\ntwo\nthree" {
		t.Fatalf("before merge: %q", got)
	}

	// Disjoint external merge inserts a line in the middle.
	s.replaceText("one\nMIDDLE\ntwo\nthree", false)
	if !redoState(s) || s.r().text() != "one!\nMIDDLE\ntwo\nthree" {
		t.Fatalf("redo 1: %q", s.r().text())
	}
	if !redoState(s) || s.r().text() != "one!\nMIDDLE\ntwo\ntHRee" {
		t.Fatalf("redo 2: %q", s.r().text())
	}
	if !undoState(s) || s.r().text() != "one!\nMIDDLE\ntwo\nthree" {
		t.Fatalf("undo 1: %q", s.r().text())
	}
	if !undoState(s) || s.r().text() != "one\nMIDDLE\ntwo\nthree" {
		t.Fatalf("undo 2: %q", s.r().text())
	}

	// Now external merge overwrites line 3 ("three" -> "THREE"), conflicting
	// with the deeper redo step while leaving the top redo step ("one" -> "one!") disjoint.
	s.replaceText("one\nMIDDLE\ntwo\nTHREE", false)
	if !redoState(s) || s.r().text() != "one!\nMIDDLE\ntwo\nTHREE" {
		t.Fatalf("redo disjoint top after partial conflict: %q", s.r().text())
	}
	if redoState(s) {
		t.Fatalf("expected conflicting deeper redo step to be dropped, left %q", s.r().text())
	}
	if !undoState(s) || s.r().text() != "one\nMIDDLE\ntwo\nTHREE" {
		t.Fatalf("undo after partial redo truncation: %q", s.r().text())
	}
}

func TestRebaseMultiCursorAndMergedRuns(t *testing.T) {
	// Multi-cursor newline insertion + external edit in the middle.
	s := newState("a\nb\nc")
	s.setSels([]selection{caret(pos{0, 1}), caret(pos{1, 1}), caret(pos{2, 1})})
	s.insert("\n!", false)
	if got := s.r().text(); got != "a\n!\nb\n!\nc\n!" {
		t.Fatalf("multi-cursor insert: %q", got)
	}
	// External inserts a prefix on line 4 ("c" -> "Zc").
	s.replaceText("a\n!\nb\n!\nZc\n!", false)
	if !undoState(s) || s.r().text() != "a\nb\nZc" {
		t.Fatalf("undo multi-cursor: %q", s.r().text())
	}
	if !redoState(s) || s.r().text() != "a\n!\nb\n!\nZc\n!" {
		t.Fatalf("redo multi-cursor: %q", s.r().text())
	}

	// Merged typing run + external insertion between typed characters.
	s2 := newState("foo")
	s2.setSels([]selection{caret(pos{0, 3})})
	for _, ch := range "abc" {
		s2.insert(string(ch), true)
	}
	if len(s2.buf.undo) != 1 || len(s2.buf.undo[0].changes) != 3 {
		t.Fatalf("expected 1 merged typing step with 3 changes, got %d steps", len(s2.buf.undo))
	}
	s2.replaceText("fooaXYZbc", false)
	if !undoState(s2) || s2.r().text() != "fooXYZ" {
		t.Fatalf("undo merged run: %q", s2.r().text())
	}
	if !redoState(s2) || s2.r().text() != "fooaXYZbc" {
		t.Fatalf("redo merged run: %q", s2.r().text())
	}
}

// verifyBufferInvariants checks rope validity, selection bounds, and
// verifies on a cloned buffer that every step in undo and redo applies
// cleanly with exact slice matches and round-trips back to the current text.
func verifyBufferInvariants(t *testing.T, s *state, ctx string) {
	t.Helper()
	r := s.r()
	if r.lines() < 1 {
		t.Fatalf("%s: rope has %d lines", ctx, r.lines())
	}
	orig := r.text()
	if !utf8.ValidString(orig) {
		t.Fatalf("%s: invalid UTF-8 in rope", ctx)
	}
	checkPos := func(ro *rope, p pos, label string) {
		if p.line < 0 || p.line >= ro.lines() {
			t.Fatalf("%s: %s line %d out of [0,%d)", ctx, label, p.line, ro.lines())
		}
		l := ro.line(p.line)
		if p.col < 0 || p.col > len(l) || (p.col < len(l) && !startsRune(l[p.col])) {
			t.Fatalf("%s: %s col %d invalid for line %q", ctx, label, p.col, l)
		}
	}
	for _, sel := range s.sels {
		checkPos(r, sel.anchor, "sel.anchor")
		checkPos(r, sel.head, "sel.head")
	}

	cloneBuf := func() *buffer {
		cp := newBuffer(orig)
		cp.undo = make([]undoStep, len(s.buf.undo))
		for i, st := range s.buf.undo {
			cp.undo[i] = undoStep{
				changes: slices.Clone(st.changes),
				before:  slices.Clone(st.before),
				after:   slices.Clone(st.after),
				kind:    st.kind,
			}
		}
		cp.redo = make([]undoStep, len(s.buf.redo))
		for i, st := range s.buf.redo {
			cp.redo[i] = undoStep{
				changes: slices.Clone(st.changes),
				before:  slices.Clone(st.before),
				after:   slices.Clone(st.after),
				kind:    st.kind,
			}
		}
		return cp
	}

	// Verify full undo drain and redo round-trip.
	ub := cloneBuf()
	nUndo := len(ub.undo)
	for step := range nUndo {
		st := ub.undo[len(ub.undo)-1]
		for i := len(st.changes) - 1; i >= 0; i-- {
			c := st.changes[i]
			checkPos(ub.r, c.from, "undo.from")
			checkPos(ub.r, c.newEnd, "undo.newEnd")
			if c.newEnd != advance(c.from, c.text) || c.oldEnd != advance(c.from, c.old) {
				t.Fatalf("%s: undo step %d change %d drifted endpoints: %+v", ctx, step, i, c)
			}
			if got := ub.r.slice(c.from, c.newEnd); got != c.text {
				t.Fatalf("%s: undo step %d change %d slice %q != text %q", ctx, step, i, got, c.text)
			}
			ub.replace(c.from, c.newEnd, c.old)
		}
		ub.undo = ub.undo[:len(ub.undo)-1]
		ub.redo = append(ub.redo, st)
	}
	for step := range nUndo {
		st := ub.redo[len(ub.redo)-1]
		for i, c := range st.changes {
			checkPos(ub.r, c.from, "undo-redo.from")
			checkPos(ub.r, c.oldEnd, "undo-redo.oldEnd")
			if got := ub.r.slice(c.from, c.oldEnd); got != c.old {
				t.Fatalf("%s: undo-redo step %d change %d slice %q != old %q", ctx, step, i, got, c.old)
			}
			ub.replace(c.from, c.oldEnd, c.text)
		}
		ub.redo = ub.redo[:len(ub.redo)-1]
	}
	if ub.r.text() != orig {
		t.Fatalf("%s: undo->redo round-trip mismatch: got %q, want %q", ctx, ub.r.text(), orig)
	}

	// Verify full redo drain and undo round-trip.
	rb := cloneBuf()
	nRedo := len(rb.redo)
	for step := range nRedo {
		st := rb.redo[len(rb.redo)-1]
		for i, c := range st.changes {
			checkPos(rb.r, c.from, "redo.from")
			checkPos(rb.r, c.oldEnd, "redo.oldEnd")
			if c.newEnd != advance(c.from, c.text) || c.oldEnd != advance(c.from, c.old) {
				t.Fatalf("%s: redo step %d change %d drifted endpoints: %+v", ctx, step, i, c)
			}
			if got := rb.r.slice(c.from, c.oldEnd); got != c.old {
				t.Fatalf("%s: redo step %d change %d slice %q != old %q", ctx, step, i, got, c.old)
			}
			rb.replace(c.from, c.oldEnd, c.text)
		}
		rb.redo = rb.redo[:len(rb.redo)-1]
		rb.undo = append(rb.undo, st)
	}
	for step := range nRedo {
		st := rb.undo[len(rb.undo)-1]
		for i := len(st.changes) - 1; i >= 0; i-- {
			c := st.changes[i]
			checkPos(rb.r, c.from, "redo-undo.from")
			checkPos(rb.r, c.newEnd, "redo-undo.newEnd")
			if got := rb.r.slice(c.from, c.newEnd); got != c.text {
				t.Fatalf("%s: redo-undo step %d change %d slice %q != text %q", ctx, step, i, got, c.text)
			}
			rb.replace(c.from, c.newEnd, c.old)
		}
		rb.undo = rb.undo[:len(rb.undo)-1]
	}
	if rb.r.text() != orig {
		t.Fatalf("%s: redo->undo round-trip mismatch: got %q, want %q", ctx, rb.r.text(), orig)
	}
}

func TestRebaseRandomizedHistoryInvariants(t *testing.T) {
	rng := rand.New(rand.NewPCG(2518, 42))
	snippets := []string{"a", "bc", " ", "\n", "x\ny", "123\n456\n", "é", "中\n文", ""}

	randPos := func(r *rope) pos {
		line := rng.IntN(r.lines())
		l := r.line(line)
		var cols []int
		for i := range l {
			if startsRune(l[i]) {
				cols = append(cols, i)
			}
		}
		cols = append(cols, len(l))
		return pos{line, cols[rng.IntN(len(cols))]}
	}

	for seq := range 250 {
		s := newState("alpha\nbeta\ngamma\ndelta")
		for op := range 40 {
			switch rng.IntN(9) {
			case 0, 1: // typed insert (may merge)
				if rng.IntN(2) == 0 {
					s.setSels([]selection{caret(randPos(s.r()))})
					s.buf.open = false
				}
				s.insert(snippets[rng.IntN(len(snippets))], true)
			case 2: // replace selection
				a, b := randPos(s.r()), randPos(s.r())
				s.setSels([]selection{{anchor: a, head: b, goal: -1}})
				s.insert(snippets[rng.IntN(len(snippets))], false)
			case 3: // backspace/delete
				if rng.IntN(2) == 0 {
					s.setSels([]selection{caret(randPos(s.r()))})
					s.buf.open = false
				}
				if rng.IntN(2) == 0 {
					s.deleteBy(s.prevChar, true)
				} else {
					s.deleteBy(s.nextChar, true)
				}
			case 4: // newline
				s.setSels([]selection{caret(randPos(s.r()))})
				s.newline()
			case 5: // multi-cursor edit
				n := 2 + rng.IntN(3)
				sels := make([]selection, n)
				for i := range sels {
					if rng.IntN(2) == 0 {
						sels[i] = caret(randPos(s.r()))
					} else {
						sels[i] = selection{anchor: randPos(s.r()), head: randPos(s.r()), goal: -1}
					}
				}
				s.setSels(sels)
				s.insert(snippets[rng.IntN(len(snippets))], false)
			case 6: // external edit outside history (replaceText(..., false))
				cur := s.r().text()
				var next string
				switch rng.IntN(4) {
				case 0: // splice random substring
					var offs []int
					for i := range cur {
						if startsRune(cur[i]) {
							offs = append(offs, i)
						}
					}
					offs = append(offs, len(cur))
					i, j := offs[rng.IntN(len(offs))], offs[rng.IntN(len(offs))]
					if j < i {
						i, j = j, i
					}
					next = cur[:i] + snippets[rng.IntN(len(snippets))] + cur[j:]
				case 1: // prepend line
					next = "EXT\n" + cur
				case 2: // append line
					next = cur + "\nEXT"
				case 3: // full replacement
					next = "replaced\n" + snippets[rng.IntN(len(snippets))]
				}
				s.replaceText(next, false)
			case 7: // undo
				undoState(s)
			case 8: // redo
				redoState(s)
			}
			verifyBufferInvariants(t, s, fmt.Sprintf("seq=%d op=%d", seq, op))
		}
	}
}
