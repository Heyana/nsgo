//go:build windows

package windows

import (
	"errors"
	"fmt"
	"math"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

// Direct3D 11 GPU view backend (docs/architecture/gpu.md, #1106).
//
// Each KindGPU view renders on the same ID3D11Device that Direct2D uses (mainGfx.d3d) into a BGRA8Unorm ID3D11Texture2D wrapped as an ID2D1Bitmap1 via IDXGISurface. Off-commit GPUFrame batches arrive through window.SubmitGPU (wmGPU) without triggering UI view-tree commits; after executing passes that touch the swapchain texture, the window marks only the GPU view damaged so Direct2D composites it cleanly beneath any HUD children.

// Additional ID3D11Device and ID3D11DeviceContext vtable indexes beyond those already declared in shader.go and video.go (verified against d3d11.h).
const (
	// ID3D11Device
	d3dCreateDepthStencilView  = 10
	d3dCreateInputLayout       = 11
	d3dCreateBlendState        = 20
	d3dCreateDepthStencilState = 21
	d3dCreateRasterizerState   = 22
	d3dCreateSamplerState      = 23
	d3dGetDeviceRemovedReason  = 39

	// ID3D11DeviceContext
	ctxVSSetConstantBuffers   = 7
	ctxPSSetSamplers          = 10
	ctxDrawIndexed            = 12
	ctxMap                    = 14
	ctxUnmap                  = 15
	ctxIASetVertexBuffers     = 18
	ctxIASetIndexBuffer       = 19
	ctxDrawIndexedInstanced   = 20
	ctxDrawInstanced          = 21
	ctxVSSetShaderResources   = 25
	ctxVSSetSamplers          = 26
	ctxOMSetBlendState        = 35
	ctxOMSetDepthStencilState = 36
	ctxRSSetState             = 43
	ctxRSSetScissorRects      = 45
	ctxCopySubresourceRegion  = 46
	ctxCopyResource           = 47
	ctxClearDepthStencilView  = 53
	ctxResolveSubresource     = 57
)

// d3d11InputElementDesc matches D3D11_INPUT_ELEMENT_DESC.
type d3d11InputElementDesc struct {
	semanticName         *byte
	semanticIndex        uint32
	format               uint32
	inputSlot            uint32
	alignedByteOffset    uint32
	inputSlotClass       uint32
	instanceDataStepRate uint32
}

// d3d11SubresourceData matches D3D11_SUBRESOURCE_DATA.
type d3d11SubresourceData struct {
	pSysMem          uintptr
	sysMemPitch      uint32
	sysMemSlicePitch uint32
}

// d3d11MappedSubresource matches D3D11_MAPPED_SUBRESOURCE.
type d3d11MappedSubresource struct {
	pData      *byte
	rowPitch   uint32
	depthPitch uint32
}

// Null-terminated ASCII semantic name kept alive for CreateInputLayout.
var d3dTexcoordSemantic = []byte("TEXCOORD\x00")

// gpuAvailable reports whether Direct3D 11 and D3DCompile are available on this Windows host (hardware adapter or WARP software fallback).
func gpuAvailable() bool {
	if _, err := devices(); err != nil {
		return false
	}
	return d3dCompile.Find() == nil
}

// compileHLSLDetailed compiles entry of src for target (vs_5_0 / ps_5_0), returning bytecode or the compiler's diagnostic text.
func compileHLSLDetailed(src, entry, target string) ([]byte, string) {
	if err := d3dCompile.Find(); err != nil {
		return nil, err.Error()
	}
	if len(src) == 0 {
		return nil, "empty HLSL source"
	}
	code := []byte(src)
	name := append([]byte("nsgo_gpu"), 0)
	e := append([]byte(entry), 0)
	t := append([]byte(target), 0)
	var blob, errs *comObject
	hr, _, _ := d3dCompile.Call(
		uintptr(unsafe.Pointer(&code[0])), uintptr(len(code)),
		uintptr(unsafe.Pointer(&name[0])),
		0, 0,
		uintptr(unsafe.Pointer(&e[0])),
		uintptr(unsafe.Pointer(&t[0])),
		0, 0,
		uintptr(unsafe.Pointer(&blob)),
		uintptr(unsafe.Pointer(&errs)),
	)
	var diag string
	if errs != nil {
		if failed(hr) {
			diag = blobString(errs)
		}
		errs.release()
	}
	if failed(hr) || blob == nil {
		if diag == "" {
			diag = hresultError("D3DCompile", hr).Error()
		}
		return nil, diag
	}
	defer blob.release()
	return []byte(blobString(blob)), ""
}

// --- Native D3D11 resource wrappers implementing d3dReleaser ---

type d3d11SwapTargets struct {
	swapTex, swapRTV, bitmap *comObject
	msaaTex, msaaRTV         *comObject
	depthTex, depthDSV       *comObject
	hdrSwap                  *comObject // optional DirectComposition HDR IDXGISwapChain1
	format                   driver.GPUTextureFormat
	colorSpace               driver.GPUColorSpace
	dxgiFmt                  uint32
	depthFmt                 driver.GPUTextureFormat
	bitmapScale              float32
	generation               uint64
}

func (s *d3d11SwapTargets) SurfaceFormat() driver.GPUTextureFormat  { return s.format }
func (s *d3d11SwapTargets) SurfaceColorSpace() driver.GPUColorSpace { return s.colorSpace }

func (s *d3d11SwapTargets) Release() {
	if s == nil {
		return
	}
	releaseAll(&s.hdrSwap, &s.bitmap, &s.depthDSV, &s.depthTex, &s.msaaRTV, &s.msaaTex, &s.swapRTV, &s.swapTex)
}

func (s *d3d11SwapTargets) presentHDR(ctx *comObject) {
	if s == nil || s.hdrSwap == nil || s.swapTex == nil || ctx == nil {
		return
	}
	var back *comObject
	if failed(s.hdrSwap.call(swapGetBuffer, 0, uintptr(unsafe.Pointer(&iidD3D11Texture2D)), uintptr(unsafe.Pointer(&back)))) || back == nil {
		return
	}
	ctx.call(ctxCopyResource, back.ptr(), s.swapTex.ptr())
	back.release()
	s.hdrSwap.call(swapPresent, 0, 0)
}

func (s *d3d11SwapTargets) ensureBitmap(dc *comObject, scale float32) *comObject {
	if s == nil || s.swapTex == nil {
		return nil
	}
	if s.bitmap != nil && s.bitmapScale == scale {
		return s.bitmap
	}
	releaseAll(&s.bitmap)
	surface := s.swapTex.query(&iidDXGISurface)
	if surface == nil {
		return nil
	}
	defer surface.release()
	if scale <= 0 {
		scale = 1
	}
	props := bitmapProperties1{
		format:    s.dxgiFmt,
		alphaMode: d2dAlphaPremultiplied,
		dpiX:      96 * scale,
		dpiY:      96 * scale,
	}
	if failed(dc.call(dcCreateBitmapFromDxgiSurface, surface.ptr(), uintptr(unsafe.Pointer(&props)),
		uintptr(unsafe.Pointer(&s.bitmap)))) {
		return nil
	}
	s.bitmapScale = scale
	return s.bitmap
}

type d3d11BufferHandle struct {
	buf *comObject // non-constant buffer (vertex/index/storage), nil for uniform-only
	cb  *comObject // constant buffer (D3D11_BIND_CONSTANT_BUFFER), nil if not uniform
	srv *comObject // optional structured-buffer SRV
}

func (b *d3d11BufferHandle) Release() {
	if b == nil {
		return
	}
	releaseAll(&b.srv, &b.cb, &b.buf)
}

type d3d11TextureHandle struct {
	tex, srv, rtv, dsv *comObject
	sliceRTVs          map[uint64]*comObject
	// shared is the shared texture an imported plane views, and mutex its IDXGIKeyedMutex (gpu_external.go).
	shared *d3d11SharedTexture
	mutex  *comObject
}

func (t *d3d11TextureHandle) Release() {
	if t == nil {
		return
	}
	for k, r := range t.sliceRTVs {
		r.release()
		delete(t.sliceRTVs, k)
	}
	t.sliceRTVs = nil
	releaseAll(&t.mutex, &t.dsv, &t.rtv, &t.srv, &t.tex)
	if t.shared != nil {
		t.shared.release()
		t.shared = nil
	}
}

func (t *d3d11TextureHandle) rtvFor(d3d *comObject, desc driver.GPUTextureDesc, mip, layer uint32) *comObject {
	if mip == 0 && layer == 0 && t.rtv != nil {
		return t.rtv
	}
	key := uint64(mip)<<32 | uint64(layer)
	if r := t.sliceRTVs[key]; r != nil {
		return r
	}
	_, _, rtvFmt, _ := d3dTextureFormats(desc.Format, desc.Usage&driver.GPUTextureSampled != 0)
	if rtvFmt == dxgiFmtUnknown {
		return nil
	}
	samples := uint32(max(desc.SampleCount, 1))
	layers := max(desc.Layers, 1)
	if desc.Dimension == driver.GPUDimCube && layers < 6 {
		layers = 6
	}
	rd := d3dRTVDescriptorForSlice(rtvFmt, samples, layers, mip, layer)
	var r *comObject
	if failed(d3d.call(d3dCreateRenderTargetView, t.tex.ptr(), uintptr(unsafe.Pointer(&rd)), uintptr(unsafe.Pointer(&r)))) {
		return nil
	}
	if t.sliceRTVs == nil {
		t.sliceRTVs = map[uint64]*comObject{}
	}
	t.sliceRTVs[key] = r
	return r
}

type d3d11SamplerHandle struct {
	state *comObject
}

func (s *d3d11SamplerHandle) Release() {
	if s == nil {
		return
	}
	releaseAll(&s.state)
}

type d3d11PipelineHandle struct {
	vs, ps      *comObject
	inputLayout *comObject
	rs          *comObject
	dss         *comObject
	bs          *comObject
}

func (p *d3d11PipelineHandle) Release() {
	if p == nil {
		return
	}
	releaseAll(&p.bs, &p.dss, &p.rs, &p.inputLayout, &p.ps, &p.vs)
}

// --- d3d11NativeOps implements d3dNativeOps over mainGfx's ID3D11Device ---

type d3d11NativeOps struct {
	scaleFn func() float32
	// shared holds each shared texture opened during the current Submit, by NT handle, cleared by EndImportBatch (gpu_external.go).
	shared map[uintptr]*d3d11SharedTexture
}

func (n *d3d11NativeOps) getDevices() (*gfx, *comObject, error) {
	g, err := devices()
	if err != nil {
		return nil, nil, err
	}
	var ctx *comObject
	g.d3d.call(d3dGetImmediateContext, uintptr(unsafe.Pointer(&ctx)))
	if ctx == nil {
		return nil, nil, errors.New("d3d11: failed to obtain immediate device context")
	}
	return g, ctx, nil
}

func (n *d3d11NativeOps) DeviceRemovedReason() uint32 {
	if mainGfx == nil || mainGfx.d3d == nil {
		return 0
	}
	hr := mainGfx.d3d.call(d3dGetDeviceRemovedReason)
	if failed(hr) {
		return uint32(hr)
	}
	return 0
}

func (n *d3d11NativeOps) CreateSwapTargets(pw, ph uint32, cfg driver.GPUViewConfig) (d3dReleaser, uint32, error) {
	g, err := devices()
	if err != nil {
		return nil, 0, err
	}
	surfFmt, surfCS, dxgiFmt, dxgiCS := d3dResolveSwapFormat(cfg)
	var hdrSwap *comObject
	if surfCS.IsHDR() {
		if sc, scErr := createHDRCompositionSwapChain(g, pw, ph, dxgiFmt, dxgiCS); scErr == nil {
			hdrSwap = sc
		} else {
			// Fall back to standard SDR Direct2D bitmap composition when DirectComposition or the HDR color space is unavailable.
			surfFmt = driver.GPUFormatBGRA8Unorm
			surfCS = driver.GPUColorSpaceSRGB
			dxgiFmt = dxgiFmtB8G8R8A8Unorm
		}
	}
	st := &d3d11SwapTargets{
		hdrSwap:    hdrSwap,
		format:     surfFmt,
		colorSpace: surfCS,
		dxgiFmt:    dxgiFmt,
		depthFmt:   cfg.DepthFormat,
		generation: gfxGeneration,
	}
	swapDesc := d3d11Texture2DDesc{
		width:       pw,
		height:      ph,
		mipLevels:   1,
		arraySize:   1,
		format:      dxgiFmt,
		sampleCount: 1,
		usage:       d3d11UsageDefault,
		bindFlags:   d3d11BindRenderTarget | d3d11BindShaderResource,
	}
	if hr := g.d3d.call(d3dCreateTexture2D, uintptr(unsafe.Pointer(&swapDesc)), 0, uintptr(unsafe.Pointer(&st.swapTex))); failed(hr) {
		st.Release()
		return nil, uint32(hr), hresultError("d3d11: CreateTexture2D swapchain", hr)
	}
	if hr := g.d3d.call(d3dCreateRenderTargetView, st.swapTex.ptr(), 0, uintptr(unsafe.Pointer(&st.swapRTV))); failed(hr) {
		st.Release()
		return nil, uint32(hr), hresultError("d3d11: CreateRenderTargetView swapchain", hr)
	}
	samples := uint32(max(cfg.SampleCount, 1))
	if samples > 1 {
		msaaDesc := d3d11Texture2DDesc{
			width:       pw,
			height:      ph,
			mipLevels:   1,
			arraySize:   1,
			format:      dxgiFmt,
			sampleCount: samples,
			usage:       d3d11UsageDefault,
			bindFlags:   d3d11BindRenderTarget,
		}
		if hr := g.d3d.call(d3dCreateTexture2D, uintptr(unsafe.Pointer(&msaaDesc)), 0, uintptr(unsafe.Pointer(&st.msaaTex))); failed(hr) {
			// Fall back to non-MSAA if the adapter rejects the sample count.
			samples = 1
		} else if hr := g.d3d.call(d3dCreateRenderTargetView, st.msaaTex.ptr(), 0, uintptr(unsafe.Pointer(&st.msaaRTV))); failed(hr) {
			releaseAll(&st.msaaTex)
			samples = 1
		}
	}
	if cfg.DepthFormat != driver.GPUFormatUndefined {
		depthTexFmt, _, _, depthDSVFmt := d3dTextureFormats(cfg.DepthFormat, false)
		depthDesc := d3d11Texture2DDesc{
			width:       pw,
			height:      ph,
			mipLevels:   1,
			arraySize:   1,
			format:      depthTexFmt,
			sampleCount: samples,
			usage:       d3d11UsageDefault,
			bindFlags:   d3d11BindDepthStencil,
		}
		if hr := g.d3d.call(d3dCreateTexture2D, uintptr(unsafe.Pointer(&depthDesc)), 0, uintptr(unsafe.Pointer(&st.depthTex))); failed(hr) {
			st.Release()
			return nil, uint32(hr), hresultError("d3d11: CreateTexture2D depth", hr)
		}
		dsvDesc := d3d11DSVDesc{format: depthDSVFmt, viewDimension: d3d11DSVDimTexture2D}
		if samples > 1 {
			dsvDesc.viewDimension = d3d11DSVDimTexture2DMS
		}
		if hr := g.d3d.call(d3dCreateDepthStencilView, st.depthTex.ptr(), uintptr(unsafe.Pointer(&dsvDesc)), uintptr(unsafe.Pointer(&st.depthDSV))); failed(hr) {
			st.Release()
			return nil, uint32(hr), hresultError("d3d11: CreateDepthStencilView swapchain", hr)
		}
	}
	if st.hdrSwap == nil {
		scale := float32(1)
		if n.scaleFn != nil {
			scale = n.scaleFn()
		}
		st.ensureBitmap(g.dc, scale)
	}
	return st, 0, nil
}

func (n *d3d11NativeOps) CreateBuffer(mainDesc d3d11BufferDesc, hasCB bool, cbDesc d3d11BufferDesc, initData, cbShadow []byte) (d3dReleaser, uint32, error) {
	g, err := devices()
	if err != nil {
		return nil, 0, err
	}
	h := &d3d11BufferHandle{}
	if hasCB {
		var subPtr uintptr
		var sub d3d11SubresourceData
		if len(cbShadow) > 0 {
			sub.pSysMem = uintptr(unsafe.Pointer(&cbShadow[0]))
			subPtr = uintptr(unsafe.Pointer(&sub))
		}
		if hr := g.d3d.call(d3dCreateBuffer, uintptr(unsafe.Pointer(&cbDesc)), subPtr, uintptr(unsafe.Pointer(&h.cb))); failed(hr) {
			return nil, uint32(hr), hresultError("d3d11: CreateBuffer constant", hr)
		}
	}
	if mainDesc.bindFlags != d3d11BindConstantBuffer {
		var subPtr uintptr
		var sub d3d11SubresourceData
		var padded []byte
		if len(initData) > 0 {
			if uint32(len(initData)) >= mainDesc.byteWidth {
				sub.pSysMem = uintptr(unsafe.Pointer(&initData[0]))
			} else {
				padded = make([]byte, mainDesc.byteWidth)
				copy(padded, initData)
				sub.pSysMem = uintptr(unsafe.Pointer(&padded[0]))
			}
			subPtr = uintptr(unsafe.Pointer(&sub))
		}
		if hr := g.d3d.call(d3dCreateBuffer, uintptr(unsafe.Pointer(&mainDesc)), subPtr, uintptr(unsafe.Pointer(&h.buf))); failed(hr) {
			h.Release()
			return nil, uint32(hr), hresultError("d3d11: CreateBuffer", hr)
		}
		if mainDesc.bindFlags&d3d11BindShaderResource != 0 {
			stride := max(mainDesc.structureByteStride, 4)
			srvDesc := d3d11SRVDesc{
				format:        dxgiFmtUnknown,
				viewDimension: d3d11SRVDimBuffer,
				u0:            0,
				u1:            mainDesc.byteWidth / stride,
			}
			if hr := g.d3d.call(d3dCreateShaderResourceView, h.buf.ptr(), uintptr(unsafe.Pointer(&srvDesc)), uintptr(unsafe.Pointer(&h.srv))); failed(hr) {
				h.Release()
				return nil, uint32(hr), hresultError("d3d11: CreateShaderResourceView buffer", hr)
			}
		}
	}
	return h, 0, nil
}

func (n *d3d11NativeOps) WriteBuffer(buf *d3dTrackedBuffer, offset uint64, data []byte) (uint32, error) {
	_, ctx, err := n.getDevices()
	if err != nil {
		return 0, err
	}
	defer ctx.release()
	h, ok := buf.handle.(*d3d11BufferHandle)
	if !ok || h == nil {
		return 0, errors.New("d3d11: invalid buffer handle")
	}
	if h.cb != nil && len(buf.cbShadow) > 0 {
		// D3D11 constant buffers require full-buffer UpdateSubresource with pDstBox == NULL.
		ctx.call(ctxUpdateSubresource, h.cb.ptr(), 0, 0, uintptr(unsafe.Pointer(&buf.cbShadow[0])), 0, 0)
	}
	if h.buf != nil && len(data) > 0 && offset < uint64(buf.mainDesc.byteWidth) {
		n := uint32(min(uint64(len(data)), uint64(buf.mainDesc.byteWidth)-offset))
		if offset == 0 && n == buf.mainDesc.byteWidth {
			ctx.call(ctxUpdateSubresource, h.buf.ptr(), 0, 0, uintptr(unsafe.Pointer(&data[0])), 0, 0)
		} else {
			box := d3d11Box{
				left:   uint32(offset),
				right:  uint32(offset) + n,
				top:    0,
				bottom: 1,
				front:  0,
				back:   1,
			}
			ctx.call(ctxUpdateSubresource, h.buf.ptr(), 0, uintptr(unsafe.Pointer(&box)), uintptr(unsafe.Pointer(&data[0])), 0, 0)
		}
	}
	return 0, nil
}

func (n *d3d11NativeOps) CreateTexture(desc driver.GPUTextureDesc, td d3d11Texture2DDesc, srv d3d11SRVDesc, rtv d3d11RTVDesc, dsv d3d11DSVDesc) (d3dReleaser, uint32, error) {
	g, err := devices()
	if err != nil {
		return nil, 0, err
	}
	h := &d3d11TextureHandle{}
	if hr := g.d3d.call(d3dCreateTexture2D, uintptr(unsafe.Pointer(&td)), 0, uintptr(unsafe.Pointer(&h.tex))); failed(hr) {
		return nil, uint32(hr), hresultError("d3d11: CreateTexture2D", hr)
	}
	if srv.format != dxgiFmtUnknown && td.bindFlags&d3d11BindShaderResource != 0 {
		if hr := g.d3d.call(d3dCreateShaderResourceView, h.tex.ptr(), uintptr(unsafe.Pointer(&srv)), uintptr(unsafe.Pointer(&h.srv))); failed(hr) {
			h.Release()
			return nil, uint32(hr), hresultError("d3d11: CreateShaderResourceView", hr)
		}
	}
	if rtv.format != dxgiFmtUnknown && td.bindFlags&d3d11BindRenderTarget != 0 {
		if hr := g.d3d.call(d3dCreateRenderTargetView, h.tex.ptr(), uintptr(unsafe.Pointer(&rtv)), uintptr(unsafe.Pointer(&h.rtv))); failed(hr) {
			h.Release()
			return nil, uint32(hr), hresultError("d3d11: CreateRenderTargetView", hr)
		}
	}
	if dsv.format != dxgiFmtUnknown && td.bindFlags&d3d11BindDepthStencil != 0 {
		if hr := g.d3d.call(d3dCreateDepthStencilView, h.tex.ptr(), uintptr(unsafe.Pointer(&dsv)), uintptr(unsafe.Pointer(&h.dsv))); failed(hr) {
			h.Release()
			return nil, uint32(hr), hresultError("d3d11: CreateDepthStencilView", hr)
		}
	}
	return h, 0, nil
}

func (n *d3d11NativeOps) WriteTexture(tex *d3dTrackedTexture, dst driver.GPUTextureCopy, data []byte, layout driver.GPUDataLayout) (uint32, error) {
	_, ctx, err := n.getDevices()
	if err != nil {
		return 0, err
	}
	defer ctx.release()
	h, ok := tex.handle.(*d3d11TextureHandle)
	if !ok || h == nil || h.tex == nil {
		return 0, errors.New("d3d11: invalid texture handle")
	}
	bpp := tex.desc.Format.BytesPerPixel()
	if bpp == 0 {
		return 0, errors.New("d3d11: WriteTexture requires a color texture format")
	}
	if layout.Offset >= uint64(len(data)) {
		return 0, nil
	}
	if dst.MipLevel >= tex.texDesc.mipLevels || dst.ArrayLayer >= tex.texDesc.arraySize {
		return 0, nil
	}
	src := data[layout.Offset:]
	mipW := max(tex.texDesc.width>>dst.MipLevel, 1)
	mipH := max(tex.texDesc.height>>dst.MipLevel, 1)
	if dst.OriginX >= mipW || dst.OriginY >= mipH {
		return 0, nil
	}
	rowPitch := layout.BytesPerRow
	if rowPitch == 0 {
		rowPitch = mipW * bpp
	}
	if rowPitch < bpp {
		return 0, nil
	}
	writeW := min(rowPitch/bpp, mipW-dst.OriginX)
	writeH := layout.RowsPerImage
	if writeH == 0 {
		writeH = uint32(len(src)) / rowPitch
	}
	if writeH == 0 {
		writeH = mipH - dst.OriginY
	}
	if dst.OriginY+writeH > mipH {
		writeH = mipH - dst.OriginY
	}
	// Clamp writeH to rows fully backed by src so UpdateSubresource never over-reads.
	rowBytes := uint64(writeW) * uint64(bpp)
	for writeH > 0 {
		need := uint64(writeH-1)*uint64(rowPitch) + rowBytes
		if need <= uint64(len(src)) {
			break
		}
		writeH--
	}
	if writeW == 0 || writeH == 0 {
		return 0, nil
	}
	sub := d3dCalcSubresource(dst.MipLevel, dst.ArrayLayer, tex.texDesc.mipLevels)
	var boxPtr uintptr
	var box d3d11Box
	if dst.OriginX != 0 || dst.OriginY != 0 || writeW != mipW || writeH != mipH {
		box = d3d11Box{
			left:   dst.OriginX,
			top:    dst.OriginY,
			front:  0,
			right:  dst.OriginX + writeW,
			bottom: dst.OriginY + writeH,
			back:   1,
		}
		boxPtr = uintptr(unsafe.Pointer(&box))
	}
	ctx.call(ctxUpdateSubresource, h.tex.ptr(), uintptr(sub), boxPtr, uintptr(unsafe.Pointer(&src[0])), uintptr(rowPitch), 0)
	return 0, nil
}

func (n *d3d11NativeOps) CreateSampler(desc d3d11SamplerDesc) (d3dReleaser, uint32, error) {
	g, err := devices()
	if err != nil {
		return nil, 0, err
	}
	h := &d3d11SamplerHandle{}
	if hr := g.d3d.call(d3dCreateSamplerState, uintptr(unsafe.Pointer(&desc)), uintptr(unsafe.Pointer(&h.state))); failed(hr) {
		return nil, uint32(hr), hresultError("d3d11: CreateSamplerState", hr)
	}
	return h, 0, nil
}

func (n *d3d11NativeOps) CreatePipeline(
	desc *driver.GPUPipelineDesc,
	compiled d3dCompiledHLSL,
	elements []d3dInputElementSpec,
	rs d3d11RasterizerDesc,
	dss d3d11DepthStencilDesc,
	bs d3d11BlendDesc,
) (d3dReleaser, driver.GPUErrorCode, uint32, error) {
	g, err := devices()
	if err != nil {
		return nil, driver.GPUErrorFailed, 0, err
	}
	vsBC, diag := compileHLSLDetailed(compiled.VertexSource, compiled.VertexEntry, "vs_5_0")
	if diag != "" {
		return nil, driver.GPUErrorShaderCompile, 0, fmt.Errorf("d3d11: vertex HLSL compile: %s", diag)
	}
	var psBC []byte
	if compiled.PixelSource != "" {
		psBC, diag = compileHLSLDetailed(compiled.PixelSource, compiled.PixelEntry, "ps_5_0")
		if diag != "" {
			return nil, driver.GPUErrorShaderCompile, 0, fmt.Errorf("d3d11: pixel HLSL compile: %s", diag)
		}
	}

	h := &d3d11PipelineHandle{}
	if hr := g.d3d.call(d3dCreateVertexShader, uintptr(unsafe.Pointer(&vsBC[0])), uintptr(len(vsBC)), 0, uintptr(unsafe.Pointer(&h.vs))); failed(hr) {
		return nil, driver.GPUErrorFailed, uint32(hr), hresultError("d3d11: CreateVertexShader", hr)
	}
	if len(psBC) > 0 {
		if hr := g.d3d.call(d3dCreatePixelShader, uintptr(unsafe.Pointer(&psBC[0])), uintptr(len(psBC)), 0, uintptr(unsafe.Pointer(&h.ps))); failed(hr) {
			h.Release()
			return nil, driver.GPUErrorFailed, uint32(hr), hresultError("d3d11: CreatePixelShader", hr)
		}
	}
	if len(elements) > 0 {
		nativeElems := make([]d3d11InputElementDesc, len(elements))
		for i, e := range elements {
			nativeElems[i] = d3d11InputElementDesc{
				semanticName:         &d3dTexcoordSemantic[0],
				semanticIndex:        e.semanticIndex,
				format:               e.format,
				inputSlot:            e.inputSlot,
				alignedByteOffset:    e.alignedByteOffset,
				inputSlotClass:       e.inputSlotClass,
				instanceDataStepRate: e.instanceDataStepRate,
			}
		}
		if hr := g.d3d.call(
			d3dCreateInputLayout,
			uintptr(unsafe.Pointer(&nativeElems[0])),
			uintptr(len(nativeElems)),
			uintptr(unsafe.Pointer(&vsBC[0])),
			uintptr(len(vsBC)),
			uintptr(unsafe.Pointer(&h.inputLayout)),
		); failed(hr) {
			h.Release()
			return nil, driver.GPUErrorInvalidDescriptor, uint32(hr), hresultError("d3d11: CreateInputLayout", hr)
		}
	}
	if hr := g.d3d.call(d3dCreateRasterizerState, uintptr(unsafe.Pointer(&rs)), uintptr(unsafe.Pointer(&h.rs))); failed(hr) {
		h.Release()
		return nil, driver.GPUErrorFailed, uint32(hr), hresultError("d3d11: CreateRasterizerState", hr)
	}
	if hr := g.d3d.call(d3dCreateDepthStencilState, uintptr(unsafe.Pointer(&dss)), uintptr(unsafe.Pointer(&h.dss))); failed(hr) {
		h.Release()
		return nil, driver.GPUErrorFailed, uint32(hr), hresultError("d3d11: CreateDepthStencilState", hr)
	}
	if hr := g.d3d.call(d3dCreateBlendState, uintptr(unsafe.Pointer(&bs)), uintptr(unsafe.Pointer(&h.bs))); failed(hr) {
		h.Release()
		return nil, driver.GPUErrorFailed, uint32(hr), hresultError("d3d11: CreateBlendState", hr)
	}
	return h, driver.GPUErrorNone, 0, nil
}

func (n *d3d11NativeOps) ExecuteCopy(cp driver.GPUCopyOp, lookup *d3dPassLookup) (uint32, error) {
	g, ctx, err := n.getDevices()
	if err != nil {
		return 0, err
	}
	defer ctx.release()

	switch cp.Kind {
	case driver.GPUCopyBufferToBuffer:
		sb := lookup.buffers[cp.SrcID]
		db := lookup.buffers[cp.DstID]
		if sb == nil || db == nil || cp.Size == 0 {
			return 0, nil
		}
		sh, _ := sb.handle.(*d3d11BufferHandle)
		dh, _ := db.handle.(*d3d11BufferHandle)
		if sh == nil || dh == nil {
			return 0, nil
		}
		if dh.buf != nil && cp.DstOffset+cp.Size <= uint64(db.mainDesc.byteWidth) {
			if sh.buf != nil && cp.SrcOffset+cp.Size <= uint64(sb.mainDesc.byteWidth) {
				box := d3d11Box{
					left:   uint32(cp.SrcOffset),
					right:  uint32(cp.SrcOffset + cp.Size),
					top:    0,
					bottom: 1,
					front:  0,
					back:   1,
				}
				ctx.call(ctxCopySubresourceRegion, dh.buf.ptr(), 0, uintptr(cp.DstOffset), 0, 0, sh.buf.ptr(), 0, uintptr(unsafe.Pointer(&box)))
			} else if len(sb.cbShadow) > 0 && cp.SrcOffset+cp.Size <= uint64(len(sb.cbShadow)) {
				// D3D11 forbids CopySubresourceRegion from a constant buffer; upload from its CPU shadow instead.
				dbox := d3d11Box{
					left:   uint32(cp.DstOffset),
					right:  uint32(cp.DstOffset + cp.Size),
					top:    0,
					bottom: 1,
					front:  0,
					back:   1,
				}
				ctx.call(ctxUpdateSubresource, dh.buf.ptr(), 0, uintptr(unsafe.Pointer(&dbox)), uintptr(unsafe.Pointer(&sb.cbShadow[cp.SrcOffset])), 0, 0)
			}
		}
		if dh.cb != nil {
			// Mirror into shadow constant buffer if source has shadow data.
			if len(sb.cbShadow) > 0 && cp.SrcOffset+cp.Size <= uint64(len(sb.cbShadow)) && cp.DstOffset+cp.Size <= uint64(len(db.cbShadow)) {
				copy(db.cbShadow[cp.DstOffset:cp.DstOffset+cp.Size], sb.cbShadow[cp.SrcOffset:cp.SrcOffset+cp.Size])
				ctx.call(ctxUpdateSubresource, dh.cb.ptr(), 0, 0, uintptr(unsafe.Pointer(&db.cbShadow[0])), 0, 0)
			}
		}

	case driver.GPUCopyTextureToTexture:
		st := lookup.textures[cp.SrcID]
		dt := lookup.textures[cp.DstID]
		if st == nil || dt == nil {
			return 0, nil
		}
		sh, _ := st.handle.(*d3d11TextureHandle)
		dh, _ := dt.handle.(*d3d11TextureHandle)
		if sh == nil || dh == nil || sh.tex == nil || dh.tex == nil {
			return 0, nil
		}
		layers := max(cp.Layers, 1)
		box := d3d11Box{
			left:   cp.SrcTex.OriginX,
			top:    cp.SrcTex.OriginY,
			front:  0,
			right:  cp.SrcTex.OriginX + cp.Width,
			bottom: cp.SrcTex.OriginY + cp.Height,
			back:   1,
		}
		for l := uint32(0); l < layers; l++ {
			srcSub := d3dCalcSubresource(cp.SrcTex.MipLevel, cp.SrcTex.ArrayLayer+l, st.texDesc.mipLevels)
			dstSub := d3dCalcSubresource(cp.DstTex.MipLevel, cp.DstTex.ArrayLayer+l, dt.texDesc.mipLevels)
			ctx.call(ctxCopySubresourceRegion, dh.tex.ptr(), uintptr(dstSub), uintptr(cp.DstTex.OriginX), uintptr(cp.DstTex.OriginY), 0,
				sh.tex.ptr(), uintptr(srcSub), uintptr(unsafe.Pointer(&box)))
		}

	case driver.GPUCopyBufferToTexture:
		sb := lookup.buffers[cp.SrcID]
		dt := lookup.textures[cp.DstID]
		if sb == nil || dt == nil || cp.Width == 0 || cp.Height == 0 {
			return 0, nil
		}
		sh, _ := sb.handle.(*d3d11BufferHandle)
		dh, _ := dt.handle.(*d3d11TextureHandle)
		if sh == nil || dh == nil || dh.tex == nil {
			return 0, nil
		}
		bpp := dt.desc.Format.BytesPerPixel()
		if bpp == 0 {
			return 0, nil
		}
		rowPitch := cp.SrcLayout.BytesPerRow
		if rowPitch == 0 {
			rowPitch = cp.Width * bpp
		}
		rowsPerImage := cp.SrcLayout.RowsPerImage
		if rowsPerImage == 0 {
			rowsPerImage = cp.Height
		}
		layers := max(cp.Layers, 1)
		sliceBytes := rowPitch * rowsPerImage
		copyBytes := sliceBytes * layers
		if copyBytes == 0 {
			return 0, nil
		}
		dbox := d3d11Box{
			left:   cp.DstTex.OriginX,
			top:    cp.DstTex.OriginY,
			front:  0,
			right:  cp.DstTex.OriginX + cp.Width,
			bottom: cp.DstTex.OriginY + cp.Height,
			back:   1,
		}
		// Uniform-only buffers cannot use CopySubresourceRegion; upload directly from cbShadow.
		if sh.buf == nil {
			if cp.SrcLayout.Offset+uint64(copyBytes) > uint64(len(sb.cbShadow)) {
				return 0, nil
			}
			for l := uint32(0); l < layers; l++ {
				dstSub := d3dCalcSubresource(cp.DstTex.MipLevel, cp.DstTex.ArrayLayer+l, dt.texDesc.mipLevels)
				off := cp.SrcLayout.Offset + uint64(l*sliceBytes)
				ctx.call(ctxUpdateSubresource, dh.tex.ptr(), uintptr(dstSub), uintptr(unsafe.Pointer(&dbox)),
					uintptr(unsafe.Pointer(&sb.cbShadow[off])), uintptr(rowPitch), 0)
			}
			return 0, nil
		}
		if cp.SrcLayout.Offset+uint64(copyBytes) > uint64(sb.mainDesc.byteWidth) {
			return 0, nil
		}
		stageDesc := d3d11BufferDesc{
			byteWidth:      copyBytes,
			usage:          d3d11UsageStaging,
			cpuAccessFlags: d3d11CPUAccessRead,
		}
		var stage *comObject
		if hr := g.d3d.call(d3dCreateBuffer, uintptr(unsafe.Pointer(&stageDesc)), 0, uintptr(unsafe.Pointer(&stage))); failed(hr) {
			return uint32(hr), hresultError("d3d11: CreateBuffer staging", hr)
		}
		defer stage.release()
		sbox := d3d11Box{
			left:   uint32(cp.SrcLayout.Offset),
			right:  uint32(cp.SrcLayout.Offset) + copyBytes,
			top:    0,
			bottom: 1,
			front:  0,
			back:   1,
		}
		ctx.call(ctxCopySubresourceRegion, stage.ptr(), 0, 0, 0, 0, sh.buf.ptr(), 0, uintptr(unsafe.Pointer(&sbox)))
		var mapped d3d11MappedSubresource
		if hr := ctx.call(ctxMap, stage.ptr(), 0, uintptr(d3d11MapRead), 0, uintptr(unsafe.Pointer(&mapped))); failed(hr) {
			return uint32(hr), hresultError("d3d11: Map staging buffer", hr)
		}
		bytes := unsafe.Slice(mapped.pData, copyBytes)
		for l := uint32(0); l < layers; l++ {
			dstSub := d3dCalcSubresource(cp.DstTex.MipLevel, cp.DstTex.ArrayLayer+l, dt.texDesc.mipLevels)
			off := l * sliceBytes
			ctx.call(ctxUpdateSubresource, dh.tex.ptr(), uintptr(dstSub), uintptr(unsafe.Pointer(&dbox)),
				uintptr(unsafe.Pointer(&bytes[off])), uintptr(rowPitch), 0)
		}
		ctx.call(ctxUnmap, stage.ptr(), 0)
	}
	return 0, nil
}

type d3d11BoundVB struct {
	id     driver.GPUID
	offset uint32
}

func (n *d3d11NativeOps) ExecutePass(pass driver.GPUPass, swapHandle d3dReleaser, swapW, swapH uint32, lookup *d3dPassLookup) (bool, uint32, error) {
	g, ctx, err := n.getDevices()
	if err != nil {
		return false, 0, err
	}
	defer ctx.release()

	swap, _ := swapHandle.(*d3d11SwapTargets)
	var rtvs [d3d11MaxSimultaneousRenderTgts]uintptr
	numRTVs := 0
	var rtW, rtH uint32
	drewSwap := false
	needSwapResolve := false

	type resolveItem struct {
		dst, src       *comObject
		dstSub, srcSub uint32
		format         uint32
	}
	var resolves []resolveItem

	for i, ca := range pass.ColorAttachments {
		if i >= d3d11MaxSimultaneousRenderTgts {
			break
		}
		var rtvPtr uintptr
		if ca.Texture == 0 {
			drewSwap = true
			if swap != nil {
				if swap.msaaRTV != nil {
					rtvPtr = swap.msaaRTV.ptr()
					needSwapResolve = true
				} else if swap.swapRTV != nil {
					rtvPtr = swap.swapRTV.ptr()
				}
			}
			if rtW == 0 {
				rtW, rtH = swapW, swapH
			}
		} else if tex := lookup.textures[ca.Texture]; tex != nil {
			if th, ok := tex.handle.(*d3d11TextureHandle); ok && th != nil {
				if rtv := th.rtvFor(g.d3d, tex.desc, ca.MipLevel, ca.ArrayLayer); rtv != nil {
					rtvPtr = rtv.ptr()
				}
				if rtW == 0 {
					rtW = max(tex.texDesc.width>>ca.MipLevel, 1)
					rtH = max(tex.texDesc.height>>ca.MipLevel, 1)
				}
				if ca.ResolveTarget != 0 {
					if rtex := lookup.textures[ca.ResolveTarget]; rtex != nil {
						if rth, ok := rtex.handle.(*d3d11TextureHandle); ok && rth != nil && rth.tex != nil && th.tex != nil {
							resolves = append(resolves, resolveItem{
								dst:    rth.tex,
								src:    th.tex,
								dstSub: d3dCalcSubresource(ca.MipLevel, ca.ArrayLayer, rtex.texDesc.mipLevels),
								srcSub: d3dCalcSubresource(ca.MipLevel, ca.ArrayLayer, tex.texDesc.mipLevels),
								format: rtex.texDesc.format,
							})
						}
					}
				}
			}
		}
		rtvs[i] = rtvPtr
		numRTVs = i + 1
		if rtvPtr != 0 && ca.LoadOp == driver.GPULoadClear {
			col := colorOf(ca.ClearColor)
			clearRGBA := [4]float32{col.r, col.g, col.b, col.a}
			ctx.call(ctxClearRenderTargetView, rtvPtr, uintptr(unsafe.Pointer(&clearRGBA[0])))
		}
	}

	var dsvPtr uintptr
	var depthFmt driver.GPUTextureFormat
	if ds := pass.DepthStencil; ds != nil {
		if ds.Texture == 0 {
			if swap != nil && swap.depthDSV != nil {
				dsvPtr = swap.depthDSV.ptr()
				depthFmt = swap.depthFmt
			}
			if rtW == 0 {
				rtW, rtH = swapW, swapH
			}
		} else if tex := lookup.textures[ds.Texture]; tex != nil {
			if th, ok := tex.handle.(*d3d11TextureHandle); ok && th != nil && th.dsv != nil {
				dsvPtr = th.dsv.ptr()
				depthFmt = tex.desc.Format
			}
			if rtW == 0 {
				rtW, rtH = tex.texDesc.width, tex.texDesc.height
			}
		}
		if dsvPtr != 0 {
			var clearFlags uint32
			if ds.DepthLoadOp == driver.GPULoadClear {
				clearFlags |= d3d11ClearDepth
			}
			if ds.StencilLoadOp == driver.GPULoadClear && depthFmt == driver.GPUFormatDepth24PlusStencil8 {
				clearFlags |= d3d11ClearStencil
			}
			if clearFlags != 0 {
				// ClearDepthStencilView(pDSV, ClearFlags, float Depth, uint8 Stencil).
				ctx.callF(ctxClearDepthStencilView, 0b0100, dsvPtr, uintptr(clearFlags), f32(ds.ClearDepth), uintptr(uint8(ds.ClearStencil)))
			}
		}
	}

	if rtW == 0 {
		rtW, rtH = swapW, swapH
	}
	if numRTVs > 0 {
		ctx.call(ctxOMSetRenderTargets, uintptr(numRTVs), uintptr(unsafe.Pointer(&rtvs[0])), dsvPtr)
	} else if dsvPtr != 0 {
		ctx.call(ctxOMSetRenderTargets, 0, 0, dsvPtr)
	}

	// Set default full-target viewport and scissor rect (ScissorEnable is always TRUE).
	vp := d3d11Viewport{x: 0, y: 0, w: float32(rtW), h: float32(rtH), minDepth: 0, maxDepth: 1}
	ctx.call(ctxRSSetViewports, 1, uintptr(unsafe.Pointer(&vp)))
	sc := d3d11Rect{left: 0, top: 0, right: int32(rtW), bottom: int32(rtH)}
	ctx.call(ctxRSSetScissorRects, 1, uintptr(unsafe.Pointer(&sc)))

	var (
		activePipe  *d3dTrackedPipeline
		activePH    *d3d11PipelineHandle
		boundGroups [8]*d3dTrackedBindGroup
		boundVBs    [8]d3d11BoundVB
		vbDirty     bool
		bgDirty     bool
		stencilRef  uint32
		blendFactor = [4]float32{0, 0, 0, 0}
		maxVSSRV    uint32
		maxPSSRV    uint32
	)

	syncDrawState := func() bool {
		if activePipe == nil || activePH == nil {
			return false
		}
		if vbDirty {
			vbDirty = false
			for slot, stride := range activePipe.vbStrides {
				if slot >= len(boundVBs) {
					break
				}
				vb := boundVBs[slot]
				if vb.id == 0 {
					continue
				}
				tb := lookup.buffers[vb.id]
				if tb == nil {
					continue
				}
				bh, _ := tb.handle.(*d3d11BufferHandle)
				if bh == nil || bh.buf == nil {
					continue
				}
				ptr := bh.buf.ptr()
				st := stride
				off := vb.offset
				ctx.call(ctxIASetVertexBuffers, uintptr(slot), 1,
					uintptr(unsafe.Pointer(&ptr)),
					uintptr(unsafe.Pointer(&st)),
					uintptr(unsafe.Pointer(&off)))
			}
		}
		if bgDirty {
			bgDirty = false
			for _, rb := range activePipe.vsRegs {
				e, ok := d3dResolveBinding(rb, boundGroups)
				if !ok {
					continue
				}
				switch rb.Kind {
				case shaderir.BindingUniformBuffer:
					if tb := lookup.buffers[e.Buffer]; tb != nil {
						if bh, _ := tb.handle.(*d3d11BufferHandle); bh != nil && bh.cb != nil {
							ptr := bh.cb.ptr()
							ctx.call(ctxVSSetConstantBuffers, uintptr(rb.Register), 1, uintptr(unsafe.Pointer(&ptr)))
						}
					}
				case shaderir.BindingTexture:
					if tt := lookup.textures[e.Texture]; tt != nil {
						if th, _ := tt.handle.(*d3d11TextureHandle); th != nil && th.srv != nil {
							ptr := th.srv.ptr()
							ctx.call(ctxVSSetShaderResources, uintptr(rb.Register), 1, uintptr(unsafe.Pointer(&ptr)))
							maxVSSRV = max(maxVSSRV, rb.Register+1)
						}
					}
				case shaderir.BindingStorageBufferRead:
					if tb := lookup.buffers[e.Buffer]; tb != nil {
						if bh, _ := tb.handle.(*d3d11BufferHandle); bh != nil && bh.srv != nil {
							ptr := bh.srv.ptr()
							ctx.call(ctxVSSetShaderResources, uintptr(rb.Register), 1, uintptr(unsafe.Pointer(&ptr)))
							maxVSSRV = max(maxVSSRV, rb.Register+1)
						}
					}
				case shaderir.BindingSampler:
					if ts := lookup.samplers[e.Sampler]; ts != nil {
						if sh, _ := ts.handle.(*d3d11SamplerHandle); sh != nil && sh.state != nil {
							ptr := sh.state.ptr()
							ctx.call(ctxVSSetSamplers, uintptr(rb.Register), 1, uintptr(unsafe.Pointer(&ptr)))
						}
					}
				}
			}
			for _, rb := range activePipe.psRegs {
				e, ok := d3dResolveBinding(rb, boundGroups)
				if !ok {
					continue
				}
				switch rb.Kind {
				case shaderir.BindingUniformBuffer:
					if tb := lookup.buffers[e.Buffer]; tb != nil {
						if bh, _ := tb.handle.(*d3d11BufferHandle); bh != nil && bh.cb != nil {
							ptr := bh.cb.ptr()
							ctx.call(ctxPSSetConstantBuffers, uintptr(rb.Register), 1, uintptr(unsafe.Pointer(&ptr)))
						}
					}
				case shaderir.BindingTexture:
					if tt := lookup.textures[e.Texture]; tt != nil {
						if th, _ := tt.handle.(*d3d11TextureHandle); th != nil && th.srv != nil {
							ptr := th.srv.ptr()
							ctx.call(ctxPSSetShaderResources, uintptr(rb.Register), 1, uintptr(unsafe.Pointer(&ptr)))
							maxPSSRV = max(maxPSSRV, rb.Register+1)
						}
					}
				case shaderir.BindingStorageBufferRead:
					if tb := lookup.buffers[e.Buffer]; tb != nil {
						if bh, _ := tb.handle.(*d3d11BufferHandle); bh != nil && bh.srv != nil {
							ptr := bh.srv.ptr()
							ctx.call(ctxPSSetShaderResources, uintptr(rb.Register), 1, uintptr(unsafe.Pointer(&ptr)))
							maxPSSRV = max(maxPSSRV, rb.Register+1)
						}
					}
				case shaderir.BindingSampler:
					if ts := lookup.samplers[e.Sampler]; ts != nil {
						if sh, _ := ts.handle.(*d3d11SamplerHandle); sh != nil && sh.state != nil {
							ptr := sh.state.ptr()
							ctx.call(ctxPSSetSamplers, uintptr(rb.Register), 1, uintptr(unsafe.Pointer(&ptr)))
						}
					}
				}
			}
		}
		return true
	}

	for _, cmd := range pass.Commands {
		switch cmd.Kind {
		case driver.GPUCmdSetViewport:
			vp := d3d11Viewport{x: cmd.F0, y: cmd.F1, w: cmd.F2, h: cmd.F3, minDepth: cmd.F4, maxDepth: cmd.F5}
			ctx.call(ctxRSSetViewports, 1, uintptr(unsafe.Pointer(&vp)))

		case driver.GPUCmdSetScissor:
			w := max(cmd.I2, 0)
			h := max(cmd.I3, 0)
			r := d3d11Rect{left: cmd.I0, top: cmd.I1, right: cmd.I0 + w, bottom: cmd.I1 + h}
			ctx.call(ctxRSSetScissorRects, 1, uintptr(unsafe.Pointer(&r)))

		case driver.GPUCmdSetPipeline:
			tp := lookup.pipelines[cmd.ID]
			if tp == nil {
				activePipe, activePH = nil, nil
				continue
			}
			ph, _ := tp.handle.(*d3d11PipelineHandle)
			if ph == nil {
				activePipe, activePH = nil, nil
				continue
			}
			activePipe, activePH = tp, ph
			var ilPtr, psPtr uintptr
			if ph.inputLayout != nil {
				ilPtr = ph.inputLayout.ptr()
			}
			if ph.ps != nil {
				psPtr = ph.ps.ptr()
			}
			ctx.call(ctxIASetInputLayout, ilPtr)
			ctx.call(ctxIASetPrimitiveTopology, uintptr(tp.topology))
			ctx.call(ctxVSSetShader, ph.vs.ptr(), 0, 0)
			ctx.call(ctxPSSetShader, psPtr, 0, 0)
			ctx.call(ctxRSSetState, ph.rs.ptr())
			ctx.call(ctxOMSetDepthStencilState, ph.dss.ptr(), uintptr(stencilRef))
			ctx.call(ctxOMSetBlendState, ph.bs.ptr(), uintptr(unsafe.Pointer(&blendFactor[0])), 0xffffffff)
			vbDirty, bgDirty = true, true

		case driver.GPUCmdSetBindGroup:
			if cmd.Slot < uint32(len(boundGroups)) {
				boundGroups[cmd.Slot] = lookup.bindGroups[cmd.ID]
				bgDirty = true
			}

		case driver.GPUCmdSetVertexBuffer:
			if cmd.Slot < uint32(len(boundVBs)) {
				boundVBs[cmd.Slot] = d3d11BoundVB{id: cmd.ID, offset: uint32(cmd.Offset)}
				vbDirty = true
			}

		case driver.GPUCmdSetIndexBuffer:
			if tb := lookup.buffers[cmd.ID]; tb != nil {
				if bh, _ := tb.handle.(*d3d11BufferHandle); bh != nil && bh.buf != nil {
					ctx.call(ctxIASetIndexBuffer, bh.buf.ptr(), uintptr(d3dIndexFormatDXGI(cmd.IndexFormat)), uintptr(uint32(cmd.Offset)))
				}
			}

		case driver.GPUCmdSetStencilReference:
			stencilRef = cmd.U0
			if activePH != nil && activePH.dss != nil {
				ctx.call(ctxOMSetDepthStencilState, activePH.dss.ptr(), uintptr(stencilRef))
			}

		case driver.GPUCmdSetBlendConstant:
			blendFactor = [4]float32{cmd.F0, cmd.F1, cmd.F2, cmd.F3}
			if activePH != nil && activePH.bs != nil {
				ctx.call(ctxOMSetBlendState, activePH.bs.ptr(), uintptr(unsafe.Pointer(&blendFactor[0])), 0xffffffff)
			}

		case driver.GPUCmdDraw:
			if syncDrawState() {
				ctx.call(ctxDraw, uintptr(cmd.U0), uintptr(cmd.U1))
			}

		case driver.GPUCmdDrawIndexed:
			if syncDrawState() {
				ctx.call(ctxDrawIndexed, uintptr(cmd.U0), uintptr(cmd.U1), uintptr(uint32(cmd.I0)))
			}

		case driver.GPUCmdDrawInstanced:
			if syncDrawState() {
				ctx.call(ctxDrawInstanced, uintptr(cmd.U0), uintptr(cmd.U1), uintptr(cmd.U2), uintptr(cmd.U3))
			}

		case driver.GPUCmdDrawIndexedInstanced:
			if syncDrawState() {
				ctx.call(ctxDrawIndexedInstanced, uintptr(cmd.U0), uintptr(cmd.U1), uintptr(cmd.U2), uintptr(uint32(cmd.I0)), uintptr(cmd.U3))
			}
		}
	}

	// Unbind render targets, SRVs, and pipeline states so Direct2D, shaderView, glassView, and subsequent passes never see stale D3D11 state or RTV/SRV hazards.
	ctx.call(ctxOMSetRenderTargets, 0, 0, 0)
	ctx.call(ctxRSSetState, 0)
	ctx.call(ctxOMSetDepthStencilState, 0, 0)
	ctx.call(ctxOMSetBlendState, 0, 0, 0xffffffff)
	if maxVSSRV > 0 {
		zeros := make([]uintptr, maxVSSRV)
		ctx.call(ctxVSSetShaderResources, 0, uintptr(maxVSSRV), uintptr(unsafe.Pointer(&zeros[0])))
	}
	if maxPSSRV > 0 {
		zeros := make([]uintptr, maxPSSRV)
		ctx.call(ctxPSSetShaderResources, 0, uintptr(maxPSSRV), uintptr(unsafe.Pointer(&zeros[0])))
	}

	// Resolve MSAA render targets at pass end.
	if needSwapResolve && swap != nil && swap.swapTex != nil && swap.msaaTex != nil {
		ctx.call(ctxResolveSubresource, swap.swapTex.ptr(), 0, swap.msaaTex.ptr(), 0, uintptr(swap.dxgiFmt))
	}
	for _, r := range resolves {
		ctx.call(ctxResolveSubresource, r.dst.ptr(), uintptr(r.dstSub), r.src.ptr(), uintptr(r.srcSub), uintptr(r.format))
	}
	if drewSwap && swap != nil && swap.hdrSwap != nil {
		swap.presentHDR(ctx)
	}

	return drewSwap, 0, nil
}

func (n *d3d11NativeOps) ReadTexture(swapHandle d3dReleaser, swapW, swapH uint32, tex *d3dTrackedTexture, region driver.GPUTextureRegion, surface bool) ([]byte, uint32, error) {
	g, ctx, err := n.getDevices()
	if err != nil {
		return nil, 0, err
	}
	defer ctx.release()

	var (
		srcTex     *comObject
		srcFmt     driver.GPUTextureFormat
		srcDXGI    uint32
		mipW, mipH uint32
		mipLevels  uint32 = 1
	)
	if surface {
		swap, _ := swapHandle.(*d3d11SwapTargets)
		if swap == nil || swap.swapTex == nil {
			return nil, 0, errors.New("d3d11: swapchain texture unavailable for readback")
		}
		srcTex = swap.swapTex
		srcFmt = swap.format
		if srcFmt == driver.GPUFormatUndefined {
			srcFmt = driver.GPUFormatBGRA8Unorm
		}
		srcDXGI = swap.dxgiFmt
		if srcDXGI == 0 {
			srcDXGI = dxgiFmtB8G8R8A8Unorm
		}
		mipW, mipH = swapW, swapH
	} else {
		if tex == nil {
			return nil, 0, errors.New("d3d11: texture unavailable for readback")
		}
		th, _ := tex.handle.(*d3d11TextureHandle)
		if th == nil || th.tex == nil {
			return nil, 0, errors.New("d3d11: texture handle unavailable for readback")
		}
		if tex.texDesc.sampleCount > 1 {
			return nil, 0, errors.New("d3d11: ReadTexture requires a non-MSAA texture")
		}
		srcTex = th.tex
		srcFmt = tex.desc.Format
		srcDXGI = tex.texDesc.format
		mipLevels = tex.texDesc.mipLevels
		mipW = max(tex.texDesc.width>>region.MipLevel, 1)
		mipH = max(tex.texDesc.height>>region.MipLevel, 1)
	}

	bpp := srcFmt.BytesPerPixel()
	if bpp == 0 {
		return nil, 0, errors.New("d3d11: ReadTexture only supports color formats")
	}
	x, y := region.X, region.Y
	if x >= mipW || y >= mipH {
		return nil, 0, errors.New("d3d11: ReadTexture region out of bounds")
	}
	w, h := region.Width, region.Height
	if w == 0 || x+w > mipW {
		w = mipW - x
	}
	if h == 0 || y+h > mipH {
		h = mipH - y
	}

	stageDesc := d3d11Texture2DDesc{
		width:       w,
		height:      h,
		mipLevels:   1,
		arraySize:   1,
		format:      srcDXGI,
		sampleCount: 1,
		usage:       d3d11UsageStaging,
		cpuAccess:   d3d11CPUAccessRead,
	}
	var stage *comObject
	if hr := g.d3d.call(d3dCreateTexture2D, uintptr(unsafe.Pointer(&stageDesc)), 0, uintptr(unsafe.Pointer(&stage))); failed(hr) {
		return nil, uint32(hr), hresultError("d3d11: CreateTexture2D staging readback", hr)
	}
	defer stage.release()

	srcSub := d3dCalcSubresource(region.MipLevel, region.ArrayLayer, mipLevels)
	box := d3d11Box{left: x, top: y, front: 0, right: x + w, bottom: y + h, back: 1}
	ctx.call(ctxCopySubresourceRegion, stage.ptr(), 0, 0, 0, 0, srcTex.ptr(), uintptr(srcSub), uintptr(unsafe.Pointer(&box)))

	var mapped d3d11MappedSubresource
	if hr := ctx.call(ctxMap, stage.ptr(), 0, uintptr(d3d11MapRead), 0, uintptr(unsafe.Pointer(&mapped))); failed(hr) {
		return nil, uint32(hr), hresultError("d3d11: Map staging readback", hr)
	}
	srcBytes := unsafe.Slice(mapped.pData, int(mapped.rowPitch)*int(h))
	out := make([]byte, int(w*h*bpp))
	if srcFmt == driver.GPUFormatBGRA8Unorm {
		d3dSwizzleBGRAtoRGBA(out, srcBytes, w, h, mapped.rowPitch)
	} else {
		d3dCopyPitchedRows(out, srcBytes, w, h, bpp, mapped.rowPitch)
	}
	ctx.call(ctxUnmap, stage.ptr(), 0)
	return out, 0, nil
}

// --- Window integration for KindGPU views ---

// gpuView holds one KindGPU view's D3D11 controller on the main UI thread.
type gpuView struct {
	w        *window
	v        *view
	ctrl     *d3dGPUController
	portal   *portal    // DirectComposition visual when presenting an HDR IDXGISwapChain3
	boundRef *comObject // swapchain last bound to portal.inner
}

func (w *window) newGPUView(v *view) *gpuView {
	gv := &gpuView{w: w, v: v}
	ops := &d3d11NativeOps{scaleFn: func() float32 { return float32(w.scale) }}
	gv.ctrl = newD3DGPUController(ops, func(ev *driver.GPUEvent) {
		if w.engine != nil && !v.deleted {
			w.engine.Dispatch(driver.Event{Kind: driver.EventGPU, ID: v.id, Data: ev})
		}
	})
	if w.gpuViews == nil {
		w.gpuViews = map[driver.NodeID]*view{}
	}
	w.gpuViews[v.id] = v
	return gv
}

func (gv *gpuView) release() {
	if gv == nil {
		return
	}
	if gv.portal != nil {
		gv.portal.inner.call(visualSetContent, 0)
		gv.portal.release()
		gv.portal = nil
		gv.boundRef = nil
	}
	if gv.ctrl != nil {
		gv.ctrl.Release()
	}
	if gv.w != nil && gv.w.gpuViews != nil && gv.v != nil {
		delete(gv.w.gpuViews, gv.v.id)
	}
}

func (gv *gpuView) hdrSwap() *comObject {
	if gv == nil || gv.ctrl == nil {
		return nil
	}
	swap, _ := gv.ctrl.swap.(*d3d11SwapTargets)
	if swap == nil || swap.generation != gfxGeneration {
		return nil
	}
	return swap.hdrSwap
}

func (gv *gpuView) ensurePortal() *portal {
	if gv.hdrSwap() == nil {
		return nil
	}
	if gv.portal == nil && gv.w != nil {
		if p, err := gv.w.newPortal(); err == nil {
			gv.portal = p
		}
	}
	return gv.portal
}

func (gv *gpuView) composeAt(at *placement) {
	hdr := gv.hdrSwap()
	if at == nil || hdr == nil {
		if gv.portal != nil && gv.boundRef != nil {
			gv.portal.inner.call(visualSetContent, 0)
			gv.boundRef = nil
		}
		return
	}
	p := gv.ensurePortal()
	if p == nil {
		return
	}
	if gv.boundRef != hdr {
		p.inner.call(visualSetContent, hdr.ptr())
		gv.boundRef = hdr
	}
	p.place(*at)
}

func (w *window) gpuPixelSize(v *view) (uint32, uint32) {
	f := frameOf(v).Size
	scale := w.scale
	if scale <= 0 {
		scale = 1
	}
	pw := uint32(math.Ceil(float64(f.Width) * scale))
	ph := uint32(math.Ceil(float64(f.Height) * scale))
	return max(pw, 1), max(ph, 1)
}

func (w *window) configureGPU(v *view, cfg driver.GPUViewConfig) {
	if v.gpuView == nil {
		v.gpuView = w.newGPUView(v)
	}
	v.gpuView.ctrl.Configure(cfg)
}

func (w *window) syncGPUViews(configured, resized []*view) {
	seen := map[driver.NodeID]bool{}
	for _, v := range configured {
		if v == nil || v.deleted || v.kind != driver.KindGPU || v.gpuView == nil {
			continue
		}
		seen[v.id] = true
		pw, ph := w.gpuPixelSize(v)
		if v.gpuView.ctrl.MountOrResize(pw, ph) {
			v.gpuView.ensurePortal()
			w.redrawView(v)
		}
	}
	for _, v := range resized {
		if v == nil || v.deleted || v.kind != driver.KindGPU || v.gpuView == nil || seen[v.id] {
			continue
		}
		pw, ph := w.gpuPixelSize(v)
		if v.gpuView.ctrl.MountOrResize(pw, ph) {
			v.gpuView.ensurePortal()
			w.redrawView(v)
		}
	}
}

func (w *window) resizeGPUViews() {
	for _, v := range w.gpuViews {
		if v == nil || v.deleted || v.gpuView == nil {
			continue
		}
		pw, ph := w.gpuPixelSize(v)
		if v.gpuView.ctrl.MountOrResize(pw, ph) {
			w.redrawView(v)
		}
	}
}

// SubmitGPU queues an off-commit GPUFrame for execution on the window's UI thread without triggering a UI view-tree commit.
func (w *window) SubmitGPU(frame driver.GPUFrame) {
	w.gpuMu.Lock()
	wake := len(w.gpuQueue) == 0
	w.gpuQueue = append(w.gpuQueue, frame)
	w.gpuMu.Unlock()
	if wake && w.hwnd != 0 {
		postMessageW.Call(w.hwnd, wmGPU, 0, 0)
	}
}

func (w *window) processGPUFrames() {
	w.gpuMu.Lock()
	frames := w.gpuQueue
	w.gpuQueue = nil
	w.gpuMu.Unlock()

	for i, frame := range frames {
		v := w.view(frame.ViewID)
		if v == nil || v.deleted || v.gpuView == nil {
			completeGPUFrame(frame)
			// Complete any pending readbacks with an error if the view unmounted.
			for _, op := range frame.Ops {
				if op.Kind == driver.GPUOpReadTexture && w.engine != nil {
					w.engine.Dispatch(driver.Event{
						Kind: driver.EventGPU,
						ID:   frame.ViewID,
						Data: &driver.GPUEvent{
							Kind:       driver.GPUEventReadback,
							Generation: frame.Generation,
							ReadbackID: op.ReadbackID,
							Message:    "GPU view unmounted",
						},
					})
				}
			}
			continue
		}
		wasLost := v.gpuView.ctrl.lost
		if v.gpuView.ctrl.Submit(frame) {
			w.redrawView(v)
		}
		// Direct3D 11's immediate context has copied the frame's data, so its sender may reuse that memory (gpu.FrameFence).
		completeGPUFrame(frame)
		if !wasLost && v.gpuView.ctrl.lost {
			for _, rest := range frames[i+1:] {
				completeGPUFrame(rest)
			}
			// D3D11 device was removed during GPU submission; trigger window-wide recovery.
			w.lost()
			return
		}
	}
}

// completeGPUFrame runs frame's OnComplete, which releases the gpu.FrameFence its sender waits on.
func completeGPUFrame(frame driver.GPUFrame) {
	if frame.OnComplete != nil {
		frame.OnComplete()
	}
}

// notifyGPUDeviceLost releases all GPU view resources on w and reports GPUEventDeviceLost before mainGfx is torn down.
func (w *window) notifyGPUDeviceLost(hr uint32, msg string) {
	for _, v := range w.gpuViews {
		if v != nil && !v.deleted && v.gpuView != nil {
			v.gpuView.ctrl.NotifyDeviceLost(hr, msg)
		}
	}
}

// recoverGPUViews recreates swapchain targets for lost GPU views after a Direct3D 11 device reset and emits GPUEventDeviceReady with the new generation.
func (w *window) recoverGPUViews() {
	if _, err := devices(); err != nil {
		return
	}
	for _, v := range w.gpuViews {
		if v != nil && !v.deleted && v.gpuView != nil && v.gpuView.ctrl.lost {
			if v.gpuView.ctrl.Recover() {
				w.redrawView(v)
			}
		}
	}
}

// releaseAllGPUViews releases every GPU view on w during window teardown.
func (w *window) releaseAllGPUViews() {
	for _, v := range w.gpuViews {
		if v != nil && v.gpuView != nil {
			v.gpuView.release()
			v.gpuView = nil
		}
	}
	w.gpuViews = nil
	w.gpuMu.Lock()
	dropped := w.gpuQueue
	w.gpuQueue = nil
	w.gpuMu.Unlock()
	for _, frame := range dropped {
		completeGPUFrame(frame)
	}
}

// drawGPU composites a KindGPU view's D3D11 swapchain texture through Direct2D,
// or places its DirectComposition HDR swapchain portal when HDR output is active.
func (p *painter) drawGPU(v *view, world matrix, width, height, radius float32, clip bool) {
	gv := v.gpuView
	if gv == nil || gv.ctrl == nil || width <= 0 || height <= 0 {
		return
	}
	swap, _ := gv.ctrl.swap.(*d3d11SwapTargets)
	if swap == nil || swap.generation != gfxGeneration {
		return
	}
	if swap.hdrSwap != nil {
		if gv.ensurePortal() != nil {
			r := float32(0)
			if clip {
				r = radius
			}
			p.placeGPU(gv, world, width, height, r)
			return
		}
	}
	bmp := swap.ensureBitmap(p.dc, p.scale)
	if bmp == nil {
		return
	}
	dst := rectF{0, 0, width, height}
	p.dc.callF(rtDrawBitmap, 0b100, bmp.ptr(), uintptr(unsafe.Pointer(&dst)), f32(1), d2dInterpolationLinear, 0)
}
