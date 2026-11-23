package driver

// Feature enablers let each backend link its shader, canvas, video, web view
// and native-component code only into apps that construct one
// (docs/architecture/layers.md, What an app links).
//
// Core's element mount methods type-assert the tree's TextEngine (unwrapping
// wrappers via PlatformUnwrapper) to the matching interface and call its
// method before the batch commits. When an app never constructs that element,
// no reachable call site mentions the interface method, so Go's linker drops
// the backend's implementation and everything reachable only through it.

// PlatformUnwrapper returns the wrapped backend platform so core can reach
// its optional feature enablers without the wrapper referencing them.
type PlatformUnwrapper interface {
	UnwrapPlatform() any
}

// ShaderEnabler is implemented by backends whose shader handling links on demand.
type ShaderEnabler interface {
	EnableShaders()
}

// CanvasEnabler is implemented by backends whose canvas drawing links on demand.
type CanvasEnabler interface {
	EnableCanvas()
}

// VideoEnabler is implemented by backends whose video player links on demand.
type VideoEnabler interface {
	EnableVideo()
}

// WebViewEnabler is implemented by backends whose web view links on demand.
type WebViewEnabler interface {
	EnableWebView()
}

// NativeEnabler is implemented by backends whose native-component bridge links on demand.
type NativeEnabler interface {
	EnableNative()
}
