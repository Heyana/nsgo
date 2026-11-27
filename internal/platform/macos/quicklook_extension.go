//go:build darwin && !ios

package macos

import (
	"os"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// QuickLookExtensionPrincipalClass is the NSExtensionPrincipalClass declared
// in QuickLookPreview.appex's Info.plist and registered at run time by
// RunQuickLook.
const QuickLookExtensionPrincipalClass = "NSGoQuickLookController"

type quickLookAppFactory func(path, url string) (engine.App, engine.HostOptions)

type quickLookEntry struct {
	view      *View
	scopedURL objc.ID
}

var (
	quickLookMakeApp     quickLookAppFactory
	quickLookOptions     Options
	quickLookControllers = map[objc.ID]*quickLookEntry{}
)

// RunQuickLook registers NSGoQuickLookController (an NSViewController
// conforming to QLPreviewingController) and hands the process to Foundation's
// NSExtensionMain. Each previewed file mounts an embedded NSGo View inside the
// controller's root view.
func RunQuickLook(makeApp func(path, url string) (engine.App, engine.HostOptions), opts Options) {
	lib, err := purego.Dlopen("/System/Library/Frameworks/Foundation.framework/Foundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		panic(err)
	}
	registerQuickLookController(makeApp, opts)
	var extensionMain func(int, unsafe.Pointer) int
	purego.RegisterLibFunc(&extensionMain, lib, "NSExtensionMain")
	argv := make([]*byte, 0, len(os.Args)+1)
	for _, arg := range os.Args {
		b := append([]byte(arg), 0)
		argv = append(argv, &b[0])
	}
	argv = append(argv, nil)
	code := extensionMain(len(os.Args), unsafe.Pointer(&argv[0]))
	runtime.KeepAlive(argv)
	os.Exit(code)
}

// registerQuickLookController loads AppKit and QuickLookUI, stores the active
// preview factory and options, and registers NSGoQuickLookController.
func registerQuickLookController(makeApp quickLookAppFactory, opts Options) objc.Class {
	loadAppKit()
	_, _ = purego.Dlopen("/System/Library/Frameworks/Quartz.framework/Frameworks/QuickLookUI.framework/QuickLookUI", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	quickLookMakeApp = makeApp
	quickLookOptions = opts

	var protocols []*objc.Protocol
	if proto := objc.GetProtocol("QLPreviewingController"); proto != nil {
		protocols = append(protocols, proto)
	}
	return register(QuickLookExtensionPrincipalClass, apple.Class("NSViewController"), protocols, []objc.MethodDef{
		{
			Cmd: sel("loadView"),
			Fn:  quickLookLoadView,
		},
		{
			Cmd: sel("preparePreviewOfFileAtURL:completionHandler:"),
			Fn:  quickLookPreparePreviewOfFileAtURL,
		},
		{
			Cmd: sel("dealloc"),
			Fn:  quickLookDealloc,
		},
	})
}

func quickLookLoadView(self objc.ID, _ objc.SEL) {
	w, h := float64(quickLookOptions.Width), float64(quickLookOptions.Height)
	if w <= 0 {
		w = 640
	}
	if h <= 0 {
		h = 480
	}
	container := objc.ID(classView).Send(selAlloc).Send(sel("initWithFrame:"), apple.Rect(0, 0, w, h))
	self.Send(sel("setView:"), container)
	container.Send(selRelease)
	self.Send(sel("setPreferredContentSize:"), apple.CGSize{Width: w, Height: h})
}

func quickLookPreparePreviewOfFileAtURL(self objc.ID, _ objc.SEL, url, handler objc.ID) {
	closeQuickLookEntry(self)
	if url == 0 || quickLookMakeApp == nil {
		apple.CallBlock(handler, uintptr(quickLookNSError("QuickLook preview has no file URL")))
		return
	}
	scoped := objc.Send[bool](url, sel("startAccessingSecurityScopedResource"))
	path := apple.GoString(url.Send(sel("path")))
	fileURL := apple.GoString(url.Send(sel("absoluteString")))
	if path == "" {
		if scoped {
			url.Send(sel("stopAccessingSecurityScopedResource"))
		}
		apple.CallBlock(handler, uintptr(quickLookNSError("QuickLook preview URL has no file path")))
		return
	}
	app, hostOpts := quickLookMakeApp(path, fileURL)
	viewOpts := quickLookOptions
	viewOpts.Host = hostOpts
	v := NewView(app, viewOpts)

	container := self.Send(sel("view"))
	bounds := objc.Send[apple.CGRect](container, selBounds)
	native := objc.ID(v.Handle())
	const (
		nsViewWidthSizable  = 2
		nsViewHeightSizable = 16
	)
	native.Send(selSetFrame, bounds)
	native.Send(selSetAutoresizingMask, uint(nsViewWidthSizable|nsViewHeightSizable))
	container.Send(selAddSubview, native)

	entry := &quickLookEntry{view: v}
	if scoped {
		url.Send(sel("retain"))
		entry.scopedURL = url
	}
	quickLookControllers[self] = entry
	apple.CallBlock(handler, 0)
}

func quickLookDealloc(self objc.ID, cmd objc.SEL) {
	closeQuickLookEntry(self)
	apple.Super(self, cmd)
}

func closeQuickLookEntry(self objc.ID) {
	entry := quickLookControllers[self]
	if entry == nil {
		return
	}
	delete(quickLookControllers, self)
	if entry.view != nil {
		entry.view.Close()
	}
	if entry.scopedURL != 0 {
		entry.scopedURL.Send(sel("stopAccessingSecurityScopedResource"))
		entry.scopedURL.Send(selRelease)
	}
}

func quickLookNSError(msg string) objc.ID {
	info := objc.ID(apple.Class("NSDictionary")).Send(
		sel("dictionaryWithObject:forKey:"),
		apple.NSString(msg),
		apple.NSString("NSLocalizedDescription"),
	)
	return objc.ID(apple.Class("NSError")).Send(
		sel("errorWithDomain:code:userInfo:"),
		apple.NSString("NSCocoaErrorDomain"),
		int(256),
		info,
	)
}
