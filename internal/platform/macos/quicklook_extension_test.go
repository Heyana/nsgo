//go:build darwin && !ios

package macos

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

func TestQuickLookControllerPreparesAndCleansUpView(t *testing.T) {
	sample := filepath.Join(t.TempDir(), "scene.scenra")
	if err := os.WriteFile(sample, []byte("hello quicklook"), 0o644); err != nil {
		t.Fatal(err)
	}
	onMain(func() {
		var gotPath, gotURL string
		cls := registerQuickLookController(func(path, url string) (engine.App, engine.HostOptions) {
			gotPath, gotURL = path, url
			host := core.NewHost()
			host.SetParams(map[string]any{"path": path, "url": url})
			return func(cx *core.Scope) core.Node {
				return core.Text{Content: cx.HostParams()["path"].(string)}
			}, engine.HostOptions{Channel: host, Settings: &engine.HostSettings{}}
		}, Options{Width: 720, Height: 480})
		if cls == 0 {
			t.Fatal("registerQuickLookController returned 0")
		}
		if proto := objc.GetProtocol("QLPreviewingController"); proto != nil {
			if !objc.Send[bool](objc.ID(cls), sel("conformsToProtocol:"), proto) {
				t.Error("NSGoQuickLookController does not conform to QLPreviewingController")
			}
		}

		pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
		defer pool.Send(sel("drain"))

		ctrl := objc.ID(cls).Send(selAlloc).Send(sel("init"))
		view := ctrl.Send(sel("view"))
		if view == 0 {
			t.Fatal("controller.view is nil after loadView")
		}
		size := objc.Send[apple.CGSize](ctrl, sel("preferredContentSize"))
		if size.Width != 720 || size.Height != 480 {
			t.Errorf("preferredContentSize = %v, want 720x480", size)
		}

		var completed bool
		var gotErr objc.ID
		done := objcblock.New(func(_ objc.Block, nsErr objc.ID) {
			completed = true
			gotErr = nsErr
		})
		defer done.Release()

		nsURL := objc.ID(apple.Class("NSURL")).Send(sel("fileURLWithPath:"), apple.NSString(sample))
		ctrl.Send(sel("preparePreviewOfFileAtURL:completionHandler:"), nsURL, done)

		if !completed {
			t.Fatal("completionHandler was not called")
		}
		if gotErr != 0 {
			t.Fatalf("completionHandler received unexpected NSError: %s", apple.GoString(gotErr.Send(sel("localizedDescription"))))
		}
		if gotPath != sample {
			t.Errorf("path = %q, want %q", gotPath, sample)
		}
		if gotURL == "" {
			t.Error("url was empty")
		}
		if count := uint(view.Send(sel("subviews")).Send(sel("count"))); count != 1 {
			t.Errorf("controller.view subviews count = %d, want 1", count)
		}
		if quickLookControllers[ctrl] == nil {
			t.Error("controller entry not tracked")
		}

		// Calling preparePreviewOfFileAtURL with nil URL reports an error and cleans up the previous view.
		completed, gotErr = false, 0
		ctrl.Send(sel("preparePreviewOfFileAtURL:completionHandler:"), objc.ID(0), done)
		if !completed || gotErr == 0 {
			t.Errorf("nil URL: completed=%v, gotErr=%v; want error", completed, gotErr)
		}
		if count := uint(view.Send(sel("subviews")).Send(sel("count"))); count != 0 {
			t.Errorf("controller.view subviews after nil URL = %d, want 0", count)
		}

		ctrl.Send(selRelease)
		if quickLookControllers[ctrl] != nil {
			t.Error("controller entry leaked after release")
		}
	})
}
