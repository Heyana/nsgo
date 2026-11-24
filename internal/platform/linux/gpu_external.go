//go:build linux && !android

package linux

import (
	"fmt"
	"runtime"

	"github.com/ebitengine/purego"
)

// EGL entry points for importing DMA-BUFs (gpu_gl_external.go), bound once through libepoxy.
var (
	eglImportOnce bool
	eglImportOK   bool

	eglCreateImageKHR            func(dpy, ctx uintptr, target uint32, buffer uintptr, attribs *int32) uintptr
	eglDestroyImageKHR           func(dpy, image uintptr) uint32
	eglGetError                  func() int32
	glEGLImageTargetTexture2DOES func(target uint32, image uintptr)
	epoxyHasEGLExtension         func(dpy uintptr, name *byte) bool
	epoxyHasGLExtension          func(name *byte) bool
	eglGetCurrentDisplay         func() uintptr
)

func loadEGLImport() bool {
	if eglImportOnce {
		return eglImportOK
	}
	eglImportOnce = true
	epoxy, err := purego.Dlopen("libepoxy.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return false
	}
	ok := true
	// libepoxy's GL and EGL functions are pointer variables it resolves on the first call; its own are functions.
	pointer := func(out any, name string) {
		addr, err := purego.Dlsym(epoxy, "epoxy_"+name)
		if err != nil || *(*uintptr)(cPointer(addr)) == 0 {
			ok = false
			return
		}
		purego.RegisterFunc(out, *(*uintptr)(cPointer(addr)))
	}
	function := func(lib uintptr, out any, name string) {
		addr, err := purego.Dlsym(lib, name)
		if err != nil {
			ok = false
			return
		}
		purego.RegisterFunc(out, addr)
	}
	pointer(&eglCreateImageKHR, "eglCreateImageKHR")
	pointer(&eglDestroyImageKHR, "eglDestroyImageKHR")
	pointer(&eglGetError, "eglGetError")
	pointer(&eglGetCurrentDisplay, "eglGetCurrentDisplay")
	pointer(&glEGLImageTargetTexture2DOES, "glEGLImageTargetTexture2DOES")
	function(epoxy, &epoxyHasEGLExtension, "epoxy_has_egl_extension")
	function(epoxy, &epoxyHasGLExtension, "epoxy_has_gl_extension")
	eglImportOK = ok
	return ok
}

// eglImporter makes EGLImages of DMA-BUFs on the EGL display GTK's GL contexts run on.
type eglImporter struct {
	dpy       uintptr
	modifiers bool
}

// newEGLImporter returns the importer for the current GL context, or nil if GTK's context is not EGL, as with GLX, or lacks EGL_EXT_image_dma_buf_import or GL_OES_EGL_image.
func newEGLImporter() *eglImporter {
	if !loadEGLImport() {
		return nil
	}
	// EGL_NO_DISPLAY unless the current context is an EGL one.
	dpy := eglGetCurrentDisplay()
	if dpy == 0 {
		return nil
	}
	has := func(name string) bool {
		cs := cstr(name)
		defer runtime.KeepAlive(cs)
		return epoxyHasEGLExtension(dpy, cs)
	}
	gl := cstr("GL_OES_EGL_image")
	defer runtime.KeepAlive(gl)
	if !has("EGL_EXT_image_dma_buf_import") || !epoxyHasGLExtension(gl) {
		return nil
	}
	return &eglImporter{dpy: dpy, modifiers: has("EGL_EXT_image_dma_buf_import_modifiers")}
}

func (e *eglImporter) Modifiers() bool { return e.modifiers }

func (e *eglImporter) CreateDMABufImage(attribs []int32) (uintptr, error) {
	img := eglCreateImageKHR(e.dpy, 0, eglLinuxDMABufTarget, 0, &attribs[0])
	runtime.KeepAlive(attribs)
	if img == 0 {
		return 0, fmt.Errorf("eglCreateImageKHR of a DMA-BUF failed with EGL error %#x", eglGetError())
	}
	return img, nil
}

func (e *eglImporter) DestroyImage(image uintptr) {
	eglDestroyImageKHR(e.dpy, image)
}

func (e epoxyGL) EGLImageTargetTexture2D(target uint32, image uintptr) {
	if glEGLImageTargetTexture2DOES != nil {
		glEGLImageTargetTexture2DOES(target, image)
	}
}
