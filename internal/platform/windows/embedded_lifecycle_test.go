package windows

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type fakeRootWatcher struct {
	changes   []embeddedRootChange
	destroyed []uintptr
	onChange  func(root uintptr, state embeddedRootState)
	onDestroy func(root uintptr)
}

type embeddedRootChange struct {
	root  uintptr
	state embeddedRootState
}

func (f *fakeRootWatcher) embeddedRootChanged(root uintptr, state embeddedRootState) {
	f.changes = append(f.changes, embeddedRootChange{root: root, state: state})
	if f.onChange != nil {
		f.onChange(root, state)
	}
}

func (f *fakeRootWatcher) embeddedRootDestroyed(root uintptr) {
	f.destroyed = append(f.destroyed, root)
	if f.onDestroy != nil {
		f.onDestroy(root)
	}
}

func TestEmbeddedRootStateInitialAndLifecycle(t *testing.T) {
	cases := []struct {
		name       string
		visible    bool
		iconic     bool
		foreground bool
		wantMin    bool
		wantLife   driver.Lifecycle
	}{
		{name: "visible foreground", visible: true, iconic: false, foreground: true, wantMin: false, wantLife: driver.LifecycleActive},
		{name: "visible background window", visible: true, iconic: false, foreground: false, wantMin: false, wantLife: driver.LifecycleInactive},
		{name: "visible minimized", visible: true, iconic: true, foreground: false, wantMin: true, wantLife: driver.LifecycleBackground},
		{name: "minimized even if foreground", visible: true, iconic: true, foreground: true, wantMin: true, wantLife: driver.LifecycleBackground},
		{name: "hidden active", visible: false, iconic: false, foreground: true, wantMin: false, wantLife: driver.LifecycleBackground},
		{name: "hidden minimized", visible: false, iconic: true, foreground: false, wantMin: true, wantLife: driver.LifecycleBackground},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := initialEmbeddedRootState(tc.visible, tc.iconic, tc.foreground)
			if s.minimized != tc.wantMin {
				t.Fatalf("minimized = %v, want %v", s.minimized, tc.wantMin)
			}
			if got := s.lifecycle(); got != tc.wantLife {
				t.Fatalf("lifecycle = %v, want %v", got, tc.wantLife)
			}
		})
	}
}

func TestEmbeddedRootStateMessageTransitions(t *testing.T) {
	s := initialEmbeddedRootState(true, false, true)
	if s.lifecycle() != driver.LifecycleActive || s.minimized {
		t.Fatalf("initial state = %+v", s)
	}

	// Unrelated resize (SIZE_RESTORED while not minimized) is a no-op.
	if next, changed := s.applyMessage(embeddedMsgSize, embeddedSizeRestored, 0); changed || next != s {
		t.Fatalf("steady SIZE_RESTORED changed state: %+v -> %+v", s, next)
	}
	// SIZE_MAXSHOW (3) and SIZE_MAXHIDE (4) are ignored.
	for _, wp := range []uintptr{3, 4} {
		if _, changed := s.applyMessage(embeddedMsgSize, wp, 0); changed {
			t.Fatalf("WM_SIZE wParam=%d unexpectedly changed state", wp)
		}
	}

	// Minimize root window.
	next, changed := s.applyMessage(embeddedMsgSize, embeddedSizeMinimized, 0)
	if !changed || !next.minimized || next.lifecycle() != driver.LifecycleBackground {
		t.Fatalf("after SIZE_MINIMIZED: (%+v, %v)", next, changed)
	}
	s = next

	// WM_ACTIVATE with WA_INACTIVE in LOWORD and minimized flag in HIWORD must deactivate.
	next, changed = s.applyMessage(embeddedMsgActivate, (1<<16)|embeddedWAInactive, 0)
	if !changed || next.active || !next.minimized || next.lifecycle() != driver.LifecycleBackground {
		t.Fatalf("after WM_ACTIVATE(HIWORD|WA_INACTIVE): (%+v, %v)", next, changed)
	}
	s = next

	// Restore root window without activating -> Inactive, not minimized.
	next, changed = s.applyMessage(embeddedMsgSize, embeddedSizeRestored, 0)
	if !changed || next.minimized || next.lifecycle() != driver.LifecycleInactive {
		t.Fatalf("after SIZE_RESTORED: (%+v, %v)", next, changed)
	}
	s = next

	// Activate root window (WA_CLICKACTIVE = 2) -> Active.
	next, changed = s.applyMessage(embeddedMsgActivate, 2, 0)
	if !changed || !next.active || next.lifecycle() != driver.LifecycleActive {
		t.Fatalf("after WM_ACTIVATE(WA_CLICKACTIVE): (%+v, %v)", next, changed)
	}
	s = next

	// Deactivate app -> Inactive.
	next, changed = s.applyMessage(embeddedMsgActivateApp, 0, 0)
	if !changed || next.active || next.lifecycle() != driver.LifecycleInactive {
		t.Fatalf("after WM_ACTIVATEAPP(0): (%+v, %v)", next, changed)
	}
	s = next

	// Reactivate app -> Active.
	next, changed = s.applyMessage(embeddedMsgActivateApp, 1, 0)
	if !changed || !next.active || next.lifecycle() != driver.LifecycleActive {
		t.Fatalf("after WM_ACTIVATEAPP(1): (%+v, %v)", next, changed)
	}
	s = next

	// Hide root window -> Background, not minimized.
	next, changed = s.applyMessage(embeddedMsgShowWindow, 0, 0)
	if !changed || next.visible || next.minimized || next.lifecycle() != driver.LifecycleBackground {
		t.Fatalf("after WM_SHOWWINDOW(0): (%+v, %v)", next, changed)
	}
	s = next

	// Minimize while hidden, then show -> stays Background because still minimized.
	s, _ = s.applyMessage(embeddedMsgSize, embeddedSizeMinimized, 0)
	next, changed = s.applyMessage(embeddedMsgShowWindow, 1, 0)
	if !changed || !next.visible || !next.minimized || next.lifecycle() != driver.LifecycleBackground {
		t.Fatalf("after WM_SHOWWINDOW(1) while minimized: (%+v, %v)", next, changed)
	}
	s = next

	// Maximize from minimized -> Active, not minimized.
	next, changed = s.applyMessage(embeddedMsgSize, embeddedSizeMaximized, 0)
	if !changed || next.minimized || next.lifecycle() != driver.LifecycleActive {
		t.Fatalf("after SIZE_MAXIMIZED: (%+v, %v)", next, changed)
	}

	// Unrelated message is ignored.
	if _, changed := next.applyMessage(0x0003, 0, 0); changed {
		t.Fatal("WM_MOVE unexpectedly changed state")
	}
}

