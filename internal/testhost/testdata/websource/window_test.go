package websource_test

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

const windowPage = `<!doctype html>
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
body, form { margin: 0; }
a, button { display: block; box-sizing: border-box; width: 220px; height: 32px; }
</style>
<a target="_blank" href="https://allowed.invalid/link">Link</a>
<button onclick="request('click')">Script</button>
<form target="_blank" method="post" action="https://allowed.invalid/form"><button>Form</button></form>
<a target="_blank" href="https://denied.invalid/link">Denied</a>
<button onclick="window.nsgo.postMessage('input-ready')">Ready</button>
<script>
function request(source) {
    const popup = window.open('https://allowed.invalid/' + source);
    window.nsgo.postMessage(source + ':' + Boolean(popup && !popup.closed));
}
setTimeout(() => request('timer'), 50);
</script>`

type windowAccess struct{}

func (windowAccess) Allows(core.AccessRequest) bool { return true }
func (windowAccess) WebHosts() []string             { return []string{"allowed.invalid"} }

func checkWindowTarget(t *testing.T) {
	t.Helper()
	switch nsgotest.Target() {
	case "macos", "linux", "ios", "ios_simulator":
	default:
		t.Skip("checks the behavior of WebKit's native popup delegate")
	}
}

var clickWindowPage = func(a *nsgotest.App, _ *testing.T, y int) {
	a.PressAt("id:page", core.Point{X: 12, Y: core.Points(y)})
}

func startWindowPage(t *testing.T, access core.Access, web core.WebView) (*nsgotest.App, func(y int)) {
	t.Helper()
	var ref core.Ref
	var scope *core.Scope
	var container core.Style
	target := nsgotest.Target()
	ios := target == "ios" || target == "ios_simulator"
	var inputReady chan struct{}
	if ios {
		// Leave the links below the status bar in the iOS test window.
		container.Padding.Top = 88
		inputReady = make(chan struct{}, 1)
		onMessage := web.OnMessage
		web.OnMessage = func(message string) {
			if message == "input-ready" {
				select {
				case inputReady <- struct{}{}:
				default:
				}
				return
			}
			if onMessage != nil {
				onMessage(message)
			}
		}
	}
	web.Ref, web.Source, web.TestID = &ref, core.WebHTML(windowPage, ""), "page"
	web.Style = core.Style{Width: core.Pt(240), Height: core.Pt(160)}
	a := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		if access != nil {
			cx.ProvideAccess(access)
		}
		// The parent makes a test step pressable while native hit
		// testing sends the mouse input to the WebView.
		return core.View{Style: container, OnPress: func(core.PressEvent) {}, Children: []core.Node{web}}
	}, nsgotest.Size(240, 160))
	click := func(y int) {
		// WebKit is a native child view on macOS and Linux, which the test hosts' presses go around, so its clicks are Ref.SendInput's trusted input.
		if target == "macos" || target == "linux" {
			scope.Post(func() { ref.SendInput(core.WebClick(12, core.Points(y))) })
			return
		}
		clickWindowPage(a, t, y)
	}
	if !ios {
		return a, click
	}
	// The simulator can drop taps during launch. Wait for the page to
	// acknowledge an inert tap before testing the actual requests.
	for range 12 {
		click(144)
		select {
		case <-inputReady:
			return a, click
		case <-time.After(250 * time.Millisecond):
		}
	}
	t.Fatal("the iOS web view did not accept the readiness tap")
	return nil, nil
}

func TestWebWindowRequestsAndAccess(t *testing.T) {
	checkWindowTarget(t)
	target := nsgotest.Target()
	ios := target == "ios" || target == "ios_simulator"
	var mu sync.Mutex
	var requests []core.WebWindowRequest
	var problems []error
	var messages []string
	a, click := startWindowPage(t, windowAccess{}, core.WebView{
		OnOpenWindow: func(request core.WebWindowRequest) {
			mu.Lock()
			defer mu.Unlock()
			requests = append(requests, request)
		},
		OnError: func(err error) {
			mu.Lock()
			defer mu.Unlock()
			problems = append(problems, err)
		},
		OnMessage: func(message string) {
			mu.Lock()
			defer mu.Unlock()
			messages = append(messages, message)
		},
	})
	requireSuppressed := func(source string) {
		t.Helper()
		a.WaitUntil(10*time.Second, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return slices.Contains(messages, source+":false") || slices.Contains(messages, source+":true")
		})
		mu.Lock()
		defer mu.Unlock()
		if slices.Contains(messages, source+":true") {
			t.Errorf("%s opened a browser window", source)
		}
	}
	requireSuppressed("timer")
	for i, action := range []string{"link", "click", "form"} {
		t.Logf("requesting a window from %s", action)
		url := "https://allowed.invalid/" + action
		// The iOS delegate can identify links and forms by navigation type.
		wantGesture := action != "click" || !ios
		click(i*32 + 16)
		a.WaitUntil(10*time.Second, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return slices.ContainsFunc(requests, func(r core.WebWindowRequest) bool { return r.URL == url }) || len(problems) != 0
		})
		if action == "click" {
			requireSuppressed("click")
		}
		mu.Lock()
		count := 0
		for _, r := range requests {
			if r.URL == url {
				count++
				if r.UserInitiated != wantGesture {
					t.Errorf("%s user gesture = %t, want %t", action, r.UserInitiated, wantGesture)
				}
			}
		}
		if count != 1 || len(problems) != 0 {
			t.Errorf("%s requests = %d, errors = %v", action, count, problems)
		}
		mu.Unlock()
	}
	click(112)
	a.WaitUntil(10*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(problems) != 0
	})
	mu.Lock()
	defer mu.Unlock()
	err, ok := problems[0].(*core.WebError)
	if len(problems) != 1 || !ok || err.URL != "https://denied.invalid/link" {
		t.Errorf("denied request reported %v", problems)
	}
	for _, r := range requests {
		switch r.URL {
		case "https://denied.invalid/link":
			t.Error("denied request reached OnOpenWindow")
		case "https://allowed.invalid/timer":
			if r.UserInitiated {
				t.Error("timer got a user gesture without a click")
			}
		}
	}
}

func TestWebWindowRequestsAreSuppressedWithoutCallback(t *testing.T) {
	checkWindowTarget(t)
	var mu sync.Mutex
	var messages []string
	a, click := startWindowPage(t, nil, core.WebView{
		OnMessage: func(message string) {
			mu.Lock()
			defer mu.Unlock()
			messages = append(messages, message)
		},
	})
	a.WaitUntil(10*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(messages) > 0
	})
	click(48)
	a.WaitUntil(10*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(messages) > 1
	})
	mu.Lock()
	defer mu.Unlock()
	for _, expected := range []string{"timer:false", "click:false"} {
		if !slices.Contains(messages, expected) {
			t.Errorf("wanted %s, page reported %v", expected, messages)
		}
	}
}
