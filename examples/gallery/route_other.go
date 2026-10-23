//go:build !js || !wasm

package main

import "github.com/oneo-me/nsgo/core"

// routeName is empty where the app has no address bar.
func routeName() string { return "" }

// setRoute does nothing where the app has no address bar.
func setRoute(string, bool) {}

// listenRoute does nothing where the app has no address bar.
func listenRoute(*core.Scope, func(string)) {}
