package diff

import (
	"slices"
	"strings"
	"testing"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/nsgotest"
)

const oldGo = `package main

import "fmt"

func main() {
	fmt.Println("hello")
	x := 1
	fmt.Println(x)
}
`

const newGo = `package main

import "fmt"

func main() {
	fmt.Println("hello, world")
	y := 2
	fmt.Println(y)
	fmt.Println("done")
}
`

func TestModelPairsAlikeLinesAndMarksWords(t *testing.T) {
	m := newModel(oldGo, newGo)
	if len(m.hunks) != 1 {
		t.Fatalf("%d hunks, want one", len(m.hunks))
	}
	h := m.hunks[0]
	if h.oldEnd-h.oldStart != 3 || h.newEnd-h.newStart != 4 || len(h.pairs) != 3 {
		t.Fatalf("hunk %+v", h)
	}
	old, new := inline(m.old[h.pairs[0][0]], m.new[h.pairs[0][1]], InlineWordAlt)
	if len(old) != 0 || len(new) != 1 || m.new[h.pairs[0][1]][new[0][0]:new[0][1]] != ", world" {
		t.Fatalf("inline %v %v", old, new)
	}
	o, n := m.revert(h)
	if o != oldGo || n != oldGo {
		t.Fatalf("revert gave %q", n)
	}
	o, _ = m.accept(h)
	if o != newGo {
		t.Fatalf("accept gave %q", o)
	}
	// A missing final line break is a change.
	if m := newModel("a\nb\n", "a\nb"); len(m.hunks) != 1 {
		t.Fatalf("final line break: %d hunks", len(m.hunks))
	}
}

const patch = `From 1234 Mon Sep 17 00:00:00 2001
Subject: [PATCH] greet

diff --git a/main.go b/main.go
index 111..222 100644
--- a/main.go
+++ b/main.go
@@ -4,4 +4,4 @@ import "fmt"

 func main() {
-	fmt.Println("hello")
+	fmt.Println("hello, world")
 	x := 1
diff --git a/old.txt b/new.txt
similarity index 90%
rename from old.txt
rename to new.txt
--- a/old.txt
+++ b/new.txt
@@ -1 +1,2 @@
 one
+two
\ No newline at end of file
`

func TestPatchFilesAndTexts(t *testing.T) {
	files, err := parsePatch(patch)
	if err != nil || len(files) != 2 {
		t.Fatalf("files %d, err %v", len(files), err)
	}
	if f := files[1]; f.status != FileRenamed || f.oldPath != "old.txt" || f.path != "new.txt" {
		t.Fatalf("rename: %+v", f)
	}
	old, new, on, nn := files[0].texts()
	if !strings.Contains(old, `"hello")`) || !strings.Contains(new, "hello, world") || on[0] != 4 || nn[0] != 4 {
		t.Fatalf("texts %q %q %v %v", old, new, on, nn)
	}
	_, new, _, _ = files[1].texts()
	if new != "one\ntwo" {
		t.Fatalf("no final line break: %q", new)
	}
}

func TestDiffViewShowsChangesAndSeparators(t *testing.T) {
	var stats Stats
	ref := &Ref{}
	var lines strings.Builder
	for i := range 40 {
		lines.WriteString("line " + string(rune('a'+i%26)) + "\n")
	}
	oldText := lines.String()
	newText := strings.Replace(oldText, "line t\n", "line T\n", 1)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return DiffView{
			Ref: ref, OldValue: func() string { return oldText }, NewValue: func() string { return newText },
			OnStatsChange: func(s Stats) { stats = s },
			OnChange:      func(o, n string) {},
			Style:         core.Style{Width: core.Pt(600), Height: core.Pt(400)},
		}
	})
	if stats != (Stats{Added: 1, Deleted: 1, Hunks: 1}) {
		t.Fatalf("stats %+v", stats)
	}
	app.Find("text:⋯ 16 unchanged lines")
	app.Find("text:⋯ 17 unchanged lines")
	app.Find("text:line T")
	app.Press("text:⋯ 16 unchanged lines")
	if app.Exists("text:⋯ 16 unchanged lines") {
		t.Error("a pressed separator still hides its lines")
	}
	if h := ref.Hunks(); len(h) != 1 || h[0].OldStart != 20 {
		t.Errorf("hunks %+v", h)
	}
	ref.RevertHunk(0)
	app.Settle()
	if ref.Stats().Hunks != 0 {
		t.Errorf("reverted: %+v", ref.Stats())
	}
}

