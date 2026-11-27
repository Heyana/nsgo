//go:build ios

package watchos

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestApplyAnimatedValuesBeforeTheFirstHostFrame(t *testing.T) {
	const id driver.NodeID = 1
	animator := driver.NewAnimator(func(driver.Event) {})
	animator.Apply([]driver.AnimationOp{
		{Kind: driver.AnimCreateValue, Node: 1, Value: 1},
		{Kind: driver.AnimBind, View: id, Prop: driver.PropTranslateX, Node: 1},
	})
	n := &watchNode{id: id}
	s := &surface{animator: animator}
	s.views = []*watchNode{nil, n}
	s.dirty = map[*watchNode]bool{}
	s.redraw = map[*watchNode]bool{}
	s.applyAnimated()
	if n.animated == nil || n.animated.TranslateX != 1 {
		t.Fatalf("animated values = %+v, want translate x 1 before a host frame", n.animated)
	}
	if !s.dirty[n] {
		t.Fatal("initial animated values did not mark the view dirty")
	}
}
