//go:build darwin && !ios

package macos

import (
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestALongScrollCommitsOnlyWhenItEnds(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	var rested time.Duration
	// Ten seconds of input or glide: never mid-scroll.
	for at := 1; at < 10_000; at += 16 {
		if commitDue(ms(at), ms(at)+commitDelay, false, true, &rested) {
			t.Fatalf("committed mid-scroll at %d ms", at)
		}
	}
	if commitDue(ms(10_000), ms(10_100), true, false, &rested) {
		t.Fatal("committed while the offset still moved")
	}
	if !commitDue(ms(10_200), ms(10_100), true, false, &rested) {
		t.Fatal("did not commit at rest")
	}
}

func TestARunningAnimationHoldsACommitOnlyBriefly(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	var rested time.Duration
	if commitDue(ms(1000), ms(900), true, true, &rested) {
		t.Fatal("committed while the frame clock runs")
	}
	if commitDue(ms(1000)+restWait-ms(32), ms(900), true, true, &rested) {
		t.Fatal("committed before the wait ran out")
	}
	if !commitDue(ms(1000)+restWait, ms(900), true, true, &rested) {
		t.Fatal("an endless animation held the commit back")
	}
	// Input again starts the wait over.
	rested = ms(1)
	if commitDue(ms(5000), ms(4900), false, true, &rested) || rested != 0 {
		t.Fatal("input did not start the wait over")
	}
}

func TestAScrollPutsThePickOffUntilItSettles(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	// Ten seconds of steps, each pushing the deadline on: never mid-scroll.
	for at := 0; at < 10_000; at += 16 {
		if repickIn(ms(at+16), ms(at)+repickDelay, false) == 0 {
			t.Fatalf("picked mid-scroll at %d ms", at)
		}
	}
	last := ms(10_000) + repickDelay
	if got := repickIn(ms(10_050), last, false); got != ms(70) {
		t.Fatalf("waits %v 50 ms after the last step, want 70ms", got)
	}
	if repickIn(last, last, false) != 0 {
		t.Fatal("did not pick when the scroll settled")
	}
	if repickIn(last, last, true) == 0 {
		t.Fatal("picked while a batch applied")
	}
}

func TestAScrollUnderWayNeedsNoRouting(t *testing.T) {
	v := &view{id: 1, sc: &scrollState{}}
	s := &surface{views: []*view{nil, v}}
	gone := &view{id: 1, sc: &scrollState{}}
	check := func(name string, phase, momentum uint, want bool) {
		t.Helper()
		if got := s.wheelContinues(phase, momentum); got != want {
			t.Errorf("%s: wheelContinues = %v, want %v", name, got, want)
		}
	}
	check("a change with no gesture", phaseChanged, 0, false)
	check("momentum with no view", 0, phaseChanged, false)

	s.wheels.gesture = &wheelGesture{}
	check("a gesture before it finds its view", phaseChanged, 0, false)
	check("a beginning", phaseBegan, 0, false)
	check("the end of a gesture without a view", 0|phaseEnded, 0, true)

	s.wheels.gesture = &wheelGesture{view: v, held: true}
	check("resting fingers choose their view", phaseChanged, 0, false)
	check("fingers lifted without scrolling", phaseCancelled, 0, true)

	s.wheels.gesture = &wheelGesture{view: v}
	check("a change", phaseChanged, 0, true)
	check("an end", phaseEnded, 0, true)
	s.wheels.gesture = &wheelGesture{view: gone}
	check("a change on an unmounted view", phaseChanged, 0, false)

	s.wheels.gesture, s.wheels.momentum = nil, v
	check("momentum", 0, phaseChanged, true)
	check("the end of momentum", 0, phaseEnded, true)
	s.wheels.momentum = gone
	check("momentum on an unmounted view", 0, phaseChanged, false)
}

// The indicator shows while the user scrolls, as AppKit's overlay scrollers do, and stays hidden while the app scrolls (#343).
func TestTheIndicatorShowsOnlyForTheUsersScrolls(t *testing.T) {
	onMain(func() {
		loadAppKit()
		s := &surface{native: objc.ID(classView).Send(selNew), animator: driver.NewAnimator(nil)}
		defer s.native.Send(selRelease)
		defer func() { s.closed = true }() // what the moves put off stays undone
		v := s.create(driver.KindScroll, 1)
		defer v.native.Send(selRelease)
		defer delete(views, v.native)
		s.views = []*view{nil, v}
		v.frame = driver.Bounds{Size: driver.Size{Width: 100, Height: 200}}
		v.sc.content = driver.Size{Width: 100, Height: 1000}
		v.sc.ticking = true // the test steps the frames
		check := func(what string, wantAt driver.Points, shown bool) {
			t.Helper()
			if v.pos() != wantAt {
				t.Fatalf("%s: offset %v, want %v", what, v.pos(), wantAt)
			}
			if got := v.sc.alpha > 0; got != shown {
				t.Errorf("%s: indicator shown %v, want %v", what, got, shown)
			}
		}
		ease := func() {
			p := v.sc.phase
			if p.kind != easing {
				t.Fatalf("phase %v, want easing", p.kind)
			}
			v.tickScroll(p.start + p.duration/2)
			if v.pos() == p.from {
				t.Fatal("the easing did not move")
			}
		}

		v.scrollTo(driver.Point{Y: 300}, false)
		check("ScrollTo", 300, false)
		v.scrollTo(driver.Point{Y: 600}, true)
		ease()
		check("an animated ScrollTo under way", v.pos(), false)
		v.tickScroll(v.sc.phase.start + v.sc.phase.duration)
		check("an animated ScrollTo", 600, false)
		v.scrollBy(driver.Point{Y: 20})
		check("ScrollBy", 620, false)

		v.wheelBy(0, -20, 0)
		check("a mouse wheel", 600, true)

		// A trackpad gesture pulled past the top springs back as the fingers lift.
		v.sc.phase, v.sc.alpha = scrollPhase{}, 0
		v.moveTo(-40)
		check("the test's own move", -40, false)
		v.sc.phase = scrollPhase{kind: wheeling, gesture: true, raw: -40}
		v.wheelEnd(0, 0)
		v.sc.alpha = 0
		ease()
		check("a spring back", v.pos(), true)
	})
}

func TestKeepScrollStartRetainsRefreshRoom(t *testing.T) {
	cfg := func(top, bottom, left, indicator driver.Points, horizontal bool) *driver.ScrollConfig {
		return &driver.ScrollConfig{
			Horizontal: horizontal,
			Behavior: driver.ScrollBehavior{
				ContentInset:     driver.Edges{Top: top, Bottom: bottom, Left: left},
				RefreshIndicator: indicator,
			},
		}
	}
	for _, tc := range []struct {
		name       string
		oldCfg     *driver.ScrollConfig
		newCfg     *driver.ScrollConfig
		refreshing bool
		at         driver.Point
		want       driver.Point
	}{
		{
			name:       "refreshing spinner keeps room on bottom inset change",
			oldCfg:     cfg(0, 0, 0, 0, false),
			newCfg:     cfg(0, 20, 0, 0, false),
			refreshing: true,
			at:         driver.Point{Y: -56},
			want:       driver.Point{Y: -56},
		},
		{
			name:       "refreshing spinner shifts with top inset increase",
			oldCfg:     cfg(0, 0, 0, 0, false),
			newCfg:     cfg(10, 0, 0, 0, false),
			refreshing: true,
			at:         driver.Point{Y: -56},
			want:       driver.Point{Y: -66},
		},
		{
			name:       "refreshing spinner shifts with top inset decrease",
			oldCfg:     cfg(10, 0, 0, 0, false),
			newCfg:     cfg(0, 0, 0, 0, false),
			refreshing: true,
			at:         driver.Point{Y: -66},
			want:       driver.Point{Y: -56},
		},
		{
			name:       "refreshing spinner keeps room on left inset change",
			oldCfg:     cfg(0, 0, 0, 0, false),
			newCfg:     cfg(0, 0, 12, 0, false),
			refreshing: true,
			at:         driver.Point{Y: -56},
			want:       driver.Point{X: -12, Y: -56},
		},
		{
			name:       "refreshing custom indicator keeps room on top inset change",
			oldCfg:     cfg(10, 0, 0, 80, false),
			newCfg:     cfg(24, 0, 0, 80, false),
			refreshing: true,
			at:         driver.Point{Y: -90},
			want:       driver.Point{Y: -104},
		},
		{
			name:       "simultaneous inset and indicator change keeps old room for easing",
			oldCfg:     cfg(0, 0, 0, 0, false),
			newCfg:     cfg(10, 0, 0, 80, false),
			refreshing: true,
			at:         driver.Point{Y: -56},
			want:       driver.Point{Y: -66},
		},
		{
			name:       "refreshing view scrolled away stays put on inset change",
			oldCfg:     cfg(10, 0, 0, 80, false),
			newCfg:     cfg(20, 30, 0, 80, false),
			refreshing: true,
			at:         driver.Point{Y: 0},
			want:       driver.Point{Y: 0},
		},
		{
			name:       "ordinary view at start follows top inset increase",
			oldCfg:     cfg(0, 0, 0, 0, false),
			newCfg:     cfg(15, 0, 0, 0, false),
			refreshing: false,
			at:         driver.Point{Y: 0},
			want:       driver.Point{Y: -15},
		},
		{
			name:       "ordinary view at start follows top inset decrease",
			oldCfg:     cfg(15, 0, 0, 0, false),
			newCfg:     cfg(0, 0, 0, 0, false),
			refreshing: false,
			at:         driver.Point{Y: -15},
			want:       driver.Point{Y: 0},
		},
		{
			name:       "ordinary view scrolled away stays put on inset change",
			oldCfg:     cfg(0, 0, 0, 0, false),
			newCfg:     cfg(15, 0, 0, 0, false),
			refreshing: false,
			at:         driver.Point{Y: 120},
			want:       driver.Point{Y: 120},
		},
		{
			name:       "horizontal view follows left inset without vertical refresh room",
			oldCfg:     cfg(0, 0, 0, 0, true),
			newCfg:     cfg(0, 0, 12, 0, true),
			refreshing: true,
			at:         driver.Point{},
			want:       driver.Point{X: -12},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := keepScrollStart(tc.oldCfg, tc.newCfg, tc.refreshing, tc.at); got != tc.want {
				t.Fatalf("keepScrollStart() = %v, want %v", got, tc.want)
			}
		})
	}
}

