//go:build !js || nsgominiprogram || nsgoharmony

package vt

import "unsafe"

func checkManifest(m manifest) error { return m.check() }

func foreignBytes(p, n uintptr) []byte { return unsafe.Slice((*byte)(hostMemory(p)), n) }

func foreignValue[T any](p uintptr) T { return *(*T)(hostMemory(p)) }

func foreignSlice[T any](p, n uintptr) []T { return unsafe.Slice((*T)(hostMemory(p)), n) }

func foreignWrite[T any](p uintptr, value T) { *(*T)(hostMemory(p)) = value }

func withForeignBytes(p, n uintptr, write func([]byte)) { write(foreignBytes(p, n)) }

func foreignCString(p uintptr) string {
	n := uintptr(0)
	for *(*byte)(unsafe.Add(hostMemory(p), n)) != 0 {
		n++
	}
	return goString(p, n)
}
