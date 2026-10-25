package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

// handle styles a resizable handle's line: a hairline that takes the
// accent while the pointer is over the handle or drags it.
func (t *theme) handle() core.Style {
	return core.Style{Background: t.line, GroupStyle: &core.GroupStyle{
		Hover: &core.Style{Background: t.borderStrong}, Active: &core.Style{Background: t.accent}}}
}

func (t *theme) pane(title, about string) core.Node {
	return core.View{Style: core.Style{FlexGrow: 1, Padding: core.All(12), Gap: 4}, Children: []core.Node{
		core.Text{Content: title, Style: core.Style{FontWeight: core.FontWeightSemibold}},
		t.note(about),
	}}
}

// resizablePage shows base.ResizablePanelGroup's panels.
func resizablePage(cx *core.Scope, t *theme) core.Node {
	sizes := core.NewSignalFunc(cx, []core.Points(nil), nil)
	inspector := core.NewSignal(cx, true)
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Resizable panels", "base.ResizablePanelGroup lays panels out with a handle between each two: a drag resizes the panel "+
			"before it, taking from those after as far as their minimums let it. Screen readers move a handle 10 points a step.",
			flow(t.plain("Toggle inspector", func() { inspector.Set(!inspector.Get()) })),
			base.ResizablePanelGroup{TestID: "panels", HandleLabel: "Resize the panels", HandleStyle: t.handle(),
				Style:    core.Style{Height: core.Pt(200), CornerRadius: radius, BorderWidth: 1, BorderColor: t.line, Overflow: core.Hidden, Background: t.surface},
				OnResize: sizes.Set,
				Panels: []base.ResizablePanel{
					{Size: func() core.Points { return 180 }, MinSize: 120, MaxSize: 280, Style: core.Style{Background: t.quiet},
						Children: []core.Node{t.pane("Sidebar", "180 points, from 120 to 280.")}},
					{Children: []core.Node{t.pane("Main", "Takes what the others leave.")}},
					{Size: func() core.Points { return 160 }, Visible: inspector.Get, Style: core.Style{Background: t.sunken},
						Children: []core.Node{t.pane("Inspector", "Hidden and shown at the size it wanted.")}},
				}},
			t.live(func() string {
				s := sizes.Get()
				if s == nil {
					return "Drag a handle."
				}
				parts := make([]string, len(s))
				for i, x := range s {
					parts[i] = fmt.Sprintf("%.0f", x)
				}
				return "Sizes: " + strings.Join(parts, ", ")
			}),
		),
		t.demo("Vertical", "The same along the other axis, with a grip on each handle.",
			base.ResizablePanelGroup{Axis: base.Vertical, HandleStyle: t.handle(),
				HandleThumbStyle: core.Style{Width: core.Pt(4), Height: core.Pt(28), CornerRadius: 2, Background: t.borderStrong},
				Style:            core.Style{Height: core.Pt(220), CornerRadius: radius, BorderWidth: 1, BorderColor: t.line, Overflow: core.Hidden, Background: t.surface},
				Panels: []base.ResizablePanel{
					{Children: []core.Node{t.pane("Editor", "Half, at first.")}},
					{MinSize: 60, Children: []core.Node{t.pane("Terminal", "At least 60 points.")}},
				}},
		),
	}}
}

// dockPanels are the dock demo's panels, by key.
var dockPanels = map[string]string{
	"files":    "The project's files.",
	"outline":  "The symbols of the open file.",
	"editor":   "Drag this tab onto another group to merge, or onto a side of one to split it.",
	"preview":  "Tabs move between the center and the docks.",
	"terminal": "The bottom dock spans the center's column.",
	"problems": "Close this tab with its ×.",
}

