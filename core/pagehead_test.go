package core

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func pageHeadItems(b *driver.MountBatch) []driver.PageHead {
	var heads []driver.PageHead
	for _, it := range b.Items {
		if it.Kind == driver.ItemPageHead {
			heads = append(heads, *it.Head)
		}
	}
	return heads
}

func TestPageHeadLayersFollowTheirVisibleOwners(t *testing.T) {
	h := newHost(t, Size{Width: 300, Height: 300}, nil)
	h.tree.SetEnvironment(Environment{OS: OSWeb, CanSetPageHead: true})
	var page, hidden, all *Signal[bool]
	var title *Signal[string]
	h.tree.Mount(func(cx *Scope) Node {
		page, hidden, all = NewSignal(cx, false), NewSignal(cx, false), NewSignal(cx, true)
		title = NewSignal(cx, "Page")
		return Show{When: all.Get, Then: func(cx *Scope) Node {
			cx.BindWebHead(func() string { return "App" }, func() string { return "Description" })
			return View{Children: []Node{Show{When: page.Get, Then: func(cx *Scope) Node {
				cx.BindWebHead(title.Get, nil)
				return View{StyleFunc: func() Style {
					if hidden.Get() {
						return Style{Display: DisplayNone}
					}
					return Style{}
				}}
			}}}}
		}}
	})
	check := func(want driver.PageHead) {
		t.Helper()
		got := pageHeadItems(h.commit())
		if len(got) != 1 || got[0] != want {
			t.Fatalf("heads=%v, want %v", got, want)
		}
	}
	base := driver.PageHead{Title: "App", Description: "Description", HasTitle: true, HasDescription: true}
	check(base)
	page.Set(true)
	check(driver.PageHead{Title: "Page", Description: "Description", HasTitle: true, HasDescription: true})
	title.Set("")
	check(driver.PageHead{Description: "Description", HasTitle: true, HasDescription: true})
	hidden.Set(true)
	check(base)
	hidden.Set(false)
	check(driver.PageHead{Description: "Description", HasTitle: true, HasDescription: true})
	page.Set(false)
	check(base)
	all.Set(false)
	check(driver.PageHead{})
	if got := pageHeadItems(h.commit()); len(got) != 0 || len(h.tree.pageHead.layers) != 0 {
		t.Fatalf("disposed metadata remains: %v, layers=%d", got, len(h.tree.pageHead.layers))
	}
}

func TestPageHeadNativeAndConfinedOwnersLeaveTheHostAlone(t *testing.T) {
	for _, os := range []OS{OSMacOS, OSWeb} {
		h := newHost(t, Size{Width: 300, Height: 300}, nil)
		h.tree.SetEnvironment(Environment{OS: os, CanSetPageHead: os == OSWeb})
		read := false
		h.tree.Mount(func(cx *Scope) Node {
			if os == OSWeb {
				cx.Confine()
			}
			cx.BindWebHead(func() string { read = true; return "Plugin" }, nil)
			return View{Web: WebViewProps{Href: "/escape"}}
		})
		if got := pageHeadItems(h.commit()); len(got) != 0 || read {
			t.Fatalf("%v changed host metadata: %v read=%v", os, got, read)
		}
		if os == OSWeb && h.tree.view.children[0].props.Href != "" {
			t.Fatal("a confined view advertises a native browser escape")
		}
	}
}
