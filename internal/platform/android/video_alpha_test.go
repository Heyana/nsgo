package android

import "testing"

// VideoNode extends TextureView, whose Android default is opaque=true. Verify
// that constructing and configuring VideoNode sets opaque=false so alpha from
// transparent decoders can composite over views behind the video.
func TestVideoNodeIsNotOpaque(t *testing.T) {
	runJavaAlgorithm(t, "videoalpha", "VideoAlphaCheck", nil, "Protocol", "VideoNode")
}
