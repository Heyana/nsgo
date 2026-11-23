//go:build !js || nsgoautomation

package miniprogram

import (
	"bytes"
	"errors"
	"image/png"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func maskOf(kinds ...driver.EventKind) driver.EventMask {
	var m driver.EventMask
	for _, k := range kinds {
		m |= 1 << k
	}
	return m
}

func TestLiveSurfacePressAndModals(t *testing.T) {
	m, h := mounted()
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemEvents, ID: 2, Events: maskOf(driver.EventPress, driver.EventPressDown, driver.EventPressUp, driver.EventLongPress)},
	}})
	s := &liveSurface{m: m}

	// Tap hits view 2 (origin 10,20 size 200x100).
	if err := s.Press(driver.Point{X: 30, Y: 40}, 0, driver.Modifiers{}); err != nil {
		t.Fatal(err)
	}
	var kinds []driver.EventKind
	for _, ev := range h.events {
		kinds = append(kinds, ev.Kind)
	}
	if len(kinds) != 3 || kinds[0] != driver.EventPressDown || kinds[1] != driver.EventPressUp || kinds[2] != driver.EventPress {
		t.Fatalf("tap events = %v, want [PressDown PressUp Press]", kinds)
	}

	// Long press dispatches EventLongPress instead of EventPress.
	h.events = nil
	if err := s.Press(driver.Point{X: 30, Y: 40}, 10*time.Millisecond, driver.Modifiers{}); err != nil {
		t.Fatal(err)
	}
	if len(h.events) != 3 || h.events[2].Kind != driver.EventLongPress {
		t.Fatalf("long press events = %+v, want final EventLongPress", h.events)
	}

	// Keyboard modifiers and secondary click are refused.
	if err := s.Press(driver.Point{X: 30, Y: 40}, 0, driver.Modifiers{Platform: true}); err == nil {
		t.Fatal("Press with modifiers succeeded")
	}
	if err := s.RightClick(driver.Point{X: 30, Y: 40}); err == nil {
		t.Fatal("RightClick succeeded")
	}

	// A modal blocks hits to views beneath it and receives hits on its own children.
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 10, View: driver.KindModal},
		{Kind: driver.ItemCreate, ID: 11, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 10, Parent: 1},
		{Kind: driver.ItemInsert, ID: 11, Parent: 10},
		frame(10, 0, 0, 375, 600),
		frame(11, 20, 20, 80, 40),
		{Kind: driver.ItemEvents, ID: 11, Events: maskOf(driver.EventPress)},
	}})
	h.events = nil
	// Point (150, 50) hits view 2 underneath, but modal 10 covers it.
	if err := s.Press(driver.Point{X: 150, Y: 50}, 0, driver.Modifiers{}); err != nil {
		t.Fatal(err)
	}
	if len(h.events) != 0 {
		t.Fatalf("press leaked through modal: %+v", h.events)
	}
	// Point (40, 40) hits modal button 11.
	if err := s.Press(driver.Point{X: 40, Y: 40}, 0, driver.Modifiers{}); err != nil {
		t.Fatal(err)
	}
	if len(h.events) != 1 || h.events[0].ID != 11 || h.events[0].Kind != driver.EventPress {
		t.Fatalf("modal press events = %+v, want press on 11", h.events)
	}
}