func TestEmbeddedRootManagerMultipleViewsOnOneRoot(t *testing.T) {
	const root uintptr = 0x100
	var installed, removed []uintptr
	mgr := newEmbeddedRootManager(embeddedRootHooks{
		install: func(r uintptr) bool {
			installed = append(installed, r)
			return true
		},
		remove: func(r uintptr) {
			removed = append(removed, r)
		},
		readState: func(r uintptr) embeddedRootState {
			return initialEmbeddedRootState(true, false, true)
		},
	})

	v1, v2 := &fakeRootWatcher{}, &fakeRootWatcher{}
	st1, ok1 := mgr.attach(v1, root)
	st2, ok2 := mgr.attach(v2, root)
	if !ok1 || !ok2 || st1.lifecycle() != driver.LifecycleActive || st2 != st1 {
		t.Fatalf("attach results = (%+v,%v), (%+v,%v)", st1, ok1, st2, ok2)
	}
	if !slices.Equal(installed, []uintptr{root}) {
		t.Fatalf("installed = %v, want single install for %x", installed, root)
	}

	// Minimize reaches both views.
	if changed, destroyed := mgr.handleMessage(root, embeddedMsgSize, embeddedSizeMinimized, 0); !changed || destroyed {
		t.Fatalf("minimize handleMessage = (%v, %v), want (true, false)", changed, destroyed)
	}
	if len(v1.changes) != 1 || len(v2.changes) != 1 {
		t.Fatalf("changes counts = %d, %d; want 1, 1", len(v1.changes), len(v2.changes))
	}
	if !v1.changes[0].state.minimized || v1.changes[0].state.lifecycle() != driver.LifecycleBackground {
		t.Fatalf("v1 minimize state = %+v", v1.changes[0].state)
	}
	if v2.changes[0].state != v1.changes[0].state {
		t.Fatalf("v2 minimize state = %+v, want %+v", v2.changes[0].state, v1.changes[0].state)
	}

	// Closing v1 must NOT remove root's subclass while v2 still observes it.
	mgr.detach(v1)
	if len(removed) != 0 {
		t.Fatalf("removed after detaching first view = %v, want empty", removed)
	}
	if _, ok := mgr.stateOf(v1); ok {
		t.Fatal("detached v1 still has tracked state")
	}

	// Restoring root reaches v2 only.
	if changed, _ := mgr.handleMessage(root, embeddedMsgSize, embeddedSizeRestored, 0); !changed {
		t.Fatal("restore did not report state change")
	}
	if len(v1.changes) != 1 || len(v2.changes) != 2 {
		t.Fatalf("changes after restore = %d, %d; want 1, 2", len(v1.changes), len(v2.changes))
	}
	if v2.changes[1].state.minimized || v2.changes[1].state.lifecycle() != driver.LifecycleActive {
		t.Fatalf("v2 restore state = %+v", v2.changes[1].state)
	}

	// Closing v2 removes the subclass once; repeated detach is a no-op.
	mgr.detach(v2)
	mgr.detach(v2)
	if !slices.Equal(removed, []uintptr{root}) {
		t.Fatalf("removed = %v, want [%x]", removed, root)
	}
}

