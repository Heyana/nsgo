//go:build !js || !wasm

package main

// openURL leaves the page to the app on platforms without a browser, which
// copies the address instead.
func openURL(string) bool { return false }
