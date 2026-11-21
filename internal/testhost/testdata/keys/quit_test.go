package keys_test

import (
	"sync/atomic"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestRequestQuitDistinguishesSystemAndUser(t *testing.T) {
	if target := nsgotest.Target(); target != "" && target != "macos" && target != "windows" && target != "linux" {
		t.Skip("quit requests run on desktop targets")
	}
	var forced, canceled atomic.Bool
	app := nsgotest.StartApp(t, func(cx *core.Scope) {
		cx.SetMainView(func(*core.Scope) core.Node { return core.Text{Content: "Quit requests"} })
		desktop.OnQuitRequest(cx, func(q *desktop.QuitRequest) {
			q.Cancel()
			q.Defer()(false)
			forced.Store(q.Forced())
			canceled.Store(q.Canceled())
		})
	}, nsgotest.Environment(core.Environment{OS: core.OSMacOS}))
	app.RequestQuit(false)
	if forced.Load() || !canceled.Load() || app.Quits != 0 {
		t.Fatalf("user quit: forced=%v canceled=%v quits=%d", forced.Load(), canceled.Load(), app.Quits)
	}
	app.RequestQuit(true)
	if !forced.Load() || canceled.Load() || app.Quits != 1 {
		t.Fatalf("session quit: forced=%v canceled=%v quits=%d", forced.Load(), canceled.Load(), app.Quits)
	}
}

// The quit shortcut reaches the platform's menu handler. A canceled or
// pending request leaves the app running, and an affirmative answer quits.
func TestDesktopQuitMenuDefers(t *testing.T) {
	target := nsgotest.Target()
	if target != "" && target != "linux" && target != "windows" {
		t.Skip("tests the Windows and Linux Quit menu shortcut")
	}
	var requests atomic.Int32
	answers := make(chan func(bool), 1)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		desktop.SetApplicationMenu(cx, []desktop.MenuItem{
			{Label: "File", Items: []desktop.MenuItem{{Role: desktop.MenuQuit, Shortcut: "ctrl+shift+q"}}},
		})
		desktop.OnQuitRequest(cx, func(q *desktop.QuitRequest) {
			if requests.Add(1) == 1 {
				q.Cancel()
				return
			}
			answers <- q.Defer()
		})
		return core.Text{Content: "Quit menu", TestID: "quit-menu"}
	}, nsgotest.Environment(core.Environment{OS: core.OSWindows}))
	app.Key("ctrl+shift+q")
	if requests.Load() != 1 || app.Quits != 0 {
		t.Fatalf("cancel: asked %d times, quit %d times", requests.Load(), app.Quits)
	}
	app.Key("ctrl+shift+q")
	app.Key("ctrl+shift+q")
	if requests.Load() != 2 || app.Quits != 0 {
		t.Fatalf("defer: asked %d times, quit %d times", requests.Load(), app.Quits)
	}
	(<-answers)(true)
	app.Settle()
	app.Find("id:quit-menu")
	if app.Quits != 1 {
		t.Fatalf("approve: quit %d times", app.Quits)
	}
}
