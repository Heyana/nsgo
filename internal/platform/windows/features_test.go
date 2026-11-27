//go:build windows

package windows

func init() {
	var w *window
	w.EnableShaders()
	w.EnableCanvas()
	w.EnableVideo()
	w.EnableWebView()
	w.EnableNative()
	w.EnableGPU()
}
