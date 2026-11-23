//go:build darwin

package apple

import (
	"sync"

	"github.com/ebitengine/purego"
)

const pixelBGRA = uint32(0x42475241) // kCVPixelFormatType_32BGRA

var (
	pixelBuffersLoaded  sync.Once
	cvPixelFormatKey    uintptr
	cvPixelBufferLock   func(uintptr, uint64) int32
	cvPixelBufferUnlock func(uintptr, uint64) int32
	cvPixelBufferBase   func(uintptr) uintptr
	cvPixelBufferRow    func(uintptr) uintptr
	cvPixelBufferWidth  func(uintptr) int
	cvPixelBufferHeight func(uintptr) int
	cvPixelBufferFormat func(uintptr) uint32
)

func loadPixelBuffers() {
	cv := load("/System/Library/Frameworks/CoreVideo.framework/CoreVideo")
	cvPixelFormatKey = constant(cv, "kCVPixelBufferPixelFormatTypeKey")
	reg := purego.RegisterLibFunc
	reg(&cvPixelBufferLock, cv, "CVPixelBufferLockBaseAddress")
	reg(&cvPixelBufferUnlock, cv, "CVPixelBufferUnlockBaseAddress")
	reg(&cvPixelBufferBase, cv, "CVPixelBufferGetBaseAddress")
	reg(&cvPixelBufferRow, cv, "CVPixelBufferGetBytesPerRow")
	reg(&cvPixelBufferWidth, cv, "CVPixelBufferGetWidth")
	reg(&cvPixelBufferHeight, cv, "CVPixelBufferGetHeight")
	reg(&cvPixelBufferFormat, cv, "CVPixelBufferGetPixelFormatType")
}
