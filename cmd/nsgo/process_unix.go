//go:build !windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// processAlive reports whether a process runs as pid. One the caller may not signal, another user's, runs too: sessionLive's token check then decides whether it is the session's.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	defer p.Release()
	err = p.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

func prepareChildProcess(*exec.Cmd) {}

// stopChildProcess asks cmd to exit cleanly with SIGINT, killing it after 5 seconds.
func stopChildProcess(cmd *exec.Cmd, done <-chan struct{}) {
	cmd.Process.Signal(os.Interrupt)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		<-done
	}
}
