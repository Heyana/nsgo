package diagram

import (
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

// diagramTooltip is a hint owned by the diagram's canvas. Its text follows the item under the pointer; the overlay itself never takes the pointer.
type diagramTooltip struct {
	open func() bool
	text func() string
}

func (diagramTooltip) Render(*core.Scope) core.Node { return nil }

func (t diagramTooltip) Hint(cx *core.Scope, anchor *core.Ref) (core.Node, func(core.HintChange)) {
	shown := core.NewSignal(cx, false)
	var stop func() bool
	change := func(c core.HintChange) {
		if stop != nil {
			stop()
			stop = nil
		}
		switch c {
		case core.HintHover:
			if cx.Env().Desktop() {
				stop = cx.AfterFunc(500*time.Millisecond, func() {
					stop = nil
					if t.open() {
						shown.Set(true)
					}
				})
			}
		case core.HintFocus:
			if t.open() {
				shown.Set(true)
			}
		case core.HintLeave, core.HintBlur:
			shown.Set(false)
		}
	}
	return core.Overlay{
		Anchor: anchor, Placement: core.PlaceTop, Offset: 4, Margin: 4,
		PointerEvents: core.PointerEventsNone,
		StyleFunc: func() core.Style {
			if !shown.Get() || !t.open() {
				return core.Style{Display: core.DisplayNone}
			}
			return core.Style{}
		},
		Children: []core.Node{core.Text{ContentFunc: t.text}},
	}, change
}
