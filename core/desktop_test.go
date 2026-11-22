package core

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestGesturesReportAndMerge(t *testing.T) {
	var pans []PanEvent
	var scales []float32
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		return View{TestID: "card", Style: Style{Width: Pt(50), Height: Pt(50)},
			OnPan:         func(e PanEvent) { pans = append(pans, e) },
			PanActivation: PanActivation{ActiveOffsetX: 20},
			OnPinch:       func(e GestureEvent) { scales = append(scales, e.Value) }}
	})
	b := h.commit()
	n := h.find("card")
	for _, it := range b.Items {
		if it.ID == n.id && it.Kind == driver.ItemEvents && (!it.Events.Has(driver.EventPan) || !it.Events.Has(driver.EventPinch) || it.Events.Has(driver.EventRotation)) {
			t.Fatalf("events = %b", it.Events)
		}
		if it.ID == n.id && it.Kind == driver.ItemViewProps && it.ViewProps.PanActivation.ActiveOffsetX != 20 {
			t.Fatalf("props = %+v", it.ViewProps)
		}
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventPan, ID: n.id, Phase: GestureBegan, Point: Point{X: 20, Y: 0}, Position: Point{X: 25, Y: 10}, PagePosition: Point{X: 35, Y: 20}})
	h.tree.Dispatch(driver.Event{Kind: driver.EventPan, ID: n.id, Phase: GestureEnded, Point: Point{X: 30, Y: 5}, Position: Point{X: 35, Y: 15}, PagePosition: Point{X: 45, Y: 25}, Velocity: Point{X: 100, Y: 0}})
	h.tree.Dispatch(driver.Event{Kind: driver.EventPinch, ID: n.id, Phase: GestureChanged, Value: 1.5})
	if len(pans) != 2 || pans[1].Translation != (Point{X: 30, Y: 5}) || pans[1].Position != (Point{X: 35, Y: 15}) || pans[1].PagePosition != (Point{X: 45, Y: 25}) || pans[1].Velocity.X != 100 || pans[1].Phase != GestureEnded {
		t.Fatalf("pans = %+v", pans)
	}
	if !slices.Equal(scales, []float32{1.5}) {
		t.Fatalf("scales = %v", scales)
	}
	if !(driver.Event{Kind: driver.EventPan, Phase: GestureChanged}).Mergeable() || (driver.Event{Kind: driver.EventPan, Phase: GestureEnded}).Mergeable() {
		t.Fatal("only a gesture's moves merge")
	}
}

func TestPanActivationDecides(t *testing.T) {
	horizontal := PanActivation{ActiveOffsetX: 10, FailOffsetY: 5}
	for _, c := range []struct {
		a    PanActivation
		d    Point
		want int
	}{
		{PanActivation{}, Point{X: 3, Y: 4}, 0},
		{PanActivation{}, Point{X: 0, Y: -12}, 1},
		{horizontal, Point{X: -12, Y: 2}, 1},
		{horizontal, Point{X: 4, Y: 6}, -1},
		{horizontal, Point{X: 6, Y: 2}, 0},
	} {
		if got := c.a.Decide(c.d); got != c.want {
			t.Errorf("%+v.Decide(%v) = %d, want %d", c.a, c.d, got, c.want)
		}
	}
}

func TestWindowCommands(t *testing.T) {
	lights := WindowControls{TrafficLights: Point{X: 20, Y: 18}}
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		cx.SetTitleBar(TitleBarHidden, 40)
		cx.SetWindowControls(lights)
		cx.SetWindowMaterial(WindowMaterialSidebar)
		cx.SetFullScreen(true, "display-2")
		cx.SetFullScreen(false)
		return View{TestID: "bar", Desktop: DesktopViewProps{WindowDrag: WindowDragOn}}
	})
	b := h.commit()
	var cmds []driver.WindowCommand
	for _, it := range b.Items {
		if it.Kind == driver.ItemWindow {
			cmds = append(cmds, *it.Window)
		}
	}
	if want := []driver.WindowCommand{
		{Action: driver.WindowSetTitleBar, TitleBar: TitleBarHidden, Height: 40},
		{Action: driver.WindowSetControls, Controls: lights},
		{Action: driver.WindowSetMaterial, Material: WindowMaterialSidebar},
		{Action: driver.WindowSetFullScreen, On: true, DisplayID: "display-2"},
		{Action: driver.WindowSetFullScreen},
	}; !slices.Equal(cmds, want) {
		t.Fatalf("window commands = %+v, want %+v", cmds, want)
	}
	if p := h.find("bar").props; p.WindowDrag != WindowDragOn {
		t.Fatalf("props = %+v", p)
	}
}

