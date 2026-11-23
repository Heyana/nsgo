//go:build js && wasm && !nsgominiprogram

package web

func init() {
	var s *surface
	s.EnableShaders()
	s.EnableCanvas()
	s.EnableVideo()
	s.EnableWebView()
	s.EnableNative()
}