// dockPage shows base.DockArea, its layout kept in storage in gpui-base's
// format.
func dockPage(cx *core.Scope, t *theme) core.Node {
	defaults := func() base.DockLayout {
		return base.DockLayout{
			Center: base.DockSplit(base.Horizontal, base.DockTabs("editor"), base.DockTabs("preview")),
			Left:   &base.DockRegion{Root: base.DockTabs("files", "outline"), Size: 180},
			Bottom: &base.DockRegion{Root: base.DockTabs("terminal", "problems"), Size: 140},
		}
	}
	initial := defaults()
	store := cx.Storage()
	const key = "gallery.dock"
	if saved, ok := store.GetItem(key); ok {
		var l base.DockLayout
		if json.Unmarshal([]byte(saved), &l) == nil && len(l.Center.Children) > 0 {
			initial = l
		}
	}
	saved := core.NewSignal(cx, 0)
	layout := core.NewSignalFunc(cx, initial, nil)
	ref := new(base.DockAreaRef)
	tab := func(cx *core.Scope, tab base.DockTab) core.Node {
		nodes := []core.Node{core.Text{Content: tab.Key}}
		if tab.Closable {
			nodes = append(nodes, base.Button{Label: "Close " + tab.Key, OnPress: func(core.PressEvent) { tab.Close() },
				Style:    core.Style{Padding: core.Symmetric(0, 4), CornerRadius: 4, Hover: &core.Style{Background: t.hover}},
				Children: []core.Node{core.Text{Content: "×", Style: t.caption()}}})
		}
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6}, Children: nodes}
	}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Dock area", "base.DockArea: drag tabs between groups and docks, resize the splits and docks, zoom into a group. "+
			"Its layout is saved as gpui-base's file; below 600 points one group shows at a time and the docks are drawers. "+
			"A panel keeps what it holds wherever it moves.",
			flow(
				t.plain("Files", func() { ref.ToggleDock(base.DockLeft) }),
				t.plain("Terminal", func() { ref.ToggleDock(base.DockBottom) }),
				t.plain("Zoom", ref.ToggleZoom),
				t.plain("Reset", func() {
					store.RemoveItem(key)
					saved.Set(0)
					layout.Set(defaults())
				}),
			),
			base.DockArea{Ref: ref, TestID: "dock", Layout: layout.Get,
				Panels: map[string]base.DockPanelOptions{"problems": {Closable: true}},
				OnLayoutChange: func(l base.DockLayout) {
					layout.Set(l)
					if data, err := json.Marshal(l); err == nil {
						_ = store.SetItem(key, string(data))
						saved.Set(len(data))
					}
				},
				RenderPanel: func(cx *core.Scope, key string) core.Node {
					return core.View{Style: core.Style{FlexGrow: 1, Padding: core.All(12), Gap: 6}, Children: []core.Node{
						core.Text{Content: strings.ToUpper(key[:1]) + key[1:], Style: core.Style{FontWeight: core.FontWeightSemibold}},
						t.note(dockPanels[key]),
						core.Input{Placeholder: "Notes", Label: key + " notes", Style: t.field()},
					}}
				},
				RenderTab: tab,
				RenderDragPreview: func(cx *core.Scope, tab base.DockTab) core.Node {
					return core.View{Style: t.popup().Merge(core.Style{Padding: core.Symmetric(4, 10)}),
						Children: []core.Node{core.Text{Content: tab.Key}}}
				},
				Style:              core.Style{Height: core.Pt(420), CornerRadius: radius, BorderWidth: 1, BorderColor: t.line, Background: t.surface},
				GroupStyle:         core.Style{Background: t.surface},
				TabBarStyle:        core.Style{Gap: 2, Padding: core.Symmetric(4, 6), Background: t.quiet, Borders: core.Edges{Bottom: 1}, BorderColors: core.EdgeColors{Bottom: t.line}},
				TabStyle:           core.Style{Padding: core.Symmetric(4, 10), CornerRadius: radius - 2, Color: t.muted, Hover: &core.Style{Background: t.hover}, Selected: &core.Style{Background: t.surface, Color: t.ink}},
				DropIndicatorStyle: core.Style{Background: withAlpha(t.accent, 0.18), BorderWidth: 1.5, BorderColor: t.accent, CornerRadius: radius},
				HandleStyle:        t.handle(),
				DockStyle:          core.Style{Background: t.sunken},
				DrawerStyle:        core.Style{Width: core.Pt(260), Background: t.surface},
				SheetStyle:         core.Style{Height: core.Pt(220), Background: t.surface, CornerRadius: dialogRadius},
				BackdropStyle:      t.backdrop(),
				HandleLabel:        "Resize",
			},
			t.live(func() string {
				if n := saved.Get(); n > 0 {
					return fmt.Sprintf("Layout saved: %d bytes of gpui-base's JSON.", n)
				}
				return "Rearrange the panels; the layout is kept across launches."
			}),
		),
	}}
}
