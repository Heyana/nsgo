package main

import (
	"fmt"
	"os/exec"
	"syscall"
)

func setupTUIProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
func stopTUIProcess(cmd *exec.Cmd) error {
	// A console break reaches this child's process group, so its signal
	// handler restores terminal modes before exiting. A GUI taskkill cannot.
	generate := syscall.NewLazyDLL("kernel32.dll").NewProc("GenerateConsoleCtrlEvent")
	if ok, _, err := generate.Call(1, uintptr(cmd.Process.Pid)); ok == 0 {
		return fmt.Errorf("terminal console break: %w", err)
	}
	return nil
}
