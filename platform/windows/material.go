//go:build windows

package windows

import (
	"sync"
	"syscall"
	"unsafe"

	"github.com/oneo-me/nsgo/core"
)

// A window's material is a DWM system backdrop, drawn behind the window's
// content where the content is transparent. Windows 11 22H2 (build 22621)
// and later draw Mica, Acrylic and Mica Alt; earlier Windows keeps the
// window opaque. The window draws through DirectComposition when it has a
// material, since only a surface with alpha shows what DWM draws behind it.

var (
	ntdll                        = syscall.NewLazyDLL("ntdll.dll")
	rtlGetVersion                = ntdll.NewProc("RtlGetVersion")
	dwmExtendFrameIntoClientArea = dwmapi.NewProc("DwmExtendFrameIntoClientArea")
)

const (
	dwmwaSystemBackdropType = 38

	backdropNone    = 1 // DWMSBT_NONE
	backdropMica    = 2 // DWMSBT_MAINWINDOW
	backdropAcrylic = 3 // DWMSBT_TRANSIENTWINDOW
	backdropMicaAlt = 4 // DWMSBT_TABBEDWINDOW

	backdropBuild = 22621
)

// margins is MARGINS: how far the DWM frame extends into the client area.
type margins struct{ Left, Right, Top, Bottom int32 }

// osVersionInfo is RTL_OSVERSIONINFOW.
type osVersionInfo struct {
	Size, Major, Minor, Build, PlatformID uint32
	CSDVersion                            [128]uint16
}

// windowsBuild returns the build of Windows 10 or 11, the maximum on a
// later major version. RtlGetVersion tells the version whatever the
// executable's manifest declares.
var windowsBuild = sync.OnceValue(func() uint32 {
	var v osVersionInfo
	v.Size = uint32(unsafe.Sizeof(v))
	rtlGetVersion.Call(uintptr(unsafe.Pointer(&v)))
	switch {
	case v.Major > 10:
		return ^uint32(0)
	case v.Major < 10:
		return 0
	}
	return v.Build
})

// systemBackdrops reports Windows 11 22H2 or later, where DWM draws the
// materials of DWMWA_SYSTEMBACKDROP_TYPE.
func systemBackdrops() bool { return windowsBuild() >= backdropBuild }

// compositionAvailable reports whether DirectComposition can start. A
// material window shows its content through it, over the backdrop.
func compositionAvailable() bool {
	if dcompositionCreateDevice2.Find() != nil {
		return false
	}
	var device *comObject
	hr, _, _ := dcompositionCreateDevice2.Call(0, uintptr(unsafe.Pointer(&iidDCompDesktopDevice)),
		uintptr(unsafe.Pointer(&device)))
	if failed(hr) || device == nil {
		return false
	}
	device.release()
	return true
}

// backdropFor returns the system backdrop closest to m: Mica and its Alt
// for window materials, Acrylic for transient surfaces.
func backdropFor(m core.WindowMaterial) int32 {
	switch m {
	case core.WindowMaterialNone:
		return backdropNone
	case core.WindowMaterialMica:
		return backdropMica
	case core.WindowMaterialTabbed:
		return backdropMicaAlt
	case core.WindowMaterialAcrylic, core.WindowMaterialMenu, core.WindowMaterialPopover,
		core.WindowMaterialHUD, core.WindowMaterialSheet, core.WindowMaterialTooltip,
		core.WindowMaterialSelection, core.WindowMaterialFullScreenUI:
		return backdropAcrylic
	}
	return backdropMica
}

// applyMaterial makes DWM draw the window's backdrop. It reports whether
// it did; the caller keeps the window opaque when it did not.
func (w *window) applyMaterial() bool {
	if w.backdrop == 0 || w.backdrop == backdropNone {
		return false
	}
	m := margins{-1, -1, -1, -1}
	if hr, _, _ := dwmExtendFrameIntoClientArea.Call(w.hwnd, uintptr(unsafe.Pointer(&m))); failed(hr) {
		return false
	}
	backdrop := w.backdrop
	if hr, _, _ := dwmSetWindowAttribute.Call(w.hwnd, dwmwaSystemBackdropType,
		uintptr(unsafe.Pointer(&backdrop)), 4); failed(hr) {
		return false
	}
	return true
}
