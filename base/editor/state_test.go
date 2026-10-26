package editor

import (
	"fmt"
	"strings"
	"testing"
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
	s.replaceText("one\n2\nthree")
	if s.primary().head != (pos{2, 3}) || s.r().text() != "one\n2\nthree" {
		t.Fatalf("%q, caret %v", s.r().text(), s.primary().head)
	}
}
