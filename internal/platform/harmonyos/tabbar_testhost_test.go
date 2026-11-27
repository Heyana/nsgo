//go:build js && wasm && nsgotest

package harmonyos

import (
	"syscall/js"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

type tabBarTestPlatform struct{ core.ApproxTextEngine }

func (tabBarTestPlatform) Mount(*driver.MountBatch)  {}
func (tabBarTestPlatform) RequestFrame()             {}
func (tabBarTestPlatform) SubmitGPU(driver.GPUFrame) {}

func TestSelectTabRoutesToNativeTabBar(t *testing.T) {
	selected := -1
	s := &surface{}
	s.m = newMounter(s)
	bridge := js.Global().Get("Object").New()
	submit := js.FuncOf(func(_ js.Value, _ []js.Value) any { return nil })
	defer submit.Release()
	bridge.Set("submit", submit)
	s.bridge = bridge

	var barID driver.NodeID
	s.engine = engine.Start(tabBarTestPlatform{}, func(cx *core.Scope) core.Node {
		cur := core.NewSignal(cx, 0)
		return core.TabBar{
			TestID: "tab-bar",
			Items: []core.TabBarItem{
				{Title: "Components"},
				{Title: "Basics"},
				{Title: "Settings"},
			},
			Selected: cur.Get,
			OnChange: func(i int) {
				selected = i
				cur.Set(i)
			},
		}
	}, engine.Options{
		Size:        driver.Size{Width: 390, Height: 844},
		Environment: driver.Environment{OS: driver.OSAndroid, Idiom: driver.IdiomPhone},
		Observe: func(_ uint32, b *driver.MountBatch) {
			s.m.mount(b)
			for _, it := range b.Items {
				if it.Kind == driver.ItemNative && it.Native != nil && it.Native.Component == driver.TabBarComponent {
					barID = it.ID
				}
			}
		},
	})
	defer s.engine.Close()
	s.engine.Sync()

	if barID == 0 {
		t.Fatal("core.TabBar did not mount a native tab bar view")
	}
	host := &testSurface{s: s}
	if err := host.SelectTab(barID, 2); err != nil {
		t.Fatalf("SelectTab(2) failed: %v", err)
	}
	s.engine.Sync()
	if selected != 2 {
		t.Fatalf("selected = %d, want 2", selected)
	}
	if err := host.SelectTab(barID, 9); err == nil {
		t.Fatal("SelectTab(9) succeeded for out-of-range index")
	}
}
