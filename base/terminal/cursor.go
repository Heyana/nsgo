package terminal

import (
	"time"

	"github.com/nsgo-dev/nsgo/base/terminal/internal/vt"
	"github.com/nsgo-dev/nsgo/core"
)

// cursorBlink runs on the platform's animation graph. Only the cursor's
// alpha is bound: the screen stays cached and the engine does no frame work.
func (v *view) cursorBlink() {
	v.cursorAlpha = core.NewAnimatedValue(v.cx, 1)
	v.cursor = core.NewSignal(v.cx, vt.Cursor{})
	v.cx.Effect(func() {
		cur := v.cursor.Get()
		v.cursorAlpha.Set(1)
		if !cur.Visible || !cur.InView || !cur.Blinking || !v.focused.Get() || v.composing.Get() != "" || v.cx.Env().ReduceMotion() {
			return
		}
		animation := core.ValueKeyframes(v.cursorAlpha, core.KeyframeTiming{Duration: time.Second, Iterations: core.Forever, Easing: core.StepsEasing(1, core.JumpEnd)}, core.StopAt(0, 1), core.StopAt(0.5, 0), core.StopAt(1, 1))
		run := v.cx.StartAnimation(core.AtFrameRate(animation, 2), nil)
		v.cx.Cleanup(run.Stop)
	})
}
