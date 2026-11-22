//go:build !nsgo_steam

package updater_test

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/updater"
)

func TestLinuxUpdateWaitsForQuitRequest(t *testing.T) {
	for _, decision := range []string{"cancel", "defer-cancel", "defer-approve"} {
		t.Run(decision, func(t *testing.T) {
			fakeDownload(t)
			var requests int
			var answer func(bool)
			allowUserQuit := false
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				cx.OnQuitRequest(func(q *core.QuitRequest) {
					requests++
					if allowUserQuit {
						return
					}
					if decision == "cancel" {
						q.Cancel()
					} else {
						answer = q.Defer()
					}
				})
				return base.Button{OnPress: func(core.PressEvent) { updater.Check(cx) },
					Children: []core.Node{core.Text{Content: "Check"}}}
			}, nsgotest.Environment(core.Environment{OS: core.OSLinux}))
			app.Update = func(bool) core.UpdateCheck { return signedNewer }
			app.Press("text:Check")
			app.Answer(0)
			app.WaitUntil(5*time.Second, func() bool { return requests != 0 || app.Restarts != 0 })
			if requests != 1 || app.Restarts != 0 || app.Quits != 0 {
				t.Fatalf("quit requests=%d, restarts=%d, quits=%d before approval", requests, app.Restarts, app.Quits)
			}
			if decision == "defer-approve" {
				answer(true)
				app.WaitUntil(time.Second, func() bool { return app.Restarts == 1 })
				if app.Quits != 1 {
					t.Fatalf("approved restart quit %d times", app.Quits)
				}
				return
			}
			if answer != nil {
				answer(false)
				app.Settle()
			}
			allowUserQuit = true
			app.RequestQuit(false)
			if requests != 2 || app.Quits != 1 || app.Restarts != 0 {
				t.Fatalf("later user quit: requests=%d, quits=%d, restarts=%d", requests, app.Quits, app.Restarts)
			}
		})
	}
}
