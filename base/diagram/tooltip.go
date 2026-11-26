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
	in := false
	var stop func() bool
	cancel := func() {
		if stop != nil {
			stop()
			stop = nil
		}
	}
	// rest shows the tooltip once the pointer has rested on an item, on desktops.
	rest := func() {
		cancel()
		if cx.Env().Desktop() {
			stop = cx.AfterFunc(500*time.Millisecond, func() {
				stop = nil
				if t.open() {
					shown.Set(true)
				}
			})
		}
	}
	// The pointer enters the diagram once but crosses many items: reaching one with a tooltip starts the rest again, and leaving it for none hides it.
	cx.Effect(func() {
		open := t.open()
		cx.Untrack(func() {
			switch {
			case !open:
				cancel()
				shown.Set(false)
			case in && !shown.GetUntracked() && stop == nil:
				rest()
			}
		})
	})
	change := func(c core.HintChange) {
		cancel()
		switch c {
		case core.HintHover:
			in = true
			rest()
		case core.HintFocus:
			if t.open() {
				shown.Set(true)
			}
		case core.HintLeave:
			in = false
			shown.Set(false)
		case core.HintBlur:
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
