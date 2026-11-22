//go:build linux && !android

package linux

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestEmbeddedViewCloseReleasesRegistrations(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	host := startTestWindow(t, func(*core.Scope) core.Node { return core.View{} }, Options{Width: 200, Height: 200})
	host.onLoop(t, func() {
		var connected func(uintptr, uint64) int32
		purego.RegisterLibFunc(&connected, gobjectLib, "g_signal_handler_is_connected")
		for i := range 10 {
			v, err := NewView(func(*core.Scope) core.Node { return core.View{} }, Options{Appearance: driver.AppearanceDark})
			if err != nil {
				t.Error(err)
				return
			}
			w, surface := v.w, v.Handle()
			settings, contrast := slices.Clone(w.settingsHandlers), w.contrastHandler
			if len(settings) == 0 || windowOf(w.handle) != w {
				t.Error("the embedded view did not register its settings and handle")
			}
			v.Close()
			if windowOf(w.handle) != nil {
				t.Errorf("iteration %d: the closed view remains in windowHandles", i)
			}
			if _, ok := lookupWidget(surface); ok {
				t.Errorf("iteration %d: the closed surface remains in the widget table", i)
			}
			for _, id := range settings {
				if connected(gtkSettingsGetDefault(), id) != 0 {
					t.Errorf("iteration %d: GtkSettings handler %d remains connected", i, id)
				}
			}
			if contrast != 0 && connected(gnomeA11ySettingsForWatch(), contrast) != 0 {
				t.Errorf("iteration %d: the contrast handler remains connected", i)
			}
			if w.schemeProvider != 0 || v.Handle() != 0 {
				t.Errorf("iteration %d: the closed view retains its scoped style or native handle", i)
			}
		}
	})
}

type teardownComponent struct {
	widget uintptr
	closed int
}

func (c *teardownComponent) View() any         { return c.widget }
func (*teardownComponent) SetProp(string, any) {}
func (*teardownComponent) Command(string, any) {}
func (c *teardownComponent) Close()            { c.closed++ }

func TestEmbeddedViewCloseReleasesMountedTree(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	host := startTestWindow(t, func(*core.Scope) core.Node { return core.View{} }, Options{Width: 200, Height: 300})
	for iteration := range 3 {
		t.Run(fmt.Sprint(iteration), func(t *testing.T) {
			var embedded *View
			var component *teardownComponent
			host.onLoop(t, func() {
				var err error
				embedded, err = NewView(func(*core.Scope) core.Node {
					return core.View{Children: []core.Node{
						core.Text{Content: "Embedded text"}, core.Input{DefaultValue: "Field"},
						core.Native{Component: "teardown", Style: core.Style{Width: driver.Pt(40), Height: driver.Pt(30)}},
					}}
				}, Options{Components: driver.NativeComponents{"teardown": func(*driver.NativeContext) driver.NativeComponent {
					component = &teardownComponent{widget: gtkBoxNew(0, 0)}
					return component
				}}})
				if err != nil {
					t.Error(err)
					return
				}
				gtkWidgetSetSizeRequest(embedded.Handle(), 160, 100)
				gtkBoxAppend(host.box, embedded.Handle())
			})
			if embedded == nil {
				t.Fatal("the embedded view did not open")
			}
			// The host owns the parent reference, including after Close.
			surface := embedded.Handle()
			t.Cleanup(func() {
				host.onLoop(t, func() {
					if surface != 0 {
						gtkWidgetUnparent(surface)
					}
				})
			})
			for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
				var ready bool
				host.onLoop(t, func() { ready = component != nil })
				if ready {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("the embedded app did not mount")
				}
			}
			host.onLoop(t, func() {
				w := embedded.w
				var find func(uintptr, uint32, uint32, uint32, uintptr, uintptr, uintptr) uint64
				var connected func(uintptr, uint64) int32
				purego.RegisterLibFunc(&find, gobjectLib, "g_signal_handler_find")
				purego.RegisterLibFunc(&connected, gobjectLib, "g_signal_handler_is_connected")
				var activity []signal
				for _, object := range []uintptr{host.win, gtkNativeGetSurface(gtkWidgetGetNative(surface))} {
					for _, data := range []uintptr{w.handle, surface} {
						const matchData = 1 << 4 // G_SIGNAL_MATCH_DATA
						if id := find(object, matchData, 0, 0, 0, 0, data); id != 0 {
							activity = append(activity, signal{object, id})
						}
					}
				}
				if len(activity) == 0 {
					t.Error("the embedded surface registered no host activity handlers")
				}
				for _, v := range w.views {
					if v != nil && v.text != nil {
						driver.HitTest(v.text, driver.Point{})
					}
				}
				var weakRef func(uintptr, uintptr, uintptr)
				purego.RegisterLibFunc(&weakRef, gobjectLib, "g_object_weak_ref")
				released := make(map[uintptr]bool)
				notify := purego.NewCallback(func(data, object uintptr) { released[object] = true })
				var contexts []uintptr
				for _, group := range [][2]uintptr{w.measureMap, w.hitMap} {
					for _, context := range group {
						if context != 0 {
							contexts = append(contexts, context)
							weakRef(context, notify, 0)
						}
					}
				}
				embedded.Close()
				embedded.Close()
				if component.closed != 1 {
					t.Errorf("component closed %d times, want once", component.closed)
				}
				for _, v := range w.views {
					if v != nil {
						t.Errorf("the closed embedded tree retains view %d", v.id)
					}
				}
				if w.measureMap != [2]uintptr{} || w.hitMap != [2]uintptr{} {
					t.Error("the closed view retains its text contexts")
				}
				for _, context := range contexts {
					if !released[context] {
						t.Errorf("text context %#x was not finalized", context)
					}
				}
				if gtkWidgetGetParent(surface) != host.box {
					t.Error("Close removed the widget from its host")
				}
				if gtkWidgetGetVisible(host.win) == 0 {
					t.Error("Close destroyed the host's window")
				}
				gtkWidgetUnparent(surface)
				surface = 0
				for _, s := range activity {
					if connected(s.instance, s.id) != 0 {
						t.Error("the removed embedded widget left an activity handler on its host")
					}
				}
			})
		})
	}
}

