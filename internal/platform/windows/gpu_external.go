//go:build windows

package windows

import (
	"errors"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Importing textures other Direct3D 11 devices share (gpu.ExternalD3D11Shared, #2296).

var (
	iidD3D11Device1   = parseGUID("a04bfb29-08ef-43d6-a49c-a9bdbdcbe686")
	iidD3D11Device3   = parseGUID("a05c8c37-d2c6-4732-b3a0-9ce0b0dc9ae6")
	iidD3D11Texture2D = parseGUID("6f15aaf2-d208-4e89-9ab4-489535d34f9c")
	iidDXGIKeyedMutex = parseGUID("9d8e1289-d7b3-465f-8126-250e349af85d")
)

// Vtable indexes (d3d11_1.h, d3d11.h, dxgi.h).
const (
	d3d1OpenSharedResource1 = 48 // ID3D11Device1
	d3d3CreateSRV1          = 57 // ID3D11Device3::CreateShaderResourceView1
	tex2DGetDesc            = 10 // ID3D11Texture2D
	keyedMutexAcquireSync   = 8  // IDXGIKeyedMutex
	keyedMutexReleaseSync   = 9
)

// d3d11SharedTexture is a shared texture opened once for all the planes imported from its handle within one Submit: opening a handle twice can make two resources, and holding the keyed mutex of one does not make the other's contents visible. EndImportBatch drops the handle map entries after each Submit without releasing open textures so a handle value Windows later recycles opens the new resource.
type d3d11SharedTexture struct {
	ops    *d3d11NativeOps
	handle uintptr
	tex    *comObject
	mutex  *comObject
	refs   int
}

func (s *d3d11SharedTexture) release() {
	if s.refs--; s.refs > 0 {
		return
	}
	releaseAll(&s.mutex, &s.tex)
	if s.ops.shared[s.handle] == s {
		delete(s.ops.shared, s.handle)
	}
}

// EndImportBatch forgets this submit's handle-to-resource entries without releasing any imported textures' references.
func (n *d3d11NativeOps) EndImportBatch() {
	clear(n.shared)
}

// openShared returns the shared texture of handle for the current submit, opening it on the device the GPU views render with the first time in this submit.
func (n *d3d11NativeOps) openShared(d3d *comObject, handle uintptr) (*d3d11SharedTexture, uint32, error) {
	if s := n.shared[handle]; s != nil {
		s.refs++
		return s, 0, nil
	}
	dev1 := d3d.query(&iidD3D11Device1)
	if dev1 == nil {
		return nil, 0, errors.New("d3d11: ID3D11Device1 is unavailable, so shared textures cannot be opened")
	}
	defer dev1.release()
	s := &d3d11SharedTexture{ops: n, handle: handle, refs: 1}
	if hr := dev1.call(d3d1OpenSharedResource1, handle, uintptr(unsafe.Pointer(&iidD3D11Texture2D)), uintptr(unsafe.Pointer(&s.tex))); failed(hr) {
		return nil, uint32(hr), hresultError("d3d11: OpenSharedResource1", hr)
	}
	if n.shared == nil {
		n.shared = map[uintptr]*d3d11SharedTexture{}
	}
	n.shared[handle] = s
	return s, 0, nil
}

// ImportTexture opens the shared NT handle and makes a view of the plane ext names.
func (n *d3d11NativeOps) ImportTexture(ext driver.GPUExternalTextureDesc) (d3dReleaser, d3d11Texture2DDesc, uint32, error) {
	g, err := devices()
	if err != nil {
		return nil, d3d11Texture2DDesc{}, 0, err
	}
	s, hr, err := n.openShared(g.d3d, ext.Handle)
	if err != nil {
		return nil, d3d11Texture2DDesc{}, hr, err
	}
	s.tex.addRef()
	h := &d3d11TextureHandle{tex: s.tex, shared: s}
	var td d3d11Texture2DDesc
	h.tex.call(tex2DGetDesc, uintptr(unsafe.Pointer(&td)))
	srv, err := d3dExternalSRVDescriptor(td, ext)
	if err != nil {
		h.Release()
		return nil, td, 0, err
	}
	if err := createPlaneSRV(g.d3d, h, srv, ext.Plane); err != nil {
		h.Release()
		return nil, td, 0, err
	}
	if ext.KeyedMutex {
		if s.mutex == nil {
			s.mutex = s.tex.query(&iidDXGIKeyedMutex)
		}
		if s.mutex == nil {
			h.Release()
			return nil, td, 0, errors.New("d3d11: imported texture has no IDXGIKeyedMutex")
		}
		s.mutex.addRef()
		h.mutex = s.mutex
	}
	return h, td, 0, nil
}

// createPlaneSRV makes h's view of plane: through ID3D11Device3::CreateShaderResourceView1, which names the plane, where the device has it, as drivers do not all pick the chroma plane of an NV12 or P010 texture from a two-channel view's format alone.
func createPlaneSRV(d3d *comObject, h *d3d11TextureHandle, srv d3d11SRVDesc, plane uint32) error {
	if dev3 := d3d.query(&iidD3D11Device3); dev3 != nil {
		defer dev3.release()
		// D3D11_SHADER_RESOURCE_VIEW_DESC1's Texture2D is MostDetailedMip, MipLevels and PlaneSlice.
		srv1 := srv
		srv1.u2 = plane
		if hr := dev3.call(d3d3CreateSRV1, h.tex.ptr(), uintptr(unsafe.Pointer(&srv1)), uintptr(unsafe.Pointer(&h.srv))); failed(hr) {
			return hresultError("d3d11: CreateShaderResourceView1 of an imported texture", hr)
		}
		return nil
	}
	if hr := d3d.call(d3dCreateShaderResourceView, h.tex.ptr(), uintptr(unsafe.Pointer(&srv)), uintptr(unsafe.Pointer(&h.srv))); failed(hr) {
		return hresultError("d3d11: CreateShaderResourceView of an imported texture", hr)
	}
	return nil
}

// keyedMutexArgs passes a UINT64 key as one argument on 64-bit Windows and as two on 32-bit.
func keyedMutexArgs(key uint64, rest ...uintptr) []uintptr {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		return append([]uintptr{uintptr(key)}, rest...)
	}
	return append([]uintptr{uintptr(uint32(key)), uintptr(key >> 32)}, rest...)
}

func (n *d3d11NativeOps) AcquireKeyedMutex(tex *d3dTrackedTexture, key uint64, timeoutMS uint32) (uint32, error) {
	h, _ := tex.handle.(*d3d11TextureHandle)
	if h == nil || h.mutex == nil {
		return 0, errors.New("d3d11: imported texture has no keyed mutex")
	}
	hr := h.mutex.call(keyedMutexAcquireSync, keyedMutexArgs(key, uintptr(timeoutMS))...)
	switch {
	case uint32(hr) == d3dKeyedMutexWaitTimeout:
		return d3dKeyedMutexWaitTimeout, nil
	case uint32(hr) == d3dKeyedMutexWaitAbandoned:
		return uint32(hr), errors.New("d3d11: the imported texture's keyed mutex was abandoned")
	case failed(hr):
		return uint32(hr), hresultError("d3d11: IDXGIKeyedMutex::AcquireSync", hr)
	}
	return 0, nil
}

func (n *d3d11NativeOps) ReleaseKeyedMutex(tex *d3dTrackedTexture, key uint64) {
	if h, _ := tex.handle.(*d3d11TextureHandle); h != nil && h.mutex != nil {
		h.mutex.call(keyedMutexReleaseSync, keyedMutexArgs(key)...)
	}
}