func TestLiveSurfaceInputTypeClearSubmitAndKeys(t *testing.T) {
	m, h := mounted()
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 4, View: driver.KindInput},
		{Kind: driver.ItemInsert, ID: 4, Parent: 1},
		frame(4, 10, 200, 200, 40),
		{Kind: driver.ItemInput, ID: 4, Input: &driver.InputConfig{Placeholder: "Name", MaxLength: 10}},
		{Kind: driver.ItemEvents, ID: 4, Events: maskOf(driver.EventFocus, driver.EventBlur, driver.EventChangeText, driver.EventSubmitEditing)},
	}})
	s := &liveSurface{m: m}

	// Typing before focus fails.
	if err := s.Type("hi"); err == nil {
		t.Fatal("Type without focus succeeded")
	}

	// Tap input 4 to focus.
	h.events, h.batches = nil, nil
	if err := s.Press(driver.Point{X: 50, Y: 220}, 0, driver.Modifiers{}); err != nil {
		t.Fatal(err)
	}
	if m.focused == nil || m.focused.id != 4 {
		t.Fatalf("focused = %v, want 4", m.focused)
	}
	if len(h.events) != 1 || h.events[0].Kind != driver.EventFocus {
		t.Fatalf("focus events = %+v", h.events)
	}

	// Type "Ada", then Backspace -> "Ad", then mod+a + Backspace -> "", then Type "Grace" + Enter.
	h.events = nil
	if err := s.Type("Ada"); err != nil {
		t.Fatal(err)
	}
	if got := m.view(4).value; got != "Ada" {
		t.Fatalf("value after Type = %q, want Ada", got)
	}
	if err := s.Key("Backspace"); err != nil {
		t.Fatal(err)
	}
	if got := m.view(4).value; got != "Ad" {
		t.Fatalf("value after Backspace = %q, want Ad", got)
	}
	if err := s.Key("mod+a"); err != nil {
		t.Fatal(err)
	}
	if err := s.Key("Backspace"); err != nil {
		t.Fatal(err)
	}
	if got := m.view(4).value; got != "" {
		t.Fatalf("value after mod+a + Backspace = %q, want empty", got)
	}
	if err := s.Type("Grace"); err != nil {
		t.Fatal(err)
	}
	h.events = nil
	if err := s.Key("Enter"); err != nil {
		t.Fatal(err)
	}
	if len(h.events) != 1 || h.events[0].Kind != driver.EventSubmitEditing || h.events[0].Text != "Grace" {
		t.Fatalf("submit events = %+v", h.events)
	}

	// Escape and Back dispatch EventBack.
	h.events = nil
	if err := s.Key("Escape"); err != nil {
		t.Fatal(err)
	}
	if err := s.Back(); err != nil {
		t.Fatal(err)
	}
	if len(h.events) != 2 || h.events[0].Kind != driver.EventBack || h.events[1].Kind != driver.EventBack {
		t.Fatalf("back events = %+v", h.events)
	}

	// Tapping outside the field blurs it.
	h.events = nil
	if err := s.Press(driver.Point{X: 300, Y: 500}, 0, driver.Modifiers{}); err != nil {
		t.Fatal(err)
	}
	if m.focused != nil {
		t.Fatalf("focused after tap outside = %v, want nil", m.focused)
	}
	if len(h.events) != 1 || h.events[0].Kind != driver.EventBlur {
		t.Fatalf("blur events = %+v", h.events)
	}
}

