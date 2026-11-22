//go:build darwin && !ios

package apple

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/videoalpha/clips"
)

func TestMacPictureInPictureRetainsRetiringSourceView(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first.mov"), filepath.Join(dir, "second.mov")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, clips.PiP, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var controllerClass objc.Class
	onMain(func() {
		loadAVKit()
		loadAVFoundation()
		registerPiPDelegate()
		controllerClass = RegisterClass("NsgoTestMacRetiringPiPController", Class("NSObject"), nil, []objc.MethodDef{
			{Cmd: sel("stopPictureInPicture"), Fn: func(objc.ID, objc.SEL) {}},
			{Cmd: sel("setDelegate:"), Fn: func(objc.ID, objc.SEL, objc.ID) {}},
			{Cmd: sel("isPictureInPicturePossible"), Fn: func(objc.ID, objc.SEL) bool { return false }},
		})
	})
	for _, change := range []string{"controls", "source"} {
		t.Run(change, func(t *testing.T) {
			onMain(func() {
				holder := objc.ID(Class("NSView")).Send(sel("alloc")).Send(sel("initWithFrame:"), Rect(0, 0, 320, 180))
				defer holder.Send(sel("release"))
				v := NewVideo(holder)
				v.Dispatch = func(driver.Event) {}
				defer v.Close()
				config := driver.VideoConfig{Source: first, Controls: true, Muted: true, Paused: true, Rate: 1, ProgressInterval: time.Second}
				v.Configure(&config)
				old := v.shown.Send(sel("retain"))
				defer old.Send(sel("release"))
				p := &PictureInPicture{controller: objc.ID(controllerClass).Send(sel("new")), state: driver.PictureInPictureState{Supported: true, Active: true}}
				p.delegate = objc.ID(pipDelegateClass).Send(sel("new"))
				pipDelegates.Store(p.delegate, p)
				v.pip = p
				if change == "controls" {
					config.Controls = false
				} else {
					config.Source = second
				}
				v.Configure(&config)
				if !p.closing || p.controller == 0 || old.Send(sel("superview")) != holder {
					t.Error("retiring PiP source left its inline view before AVKit stopped")
				}
				if v.shown == old || v.shown == 0 {
					t.Error("new source reused the retiring native view")
				}
				p.delegate.Send(sel("pictureInPictureControllerDidStopPictureInPicture:"), p.controller)
				if p.controller != 0 || old.Send(sel("superview")) != 0 {
					t.Error("stopped PiP retained its retired native view")
				}
			})
		})
	}
}
