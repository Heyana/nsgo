//go:build nsgodev

package base

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/devtools"
)

type testNavRoute struct {
	Name string
	ID   int
}

type unexportedRoute struct {
	name string
}

type funcRoute struct {
	Name string
	Run  func()
}

func TestNavSurvivesDevRestart(t *testing.T) {
	devtools.LoadState(nil)
	t.Cleanup(func() { devtools.LoadState(nil) })

	root1 := core.NewRoot(core.RootOptions{Post: func(func()) {}})
	nav1 := NewNav(root1, testNavRoute{Name: "home"})
	nav1.Push(testNavRoute{Name: "settings", ID: 2})
	nav1.Push(testNavRoute{Name: "profile", ID: 9})
	snap := devtools.SnapshotState()
	root1.Dispose()

	// Simulated restart: reload snapshot into fresh state.
	devtools.LoadState(nil)
	devtools.LoadState(snap)

	root2 := core.NewRoot(core.RootOptions{Post: func(func()) {}})
	defer root2.Dispose()
	nav2 := NewNav(root2, testNavRoute{Name: "home"})
	want := []testNavRoute{{Name: "home"}, {Name: "settings", ID: 2}, {Name: "profile", ID: 9}}
	if got := nav2.Routes(); !slices.Equal(got, want) {
		t.Fatalf("restored nav routes = %+v, want %+v", got, want)
	}

	// Popping updates the saved stack for subsequent restarts.
	nav2.Pop(1)
	snap2 := devtools.SnapshotState()
	devtools.LoadState(nil)
	devtools.LoadState(snap2)

	root3 := core.NewRoot(core.RootOptions{Post: func(func()) {}})
	defer root3.Dispose()
	nav3 := NewNav(root3, testNavRoute{Name: "home"})
	want2 := []testNavRoute{{Name: "home"}, {Name: "settings", ID: 2}}
	if got := nav3.Routes(); !slices.Equal(got, want2) {
		t.Fatalf("restored nav routes after pop = %+v, want %+v", got, want2)
	}
}

func TestMultipleNavsKeepIndependentStacks(t *testing.T) {
	devtools.LoadState(nil)
	t.Cleanup(func() { devtools.LoadState(nil) })

	root1 := core.NewRoot(core.RootOptions{Post: func(func()) {}})
	left1 := NewNav(root1, "inbox")
	right1 := NewNav(root1, "overview")
	left1.Push("thread-42")
	right1.Push("metrics")
	right1.Push("cpu")
	snap := devtools.SnapshotState()
	root1.Dispose()

	devtools.LoadState(nil)
	devtools.LoadState(snap)

	root2 := core.NewRoot(core.RootOptions{Post: func(func()) {}})
	defer root2.Dispose()
	left2 := NewNav(root2, "inbox")
	right2 := NewNav(root2, "overview")
	if got := left2.Routes(); !slices.Equal(got, []string{"inbox", "thread-42"}) {
		t.Fatalf("left nav = %v, want [inbox thread-42]", got)
	}
	if got := right2.Routes(); !slices.Equal(got, []string{"overview", "metrics", "cpu"}) {
		t.Fatalf("right nav = %v, want [overview metrics cpu]", got)
	}
}

func TestNavDegradesGracefullyForUnserializableRoutes(t *testing.T) {
	devtools.LoadState(nil)
	t.Cleanup(func() { devtools.LoadState(nil) })

	root1 := core.NewRoot(core.RootOptions{Post: func(func()) {}})
	nAny := NewNav[any](root1, testNavRoute{Name: "home"})
	nAny.Push(testNavRoute{Name: "detail", ID: 1})
	nUnexported := NewNav(root1, unexportedRoute{name: "home"})
	nUnexported.Push(unexportedRoute{name: "next"})
	nFunc := NewNav(root1, funcRoute{Name: "home", Run: func() {}})
	nFunc.Push(funcRoute{Name: "next", Run: func() {}})
	snap := devtools.SnapshotState()
	root1.Dispose()

	if len(snap) != 0 {
		t.Fatalf("unserializable nav routes saved unexpected keys: %v", snap)
	}

	devtools.LoadState(snap)
	root2 := core.NewRoot(core.RootOptions{Post: func(func()) {}})
	defer root2.Dispose()
	nAny2 := NewNav[any](root2, testNavRoute{Name: "home"})
	if got := nAny2.Routes(); len(got) != 1 || got[0] != (testNavRoute{Name: "home"}) {
		t.Fatalf("any nav corrupted on restart: %#v", got)
	}
	nUnexported2 := NewNav(root2, unexportedRoute{name: "home"})
	if got := nUnexported2.Routes(); len(got) != 1 || got[0].name != "home" {
		t.Fatalf("unexported struct nav corrupted on restart: %#v", got)
	}
	nFunc2 := NewNav(root2, funcRoute{Name: "home"})
	if got := nFunc2.Routes(); len(got) != 1 || got[0].Name != "home" {
		t.Fatalf("func struct nav corrupted on restart: %#v", got)
	}
}
