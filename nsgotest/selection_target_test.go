package nsgotest

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

type selectionSurface struct {
	noSurface
	requested driver.TextRange
}

func (s *selectionSurface) SelectRange(selection driver.TextRange) (driver.TextRange, error) {
	s.requested = selection
	// A native editor can normalize a range further, such as a grapheme's
	// boundaries. The model must use what it actually returned.
	return driver.TextRange{Start: 3, End: 4}, nil
}

func TestTargetSelectReadsTheNativeRange(t *testing.T) {
	app := Start(t, func(*core.Scope) core.Node {
		return core.Input{TestID: "field", DefaultValue: "A😀e\u0301Z"}
	})
	v := app.Find("id:field")
	s := &selectionSurface{}
	tg := &target{a: app, surface: s}
	tg.selectRange(v, 999, 3)
	if s.requested != (driver.TextRange{Start: 3, End: 6}) {
		t.Fatalf("requested native range = %v", s.requested)
	}
	if v.Selection != (driver.TextRange{Start: 3, End: 4}) {
		t.Fatalf("model discarded native normalization: %v", v.Selection)
	}
}
