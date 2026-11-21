package textselect

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// monospace lays a text out on one line, 10 points a UTF-16 unit.
type monospace int

func (n monospace) HitTest(p driver.Point) driver.Caret {
	return driver.Caret{Offset: min(max(int(p.X+5)/10, 0), int(n))}
}

func (n monospace) CaretRect(c driver.Caret) (driver.Bounds, bool) {
	return driver.Bounds{Origin: driver.Point{X: driver.Points(c.Offset * 10)}, Size: driver.Size{Height: 20}}, true
}

func (n monospace) RangeRects(start, end int, out []driver.Bounds) []driver.Bounds {
	return append(out, driver.Bounds{Origin: driver.Point{X: driver.Points(start * 10)}, Size: driver.Size{Width: driver.Points((end - start) * 10), Height: 20}})
}

// TestTellFollowsTheTextWithinOneText holds Tell to reporting a selection that grows within one text, whose extent, the text's box, stays, and to reporting none once it clears.
func TestTellFollowsTheTextWithinOneText(t *testing.T) {
	const content = "abcdefgh"
	s := &Selection{Area: 1, Texts: []Text{{ID: 2, Content: content, Box: driver.Bounds{Origin: driver.Point{X: 100, Y: 50}, Size: driver.Size{Width: 80, Height: 20}}, Layout: monospace(len(content))}}}
	area := driver.Point{X: 100, Y: 40}
	s.Model.Press(s, driver.Point{X: 100, Y: 60}, 1, false)
	s.Model.Drag(s, driver.Point{X: 130, Y: 60})
	ev, ok := s.Tell(area, false)
	box := driver.Bounds{Origin: driver.Point{Y: 10}, Size: driver.Size{Width: 80, Height: 20}}
	if !ok || ev.Text != "abc" || ev.Point != box.Origin || ev.Size != box.Size || ev.ID != 1 {
		t.Fatalf("first report = %+v, %v", ev, ok)
	}
	s.Model.Drag(s, driver.Point{X: 150, Y: 60})
	if ev, ok = s.Tell(area, false); !ok || ev.Text != "abcde" || ev.Point != box.Origin || ev.Size != box.Size {
		t.Fatalf("grown within the text = %+v, %v", ev, ok)
	}
	if _, ok = s.Tell(area, false); ok {
		t.Fatal("reported again with nothing changed")
	}
	if ev, ok = s.Tell(area, true); !ok || ev.Text != "" || ev.Size != (driver.Size{}) {
		t.Fatalf("cleared = %+v, %v", ev, ok)
	}
}

// TestTellReportsRepeatedTextElsewhere holds Tell to reporting again when the selection moves to the same words elsewhere: text and extent stay, the ranges differ.
func TestTellReportsRepeatedTextElsewhere(t *testing.T) {
	const content = "same same"
	s := &Selection{Area: 1, Texts: []Text{{ID: 2, Content: content, Box: driver.Bounds{Origin: driver.Point{X: 100, Y: 50}, Size: driver.Size{Width: 90, Height: 20}}, Layout: monospace(len(content))}}}
	area := driver.Point{X: 100, Y: 40}
	s.Model.Set(s, driver.TextPosition{Offset: 0}, driver.TextPosition{Offset: 4})
	ev, ok := s.Tell(area, false)
	if !ok || ev.Text != "same" || len(ev.Ranges) != 1 || ev.Ranges[0] != (driver.SelectedRange{ID: 2, End: 4}) {
		t.Fatalf("first report = %+v, %v", ev, ok)
	}
	s.Model.Set(s, driver.TextPosition{Offset: 5}, driver.TextPosition{Offset: 9})
	ev, ok = s.Tell(area, false)
	if !ok || ev.Text != "same" || len(ev.Ranges) != 1 || ev.Ranges[0] != (driver.SelectedRange{ID: 2, Start: 5, End: 9}) {
		t.Fatalf("repeated text elsewhere = %+v, %v", ev, ok)
	}
	if _, ok = s.Tell(area, false); ok {
		t.Fatal("reported again with nothing changed")
	}
}
