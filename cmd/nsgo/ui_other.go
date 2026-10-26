//go:build !windows

package main

// enableVT does nothing: terminals outside Windows interpret escape
// sequences themselves.
func enableVT() {}
