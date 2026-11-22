package android

import "testing"

// Exercise the runtime's WebChromeClient with delayed system answers, including
// changes while the permission dialog is open. The JVM fixture supplies only
// Android's request and callback types; the capture policy is runtime source.
func TestWebMediaPermissionLifetime(t *testing.T) {
	runJavaAlgorithm(t, "webmedia", "WebMediaCheck", nil, "WebMedia")
}
