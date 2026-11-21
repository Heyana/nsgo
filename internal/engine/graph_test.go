package engine_test

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

type noopPlatform struct{ core.ApproxTextEngine }

func (noopPlatform) Mount(*driver.MountBatch)  {}
func (noopPlatform) RequestFrame()             {}
func (noopPlatform) SubmitGPU(driver.GPUFrame) {}

func TestEngineGraphNumbersConcurrentEngines(t *testing.T) {
	// Disabled DevTools leaves Graph at 0.
	e0 := engine.Start(noopPlatform{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{Size: driver.Size{Width: 100, Height: 100}})
	e0.Sync()
	if got := e0.Graph(); got != 0 {
		e0.Close()
		t.Fatalf("Graph with devtools disabled = %d, want 0", got)
	}
	e0.Close()

	devtools.Enabled = true
	t.Cleanup(func() { devtools.Enabled = false })

	e1 := engine.Start(noopPlatform{}, core.AppRoot(func(cx *core.Scope) {
		cx.OpenWindow(core.WindowOptions{Title: "One", Width: 200, Height: 100}, func(*core.Scope) core.Node { return core.Text{Content: "one"} })
	}), engine.Options{Windows: &engine.WindowOptions{Width: 200, Height: 100}})
	defer e1.Close()

	e2 := engine.Start(noopPlatform{}, func(*core.Scope) core.Node { return core.Text{Content: "two"} }, engine.Options{Size: driver.Size{Width: 200, Height: 100}})
	defer e2.Close()

	e1.Sync()
	e2.Sync()

	g1, g2 := e1.Graph(), e2.Graph()
	if g1 <= 0 || g2 <= 0 || g1 == g2 {
		t.Fatalf("concurrent engines' graphs = (%d, %d), want distinct positive numbers", g1, g2)
	}
	// Attached child surface shares its engine's graph number.
	child := e1.Attach(1, noopPlatform{})
	if child.Graph() != g1 {
		t.Fatalf("attached surface Graph = %d, want %d", child.Graph(), g1)
	}
	child.Close()
	// Every surface registered in DevTools reports the matching graph ID.
	seen := map[int]int{}
	for _, s := range devtools.Surfaces() {
		seen[s.Graph()]++
	}
	if seen[g1] == 0 || seen[g2] == 0 {
		t.Fatalf("devtools surfaces by graph = %v, want entries for %d and %d", seen, g1, g2)
	}
}
