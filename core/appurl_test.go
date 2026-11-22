package core

import (
	"github.com/nsgo-dev/nsgo/internal/driver"
	"strings"
	"testing"
)

func addressHost(t *testing.T, app func(*Scope) Node) *host {
	h := newHost(t, Size{Width: 300, Height: 200}, nil)
	h.tree.SetEnvironment(Environment{OS: OSWeb, PageURL: "https://app.example/start"})
	h.tree.Mount(app)
	h.commit()
	return h
}
func TestAppURLInitialReactiveCoalescingAndValidation(t *testing.T) {
	var cx *Scope
	var values []string
	h := addressHost(t, func(c *Scope) Node { cx = c; c.Effect(func() { values = append(values, c.AppURL()) }); return View{} })
	if cx.AppURL() != "https://app.example/start" {
		t.Fatal(cx.AppURL())
	}
	if err := cx.SetAppURL("/folder/one", AppURLOptions{History: HistoryPush}); err != nil {
		t.Fatal(err)
	}
	if err := cx.SetAppURL("two"); err != nil {
		t.Fatal(err)
	}
	item := command(h.commit(), driver.ItemHistory)
	if item == nil || item.Address == nil || !item.Address.Set || !item.Address.Push || item.Address.URL != "https://app.example/folder/two" {
		t.Fatalf("history=%+v", item)
	}
	if len(values) != 3 {
		t.Fatalf("tracked reads=%v", values)
	}
	for _, raw := range []string{"https://other.example/x", "//other.example/x", "javascript:alert(1)", "https://me@app.example/x", "/bad path", "/bad\npath", "\\\\other.example/x", strings.Repeat("a", 16385), "/%xz"} {
		if err := cx.SetAppURL(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	if err := cx.SetAppURL("/x", AppURLOptions{History: 99}); err == nil {
		t.Fatal("invalid history accepted")
	}
	if cx.AppURL() != "https://app.example/folder/two" {
		t.Fatal("rejected write changed address")
	}
}
func TestAppURLBaseNativeLinksAndConfinedSubtrees(t *testing.T) {
	h := newHost(t, Size{Width: 300, Height: 200}, func(*Scope) Node { return View{} })
	h.tree.SetEnvironment(Environment{PageURL: "https://app.example/a", PageBaseURL: "https://app.example/base/"})
	if err := h.root.SetAppURL("next"); err != nil {
		t.Fatal(err)
	}
	if h.root.AppURL() != "https://app.example/base/next" {
		t.Fatal(h.root.AppURL())
	}
	confined := h.root.Child()
	confined.Confine()
	if confined.AppURL() != "" || confined.SetAppURL("/outside") == nil {
		t.Fatal("confined code reached application address")
	}
	native := newHost(t, Size{}, func(*Scope) Node { return View{} })
	native.tree.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: []string{"demo://first", "demo://last"}})
	if native.root.AppURL() != "demo://last" {
		t.Fatal(native.root.AppURL())
	}
	if err := native.root.SetAppURL("demo://chosen"); err != nil || native.root.AppURL() != "demo://chosen" {
		t.Fatal(err)
	}
}
func TestAppURLTraversalCallbacksAndUnsubscribe(t *testing.T) {
	h := addressHost(t, func(*Scope) Node { return View{} })
	var got []AppURLChange
	child := h.root.Child()
	stop := child.OnAppURLChange(func(e AppURLChange) { got = append(got, e) })
	change := func(u, reason string) {
		h.tree.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: &driver.PageAddressChange{URL: u, Reason: reason}})
		h.commit()
	}
	change("https://app.example/back", "back")
	if h.root.AppURL() != "https://app.example/back" || len(got) != 1 || got[0].Reason != "back" {
		t.Fatalf("url=%s changes=%v", h.root.AppURL(), got)
	}
	stop()
	stop()
	change("https://app.example/forward", "forward")
	if len(got) != 1 {
		t.Fatal("unsubscribed callback ran")
	}
	child.OnAppURLChange(func(e AppURLChange) { got = append(got, e) })
	child.Dispose()
	change("https://app.example/#hash", "address")
	if len(got) != 1 {
		t.Fatal("disposed callback ran")
	}
	change("https://evil.example/", "address")
	if h.root.AppURL() != "https://app.example/#hash" {
		t.Fatal("cross-origin browser payload accepted")
	}
}
func TestAppURLBackRespectsModalRefusalAndDismissalWrites(t *testing.T) {
	for _, mode := range []string{"accept", "refuse", "replace"} {
		t.Run(mode, func(t *testing.T) {
			var visible *Signal[bool]
			var cx *Scope
			h := addressHost(t, func(c *Scope) Node {
				cx = c
				visible = NewSignal(c, true)
				return Show{When: visible.Get, Then: func(*Scope) Node {
					return Modal{OnRequestClose: func(string) {
						if mode != "refuse" {
							visible.Set(false)
						}
						if mode == "replace" {
							if err := cx.SetAppURL("/chosen"); err != nil {
								t.Fatal(err)
							}
						}
					}, Children: []Node{View{}}}
				}}
			})
			if err := cx.SetAppURL("/modal"); err != nil {
				t.Fatal(err)
			}
			h.commit()
			h.tree.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: &driver.PageAddressChange{URL: "https://app.example/start", Reason: "back", Depth: 0}})
			h.commit()
			want := "https://app.example/start"
			if mode == "refuse" {
				want = "https://app.example/modal"
			}
			if mode == "replace" {
				want = "https://app.example/chosen"
			}
			if cx.AppURL() != want {
				t.Fatalf("URL=%q want%q", cx.AppURL(), want)
			}
		})
	}
}

func TestAppURLReactiveReplacementWinsOverItsTrigger(t *testing.T) {
	var cx *Scope
	h := addressHost(t, func(c *Scope) Node {
		cx = c
		c.Effect(func() {
			if c.AppURL() == "https://app.example/redirect" {
				if err := c.SetAppURL("/destination"); err != nil {
					t.Error(err)
				}
			}
		})
		return View{}
	})
	if err := cx.SetAppURL("/redirect", AppURLOptions{History: HistoryPush}); err != nil {
		t.Fatal(err)
	}
	item := command(h.commit(), driver.ItemHistory)
	if cx.AppURL() != "https://app.example/destination" || item.Address.URL != cx.AppURL() || !item.Address.Push {
		t.Fatalf("signal=%s item=%+v", cx.AppURL(), item.Address)
	}
}

func TestAppURLStaleEnvironmentDoesNotOverwriteApplicationWrite(t *testing.T) {
	h := addressHost(t, func(*Scope) Node { return View{} })
	if err := h.root.SetAppURL("/chosen"); err != nil {
		t.Fatal(err)
	}
	h.tree.SetEnvironment(Environment{OS: OSWeb, PageURL: "https://app.example/older", Lifecycle: LifecycleActive})
	h.commit()
	h.tree.SetEnvironment(Environment{OS: OSWeb, PageURL: "https://app.example/start", Lifecycle: LifecycleInactive})
	if h.root.AppURL() != "https://app.example/chosen" {
		t.Fatal("environment replay overwrote application URL", h.root.AppURL())
	}
}

func TestAppURLTraversalAtTheSameAddressDoesNotNotify(t *testing.T) {
	h := addressHost(t, func(*Scope) Node { return View{} })
	changes := 0
	h.root.OnAppURLChange(func(AppURLChange) { changes++ })
	h.tree.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: &driver.PageAddressChange{URL: h.root.AppURL(), Reason: "back"}})
	h.commit()
	if changes != 0 {
		t.Fatal("unchanged address emitted a change")
	}
}