func TestTextContextsReleaseTheirFontMaps(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	host := startTestWindow(t, func(*core.Scope) core.Node { return core.View{} }, Options{Width: 100, Height: 100})
	host.onLoop(t, func() {
		var getMap func(uintptr) uintptr
		var weakRef func(uintptr, uintptr, uintptr)
		purego.RegisterLibFunc(&getMap, pangoLib, "pango_context_get_font_map")
		purego.RegisterLibFunc(&weakRef, gobjectLib, "g_object_weak_ref")
		released := make(map[uintptr]bool)
		notify := purego.NewCallback(func(data, object uintptr) { released[object] = true })
		w := &window{}
		for _, rtl := range []bool{false, true} {
			context := w.measureContext(rtl)
			fontMap := getMap(context)
			released[fontMap] = false
			weakRef(fontMap, notify, 0)
			gObjectUnref(context)
			if !released[fontMap] {
				t.Errorf("RTL=%v: the text context's font map still has an owner after the context is released", rtl)
			}
		}
	})
}

func TestEmbeddedViewCloseReleasesLeavingViews(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	host := startTestWindow(t, func(*core.Scope) core.Node { return core.View{} }, Options{Width: 200, Height: 200})
	var embedded *View
	host.onLoop(t, func() {
		var err error
		embedded, err = NewView(func(*core.Scope) core.Node { return core.View{} }, Options{})
		if err != nil {
			t.Error(err)
			return
		}
		gtkWidgetSetSizeRequest(embedded.Handle(), 160, 100)
		gtkBoxAppend(host.box, embedded.Handle())
	})
	if embedded == nil {
		t.Fatal("the embedded view did not open")
	}
	surface := embedded.Handle()
	t.Cleanup(func() { host.onLoop(t, func() { gtkWidgetUnparent(surface) }) })
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		var ready bool
		host.onLoop(t, func() { ready = embedded.w.view(embedded.w.root) != nil })
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the embedded app did not mount")
		}
	}
	host.onLoop(t, func() {
		w := embedded.w
		leaving, inner := driver.NodeID(len(w.views)), driver.NodeID(len(w.views)+1)
		frame := driver.Bounds{Size: driver.Size{Width: 40, Height: 30}}
		w.apply(&driver.MountBatch{Root: w.root, Items: []driver.MountItem{
			{Kind: driver.ItemCreate, ID: leaving, View: driver.KindView},
			{Kind: driver.ItemFrame, ID: leaving, Frame: frame},
			{Kind: driver.ItemInsert, ID: leaving, Parent: w.root},
			{Kind: driver.ItemCreate, ID: inner, View: driver.KindView},
			{Kind: driver.ItemFrame, ID: inner, Frame: frame},
			{Kind: driver.ItemInsert, ID: inner, Parent: leaving},
		}})
		var views []*view
		var widgets []uintptr
		for _, id := range []driver.NodeID{leaving, inner} {
			v := w.view(id)
			if v == nil || v.widget == 0 {
				t.Errorf("view %d did not mount", id)
				return
			}
			views, widgets = append(views, v), append(widgets, v.widget)
		}
		w.apply(&driver.MountBatch{Root: w.root, LayoutAnimation: &driver.LayoutAnimation{
			Delete: &driver.LayoutTransition{Duration: time.Hour},
		}, Items: []driver.MountItem{
			{Kind: driver.ItemDelete, ID: leaving},
			{Kind: driver.ItemDelete, ID: inner},
		}})
		if !slices.Contains(w.layouts, views[0]) || !views[0].dying || !views[1].dying {
			t.Fatal("the deleted view is not leaving")
		}
		embedded.Close()
		if len(w.layouts) != 0 {
			t.Error("the closed surface keeps its layout animations")
		}
		for i, v := range views {
			if v.widget != 0 || v.dying {
				t.Errorf("leaving view %d was not released", v.id)
			}
			if _, ok := lookupWidget(widgets[i]); ok {
				t.Errorf("leaving view %d remains in the widget table", v.id)
			}
		}
	})
}