// TestReopenOrdersTheOneSurfaceFront: where the platform opens no windows of the app's, as on Windows and Linux, a reopen no handler hears brings the app's one window to the front, and one a handler hears leaves it to the handler.
func TestReopenOrdersTheOneSurfaceFront(t *testing.T) {
	var cx0 *Scope
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		cx0 = cx
		return Text{Content: "main"}
	})
	h.commit()
	orders := func() int {
		n := 0
		for _, it := range h.commit().Items {
			if it.Kind == driver.ItemWindow && it.Window.Action == driver.WindowOrderFront {
				n++
			}
		}
		return n
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventReopen})
	if n := orders(); n != 1 {
		t.Fatalf("a reopen without a handler ordered the window front %d times, want 1", n)
	}
	heard := 0
	cx0.OnReopen(func() { heard++ })
	h.tree.Dispatch(driver.Event{Kind: driver.EventReopen})
	if n := orders(); n != 0 || heard != 1 {
		t.Fatalf("a reopen with a handler: %d orders front, heard %d times", n, heard)
	}
}

func TestApplicationMenuFollowsItsOwner(t *testing.T) {
	var log []string
	var open *Signal[bool]
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		open = NewSignal(cx, true)
		return Show{When: open.Get, Then: func(cx *Scope) Node {
			cx.SetApplicationMenu([]MenuItem{
				{Label: "App", Items: []MenuItem{{Role: MenuQuit}}},
				{Label: "File", Items: []MenuItem{
					{Label: "New", Shortcut: "Cmd+N", OnSelect: func() { log = append(log, "new") }},
					{Separator: true},
					{Label: "Wrap", Checked: true},
				}},
			})
			return View{}
		}}
	})
	var menu []driver.MenuEntry
	for _, it := range h.commit().Items {
		if it.Kind == driver.ItemMenu {
			menu = it.Menu
		}
	}
	if len(menu) != 2 || menu[0].Items[0].Label != "Quit" || menu[0].Items[0].ID != 0 {
		t.Fatalf("menu = %+v", menu)
	}
	file := menu[1].Items
	if file[0].Shortcut != "meta+n" || file[0].ID == 0 || !file[1].Separator || !file[2].Checkable {
		t.Fatalf("file = %+v", file)
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventMenu, Count: file[0].ID})
	open.Set(false)
	restored := false
	for _, it := range h.commit().Items {
		restored = restored || it.Kind == driver.ItemMenu && it.Menu == nil
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventMenu, Count: file[0].ID})
	if !restored || !slices.Equal(log, []string{"new"}) {
		t.Fatalf("restored = %v, log = %q", restored, log)
	}
}

func TestClosingApplicationMenuOwnerRestoresPreviousSurface(t *testing.T) {
	h := newHost(t, Size{Width: 100, Height: 100}, func(*Scope) Node { return View{} })
	rt := h.tree.rt
	previous := &Tree{rt: rt, root: h.tree.root, menuOrder: 1}
	owner := &Tree{rt: rt, root: h.tree.root, menuOrder: 2, closed: true}
	rt.surfaces = []*Tree{previous, owner}
	rt.menuBar = owner
	owner.menuClosed()
	if rt.menuBar != previous || !previous.keysChanged || !previous.changed {
		t.Fatalf("menu bar = %p, previous flags keys=%v changed=%v; want previous and both invalidated", rt.menuBar, previous.keysChanged, previous.changed)
	}
}

func TestDockMenuFollowsItsOwner(t *testing.T) {
	var log []string
	var open *Signal[bool]
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		open = NewSignal(cx, true)
		return Show{When: open.Get, Then: func(cx *Scope) Node {
			cx.SetDockMenu([]MenuItem{
				{Label: "New Window", OnSelect: func() { log = append(log, "new") }},
				{Label: "More", Items: []MenuItem{{Label: "Open", OnSelect: func() { log = append(log, "open") }}}},
			})
			return View{}
		}}
	})
	var menu []driver.MenuEntry
	for _, it := range h.commit().Items {
		if it.Kind == driver.ItemDockMenu {
			menu = it.Menu
		}
	}
	if len(menu) != 2 || menu[0].ID == 0 || menu[1].Items[0].ID == 0 {
		t.Fatalf("dock menu = %+v", menu)
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventDockMenu, Count: menu[1].Items[0].ID})
	open.Set(false)
	removed := false
	for _, it := range h.commit().Items {
		removed = removed || it.Kind == driver.ItemDockMenu && it.Menu == nil
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventDockMenu, Count: menu[0].ID})
	if !removed || !slices.Equal(log, []string{"open"}) {
		t.Fatalf("removed = %v, log = %q", removed, log)
	}
}

