package core

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"unicode"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// HistoryBehavior chooses whether SetAppURL replaces or pushes an address.
type HistoryBehavior uint8

const (
	HistoryReplace HistoryBehavior = iota
	HistoryPush
)

// AppURLOptions chooses how the browser records an application address.
type AppURLOptions struct{ History HistoryBehavior }

// AppURLChange reports an address reached by browser navigation or a hash edit. Reason is "back", "forward" or "address". Application writes do not notify these handlers; they update the tracked AppURL immediately.
type AppURLChange struct{ URL, Reason string }

type appURLHandler struct {
	owner *Scope
	fn    func(AppURLChange)
}
type appAddress struct {
	value      *Signal[string]
	page, base string
	tree       *Tree
	pending    *driver.PageAddress
	version    uint64
	handlers   []*appURLHandler
}

func (cx *Scope) appAddress() *appAddress {
	if cx.rt.address == nil {
		cx.rt.address = &appAddress{value: NewSignal(cx.rt.root, "")}
	}
	return cx.rt.address
}

// AppURL reads the app's address as a tracked value. On the web this is the page address before the app renders. Elsewhere it is the last link opened with the app or the value SetAppURL kept. A confined subtree reads none.
func (cx *Scope) AppURL() string {
	if cx == nil || cx.Disposed() || confinement(cx.current()) != nil {
		return ""
	}
	return cx.appAddress().value.Get()
}

// SetAppURL sets the app's address, replacing the browser entry by default. HistoryPush asks for a new entry; a navigation push in the same commit supplies that entry instead. Relative addresses resolve against the page's base and must stay on its origin. No navigation or network request occurs. Other targets keep the address. Confined subtrees cannot change it.
func (cx *Scope) SetAppURL(raw string, options ...AppURLOptions) error {
	if cx == nil || cx.Disposed() {
		return fmt.Errorf("core: no live scope")
	}
	if confinement(cx.current()) != nil {
		return AccessDenied("appURL", raw)
	}
	if len(options) > 1 {
		return fmt.Errorf("core: SetAppURL takes at most one options value")
	}
	var opts AppURLOptions
	if len(options) == 1 {
		opts = options[0]
	}
	if opts.History > HistoryPush {
		return fmt.Errorf("core: invalid application URL history behavior")
	}
	if len(raw) > 16<<10 || strings.ContainsRune(raw, '\\') || strings.IndexFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return fmt.Errorf("core: invalid application address")
	}
	a := cx.appAddress()
	value := raw
	if a.page != "" {
		var err error
		base := a.base
		if base == "" {
			base = a.value.GetUntracked()
		}
		value, err = resolveAppURL(raw, a.page, base)
		if err != nil {
			return err
		}
	} else if _, err := url.Parse(raw); err != nil {
		return fmt.Errorf("core: invalid application address: %w", err)
	}
	a.version++
	if a.tree != nil && a.page != "" {
		push := opts.History == HistoryPush || a.pending != nil && a.pending.Push
		a.pending = &driver.PageAddress{URL: value, Push: push, Set: true}
		a.tree.changed = true
	}
	a.value.Set(value)
	return nil
}

// OnAppURLChange observes browser back, forward and address changes while cx lives. Its returned function unsubscribes. Read AppURL for the initial address; changes made before subscribing are not queued.
func (cx *Scope) OnAppURLChange(fn func(AppURLChange)) func() {
	if cx == nil || cx.Disposed() || fn == nil || confinement(cx.current()) != nil {
		return func() {}
	}
	a := cx.appAddress()
	h := &appURLHandler{owner: cx.current(), fn: fn}
	a.handlers = append(a.handlers, h)
	stop := func() { a.handlers = slices.DeleteFunc(a.handlers, func(x *appURLHandler) bool { return x == h }) }
	h.owner.Cleanup(stop)
	return stop
}
func (cx *Scope) setAddressValue(value string) {
	a := cx.appAddress()
	if a.page == "" {
		a.value.Set(value)
	}
}
func resolveAppURL(raw, page, base string) (string, error) {
	if driver.ResolvePageAddress != nil {
		return driver.ResolvePageAddress(raw, page, base)
	}
	p, err := url.Parse(page)
	if err != nil {
		return "", err
	}
	if base == "" {
		base = page
	}
	b, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	v, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("core: invalid application address: %w", err)
	}
	v = b.ResolveReference(v)
	v.Scheme = strings.ToLower(v.Scheme)
	port := func(u *url.URL) string {
		if s := u.Port(); s != "" {
			return s
		}
		if u.Scheme == "https" {
			return "443"
		}
		return "80"
	}
	if v.User != nil || v.Opaque != "" || v.Scheme != p.Scheme || !strings.EqualFold(v.Hostname(), p.Hostname()) || port(v) != port(p) || v.Scheme != "http" && v.Scheme != "https" {
		return "", fmt.Errorf("core: application address must stay on the page's origin")
	}
	v.Scheme, v.Host = p.Scheme, p.Host
	return v.String(), nil
}
func (t *Tree) updateAppAddress(previous, env Environment) {
	if env.PageURL != "" && (t.rt.address == nil || t.rt.address.page == "") {
		a := t.root.appAddress()
		a.page, a.base, a.tree = env.PageURL, env.PageBaseURL, t
		a.value.Set(env.PageURL)
	} else if env.OpenedURL != "" && env.OpenedURL != previous.OpenedURL {
		t.root.setAddressValue(env.OpenedURL)
	}
}
func (t *Tree) pageAddressChanged(change driver.PageAddressChange) {
	a := t.root.appAddress()
	if a.tree != t || a.page == "" {
		return
	}
	value, err := resolveAppURL(change.URL, a.page, a.base)
	if err != nil {
		return
	}
	version := a.version
	if change.Reason == "back" {
		for range max(0, t.history-change.Depth) {
			t.Dispatch(driver.Event{Kind: driver.EventBack})
		}
	}
	// A refused back retains the address the app is showing. A handler may also have set a replacement address while its screen was dismissed.
	if t.historyDepth() > change.Depth || a.version != version {
		t.historyAgain, t.changed = true, true
		return
	}
	t.within(t.root, func() {
		changed := a.value.GetUntracked() != value
		a.value.Set(value)
		if changed && change.Reason != "" {
			event := AppURLChange{URL: value, Reason: change.Reason}
			for _, h := range slices.Clone(a.handlers) {
				if !h.owner.Disposed() {
					t.within(h.owner, func() { h.fn(event) })
				}
			}
		}
	})
	t.historyAgain, t.changed = true, true
}
