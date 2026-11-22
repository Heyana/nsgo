//go:build nsgodev

package dev

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// overlayHarness drives synchronous multi-surface trees (a desktop app with windows/panels, plus optional embedded-view graphs) with a fresh shared overlayController.
type overlayHarness struct {
	t      *testing.T
	ctrl   *overlayController
	queue  []func()
	out    *devOut
	mu     sync.Mutex
	msgs   []devMessage
	server *httptest.Server
}

func newOverlayHarness(t *testing.T, withDevtools, highlight bool, layout core.LayoutAudit) *overlayHarness {
	t.Helper()
	h := &overlayHarness{t: t}
	h.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_nsgo/devtools/out" {
			return
		}
		sc := bufio.NewScanner(r.Body)
		sc.Buffer(make([]byte, 1<<16), 4<<20)
		for sc.Scan() {
			var m devMessage
			if json.Unmarshal(sc.Bytes(), &m) == nil {
				h.mu.Lock()
				h.msgs = append(h.msgs, m)
				h.mu.Unlock()
			}
		}
	}))
	t.Cleanup(h.server.Close)

	prevDevtools := devDevtools
	prevEnabled := devtools.Enabled
	prevMount := devtools.MountOverlay
	prevCtrl := devOverlays.Load()
	prevChan := devChannel

	if withDevtools {
		devDevtools = "1"
		devtools.Enabled = true
	} else {
		devDevtools = ""
		devtools.Enabled = false
	}
	h.out = &devOut{server: h.server.URL, wake: make(chan struct{}, 1), hurry: make(chan struct{}, 1)}
	go h.out.run()
	devChannel = h.out
	h.ctrl = startDevOverlay(highlight, layout)
	h.ctrl.layoutOut = h.out

	t.Cleanup(func() {
		devtools.MountOverlay = prevMount
		devOverlays.Store(prevCtrl)
		devChannel = prevChan
		devDevtools = prevDevtools
		devtools.Enabled = prevEnabled
	})
	return h
}

func (h *overlayHarness) post(fn func()) {
	h.queue = append(h.queue, fn)
}

func (h *overlayHarness) drain(trees ...*core.Tree) []*driver.MountBatch {
	var batches []*driver.MountBatch
	for range 16 {
		ran := false
		for len(h.queue) > 0 {
			fn := h.queue[0]
			h.queue = h.queue[1:]
			fn()
			ran = true
		}
		committed := false
		for _, tr := range trees {
			if tr == nil || tr.Closed() {
				continue
			}
			if b := tr.Commit(); b != nil {
				batches = append(batches, b)
				committed = true
			}
		}
		if !ran && !committed {
			break
		}
	}
	return batches
}

