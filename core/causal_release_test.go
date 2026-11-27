//go:build !nsgodev

package core

import (
	"reflect"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestReleaseCausalPlaceholdersHaveZeroSize(t *testing.T) {
	for name, size := range map[string]uintptr{
		"causalState":      reflect.TypeFor[causalState]().Size(),
		"scopeCausal":      reflect.TypeFor[scopeCausal]().Size(),
		"signalCausal":     reflect.TypeFor[signalCausal]().Size(),
		"compCausal":       reflect.TypeFor[compCausal]().Size(),
		"cleanupCausal":    reflect.TypeFor[cleanupCausal]().Size(),
		"soundWaitCausal":  reflect.TypeFor[soundWaitCausal]().Size(),
		"signalWriteToken": reflect.TypeFor[signalWriteToken]().Size(),
		"compRunToken":     reflect.TypeFor[compRunToken]().Size(),
		"disposalToken":    reflect.TypeFor[disposalToken]().Size(),
	} {
		if size != 0 {
			t.Errorf("release placeholder %s has size %d, want 0", name, size)
		}
	}
}

func TestReleaseBuildRecordsNoCausalSurfaceOrTriggers(t *testing.T) {
	prevDev, prevTrace := devtools.Enabled, driver.TraceCauses
	devtools.Enabled, driver.TraceCauses = true, true
	defer func() {
		devtools.Enabled, driver.TraceCauses = prevDev, prevTrace
	}()

	root := NewRoot(RootOptions{CheckCommits: true, Post: func(func()) {}})
	defer root.Dispose()
	tree := NewTree(root, ApproxTextEngine{})
	tree.Resize(Size{Width: 100, Height: 100}, Geometry{})

	var s *Signal[int]
	tree.Mount(func(cx *Scope) Node {
		s = NewSignal(cx, 0)
		cx.Effect(func() { _ = s.Get() })
		return Text{ContentFunc: func() string {
			if s.Get() > 0 {
				return "on"
			}
			return "off"
		}}
	})
	tree.Commit()

	if _, ok := any(&devSurface{tree}).(devtools.CausalSurface); ok {
		t.Fatal("release devSurface must not implement devtools.CausalSurface")
	}

	s.Set(1)
	b := tree.Commit()
	if b == nil || b.Causes == nil {
		t.Fatal("commit recorded no Causes")
	}
	if len(b.Causes.Triggers) != 0 {
		t.Fatalf("release build populated Causes.Triggers = %+v", b.Causes.Triggers)
	}
}
