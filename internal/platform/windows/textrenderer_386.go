//go:build windows && 386

package windows

import "syscall"

// On 386 every stdcall parameter, float32 included, is one word on the stack.
func textDrawGlyphRunSlot() uintptr {
	return syscall.NewCallback(textDrawGlyphRunWords)
}
