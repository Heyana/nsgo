//go:build darwin && !ios

package macos

import (
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

// panelWindow returns the surface's window while a sheet of it can be seen. Occlusion by another window alone does not detach a sheet.
func (s *surface) panelWindow() objc.ID {
	w := s.window
	if s.closed || w == 0 || s.kind == driver.SurfaceTrayIcon ||
		!objc.Send[bool](w, sel("isVisible")) || objc.Send[bool](w, sel("isMiniaturized")) ||
		objc.Send[bool](nsApp(), sel("isHidden")) {
		return 0
	}
	f := objc.Send[apple.CGRect](w, selFrame)
	for _, screen := range screenFrames("frame") {
		if min(f.Origin.X+f.Size.Width, screen.Origin.X+screen.Size.Width) > max(f.Origin.X, screen.Origin.X) &&
			min(f.Origin.Y+f.Size.Height, screen.Origin.Y+screen.Size.Height) > max(f.Origin.Y, screen.Origin.Y) {
			return w
		}
	}
	return 0
}

// beginFilePanel presents a picker or save dialog without tying it to a window the user cannot see. Both AppKit APIs copy the completion block.
func (s *surface) beginFilePanel(panel objc.ID, answer func(int)) {
	if s.filePanels == nil {
		s.filePanels = map[objc.ID]struct{}{}
	}
	s.filePanels[panel] = struct{}{}
	complete := func(response int) {
		delete(s.filePanels, panel)
		answer(response)
	}
	if s.closed {
		complete(0)
		return
	}
	done := objcblock.New(func(_ objc.Block, response int) { complete(response) })
	defer done.Release()
	if window := s.panelWindow(); window != 0 {
		panel.Send(sel("beginSheetModalForWindow:completionHandler:"), window, done)
	} else {
		activateApp()
		panel.Send(sel("beginWithCompletionHandler:"), done)
	}
}

// A standalone NSAlert owns a nested modal loop. Queue subsequent alerts until that loop returns, so another surface cannot nest its modal alert.
var standaloneAlerts struct {
	running bool
	pending []func()
}

func queueStandaloneAlert(show func()) {
	standaloneAlerts.pending = append(standaloneAlerts.pending, show)
	if standaloneAlerts.running {
		return
	}
	standaloneAlerts.running = true
	// A main-dispatch-queue block cannot drain that same serial queue in runModal. The run loop's own block leaves engine mounts and test-host calls free to execute while the alert waits for its answer.
	block := objcblock.New(func(_ objc.Block) {
		defer func() { standaloneAlerts.running = false }()
		for len(standaloneAlerts.pending) > 0 {
			show := standaloneAlerts.pending[0]
			standaloneAlerts.pending[0] = nil
			standaloneAlerts.pending = standaloneAlerts.pending[1:]
			show()
		}
	})
	modes := objc.ID(apple.Class("NSMutableArray")).Send(sel("array"))
	modes.Send(sel("addObject:"), apple.NSString("kCFRunLoopDefaultMode"))
	modes.Send(sel("addObject:"), apple.Global(appKitPath, "NSModalPanelRunLoopMode"))
	objc.ID(apple.Class("NSRunLoop")).Send(sel("mainRunLoop")).Send(sel("performInModes:block:"), modes, block)
	block.Release()
}

// closeDialogs cancels only this surface's dialogs, including a standalone alert's modal loop; queued alerts observe closed before showing.
func (s *surface) closeDialogs() {
	for panel := range s.filePanels {
		panel.Send(sel("cancel:"), objc.ID(0))
	}
	for _, shown := range s.alerts {
		window := shown.alert.Send(selWindow)
		if parent := window.Send(sel("sheetParent")); parent != 0 {
			parent.Send(sel("endSheet:returnCode:"), window, 0)
		} else if nsApp().Send(sel("modalWindow")) == window {
			nsApp().Send(sel("abortModal"))
			window.Send(sel("orderOut:"), objc.ID(0))
		}
	}
}
