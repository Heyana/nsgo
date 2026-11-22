package nsgotest

import (
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

type settlingSurface struct {
	noSurface
	sync      func()
	animating func() bool
}

// Native edits precede the app's controlled value; applying the two in separate groups used to put the old native text back over that value.
func TestTargetModelKeepsEventsAndCommitsInOrder(t *testing.T) {
	a := Start(t, func(*core.Scope) core.Node { return core.Input{TestID: "field"} })
	v := a.Find("id:field")
	tg := &target{a: a}
	tg.event(driver.Event{Kind: driver.EventChangeText, ID: v.ID, Text: "loud", Count: 1})
	tg.observe(0, &driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemInputValue, ID: v.ID, Value: "LOUD", Count: 1}}})
	tg.drain()
	if v.Text != "LOUD" {
		t.Fatalf("controlled text = %q, want LOUD", v.Text)
	}
	// A subsequent native edit wins over a preceding commit.
	tg.observe(0, &driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemInputValue, ID: v.ID, Value: "OTHER", Count: 1}}})
	tg.event(driver.Event{Kind: driver.EventChangeText, ID: v.ID, Text: "OTHER!", Count: 2})
	tg.drain()
	if v.Text != "OTHER!" {
		t.Fatalf("subsequent edit = %q, want OTHER!", v.Text)
	}
	// A slow response to an earlier edit must be rejected just as the native field rejects an ItemInputValue with an old edit count.
	tg.event(driver.Event{Kind: driver.EventChangeText, ID: v.ID, Text: "OTHER!!", Count: 3})
	tg.observe(0, &driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemInputValue, ID: v.ID, Value: "STALE", Count: 2}}})
	tg.drain()
	if v.Text != "OTHER!!" {
		t.Fatalf("a stale controlled value replaced the latest edit with %q", v.Text)
	}
	// An old view's focus must not reach a new view reusing its ID.
	tg.event(driver.Event{Kind: driver.EventFocus, ID: v.ID})
	tg.observe(0, &driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemDelete, ID: v.ID},
		{Kind: driver.ItemCreate, ID: v.ID, View: driver.KindInput},
	}})
	tg.drain()
	if a.views[v.ID].Focused {
		t.Fatal("a focus event for the deleted field reached its replacement")
	}
}

func (s settlingSurface) Sync() { s.sync() }
func (s settlingSurface) Animating() bool {
	return s.animating != nil && s.animating()
}

func settleFixture(t *testing.T) (*target, *View, *driver.MountBatch) {
	t.Helper()
	a := Start(t, func(*core.Scope) core.Node { return core.Text{Content: "before"} })
	v := a.Find("text:before")
	batch := &driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemText, ID: v.ID,
		Text: &driver.TextContent{Text: "after"}}}}
	return &target{a: a}, v, batch
}

func TestTargetSettleWaitsForThePendingCommit(t *testing.T) {
	tg, view, batch := settleFixture(t)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	var calls int
	tg.surface = settlingSurface{sync: func() {
		calls++
		if calls == 1 {
			close(entered)
			<-release
			tg.observe(0, batch)
		}
	}}
	go func() { tg.settle(); close(done) }()
	t.Cleanup(func() { once.Do(func() { close(release) }); <-done })
	select {
	case <-entered:
	case <-done:
		t.Fatal("Settle returned without waiting for the engine")
	case <-time.After(5 * time.Second):
		t.Fatal("Settle did not synchronize with the engine")
	}
	select {
	case <-done:
		t.Fatal("Settle treated the pending commit as a quiet app")
	case <-time.After(2 * quietFor):
	}
	once.Do(func() { close(release) })
	<-done
	if !tg.quiet || view.Text != "after" {
		t.Fatalf("quiet=%v, text=%q, want the completed commit", tg.quiet, view.Text)
	}
}

