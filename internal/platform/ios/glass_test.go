//go:build ios && nsgotest

package ios

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestInteractiveLiquidGlassRoutesTouches(t *testing.T) {
	if apple.Class("UIGlassEffect") == 0 {
		t.Skip("UIGlassEffect requires iOS 26 or later")
	}
	var (
		glassPresses atomic.Int32
		childPresses atomic.Int32
		glassSig     *core.Signal[core.GlassEffect]
		interSig     *core.Signal[bool]
	)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		glassSig = core.NewSignal(cx, core.GlassRegular)
		interSig = core.NewSignal(cx, true)
		return core.View{
			Style: core.Style{Width: core.Pt(300), Height: core.Pt(200), Padding: core.All(20)},
			Children: []core.Node{
				core.View{
					TestID: "glass",
					OnPress: func(core.PressEvent) {
						glassPresses.Add(1)
					},
					StyleFunc: func() core.Style {
						return core.Style{
							Width:            core.Pt(200),
							Height:           core.Pt(100),
							CornerRadius:     16,
							Direction:        core.Row,
							Padding:          core.All(10),
							Gap:              20,
							GlassEffect:      glassSig.Get(),
							GlassInteractive: interSig.Get(),
						}
					},
					Children: []core.Node{
						core.Text{
							TestID:  "label",
							Content: "Glass",
							Style:   core.Style{Width: core.Pt(80), Height: core.Pt(40)},
						},
						core.View{
							TestID: "child",
							OnPress: func(core.PressEvent) {
								childPresses.Add(1)
							},
							Style: core.Style{Width: core.Pt(70), Height: core.Pt(40)},
						},
					},
				},
			},
		}
	})

	findView := func(id string) *view {
		for _, v := range runner.views {
			if v != nil && v.props.TestID == id {
				return v
			}
		}
		return nil
	}

	var skipMsg, failMsg string
	onMainSync(func() {
		glassView := findView("glass")
		childBtn := findView("child")
		if glassView == nil || childBtn == nil {
			failMsg = fmt.Sprintf("views did not mount: glass=%v child=%v", glassView, childBtn)
			return
		}
		if glassView.backdrop == 0 {
			failMsg = "expected UIVisualEffectView backdrop for Liquid Glass"
			return
		}
		effect := glassView.backdrop.Send(sel("effect"))
		if !objc.Send[bool](effect, sel("respondsToSelector:"), selSetInteractive) {
			skipMsg = "UIGlassEffect.setInteractive: unavailable"
			return
		}
		if !objc.Send[bool](effect, sel("isInteractive")) {
			failMsg = "UIGlassEffect.isInteractive = false, want true"
			return
		}
		if !objc.Send[bool](glassView.backdrop, sel("isUserInteractionEnabled")) {
			failMsg = "UIVisualEffectView.isUserInteractionEnabled = false, want true"
			return
		}
		// Bare glass and non-pressable label both route to the interactive glass backdrop.
		for _, pt := range []struct {
			name string
			p    apple.CGPoint
		}{
			{"bare glass", apple.CGPoint{X: 50, Y: 80}},
			{"non-pressable label", apple.CGPoint{X: 30, Y: 25}},
		} {
			hit := glassView.native.Send(sel("hitTest:withEvent:"), pt.p, objc.ID(0))
			if !glassView.isBackdropHit(hit) {
				failMsg = fmt.Sprintf("hitTest(%s) = %v, want descendant of backdrop %v", pt.name, hit, glassView.backdrop)
				return
			}
		}
		// Pressable child keeps its own hit target.
		if hit := glassView.native.Send(sel("hitTest:withEvent:"), apple.CGPoint{X: 145, Y: 30}, objc.ID(0)); hit != childBtn.native {
			failMsg = fmt.Sprintf("hitTest(pressable child) = %v, want childBtn.native %v", hit, childBtn.native)
			return
		}
	})
	if skipMsg != "" {
		t.Skip(skipMsg)
	}
	if failMsg != "" {
		t.Fatal(failMsg)
	}

	app.Press("id:label")
	if got := glassPresses.Load(); got != 1 {
		t.Fatalf("pressing label inside interactive glass fired %d presses, want 1", got)
	}
	app.Press("id:child")
	if childPresses.Load() != 1 || glassPresses.Load() != 1 {
		t.Fatalf("pressing child inside interactive glass: child=%d glass=%d, want 1 and 1", childPresses.Load(), glassPresses.Load())
	}

	app.Post(func() { interSig.Set(false) })
	app.Settle()
	onMainSync(func() {
		glassView := findView("glass")
		effect := glassView.backdrop.Send(sel("effect"))
		if objc.Send[bool](effect, sel("isInteractive")) {
			failMsg = "UIGlassEffect.isInteractive stayed true after disabling"
			return
		}
		if objc.Send[bool](glassView.backdrop, sel("isUserInteractionEnabled")) {
			failMsg = "UIVisualEffectView.isUserInteractionEnabled stayed true after disabling"
			return
		}
		if hit := glassView.native.Send(sel("hitTest:withEvent:"), apple.CGPoint{X: 50, Y: 80}, objc.ID(0)); hit != glassView.native {
			failMsg = fmt.Sprintf("hitTest after disabling GlassInteractive = %v, want glassView.native %v", hit, glassView.native)
			return
		}
	})
	if failMsg != "" {
		t.Fatal(failMsg)
	}

	app.Post(func() { glassSig.Set(core.GlassNone) })
	app.Settle()
	onMainSync(func() {
		glassView := findView("glass")
		if glassView.backdrop != 0 || glassView.backdropKind != backdropNone || glassView.glassInteractive {
			failMsg = fmt.Sprintf("removing glass left backdrop=%v kind=%v interactive=%v", glassView.backdrop, glassView.backdropKind, glassView.glassInteractive)
		}
	})
	if failMsg != "" {
		t.Fatal(failMsg)
	}
}
