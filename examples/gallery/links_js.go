//go:build js && wasm

package main

import "syscall/js"

// openURL opens url in a new browser tab. It reports whether the platform
// opened it.
func openURL(url string) bool {
	js.Global().Call("open", url, "_blank", "noopener,noreferrer")
	return true
}