func TestTargetSettleChecksTheEngineBeforeReturningQuiet(t *testing.T) {
	tg, view, batch := settleFixture(t)
	calls := 0
	tg.surface = settlingSurface{sync: func() {
		calls++
		if calls == 2 {
			// A commit started after the first barrier but did not send its batch until the second barrier completed.
			tg.observe(0, batch)
		}
	}}
	tg.settle()
	if !tg.quiet || view.Text != "after" {
		t.Fatalf("quiet=%v, text=%q, want the commit that finished during the quiet interval", tg.quiet, view.Text)
	}
}

func TestTargetSettleTakesFocusReportedAtItsLastNativeCheck(t *testing.T) {
	a := Start(t, func(*core.Scope) core.Node { return core.Input{TestID: "field"} })
	field := a.Find("id:field")
	tg := &target{a: a, live: true}
	a.target = tg
	defer func() { a.target = nil }()
	tg.observe(0, &driver.MountBatch{Root: a.rootID, Items: []driver.MountItem{{Kind: driver.ItemFocus, ID: field.ID}}})
	tg.drain()
	if field.Focused {
		t.Fatal("the engine's focus request changed focus before the platform reported it")
	}
	syncs := 0
	reported := false
	tg.surface = settlingSurface{sync: func() { syncs++ }, animating: func() bool {
		if syncs == 2 && !reported {
			tg.event(driver.Event{Kind: driver.EventFocus, ID: field.ID})
			reported = true
		}
		return false
	}}
	tg.settle()
	if !tg.quiet || !field.Focused {
		t.Fatalf("quiet=%v, focused=%v, want the native focus in the returned model", tg.quiet, field.Focused)
	}
}

func TestTargetFocusFollowsReportedEventsOnEveryTarget(t *testing.T) {
	for _, targetName := range []string{"web", "ios_simulator", "tvos_simulator", "watchos_simulator", "android", "harmonyos"} {
		t.Run(targetName, func(t *testing.T) {
			a := Start(t, func(*core.Scope) core.Node {
				return core.View{Children: []core.Node{
					core.Input{TestID: "first"},
					core.Input{TestID: "second"},
				}}
			})
			t.Setenv("NSGO_TEST_TARGET", targetName)
			first, second := a.Find("id:first"), a.Find("id:second")
			tg := &target{a: a}
			a.target = tg
			defer func() { a.target = nil }()

			// Focus requests do not mutate the target model before native focus events arrive.
			tg.observe(0, &driver.MountBatch{Root: a.rootID, Items: []driver.MountItem{{Kind: driver.ItemFocus, ID: first.ID}}})
			tg.drain()
			if first.Focused || second.Focused {
				t.Fatalf("ItemFocus mutated focus on %s: first=%v second=%v", targetName, first.Focused, second.Focused)
			}

			// A reported focus event moves focus into first.
			tg.surfaceEvent(0, driver.Event{Kind: driver.EventFocus, ID: first.ID})
			tg.drain()
			if !first.Focused || second.Focused {
				t.Fatalf("EventFocus did not focus first on %s: first=%v second=%v", targetName, first.Focused, second.Focused)
			}

			// Blur requests also wait for the platform's reported EventBlur.
			tg.observe(0, &driver.MountBatch{Root: a.rootID, Items: []driver.MountItem{{Kind: driver.ItemBlur, ID: first.ID}}})
			tg.drain()
			if !first.Focused {
				t.Fatalf("ItemBlur cleared focus on %s before EventBlur", targetName)
			}

			// A settle waits for animating() to finish queued mounts and takes the focus/blur events reported there.
			tg.observe(0, &driver.MountBatch{Root: a.rootID, Items: []driver.MountItem{{Kind: driver.ItemFocus, ID: second.ID}}})
			applied := false
			tg.surface = settlingSurface{sync: func() {}, animating: func() bool {
				if !applied {
					applied = true
					tg.surfaceEvent(0, driver.Event{Kind: driver.EventBlur, ID: first.ID})
					tg.surfaceEvent(0, driver.Event{Kind: driver.EventFocus, ID: second.ID})
					return true
				}
				return false
			}}
			tg.settle()
			if !tg.quiet || first.Focused || !second.Focused {
				t.Fatalf("settle on %s: quiet=%v first=%v second=%v", targetName, tg.quiet, first.Focused, second.Focused)
			}
		})
	}
}

