package miniprogram

import (
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestSelectionAreaLongPressDragHandlesAndClear(t *testing.T) {
	h := &fakeHost{}
	m := newMounter(h)
	te := NewTextEngine(&approx{})
	at1 := plain("hello world")
	l1 := te.Prepare(at1).Layout(driver.Size{Width: 200, Height: 12})
	at2 := plain("skip me")
	l2 := te.Prepare(at2).Layout(driver.Size{Width: 200, Height: 12})
	at3 := plain("second line")
	l3 := te.Prepare(at3).Layout(driver.Size{Width: 200, Height: 12})

	m.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 3, View: driver.KindText},
		{Kind: driver.ItemCreate, ID: 4, View: driver.KindText},
		{Kind: driver.ItemCreate, ID: 5, View: driver.KindText},
		{Kind: driver.ItemInsert, ID: 2, Parent: 1},
		{Kind: driver.ItemInsert, ID: 3, Parent: 2},
		{Kind: driver.ItemInsert, ID: 4, Parent: 2},
		{Kind: driver.ItemInsert, ID: 5, Parent: 2},
		frame(1, 0, 0, 300, 300),
		frame(2, 10, 20, 250, 120),
		frame(3, 0, 0, 200, 12),
		frame(4, 0, 20, 200, 12),
		frame(5, 0, 40, 200, 12),
		{Kind: driver.ItemViewProps, ID: 2, ViewProps: &driver.ViewProps{SelectionArea: true}},
		{Kind: driver.ItemViewProps, ID: 3, ViewProps: &driver.ViewProps{SelectionColor: driver.RGBA(0xff0000, 0.3)}},
		{Kind: driver.ItemViewProps, ID: 4, ViewProps: &driver.ViewProps{Selectable: driver.SelectableOff}},
		{Kind: driver.ItemViewProps, ID: 5, ViewProps: &driver.ViewProps{SelectionColor: driver.RGBA(0xff0000, 0.3)}},
		{Kind: driver.ItemEvents, ID: 2, Events: 1 << driver.EventSelectionChange},
		{Kind: driver.ItemText, ID: 3, Text: &driver.TextContent{Text: at1.Text, Layout: l1, Content: driver.Bounds{Size: driver.Size{Width: 200, Height: 12}}}},
		{Kind: driver.ItemText, ID: 4, Text: &driver.TextContent{Text: at2.Text, Layout: l2, Content: driver.Bounds{Size: driver.Size{Width: 200, Height: 12}}}},
		{Kind: driver.ItemText, ID: 5, Text: &driver.TextContent{Text: at3.Text, Layout: l3, Content: driver.Bounds{Size: driver.Size{Width: 200, Height: 12}}}},
	}})

	// SelectionArea enables long-press and touch tracking.
	if eOps := h.ops("e"); !strings.Contains(strings.Join(eOps, " "), "[2,12]") {
		t.Fatalf("selection area event bits = %v, want [2,12]", eOps)
	}

	// Long-press on "hello" (view 3 at page 10, 20; "hello" is x=0..25).
	m.press(2, driver.EventLongPress, driver.Point{X: 20, Y: 26})
	if len(h.events) != 1 || h.events[0].Kind != driver.EventSelectionChange || h.events[0].ID != 2 || h.events[0].Text != "hello" {
		t.Fatalf("after long press events = %+v", h.events)
	}
	if len(h.events[0].Ranges) != 1 || h.events[0].Ranges[0] != (driver.SelectedRange{ID: 3, Start: 0, End: 5}) {
		t.Fatalf("ranges = %+v", h.events[0].Ranges)
	}
	hlOps := h.ops("hl")
	if len(hlOps) == 0 || !strings.Contains(hlOps[len(hlOps)-1], `"c":"rgba(255,0,0,0.3`) || !strings.Contains(hlOps[len(hlOps)-1], `"r":[[0,0,25,12]]`) {
		t.Fatalf("hl ops = %v", hlOps)
	}
	shOps := h.ops("sh")
	if len(shOps) == 0 || !strings.Contains(shOps[len(shOps)-1], `"c":"rgba(255,0,0,1)"`) {
		t.Fatalf("sh ops = %v", shOps)
	}

	// Drag into view 5 (at page y=60, skips SelectableOff view 4!). "second" is local x=0..30 (page x=10..40).
	m.touch(2, 10, driver.GestureChanged, []finger{{id: 1, page: driver.Point{X: 25, Y: 66}}}, nil, 50*time.Millisecond)
	m.touch(2, 11, driver.GestureEnded, []finger{{id: 1, page: driver.Point{X: 25, Y: 66}}}, nil, 80*time.Millisecond)
	last := h.events[len(h.events)-1]
	if last.Text != "hello world\nsecond" {
		t.Fatalf("multi-text selection = %q, want %q", last.Text, "hello world\nsecond")
	}
	if len(last.Ranges) != 2 || last.Ranges[0].ID != 3 || last.Ranges[1].ID != 5 {
		t.Fatalf("multi-text ranges = %+v", last.Ranges)
	}

	// Copy writes selected text to the clipboard.
	if copied := m.copySelection(); copied != "hello world\nsecond" || len(h.clipboard) != 1 || h.clipboard[0] != copied {
		t.Fatalf("copySelection = %q, clipboard = %v", copied, h.clipboard)
	}

	// Dragging the end handle back to "hello " trims the selection onto view 3 alone.
	m.selectionHandle(2, 1, driver.GestureBegan, driver.Point{X: 40, Y: 66})
	m.selectionHandle(2, 1, driver.GestureChanged, driver.Point{X: 35, Y: 26})
	m.selectionHandle(2, 1, driver.GestureEnded, driver.Point{X: 35, Y: 26})
	last = h.events[len(h.events)-1]
	if len(last.Ranges) != 1 || last.Ranges[0].ID != 3 {
		t.Fatalf("after handle drag ranges = %+v (text %q)", last.Ranges, last.Text)
	}

	// Select All selects both selectable texts.
	m.selectAllSelection()
	last = h.events[len(h.events)-1]
	if last.Text != "hello world\nsecond line" {
		t.Fatalf("selectAll text = %q", last.Text)
	}

	// Touching outside clears highlights, handles and fires empty EventSelectionChange.
	m.touch(1, 20, driver.GestureBegan, []finger{{id: 2, page: driver.Point{X: 5, Y: 5}}}, nil, 100*time.Millisecond)
	last = h.events[len(h.events)-1]
	if last.Kind != driver.EventSelectionChange || last.Text != "" || len(last.Ranges) != 0 {
		t.Fatalf("after clear event = %+v", last)
	}
	if shOps = h.ops("sh"); shOps[len(shOps)-1] != `[2,null]` {
		t.Fatalf("sh after clear = %v", shOps)
	}
}

