// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2024 The Ebitengine Authors
//go:build android && arm64

package purego

// Source for constants: https://android.googlesource.com/platform/bionic/+/refs/heads/main/libc/include/dlfcn.h

const (
	is64bit      = 1 << (^uintptr(0) >> 63) / 2
	is32bit      = 1 - is64bit
	RTLD_DEFAULT = is32bit * 0xffffffff
	RTLD_LAZY    = 0x00000001
	RTLD_NOW     = is64bit * 0x00000002
	RTLD_LOCAL   = 0x00000000
	RTLD_GLOBAL  = is64bit*0x00100 | is32bit*0x00000002
)

func Dlopen(path string, mode int) (uintptr, error) {
	return dlopenPath(path, mode)
}

func Dlsym(handle uintptr, name string) (uintptr, error) {
	return dlsymName(handle, name)
}

func Dlclose(handle uintptr) error {
	return dlcloseHandle(handle)
}

func loadSymbol(handle uintptr, name string) (uintptr, error) {
	return Dlsym(handle, name)
}
