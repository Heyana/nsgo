package android

import (
	"io"
	"os"
	"syscall"
)

const hostRuntime = false

func runtimeStreams() (io.Reader, io.WriteCloser) {
	fd, err := syscall.Dup(1)
	if err != nil {
		panic(err)
	}
	if err := syscall.Dup3(2, 1, 0); err != nil {
		panic(err)
	}
	return os.Stdin, os.NewFile(uintptr(fd), "nsgo-runtime")
}
