//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// stdinIsTerminal reports whether standard input is a terminal, which answers a window size request, unlike a file, a pipe or /dev/null.
func stdinIsTerminal() bool {
	conn, err := os.Stdin.SyscallConn()
	if err != nil {
		return false
	}
	tty := false
	conn.Control(func(fd uintptr) {
		_, err := unix.IoctlGetWinsize(int(fd), unix.TIOCGWINSZ)
		tty = err == nil
	})
	return tty
}