func TestEmbeddedRootManagerNCDestroyCleanup(t *testing.T) {
	const root uintptr = 0x200
	var events []string
	mgr := newEmbeddedRootManager(embeddedRootHooks{
		install: func(r uintptr) bool {
			events = append(events, "install")
			return true
		},
		remove: func(r uintptr) {
			events = append(events, "remove")
		},
		readState: func(r uintptr) embeddedRootState {
			return initialEmbeddedRootState(true, false, true)
		},
	})

	v1 := &fakeRootWatcher{onDestroy: func(uintptr) { events = append(events, "destroy-v1") }}
	v2 := &fakeRootWatcher{onDestroy: func(uintptr) { events = append(events, "destroy-v2") }}
	mgr.attach(v1, root)
	mgr.attach(v2, root)

	changed, destroyed := mgr.handleMessage(root, embeddedMsgNCDestroy, 0, 0)
	if changed || !destroyed {
		t.Fatalf("WM_NCDESTROY handleMessage = (%v, %v), want (false, true)", changed, destroyed)
	}
	wantEvents := []string{"install", "remove", "destroy-v1", "destroy-v2"}
	if !slices.Equal(events, wantEvents) {
		t.Fatalf("events = %v, want %v", events, wantEvents)
	}
	if !slices.Equal(v1.destroyed, []uintptr{root}) || !slices.Equal(v2.destroyed, []uintptr{root}) {
		t.Fatalf("destroyed notifications = %v, %v", v1.destroyed, v2.destroyed)
	}
	if _, ok := mgr.stateOf(v1); ok {
		t.Fatal("v1 still attached after WM_NCDESTROY")
	}
	if _, ok := mgr.stateOf(v2); ok {
		t.Fatal("v2 still attached after WM_NCDESTROY")
	}

	// Later detaches and messages must not remove the subclass again or panic.
	mgr.detach(v1)
	mgr.detach(v2)
	if c, d := mgr.handleMessage(root, embeddedMsgSize, embeddedSizeMinimized, 0); c || d {
		t.Fatalf("post-destroy message = (%v, %v), want (false, false)", c, d)
	}
	if !slices.Equal(events, wantEvents) {
		t.Fatalf("events mutated after WM_NCDESTROY: %v", events)
	}
}

