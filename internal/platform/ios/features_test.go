//go:build ios

package ios

func init() {
	var s *surface
	s.EnableShaders()
	s.EnableCanvas()
	s.EnableVideo()
	s.EnableWebView()
	s.EnableNative()
	s.EnableGPU()
}
