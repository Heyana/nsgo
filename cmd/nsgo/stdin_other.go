//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !windows

package main

// stdinIsTerminal reports no terminal where the tool asks the system nothing about one.
func stdinIsTerminal() bool { return false }
