package terminal

import (
	"bytes"

	"github.com/nsgo-dev/nsgo/base/terminal/internal/vt"
	"github.com/nsgo-dev/nsgo/core"
)

// promptDistance counts editable graphemes in the current logical input line, ignoring prompt prefixes and the spacer cells of wide characters. The cursor may sit just after its last printed input cell. Terminal.mu is held.
func (t *Terminal) promptDistance(col, row int) (int, bool) {
	if t.term == nil || !t.term.AtBottom() || !t.term.CursorInInput() {
		return 0, false
	}
	if ref, ok := t.term.CellAt(col, row); ok && col > 0 && t.term.CellWide(ref) == vt.SpacerTail {
		col--
	}
	x, y := t.term.Cursor()
	_, rows := t.term.Size()
	first, last := y, y
	for first > 0 {
		kind, _ := t.term.Row(first)
		_, wrap := t.term.Row(first - 1)
		if kind != vt.PromptContinuation || !wrap {
			break
		}
		first--
	}
	for last+1 < rows {
		_, wrap := t.term.Row(last)
		kind, _ := t.term.Row(last + 1)
		if !wrap || kind != vt.PromptContinuation {
			break
		}
		last++
	}
	if row < first || row > last {
		return 0, false
	}
	var input []gridCell
	for line := first; line <= last; line++ {
		_, cols := t.term.RowText(line)
		previous := -1
		for _, cell := range cols {
			if cell == previous {
				continue
			}
			previous = cell
			ref, ok := t.term.CellAt(cell, line)
			if ok && t.term.CellIsInput(ref) {
				input = append(input, gridCell{cell, line})
			}
		}
	}
	before := func(a, b gridCell) bool { return a.y < b.y || a.y == b.y && a.x < b.x }
	index := func(at gridCell) int {
		i := 0
		for i < len(input) && before(input[i], at) {
			i++
		}
		return i
	}
	return index(gridCell{col, row}) - index(gridCell{x, y}), true
}

// movePrompt sends ordinary cursor keys, encoded in the program's active key modes, as one contiguous input. The release rechecks the prompt because output may have ended input or replaced the prompt since the press.
func (v *view) movePrompt(col, row int) {
	t := v.t
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.redraw()
	defer t.mu.Unlock()
	distance, ok := t.promptDistance(col, row)
	if !ok || distance == 0 {
		return
	}
	if v.keys == nil {
		var err error
		v.keys, err = vt.NewKeyEncoder()
		if err != nil {
			return
		}
	}
	key := vt.KeyArrowRight
	if distance < 0 {
		key = vt.KeyArrowLeft
		distance = -distance
	}
	v.keys.Sync(t.term, false)
	data := v.keys.Encode(vt.KeyEvent{Action: vt.KeyPress, Key: key})
	if len(data) > 0 {
		t.in.push(bytes.Repeat(data, distance))
		t.term.SetSelection(nil)
	}
}

// promptCell excludes padding and points beyond the viewport instead of clamping an outside release onto an edge cell.
func (v *view) promptCell(p core.Point) (col, row int, inside bool) {
	x, y := float32(p.X), float32(float64(p.Y)-v.scrollY)
	if x < v.padX || y < v.padY || x >= v.padX+float32(v.cols)*v.cellW || y >= v.padY+float32(v.rows)*v.cellH {
		return 0, 0, false
	}
	col, row, _, _ = v.cellAt(p)
	return col, row, true
}
