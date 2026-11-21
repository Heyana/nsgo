package main

import (
	"errors"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

var (
	procKernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procGenerateConsoleCtrlEvent = procKernel32.NewProc("GenerateConsoleCtrlEvent")
)

// processAlive reports whether a process runs as pid. One the caller may not open, another user's or an elevated one's, runs too: sessionLive's token check then decides whether it is the session's.
func processAlive(pid int) bool {
	if pid <= 0 || uint64(pid) > uint64(^uint32(0)) {
		return false
	}
	h, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return errors.Is(err, syscall.ERROR_ACCESS_DENIED)
	}
	defer syscall.CloseHandle(h)
	// An exited process is signaled, even if its exit code is STILL_ACTIVE (259). A zero timeout checks its state without waiting for it to exit.
	state, err := syscall.WaitForSingleObject(h, 0)
	return err == nil && state == syscall.WAIT_TIMEOUT
}

// prepareChildProcess puts cmd in a process group of its own so Ctrl+Break reaches it without interrupting this process.
func prepareChildProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// stopChildProcess asks cmd to exit cleanly with Ctrl+Break, falling back to ending its process tree so neither nsgo dev nor the app it started stays behind.
func stopChildProcess(cmd *exec.Cmd, done <-chan struct{}) {
	const ctrlBreakEvent = 1
	pid := cmd.Process.Pid
	if r, _, _ := procGenerateConsoleCtrlEvent.Call(ctrlBreakEvent, uintptr(uint32(pid))); r != 0 {
		select {
		case <-done:
			return
		case <-time.After(5 * time.Second):
		}
	}
	// Without a shared console (for example under an MCP host started with no console) or past the timeout, kill the child and the app it started.
	exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cmd.Process.Kill()
		<-done
	}
}
