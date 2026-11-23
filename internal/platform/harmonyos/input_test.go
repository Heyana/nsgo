//go:build js && wasm

package harmonyos

import (
	"encoding/json"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestInputSendsEditability checks that a field's input operation carries its editability at o[13] and o[14], its padding (top, right, bottom, left) at o[15]..o[18] (#1598), whether it hides its scroll indicator at o[19] (#1596), and its font family, italic style, line height and alignment at o[20]..o[23] (#1597).
func TestInputSendsEditability(t *testing.T) {
	pad := driver.Edges{Top: 18, Right: 20, Bottom: 22, Left: 12}
	for _, c := range []driver.InputConfig{
		{Readonly: true},
		{Disabled: true},
		{Multiline: true, Padding: pad, HideIndicator: true},
		{Style: driver.TextStyle{Font: driver.Font{Family: "Avenir", Italic: true}, LineHeight: 28, Align: driver.TextAlignCenter}},
		{},
	} {
		m := &mounter{views: map[driver.NodeID]*view{}}
		m.input(4, &c)
		var got [][]any
		if err := json.Unmarshal(m.o.bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || len(got[0]) != 24 || got[0][0] != "in" {
			t.Fatalf("input operations %v, want one in with 22 arguments", got)
		}
		o := got[0]
		if o[13] != c.Readonly || o[14] != c.Disabled || o[19] != c.HideIndicator {
			t.Errorf("readonly %v, disabled %v, hideIndicator %v; want %v, %v, %v",
				o[13], o[14], o[19], c.Readonly, c.Disabled, c.HideIndicator)
		}
		if o[15] != float64(c.Padding.Top) || o[16] != float64(c.Padding.Right) ||
			o[17] != float64(c.Padding.Bottom) || o[18] != float64(c.Padding.Left) {
			t.Errorf("padding %v %v %v %v, want %+v", o[15], o[16], o[17], o[18], c.Padding)
		}
		if o[20] != c.Style.Font.Family || o[21] != c.Style.Font.Italic ||
			o[22] != float64(c.Style.LineHeight) || o[23] != float64(c.Style.Align) {
			t.Errorf("style %v %v %v %v, want %q %v %v %v", o[20], o[21], o[22], o[23],
				c.Style.Font.Family, c.Style.Font.Italic, c.Style.LineHeight, c.Style.Align)
		}
	}
}

func TestMultilineInputScrolls(t *testing.T) {
	s := &surface{}
	m := newMounter(s)
	m.views[1] = &view{id: 1, kind: driver.KindView, frame: driver.Bounds{Size: driver.Size{Width: 300, Height: 400}}}
	m.views[2] = &view{id: 2, kind: driver.KindInput, parent: 1, frame: driver.Bounds{Origin: driver.Point{X: 10, Y: 10}, Size: driver.Size{Width: 200, Height: 80}}}
	m.views[3] = &view{id: 3, kind: driver.KindInput, parent: 1, frame: driver.Bounds{Origin: driver.Point{X: 10, Y: 120}, Size: driver.Size{Width: 200, Height: 40}}}
	m.input(2, &driver.InputConfig{Multiline: true, HideIndicator: true})
	m.input(3, &driver.InputConfig{Multiline: false})

	if got := m.scrollTarget(driver.Point{X: 50, Y: 40}); got != 2 {
		t.Fatalf("multiline scrollTarget = %d, want 2", got)
	}
	if got := m.scrollTarget(driver.Point{X: 50, Y: 140}); got != 0 {
		t.Fatalf("single-line scrollTarget = %d, want 0", got)
	}

	m.o.reset()
	m.scrollTo(2, driver.Point{Y: 45}, false)
	var ops [][]any
	if err := json.Unmarshal(m.o.bytes(), &ops); err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0][0] != "so" || ops[0][1] != float64(2) || ops[0][3] != float64(45) {
		t.Fatalf("scrollTo ops = %v, want [so 2 0 45 false]", ops)
	}
	if m.views[2].applied != 1 {
		t.Fatalf("applied = %d, want 1", m.views[2].applied)
	}
}
