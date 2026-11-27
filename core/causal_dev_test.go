//go:build nsgodev

package core_test

import (
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func enableCausalDevTools(t *testing.T) {
	t.Helper()
	prevDev, prevTrace := devtools.Enabled, driver.TraceCauses
	devtools.Enabled, driver.TraceCauses = true, true
	t.Cleanup(func() {
		devtools.Enabled, driver.TraceCauses = prevDev, prevTrace
	})
}

func causalSurfaceOf(t *testing.T, s devtools.Surface) devtools.CausalSurface {
	t.Helper()
	cs, ok := s.(devtools.CausalSurface)
	if !ok {
		t.Fatalf("surface %T does not implement devtools.CausalSurface in nsgodev build", s)
	}
	return cs
}

func TestCausalSignalTriggersDirectAndBatched(t *testing.T) {
	enableCausalDevTools(t)

	var (
		count      *core.Signal[int]
		effectCx   *core.Scope
		effectSite string
		runs       int
	)
	root := core.NewRoot(core.RootOptions{CheckCommits: true, Post: func(func()) {}, Clock: core.NewManualClock(time.Unix(0, 0))})
	defer root.Dispose()
	tree := core.NewTree(root, core.ApproxTextEngine{})
	tree.Resize(core.Size{Width: 100, Height: 100}, core.Geometry{})
	tree.Mount(func(cx *core.Scope) core.Node {
		count = core.NewSignal(cx, 0)
		_, file, ln, _ := runtime.Caller(0)
		cx.Effect(func() {
			effectCx = cx.Child()
			_ = count.Get()
			runs++
		})
		effectSite = file + ":" + strconv.Itoa(ln+1)
		return core.Text{ContentFunc: func() string { return strconv.Itoa(count.Get()) }}
	})
	tree.Commit()
	if runs != 1 {
		t.Fatalf("initial effect runs = %d, want 1", runs)
	}
	// Initial runs have no signal-write trigger cause.
	if got := core.DevTriggers(root, 0); len(got) != 0 {
		t.Fatalf("initial runs recorded %+v, want none", got)
	}

	surfaces := devtools.Surfaces()
	if len(surfaces) == 0 {
		t.Fatal("no devtools surface registered")
	}
	cs := causalSurfaceOf(t, surfaces[len(surfaces)-1])
	t.Cleanup(func() { devtools.Unregister(cs) })

	// Find count's signal ID from Owners() to confirm signal ID matches causal records.
	var ownerSignalID uint64
	var walkValues func(o devtools.Owner)
	walkValues = func(o devtools.Owner) {
		for _, v := range o.Values {
			if v.Type == "int" && v.Text == "0" && strings.Contains(v.Site, "causal_dev_test.go") {
				ownerSignalID = v.ID
			}
		}
		for _, c := range o.Children {
			walkValues(c)
		}
	}
	walkValues(cs.Owners())
	if ownerSignalID == 0 {
		t.Fatal("could not locate count signal in Owners()")
	}

	beforeCauses := len(core.DevCauses(root, 0))

	// 1. Direct write.
	_, file, writeLine1, _ := runtime.Caller(0)
	count.Set(1)
	wantWrite1 := file + ":" + strconv.Itoa(writeLine1+1)

	batch := tree.Commit()
	if batch == nil || batch.Causes == nil {
		t.Fatal("commit recorded no Causes")
	}
	if len(batch.Causes.Triggers) < 2 {
		t.Fatalf("commit Triggers = %+v, want binding and effect", batch.Causes.Triggers)
	}

	causes := core.DevCauses(root, 0)
	if len(causes) != beforeCauses+1 {
		t.Fatalf("DevCauses = %+v, want %d writes", causes, beforeCauses+1)
	}
	c1 := causes[len(causes)-1]
	wantSeq1 := uint64(beforeCauses + 1)
	if c1.Seq != wantSeq1 || c1.SignalID != ownerSignalID || c1.Generation != 1 || c1.Site != wantWrite1 {
		t.Fatalf("cause 1 = %+v, want seq=%d signalID=%d gen=1 site=%s", c1, wantSeq1, ownerSignalID, wantWrite1)
	}

	triggers := core.DevTriggers(root, 0)
	if len(triggers) != 2 {
		t.Fatalf("DevTriggers = %+v, want 2 (binding + effect)", triggers)
	}
	var effectRun *core.TriggerRun
	for i := range triggers {
		if triggers[i].Kind == "effect" {
			effectRun = &triggers[i]
		}
	}
	if effectRun == nil {
		t.Fatalf("no effect trigger in %+v", triggers)
	}
	if effectRun.Cause != c1 || effectRun.Owner == 0 || effectRun.Site != effectSite || effectRun.Retriggered != 0 || effectRun.Duration < 0 {
		t.Fatalf("effectRun = %+v, want cause=%+v site=%s retriggered=0", *effectRun, c1, effectSite)
	}

	// Verify Owners() exposes owner IDs and computation LastCause/LastTriggerSeq.
	var foundComp *devtools.Computation
	var walkComps func(o devtools.Owner)
	walkComps = func(o devtools.Owner) {
		if o.ID == 0 {
			t.Errorf("owner %q has zero ID", o.Name)
		}
		for i := range o.Computations {
			if o.Computations[i].Kind == "effect" && o.Computations[i].Site == effectSite {
				cp := o.Computations[i]
				foundComp = &cp
			}
		}
		for _, c := range o.Children {
			walkComps(c)
		}
	}
	walkComps(cs.Owners())
	if foundComp == nil || foundComp.LastTriggerSeq != wantSeq1 || foundComp.LastCause == nil || foundComp.LastCause.SignalID != ownerSignalID {
		t.Fatalf("Owners() effect computation = %+v, want LastTriggerSeq=%d SignalID=%d", foundComp, wantSeq1, ownerSignalID)
	}
	_ = effectCx

	// 2. Batched double write coalesces and increments Retriggered.
	_, _, batchLine, _ := runtime.Caller(0)
	root.Batch(func() {
		count.Set(2)
		count.Set(3)
	})
	wantWrite3 := file + ":" + strconv.Itoa(batchLine+3)
	tree.Commit()

	causes = core.DevCauses(root, 0)
	if len(causes) != beforeCauses+3 {
		t.Fatalf("DevCauses len = %d, want %d", len(causes), beforeCauses+3)
	}
	wantSeq3 := uint64(beforeCauses + 3)
	lastCause := causes[len(causes)-1]
	if lastCause.Seq != wantSeq3 || lastCause.Generation != 3 || lastCause.Site != wantWrite3 {
		t.Fatalf("cause 3 = %+v, want seq=%d gen=3 site=%s", lastCause, wantSeq3, wantWrite3)
	}
	recent := cs.Triggers(2)
	if len(recent) != 2 {
		t.Fatalf("Triggers(2) = %+v, want 2", recent)
	}
	for _, r := range recent {
		if r.Cause.Seq != wantSeq3 || r.Cause.Generation != 3 || r.Retriggered != 1 {
			t.Errorf("batched trigger = %+v, want seq=%d gen=3 retriggered=1", r, wantSeq3)
		}
	}

	// Causes(10) pairs causes with their triggered runs.
	summaries := cs.Causes(10)
	if len(summaries) != beforeCauses+3 {
		t.Fatalf("Causes(10) len = %d, want %d", len(summaries), beforeCauses+3)
	}
	if len(summaries[beforeCauses].Triggers) != 2 {
		t.Errorf("cause 1 triggers = %+v, want 2", summaries[beforeCauses].Triggers)
	}
	if len(summaries[len(summaries)-1].Triggers) != 2 {
		t.Errorf("cause 3 triggers = %+v, want 2", summaries[len(summaries)-1].Triggers)
	}
}

func TestCausalMemoChainAndDiamondDeduplication(t *testing.T) {
	enableCausalDevTools(t)

	root := core.NewRoot(core.RootOptions{Post: func(func()) {}})
	defer root.Dispose()

	s := core.NewSignal(root, 1)
	// Diamond: s -> mA, mB -> effect; plus unchanged memo mParity -> parityEffect.
	mA := core.NewMemo(root, func() int { return s.Get() + 10 })
	mB := core.NewMemo(root, func() int { return s.Get() * 2 })
	mParity := core.NewMemo(root, func() int { return s.Get() % 2 })

	diamondRuns, parityRuns := 0, 0
	root.Effect(func() {
		_ = mA.Get() + mB.Get()
		diamondRuns++
	})
	root.Effect(func() {
		_ = mParity.Get()
		parityRuns++
	})
	if diamondRuns != 1 || parityRuns != 1 {
		t.Fatalf("initial runs = %d, %d; want 1, 1", diamondRuns, parityRuns)
	}

	// Write 1 -> 3: parity stays odd (1), so mParity runs without changing and parityEffect must NOT run.
	s.Set(3)
	if diamondRuns != 2 || parityRuns != 1 {
		t.Fatalf("after s.Set(3), runs = %d, %d; want 2, 1", diamondRuns, parityRuns)
	}

	triggers := core.DevTriggers(root, 0)
	// Expect: mA, mB, mParity (3 memos) + diamondEffect (1 effect) = 4 runs, all under seq=1 with Retriggered=0.
	if len(triggers) != 4 {
		t.Fatalf("triggers = %+v, want 4 runs (3 memos + 1 diamond effect)", triggers)
	}
	effectCount := 0
	for _, r := range triggers {
		if r.Cause.Seq != 1 {
			t.Errorf("trigger %+v has seq %d, want 1", r, r.Cause.Seq)
		}
		if r.Retriggered != 0 {
			t.Errorf("single-write diamond trigger %+v has Retriggered=%d, want 0", r, r.Retriggered)
		}
		if r.Kind == "effect" {
			effectCount++
		}
	}
	if effectCount != 1 {
		t.Fatalf("effect triggers = %d, want 1 (parityEffect skipped)", effectCount)
	}
}

func TestCausalTriggerRingBufferBounded(t *testing.T) {
	enableCausalDevTools(t)

	root := core.NewRoot(core.RootOptions{Post: func(func()) {}})
	defer root.Dispose()

	s := core.NewSignal(root, 0)
	root.Effect(func() { _ = s.Get() })
	for i := 1; i <= 300; i++ {
		s.Set(i)
	}
	causes := core.DevCauses(root, 0)
	triggers := core.DevTriggers(root, 0)
	if len(causes) != 256 || len(triggers) != 256 {
		t.Fatalf("ring buffer sizes = (%d, %d), want (256, 256)", len(causes), len(triggers))
	}
	if causes[0].Seq != 45 || causes[len(causes)-1].Seq != 300 {
		t.Fatalf("cause seq range = [%d..%d], want [45..300]", causes[0].Seq, causes[len(causes)-1].Seq)
	}
	if triggers[0].Cause.Seq != 45 || triggers[len(triggers)-1].Cause.Seq != 300 {
		t.Fatalf("trigger seq range = [%d..%d], want [45..300]", triggers[0].Cause.Seq, triggers[len(triggers)-1].Cause.Seq)
	}
}

func TestCausalScopeDisposalReasonsAndCascade(t *testing.T) {
	enableCausalDevTools(t)

	root := core.NewRoot(core.RootOptions{CheckCommits: true, Post: func(func()) {}})
	defer root.Dispose()
	tree := core.NewTree(root, core.ApproxTextEngine{})
	tree.Resize(core.Size{Width: 100, Height: 100}, core.Geometry{})

	// 1. Explicit disposal with parent cascade and custom Cleanup.
	parent := root.Child()
	child := parent.Child()
	_, file, cleanupLine, _ := runtime.Caller(0)
	child.Cleanup(func() {})
	wantCleanupSite := file + ":" + strconv.Itoa(cleanupLine+1)

	parentID := core.DevOwnerID(parent)
	childID := core.DevOwnerID(child)

	_, _, disposeLine, _ := runtime.Caller(0)
	parent.Dispose()
	wantDisposeSite := file + ":" + strconv.Itoa(disposeLine+1)

	disposals := core.DevDisposals(root, 0)
	if len(disposals) != 2 {
		t.Fatalf("disposals = %+v, want cascade + explicit records", disposals)
	}
	casc, expl := disposals[0], disposals[1]
	if casc.Reason != core.DisposalCascade || casc.RootOwner != parentID || casc.Owner != childID || casc.Caller != wantDisposeSite {
		t.Fatalf("cascade record = %+v, want reason=cascade root=%d owner=%d caller=%s", casc, parentID, childID, wantDisposeSite)
	}
	if len(casc.Cleanups) != 1 || casc.Cleanups[0].Label != "cleanup" || casc.Cleanups[0].Site != wantCleanupSite {
		t.Fatalf("cascade cleanups = %+v, want cleanup at %s", casc.Cleanups, wantCleanupSite)
	}
	if expl.Reason != core.DisposalExplicit || expl.RootOwner != parentID || expl.Owner != parentID || expl.Caller != wantDisposeSite {
		t.Fatalf("explicit record = %+v, want reason=dispose root=%d owner=%d caller=%s", expl, parentID, parentID, wantDisposeSite)
	}

	// 2. Re-run cleanup via Show branch flip triggered by a signal write.
	var shown *core.Signal[bool]
	var branchCleanupSite string
	tree.Mount(func(cx *core.Scope) core.Node {
		shown = core.NewSignal(cx, true)
		return core.Show{
			When: shown.Get,
			Then: func(cx *core.Scope) core.Node {
				_, f, l, _ := runtime.Caller(0)
				cx.Cleanup(func() {})
				branchCleanupSite = f + ":" + strconv.Itoa(l+1)
				return core.Text{Content: "visible"}
			},
		}
	})
	tree.Commit()

	shown.Set(false)
	tree.Commit()

	causes := core.DevCauses(root, 0)
	flipSeq := causes[len(causes)-1].Seq
	disposals = core.DevDisposals(root, 0)
	var sawRerun, sawBranchCascade bool
	for _, d := range disposals {
		if d.Reason == core.DisposalRerun && d.TriggerSeq == flipSeq {
			sawRerun = true
		}
		if d.Reason == core.DisposalCascade && d.TriggerSeq == flipSeq {
			for _, cl := range d.Cleanups {
				if cl.Label == "cleanup" && cl.Site == branchCleanupSite {
					sawBranchCascade = true
				}
			}
		}
	}
	if !sawRerun || !sawBranchCascade {
		t.Fatalf("Show flip disposals = %+v, want rerun (triggerSeq=%d) and cascade cleanup at %s", disposals, flipSeq, branchCleanupSite)
	}
}

func TestCausalDisposalBuiltInResourceLabels(t *testing.T) {
	enableCausalDevTools(t)

	// Desktop resources: window, panel, tray, audio-player, sound.
	root := core.NewRoot(core.RootOptions{
		Post:    func(func()) {},
		Windows: &core.WindowOptions{Width: 400, Height: 300},
		Tray:    true,
	})
	defer root.Dispose()
	tree := core.NewTree(root, core.ApproxTextEngine{})
	tree.SetEnvironment(driver.Environment{OS: driver.OSMacOS, Idiom: driver.IdiomDesktop, Audio: true})
	tree.Mount(core.AppRoot(func(cx *core.Scope) {
		cx.SetMainView(func(*core.Scope) core.Node { return core.View{} })
	}))
	tree.Commit()

	scope := root.Child()
	win := scope.OpenWindow(core.WindowOptions{Title: "Child"}, func(*core.Scope) core.Node { return core.View{} })
	panel := scope.OpenPanel(core.PanelOptions{Width: 200, Height: 120}, func(*core.Scope) core.Node { return core.View{} })
	tray := scope.NewTray(core.TrayOptions{Tooltip: "Status"})
	player := scope.NewAudioPlayer(core.AudioSource{URI: "asset:theme.mp3"}, core.AudioPlayerOptions{})
	scope.LoadSound(core.AudioSource{URI: "asset:click.wav"}, func(*core.Sound, error) {})
	if win == nil || panel == nil || tray == nil || player == nil {
		t.Fatalf("expected non-nil desktop/audio resources: win=%v panel=%v tray=%v player=%v", win, panel, tray, player)
	}

	scope.Dispose()

	labels := map[string]string{}
	for _, d := range core.DevDisposals(root, 0) {
		for _, cl := range d.Cleanups {
			if cl.Label != "" {
				labels[cl.Label] = cl.Site
			}
		}
	}
	for _, want := range []string{"window", "panel", "tray", "audio-player", "sound"} {
		site, ok := labels[want]
		if !ok {
			t.Errorf("missing cleanup label %q in %+v", want, labels)
		} else if !strings.Contains(site, "causal_dev_test.go") {
			t.Errorf("cleanup %q site = %q, want test file site", want, site)
		}
	}

	// Android overlay resource: overlay.
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		ovScope := cx.Child()
		ov := ovScope.OpenOverlay(core.OverlayOptions{Width: 200, Height: 100}, func(*core.Scope) core.Node { return core.View{} })
		if ov == nil {
			t.Error("OpenOverlay returned nil on Android")
		}
		ovScope.Dispose()
		return core.View{}
	}, nsgotest.Environment(driver.Environment{OS: driver.OSAndroid, Idiom: driver.IdiomPhone}))
	surfaces := devtools.Surfaces()
	cs := causalSurfaceOf(t, surfaces[len(surfaces)-1])
	t.Cleanup(func() { devtools.Unregister(cs) })
	var sawOverlay bool
	app.Post(func() {
		for _, d := range cs.Disposals(50) {
			for _, cl := range d.Cleanups {
				if cl.Label == "overlay" && strings.Contains(cl.Site, "causal_dev_test.go") {
					sawOverlay = true
				}
			}
		}
	})
	app.Settle()
	if !sawOverlay {
		t.Fatal("overlay disposal cleanup label not recorded")
	}
}

func TestCausalDisposalRingBufferBounded(t *testing.T) {
	enableCausalDevTools(t)

	root := core.NewRoot(core.RootOptions{Post: func(func()) {}})
	defer root.Dispose()

	for range 300 {
		c := root.Child()
		c.Dispose()
	}
	disposals := core.DevDisposals(root, 0)
	if len(disposals) != 256 {
		t.Fatalf("disposals ring len = %d, want 256", len(disposals))
	}
	if disposals[0].Seq != 45 || disposals[len(disposals)-1].Seq != 300 {
		t.Fatalf("disposal seq range = [%d..%d], want [45..300]", disposals[0].Seq, disposals[len(disposals)-1].Seq)
	}
}