func TestLiveSurfaceScrollAndDrag(t *testing.T) {
	m, h := mounted()
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 5, View: driver.KindScroll},
		{Kind: driver.ItemCreate, ID: 6, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 5, Parent: 1},
		{Kind: driver.ItemInsert, ID: 6, Parent: 5},
		frame(5, 0, 150, 300, 200),
		frame(6, 0, 450, 300, 50),
		{Kind: driver.ItemScrollConfig, ID: 5, Scroll: &driver.ScrollConfig{}},
		{Kind: driver.ItemScrollContent, ID: 5, Size: driver.Size{Width: 300, Height: 600}},
		{Kind: driver.ItemEvents, ID: 5, Events: maskOf(driver.EventScroll, driver.EventScrollEnd)},
		{Kind: driver.ItemEvents, ID: 6, Events: maskOf(driver.EventPress)},
	}})
	s := &liveSurface{m: m}

	// Scrolling outside any scroll view returns an error.
	if err := s.Scroll(driver.Point{X: 50, Y: 10}, driver.Point{Y: 100}); err == nil {
		t.Fatal("Scroll outside scroll view succeeded")
	}

	// Scroll down past max (600 - 200 = 400) clamps to 400.
	h.events, h.batches = nil, nil
	if err := s.Scroll(driver.Point{X: 50, Y: 200}, driver.Point{Y: 500}); err != nil {
		t.Fatal(err)
	}
	if got := m.view(5).offset.Y; got != 400 {
		t.Fatalf("offset.Y = %v, want 400", got)
	}
	if ops := h.ops("so"); len(ops) == 0 {
		t.Fatal("Scroll emitted no so op")
	}
	// After scrolling by 400, child 6 (y=450..500 inside scroll at y=150) is at page y=200..250.
	h.events = nil
	if err := s.Press(driver.Point{X: 50, Y: 220}, 0, driver.Modifiers{}); err != nil {
		t.Fatal(err)
	}
	if len(h.events) != 1 || h.events[0].ID != 6 || h.events[0].Kind != driver.EventPress {
		t.Fatalf("scrolled child press = %+v, want press on 6", h.events)
	}

	// Drag drives pan gesture phases on view 2.
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemEvents, ID: 2, Events: maskOf(driver.EventPan)},
	}})
	h.events = nil
	if err := s.Drag([]driver.Point{{X: 20, Y: 30}, {X: 80, Y: 30}}); err != nil {
		t.Fatal(err)
	}
	var began, ended bool
	for _, ev := range h.events {
		if ev.Kind == driver.EventPan && ev.ID == 2 {
			if ev.Phase == driver.GestureBegan {
				began = true
			}
			if ev.Phase == driver.GestureEnded {
				ended = true
			}
		}
	}
	if !began || !ended {
		t.Fatalf("pan drag events missing began/ended: %+v", h.events)
	}
}

func TestLiveSurfaceAlertsMenusAndScreenshot(t *testing.T) {
	m, h := mounted()
	m.viewport = driver.Size{Width: 375, Height: 600}
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemViewStyle, ID: 2, ViewStyle: &driver.ViewStyle{Background: 0xff0000ff}},
		{Kind: driver.ItemAlert, ID: 0, Alert: &driver.AlertConfig{
			ID:      7,
			Title:   "Keep going?",
			Actions: []driver.AlertAction{{Label: "Yes"}, {Label: "No", Role: driver.AlertCancel}},
		}},
	}})
	s := &liveSurface{m: m}

	if err := s.Answer(99, 0); err == nil {
		t.Fatal("Answer unknown alert succeeded")
	}
	if err := s.Answer(7, 5); err == nil {
		t.Fatal("Answer out-of-range button succeeded")
	}
	h.events = nil
	if err := s.Answer(7, 0); err != nil {
		t.Fatal(err)
	}
	if len(h.events) != 1 || h.events[0].Kind != driver.EventAlert || h.events[0].Count != 7 || h.events[0].Index != 0 {
		t.Fatalf("alert events = %+v", h.events)
	}
	// A duplicate answer is refused once the alert has been answered.
	if err := s.Answer(7, 0); err == nil {
		t.Fatal("duplicate Answer succeeded")
	}

	// Menus and IME composition report unsupported sentinel errors.
	if !errors.Is(s.ChooseMenu(1), testhost.ErrNoMenu) || !errors.Is(s.ChooseDockMenu(1), testhost.ErrNoMenu) || !errors.Is(s.ChoosePopup(1), testhost.ErrNoMenu) {
		t.Fatal("menus did not report ErrNoMenu")
	}
	if !errors.Is(s.Compose("a"), testhost.ErrNoInputMethod) || !errors.Is(s.Commit("a"), testhost.ErrNoInputMethod) {
		t.Fatal("IME did not report ErrNoInputMethod")
	}

	// Screenshot produces a valid PNG matching the viewport with view 2's red fill.
	raw, err := s.Screenshot()
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("screenshot is not a valid PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 375 || b.Dy() != 600 {
		t.Fatalf("screenshot bounds = %v, want 375x600", b)
	}
	r, g, b, a := img.At(50, 50).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 || a>>8 != 255 {
		t.Fatalf("pixel (50,50) = (%d,%d,%d,%d), want red (255,0,0,255)", r>>8, g>>8, b>>8, a>>8)
	}
}
