//go:build !windows

package main

import (
	"os"
	"os/exec"
)

func setupTUIProcess(*exec.Cmd)          {}
func stopTUIProcess(cmd *exec.Cmd) error { return cmd.Process.Signal(os.Interrupt) }