// Pending app icon menus follow call order across surfaces, even though the app surface always commits before its child windows.
func TestDockMenuPendingReplacementAcrossSurfaces(t *testing.T) {
	cases := []struct {
		name           string
		appLast, clear bool
	}{
		{"window then app", true, false},
		{"app then window", false, false},
		{"app clears window", true, true},
		{"window clears app", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var app, child *Scope
			w := newWindows(t, func(cx *Scope) {
				app = cx
				cx.OpenWindow(WindowOptions{}, func(cx *Scope) Node { child = cx; return View{} })
			})
			first, last, latestSurface := app, child, uint32(1)
			if tc.appLast {
				first, last, latestSurface = child, app, 0
			}
			first.SetDockMenu([]MenuItem{{Label: "Old", OnSelect: func() {}}})
			var latest []MenuItem
			if !tc.clear {
				latest = []MenuItem{{Label: "Latest", OnSelect: func() {}}}
			}
			last.SetDockMenu(latest)
			w.commit()
			for _, surface := range []uint32{0, 1} {
				items := w.take(surface, driver.ItemDockMenu)
				if surface != latestSurface {
					if len(items) != 0 {
						t.Errorf("superseded surface %d sent %d menus", surface, len(items))
					}
				} else if len(items) != 1 {
					t.Errorf("latest surface sent %d menus", len(items))
				} else if tc.clear && items[0].Menu != nil || !tc.clear && (len(items[0].Menu) != 1 || items[0].Menu[0].Label != "Latest") {
					t.Errorf("latest menu %+v", items[0].Menu)
				}
			}
		})
	}
}

// dockMenus are the app icon menus a batch sends, in order.
func dockMenus(b *driver.MountBatch) [][]driver.MenuEntry {
	var menus [][]driver.MenuEntry
	for _, it := range b.Items {
		if it.Kind == driver.ItemDockMenu {
			menus = append(menus, it.Menu)
		}
	}
	return menus
}

// TestDockMenuTasksByLabel runs a Windows Jump List's tasks, which name the item by its label, as the app starts with one before it set its menu and later: an enabled top-level action runs once, anything else does nothing.
func TestDockMenuTasksByLabel(t *testing.T) {
	var log []string
	var ready *Signal[bool]
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		ready = NewSignal(cx, false)
		cx.Effect(func() {
			if !ready.Get() {
				return
			}
			cx.SetDockMenu([]MenuItem{
				{Label: "New Window", OnSelect: func() { log = append(log, "new") }},
				{Label: "Paused", Disabled: true, OnSelect: func() { log = append(log, "paused") }},
				{Label: "More", Items: []MenuItem{{Label: "Open", OnSelect: func() { log = append(log, "open") }}}},
			})
		})
		return View{}
	})
	h.commit()
	h.tree.Dispatch(driver.Event{Kind: driver.EventDockMenu, Text: "New Window"})
	h.tree.Dispatch(driver.Event{Kind: driver.EventDockMenu, Text: "Missing"})
	h.drain()
	if len(log) != 0 {
		t.Fatalf("before the menu: log = %q", log)
	}
	ready.Set(true)
	h.commit()
	h.drain()
	if !slices.Equal(log, []string{"new"}) {
		t.Fatalf("once the menu is set: log = %q", log)
	}
	for _, label := range []string{"New Window", "Paused", "More", "Open", "Missing"} {
		h.tree.Dispatch(driver.Event{Kind: driver.EventDockMenu, Text: label})
	}
	h.drain()
	if !slices.Equal(log, []string{"new", "new"}) {
		t.Fatalf("log = %q", log)
	}
}

// TestDockMenuTaskAfterItsMenuWent: a task chosen after the menu's owner went, as Windows keeps the Jump List, does nothing, and does not run when a menu with that label comes back.
func TestDockMenuTaskAfterItsMenuWent(t *testing.T) {
	var log []string
	var open *Signal[bool]
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		open = NewSignal(cx, true)
		return Show{When: open.Get, Then: func(cx *Scope) Node {
			cx.SetDockMenu([]MenuItem{{Label: "New Window", OnSelect: func() { log = append(log, "new") }}})
			return View{}
		}}
	})
	h.commit()
	open.Set(false)
	h.commit()
	h.tree.Dispatch(driver.Event{Kind: driver.EventDockMenu, Text: "New Window"})
	h.drain()
	open.Set(true)
	h.commit()
	h.drain()
	if len(log) != 0 {
		t.Fatalf("log = %q", log)
	}
}