func TestStandaloneSelectableTextAndBatchRefresh(t *testing.T) {
	h := &fakeHost{}
	m := newMounter(h)
	te := NewTextEngine(&approx{})
	at := plain("copy this")
	l := te.Prepare(at).Layout(driver.Size{Width: 100, Height: 12})

	m.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindText},
		{Kind: driver.ItemInsert, ID: 2, Parent: 1},
		frame(1, 0, 0, 200, 100),
		frame(2, 0, 0, 100, 12),
		{Kind: driver.ItemViewProps, ID: 2, ViewProps: &driver.ViewProps{Selectable: driver.SelectableOn}},
		{Kind: driver.ItemEvents, ID: 2, Events: 1 << driver.EventSelectionChange},
		{Kind: driver.ItemText, ID: 2, Text: &driver.TextContent{Text: at.Text, Layout: l, Content: driver.Bounds{Size: driver.Size{Width: 100, Height: 12}}}},
	}})

	m.press(2, driver.EventLongPress, driver.Point{X: 10, Y: 6})
	if len(h.events) != 1 || h.events[0].ID != 2 || h.events[0].Text != "copy" {
		t.Fatalf("standalone selectable text = %+v", h.events)
	}

	// Deleting the selectable text clears the selection cleanly.
	m.mount(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemDelete, ID: 2}}})
	if m.selecting != nil {
		t.Fatal("selection remained after deleting area")
	}
}
