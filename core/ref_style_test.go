package core

import "testing"

// TestRefSetStyleKeepsWhatItHoldsMounted: the point of SetStyle is that the
// subtree stays. A version that rebuilt the element would unmount the child
// and mount it again, which is the cost this exists to avoid.
func TestRefSetStyleKeepsWhatItHoldsMounted(t *testing.T) {
	body := &Ref{}
	child := &Ref{}
	h := newHost(t, Size{Width: 200, Height: 120}, func(cx *Scope) Node {
		return View{TestID: "body", Ref: body, Children: []Node{
			View{TestID: "child", Ref: child, Style: Style{Height: Pt(20)}},
		}}
	})
	h.commit()
	if n := h.find("child"); n == nil {
		t.Fatal("child not mounted")
	}
	childHeight := h.find("child").style.Height

	body.SetStyle(Style{Visibility: VisibilityHidden})
	b := h.commit()

	if n := h.find("child"); n == nil || n.deleted {
		t.Fatal("SetStyle unmounted the child")
	}
	if got := h.find("child").style.Height; got != childHeight {
		t.Fatalf("child height = %v, want %v", got, childHeight)
	}
	if h.find("body").style.Visibility != VisibilityHidden {
		t.Fatalf("body visibility = %v, want hidden", h.find("body").style.Visibility)
	}
	if got := len(b.Items); got != 1 {
		t.Fatalf("batch = %d items, want just the view's style", got)
	}
}

// TestRefSetClassKeepsWhatItHoldsMounted: the same for classes, which the
// panel swaps between its strip and its card.
func TestRefSetClassKeepsWhatItHoldsMounted(t *testing.T) {
	body := &Ref{}
	h := newHost(t, Size{Width: 200, Height: 120}, func(cx *Scope) Node {
		return View{TestID: "body", Ref: body, Class: "p-2", Children: []Node{
			View{TestID: "child", Style: Style{Height: Pt(20)}},
		}}
	})
	h.commit()

	body.SetClass("p-4")
	h.commit()

	if n := h.find("child"); n == nil || n.deleted {
		t.Fatal("SetClass unmounted the child")
	}
	if pad := h.find("body").style.Padding.Top; pad != 16 {
		t.Fatalf("body padding top = %v, want 16 after SetClass", pad)
	}
}
