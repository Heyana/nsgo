//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// enableVT asks the console to interpret escape sequences, which cmd.exe
// does not do on its own: without it, colors and the spinner's line
// clearing would print as text.
func enableVT() {
	const enableVirtualTerminalProcessing = 0x0004
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")
	var mode uint32
	if r, _, _ := getConsoleMode.Call(uintptr(syscall.Stderr), uintptr(unsafe.Pointer(&mode))); r == 0 {
		return
	}
	setConsoleMode.Call(uintptr(syscall.Stderr), uintptr(mode|enableVirtualTerminalProcessing))
}
