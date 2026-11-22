package android

import (
	"math"
	"slices"
	"testing"
	"unicode/utf16"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const (
	selectionWord byte = iota
	selectionStart
	selectionEnd
	selectionAll
	selectionClear
	selectionOuterScroll
	selectionInnerShift
	selectionDetach
)

type selectionReport struct {
	text string
	box  driver.Bounds
}

type selectionStep struct {
	name string
	op   byte
	x, y driver.Points
}

func readSelectionReports(r *reader) []selectionReport {
	n := int(r.u32())
	out := make([]selectionReport, n)
	for i := range n {
		out[i] = selectionReport{
			text: r.str(),
			box: driver.Bounds{
				Origin: driver.Point{X: driver.Points(int32(r.u32())), Y: driver.Points(int32(r.u32()))},
				Size:   driver.Size{Width: driver.Points(int32(r.u32())), Height: driver.Points(int32(r.u32()))},
			},
		}
	}
	return out
}

func runSelectionFixture(t *testing.T, host, area driver.Point, nodes selectionFixtures, steps []selectionStep) []byte {
	t.Helper()
	var input writer
	input.floats(float32(host.X), float32(host.Y), float32(area.X), float32(area.Y))
	input.u32(uint32(nodes.Len()))
	for _, node := range nodes {
		input.str(node.content)
		input.floats(float32(node.frame.Origin.X), float32(node.frame.Origin.Y),
			float32(node.frame.Size.Width), float32(node.frame.Size.Height))
	}
	input.u32(uint32(len(steps)))
	for _, step := range steps {
		input.byte(step.op)
		input.floats(float32(step.x), float32(step.y))
	}
	return runJavaAlgorithm(t, "selection", "SelectionCheck", input.buf, "Selection", "Protocol", "TextCopy")
}

type selectionFixture struct {
	content string
	frame   driver.Bounds
}

type selectionFixtures []selectionFixture

func (f selectionFixtures) Len() int                  { return len(f) }
func (f selectionFixtures) Content(i int) string      { return f[i].content }
func (f selectionFixtures) Frame(i int) driver.Bounds { return f[i].frame }
func (f selectionFixtures) Key(i int) uint64          { return uint64(i + 1) }

func (f selectionFixtures) OffsetAt(i int, p driver.Point) int {
	// The JVM fixture lays one UTF-16 code unit in ten points.
	return int(math.Round(float64(p.X-f[i].frame.Origin.X) / 10))
}

func TestSelectionMatchesJava(t *testing.T) {
	fixture := func(content string, x, y driver.Points) selectionFixture {
		return selectionFixture{content, driver.Bounds{
			Origin: driver.Point{X: x, Y: y},
			Size:   driver.Size{Width: driver.Points(len(utf16.Encode([]rune(content))) * 10), Height: 20},
		}}
	}
	nodes := selectionFixtures{fixture("alpha beta", 0, 0), fixture("pi 🚀", 150, 0), fixture("next\uFFFC row", 0, 40)}
	steps := []selectionStep{
		{"empty", selectionClear, 0, 0},
		{"word", selectionWord, 16, 5},
		{"unchanged handle", selectionEnd, 50, 5},
		{"grow within one text", selectionEnd, 80, 5},
		{"extend into a surrogate pair", selectionEnd, 185, 5},
		{"extend across an emoji", selectionEnd, 200, 5},
		{"between rows", selectionEnd, 15, 30},
		{"extend over an attachment", selectionEnd, 85, 45},
		{"collapse at the end", selectionStart, 200, 80},
		{"word after an attachment", selectionWord, 69, 45},
		{"nearest text on a row", selectionStart, 145, 5},
		{"second word", selectionWord, 75, 5},
		{"cross the handles", selectionEnd, 13, 5},
		{"select all", selectionAll, 0, 0},
		{"select all again", selectionAll, 0, 0},
		{"clear", selectionClear, 0, 0},
		{"clear again", selectionClear, 0, 0},
		{"moving a cleared handle", selectionEnd, 50, 5},
	}
	// Place host and area at non-zero offsets so the test checks area-relative boxes.
	out := runSelectionFixture(t, driver.Point{X: 12, Y: 34}, driver.Point{X: 40, Y: 60}, nodes, steps)
	r := reader{out}
	var model driver.SelectionModel
	var told driver.Bounds
	var toldText string
	for _, step := range steps {
		point := driver.Point{X: step.x, Y: step.y}
		switch step.op {
		case selectionWord:
			model.SelectWordAt(nodes, point)
		case selectionStart:
			model.MoveHandle(nodes, driver.HandleStart, point)
		case selectionEnd:
			model.MoveHandle(nodes, driver.HandleEnd, point)
		case selectionAll:
			model.SelectAll(nodes)
		case selectionClear:
			model.Clear()
		}
		empty, copied := r.byte() != 0, r.str()
		var ranges []driver.SelectionRange
		for i := range nodes {
			from, to := int(r.u32()), int(r.u32())
			if from < to {
				ranges = append(ranges, driver.SelectionRange{Text: i, Start: from, End: to})
			}
		}
		reports := readSelectionReports(&r)
		if want := model.IsEmpty(); empty != want {
			t.Errorf("%s: Java empty %t, Go %t", step.name, empty, want)
		}
		if want := model.Text(nodes); copied != want {
			t.Errorf("%s: Java copied %q, Go %q", step.name, copied, want)
		}
		if want := model.Ranges(nodes); !slices.Equal(ranges, want) {
			t.Errorf("%s: Java ranges %v, Go %v", step.name, ranges, want)
		}
		extent := model.Extent(nodes)
		wantText := ""
		if extent.Size.Height > 0 {
			wantText = model.Text(nodes)
		}
		var wantReports []selectionReport
		if extent != told || wantText != toldText {
			told, toldText = extent, wantText
			wantReports = []selectionReport{{text: wantText, box: extent}}
		}
		if !slices.Equal(reports, wantReports) {
			t.Errorf("%s: Java reports %+v, Go %+v", step.name, reports, wantReports)
		}
	}
	if len(r.buf) != 0 {
		t.Errorf("Java wrote %d unexpected bytes", len(r.buf))
	}
}

func TestSelectionRefreshReportsChanges(t *testing.T) {
	nodes := selectionFixtures{
		{content: "first line", frame: driver.Bounds{Origin: driver.Point{X: 10, Y: 20}, Size: driver.Size{Width: 100, Height: 20}}},
		{content: "second line", frame: driver.Bounds{Origin: driver.Point{X: 10, Y: 50}, Size: driver.Size{Width: 110, Height: 20}}},
	}
	steps := []selectionStep{
		{"select first word", selectionWord, 25, 25},
		{"outer scroll keeps area-relative extent", selectionOuterScroll, 0, -30},
		{"inner shift moves extent in area", selectionInnerShift, 5, 15},
		{"detach clears and reports empty", selectionDetach, 0, 0},
	}
	wantReports := [][]selectionReport{
		{{text: "first", box: driver.Bounds{Origin: driver.Point{X: 10, Y: 20}, Size: driver.Size{Width: 100, Height: 20}}}},
		nil,
		{{text: "first", box: driver.Bounds{Origin: driver.Point{X: 15, Y: 35}, Size: driver.Size{Width: 100, Height: 20}}}},
		{{text: "", box: driver.Bounds{}}},
	}
	out := runSelectionFixture(t, driver.Point{X: 8, Y: 16}, driver.Point{X: 20, Y: 40}, nodes, steps)
	r := reader{out}
	for i, step := range steps {
		_ = r.byte()
		_ = r.str()
		for range nodes {
			_, _ = r.u32(), r.u32()
		}
		got := readSelectionReports(&r)
		if !slices.Equal(got, wantReports[i]) {
			t.Errorf("%s: Java reports %+v, want %+v", step.name, got, wantReports[i])
		}
	}
	if len(r.buf) != 0 {
		t.Errorf("Java wrote %d unexpected bytes", len(r.buf))
	}
}

func TestSelectionProcessTextToolbar(t *testing.T) {
	out := runJavaAlgorithm(t, "selection", "ProcessTextCheck", nil, "Selection", "Protocol", "TextCopy")
	r := reader{out}

	if action, mime, flags := r.str(), r.str(), r.u32(); action != "android.intent.action.PROCESS_TEXT" || mime != "text/plain" || flags != 0 {
		t.Fatalf("query intent = (%q, %q, %d), want (PROCESS_TEXT, text/plain, 0)", action, mime, flags)
	}

	const typeFloating = 1
	if modeType := r.u32(); modeType != typeFloating {
		t.Fatalf("ActionMode type = %d, want %d (TYPE_FLOATING)", modeType, typeFloating)
	}

	type menuItem struct {
		group, id, order, showAsAction uint32
		title                          string
	}
	nItems := int(r.u32())
	items := make([]menuItem, nItems)
	for i := range items {
		items[i] = menuItem{r.u32(), r.u32(), r.u32(), r.u32(), r.str()}
	}
	wantItems := []menuItem{
		{0, 1, 0, 0, "res:1"}, // android.R.id.copy
		{0, 2, 1, 0, "res:2"}, // android.R.id.selectAll
		{1, 0, 100, 1, "Translate"},
		{1, 0, 101, 1, "Dictionary"},
		{1, 0, 102, 1, "Own Action"},
	}
	if !slices.Equal(items, wantItems) {
		t.Fatalf("toolbar menu items =\n  %v\nwant\n  %v", items, wantItems)
	}

	handledTranslate := r.byte() != 0
	finishedAfterTranslate := r.byte() != 0
	handledDictionary := r.byte() != 0
	handledOwn := r.byte() != 0
	missingOk := r.byte() != 0
	deniedOk := r.byte() != 0
	if !handledTranslate || !finishedAfterTranslate || !handledDictionary || !handledOwn {
		t.Fatalf("click outcomes = (translate=%t finished=%t dict=%t own=%t), want all true",
			handledTranslate, finishedAfterTranslate, handledDictionary, handledOwn)
	}
	if missingOk || deniedOk {
		t.Fatalf("failed launches returned (missing=%t denied=%t), want false", missingOk, deniedOk)
	}

	type launchedIntent struct {
		action, mime, pkg, cls, text string
		readonly                     bool
		flags                        uint32
	}
	nLaunched := int(r.u32())
	launched := make([]launchedIntent, nLaunched)
	for i := range launched {
		launched[i] = launchedIntent{r.str(), r.str(), r.str(), r.str(), r.str(), r.byte() != 0, r.u32()}
	}
	const flagNewTask = 0x10000000
	wantLaunched := []launchedIntent{
		{
			action:   "android.intent.action.PROCESS_TEXT",
			mime:     "text/plain",
			pkg:      "com.google.android.apps.translate",
			cls:      "com.google.android.apps.translate.TranslateActivity",
			text:     "alpha",
			readonly: true,
			flags:    0,
		},
		{
			action:   "android.intent.action.PROCESS_TEXT",
			mime:     "text/plain",
			pkg:      "com.example.allowed",
			cls:      "com.example.allowed.AllowedActivity",
			text:     "alpha beta",
			readonly: true,
			flags:    0,
		},
		{
			action:   "android.intent.action.PROCESS_TEXT",
			mime:     "text/plain",
			pkg:      "dev.oneo.nsgo.test",
			cls:      "dev.oneo.nsgo.test.InternalActivity",
			text:     "gamma",
			readonly: true,
			flags:    flagNewTask,
		},
	}
	if !slices.Equal(launched, wantLaunched) {
		t.Fatalf("launched intents =\n  %+v\nwant\n  %+v", launched, wantLaunched)
	}
	if len(r.buf) != 0 {
		t.Errorf("Java wrote %d unexpected bytes", len(r.buf))
	}
}