// TestDockMenuSetAgainForgetsTheItemsBefore: an effect setting the menu again sends it in place of the one before, without removing it between, and an item of the menu before, chosen as its menu was replaced, does nothing.
func TestDockMenuSetAgainForgetsTheItemsBefore(t *testing.T) {
	var log []string
	var version *Signal[int]
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		version = NewSignal(cx, 0)
		cx.Effect(func() {
			v := version.Get()
			cx.SetDockMenu([]MenuItem{{Label: "Library", OnSelect: func() { log = append(log, fmt.Sprint("library ", v)) }}})
		})
		return View{}
	})
	first := dockMenus(h.commit())
	if len(first) != 1 || len(first[0]) != 1 || first[0][0].ID == 0 {
		t.Fatalf("dock menus = %+v", first)
	}
	version.Set(1)
	second := dockMenus(h.commit())
	if len(second) != 1 || len(second[0]) != 1 || second[0][0].ID == first[0][0].ID {
		t.Fatalf("dock menus set again = %+v", second)
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventDockMenu, Count: first[0][0].ID})
	h.tree.Dispatch(driver.Event{Kind: driver.EventDockMenu, Count: second[0][0].ID})
	h.tree.Dispatch(driver.Event{Kind: driver.EventDockMenu, Text: "Library"})
	if !slices.Equal(log, []string{"library 1", "library 1"}) {
		t.Fatalf("log = %q", log)
	}
}

func TestRecentFilesCommands(t *testing.T) {
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		cx.AddRecentFile("example.nsgo")
		cx.AddRecentFile("")
		cx.ClearRecentFiles()
		return View{}
	})
	var recent []driver.MountItem
	for _, item := range h.commit().Items {
		if item.Kind == driver.ItemRecentFile {
			recent = append(recent, item)
		}
	}
	abs, err := filepath.Abs("example.nsgo")
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 || recent[0].Value != abs || recent[0].On || !recent[1].On {
		t.Fatalf("recent file commands = %+v", recent)
	}
}

// TestApplicationMenuSetAgainKeepsTheBar sends a menu an effect sets
// again in place of the one before, without the default between them,
// which a menu bar would show for a moment.
func TestApplicationMenuSetAgainKeepsTheBar(t *testing.T) {
	var checked *Signal[bool]
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		checked = NewSignal(cx, true)
		cx.Effect(func() {
			cx.SetApplicationMenu([]MenuItem{{Label: "View", Items: []MenuItem{{Label: "Sidebar", Checked: checked.Get()}}}})
		})
		return View{}
	})
	h.commit()
	checked.Set(false)
	var menus [][]driver.MenuEntry
	for _, it := range h.commit().Items {
		if it.Kind == driver.ItemMenu {
			menus = append(menus, it.Menu)
		}
	}
	if len(menus) != 1 || menus[0] == nil || menus[0][0].Items[0].Checked {
		t.Fatalf("menus sent = %+v, want the new one alone", menus)
	}
}

func TestApplicationMenuStandardRolesAndRadios(t *testing.T) {
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		cx.SetApplicationMenu([]MenuItem{
			{Label: "App", Items: []MenuItem{{Role: MenuServices}}},
			{Role: MenuWindowMenu, Items: []MenuItem{{Role: MenuBringAllToFront}}},
			{Role: MenuHelpMenu},
			{Label: "Edit", Items: []MenuItem{
				{Role: MenuPasteAndMatchStyle, When: MenuAuto}, {Role: MenuDelete, Shortcut: "delete", When: MenuAuto},
				{Role: MenuStartSpeaking}, {Role: MenuStopSpeaking},
				{Label: "One", Radio: true, Checked: true, OnSelect: func() {}}, {Label: "Two", Radio: true, OnSelect: func() {}},
			}},
		})
		return View{}
	})
	var menu []driver.MenuEntry
	for _, it := range h.commit().Items {
		if it.Kind == driver.ItemMenu {
			menu = it.Menu
		}
	}
	if len(menu) != 4 || menu[0].Items[0].Role != MenuServices || menu[0].Items[0].Label != "Services" ||
		menu[1].Role != MenuWindowMenu || menu[1].Label != "Window" || menu[1].Items[0].Role != MenuBringAllToFront ||
		menu[2].Role != MenuHelpMenu || menu[2].Label != "Help" || len(menu[2].Items) != 0 {
		t.Fatalf("menus = %+v", menu)
	}
	items := menu[3].Items
	// No text field handles Paste and Match Style's key, so its shortcut stays the menu's; Delete's key is the field's.
	if len(items) != 6 || items[0].Role != MenuPasteAndMatchStyle || items[0].NotInText || items[1].Role != MenuDelete || !items[1].NotInText {
		t.Fatalf("editing roles = %+v", items)
	}
	if !items[4].Radio || !items[4].Checkable || !items[4].Checked || items[4].ID == 0 || !items[5].Radio || !items[5].Checkable || items[5].Checked {
		t.Fatalf("radios = %+v", items[4:])
	}
}

