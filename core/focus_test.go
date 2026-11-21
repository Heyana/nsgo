package core

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestMoveFocusDirectionalGridAndSkipsNonStops(t *testing.T) {
	var root *Scope
	var pressed []string
	var backs int
	h := newHost(t, Size{Width: 300, Height: 300}, func(cx *Scope) Node {
		root = cx
		cx.OnBackPress(func() bool {
			backs++
			return true
		})
		btn := func(id string, dis, noFocus bool) Node {
			return View{
				TestID:   id,
				Style:    Style{Width: Pt(100), Height: Pt(40)},
				Disabled: func() bool { return dis },
				NoFocus:  noFocus,
				OnPress:  func(e PressEvent) { pressed = append(pressed, id) },
			}
		}
		return View{Style: Style{Direction: Column, Gap: 10}, Children: []Node{
			View{Style: Style{Direction: Row, Gap: 10}, Children: []Node{
				btn("a", false, false),
				btn("disabled", true, false),
				btn("b", false, false),
			}},
			View{Style: Style{Direction: Row, Gap: 10}, Children: []Node{
				btn("c", false, false),
				btn("nofocus", false, true),
				btn("d", false, false),
			}},
		}}
	})
	h.root.Env().set(Environment{OS: OSMacOS})
	h.commit()

	focusAndApply := func(dir FocusDirection, wrap bool) NodeID {
		t.Helper()
		if !MoveFocus(root, dir, wrap) {
			return 0
		}
		c := command(h.commit(), driver.ItemFocus)
		if c == nil {
			t.Fatal("MoveFocus moved without emitting ItemFocus")
		}
		h.tree.Dispatch(driver.Event{Kind: driver.EventFocus, ID: c.ID})
		h.commit()
		return c.ID
	}

	a, b, cNode, d := h.find("a").id, h.find("b").id, h.find("c").id, h.find("d").id

	// Initial Down focuses first stop (a).
	if got := focusAndApply(FocusDirDown, false); got != a {
		t.Fatalf("first Down focused %d, want a (%d)", got, a)
	}
	// Right skips disabled and lands on b.
	if got := focusAndApply(FocusDirRight, false); got != b {
		t.Fatalf("Right from a focused %d, want b (%d)", got, b)
	}
	// Down from b lands on d (right column, second row).
	if got := focusAndApply(FocusDirDown, false); got != d {
		t.Fatalf("Down from b focused %d, want d (%d)", got, d)
	}
	// Left from d skips nofocus and lands on c.
	if got := focusAndApply(FocusDirLeft, false); got != cNode {
		t.Fatalf("Left from d focused %d, want c (%d)", got, cNode)
	}
	// Down from c with wrap=false stays put.
	if got := focusAndApply(FocusDirDown, false); got != 0 {
		t.Fatalf("Down from bottom with wrap=false moved to %d", got)
	}
	// Down from c with wrap=true wraps in Tab order to d.
	if got := focusAndApply(FocusDirDown, true); got != d {
		t.Fatalf("Down from c with wrap=true moved to %d, want d (%d)", got, d)
	}
	// Up from d returns to b.
	if got := focusAndApply(FocusDirUp, false); got != b {
		t.Fatalf("Up from d focused %d, want b (%d)", got, b)
	}

	// ActivateFocused presses b with Keyboard modality.
	if !ActivateFocused(root) {
		t.Fatal("ActivateFocused returned false on focused button")
	}
	if len(pressed) != 1 || pressed[0] != "b" {
		t.Fatalf("pressed = %v, want [b]", pressed)
	}

	// TriggerBack runs OnBackPress.
	TriggerBack(root)
	if backs != 1 {
		t.Fatalf("backs = %d, want 1", backs)
	}
}

func TestMoveFocusOnPhonesUsesNativeFocus(t *testing.T) {
	// Android and iPhones focus pressable views natively (driver.Environment.FocusesViews), so a gamepad move commands ItemFocus there too and the platform's report moves the focus, as on desktops.
	var root *Scope
	var focused, pressed []string
	h := newHost(t, Size{Width: 300, Height: 300}, func(cx *Scope) Node {
		root = cx
		return View{Style: Style{Direction: Column, Gap: 10}, Children: []Node{
			View{
				TestID:  "first",
				Style:   Style{Width: Pt(120), Height: Pt(40)},
				OnFocus: func() { focused = append(focused, "first:in") },
				OnBlur:  func() { focused = append(focused, "first:out") },
				OnPress: func(PressEvent) { pressed = append(pressed, "first") },
			},
			View{
				TestID:  "second",
				Style:   Style{Width: Pt(120), Height: Pt(40)},
				OnFocus: func() { focused = append(focused, "second:in") },
				OnPress: func(PressEvent) { pressed = append(pressed, "second") },
			},
		}}
	})
	h.root.Env().set(Environment{OS: OSAndroid})
	h.commit()

	move := func(want string) {
		t.Helper()
		if !MoveFocus(root, FocusDirDown, false) {
			t.Fatalf("MoveFocus to %s failed", want)
		}
		c := command(h.commit(), driver.ItemFocus)
		if c == nil || c.ID != h.find(want).id {
			t.Fatalf("MoveFocus commanded %+v, want ItemFocus on %s", c, want)
		}
		if f := FocusedRef(root); f != nil && f.n == h.find(want) {
			t.Fatalf("%s took the focus before the platform reported it", want)
		}
		if old := FocusedRef(root); old != nil {
			h.tree.Dispatch(driver.Event{Kind: driver.EventBlur, ID: old.n.id})
		}
		h.tree.Dispatch(driver.Event{Kind: driver.EventFocus, ID: c.ID})
		h.commit()
		if f := FocusedRef(root); f == nil || f.n != h.find(want) {
			t.Fatalf("focused = %v, want %s", f, want)
		}
	}
	move("first")
	move("second")
	if !ActivateFocused(root) {
		t.Fatal("ActivateFocused failed")
	}
	if len(pressed) != 1 || pressed[0] != "second" {
		t.Fatalf("pressed = %v, want [second]", pressed)
	}
	if want := []string{"first:in", "first:out", "second:in"}; !slices.Equal(focused, want) {
		t.Fatalf("focus events = %v, want %v", focused, want)
	}
}
