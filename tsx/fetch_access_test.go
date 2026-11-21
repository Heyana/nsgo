package tsx

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx/internal/js"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// Hold the engine in an unrelated owner's computation until the fetch worker posts its next action. The redirect must wait for its own sandbox's policy instead of reading the engine's unrelated owner.
func TestFetchRedirectUsesItsSandboxWhileAnotherOwnerRuns(t *testing.T) {
	for _, test := range []struct {
		name      string
		inherited bool
		allowed   bool
		dispose   bool
	}{
		{name: "sandbox capability"},
		{name: "inherited host restriction", inherited: true},
		{name: "allowed redirect", allowed: true},
		{name: "disposed while the redirect waits", dispose: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			posts := make(chan func(), 8)
			root := core.NewRoot(core.RootOptions{Post: func(fn func()) { posts <- fn }})
			defer root.Dispose()
			owner := root.Child()
			caps := &pkg.Capabilities{Network: &pkg.Network{Hosts: []string{"api.example.com"}}}
			owner.Run(func() { owner.ProvideAccess(access{caps}) })
			if test.inherited {
				owner = owner.Child()
				owner.Run(func() {
					owner.ProvideAccess(access{&pkg.Capabilities{Network: &pkg.Network{Hosts: []string{"*"}}}})
				})
			}
			r := decoding()
			r.cx, r.sandbox = owner, &sandboxed{caps: true}
			started, release := make(chan struct{}), make(chan struct{})
			requests := make(chan string, 2)
			location := "https://evil.io/"
			if test.allowed {
				location = "https://api.example.com/next"
			}
			fetcherKey.Provide(owner, func(req *http.Request) (*http.Response, error) {
				requests <- req.URL.String()
				if req.URL.Path == "/away" {
					close(started)
					<-release
					return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {location}},
						Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Request: req}, nil
			})
			completed, success := false, false
			var code, permission, resource string
			owner.Run(func() {
				r.fetch(js.FunctionCall{Arguments: []js.Value{
					r.vm.ToValue("https://api.example.com/away"), r.vm.ToValue("GET"), r.vm.NewArray(), js.Undefined(), r.vm.ToValue("follow"),
					r.function(func(js.FunctionCall) js.Value {
						completed, success = true, true
						return js.Undefined()
					}),
					r.function(func(call js.FunctionCall) js.Value {
						completed = true
						err := r.vm.ToObject(call.Argument(0))
						code, permission, resource = err.Get("code").String(), err.Get("permission").String(), err.Get("resource").String()
						return js.Undefined()
					}),
				}})
			})
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("the initial request did not start")
			}
			next := func() func() {
				t.Helper()
				select {
				case fn := <-posts:
					return fn
				case <-time.After(5 * time.Second):
					t.Fatal("the fetch worker did not post its next action")
					return nil
				}
			}
			var step func()
			root.Run(func() {
				close(release)
				step = next()
			})
			if test.dispose {
				// Dispose the runtime's scope alone while the app lives on: the queued check is dropped, as the engine drops what a disposed scope posted, and the worker must still finish instead of waiting for the app to end.
				owner.Dispose()
				step()
				next()()
				if completed || len(requests) != 1 {
					t.Fatal("the disposed fetch completed or sent its redirect")
				}
				return
			}
			for !completed {
				step()
				if !completed {
					step = next()
				}
			}
			if success != test.allowed || !test.allowed && (code != core.CodeAccessDenied || permission != core.AccessNetwork || resource != location) {
				t.Fatalf("redirect success %v, error %q %q %q; allowed %v", success, code, permission, resource, test.allowed)
			}
			if got := <-requests; got != "https://api.example.com/away" {
				t.Fatalf("initial request %q", got)
			}
			if !test.allowed && len(requests) != 0 {
				t.Fatalf("refused redirect reached the network: %s", <-requests)
			}
		})
	}
}
