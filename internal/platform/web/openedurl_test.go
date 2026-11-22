//go:build js && wasm

package web

import (
	"os"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/mobile"
)

func TestOpenedURLTracksBrowserAddress(t *testing.T) {
	if target := os.Getenv("NSGO_TEST_TARGET"); target != "web" {
		t.Fatalf("requires nsgo test -target web, got %q", target)
	}
	g := js.Global()
	location := g.Get("location")
	initial := location.Get("origin").String() + "/opened1475/start?table=12#details"
	g.Get("history").Call("replaceState", nil, "", initial)
	first := make(chan string, 1)
	reads := make(chan string, 32)
	var scope *core.Scope
	s := open(func(cx *core.Scope) core.Node {
		scope = cx
		first <- mobile.OpenedURL(cx)
		return core.Text{ContentFunc: func() string {
			u := mobile.OpenedURL(cx)
			select {
			case reads <- u:
			default:
			}
			return u
		}}
	}, Options{}, nil)
	defer s.engine.Close()
	select {
	case got := <-first:
		if got != initial {
			t.Fatalf("first render=%q, want %q", got, initial)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no first render")
	}
	await := func(want string) {
		t.Helper()
		timeout := time.After(5 * time.Second)
		for {
			select {
			case got := <-reads:
				if got == want {
					return
				}
			case <-timeout:
				t.Fatalf("mobile.OpenedURL did not become %q", want)
			}
		}
	}
	await(initial)
	location.Set("hash", "external")
	await(location.Get("origin").String() + "/opened1475/start?table=12#external")
	errch := make(chan error, 1)
	scope.Post(func() {
		errch <- scope.SetAppURL("/opened1475/next?q=a%20b", core.AppURLOptions{History: core.HistoryPush})
	})
	if err := <-errch; err != nil {
		t.Fatal(err)
	}
	await(location.Get("origin").String() + "/opened1475/next?q=a%20b")
	g.Get("history").Call("back")
	await(location.Get("origin").String() + "/opened1475/start?table=12#external")
	g.Get("history").Call("forward")
	await(location.Get("origin").String() + "/opened1475/next?q=a%20b")
}

func TestEmbeddedOpenedURLLeavesHostAddressPrivate(t *testing.T) {
	if target := os.Getenv("NSGO_TEST_TARGET"); target != "web" {
		t.Fatalf("requires nsgo test -target web, got %q", target)
	}
	doc := js.Global().Get("document")
	mount := doc.Call("createElement", "div")
	mount.Set("id", "opened1475-embedded")
	doc.Get("body").Call("appendChild", mount)
	defer mount.Call("remove")
	first := make(chan string, 1)
	s := open(func(cx *core.Scope) core.Node { first <- mobile.OpenedURL(cx); return core.View{} }, Options{Mount: "#opened1475-embedded"}, nil)
	defer s.engine.Close()
	select {
	case got := <-first:
		if got != "" {
			t.Fatalf("embedded app inherited host address %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no first render")
	}
}
