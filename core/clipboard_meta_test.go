package core

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestClipboardChangeCountReadsTheCountAndDispatchesIt(t *testing.T) {
	var cx *Scope
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node { cx = c; return View{} })
	h.commit()

	var count int
	cx.ClipboardChangeCount(func(n int) { count = n })
	it := command(h.commit(), driver.ItemReadClipboard)
	if it == nil || it.Clipboard != driver.ClipboardReadChangeCount {
		t.Fatalf("ClipboardChangeCount sent %+v", it)
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventClipboard, Count: it.Count, ChangeCount: 42})

	if count != 42 {
		t.Fatalf("change count = %d, want 42", count)
	}
}

func TestClipboardTypesReadTheTypesAndDispatchesThem(t *testing.T) {
	var cx *Scope
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node { cx = c; return View{} })
	h.commit()

	var types []string
	cx.ClipboardTypes(func(ts []string) { types = ts })
	it := command(h.commit(), driver.ItemReadClipboard)
	if it == nil || it.Clipboard != driver.ClipboardReadTypes {
		t.Fatalf("ClipboardTypes sent %+v", it)
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventClipboard, Count: it.Count,
		Types: []string{"public.utf8-plain-text", "org.nspasteboard.TransientType"}})

	if len(types) != 2 || types[1] != "org.nspasteboard.TransientType" {
		t.Fatalf("types = %v", types)
	}
}
