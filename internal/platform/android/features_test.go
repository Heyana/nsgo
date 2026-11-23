//go:build android

package android

func init() {
	var s *surface
	s.EnableShaders()
	s.EnableCanvas()
	s.EnableVideo()
	s.EnableWebView()
	s.EnableNative()
}