func TestDiffViewShowsAPatchByFile(t *testing.T) {
	var toggled []File
	ref := &Ref{}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return DiffView{Ref: ref, Patch: func() string { return patch }, Unified: true,
			OnFileToggle: func(f File) { toggled = append(toggled, f) },
			Style:        core.Style{Width: core.Pt(600), Height: core.Pt(400)}}
	})
	app.Find(`text:import "fmt"`)
	app.Find("text:old.txt → new.txt")
	files := ref.Files()
	if len(files) != 2 || files[0].Added != 1 || files[1].Added != 1 {
		t.Fatalf("files %+v", files)
	}
	app.Press("label:main.go")
	if len(toggled) != 1 || !toggled[0].Collapsed || app.Exists("text:hello, world") {
		t.Fatalf("collapse: %+v", toggled)
	}
	if !slices.ContainsFunc(app.Texts(), func(s string) bool { return s == "two" }) {
		t.Error("the other file's lines are gone")
	}
}

func TestDiffViewSelectsLinesAndAnnotates(t *testing.T) {
	var selections []*LineRange
	ref := &Ref{}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		notes := core.NewSignalFunc(cx, []Annotation(nil), slices.Equal)
		return DiffView{
			Ref: ref, OldValue: func() string { return oldGo }, NewValue: func() string { return newGo },
			Annotations:           notes.Get,
			OnLineSelectionChange: func(r *LineRange) { selections = append(selections, r) },
			RenderAnnotation: func(cx *core.Scope, a Annotation) core.Node {
				return core.Text{Content: "Note on " + a.ID, Style: core.Style{Height: core.Pt(30)}}
			},
			RenderAddAnnotation: func(cx *core.Scope, lines LineRange) core.Node {
				return core.View{Role: core.RoleButton, Label: "Add note", Style: core.Style{Width: core.Pt(12), Height: core.Pt(12)}, OnPress: func(core.PressEvent) {
					notes.Set(append(notes.Get(), Annotation{ID: "n1", Side: lines.Side, Line: lines.End}))
				}}
			},
			Style: core.Style{Width: core.Pt(800), Height: core.Pt(400)},
		}
	})
	// The new side's number 7 is `y := 2`.
	app.Press("text:7 >> nth=1")
	if len(selections) != 1 || *selections[0] != (LineRange{Side: New, Start: 6, End: 6}) {
		t.Fatalf("selections %+v", selections)
	}
	ref.v.selectLine(New, 7, true)
	app.Settle()
	if got := ref.SelectedText(); got != "\ty := 2\n\tfmt.Println(y)" {
		t.Fatalf("selected %q", got)
	}
	top := func() float32 { return ref.v.tops[ref.v.rowOfLine(New, 8)] }
	below := top()
	app.Press("label:Add note")
	app.Find("text:Note on n1")
	if moved := top(); moved != below+30 {
		t.Errorf("the row under the note at %v, was %v", moved, below)
	}
	ref.SetSelectedLines(nil)
	if _, ok := ref.SelectedLines(); ok || len(selections) != 2 {
		t.Errorf("cleared silently: %+v", selections)
	}
}

func TestDiffViewWrapsLongLines(t *testing.T) {
	long := strings.Repeat("word ", 40)
	ref := &Ref{}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return DiffView{Ref: ref, OldValue: func() string { return "a\n" }, NewValue: func() string { return "a\n" + long + "\nb\n" },
			SoftWrap: true, Unified: true, Style: core.Style{Width: core.Pt(400), Height: core.Pt(400)}}
	})
	if n := len(app.Query("text:word word")); n < 3 {
		t.Errorf("the long line in %d rows", n)
	}
	if y := ref.v.tops[ref.v.rowOfLine(New, 2)]; y < 80 {
		t.Errorf("the line after the wrapped one at %v", y)
	}
}

func TestWrapStartsBreakAfterSpaces(t *testing.T) {
	if got := wrapStarts("aaa bbb ccc", 5); !slices.Equal(got, []int{0, 4, 8}) {
		t.Errorf("starts %v", got)
	}
	if got := wrapStarts("abcdefghij", 4); !slices.Equal(got, []int{0, 4, 8}) {
		t.Errorf("hard breaks %v", got)
	}
}
