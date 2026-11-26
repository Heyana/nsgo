package main

import "syscall"

// stdinIsTerminal reports whether standard input is a console, which has a console mode, unlike a file, a pipe or NUL.
func stdinIsTerminal() bool {
	var mode uint32
	return syscall.GetConsoleMode(syscall.Stdin, &mode) == nil
}
