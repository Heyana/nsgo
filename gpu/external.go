package gpu

import (
	"fmt"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// ExternalTextureKind identifies the GPU memory Device.ImportTexture opens.
type ExternalTextureKind uint8

const (
	// ExternalD3D11Shared is a Direct3D 11 texture another device shares through an NT handle (IDXGIResource1::CreateSharedHandle), such as a D3D11VA decoder's frame. Windows only.
	ExternalD3D11Shared ExternalTextureKind = ExternalTextureKind(driver.GPUExternalD3D11Shared)
	// ExternalDMABuf is a Linux DMA-BUF, such as a VAAPI surface exported with vaExportSurfaceHandle or a GBM buffer. Linux only, where GTK's GL context runs on EGL.
	ExternalDMABuf ExternalTextureKind = ExternalTextureKind(driver.GPUExternalDMABuf)
)

// DRMFormatModInvalid is DRM_FORMAT_MOD_INVALID, the ExternalTextureDesc.Modifier of a DMA-BUF whose layout its driver implies.
const DRMFormatModInvalid = driver.GPUDRMFormatModInvalid

// DMABufPlane is one plane of a DMA-BUF: its file descriptor, byte offset and row pitch.
type DMABufPlane struct {
	FD     int
	Offset uint32
	Pitch  uint32
}

// ExternalTextureDesc describes a texture Device.ImportTexture opens from GPU memory another device or API owns.
//
// An import is one plane sampled as a plain 2D texture: import a planar NV12 frame as two textures, its luma as FormatR8Unorm and its chroma as FormatRG8Unorm at half the size, and convert to RGB in the shader. A 10-bit P010 frame's planes are FormatR16Unorm and FormatRG16Unorm.
//
// The caller keeps ownership of Handle and the planes' FDs: keep them open until the FrameFence of the first frame submitted after ImportTexture completes, after which the backend holds its own reference, and close them yourself.
type ExternalTextureDesc struct {
	Label string
	Kind  ExternalTextureKind
	// Width and Height are the plane's size in texels.
	Width, Height uint32
	// Format is the plane's format as the shader samples it: FormatR8Unorm, FormatRG8Unorm, FormatR16Unorm, FormatRG16Unorm, FormatRGBA8Unorm or FormatBGRA8Unorm.
	Format TextureFormat

	// Handle is the shared texture's NT handle (ExternalD3D11Shared).
	Handle uintptr
	// Plane picks the plane of an NV12 or P010 texture: 0 for luma, 1 for chroma (ExternalD3D11Shared).
	Plane uint32
	// KeyedMutex makes the backend hold the texture's IDXGIKeyedMutex while a submitted frame reads it, acquiring AcquireKey and releasing ReleaseKey (ExternalD3D11Shared). The texture must have been created with D3D11_RESOURCE_MISC_SHARED_KEYEDMUTEX; with both keys 0 it is a plain mutex.
	KeyedMutex bool
	AcquireKey uint64
	ReleaseKey uint64

	// FourCC is the plane's single-plane DRM format: DRM_FORMAT_R8, GR88, R16, GR1616, ABGR8888, XBGR8888, ARGB8888 or XRGB8888 (ExternalDMABuf).
	FourCC uint32
	// Modifier is the DRM format modifier: 0 for DRM_FORMAT_MOD_LINEAR, DRMFormatModInvalid for none (ExternalDMABuf).
	Modifier uint64
	// Planes holds the plane's DMA-BUF, one (ExternalDMABuf).
	Planes []DMABufPlane
}

// SupportsExternalTexture reports whether d can import textures of kind. Where it is false, ImportTexture returns ErrUnsupported: copy the frame to memory and upload it with WriteTexture instead.
func (d *Device) SupportsExternalTexture(kind ExternalTextureKind) bool {
	if d == nil {
		return false
	}
	switch kind {
	case ExternalD3D11Shared:
		return d.features.ImportD3D11Shared
	case ExternalDMABuf:
		return d.features.ImportDMABuf
	default:
		return false
	}
}

// ImportTexture opens GPU memory another device or API owns, such as a hardware decoder's frame, as a sampled 2D texture scoped to cx, without copying it. The texture goes into bind groups and is sampled like any other; it cannot be written, copied, read back, rendered to or retained across device loss, so render it to a texture of your own to do those. Close releases what the backend opened, after the frames already submitted finish with it: give the frame back to its owner, such as FFmpeg's pool, once the FrameFence of the last frame that sampled it completes.
//
// The backend opens the memory when the next frame is submitted; an import it rejects, such as a handle that does not open or a format that does not match, arrives on ViewProps.OnError.
func (d *Device) ImportTexture(cx *core.Scope, desc ExternalTextureDesc) (*Texture, error) {
	if !d.alive() {
		return nil, Error{Code: ErrDeviceLost, Message: "device is lost or disposed"}
	}
	if !d.SupportsExternalTexture(desc.Kind) {
		return nil, Error{Code: ErrUnsupported, Message: fmt.Sprintf("importing external textures of kind %d is not supported on this device", desc.Kind)}
	}
	if desc.Width == 0 || desc.Height == 0 {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "imported texture width and height must be positive"}
	}
	if d.limits.MaxTextureDimension2D > 0 && (desc.Width > d.limits.MaxTextureDimension2D || desc.Height > d.limits.MaxTextureDimension2D) {
		return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("imported texture %dx%d exceeds limit %d", desc.Width, desc.Height, d.limits.MaxTextureDimension2D)}
	}
	switch desc.Format {
	case FormatR8Unorm, FormatRG8Unorm, FormatR16Unorm, FormatRG16Unorm, FormatRGBA8Unorm, FormatBGRA8Unorm:
	default:
		return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("textures cannot be imported in format %s", desc.Format)}
	}
	switch desc.Kind {
	case ExternalD3D11Shared:
		if desc.Handle == 0 {
			return nil, Error{Code: ErrInvalidDescriptor, Message: "ExternalD3D11Shared requires a shared Handle"}
		}
		if desc.Plane > 1 {
			return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("plane %d does not exist (0 is luma, 1 is chroma)", desc.Plane)}
		}
	case ExternalDMABuf:
		if desc.FourCC == 0 {
			return nil, Error{Code: ErrInvalidDescriptor, Message: "ExternalDMABuf requires a DRM FourCC"}
		}
		if len(desc.Planes) != 1 {
			return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("ExternalDMABuf imports one plane, got %d", len(desc.Planes))}
		}
		for i, p := range desc.Planes {
			if p.FD < 0 {
				return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("DMA-BUF plane %d has no file descriptor", i)}
			}
		}
	}
	lbl := debugLabel(desc.Label)
	dd := driver.GPUExternalTextureDesc{
		Label:      lbl,
		Kind:       driver.GPUExternalTextureKind(desc.Kind),
		Width:      desc.Width,
		Height:     desc.Height,
		Format:     driver.GPUTextureFormat(desc.Format),
		Handle:     desc.Handle,
		Plane:      desc.Plane,
		KeyedMutex: desc.KeyedMutex,
		AcquireKey: desc.AcquireKey,
		ReleaseKey: desc.ReleaseKey,
		FourCC:     desc.FourCC,
		Modifier:   desc.Modifier,
	}
	for _, p := range desc.Planes {
		dd.Planes = append(dd.Planes, driver.GPUDMABufPlane{FD: p.FD, Offset: p.Offset, Pitch: p.Pitch})
	}
	id := d.nextID()
	d.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpImportTexture, ID: id, External: &dd})
	tex := &Texture{
		dev:   d,
		id:    id,
		epoch: d.generation,
		desc: TextureDesc{
			Label:       lbl,
			Dimension:   Dim2D,
			Width:       desc.Width,
			Height:      desc.Height,
			Layers:      1,
			MipLevels:   1,
			SampleCount: 1,
			Format:      desc.Format,
			Usage:       TextureSampled,
		},
		external: true,
	}
	d.bindCleanup(cx, func() { _ = tex.Close() })
	return tex, nil
}

// Imported reports whether t was opened by Device.ImportTexture.
func (t *Texture) Imported() bool { return t != nil && t.external }
