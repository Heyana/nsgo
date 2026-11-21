package nsgotest

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
)

func TestGlassInteractiveStyleAndPress(t *testing.T) {
	presses := 0
	var interactive *core.Signal[bool]
	app := Start(t, func(cx *core.Scope) core.Node {
		interactive = core.NewSignal(cx, true)
		return core.View{
			TestID: "glass-btn",
			Role:   core.RoleButton,
			Label:  "Glass button",
			OnPress: func(core.PressEvent) {
				presses++
			},
			StyleFunc: func() core.Style {
				return core.Style{
					Width:            core.Pt(120),
					Height:           core.Pt(44),
					GlassEffect:      core.GlassRegular,
					GlassInteractive: interactive.Get(),
				}
			},
			Children: []core.Node{core.Text{Content: "Tap"}},
		}
	})
	v := app.Find("id:glass-btn")
	if v.Style.Glass != core.GlassRegular || !v.Style.GlassInteractive {
		t.Fatalf("initial style = %+v, want GlassRegular + GlassInteractive", v.Style)
	}
	app.Press("id:glass-btn")
	if presses != 1 {
		t.Fatalf("presses = %d, want 1", presses)
	}
	app.Post(func() { interactive.Set(false) })
	app.Settle()
	v = app.Find("id:glass-btn")
	if v.Style.Glass != core.GlassRegular || v.Style.GlassInteractive {
		t.Fatalf("after disable: style = %+v, want GlassRegular + !GlassInteractive", v.Style)
	}
}
