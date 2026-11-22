package formula

import (
	"github.com/nsgo-dev/nsgo/base/textview"
	"github.com/nsgo-dev/nsgo/core"
)

// RenderMath shows a text view's formula with r, as textview.TextView.RenderMath takes it: an inline one, or a display one with Display's larger operators and limits, wrapped to its block's padded viewport. TextView centers its lines and scrolls any unbreakable overflow. A copy takes them as their TeX between $ and $$.
func (r *Renderer) RenderMath(cx *core.Scope, m textview.Math) core.Node {
	return View{Source: m.TeX, Display: m.Display, Renderer: r}
}

// RenderMath is Default().RenderMath, for textview.TextView{RenderMath: formula.RenderMath}.
func RenderMath(cx *core.Scope, m textview.Math) core.Node { return Default().RenderMath(cx, m) }
