package nsgotest

import (
	"slices"
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Surface is an open window, panel or tray view, as nsgotest's steps see it. A surface's handle stays tied to it after it closes. View queries and input steps use this surface; app-wide service fixtures and clocks stay on App.
type Surface struct {
	app    *App
	model  *App
	config SurfaceConfig
	label  string
	closed bool
}

// Surfaces lists the open surfaces in opening order, with the main
// window first. A platform with one surface has just its main surface.
func (a *App) Surfaces() []*Surface {
	a.t.Helper()
	a.Settle()
	var out []*Surface
	if a.surfaceMain != nil && !a.surfaceMain.closed {
		out = append(out, a.surfaceMain)
	}
	for _, config := range a.Opened {
		s := a.surfaceModels[config.ID]
		if s != nil && !s.closed && s != a.surfaceMain {
			out = append(out, s)
		}
	}
	return out
}

// Surface finds one open surface by title:Title, id:root-test-id or
// index:0 (zero-based in Surfaces). It fails for no or several matches.
func (a *App) Surface(selector string) *Surface {
	a.t.Helper()
	surfaces := a.Surfaces()
	kind, value, _ := strings.Cut(selector, ":")
	var found []*Surface
	switch kind {
	case "index":
		if index, err := strconv.Atoi(value); err == nil && index >= 0 && index < len(surfaces) {
			return surfaces[index]
		}
	case "title":
		for _, s := range surfaces {
			if s.title() == value {
				found = append(found, s)
			}
		}
	case "id":
		for _, s := range surfaces {
			// The tree's root view wraps the content the app returned.
			root := s.model.views[s.model.rootID]
			if root != nil && slices.ContainsFunc(root.Children, func(v *View) bool { return v.Props.TestID == value }) {
				found = append(found, s)
			}
		}
	default:
		a.t.Fatalf("nsgotest: invalid surface selector %q: use title:, id: or index:", selector)
	}
	if len(found) != 1 {
		a.t.Fatalf("nsgotest: surface selector %q matched %d open surfaces", selector, len(found))
	}
	return found[0]
}

// Config returns the surface's opening configuration.
func (s *Surface) Config() SurfaceConfig { return s.config }

// SystemGestures returns this surface's combined system gesture request.
func (s *Surface) SystemGestures() core.SystemGestureOptions {
	s.app.t.Helper()
	options := s.open().SystemGestures
	options.AndroidRects = slices.Clone(options.AndroidRects)
	return options
}

// DragTo drags source onto target inside this surface.
func (s *Surface) DragTo(source, target string) {
	s.app.t.Helper()
	s.open().DragTo(source, target)
}

func (s *Surface) title() string {
	return s.label
}

func (s *Surface) open() *App {
	s.app.t.Helper()
	s.app.Settle()
	return s.checkOpen()
}

// checkOpen validates the handle without settling, including while queued posts run.
func (s *Surface) checkOpen() *App {
	s.app.t.Helper()
	if s.closed || s.model.tree != nil && s.model.tree.Closed() {
		s.app.t.Fatalf("nsgotest: surface %q is closed", s.title())
	}
	if t := s.model.target; t != nil && t.surface == nil {
		s.app.t.Fatalf("nsgotest: platform surface %q did not open", s.title())
	}
	return s.model
}

// Close asks the surface to close as its close button does, and reports
// whether it closed. Its OnClosing handlers can cancel the request.
func (s *Surface) Close() bool {
	s.app.t.Helper()
	m := s.open()
	if m.target != nil {
		if s.config.ID == 0 {
			m.t.Fatalf("nsgotest: surface %q has no app window to close", s.title())
		}
		m.target.closeWindow()
		m.done()
	} else {
		if m.tree.Window() == nil {
			m.t.Fatalf("nsgotest: surface %q has no window to close", s.title())
		}
		m.dispatch(driver.Event{Kind: driver.EventWindowClose})
	}
	return s.closed
}

func (a *App) openSurface(config SurfaceConfig, t *core.Tree, main bool) *Surface {
	if a.surfaceModels == nil {
		a.surfaceModels = map[uint32]*Surface{}
	}
	if s := a.surfaceModels[config.ID]; s != nil {
		return s
	}
	model := a
	if !main {
		model = &App{
			t: a.t, owner: a, Clock: a.Clock, tree: t, root: a.root, env: a.env, start: a.start,
			views: map[driver.NodeID]*View{}, edits: map[driver.NodeID]uint32{},
			ImageCacheLimit: a.ImageCacheLimit,
		}
		model.anim = driver.NewAnimator(func(ev driver.Event) { a.postTo(t, ev) })
	}
	if t != nil {
		t.SetGPUSubmitter(a.submitGPU)
	}
	s := &Surface{app: a, model: model, config: config, label: config.Title}
	a.surfaceModels[config.ID] = s
	if main {
		a.surfaceMain = s
	}
	return s
}

func (s *Surface) updateTitle(b *driver.MountBatch) {
	for _, it := range b.Items {
		if it.Kind == driver.ItemTitle {
			s.label = it.Value
		}
	}
}

func (a *App) surfaceConfig(id uint32) SurfaceConfig {
	if index := slices.IndexFunc(a.Opened, func(c SurfaceConfig) bool { return c.ID == id }); index >= 0 {
		return a.Opened[index]
	}
	return SurfaceConfig{ID: id, Kind: SurfaceWindow, Main: true}
}

func (a *App) closeSurface(id uint32) {
	if s := a.surfaceModels[id]; s != nil {
		s.closed = true
		if s.model != a {
			s.model.releaseDrawings()
		}
		delete(a.surfaceModels, id)
	}
}

// surfaceWork reports whether any modeled headless surface needs another frame.
func (a *App) surfaceWork() bool {
	needsFrame := func(m *App) bool {
		if m.anim.Ending() || m.tree.Pacing() || m.tree.PendingTransitionEnds() || len(m.presenting) > 0 {
			return true
		}
		for _, v := range m.views {
			if v.motion != nil {
				return true
			}
		}
		return false
	}
	if needsFrame(a) {
		return true
	}
	for _, s := range a.surfaceModels {
		if s.model != a && needsFrame(s.model) {
			return true
		}
	}
	return false
}
