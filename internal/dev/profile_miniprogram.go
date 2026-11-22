//go:build nsgodev && nsgominiprogram

package dev

// devProfile is unused in mini programs, which leave runtime/pprof and runtime/trace out to fit the 2 MB package cap.
var devProfile = ""

func receiveRuntimeProfile(string) {}