func (h *overlayHarness) flushMessages(want func([]devMessage) bool) []devMessage {
	h.t.Helper()
	// Hurry devOut so tests don't wait 250ms unnecessarily.
	select {
	case h.out.hurry <- struct{}{}:
	default:
	}
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		h.mu.Lock()
		got := slices.Clone(h.msgs)
		h.mu.Unlock()
		if want(got) {
			return got
		}
		select {
		case h.out.hurry <- struct{}{}:
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.t.Fatalf("timed out waiting for DevTools messages; got %+v", h.msgs)
	return nil
}

// surfaceModel tracks the views created/updated/deleted in a tree's mount batches, so tests can inspect overlays, canvases, pick catchers, and keymaps without reaching into core internals.
type surfaceModel struct {
	tree    *core.Tree
	views   map[core.NodeID]driver.ViewProps
	frames  map[core.NodeID]core.Bounds
	parents map[core.NodeID]core.NodeID
	styles  map[core.NodeID]driver.ViewStyle
	ops     map[core.NodeID][]driver.DrawOp
	keymap  []driver.KeyBinding
}

func newSurfaceModel(t *core.Tree) *surfaceModel {
	return &surfaceModel{
		tree:    t,
		views:   map[core.NodeID]driver.ViewProps{},
		frames:  map[core.NodeID]core.Bounds{},
		parents: map[core.NodeID]core.NodeID{},
		styles:  map[core.NodeID]driver.ViewStyle{},
		ops:     map[core.NodeID][]driver.DrawOp{},
	}
}

func (m *surfaceModel) apply(b *driver.MountBatch) {
	if b == nil {
		return
	}
	for _, it := range b.Items {
		switch it.Kind {
		case driver.ItemCreate:
			if it.ViewProps != nil {
				m.views[it.ID] = *it.ViewProps
			} else {
				m.views[it.ID] = driver.ViewProps{}
			}
		case driver.ItemViewProps:
			if it.ViewProps != nil {
				m.views[it.ID] = *it.ViewProps
			}
		case driver.ItemFrame:
			m.frames[it.ID] = it.Frame
		case driver.ItemInsert:
			m.parents[it.ID] = it.Parent
		case driver.ItemViewStyle:
			if it.ViewStyle != nil {
				m.styles[it.ID] = *it.ViewStyle
			}
		case driver.ItemCanvas:
			if it.Drawing != nil {
				m.ops[it.ID] = it.Drawing.Ops
			}
		case driver.ItemKeymap:
			m.keymap = it.Keys
		case driver.ItemDelete:
			delete(m.views, it.ID)
			delete(m.frames, it.ID)
			delete(m.parents, it.ID)
			delete(m.styles, it.ID)
			delete(m.ops, it.ID)
		}
	}
}

func (m *surfaceModel) findByTestID(testID string) (core.NodeID, bool) {
	for id, p := range m.views {
		if p.TestID == testID {
			return id, true
		}
	}
	return 0, false
}

func (m *surfaceModel) hasTestID(testID string) bool {
	_, ok := m.findByTestID(testID)
	return ok
}

// highlightBoxes counts the blue 2pt update highlight outlines currently mounted under nsgo-update-highlights.
func (m *surfaceModel) highlightBoxes() int {
	overlayID, ok := m.findByTestID(devtools.OverlayTestID)
	if !ok {
		return 0
	}
	n := 0
	for id, st := range m.styles {
		if m.parents[id] == overlayID && st.BorderWidth == 2 && st.BorderColor == core.RGB(0x2f9bff) {
			n++
		}
	}
	return n
}

func (m *surfaceModel) pressKey(t *testing.T, key string) {
	t.Helper()
	id, _ := driver.FindKey(m.keymap, []string{key}, 0, false)
	if id == 0 {
		t.Fatalf("keymap %+v lacks %q", m.keymap, key)
	}
	m.tree.Dispatch(driver.Event{Kind: driver.EventKey, Count: id})
}

func drainModels(h *overlayHarness, models ...*surfaceModel) {
	for range 16 {
		ran := false
		for len(h.queue) > 0 {
			fn := h.queue[0]
			h.queue = h.queue[1:]
			fn()
			ran = true
		}
		committed := false
		for _, m := range models {
			if m == nil || m.tree == nil || m.tree.Closed() {
				continue
			}
			if b := m.tree.Commit(); b != nil {
				m.apply(b)
				committed = true
			}
		}
		if !ran && !committed {
			break
		}
	}
}

func surfaceTree(app *core.Tree, w *core.Window) *core.Tree {
	for _, s := range app.Surfaces() {
		if s.Window() == w {
			return s
		}
	}
	return nil
}

// Every surface opened in a dev build — main window, secondary window, FitContent panel, and embedded view — mounts DevTools' overlay, leaving the hidden app coordinator tree without one, and -highlight / DevTools highlight toggles apply across all of them.
func TestDevOverlayMountsAndHighlightsEverySurface(t *testing.T) {
	h := newOverlayHarness(t, true, true, core.LayoutAudit{})
	root := core.NewRoot(core.RootOptions{Post: h.post, Windows: &core.WindowOptions{Width: 400, Height: 300}})
	appTree := core.NewTree(root, core.ApproxTextEngine{})
	defer root.Dispose()

	var mainSig, secSig, panelSig, embedSig *core.Signal[int]
	var secWin, panel *core.Window
	var appScope *core.Scope

	appTree.Mount(core.AppRoot(func(cx *core.Scope) {
		appScope = cx
		cx.SetMainView(func(cx *core.Scope) core.Node {
			mainSig = core.NewSignal(cx, 0)
			return core.Text{TestID: "main-text", ContentFunc: func() string { return "M" + strconv.Itoa(mainSig.Get()) }}
		})
		secWin = cx.OpenWindow(core.WindowOptions{Title: "Secondary", Width: 320, Height: 240}, func(cx *core.Scope) core.Node {
			secSig = core.NewSignal(cx, 0)
			return core.Text{TestID: "sec-text", ContentFunc: func() string { return "S" + strconv.Itoa(secSig.Get()) }}
		})
		panel = cx.OpenPanel(core.PanelOptions{Width: 300, Height: 200, FitContent: true}, func(cx *core.Scope) core.Node {
			panelSig = core.NewSignal(cx, 0)
			return core.View{TestID: "panel-card", Style: core.Style{Width: core.Pt(180), Height: core.Pt(90)}, Children: []core.Node{
				core.Text{TestID: "panel-text", ContentFunc: func() string { return "P" + strconv.Itoa(panelSig.Get()) }},
			}}
		})
	}))

	// Separate graph simulating nsgo.NewView (its own root + tree ID 0).
	embedRoot := core.NewRoot(core.RootOptions{Post: h.post})
	embedTree := core.NewTree(embedRoot, core.ApproxTextEngine{})
	defer embedRoot.Dispose()
	embedTree.Resize(core.Size{Width: 260, Height: 160}, core.Geometry{})
	embedTree.Mount(func(cx *core.Scope) core.Node {
		embedSig = core.NewSignal(cx, 0)
		return core.Text{TestID: "embed-text", ContentFunc: func() string { return "E" + strconv.Itoa(embedSig.Get()) }}
	})

	appModel := newSurfaceModel(appTree)
	mainModel := newSurfaceModel(surfaceTree(appTree, root.MainWindow()))
	secModel := newSurfaceModel(surfaceTree(appTree, secWin))
	panelModel := newSurfaceModel(surfaceTree(appTree, panel))
	embedModel := newSurfaceModel(embedTree)
	all := []*surfaceModel{appModel, mainModel, secModel, panelModel, embedModel}
	drainModels(h, all...)

	if appModel.hasTestID(devtools.OverlayTestID) {
		t.Fatal("hidden app tree (tree 0 with Windows) should not mount DevTools' overlay")
	}
	for name, m := range map[string]*surfaceModel{"main": mainModel, "secondary": secModel, "panel": panelModel, "embedded": embedModel} {
		if !m.hasTestID(devtools.OverlayTestID) {
			t.Errorf("%s surface did not mount DevTools' overlay", name)
		}
		if m.highlightBoxes() == 0 {
			t.Errorf("%s surface did not outline its initial commit under -highlight", name)
		}
	}
	// FitContent panel keeps its content size even with DevTools' overlay mounted.
	if got := panelModel.frames[1].Size; got != (core.Size{Width: 180, Height: 90}) {
		t.Errorf("FitContent panel root size = %v, want 180×90", got)
	}

	// Turn highlight OFF through DevTools command: applies to every surface.
	runDevCommand(devCommand{Op: "highlight", On: false}, h.out)
	drainModels(h, all...)
	before := map[string]int{
		"main": mainModel.highlightBoxes(), "secondary": secModel.highlightBoxes(),
		"panel": panelModel.highlightBoxes(), "embedded": embedModel.highlightBoxes(),
	}
	mainSig.Set(1)
	secSig.Set(1)
	panelSig.Set(1)
	embedSig.Set(1)
	drainModels(h, all...)
	for name, m := range map[string]*surfaceModel{"main": mainModel, "secondary": secModel, "panel": panelModel, "embedded": embedModel} {
		if got := m.highlightBoxes(); got != before[name] {
			t.Errorf("%s surface outlined a commit after highlight was turned off: before=%d after=%d", name, before[name], got)
		}
	}

	// Turn highlight back ON: applies to existing surfaces AND a newly opened window.
	runDevCommand(devCommand{Op: "highlight", On: true}, h.out)
	drainModels(h, all...)
	var lateSig *core.Signal[int]
	lateWin := appScope.OpenWindow(core.WindowOptions{Title: "Late", Width: 240, Height: 180}, func(cx *core.Scope) core.Node {
		lateSig = core.NewSignal(cx, 0)
		return core.Text{TestID: "late-text", ContentFunc: func() string { return "L" + strconv.Itoa(lateSig.Get()) }}
	})
	lateModel := newSurfaceModel(surfaceTree(appTree, lateWin))
	all = append(all, lateModel)
	drainModels(h, all...)

	if !lateModel.hasTestID(devtools.OverlayTestID) || lateModel.highlightBoxes() == 0 {
		t.Fatalf("late window did not inherit highlight=on: overlay=%v boxes=%d", lateModel.hasTestID(devtools.OverlayTestID), lateModel.highlightBoxes())
	}
	secBefore := secModel.highlightBoxes()
	embedBefore := embedModel.highlightBoxes()
	secSig.Set(2)
	embedSig.Set(2)
	drainModels(h, all...)
	if secModel.highlightBoxes() <= secBefore {
		t.Errorf("secondary window did not outline update after highlight was turned back on")
	}
	if embedModel.highlightBoxes() <= embedBefore {
		t.Errorf("embedded view did not outline update after highlight was turned back on")
	}
}

// Layout audit (-layout) mounts across every surface, syncs keyboard toggles (mod+alt+shift+l) and DevTools Layout commands across windows, panels and embedded views, preserves config for windows opened while hidden, and yields to an app's own binding of the key.
func TestDevOverlayLayoutSyncsAcrossSurfacesAndKeyboard(t *testing.T) {
	h := newOverlayHarness(t, true, false, core.LayoutAudit{Grid: 4})
	root := core.NewRoot(core.RootOptions{Post: h.post, Windows: &core.WindowOptions{Width: 400, Height: 300}})
	appTree := core.NewTree(root, core.ApproxTextEngine{})
	defer root.Dispose()

	var secWin, panel, customKeyWin *core.Window
	var appScope *core.Scope
	appOwnKey := 0
	appTree.Mount(core.AppRoot(func(cx *core.Scope) {
		appScope = cx
		cx.SetMainView(func(cx *core.Scope) core.Node { return core.View{TestID: "main"} })
		secWin = cx.OpenWindow(core.WindowOptions{Title: "Secondary"}, func(cx *core.Scope) core.Node { return core.View{TestID: "sec"} })
		panel = cx.OpenPanel(core.PanelOptions{Width: 260, Height: 180}, func(cx *core.Scope) core.Node { return core.View{TestID: "panel"} })
		customKeyWin = cx.OpenWindow(core.WindowOptions{Title: "CustomKey"}, func(cx *core.Scope) core.Node {
			cx.OnKey("mod+alt+shift+l", func() { appOwnKey++ })
			return core.View{TestID: "custom"}
		})
	}))

	embedRoot := core.NewRoot(core.RootOptions{Post: h.post})
	embedTree := core.NewTree(embedRoot, core.ApproxTextEngine{})
	defer embedRoot.Dispose()
	embedTree.Resize(core.Size{Width: 240, Height: 160}, core.Geometry{})
	embedTree.Mount(func(cx *core.Scope) core.Node { return core.View{TestID: "embed"} })

	mainModel := newSurfaceModel(surfaceTree(appTree, root.MainWindow()))
	secModel := newSurfaceModel(surfaceTree(appTree, secWin))
	panelModel := newSurfaceModel(surfaceTree(appTree, panel))
	customModel := newSurfaceModel(surfaceTree(appTree, customKeyWin))
	embedModel := newSurfaceModel(embedTree)
	all := []*surfaceModel{newSurfaceModel(appTree), mainModel, secModel, panelModel, customModel, embedModel}
	drainModels(h, all...)

	for name, m := range map[string]*surfaceModel{"main": mainModel, "secondary": secModel, "panel": panelModel, "embedded": embedModel} {
		if !m.hasTestID("nsgo-layout-audit") {
			t.Fatalf("%s surface did not start with layout audit shown", name)
		}
	}

	// An app's own mod+alt+shift+l binding wins over DevTools' audit key.
	customModel.pressKey(t, "mod+alt+shift+l")
	drainModels(h, all...)
	if appOwnKey != 1 || !mainModel.hasTestID("nsgo-layout-audit") {
		t.Fatalf("appOwnKey=%d, main audit shown=%v: app's own binding should win", appOwnKey, mainModel.hasTestID("nsgo-layout-audit"))
	}

	// Pressing mod+alt+shift+l in the secondary window hides the audit across ALL surfaces.
	secModel.pressKey(t, "mod+alt+shift+l")
	drainModels(h, all...)
	for name, m := range map[string]*surfaceModel{"main": mainModel, "secondary": secModel, "panel": panelModel, "embedded": embedModel} {
		if m.hasTestID("nsgo-layout-audit") {
			t.Errorf("%s surface kept layout audit shown after mod+alt+shift+l in secondary window", name)
		}
	}
	h.flushMessages(func(msgs []devMessage) bool {
		return slices.ContainsFunc(msgs, func(m devMessage) bool { return m.Type == "layout" && m.Data == false })
	})

	// A window opened while hidden starts hidden, yet still binds mod+alt+shift+l and re-shows everywhere with the original grid=4 config.
	lateWin := appScope.OpenWindow(core.WindowOptions{Title: "Late"}, func(cx *core.Scope) core.Node { return core.View{TestID: "late"} })
	lateModel := newSurfaceModel(surfaceTree(appTree, lateWin))
	all = append(all, lateModel)
	drainModels(h, all...)
	if lateModel.hasTestID("nsgo-layout-audit") {
		t.Fatal("window opened while audit is hidden should start with audit hidden")
	}

	lateModel.pressKey(t, "mod+alt+shift+l")
	drainModels(h, all...)
	for name, m := range map[string]*surfaceModel{"main": mainModel, "secondary": secModel, "panel": panelModel, "embedded": embedModel, "late": lateModel} {
		if !m.hasTestID("nsgo-layout-audit") {
			t.Errorf("%s surface did not re-show layout audit after mod+alt+shift+l in late window", name)
		}
	}

	// DevTools layout command hides and shows across all surfaces too.
	runDevCommand(devCommand{Op: "layout", On: false}, h.out)
	drainModels(h, all...)
	for name, m := range map[string]*surfaceModel{"main": mainModel, "secondary": secModel, "panel": panelModel, "embedded": embedModel, "late": lateModel} {
		if m.hasTestID("nsgo-layout-audit") {
			t.Errorf("%s surface did not hide layout audit on DevTools layout=false", name)
		}
	}
	runDevCommand(devCommand{Op: "layout", On: true}, h.out)
	drainModels(h, all...)
	for name, m := range map[string]*surfaceModel{"main": mainModel, "secondary": secModel, "panel": panelModel, "embedded": embedModel, "late": lateModel} {
		if !m.hasTestID("nsgo-layout-audit") {
			t.Errorf("%s surface did not show layout audit on DevTools layout=true", name)
		}
	}
}

// Pick activates across every surface (windows, panels and multiple embedded views with colliding tree ID 0), reports the exact surface and node ID of the picked view without pressing it, and ends Pick across all other surfaces.
func TestDevOverlayPickAcrossWindowsPanelsAndEmbeddedViews(t *testing.T) {
	h := newOverlayHarness(t, true, false, core.LayoutAudit{})
	root := core.NewRoot(core.RootOptions{Post: h.post, Windows: &core.WindowOptions{Width: 300, Height: 200}})
	appTree := core.NewTree(root, core.ApproxTextEngine{})
	defer root.Dispose()

	pressed := 0
	var secWin, panel *core.Window
	appTree.Mount(core.AppRoot(func(cx *core.Scope) {
		cx.SetMainView(func(cx *core.Scope) core.Node {
			return core.View{TestID: "main-btn", Style: core.Style{Width: core.Pt(100), Height: core.Pt(40)}, OnPress: func(core.PressEvent) { pressed++ }}
		})
		secWin = cx.OpenWindow(core.WindowOptions{Title: "Secondary", Width: 300, Height: 200}, func(cx *core.Scope) core.Node {
			return core.View{TestID: "sec-btn", Style: core.Style{Width: core.Pt(100), Height: core.Pt(40)}, OnPress: func(core.PressEvent) { pressed++ }}
		})
		panel = cx.OpenPanel(core.PanelOptions{Width: 240, Height: 120}, func(cx *core.Scope) core.Node {
			return core.View{TestID: "panel-btn", Style: core.Style{Width: core.Pt(100), Height: core.Pt(40)}, OnPress: func(core.PressEvent) { pressed++ }}
		})
	}))

	// Two separate nsgo.NewView-style graphs, BOTH having tree ID 0!
	embed1Root := core.NewRoot(core.RootOptions{Post: h.post})
	embed1Tree := core.NewTree(embed1Root, core.ApproxTextEngine{})
	defer embed1Root.Dispose()
	embed1Tree.Resize(core.Size{Width: 200, Height: 100}, core.Geometry{})
	embed1Tree.Mount(func(cx *core.Scope) core.Node {
		return core.View{TestID: "embed1-btn", Style: core.Style{Width: core.Pt(100), Height: core.Pt(40)}, OnPress: func(core.PressEvent) { pressed++ }}
	})

	embed2Root := core.NewRoot(core.RootOptions{Post: h.post})
	embed2Tree := core.NewTree(embed2Root, core.ApproxTextEngine{})
	defer embed2Root.Dispose()
	embed2Tree.Resize(core.Size{Width: 200, Height: 100}, core.Geometry{})
	embed2Tree.Mount(func(cx *core.Scope) core.Node {
		return core.View{TestID: "embed2-btn", Style: core.Style{Width: core.Pt(100), Height: core.Pt(40)}, OnPress: func(core.PressEvent) { pressed++ }}
	})

	mainModel := newSurfaceModel(surfaceTree(appTree, root.MainWindow()))
	secModel := newSurfaceModel(surfaceTree(appTree, secWin))
	panelModel := newSurfaceModel(surfaceTree(appTree, panel))
	embed1Model := newSurfaceModel(embed1Tree)
	embed2Model := newSurfaceModel(embed2Tree)
	all := []*surfaceModel{newSurfaceModel(appTree), mainModel, secModel, panelModel, embed1Model, embed2Model}
	drainModels(h, all...)

	// All 5 listed surfaces must get distinct DevTools IDs despite both embedded views having tree ID 0.
	ids := h.ctrl.surfaceIDs(nil)
	if len(ids) != 5 {
		t.Fatalf("surfaceIDs = %v, want 5 distinct surfaces", ids)
	}
	seenIDs := map[int]bool{}
	for _, id := range ids {
		if seenIDs[id] {
			t.Fatalf("duplicate surface ID %d in %v", id, ids)
		}
		seenIDs[id] = true
	}

	// Start Pick: every surface mounts its nsgo-devtools-pick catcher.
	runDevCommand(devCommand{Op: "pick", On: true}, h.out)
	drainModels(h, all...)
	for name, m := range map[string]*surfaceModel{"main": mainModel, "secondary": secModel, "panel": panelModel, "embed1": embed1Model, "embed2": embed2Model} {
		if !m.hasTestID("nsgo-devtools-pick") {
			t.Fatalf("%s surface did not enter Pick mode", name)
		}
	}

	// Pick the button in the secondary window.
	secBtnID, ok := secModel.findByTestID("sec-btn")
	if !ok {
		t.Fatal("missing sec-btn")
	}
	secPickID, _ := secModel.findByTestID("nsgo-devtools-pick")
	secModel.tree.Dispatch(driver.Event{Kind: driver.EventMouseMove, ID: secPickID, Point: core.Point{X: 20, Y: 20}})
	secModel.tree.Dispatch(driver.Event{Kind: driver.EventPressDown, ID: secPickID, Point: core.Point{X: 20, Y: 20}})
	drainModels(h, all...)

	if pressed != 0 {
		t.Fatalf("pressed = %d, want 0 while picking", pressed)
	}
	for name, m := range map[string]*surfaceModel{"main": mainModel, "secondary": secModel, "panel": panelModel, "embed1": embed1Model, "embed2": embed2Model} {
		if m.hasTestID("nsgo-devtools-pick") {
			t.Errorf("%s surface stayed in Pick mode after secondary window picked", name)
		}
	}
	secDevID := h.ctrl.surfaceID(h.ctrl.findSurface(ids[1]))
	h.flushMessages(func(msgs []devMessage) bool {
		return slices.ContainsFunc(msgs, func(m devMessage) bool {
			return m.Type == "picked" && m.Surface == secDevID && m.ID == uint32(secBtnID)
		})
	})

	// Pick again, this time inside the second embedded view (embed2).
	runDevCommand(devCommand{Op: "pick", On: true}, h.out)
	drainModels(h, all...)
	embed2BtnID, ok := embed2Model.findByTestID("embed2-btn")
	if !ok {
		t.Fatal("missing embed2-btn")
	}
	embed2PickID, _ := embed2Model.findByTestID("nsgo-devtools-pick")
	embed2Model.tree.Dispatch(driver.Event{Kind: driver.EventPressDown, ID: embed2PickID, Point: core.Point{X: 20, Y: 20}})
	drainModels(h, all...)

	embed2DevID := h.ctrl.surfaceID(h.ctrl.findSurface(ids[4]))
	h.flushMessages(func(msgs []devMessage) bool {
		return slices.ContainsFunc(msgs, func(m devMessage) bool {
			return m.Type == "picked" && m.Surface == embed2DevID && m.ID == uint32(embed2BtnID)
		})
	})
	for name, m := range map[string]*surfaceModel{"main": mainModel, "secondary": secModel, "panel": panelModel, "embed1": embed1Model, "embed2": embed2Model} {
		if m.hasTestID("nsgo-devtools-pick") {
			t.Errorf("%s surface stayed in Pick mode after embedded view picked", name)
		}
	}
}

// Closing secondary windows, panels, and embedded views cleanly unregisters their overlays without leaking, and subsequent shared commands only reach live surfaces.
func TestDevOverlayCleanupOnSurfaceClose(t *testing.T) {
	h := newOverlayHarness(t, true, false, core.LayoutAudit{})
	root := core.NewRoot(core.RootOptions{Post: h.post, Windows: &core.WindowOptions{Width: 300, Height: 200}})
	appTree := core.NewTree(root, core.ApproxTextEngine{})
	defer root.Dispose()

	var secWin, panel *core.Window
	appTree.Mount(core.AppRoot(func(cx *core.Scope) {
		cx.SetMainView(func(cx *core.Scope) core.Node { return core.View{TestID: "main"} })
		secWin = cx.OpenWindow(core.WindowOptions{Title: "Secondary"}, func(cx *core.Scope) core.Node { return core.View{TestID: "sec"} })
		panel = cx.OpenPanel(core.PanelOptions{Width: 200, Height: 150}, func(cx *core.Scope) core.Node { return core.View{TestID: "panel"} })
	}))

	embedRoot := core.NewRoot(core.RootOptions{Post: h.post})
	embedTree := core.NewTree(embedRoot, core.ApproxTextEngine{})
	embedTree.Resize(core.Size{Width: 200, Height: 100}, core.Geometry{})
	embedTree.Mount(func(cx *core.Scope) core.Node { return core.View{TestID: "embed"} })

	mainModel := newSurfaceModel(surfaceTree(appTree, root.MainWindow()))
	secModel := newSurfaceModel(surfaceTree(appTree, secWin))
	panelModel := newSurfaceModel(surfaceTree(appTree, panel))
	embedModel := newSurfaceModel(embedTree)
	all := []*surfaceModel{newSurfaceModel(appTree), mainModel, secModel, panelModel, embedModel}
	drainModels(h, all...)

	if got := len(h.ctrl.surfaces); got != 4 {
		t.Fatalf("registered overlays = %d, want 4", got)
	}

	secWin.Close()
	panel.Close()
	embedRoot.Dispose()
	drainModels(h, all...)

	h.ctrl.mu.Lock()
	remainingSurfaces := len(h.ctrl.surfaces)
	remainingIDs := len(h.ctrl.devIDs)
	h.ctrl.mu.Unlock()
	if remainingSurfaces != 1 || remainingIDs != 1 {
		t.Fatalf("after closing secondary window, panel and embedded view: surfaces=%d devIDs=%d, want 1 and 1", remainingSurfaces, remainingIDs)
	}

	// Commands after close still work cleanly on the remaining main window.
	runDevCommand(devCommand{Op: "layout", On: true}, h.out)
	runDevCommand(devCommand{Op: "highlight", On: true}, h.out)
	runDevCommand(devCommand{Op: "pick", On: true}, h.out)
	drainModels(h, all...)
	if !mainModel.hasTestID("nsgo-layout-audit") || !mainModel.hasTestID("nsgo-devtools-pick") {
		t.Fatal("remaining main window did not receive commands after other surfaces closed")
	}
}
