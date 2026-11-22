//go:build js && wasm

package web

import (
	"strings"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func addressFrame(t *testing.T) (js.Value, *listeners) {
	t.Helper()
	g := js.Global()
	doc := g.Get("document")
	frame := doc.Call("createElement", "iframe")
	loaded := make(chan struct{}, 4)
	onLoad := js.FuncOf(func(js.Value, []js.Value) any {
		select {
		case loaded <- struct{}{}:
		default:
		}
		return nil
	})
	frame.Call("addEventListener", "load", onLoad)
	frame.Set("src", g.Get("location").Get("origin").String()+"/__nsgo_address_fixture.html")
	doc.Get("body").Call("appendChild", frame)
	select {
	case <-loaded:
	case <-time.After(5 * time.Second):
		t.Fatal("address fixture frame did not load")
	}
	ls := new(listeners)
	t.Cleanup(func() { ls.release(); frame.Call("remove"); onLoad.Release() })
	win := frame.Get("contentWindow")
	win.Get("history").Call("pushState", map[string]any{"fixture": true}, "", win.Get("location").Get("href").String()+"#outside")
	win.Get("history").Call("pushState", map[string]any{"fixture": true}, "", withoutHash(win.Get("location").Get("href").String())+"#page")
	return win, ls
}
func waitAddress(t *testing.T, what string, ready func() bool) {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for !ready() {
		if time.Now().After(until) {
			t.Fatal("timed out: " + what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func TestPageAddressPushReplaceBackForwardAndHash(t *testing.T) {
	win, ls := addressFrame(t)
	var changes []driver.PageAddressChange
	h := new(history)
	h.start(win, ls, func(c driver.PageAddressChange) { changes = append(changes, c); h.set(c.Depth, nil) })
	origin := win.Get("location").Get("origin").String()
	write := func(path string, push bool) { h.set(0, &driver.PageAddress{URL: origin + path, Set: true, Push: push}) }
	write("/address/one", false)
	length := h.browser().Get("length").Int()
	write("/address/two", true)
	write("/address/three", true)
	if h.browser().Get("length").Int() != length+2 {
		t.Fatal("pushed addresses did not add two entries")
	}
	h.browser().Call("back")
	waitAddress(t, "address back", func() bool { return len(changes) == 1 })
	if h.url() != origin+"/address/two" || changes[0].Reason != "back" {
		t.Fatalf("back URL=%s changes=%v", h.url(), changes)
	}
	h.browser().Call("forward")
	waitAddress(t, "address forward", func() bool { return len(changes) == 2 })
	if h.url() != origin+"/address/three" || changes[1].Reason != "forward" {
		t.Fatal(changes)
	}
	win.Get("location").Set("hash", "typed")
	waitAddress(t, "hash edit", func() bool { return len(changes) == 3 })
	if changes[2].Reason != "address" || !strings.HasSuffix(changes[2].URL, "#typed") {
		t.Fatal(changes)
	}
	h.browser().Call("back")
	waitAddress(t, "hash back", func() bool { return len(changes) == 4 })
	if h.url() != origin+"/address/three" || changes[3].Reason != "back" {
		t.Fatal(changes)
	}
}
func TestPageAddressNavigationCoalescesAndWaitsForHistoryGo(t *testing.T) {
	win, ls := addressFrame(t)
	h := new(history)
	h.start(win, ls, func(driver.PageAddressChange) {})
	origin := win.Get("location").Get("origin").String()
	initial := h.browser().Get("length").Int()
	h.set(1, &driver.PageAddress{URL: origin + "/detail", Set: true, Push: true})
	if h.browser().Get("length").Int() != initial+1 {
		t.Fatal("navigation and URL made separate entries")
	}
	h.set(2, &driver.PageAddress{URL: origin + "/modal", Set: true})
	h.set(0, nil) // asynchronous traversal back across both native pages
	if !h.moving {
		t.Fatal("history.go did not enter pending state")
	}
	h.set(0, &driver.PageAddress{URL: origin + "/intermediate", Set: true, Push: true})
	h.set(0, &driver.PageAddress{URL: origin + "/latest", Set: true})
	waitAddress(t, "pending address after go", func() bool { return !h.moving && h.url() == origin+"/latest" })
	if h.index != 1 || h.entries[1].depth != 0 || h.entries[1].url != origin+"/latest" {
		t.Fatalf("entries=%+v index=%d", h.entries, h.index)
	}
}
func TestPageAddressRefusedBackRestoresExistingEntry(t *testing.T) {
	win, ls := addressFrame(t)
	h := new(history)
	refused := 0
	h.start(win, ls, func(c driver.PageAddressChange) {
		if c.Reason == "back" {
			refused++
			h.set(1, nil)
		}
	})
	origin := win.Get("location").Get("origin").String()
	h.set(0, &driver.PageAddress{URL: origin + "/home", Set: true})
	h.set(1, &driver.PageAddress{URL: origin + "/protected", Set: true})
	length := h.browser().Get("length").Int()
	h.browser().Call("back")
	waitAddress(t, "refused back", func() bool { return refused > 0 && !h.moving && h.from < 0 && h.url() == origin+"/protected" })
	if h.browser().Get("length").Int() != length || h.index != 1 {
		t.Fatal("refused back replaced history instead of restoring")
	}
}
func TestPageAddressReloadStartsAtTheNamedAddress(t *testing.T) {
	win, ls := addressFrame(t)
	old := new(history)
	old.start(win, ls, func(driver.PageAddressChange) {})
	origin := win.Get("location").Get("origin").String()
	old.set(0, &driver.PageAddress{URL: origin + "/deep/first", Set: true, Push: true})
	old.set(0, &driver.PageAddress{URL: origin + "/deep/link?film=42", Set: true, Push: true})
	// A fresh player has no stale mirror; its first entry is the current URL.
	ls.release()
	next := new(history)
	next.start(win, ls, func(driver.PageAddressChange) {})
	if next.session == old.session {
		t.Fatal("reloaded players share the same history session")
	}
	if next.entries[0].url != origin+"/deep/link?film=42" || next.index != 0 {
		t.Fatal(next.entries)
	}
	win.Get("history").Call("back")
	waitAddress(t, "leave prior player", func() bool { return !next.owned && next.url() == withoutHash(old.entries[0].url) })
}

func TestPageAddressUsesBrowserURLSerialization(t *testing.T) {
	origin := js.Global().Get("location").Get("origin").String()
	for _, c := range []struct{ raw, page, base, want string }{
		{"?q=中文", origin + "/here", "", origin + "/here?q=%E4%B8%AD%E6%96%87"},
		{"https://EXAMPLE.COM:443/next", "https://example.com/start", "", "https://example.com/next"},
		{"https://bücher.example/path", "https://xn--bcher-kva.example/start", "", "https://xn--bcher-kva.example/path"},
	} {
		got, err := resolvePageAddress(c.raw, c.page, c.base)
		if err != nil || got != c.want {
			t.Errorf("resolve %q=%q,%v", c.raw, got, err)
		}
	}
	if _, err := resolvePageAddress("https://other.example/x", origin+"/here", ""); err == nil {
		t.Fatal("cross-origin address accepted")
	}
}
