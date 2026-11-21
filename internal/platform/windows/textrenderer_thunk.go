//go:build windows && (amd64 || arm64)

package windows

import "syscall"

// The DrawGlyphRun thunk's target, read by textrenderer_*.s, and its address.
var (
	textDrawGlyphRunTarget    = syscall.NewCallback(textDrawGlyphRunWords)
	textDrawGlyphRunThunkAddr uintptr
)

func textDrawGlyphRunSlot() uintptr { return textDrawGlyphRunThunkAddr }