// Changing ContentInset while a macOS scroll view rests at its refresh indicator must retain the refresh room (#1546), as on Windows and Linux.
func TestRefreshingKeepsRoomAcrossInsetChanges(t *testing.T) {
	onMain(func() {
		loadAppKit()
		s := &surface{native: objc.ID(classView).Send(selNew), animator: driver.NewAnimator(nil)}
		defer s.native.Send(selRelease)
		defer func() { s.closed = true }()
		v := s.create(driver.KindScroll, 1)
		defer v.native.Send(selRelease)
		defer delete(views, v.native)
		s.views = []*view{nil, v}
		v.frame = driver.Bounds{Size: driver.Size{Width: 200, Height: 300}}
		v.sc.content = driver.Size{Width: 200, Height: 1000}

		settleEase := func(what string) {
			t.Helper()
			p := v.sc.phase
			if p.kind != easing {
				t.Fatalf("%s: phase %v, want easing", what, p.kind)
			}
			v.sc.ticking = v.tickScroll(p.start + p.duration)
		}
		checkAt := func(what string, wantAt, wantReported driver.Point) {
			t.Helper()
			if v.sc.at != wantAt {
				t.Fatalf("%s: offset = %v, want %v", what, v.sc.at, wantAt)
			}
			if got := v.reported(); got != wantReported {
				t.Fatalf("%s: reported = %v, want %v", what, got, wantReported)
			}
		}

		// Start refreshing with the built-in spinner (56 pt room).
		cfg := driver.ScrollConfig{Refreshable: true, Refreshing: true}
		v.configureScroll(&cfg)
		settleEase("start refresh")
		checkAt("rest at spinner", driver.Point{Y: -56}, driver.Point{Y: 0})

		// Non-top inset changes and layout changes retain the refresh room.
		cfg.Behavior.ContentInset.Bottom = 20
		v.configureScroll(&cfg)
		checkAt("bottom inset change", driver.Point{Y: -56}, driver.Point{Y: 0})

		v.setContent(driver.Size{Width: 200, Height: 1200})
		v.frame.Size.Height = 280
		v.sizeScroll()
		checkAt("content and viewport change", driver.Point{Y: -56}, driver.Point{Y: 0})

		// Top and left inset changes keep the refresh room above the new inset.
		cfg.Behavior.ContentInset.Top = 10
		v.configureScroll(&cfg)
		checkAt("top inset increase", driver.Point{Y: -66}, driver.Point{Y: -10})

		cfg.Behavior.ContentInset.Left = 8
		v.configureScroll(&cfg)
		checkAt("left inset change", driver.Point{X: -8, Y: -66}, driver.Point{X: -8, Y: -10})

		// Simultaneous ContentInset and RefreshIndicator changes keep the previous room and ease to the new indicator height.
		cfg.Behavior.ContentInset.Top = 14
		cfg.Behavior.RefreshIndicator = 80
		v.configureScroll(&cfg)
		checkAt("before custom indicator ease", driver.Point{X: -8, Y: -70}, driver.Point{X: -8, Y: -14})
		settleEase("custom indicator height change")
		checkAt("rest at custom indicator", driver.Point{X: -8, Y: -94}, driver.Point{X: -8, Y: -14})

		// Decreasing the top inset while resting at a custom indicator keeps the custom room and commits it when idle.
		cfg.Behavior.ContentInset.Top = 10
		v.configureScroll(&cfg)
		checkAt("top inset decrease at custom indicator", driver.Point{X: -8, Y: -90}, driver.Point{X: -8, Y: -10})
		if v.sc.committed != (driver.Point{X: -8, Y: -90}) {
			t.Fatalf("committed = %v, want {-8 -90}", v.sc.committed)
		}

		// Ending refresh eases back to the ordinary top inset.
		cfg.Refreshing = false
		v.configureScroll(&cfg)
		settleEase("end refresh")
		checkAt("after refresh ends", driver.Point{X: -8, Y: -10}, driver.Point{X: -8, Y: -10})

		// Ordinary inset updates still keep a non-refreshing view at its start and leave scrolled views alone.
		cfg.Behavior.ContentInset.Top = 24
		v.configureScroll(&cfg)
		checkAt("ordinary start inset change", driver.Point{X: -8, Y: -24}, driver.Point{X: -8, Y: -24})

		v.scrollTo(driver.Point{Y: 120}, false)
		cfg.Behavior.ContentInset.Top = 30
		v.configureScroll(&cfg)
		checkAt("ordinary scrolled inset change", driver.Point{X: -8, Y: 120}, driver.Point{X: -8, Y: 120})

		// Explicitly scrolling away while refreshing does not snap back to the refresh room on inset changes.
		v.scrollTo(driver.Point{Y: -30}, false)
		cfg.Refreshing = true
		v.configureScroll(&cfg)
		settleEase("restart refresh")
		checkAt("rest at restarted custom indicator", driver.Point{X: -8, Y: -110}, driver.Point{X: -8, Y: -30})

		v.scrollTo(driver.Point{Y: 0}, false)
		cfg.Behavior.ContentInset.Bottom = 40
		v.configureScroll(&cfg)
		checkAt("scrolled away during refresh", driver.Point{X: -8, Y: 0}, driver.Point{X: -8, Y: 0})

		// A pull-to-refresh confirmed after its spring-back settles also retains its room across inset changes.
		cfg = driver.ScrollConfig{Refreshable: true}
		v.sc.refreshing = false
		v.sc.at = driver.Point{}
		v.sc.committed = driver.Point{}
		v.configureScroll(&cfg)
		v.sc.phase = scrollPhase{kind: wheeling, gesture: true, raw: -80}
		v.moveTo(-80)
		v.wheelEnd(0, 0)
		if !v.sc.refreshing {
			t.Fatal("pull past threshold did not enter refreshing")
		}
		settleEase("pull-to-refresh spring back")
		checkAt("local refresh rest", driver.Point{Y: -56}, driver.Point{Y: 0})

		cfg.Refreshing = true
		cfg.Behavior.ContentInset = driver.Edges{Top: 12, Bottom: 20}
		v.configureScroll(&cfg)
		checkAt("confirming commit with inset change", driver.Point{Y: -68}, driver.Point{Y: -12})
	})
}
