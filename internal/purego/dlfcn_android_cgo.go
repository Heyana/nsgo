//go:build android && cgo

package purego

import "github.com/nsgo-dev/nsgo/internal/purego/internal/cgo"

func dlopenPath(path string, mode int) (uintptr, error)      { return cgo.Dlopen(path, mode) }
func dlsymName(handle uintptr, name string) (uintptr, error) { return cgo.Dlsym(handle, name) }
func dlcloseHandle(handle uintptr) error                     { return cgo.Dlclose(handle) }