func TestTargetSettleBoundsABlockedEngineAndReusesItsWaiter(t *testing.T) {
	tg, _, _ := settleFixture(t)
	release := make(chan struct{})
	var calls atomic.Int32
	tg.surface = settlingSurface{sync: func() { calls.Add(1); <-release }}
	t.Cleanup(func() { close(release); <-tg.syncing })
	for range 3 {
		start := time.Now()
		tg.settleUntil(start.Add(20 * time.Millisecond))
		if tg.quiet || time.Since(start) > time.Second {
			t.Fatalf("quiet=%v after %s with a blocked engine", tg.quiet, time.Since(start))
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("started %d engine waiters, want one across timed-out steps", got)
	}
}

func TestTargetSettleBoundsChangingAppsAndPreservesMeasure(t *testing.T) {
	for _, measure := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "measure"}[measure], func(t *testing.T) {
			tg, _, batch := settleFixture(t)
			tg.a.measure = measure
			tg.surface = settlingSurface{sync: func() {}, animating: func() bool {
				tg.observe(0, batch) // The app keeps committing between polls.
				return false
			}}
			start := time.Now()
			tg.settleUntil(start.Add(500 * time.Millisecond))
			if tg.quiet != measure {
				t.Fatalf("quiet=%v, want %v for an app that keeps committing", tg.quiet, measure)
			}
			if time.Since(start) > 2*time.Second {
				t.Fatal("Settle outlasted its deadline")
			}
		})
	}
}

// engineFixture is a target whose engine runs posts and barriers in order on a goroutine of its own, which owns the app's graph, as a platform's does.
func engineFixture(t *testing.T) (tg *target, count *core.Signal[int], ranOnEngine func() bool) {
	t.Helper()
	tg, _, _ = settleFixture(t)
	tg.a.target = tg
	queue := make(chan func(), 1024)
	stop := make(chan struct{})
	ready := make(chan *core.Scope)
	var engine atomic.Uint64
	go func() {
		engine.Store(currentGoroutineForTest())
		root := core.NewRoot(core.RootOptions{CheckCommits: true, Post: func(fn func()) { queue <- fn }})
		count = core.NewSignal(root, 0)
		ready <- root
		for {
			select {
			case fn := <-queue:
				fn()
			case <-stop:
				return
			}
		}
	}()
	tg.appScope = <-ready
	t.Cleanup(func() { close(stop) })
	tg.surface = settlingSurface{sync: func() {
		done := make(chan struct{})
		queue <- func() { close(done) }
		<-done
	}}
	return tg, count, func() bool { return engine.Load() == currentGoroutineForTest() }
}

func TestTargetPostsRunOnTheEngineAndSettleWhatTheyPost(t *testing.T) {
	tg, count, onEngine := engineFixture(t)
	var off atomic.Int32
	var hop func(left int)
	hop = func(left int) {
		if !onEngine() {
			off.Add(1)
		}
		count.Update(func(n int) int { return n + 1 })
		if left > 0 {
			tg.a.Post(func() { hop(left - 1) })
		}
	}
	tg.a.Post(func() { hop(40) })
	tg.settle()
	if !tg.quiet {
		t.Fatal("Settle did not find the app quiet after the posts")
	}
	if off.Load() != 0 {
		t.Fatalf("%d posted functions ran off the engine's goroutine", off.Load())
	}
	var got int
	tg.a.Post(func() { got = count.Get() })
	tg.settle()
	if got != 41 {
		t.Fatalf("the step ran %d of 41 chained posts", got)
	}
}

func currentGoroutineForTest() uint64 {
	var buf [64]byte
	fields := strings.Fields(string(buf[:runtime.Stack(buf[:], false)]))
	id, _ := strconv.ParseUint(fields[1], 10, 64)
	return id
}