func TestEmbeddedRootManagerReparenting(t *testing.T) {
	const (
		rootA uintptr = 0x301
		rootB uintptr = 0x302
	)
	var installed, removed []uintptr
	initial := map[uintptr]embeddedRootState{
		rootA: initialEmbeddedRootState(true, false, true),
		rootB: initialEmbeddedRootState(true, true, false),
	}
	mgr := newEmbeddedRootManager(embeddedRootHooks{
		install: func(r uintptr) bool {
			installed = append(installed, r)
			return true
		},
		remove: func(r uintptr) {
			removed = append(removed, r)
		},
		readState: func(r uintptr) embeddedRootState {
			return initial[r]
		},
	})

	v1, v2 := &fakeRootWatcher{}, &fakeRootWatcher{}
	mgr.attach(v1, rootA)
	mgr.attach(v2, rootA)

	// Reparent v1 from rootA (active) to rootB (minimized).
	st, changed, attached := mgr.sync(v1, rootB)
	if !changed || !attached || !st.minimized || st.lifecycle() != driver.LifecycleBackground {
		t.Fatalf("sync(v1, rootB) = (%+v, %v, %v)", st, changed, attached)
	}
	if len(removed) != 0 {
		t.Fatalf("rootA removed while v2 still attached: %v", removed)
	}
	if !slices.Equal(installed, []uintptr{rootA, rootB}) {
		t.Fatalf("installed = %v, want [%x %x]", installed, rootA, rootB)
	}

	// Synchronizing to the same root is a no-op.
	if _, changed, attached := mgr.sync(v1, rootB); changed || !attached {
		t.Fatalf("no-op sync(v1, rootB) = (changed=%v, attached=%v), want (false, true)", changed, attached)
	}

	// rootA minimizes: only v2 hears it.
	mgr.handleMessage(rootA, embeddedMsgSize, embeddedSizeMinimized, 0)
	if len(v1.changes) != 0 || len(v2.changes) != 1 || v2.changes[0].root != rootA {
		t.Fatalf("after rootA minimize: v1=%+v v2=%+v", v1.changes, v2.changes)
	}

	// rootB restores and activates: only v1 hears it.
	mgr.handleMessage(rootB, embeddedMsgSize, embeddedSizeRestored, 0)
	mgr.handleMessage(rootB, embeddedMsgActivate, 1, 0)
	if len(v1.changes) != 2 || v1.changes[1].state.lifecycle() != driver.LifecycleActive || v1.changes[1].state.minimized {
		t.Fatalf("after rootB restore+activate: v1=%+v", v1.changes)
	}
	if len(v2.changes) != 1 {
		t.Fatalf("v2 unexpectedly heard rootB events: %+v", v2.changes)
	}

	// Reparent v2 to rootB as well: rootA's subclass is now removed, and rootB is NOT installed a second time.
	st, changed, attached = mgr.sync(v2, rootB)
	if !changed || !attached || st.minimized || st.lifecycle() != driver.LifecycleActive {
		t.Fatalf("sync(v2, rootB) = (%+v, %v, %v)", st, changed, attached)
	}
	if !slices.Equal(removed, []uintptr{rootA}) {
		t.Fatalf("removed = %v, want [%x]", removed, rootA)
	}
	if !slices.Equal(installed, []uintptr{rootA, rootB}) {
		t.Fatalf("rootB reinstalled on second reparent: %v", installed)
	}

	// Detach both via sync(0).
	if _, changed, attached := mgr.sync(v1, 0); !changed || attached {
		t.Fatalf("sync(v1, 0) = (changed=%v, attached=%v), want (true, false)", changed, attached)
	}
	if len(removed) != 1 {
		t.Fatalf("rootB removed early: %v", removed)
	}
	if _, changed, attached := mgr.sync(v2, 0); !changed || attached {
		t.Fatalf("sync(v2, 0) = (changed=%v, attached=%v), want (true, false)", changed, attached)
	}
	if !slices.Equal(removed, []uintptr{rootA, rootB}) {
		t.Fatalf("removed = %v, want [%x %x]", removed, rootA, rootB)
	}
}

func TestEmbeddedRootManagerInstallFailureAndReentrancy(t *testing.T) {
	const (
		failedRoot uintptr = 0x401
		liveRoot   uintptr = 0x402
	)
	var removed []uintptr
	mgr := newEmbeddedRootManager(embeddedRootHooks{
		install: func(r uintptr) bool { return r != failedRoot },
		remove:  func(r uintptr) { removed = append(removed, r) },
		readState: func(r uintptr) embeddedRootState {
			return initialEmbeddedRootState(true, r == failedRoot, r == liveRoot)
		},
	})

	// Failed install still returns the read initial state and never calls remove.
	vf := &fakeRootWatcher{}
	st, ok := mgr.attach(vf, failedRoot)
	if ok || !st.minimized || st.lifecycle() != driver.LifecycleBackground {
		t.Fatalf("failed attach = (%+v, %v)", st, ok)
	}
	mgr.detach(vf)
	if len(removed) != 0 {
		t.Fatalf("removed on failed attach = %v, want empty", removed)
	}

	// Re-entrant detach of self and sibling inside notification must not deadlock or deliver to detached sibling.
	v1, v2 := &fakeRootWatcher{}, &fakeRootWatcher{}
	v1.onChange = func(uintptr, embeddedRootState) {
		mgr.detach(v2)
		mgr.detach(v1)
	}
	mgr.attach(v1, liveRoot)
	mgr.attach(v2, liveRoot)
	mgr.handleMessage(liveRoot, embeddedMsgSize, embeddedSizeMinimized, 0)
	if len(v1.changes) != 1 || len(v2.changes) != 0 {
		t.Fatalf("re-entrant changes = v1:%d v2:%d, want 1, 0", len(v1.changes), len(v2.changes))
	}
	if !slices.Equal(removed, []uintptr{liveRoot}) {
		t.Fatalf("removed after re-entrant detach = %v, want [%x]", removed, liveRoot)
	}
}
