//go:build js

package web

import (
	"strconv"
	"strings"
	"syscall/js"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type historyEntry struct {
	depth int
	url   string
}

// history mirrors entry indices separately from navigation depths: addresses may create several entries at one depth. moving waits for an asynchronous history.go before applying another commit's address.
type history struct {
	owned        bool
	win          js.Value
	session      string
	entries      []historyEntry
	index, depth int
	moving       bool
	destination  int
	from         int
	baseIndex    int
	pending      *driver.PageAddress
	notify       func(driver.PageAddressChange)
}

func (s *surface) listenHistory(ls *listeners, whole bool) {
	win := js.Global().Get("window")
	if !whole || !ownsHistory(win) {
		ls.on(s.host, "mouseup", func(e js.Value) {
			if e.Get("button").Int() == 3 {
				e.Call("preventDefault")
				s.engine.Dispatch(driver.Event{Kind: driver.EventBack})
			}
		})
		ls.on(s.doc, "keydown", func(e js.Value) {
			if e.Get("key").String() == "BrowserBack" {
				e.Call("preventDefault")
				s.engine.Dispatch(driver.Event{Kind: driver.EventBack})
			}
		})
		return
	}
	s.history.start(win, ls, func(change driver.PageAddressChange) {
		if s.engine != nil {
			s.engine.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: &change})
		}
	})
}

func (h *history) start(win js.Value, ls *listeners, notify func(driver.PageAddressChange)) {
	h.owned, h.win, h.notify = true, win, notify
	h.session = strconv.FormatInt(time.Now().UnixNano(), 36)
	h.from, h.baseIndex = -1, -1
	h.entries = []historyEntry{{url: h.url()}}
	nav := win.Get("navigation")
	if nav.Truthy() && nav.Get("currentEntry").Truthy() {
		h.baseIndex = nav.Get("currentEntry").Get("index").Int()
	}
	h.browser().Call("replaceState", h.state(), "")
	ls.on(win, "popstate", func(e js.Value) { h.popState(e.Get("state")) })
	ls.on(win, "hashchange", func(js.Value) {
		if h.owned && h.url() != h.entries[h.index].url {
			h.addressEntry()
		}
	})
}

// ownsHistory leaves an embedded app's containing page alone unless its same-origin frame explicitly opts in.
func ownsHistory(win js.Value) bool {
	if win.Get("top").Equal(win) {
		return true
	}
	frame := win.Get("frameElement")
	return frame.Truthy() && frame.Call("hasAttribute", "data-nsgo-history").Bool()
}
func (h *history) browser() js.Value { return h.win.Get("history") }
func (h *history) url() string       { return h.win.Get("location").Get("href").String() }
func (h *history) state() map[string]any {
	return map[string]any{"nsgo": h.entries[h.index].depth, "session": h.session, "index": h.index}
}
func (h *history) tell(reason string) {
	if h.notify != nil {
		h.notify(driver.PageAddressChange{URL: h.url(), Reason: reason, Depth: h.entries[h.index].depth})
	}
}

func (h *history) popState(state js.Value) {
	if !h.owned {
		return
	}
	if state.Type() == js.TypeObject && state.Truthy() && state.Get("nsgo").Type() == js.TypeNumber && state.Get("session").String() != h.session {
		// A reload can leave several entries from the old player. Skip all of them before leaving through the containing page's own entry.
		h.browser().Call("back")
		return
	}
	if state.Type() != js.TypeObject || !state.Truthy() || state.Get("session").String() != h.session || state.Get("index").Type() != js.TypeNumber {
		nav := h.win.Get("navigation")
		before := h.baseIndex >= 0 && nav.Truthy() && nav.Get("currentEntry").Truthy() && nav.Get("currentEntry").Get("index").Int() < h.baseIndex
		// The page's entries from before this player (and entries of an earlier run) leave the app. A fresh same-document hash entry belongs to it.
		if !before && (state.Type() != js.TypeObject || !state.Truthy() || state.Get("session").Type() == js.TypeUndefined) && withoutHash(h.url()) == withoutHash(h.entries[h.index].url) && h.url() != h.entries[h.index].url {
			h.addressEntry()
			return
		}
		h.owned = false
		h.browser().Call("back")
		return
	}
	index := state.Get("index").Int()
	if index < 0 || index >= len(h.entries) {
		h.browser().Call("back")
		return
	}
	old := h.index
	h.index = index
	h.entries[index].url = h.url()
	if h.moving && index == h.destination {
		h.moving = false
		explicit := h.pending != nil
		h.sync()
		if !h.moving && !explicit {
			h.tell("")
		}
		return
	}
	h.moving = false
	h.from = old
	reason := "back"
	if index > old {
		reason = "forward"
	}
	h.tell(reason)
}
func withoutHash(raw string) string { head, _, _ := strings.Cut(raw, "#"); return head }
func (h *history) addressEntry() {
	h.entries = append(h.entries[:h.index+1], historyEntry{depth: h.depth, url: h.url()})
	h.index++
	h.browser().Call("replaceState", h.state(), "")
	h.tell("address")
}
func (h *history) goTo(index int) {
	if index == h.index {
		return
	}
	h.moving, h.destination = true, index
	h.browser().Call("go", index-h.index)
}
func (h *history) push(depth int, url string) {
	h.entries = append(h.entries[:h.index+1], historyEntry{depth: depth, url: url})
	h.index++
	h.browser().Call("pushState", h.state(), "", url)
	h.entries[h.index].url = h.url()
}

// setHistory applies one committed navigation/address change. Last address wins while history.go is pending; a requested push survives coalescing.
func (s *surface) setHistory(depth int, address *driver.PageAddress) { s.history.set(depth, address) }
func (h *history) set(depth int, address *driver.PageAddress) {
	if !h.owned {
		return
	}
	h.depth = depth
	if address != nil && address.Set {
		copy := *address
		copy.Push = copy.Push || h.pending != nil && h.pending.Push
		h.pending = &copy
	}
	h.sync()
}
func (h *history) sync() {
	if h.moving {
		return
	}
	current := h.entries[h.index].depth
	if h.from >= 0 && h.depth > current {
		// A back the app refused goes forward to the existing entry, retaining both its original URL and the browser's forward history.
		for i := min(h.from, len(h.entries)-1); i > h.index; i-- {
			if h.entries[i].depth == h.depth {
				h.from = -1
				h.goTo(i)
				return
			}
		}
	}
	h.from = -1
	if current > h.depth {
		target := h.index
		for target > 0 && h.entries[target].depth > h.depth {
			target--
		}
		if target != h.index {
			h.goTo(target)
			return
		}
	}
	pushed := false
	for h.entries[h.index].depth < h.depth {
		h.push(h.entries[h.index].depth+1, h.url())
		pushed = true
	}
	if p := h.pending; p != nil {
		h.pending = nil
		if p.Push && !pushed && p.URL != h.url() {
			h.push(h.depth, p.URL)
		} else {
			h.entries[h.index].url = p.URL
			h.browser().Call("replaceState", h.state(), "", p.URL)
			h.entries[h.index].url = h.url()
		}
		if h.url() != p.URL {
			h.tell("")
		}
	}
}
