//go:build js && wasm

package harmonyos

import (
	"encoding/json"
	"reflect"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

func TestWebMountSendsBothScripts(t *testing.T) {
	m := newMounter(nil)
	item := driver.MountItem{
		Kind: driver.ItemWeb,
		ID:   4,
		Web: &driver.WebConfig{
			Source:                          driver.WebSource{Src: "https://example.test/"},
			NoJavaScript:                    true,
			InjectedJavaScriptBeforeContent: "window.before = \"start\";",
			InjectedJavaScript:              "window.after = 'end';\n",
		},
	}
	m.item(&driver.MountBatch{}, &item)
	var got [][]any
	if err := json.Unmarshal(m.o.bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := [][]any{{"web", float64(4), item.Web.Source.Src, true,
		item.Web.InjectedJavaScriptBeforeContent, item.Web.InjectedJavaScript}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("web operations = %v, want %v", got, want)
	}
}

func TestWebCookieStoreAnswersUnavailable(t *testing.T) {
	bridge := js.Global().Get("Object").New()
	submit := js.FuncOf(func(js.Value, []js.Value) any { return nil })
	t.Cleanup(submit.Release)
	bridge.Set("submit", submit)
	bridge.Set("tick", submit)
	s := &surface{bridge: bridge}
	s.m = newMounter(s)

	type result struct {
		name string
		err  error
	}
	results := make(chan result, 5)
	answer := func(name string) func(error) {
		return func(err error) { results <- result{name, err} }
	}
	answers := make(chan driver.Event, 5)
	s.engine = engine.Start(s, func(cx *core.Scope) core.Node {
		cx.SetWebCookies([]core.WebCookie{{Name: "session", Domain: "example.test", Value: "test"}}, answer("set"))
		for name, filter := range map[string]core.WebCookieFilter{
			"all":    {},
			"domain": {Domain: "example.test"},
			"url":    {URL: "https://example.test/"},
		} {
			cx.WebCookies(filter, func(cookies []core.WebCookie, err error) {
				if cookies != nil {
					t.Errorf("%s listed %v, want nil cookies", name, cookies)
				}
				answer(name)(err)
			})
		}
		cx.ClearWebsiteData(answer("clear"))
		return core.View{}
	}, engine.Options{
		Environment: driver.Environment{HasWebView: true},
		Event: func(_ uint32, ev driver.Event) {
			if ev.Kind == driver.EventWebCookies {
				answers <- ev
			}
		},
	})
	t.Cleanup(s.engine.Close)

	pending := map[string]bool{"set": true, "all": true, "domain": true, "url": true, "clear": true}
	counts := map[uint32]bool{}
	deadline := time.After(2 * time.Second)
	for range 5 {
		select {
		case got := <-results:
			if !pending[got.name] || core.ErrorCode(got.err) != core.CodeUnavailable {
				t.Errorf("%s callback = %v, want one Unavailable answer", got.name, got.err)
			}
			delete(pending, got.name)
		case <-deadline:
			t.Fatalf("web cookie callbacks still pending: %v", pending)
		}
		select {
		case ev := <-answers:
			if ev.Count == 0 || counts[ev.Count] {
				t.Errorf("duplicate or missing request ID in %+v", ev)
			}
			counts[ev.Count] = true
		case <-deadline:
			t.Fatal("web cookie callback had no backend event")
		}
	}
}
