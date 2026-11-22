package scrollbar

import (
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestSettlePreservesTheScrollLifecycle(t *testing.T) {
	frame := driver.Size{Width: 100, Height: 200}
	content := driver.Size{Width: 500, Height: 1000}
	for _, tc := range []struct {
		name       string
		config     driver.ScrollConfig
		offset     driver.Point
		reduced    bool
		wantTarget driver.Point
		want       []string
	}{
		{name: "free view", offset: driver.Point{Y: 80}, want: []string{"rest"}},
		{name: "already snapped", config: driver.ScrollConfig{Behavior: driver.ScrollBehavior{Paging: true}},
			offset: driver.Point{Y: 200}, want: []string{"rest"}},
		{name: "reduced motion", config: driver.ScrollConfig{Behavior: driver.ScrollBehavior{Paging: true}},
			offset: driver.Point{Y: 260}, reduced: true, wantTarget: driver.Point{Y: 200},
			want: []string{"EventMomentumStart", "scroll", "EventMomentumEnd", "rest"}},
		{name: "animated snap", config: driver.ScrollConfig{Behavior: driver.ScrollBehavior{Paging: true}},
			offset: driver.Point{Y: 260}, wantTarget: driver.Point{Y: 200},
			want: []string{"EventMomentumStart", "animate"}},
		{name: "horizontal snap", config: driver.ScrollConfig{Horizontal: true, Behavior: driver.ScrollBehavior{Paging: true}},
			offset: driver.Point{X: 160}, wantTarget: driver.Point{X: 200},
			want: []string{"EventMomentumStart", "animate"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var actions []string
			var target driver.Point
			host := SettleHost{
				Scroll: func(to driver.Point) {
					actions = append(actions, "scroll")
					target = to
				},
				Animate: func(to driver.Point) {
					actions = append(actions, "animate")
					target = to
				},
				Event: func(kind driver.EventKind) { actions = append(actions, kind.String()) },
				Rest:  func() { actions = append(actions, "rest") },
			}
			host.Settle(&tc.config, frame, content, tc.offset, 0, tc.reduced)
			if !slices.Equal(actions, tc.want) || target != tc.wantTarget {
				t.Fatalf("actions %v, target %v, want %v at %v", actions, target, tc.want, tc.wantTarget)
			}
		})
	}
}

func TestRestReportsBeforeStartingTheSnappingClamp(t *testing.T) {
	animator := driver.NewAnimator(nil)
	animator.Apply([]driver.AnimationOp{
		{Kind: driver.AnimCreateValue, Node: 1},
		{Kind: driver.AnimCreateDiffClamp, Node: 2, Input: 1, Min: 0, Max: 40, Snap: true},
		{Kind: driver.AnimBindScroll, View: 9, Y: 1},
	})
	animator.Value(2)
	animator.Scroll(9, driver.Point{Y: 25})
	var actions []string
	Rest(animator, 9, func() {
		if animator.Running() {
			t.Error("the snapping clamp started before the scroll-end report")
		}
		actions = append(actions, "report")
	}, func() {
		if !animator.Running() {
			t.Error("the frame clock started without a snapping clamp")
		}
		actions = append(actions, "clock")
	})
	if !slices.Equal(actions, []string{"report", "clock"}) {
		t.Fatalf("rest actions %v", actions)
	}
	animator.Step(0)
	animator.Step(300 * time.Millisecond)
	value, _, _ := animator.Value(2)
	if value != 0 || animator.Running() {
		t.Fatalf("snapping clamp settled at %v, running %v", value, animator.Running())
	}
	actions = nil
	Rest(animator, 9, func() { actions = append(actions, "report") }, func() { actions = append(actions, "clock") })
	if !slices.Equal(actions, []string{"report"}) {
		t.Fatalf("resting clamp restarted the clock: %v", actions)
	}
}
