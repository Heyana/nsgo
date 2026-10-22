package core

import (
	"slices"
	"testing"
)

func TestGesturesReportAndMerge(t *testing.T) {
	var pans []PanEvent
	var scales []float32
	h := newHost(t, Size{100, 100}, func(cx *Scope) Node {
		return View{TestID: "card", Style: Style{Width: Pt(50), Height: Pt(50)},
			OnPan:         func(e PanEvent) { pans = append(pans, e) },
			PanActivation: PanActivation{ActiveOffsetX: 20},
			OnPinch:       func(e GestureEvent) { scales = append(scales, e.Value) }}
	})
	b := h.commit()
	n := h.find("card")
	for _, it := range b.Items {
		if it.ID == n.id && it.Kind == ItemEvents && (!it.Events.Has(EventPan) || !it.Events.Has(EventPinch) || it.Events.Has(EventRotation)) {
			t.Fatalf("events = %b", it.Events)
		}
		if it.ID == n.id && it.Kind == ItemViewProps && it.ViewProps.PanActivation.ActiveOffsetX != 20 {
			t.Fatalf("props = %+v", it.ViewProps)
		}
	}
	h.tree.Dispatch(Event{Kind: EventPan, ID: n.id, Phase: GestureBegan, Point: Point{20, 0}})
	h.tree.Dispatch(Event{Kind: EventPan, ID: n.id, Phase: GestureEnded, Point: Point{30, 5}, Velocity: Point{100, 0}})
	h.tree.Dispatch(Event{Kind: EventPinch, ID: n.id, Phase: GestureChanged, Value: 1.5})
	if len(pans) != 2 || pans[1].Translation != (Point{30, 5}) || pans[1].Velocity.X != 100 || pans[1].Phase != GestureEnded {
		t.Fatalf("pans = %+v", pans)
	}
	if !slices.Equal(scales, []float32{1.5}) {
		t.Fatalf("scales = %v", scales)
	}
	if !(Event{Kind: EventPan, Phase: GestureChanged}).Mergeable() || (Event{Kind: EventPan, Phase: GestureEnded}).Mergeable() {
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
		{PanActivation{}, Point{3, 4}, 0},
		{PanActivation{}, Point{0, -12}, 1},
		{horizontal, Point{-12, 2}, 1},
		{horizontal, Point{4, 6}, -1},
		{horizontal, Point{6, 2}, 0},
	} {
		if got := c.a.Decide(c.d); got != c.want {
			t.Errorf("%+v.Decide(%v) = %d, want %d", c.a, c.d, got, c.want)
		}
	}
}

func TestWindowCommands(t *testing.T) {
	h := newHost(t, Size{200, 100}, func(cx *Scope) Node {
		cx.SetTitleBar(TitleBarHidden, 40)
		return View{TestID: "bar", WindowDrag: WindowDragOn}
	})
	b := h.commit()
	var cmd *WindowCommand
	for _, it := range b.Items {
		if it.Kind == ItemWindow {
			cmd = it.Window
		}
	}
	if cmd == nil || *cmd != (WindowCommand{Action: WindowSetTitleBar, TitleBar: TitleBarHidden, Height: 40}) {
		t.Fatalf("window command = %+v", cmd)
	}
	if p := h.find("bar").props; p.WindowDrag != WindowDragOn {
		t.Fatalf("props = %+v", p)
	}
}

func TestApplicationMenuFollowsItsOwner(t *testing.T) {
	var log []string
	var open *Signal[bool]
	h := newHost(t, Size{100, 100}, func(cx *Scope) Node {
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
	var menu []MenuEntry
	for _, it := range h.commit().Items {
		if it.Kind == ItemMenu {
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
	h.tree.Dispatch(Event{Kind: EventMenu, Count: file[0].ID})
	open.Set(false)
	restored := false
	for _, it := range h.commit().Items {
		restored = restored || it.Kind == ItemMenu && it.Menu == nil
	}
	h.tree.Dispatch(Event{Kind: EventMenu, Count: file[0].ID})
	if !restored || !slices.Equal(log, []string{"new"}) {
		t.Fatalf("restored = %v, log = %q", restored, log)
	}
}

func TestBordersPerSide(t *testing.T) {
	red, blue := RGB(0xff0000), RGB(0x0000ff)
	for _, c := range []struct {
		s    Style
		want ViewStyle
	}{
		{Style{BorderWidth: 2, BorderColor: red}, ViewStyle{BorderWidth: 2, BorderColor: red}},
		{Style{Borders: All(3), BorderColor: red}, ViewStyle{BorderWidth: 3, BorderColor: red}},
		{Style{Borders: Edges{Bottom: 1}, BorderColor: red},
			ViewStyle{Borders: Edges{Bottom: 1}, BorderColors: AllColors(red)}},
		{Style{BorderWidth: 2, BorderColor: red, BorderColors: EdgeColors{Left: blue}},
			ViewStyle{Borders: All(2), BorderColors: EdgeColors{red, red, red, blue}}},
	} {
		got := c.s.viewStyle()
		got.Opacity = 0
		if got != c.want {
			t.Errorf("%+v: got %+v, want %+v", c.s, got, c.want)
		}
	}
}
