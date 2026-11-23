//go:build linux && !android

package linux

func init() {
	var w *window
	w.EnableShaders()
	w.EnableCanvas()
	w.EnableVideo()
	w.EnableWebView()
	w.EnableNative()
}
