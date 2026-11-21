//go:build ios && nsgotest

package ios

import (
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/core"
)

// SystemBarsForTest reads UIKit's status bar, home indicator preference and safe area.
func SystemBarsForTest() (statusHidden, homeAutoHidden bool, safe core.Edges) {
	onMainSync(func() {
		manager := runner.window.Send(sel("windowScene")).Send(sel("statusBarManager"))
		statusHidden = objc.Send[bool](manager, sel("isStatusBarHidden"))
		homeAutoHidden = objc.Send[bool](runner.controller, sel("prefersHomeIndicatorAutoHidden"))
		safe = runner.environment().SafeArea
	})
	return
}