func TestBordersPerSide(t *testing.T) {
	red, blue := RGB(0xff0000), RGB(0x0000ff)
	for _, c := range []struct {
		s    Style
		want driver.ViewStyle
	}{
		{Style{BorderWidth: 2, BorderColor: red}, driver.ViewStyle{BorderWidth: 2, BorderColor: red}},
		{Style{Borders: All(3), BorderColor: red}, driver.ViewStyle{BorderWidth: 3, BorderColor: red}},
		{Style{Borders: Edges{Bottom: 1}, BorderColor: red},
			driver.ViewStyle{Borders: Edges{Bottom: 1}, BorderColors: AllColors(red)}},
		{Style{BorderWidth: 2, BorderColor: red, BorderColors: EdgeColors{Left: blue}},
			driver.ViewStyle{Borders: All(2), BorderColors: EdgeColors{Top: red, Right: red, Bottom: red, Left: blue}}},
	} {
		got := c.s.viewStyle(Size{}, 0, false)
		got.Opacity = 0
		if got != c.want {
			t.Errorf("%+v: got %+v, want %+v", c.s, got, c.want)
		}
	}
}

func TestCursorStyleReachesTheView(t *testing.T) {
	s := Style{Cursor: CursorColResize}
	if got := s.viewStyle(Size{}, 0, false).Cursor; got != CursorColResize {
		t.Fatalf("view cursor = %v, want the style's", got)
	}
	if got := (Style{Cursor: CursorRowResize}).Merge(Style{Cursor: CursorPointer}).Cursor; got != CursorPointer {
		t.Fatalf("merged cursor = %v, want the later style's", got)
	}
	if got := CursorRowResize.Name(); got != "row-resize" {
		t.Fatalf("cursor name = %q, want its CSS name", got)
	}
}

func TestMenuWhenLeavesInputsTheirKeys(t *testing.T) {
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		cx.SetApplicationMenu([]MenuItem{{Label: "Edit", Items: []MenuItem{
			{Label: "Find", Shortcut: "mod+f", When: MenuAuto},
			{Label: "Select line", Shortcut: "mod+a", When: MenuAuto},
			{Label: "Run", Shortcut: "mod+r", When: MenuNotEditing},
			{Label: "Save", Shortcut: "mod+s"},
		}}})
		return View{}
	})
	var menu []driver.MenuEntry
	for _, it := range h.commit().Items {
		if it.Kind == driver.ItemMenu {
			menu = it.Menu
		}
	}
	var got []bool
	for _, e := range menu[0].Items {
		got = append(got, e.NotInText)
	}
	if !slices.Equal(got, []bool{false, true, true, false}) {
		t.Errorf("NotInText = %v: auto leaves select all, notEditing every key", got)
	}
}

func TestCursorPointAnswersOnceAndReportsUnsupported(t *testing.T) {
	h := newHost(t, Size{Width: 100, Height: 100}, func(*Scope) Node { return View{} })
	h.tree.SetEnvironment(Environment{OS: OSWindows})
	var got Point
	var available bool
	calls := 0
	done := func(p Point, ok bool) {
		got, available = p, ok
		calls++
	}
	h.root.CursorPoint(done)
	var id uint32
	for _, it := range h.commit().Items {
		if it.Kind == driver.ItemCursorPoint {
			id = it.Count
		}
	}
	if id == 0 || calls != 0 {
		t.Fatalf("request id = %d, calls = %d", id, calls)
	}
	want := Point{X: -240, Y: 125}
	answer := driver.Event{Kind: driver.EventCursorPoint, Count: id, Point: want, Finished: true}
	h.tree.Dispatch(answer)
	h.tree.Dispatch(answer)
	if got != want || !available || calls != 1 {
		t.Fatalf("got %v, available = %v, calls = %d", got, available, calls)
	}
	for _, env := range []Environment{{OS: OSIOS}, {OS: OSWindows, Simulated: true}} {
		before := calls
		h.tree.SetEnvironment(env)
		h.root.CursorPoint(done)
		if got != (Point{}) || available || calls != before+1 {
			t.Fatalf("unsupported environment %+v returned %v, available = %v, calls = %d", env, got, available, calls-before)
		}
	}
}
