//go:build linux && !android

package linux

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

func TestIsolatedWebInput(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if os.Getenv("NSGO_TEST_WEBKIT_PROCESS") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_WEBKIT_PROCESS="+t.Name())
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		if strings.Contains(string(out), "--- SKIP") {
			t.Skipf("%s", out)
		}
		return
	}
	messages := make(chan string, 16)
	scopes := make(chan *core.Scope, 1)
	var ref core.Ref
	w := startTestWindow(t, func(cx *core.Scope) core.Node {
		scopes <- cx
		return core.WebView{
			Ref: &ref, Style: core.Style{Width: core.Pt(200), Height: core.Pt(100)},
			Source: core.WebHTML(`<!doctype html><style>
body { margin: 0 } button,input { display: block; box-sizing: border-box; width: 200px; height: 32px }
</style><button onclick="window.nsgo.postMessage('click:' + event.isTrusted)">Click</button>
<input onfocus="window.nsgo.postMessage('focus')"
 oninput="if(this.value.length===1||this.value.length===301)window.nsgo.postMessage('text:' + this.value + ':' + event.isTrusted)"
 onkeydown="if(event.key==='Enter')window.nsgo.postMessage('enter:' + event.isTrusted)">
<script>window.nsgo.postMessage('ready')</script>`, ""),
			OnMessage: func(message string) { messages <- message },
		}
	}, Options{Title: "nsgo-isolated-web-input", Width: 200, Height: 100})
	cx := <-scopes
	require := func(want string) {
		t.Helper()
		select {
		case got := <-messages:
			if got != want {
				t.Fatalf("page reported %q, want %q", got, want)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("page did not report %q", want)
		}
	}
	require("ready")
	w.onLoop(t, func() {
		if !w.canSendWebInput() {
			t.Skip("trusted web input needs X11 and XTest")
		}
		// Enable exactly the isolation nsgotest uses, after the ordinary native fixture has initialized its engine and page.
		w.opts.test = &testOpen{}
	})
	t.Cleanup(func() { w.onLoop(t, func() { w.opts.test = nil }) })
	cx.Post(func() { ref.SendInput(core.WebClick(12, 16)) })
	require("click:true")
	w.onLoop(t, func() {
		display, xid, ok := w.x11Window()
		if !ok {
			t.Fatal("no X11 test surface")
		}
		// An unrecorded native click uses the same XTest source but has no matching request serial. It must stay outside the page.
		xWebInput.warpPointer(display, 0, xid, 0, 0, 0, 0, 12, 16)
		xWebInput.fakeButton(display, 1, 1, 0)
		xWebInput.fakeButton(display, 1, 0, 0)
		xWebInput.sync(display, 0)
	})
	select {
	case got := <-messages:
		t.Fatalf("isolated page accepted unrecorded input: %q", got)
	case <-time.After(300 * time.Millisecond):
	}
	cx.Post(func() { ref.SendInput(core.WebClick(12, 48)) })
	require("focus")
	cx.Post(func() { ref.SendInput(core.WebText("a")) })
	require("text:a:true")
	// A queued text command must retain every event until GTK dispatches it.
	text := strings.Repeat("b", 300)
	cx.Post(func() { ref.SendInput(core.WebText(text)) })
	require("text:a" + text + ":true")
	cx.Post(func() { ref.SendInput(core.WebKey("Enter")) })
	require("enter:true")
}
