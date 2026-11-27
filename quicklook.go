package nsgo

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/backend"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

// QuickLookPreview builds the root element previewing the file at path in a
// macOS QuickLook Preview Extension.
type QuickLookPreview func(cx *core.Scope, path string) core.Node

// RunQuickLook runs a macOS QuickLook Preview Extension
// (com.apple.quicklook.preview), registering an NSViewController conforming to
// QLPreviewingController that mounts an embedded NSGo view for each previewed
// file and handing control to NSExtensionMain.
//
// preview receives the previewed file's path; the view's host channel also
// provides "path" and "url" in core.Scope.HostParams. WindowSize sets the
// controller's preferred content size (640 by 480 by default). Component,
// Fonts, Appearance, DataDir and OnError apply.
//
// On platforms other than macOS, RunQuickLook returns immediately.
func RunQuickLook(preview QuickLookPreview, opts ...Option) {
	o := options{platform: backend.Options{Width: 640, Height: 480, Appearance: startAppearance()}}
	for _, fn := range opts {
		fn(&o)
	}
	h := engine.HostOptions{OnError: o.onError, DataDir: o.dataDir}
	backend.RunQuickLook(preview, o.platform, h)
}
