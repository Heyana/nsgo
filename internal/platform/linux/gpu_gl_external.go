package linux

import (
	"fmt"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Importing DMA-BUFs as textures (gpu.ExternalDMABuf, #2296): each import is one plane in a single-plane DRM format, made an EGLImage with EGL_EXT_image_dma_buf_import and bound to a plain GL_TEXTURE_2D with glEGLImageTargetTexture2DOES, so shaders sample it with sampler2D, never samplerExternalOES. This file has no build constraint so its logic is tested on every host.

// glExternalImporter makes and destroys the EGLImages imported textures are bound to.
type glExternalImporter interface {
	// CreateDMABufImage makes an EGLImage of the DMA-BUF attribs describes.
	CreateDMABufImage(attribs []int32) (uintptr, error)
	DestroyImage(image uintptr)
	// Modifiers reports EGL_EXT_image_dma_buf_import_modifiers.
	Modifiers() bool
}

// DRM fourccs (drm_fourcc.h) of the single-plane formats imports take.
const (
	drmFormatR8       = 0x20203852 // 'R', '8', ' ', ' '
	drmFormatGR88     = 0x38385247 // 'G', 'R', '8', '8'
	drmFormatR16      = 0x20363152 // 'R', '1', '6', ' '
	drmFormatGR1616   = 0x32335247 // 'G', 'R', '3', '2'
	drmFormatABGR8888 = 0x34324241 // 'A', 'B', '2', '4'
	drmFormatXBGR8888 = 0x34324258 // 'X', 'B', '2', '4'
	drmFormatARGB8888 = 0x34325241 // 'A', 'R', '2', '4'
	drmFormatXRGB8888 = 0x34325258 // 'X', 'R', '2', '4'
)

// EGL_EXT_image_dma_buf_import(_modifiers) attributes.
const (
	eglNone                  = 0x3038
	eglHeight                = 0x3056
	eglWidth                 = 0x3057
	eglLinuxDRMFourCC        = 0x3271
	eglDMABufPlane0FD        = 0x3272
	eglDMABufPlane0Offset    = 0x3273
	eglDMABufPlane0Pitch     = 0x3274
	eglDMABufPlane0ModLo     = 0x3443
	eglDMABufPlane0ModHi     = 0x3444
	eglLinuxDMABufTarget     = 0x3270
	drmFormatModLinear       = 0
	glDMABufImportPlaneCount = 1
)

// drmPlaneFormat is the texture format a shader samples a single-plane DRM format as.
func drmPlaneFormat(fourcc uint32) (driver.GPUTextureFormat, bool) {
	switch fourcc {
	case drmFormatR8:
		return driver.GPUFormatR8Unorm, true
	case drmFormatGR88:
		return driver.GPUFormatRG8Unorm, true
	case drmFormatR16:
		return driver.GPUFormatR16Unorm, true
	case drmFormatGR1616:
		return driver.GPUFormatRG16Unorm, true
	case drmFormatABGR8888, drmFormatXBGR8888:
		return driver.GPUFormatRGBA8Unorm, true
	case drmFormatARGB8888, drmFormatXRGB8888:
		return driver.GPUFormatBGRA8Unorm, true
	default:
		return driver.GPUFormatUndefined, false
	}
}

// dmaBufImageAttribs builds eglCreateImageKHR's attribute list for desc, naming its modifier when it has one and the display takes modifiers.
func dmaBufImageAttribs(desc *driver.GPUExternalTextureDesc, modifiers bool) ([]int32, error) {
	f, ok := drmPlaneFormat(desc.FourCC)
	if !ok {
		return nil, fmt.Errorf("DMA-BUF format %#08x is not a single-plane format; import each plane as DRM_FORMAT_R8, GR88, R16 or GR1616", desc.FourCC)
	}
	if f != desc.Format {
		return nil, fmt.Errorf("DMA-BUF format %#08x is sampled as %s, not %s", desc.FourCC, f, desc.Format)
	}
	if len(desc.Planes) != glDMABufImportPlaneCount {
		return nil, fmt.Errorf("DMA-BUF format %#08x has 1 plane, got %d", desc.FourCC, len(desc.Planes))
	}
	p := desc.Planes[0]
	attribs := []int32{
		eglWidth, int32(desc.Width),
		eglHeight, int32(desc.Height),
		eglLinuxDRMFourCC, int32(desc.FourCC),
		eglDMABufPlane0FD, int32(p.FD),
		eglDMABufPlane0Offset, int32(p.Offset),
		eglDMABufPlane0Pitch, int32(p.Pitch),
	}
	switch {
	case desc.Modifier == driver.GPUDRMFormatModInvalid:
	case modifiers:
		attribs = append(attribs, eglDMABufPlane0ModLo, int32(uint32(desc.Modifier)), eglDMABufPlane0ModHi, int32(uint32(desc.Modifier>>32)))
	case desc.Modifier != drmFormatModLinear:
		return nil, fmt.Errorf("DMA-BUF modifier %#x needs EGL_EXT_image_dma_buf_import_modifiers", desc.Modifier)
	}
	return append(attribs, eglNone), nil
}

// importTexture binds an EGLImage of desc's DMA-BUF to a new GL texture.
func (c *glGPUContext) importTexture(id driver.GPUID, desc *driver.GPUExternalTextureDesc) *glGPUError {
	if desc == nil {
		return &glGPUError{code: driver.GPUErrorInvalidDescriptor, message: "nil external texture descriptor"}
	}
	if desc.Kind != driver.GPUExternalDMABuf || c.importer == nil {
		return &glGPUError{code: driver.GPUErrorUnsupported, message: fmt.Sprintf("external textures of kind %d cannot be imported here", desc.Kind)}
	}
	attribs, err := dmaBufImageAttribs(desc, c.importer.Modifiers())
	if err != nil {
		return &glGPUError{code: driver.GPUErrorInvalidDescriptor, message: err.Error()}
	}
	img, err := c.importer.CreateDMABufImage(attribs)
	if err != nil {
		return &glGPUError{code: driver.GPUErrorFailed, message: err.Error()}
	}
	c.deleteTexture(id)
	glID := c.gl.GenTexture()
	c.gl.BindTexture(gpuGLTexture2D, glID)
	c.gl.TexParameteri(gpuGLTexture2D, gpuGLTextureBaseLevel, 0)
	c.gl.TexParameteri(gpuGLTexture2D, gpuGLTextureMaxLevel, 0)
	c.gl.TexParameteri(gpuGLTexture2D, gpuGLTextureMinFilter, gpuGLNearest)
	c.gl.TexParameteri(gpuGLTexture2D, gpuGLTextureMagFilter, gpuGLNearest)
	c.gl.TexParameteri(gpuGLTexture2D, gpuGLTextureWrapS, gpuGLClampToEdge)
	c.gl.TexParameteri(gpuGLTexture2D, gpuGLTextureWrapT, gpuGLClampToEdge)
	c.gl.EGLImageTargetTexture2D(gpuGLTexture2D, img)
	glErr := c.gl.GetError()
	c.gl.BindTexture(gpuGLTexture2D, 0)
	if glErr != 0 {
		c.gl.DeleteTexture(glID)
		c.importer.DestroyImage(img)
		return &glGPUError{code: driver.GPUErrorFailed, message: fmt.Sprintf("glEGLImageTargetTexture2DOES failed with GL error %#x", glErr)}
	}
	c.textures[id] = &glGPUTexture{
		glID:      glID,
		target:    gpuGLTexture2D,
		dim:       driver.GPUDim2D,
		format:    desc.Format,
		width:     desc.Width,
		height:    desc.Height,
		layers:    1,
		mipLevels: 1,
		eglImage:  img,
	}
	return nil
}

// deleteTexture deletes texture id and the EGLImage an imported one is bound to.
func (c *glGPUContext) deleteTexture(id driver.GPUID) {
	t, ok := c.textures[id]
	if !ok {
		return
	}
	if t.glID != 0 {
		c.gl.DeleteTexture(t.glID)
	}
	if t.eglImage != 0 && c.importer != nil {
		c.importer.DestroyImage(t.eglImage)
	}
	delete(c.textures, id)
}
