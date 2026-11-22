//go:build ios && !tvos && nsgotest

package ios

import (
	"os"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/mobile"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestSystemFullAppLinkDelivery is the separate sender used by testdata/openedlinks/verify.py. UIKit performs real Launch Services delivery, including a cold launch; no receiver delegate is injected.
func TestSystemFullAppLinkDelivery(t *testing.T) {
	link := os.Getenv("NSGO_IOS_LINK1474_URL")
	if link == "" {
		t.Skip("needs the owned full-app receiver and NSGO_IOS_LINK1474_URL")
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.Text{Content: "Send an invocation link"}
	})
	app.WaitFor("text:Send an invocation link", 5*time.Second)
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		active := false
		onMainSync(func() {
			application := objc.ID(apple.Class("UIApplication")).Send(sel("sharedApplication"))
			active = objc.Send[int](application, sel("applicationState")) == 0 && objc.Send[bool](runner.window, sel("isKeyWindow"))
		})
		if active {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the link sender did not become active")
		}
	}
	opened := make(chan bool, 1)
	// The test host records core OpenURL commands in its model. Call the native sender here so Launch Services really routes this request.
	onMainSync(func() { openSystemURL(link, func(ok bool) { opened <- ok }) })
	select {
	case ok := <-opened:
		if !ok {
			t.Fatal("UIKit did not open the receiver's link")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("UIKit did not answer the link request")
	}
}

func TestFullAppInvocationURLContexts(t *testing.T) {
	const cold = "nsgo1474://order?table=12"
	const warm = "nsgo1474://order?table=21#detail"
	var before string
	var pending []string
	onMainSync(func() {
		before, pending = openedURL, launchLinks
		openedURL, launchLinks = "", nil
		url := objc.ID(apple.Class("NSURL")).Send(sel("URLWithString:"), apple.NSString(cold))
		contexts, release := fileContextsForTest([]objc.ID{url})
		defer release()
		options := objc.ID(fileTestOptionsClass).Send(selNew)
		defer options.Send(selRelease)
		fileTestContexts[options] = contexts
		defer delete(fileTestContexts, options)
		takeConnectionLinks(options)
	})
	defer onMainSync(func() { openedURL, launchLinks = before, pending })
	type delivery struct{ url, current string }
	delivered := make(chan delivery, 4)
	var initial string
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		initial = mobile.OpenedURL(cx)
		mobile.OnOpenLinks(cx, func(links []string) {
			for _, link := range links {
				delivered <- delivery{link, mobile.OpenedURL(cx)}
			}
		})
		return core.Text{TestID: "invocation", ContentFunc: func() string { return mobile.OpenedURL(cx) }}
	})
	if initial != cold {
		t.Fatalf("initial full-app URL=%q", initial)
	}
	for _, link := range []string{cold, warm} {
		if link == warm {
			onMainSync(func() {
				url := objc.ID(apple.Class("NSURL")).Send(sel("URLWithString:"), apple.NSString(warm))
				contexts, release := fileContextsForTest([]objc.ID{url})
				defer release()
				scene := runner.activeWindowScene()
				scene.Send(sel("delegate")).Send(sel("scene:openURLContexts:"), scene, contexts)
			})
		}
		select {
		case got := <-delivered:
			if got.url != link || got.current != link {
				t.Fatalf("link/env delivery=%+v, want %s", got, link)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("ordinary OnOpenLinks delivery was lost")
		}
		app.WaitUntil(5*time.Second, func() bool { return app.Find("id:invocation").Text == link })
	}
	const universal = "https://example.test/order?table=31"
	onMainSync(func() {
		activity := objc.ID(apple.Class("NSUserActivity")).Send(selAlloc).Send(sel("initWithActivityType:"), apple.NSString("NSUserActivityTypeBrowsingWeb"))
		defer activity.Send(selRelease)
		url := objc.ID(apple.Class("NSURL")).Send(sel("URLWithString:"), apple.NSString(universal))
		activity.Send(sel("setWebpageURL:"), url)
		scene := runner.activeWindowScene()
		scene.Send(sel("delegate")).Send(sel("scene:continueUserActivity:"), scene, activity)
	})
	app.WaitUntil(5*time.Second, func() bool { return app.Find("id:invocation").Text == universal })
}
