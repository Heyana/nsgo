//go:build android || nsgoandroidhost

package android

func init() {
	var s *surface
	s.EnableShaders()
	s.EnableCanvas()
	s.EnableVideo()
	s.EnableWebView()
	s.EnableNative()
}
