//go:build darwin

package apple

import (
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestEffectRegistryTargetsTheRootOnlyForCoveringShaders(t *testing.T) {
	var effects Effects[int]
	containers := 0
	host := EffectHost[int]{
		Root: 1, Scale: 2,
		Native: func(v int) objc.ID { return objc.ID(v * 10) },
		Container: func(v int) objc.ID {
			containers++
			return objc.ID(v * 100)
		},
	}
	fx := effects.ensure(2, host)
	for _, c := range []struct {
		role driver.ShaderRole
		view objc.ID
	}{
		{driver.ShaderEffect, 20},
		{driver.ShaderNav, 20},
		{driver.ShaderScreen, 10},
		{driver.ShaderScreenTransition, 10},
	} {
		fx.shader = &driver.Shader{Role: c.role}
		target := effects.target(2, host)
		if target.View != c.view || target.Root != 10 || target.Scale != 2 {
			t.Fatalf("role %v targets %+v", c.role, target)
		}
		if containers != 0 {
			t.Fatal("resolving a target eagerly made its container")
		}
		if got := target.Holder(); got != c.view*10 {
			t.Errorf("container = %d, want %d", got, c.view*10)
		}
		containers = 0
	}
	host.Root = 0
	if target := effects.target(2, host); target.View != 0 || target.Root != 0 || target.Holder != nil {
		t.Error("a covering shader without a root has a native target")
	}
}

func TestEffectRegistryReleasesCapturesAndReplacedViews(t *testing.T) {
	var effects Effects[int]
	host := EffectHost[int]{Current: func(v int) bool { return v == 2 }}
	capture := effects.ensure(1, host)
	if effects.ensure(1, host) != capture {
		t.Fatal("installing a transition discarded its existing capture")
	}
	effects.Set(1, &driver.Shader{}, host)
	if effects.Get(1) != nil {
		t.Fatal("a removed shader kept its uninstalled capture")
	}
	stale, current := effects.ensure(1, host), effects.ensure(2, host)
	effects.active = map[int]bool{1: true, 2: true}
	effects.Invalidate()
	if !stale.stale || !current.stale {
		t.Fatal("a surface change did not invalidate every installed effect")
	}
	// A replaced view is retired without resolving or drawing its old target.
	delete(effects.active, 2)
	effects.Draw(host)
	if effects.Get(1) != nil || effects.Get(2) != current || len(effects.active) != 0 {
		t.Fatal("retiring a stale view changed a current view's effect")
	}
	effects.End(2)
	effects.End(2)
	if len(effects.entries) != 0 {
		t.Fatal("the completed effects still retain their views")
	}
}
