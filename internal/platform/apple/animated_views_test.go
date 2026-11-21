//go:build darwin

package apple

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type animatedViewTestComponent struct {
	driver.NativeComponent
	calls *[]string
	value any
}

func (c *animatedViewTestComponent) SetProp(name string, value any) {
	*c.calls = append(*c.calls, name)
	c.value = value
}

func TestApplyAnimatedDeliversChangesInDrawingOrder(t *testing.T) {
	const mounted driver.NodeID = 1
	const missing driver.NodeID = 2
	a := driver.NewAnimator(nil)
	a.Apply([]driver.AnimationOp{
		{Kind: driver.AnimCreateValue, Node: 1, Value: 0.5},
		{Kind: driver.AnimBind, View: mounted, Prop: driver.PropOpacity, Node: 1},
		{Kind: driver.AnimBind, View: mounted, Prop: driver.PropTextColor, Node: 1},
		{Kind: driver.AnimBind, View: missing, Prop: driver.PropOpacity, Node: 1},
	})
	for _, id := range []driver.NodeID{mounted, missing} {
		a.BindCanvas(id, []driver.DrawSlot{{Node: 1}})
		a.BindNative(id, []driver.NativeBinding{{Name: "native", Node: 1}})
	}

	var applied driver.AnimatedValues
	var calls []string
	component := &animatedViewTestComponent{calls: &calls}
	viewOf := func(id driver.NodeID) *driver.AnimatedValues {
		if id == mounted {
			return &applied
		}
		return nil
	}
	paint := func(name string) func(*driver.AnimatedValues) {
		return func(*driver.AnimatedValues) { calls = append(calls, name) }
	}
	step := func(want ...string) {
		t.Helper()
		calls = nil
		ApplyAnimated(a, viewOf, func(v *driver.AnimatedValues) *driver.AnimatedValues { return v },
			paint("style"), paint("text"), paint("canvas"),
			func(*driver.AnimatedValues) driver.NativeComponent { return component },
			func() { calls = append(calls, "sticky") })
		if !slices.Equal(calls, want) {
			t.Fatalf("calls = %v, want %v", calls, want)
		}
	}
	step("style", "text", "canvas", "native", "sticky")
	if !applied.Has(driver.PropTextColor) || applied.Opacity != 0.5 || component.value != 0.5 {
		t.Fatalf("bound values were not applied: %+v, native %v", applied, component.value)
	}

	a.Apply([]driver.AnimationOp{{Kind: driver.AnimSet, Node: 1, Value: 0.25}})
	step("style", "canvas", "native", "sticky")
	a.Apply([]driver.AnimationOp{{Kind: driver.AnimBind, View: mounted, Prop: driver.PropTextColor}})
	step("style", "text", "sticky")
	if applied.Has(driver.PropTextColor) {
		t.Fatal("text color is still bound")
	}
	step("sticky")
}
