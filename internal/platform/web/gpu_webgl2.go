//go:build js && wasm

package web

import (
	"fmt"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shaderir/glsl"
)

type gl2Buffer struct {
	glBuf  js.Value
	target uint32
	size   uint64
	desc   driver.GPUBufferDesc
}

type gl2Texture struct {
	glTex  js.Value
	glRB   js.Value
	isRB   bool
	target uint32
	info   glTextureFormatInfo
	desc   driver.GPUTextureDesc
}

type gl2Sampler struct {
	glSampler    js.Value
	glSamplerMip js.Value
	desc         driver.GPUSamplerDesc
}

type gl2BindGroup struct {
	layoutID driver.GPUID
	entries  map[uint32]driver.GPUBindGroupEntry
}

// gl2UniformBlockSlot and gl2SamplerPair mirror the GLSL writer's types
// without naming its package, so an app that draws no GPU view links none of
// the writer.
type gl2UniformBlockSlot struct {
	Group     uint32
	Binding   uint32
	BlockName string
	Slot      uint32
}

type gl2SamplerPair struct {
	Name           string
	Unit           uint32
	TextureGroup   uint32
	TextureBinding uint32
	SamplerGroup   uint32
	SamplerBinding uint32
}

type gl2Pipeline struct {
	prog          js.Value
	vao           js.Value
	desc          driver.GPUPipelineDesc
	uniformBlocks []gl2UniformBlockSlot
	samplerPairs  []gl2SamplerPair
}

// gl2Blocks copies the writer's block slots into gl2UniformBlockSlot.
func gl2Blocks(blocks []glsl.UniformBlockSlot) []gl2UniformBlockSlot {
	out := make([]gl2UniformBlockSlot, len(blocks))
	for i, b := range blocks {
		out[i] = gl2UniformBlockSlot{b.Group, b.Binding, b.BlockName, b.Slot}
	}
	return out
}

// gl2Pairs copies the writer's sampler pairs into gl2SamplerPair.
func gl2Pairs(pairs []glsl.SamplerPair) []gl2SamplerPair {
	out := make([]gl2SamplerPair, len(pairs))
	for i, sp := range pairs {
		out[i] = gl2SamplerPair{sp.Name, sp.Unit, sp.TextureGroup, sp.TextureBinding, sp.SamplerGroup, sp.SamplerBinding}
	}
	return out
}

type gl2BoundGroup struct {
	id             driver.GPUID
	dynamicOffsets []uint32
}

type gl2BoundVB struct {
	id     driver.GPUID
	offset uint64
}

type gl2BoundIB struct {
	id     driver.GPUID
	format driver.GPUIndexFormat
	offset uint64
}

type webgl2Backend struct {
	g  *gpuView
	gl js.Value

	width, height int
	hasDepth      bool
	hasStencil    bool
	anisoExt      js.Value
	maxAniso      float64

	passFBO           js.Value
	passFBOColorCount int
	readFBO           js.Value
	drawFBO           js.Value

	buffers    map[driver.GPUID]*gl2Buffer
	textures   map[driver.GPUID]*gl2Texture
	samplers   map[driver.GPUID]*gl2Sampler
	layouts    map[driver.GPUID]*driver.GPUBindGroupLayoutDesc
	bindGroups map[driver.GPUID]*gl2BindGroup
	pipelines  map[driver.GPUID]*gl2Pipeline

	canvasListeners listeners
	destroyed       bool
}

func (g *gpuView) initWebGL2() {
	if g.ctrl.state == gpuStateDestroyed {
		return
	}
	hasDepth := g.ctrl.cfg.DepthFormat.IsDepth()
	hasStencil := g.ctrl.cfg.DepthFormat == driver.GPUFormatDepth24PlusStencil8
	opts := map[string]any{
		"alpha":                 true,
		"premultipliedAlpha":    true,
		"depth":                 hasDepth,
		"stencil":               hasStencil,
		"antialias":             g.ctrl.cfg.SampleCount > 1,
		"preserveDrawingBuffer": true,
	}
	gl := g.canvas.Call("getContext", "webgl2", opts)
	if !gl.Truthy() {
		g.emitEvent(g.ctrl.makeError(driver.GPUErrorUnsupported, "neither WebGPU nor WebGL 2 is available"))
		return
	}

	b := &webgl2Backend{
		g:          g,
		gl:         gl,
		width:      max(g.ctrl.width, 1),
		height:     max(g.ctrl.height, 1),
		hasDepth:   hasDepth,
		hasStencil: hasStencil,
		buffers:    map[driver.GPUID]*gl2Buffer{},
		textures:   map[driver.GPUID]*gl2Texture{},
		samplers:   map[driver.GPUID]*gl2Sampler{},
		layouts:    map[driver.GPUID]*driver.GPUBindGroupLayoutDesc{},
		bindGroups: map[driver.GPUID]*gl2BindGroup{},
		pipelines:  map[driver.GPUID]*gl2Pipeline{},
	}
	b.probeExtensions()

	// Wire WebGL 2 context loss and automatic restoration.
	b.canvasListeners.on(g.canvas, "webglcontextlost", func(e js.Value) {
		if b.destroyed || g.ctrl.state == gpuStateDestroyed {
			return
		}
		// preventDefault() is required by the WebGL spec so the browser fires webglcontextrestored.
		e.Call("preventDefault")
		b.clearGLObjectMaps()
		if ev := g.ctrl.markLost(driver.GPULostReset, "WebGL 2 context lost"); ev != nil {
			g.emitEvent(ev)
		}
	})
	b.canvasListeners.on(g.canvas, "webglcontextrestored", func(js.Value) {
		if b.destroyed || g.ctrl.state == gpuStateDestroyed {
			return
		}
		b.probeExtensions()
		limits := b.readLimits()
		features := b.features()
		if ev := g.ctrl.markReady(gpuBackendWebGL2, driver.GPUFormatRGBA8Unorm, limits, features); ev != nil {
			g.emitEvent(ev)
		}
	})

	g.backend = b
	limits := b.readLimits()
	features := b.features()
	if ev := g.ctrl.markReady(gpuBackendWebGL2, driver.GPUFormatRGBA8Unorm, limits, features); ev != nil {
		g.emitEvent(ev)
	}
}

func (b *webgl2Backend) probeExtensions() {
	// Enable floating-point color render targets (RGBA16Float) when supported.
	b.gl.Call("getExtension", "EXT_color_buffer_float")
	b.gl.Call("getExtension", "EXT_color_buffer_half_float")
	b.anisoExt = b.gl.Call("getExtension", "EXT_texture_filter_anisotropic")
	if !b.anisoExt.Truthy() {
		b.anisoExt = b.gl.Call("getExtension", "WEBKIT_EXT_texture_filter_anisotropic")
	}
	if b.anisoExt.Truthy() {
		b.maxAniso = b.gl.Call("getParameter", b.anisoExt.Get("MAX_TEXTURE_MAX_ANISOTROPY_EXT")).Float()
	} else {
		b.maxAniso = 1
	}
}

func (b *webgl2Backend) readLimits() driver.GPULimits {
	var l driver.GPULimits
	if v := b.gl.Call("getParameter", int(glMaxTextureSize)); v.Type() == js.TypeNumber && v.Int() > 0 {
		l.MaxTextureDimension2D = uint32(v.Int())
	}
	if v := b.gl.Call("getParameter", int(glMax3DTextureSize)); v.Type() == js.TypeNumber && v.Int() > 0 {
		l.MaxTextureDimension3D = uint32(v.Int())
	} else {
		l.MaxTextureDimension3D = 256
	}
	if v := b.gl.Call("getParameter", int(glMaxUniformBlockSize)); v.Type() == js.TypeNumber && v.Float() > 0 {
		l.MaxUniformBufferSize = uint64(v.Float())
	}
	if v := b.gl.Call("getParameter", int(glMaxVertexAttribs)); v.Type() == js.TypeNumber && v.Int() > 0 {
		l.MaxVertexAttributes = uint32(v.Int())
	}
	if v := b.gl.Call("getParameter", int(glUniformBufOffsetAlign)); v.Type() == js.TypeNumber && v.Int() > 0 {
		l.MinUniformBufferAlignment = uint32(v.Int())
	}
	return clampGPULimits(l)
}

func (b *webgl2Backend) features() driver.GPUFeatures {
	return driver.GPUFeatures{
		StorageBuffers: false,
		Compute:        false,
		MSAA4x:         true,
		Depth32Float:   true,
		Anisotropy:     b.anisoExt.Truthy(),
		Texture3D:      true,
	}
}

func (b *webgl2Backend) resize(width, height int) {
	if b.destroyed {
		return
	}
	b.width = max(width, 1)
	b.height = max(height, 1)
}

func (b *webgl2Backend) prepareCapture() {}

func (b *webgl2Backend) ensurePassFBO() js.Value {
	if !b.passFBO.Truthy() {
		b.passFBO = b.gl.Call("createFramebuffer")
	}
	return b.passFBO
}

func (b *webgl2Backend) ensureReadFBO() js.Value {
	if !b.readFBO.Truthy() {
		b.readFBO = b.gl.Call("createFramebuffer")
	}
	return b.readFBO
}

func (b *webgl2Backend) ensureDrawFBO() js.Value {
	if !b.drawFBO.Truthy() {
		b.drawFBO = b.gl.Call("createFramebuffer")
	}
	return b.drawFBO
}

func (b *webgl2Backend) submit(frame driver.GPUFrame) {
	if b.destroyed || !b.gl.Truthy() {
		return
	}
	var readbacks []driver.GPUResourceOp
	for _, op := range frame.Ops {
		if op.Kind == driver.GPUOpReadTexture {
			readbacks = append(readbacks, op)
			continue
		}
		b.execOp(op)
	}
	for _, cp := range frame.Copies {
		b.execCopy(cp)
	}
	for _, pass := range frame.Passes {
		b.execPass(pass)
	}
	for _, rb := range readbacks {
		b.execReadTexture(rb)
	}
}

func (b *webgl2Backend) execOp(op driver.GPUResourceOp) {
	gl := b.gl
	switch op.Kind {
	case driver.GPUOpCreateBuffer:
		if op.Buffer == nil {
			return
		}
		desc := *op.Buffer
		target := glBufferTarget(desc.Usage)
		usage := glStaticDraw
		if desc.Usage&(driver.GPUBufferUniform|driver.GPUBufferCopyDst) != 0 {
			usage = glDynamicDraw
		}
		glBuf := gl.Call("createBuffer")
		gl.Call("bindBuffer", int(target), glBuf)
		if len(desc.Data) > 0 {
			arr := js.Global().Get("Uint8Array").New(len(desc.Data))
			js.CopyBytesToJS(arr, desc.Data)
			if uint64(len(desc.Data)) == desc.Size {
				gl.Call("bufferData", int(target), arr, int(usage))
			} else {
				gl.Call("bufferData", int(target), int(desc.Size), int(usage))
				gl.Call("bufferSubData", int(target), 0, arr)
			}
		} else {
			gl.Call("bufferData", int(target), int(desc.Size), int(usage))
		}
		gl.Call("bindBuffer", int(target), js.Null())
		b.buffers[op.ID] = &gl2Buffer{glBuf: glBuf, target: target, size: desc.Size, desc: desc}

	case driver.GPUOpWriteBuffer:
		buf, ok := b.buffers[op.ID]
		if !ok || len(op.Data) == 0 {
			return
		}
		arr := js.Global().Get("Uint8Array").New(len(op.Data))
		js.CopyBytesToJS(arr, op.Data)
		gl.Call("bindBuffer", int(buf.target), buf.glBuf)
		gl.Call("bufferSubData", int(buf.target), int(op.Offset), arr)
		gl.Call("bindBuffer", int(buf.target), js.Null())

	case driver.GPUOpDestroyBuffer:
		if buf, ok := b.buffers[op.ID]; ok {
			gl.Call("deleteBuffer", buf.glBuf)
			delete(b.buffers, op.ID)
		}

	case driver.GPUOpCreateTexture:
		if op.Texture == nil {
			return
		}
		desc := *op.Texture
		info := glFormatInfo(desc.Format)
		samples := max(desc.SampleCount, 1)
		mips := max(desc.MipLevels, 1)
		layers := max(desc.Layers, 1)
		if samples > 1 {
			rb := gl.Call("createRenderbuffer")
			gl.Call("bindRenderbuffer", int(glRenderbuffer), rb)
			gl.Call("renderbufferStorageMultisample", int(glRenderbuffer), samples, int(info.InternalFormat), int(desc.Width), int(desc.Height))
			gl.Call("bindRenderbuffer", int(glRenderbuffer), js.Null())
			b.textures[op.ID] = &gl2Texture{glRB: rb, isRB: true, info: info, desc: desc}
			return
		}
		target := glTextureTarget(desc.Dimension)
		glTex := gl.Call("createTexture")
		gl.Call("bindTexture", int(target), glTex)
		switch desc.Dimension {
		case driver.GPUDim2DArray:
			gl.Call("texStorage3D", int(target), int(mips), int(info.InternalFormat), int(desc.Width), int(desc.Height), int(layers))
		case driver.GPUDim3D:
			gl.Call("texStorage3D", int(target), int(mips), int(info.InternalFormat), int(desc.Width), int(desc.Height), int(max(desc.Depth, 1)))
		default:
			gl.Call("texStorage2D", int(target), int(mips), int(info.InternalFormat), int(desc.Width), int(desc.Height))
		}
		gl.Call("texParameteri", int(target), int(glTextureMinFilter), int(glNearest))
		gl.Call("texParameteri", int(target), int(glTextureMagFilter), int(glNearest))
		gl.Call("texParameteri", int(target), int(glTextureWrapS), int(glClampToEdge))
		gl.Call("texParameteri", int(target), int(glTextureWrapT), int(glClampToEdge))
		if desc.Dimension == driver.GPUDim3D || desc.Dimension == driver.GPUDimCube {
			gl.Call("texParameteri", int(target), int(glTextureWrapR), int(glClampToEdge))
		}
		gl.Call("bindTexture", int(target), js.Null())
		b.textures[op.ID] = &gl2Texture{glTex: glTex, target: target, info: info, desc: desc}

	case driver.GPUOpWriteTexture:
		tex, ok := b.textures[op.ID]
		if !ok || tex.isRB || len(op.Data) == 0 {
			return
		}
		off := int(op.DataLayout.Offset)
		if off >= len(op.Data) {
			return
		}
		src := op.Data[off:]
		bpp := tex.info.BytesPerPixel
		if bpp == 0 {
			bpp = 4
		}
		mipW := mipDim(tex.desc.Width, op.TextureDst.MipLevel)
		mipH := mipDim(tex.desc.Height, op.TextureDst.MipLevel)
		bpr := op.DataLayout.BytesPerRow
		if bpr == 0 {
			bpr = mipW * bpp
		}
		rows := op.DataLayout.RowsPerImage
		if rows == 0 {
			if bpr > 0 {
				rows = uint32(len(src)) / bpr
			}
			if rows == 0 {
				rows = mipH
			}
		}
		w := mipW - min(op.TextureDst.OriginX, mipW)
		if bpr/bpp < w && bpr >= bpp {
			w = bpr / bpp
		}
		h := min(rows, mipH-min(op.TextureDst.OriginY, mipH))
		if w == 0 || h == 0 {
			return
		}
		arr := js.Global().Get("Uint8Array").New(len(src))
		js.CopyBytesToJS(arr, src)

		gl.Call("bindTexture", int(tex.target), tex.glTex)
		gl.Call("pixelStorei", int(glUnpackFlipYWebGL), 0)
		gl.Call("pixelStorei", int(glUnpackPremultWebGL), 0)
		gl.Call("pixelStorei", int(glUnpackAlignment), 1)
		rowPixels := int(bpr / bpp)
		if rowPixels > int(w) {
			gl.Call("pixelStorei", int(glUnpackRowLength), rowPixels)
		} else {
			gl.Call("pixelStorei", int(glUnpackRowLength), 0)
		}
		switch tex.desc.Dimension {
		case driver.GPUDimCube:
			face := glTextureCubeMapPosX + op.TextureDst.ArrayLayer
			gl.Call("texSubImage2D", int(face), int(op.TextureDst.MipLevel),
				int(op.TextureDst.OriginX), int(op.TextureDst.OriginY), int(w), int(h),
				int(tex.info.Format), int(tex.info.Type), arr)
		case driver.GPUDim2DArray:
			gl.Call("texSubImage3D", int(glTexture2DArray), int(op.TextureDst.MipLevel),
				int(op.TextureDst.OriginX), int(op.TextureDst.OriginY), int(op.TextureDst.ArrayLayer),
				int(w), int(h), 1,
				int(tex.info.Format), int(tex.info.Type), arr)
		case driver.GPUDim3D:
			mipD := mipDim(max(tex.desc.Depth, 1), op.TextureDst.MipLevel)
			if op.TextureDst.OriginZ < mipD {
				sliceRows := op.DataLayout.RowsPerImage
				if sliceRows == 0 {
					sliceRows = h
				}
				sliceBytes := uint64(bpr) * uint64(max(sliceRows, 1))
				minSlice := uint64(h-1)*uint64(bpr) + uint64(w*bpp)
				if sliceBytes > 0 && uint64(len(src)) >= minSlice {
					d := min(mipD-op.TextureDst.OriginZ, uint32((uint64(len(src))-minSlice)/sliceBytes)+1)
					if sliceRows > h {
						gl.Call("pixelStorei", int(glUnpackImageHeight), int(sliceRows))
					}
					gl.Call("texSubImage3D", int(glTexture3D), int(op.TextureDst.MipLevel),
						int(op.TextureDst.OriginX), int(op.TextureDst.OriginY), int(op.TextureDst.OriginZ),
						int(w), int(h), int(d),
						int(tex.info.Format), int(tex.info.Type), arr)
					if sliceRows > h {
						gl.Call("pixelStorei", int(glUnpackImageHeight), 0)
					}
				}
			}
		default:
			gl.Call("texSubImage2D", int(glTexture2D), int(op.TextureDst.MipLevel),
				int(op.TextureDst.OriginX), int(op.TextureDst.OriginY), int(w), int(h),
				int(tex.info.Format), int(tex.info.Type), arr)
		}
		gl.Call("pixelStorei", int(glUnpackRowLength), 0)
		gl.Call("bindTexture", int(tex.target), js.Null())

	case driver.GPUOpDestroyTexture:
		if tex, ok := b.textures[op.ID]; ok {
			if tex.isRB {
				gl.Call("deleteRenderbuffer", tex.glRB)
			} else {
				gl.Call("deleteTexture", tex.glTex)
			}
			delete(b.textures, op.ID)
		}

	case driver.GPUOpCreateSampler:
		if op.Sampler == nil {
			return
		}
		desc := *op.Sampler
		b.samplers[op.ID] = &gl2Sampler{
			glSampler:    b.createGLSampler(desc, false),
			glSamplerMip: b.createGLSampler(desc, true),
			desc:         desc,
		}

	case driver.GPUOpDestroySampler:
		if s, ok := b.samplers[op.ID]; ok {
			gl.Call("deleteSampler", s.glSampler)
			gl.Call("deleteSampler", s.glSamplerMip)
			delete(b.samplers, op.ID)
		}

	case driver.GPUOpCreateBindGroupLayout:
		if op.BindGroupLayout == nil {
			return
		}
		cp := *op.BindGroupLayout
		b.layouts[op.ID] = &cp

	case driver.GPUOpDestroyBindGroupLayout:
		delete(b.layouts, op.ID)

	case driver.GPUOpCreateBindGroup:
		if op.BindGroup == nil {
			return
		}
		m := make(map[uint32]driver.GPUBindGroupEntry, len(op.BindGroup.Entries))
		for _, e := range op.BindGroup.Entries {
			m[e.Binding] = e
		}
		b.bindGroups[op.ID] = &gl2BindGroup{
			layoutID: op.BindGroup.Layout,
			entries:  m,
		}

	case driver.GPUOpDestroyBindGroup:
		delete(b.bindGroups, op.ID)

	case driver.GPUOpCreatePipeline:
		if op.Pipeline == nil {
			return
		}
		b.createPipeline(op.ID, *op.Pipeline)

	case driver.GPUOpDestroyPipeline:
		if p, ok := b.pipelines[op.ID]; ok {
			gl.Call("deleteProgram", p.prog)
			gl.Call("deleteVertexArray", p.vao)
			delete(b.pipelines, op.ID)
		}
	}
}

func (b *webgl2Backend) createGLSampler(desc driver.GPUSamplerDesc, hasMips bool) js.Value {
	gl := b.gl
	s := gl.Call("createSampler")
	gl.Call("samplerParameteri", s, int(glTextureMinFilter), int(glMinFilter(desc.MinFilter, desc.MipmapFilter, hasMips)))
	gl.Call("samplerParameteri", s, int(glTextureMagFilter), int(glMagFilter(desc.MagFilter)))
	gl.Call("samplerParameteri", s, int(glTextureWrapS), int(glWrapMode(desc.AddressU)))
	gl.Call("samplerParameteri", s, int(glTextureWrapT), int(glWrapMode(desc.AddressV)))
	gl.Call("samplerParameteri", s, int(glTextureWrapR), int(glWrapMode(desc.AddressW)))
	if desc.Compare != driver.GPUCompareUndefined {
		gl.Call("samplerParameteri", s, int(glTextureCompareMode), int(glCompareRefToTexture))
		gl.Call("samplerParameteri", s, int(glTextureCompareFunc), int(glCompareFunc(desc.Compare)))
	}
	if desc.MaxAnisotropy > 1 && b.anisoExt.Truthy() {
		aniso := float64(desc.MaxAnisotropy)
		if b.maxAniso > 0 && aniso > b.maxAniso {
			aniso = b.maxAniso
		}
		gl.Call("samplerParameterf", s, b.anisoExt.Get("TEXTURE_MAX_ANISOTROPY_EXT"), aniso)
	}
	return s
}

func (b *webgl2Backend) compileShader(kind uint32, src string) (js.Value, error) {
	gl := b.gl
	sh := gl.Call("createShader", int(kind))
	gl.Call("shaderSource", sh, src)
	gl.Call("compileShader", sh)
	if !gl.Call("getShaderParameter", sh, int(glCompileStatus)).Truthy() {
		info := gl.Call("getShaderInfoLog", sh).String()
		gl.Call("deleteShader", sh)
		return js.Null(), fmt.Errorf("webgl2 shader compile: %s", info)
	}
	return sh, nil
}

func (b *webgl2Backend) createPipeline(id driver.GPUID, desc driver.GPUPipelineDesc) {
	gl := b.gl
	compiled, err := compileWebGL2Shaders(desc.Vertex, desc.Fragment)
	if err != nil {
		b.g.emitEvent(b.g.ctrl.makeError(driver.GPUErrorShaderCompile, err.Error()))
		return
	}
	vs, err := b.compileShader(glVertexShader, compiled.VertexSource)
	if err != nil {
		b.g.emitEvent(b.g.ctrl.makeError(driver.GPUErrorShaderCompile, err.Error()))
		return
	}
	fs, err := b.compileShader(glFragmentShader, compiled.FragmentSource)
	if err != nil {
		gl.Call("deleteShader", vs)
		b.g.emitEvent(b.g.ctrl.makeError(driver.GPUErrorShaderCompile, err.Error()))
		return
	}
	prog := gl.Call("createProgram")
	gl.Call("attachShader", prog, vs)
	gl.Call("attachShader", prog, fs)
	gl.Call("linkProgram", prog)
	gl.Call("deleteShader", vs)
	gl.Call("deleteShader", fs)
	if !gl.Call("getProgramParameter", prog, int(glLinkStatus)).Truthy() {
		info := gl.Call("getProgramInfoLog", prog).String()
		gl.Call("deleteProgram", prog)
		b.g.emitEvent(b.g.ctrl.makeError(driver.GPUErrorShaderCompile, "webgl2 program link: "+info))
		return
	}

	// Bind uniform block slots per glsl.CompilePipeline table.
	for _, ub := range compiled.UniformBlocks {
		idx := gl.Call("getUniformBlockIndex", prog, ub.BlockName)
		if idx.Type() == js.TypeNumber && uint32(idx.Int()) != glInvalidIndex {
			gl.Call("uniformBlockBinding", prog, idx.Int(), int(ub.Slot))
		}
	}

	// Bind combined sampler uniforms to their assigned texture units.
	gl.Call("useProgram", prog)
	for _, sp := range compiled.SamplerPairs {
		loc := gl.Call("getUniformLocation", prog, sp.Name)
		if loc.Truthy() {
			gl.Call("uniform1i", loc, int(sp.Unit))
		}
	}

	vao := gl.Call("createVertexArray")
	b.pipelines[id] = &gl2Pipeline{
		prog:          prog,
		vao:           vao,
		desc:          desc,
		uniformBlocks: gl2Blocks(compiled.UniformBlocks),
		samplerPairs:  gl2Pairs(compiled.SamplerPairs),
	}
}

func (b *webgl2Backend) execCopy(cp driver.GPUCopyOp) {
	gl := b.gl
	switch cp.Kind {
	case driver.GPUCopyBufferToBuffer:
		src, ok1 := b.buffers[cp.SrcID]
		dst, ok2 := b.buffers[cp.DstID]
		if !ok1 || !ok2 {
			return
		}
		gl.Call("bindBuffer", int(glCopyReadBuffer), src.glBuf)
		gl.Call("bindBuffer", int(glCopyWriteBuffer), dst.glBuf)
		gl.Call("copyBufferSubData", int(glCopyReadBuffer), int(glCopyWriteBuffer), int(cp.SrcOffset), int(cp.DstOffset), int(cp.Size))
		gl.Call("bindBuffer", int(glCopyReadBuffer), js.Null())
		gl.Call("bindBuffer", int(glCopyWriteBuffer), js.Null())

	case driver.GPUCopyBufferToTexture:
		src, ok1 := b.buffers[cp.SrcID]
		dst, ok2 := b.textures[cp.DstID]
		if !ok1 || !ok2 || dst.isRB {
			return
		}
		gl.Call("bindBuffer", int(glPixelUnpackBuffer), src.glBuf)
		gl.Call("bindTexture", int(dst.target), dst.glTex)
		gl.Call("pixelStorei", int(glUnpackAlignment), 1)
		bpp := dst.info.BytesPerPixel
		if bpp == 0 {
			bpp = 4
		}
		if cp.SrcLayout.BytesPerRow > 0 {
			gl.Call("pixelStorei", int(glUnpackRowLength), int(cp.SrcLayout.BytesPerRow/bpp))
		}
		switch dst.desc.Dimension {
		case driver.GPUDim2DArray:
			gl.Call("texSubImage3D", int(glTexture2DArray), int(cp.DstTex.MipLevel),
				int(cp.DstTex.OriginX), int(cp.DstTex.OriginY), int(cp.DstTex.ArrayLayer),
				int(cp.Width), int(cp.Height), int(max(cp.Layers, 1)),
				int(dst.info.Format), int(dst.info.Type), int(cp.SrcLayout.Offset))
		case driver.GPUDim3D:
			if cp.SrcLayout.RowsPerImage > 0 {
				gl.Call("pixelStorei", int(glUnpackImageHeight), int(cp.SrcLayout.RowsPerImage))
			}
			gl.Call("texSubImage3D", int(glTexture3D), int(cp.DstTex.MipLevel),
				int(cp.DstTex.OriginX), int(cp.DstTex.OriginY), int(cp.DstTex.OriginZ),
				int(cp.Width), int(cp.Height), int(max(cp.Layers, 1)),
				int(dst.info.Format), int(dst.info.Type), int(cp.SrcLayout.Offset))
			if cp.SrcLayout.RowsPerImage > 0 {
				gl.Call("pixelStorei", int(glUnpackImageHeight), 0)
			}
		default:
			faceTarget := dst.target
			if dst.desc.Dimension == driver.GPUDimCube {
				faceTarget = glTextureCubeMapPosX + cp.DstTex.ArrayLayer
			}
			gl.Call("texSubImage2D", int(faceTarget), int(cp.DstTex.MipLevel),
				int(cp.DstTex.OriginX), int(cp.DstTex.OriginY), int(cp.Width), int(cp.Height),
				int(dst.info.Format), int(dst.info.Type), int(cp.SrcLayout.Offset))
		}
		gl.Call("pixelStorei", int(glUnpackRowLength), 0)
		gl.Call("bindBuffer", int(glPixelUnpackBuffer), js.Null())
		gl.Call("bindTexture", int(dst.target), js.Null())

	case driver.GPUCopyTextureToTexture:
		src, ok1 := b.textures[cp.SrcID]
		dst, ok2 := b.textures[cp.DstID]
		if !ok1 || !ok2 {
			return
		}
		rfbo := b.ensureReadFBO()
		dfbo := b.ensureDrawFBO()
		gl.Call("bindFramebuffer", int(glReadFramebuffer), rfbo)
		gl.Call("bindFramebuffer", int(glDrawFramebuffer), dfbo)
		layers := max(cp.Layers, 1)
		for l := uint32(0); l < layers; l++ {
			srcLayer := cp.SrcTex.ArrayLayer + l
			if src.desc.Dimension == driver.GPUDim3D {
				srcLayer = cp.SrcTex.OriginZ + l
			}
			dstLayer := cp.DstTex.ArrayLayer + l
			if dst.desc.Dimension == driver.GPUDim3D {
				dstLayer = cp.DstTex.OriginZ + l
			}
			b.attachTextureToFBO(glReadFramebuffer, glColorAttachment0, src, cp.SrcTex.MipLevel, srcLayer)
			b.attachTextureToFBO(glDrawFramebuffer, glColorAttachment0, dst, cp.DstTex.MipLevel, dstLayer)
			gl.Call("blitFramebuffer",
				int(cp.SrcTex.OriginX), int(cp.SrcTex.OriginY), int(cp.SrcTex.OriginX+cp.Width), int(cp.SrcTex.OriginY+cp.Height),
				int(cp.DstTex.OriginX), int(cp.DstTex.OriginY), int(cp.DstTex.OriginX+cp.Width), int(cp.DstTex.OriginY+cp.Height),
				int(glColorBufferBit), int(glNearest))
		}
		gl.Call("bindFramebuffer", int(glReadFramebuffer), js.Null())
		gl.Call("bindFramebuffer", int(glDrawFramebuffer), js.Null())
	}
}

func (b *webgl2Backend) attachTextureToFBO(fboTarget, attachment uint32, tex *gl2Texture, mip, layer uint32) {
	gl := b.gl
	if tex == nil {
		gl.Call("framebufferTexture2D", int(fboTarget), int(attachment), int(glTexture2D), js.Null(), 0)
		return
	}
	if tex.isRB {
		gl.Call("framebufferRenderbuffer", int(fboTarget), int(attachment), int(glRenderbuffer), tex.glRB)
		return
	}
	switch tex.desc.Dimension {
	case driver.GPUDimCube:
		gl.Call("framebufferTexture2D", int(fboTarget), int(attachment), int(glTextureCubeMapPosX+layer), tex.glTex, int(mip))
	case driver.GPUDim2DArray, driver.GPUDim3D:
		gl.Call("framebufferTextureLayer", int(fboTarget), int(attachment), tex.glTex, int(mip), int(layer))
	default:
		gl.Call("framebufferTexture2D", int(fboTarget), int(attachment), int(glTexture2D), tex.glTex, int(mip))
	}
}

func (b *webgl2Backend) execPass(pass driver.GPUPass) {
	gl := b.gl
	isSwapchain := isSwapchainPass(pass)
	targetW, targetH := b.width, b.height

	if isSwapchain {
		gl.Call("bindFramebuffer", int(glFramebuffer), js.Null())
	} else {
		fbo := b.ensurePassFBO()
		gl.Call("bindFramebuffer", int(glFramebuffer), fbo)
		if len(pass.ColorAttachments) == 0 {
			for i := 0; i < b.passFBOColorCount; i++ {
				b.attachTextureToFBO(glFramebuffer, glColorAttachment0+uint32(i), nil, 0, 0)
			}
			b.passFBOColorCount = 0
			gl.Call("drawBuffers", []any{int(glNone)})
		} else {
			drawBufs := make([]any, 0, len(pass.ColorAttachments))
			for i, ca := range pass.ColorAttachments {
				att := glColorAttachment0 + uint32(i)
				tex := b.textures[ca.Texture]
				b.attachTextureToFBO(glFramebuffer, att, tex, ca.MipLevel, ca.ArrayLayer)
				if i == 0 && tex != nil {
					targetW = int(mipDim(tex.desc.Width, ca.MipLevel))
					targetH = int(mipDim(tex.desc.Height, ca.MipLevel))
				}
				drawBufs = append(drawBufs, int(att))
			}
			for i := len(pass.ColorAttachments); i < b.passFBOColorCount; i++ {
				b.attachTextureToFBO(glFramebuffer, glColorAttachment0+uint32(i), nil, 0, 0)
			}
			b.passFBOColorCount = len(pass.ColorAttachments)
			gl.Call("drawBuffers", drawBufs)
		}
		if pass.DepthStencil != nil && pass.DepthStencil.Texture != 0 {
			dtex := b.textures[pass.DepthStencil.Texture]
			att := glDepthAttachment
			if dtex != nil && dtex.info.HasStencil {
				att = glDepthStencilAttach
			}
			b.attachTextureToFBO(glFramebuffer, att, dtex, 0, 0)
			if len(pass.ColorAttachments) == 0 && dtex != nil {
				targetW = max(int(dtex.desc.Width), 1)
				targetH = max(int(dtex.desc.Height), 1)
			}
		} else {
			gl.Call("framebufferTexture2D", int(glFramebuffer), int(glDepthStencilAttach), int(glTexture2D), js.Null(), 0)
			gl.Call("framebufferTexture2D", int(glFramebuffer), int(glDepthAttachment), int(glTexture2D), js.Null(), 0)
		}
	}

	gl.Call("viewport", 0, 0, targetW, targetH)
	gl.Call("depthRange", 0.0, 1.0)
	gl.Call("disable", int(glScissorTest))

	// Ensure write masks do not block clears.
	gl.Call("colorMask", true, true, true, true)
	gl.Call("depthMask", true)

	if isSwapchain {
		var bits uint32
		if len(pass.ColorAttachments) > 0 && pass.ColorAttachments[0].LoadOp == driver.GPULoadClear {
			r, g, bl, a := pass.ColorAttachments[0].ClearColor.Components()
			gl.Call("clearColor", r, g, bl, a)
			bits |= glColorBufferBit
		}
		if pass.DepthStencil != nil {
			if pass.DepthStencil.DepthLoadOp == driver.GPULoadClear {
				gl.Call("clearDepth", float64(pass.DepthStencil.ClearDepth))
				bits |= glDepthBufferBit
			}
			if b.hasStencil && pass.DepthStencil.StencilLoadOp == driver.GPULoadClear {
				gl.Call("clearStencil", int(pass.DepthStencil.ClearStencil))
				bits |= glStencilBufferBit
			}
		}
		if bits != 0 {
			gl.Call("clear", int(bits))
		}
	} else {
		for i, ca := range pass.ColorAttachments {
			if ca.LoadOp == driver.GPULoadClear {
				r, g, bl, a := ca.ClearColor.Components()
				gl.Call("clearBufferfv", int(glColorEnum), i, []any{r, g, bl, a})
			}
		}
		if pass.DepthStencil != nil && pass.DepthStencil.Texture != 0 {
			var bits uint32
			if pass.DepthStencil.DepthLoadOp == driver.GPULoadClear {
				gl.Call("clearDepth", float64(pass.DepthStencil.ClearDepth))
				bits |= glDepthBufferBit
			}
			if pass.DepthStencil.StencilLoadOp == driver.GPULoadClear {
				if dtex := b.textures[pass.DepthStencil.Texture]; dtex != nil && dtex.info.HasStencil {
					gl.Call("clearStencil", int(pass.DepthStencil.ClearStencil))
					bits |= glStencilBufferBit
				}
			}
			if bits != 0 {
				gl.Call("clear", int(bits))
			}
		}
	}

	var (
		activePipe  *gl2Pipeline
		boundGroups = map[uint32]gl2BoundGroup{}
		boundVBs    = map[uint32]gl2BoundVB{}
		boundIB     gl2BoundIB
	)

	for _, c := range pass.Commands {
		switch c.Kind {
		case driver.GPUCmdSetViewport:
			gl.Call("viewport", int(c.F0), glViewportY(c.F1, c.F3, targetH), int(c.F2), int(c.F3))
			gl.Call("depthRange", float64(c.F4), float64(c.F5))

		case driver.GPUCmdSetScissor:
			gl.Call("enable", int(glScissorTest))
			gl.Call("scissor", int(c.I0), glScissorY(c.I1, c.I3, targetH), int(c.I2), int(c.I3))

		case driver.GPUCmdSetPipeline:
			p, ok := b.pipelines[c.ID]
			if !ok {
				continue
			}
			activePipe = p
			b.applyPipelineState(p)

		case driver.GPUCmdSetBindGroup:
			boundGroups[c.Slot] = gl2BoundGroup{id: c.ID, dynamicOffsets: c.DynamicOffsets}

		case driver.GPUCmdSetVertexBuffer:
			boundVBs[c.Slot] = gl2BoundVB{id: c.ID, offset: c.Offset}

		case driver.GPUCmdSetIndexBuffer:
			boundIB = gl2BoundIB{id: c.ID, format: c.IndexFormat, offset: c.Offset}

		case driver.GPUCmdSetBlendConstant:
			gl.Call("blendColor", float64(c.F0), float64(c.F1), float64(c.F2), float64(c.F3))

		case driver.GPUCmdDraw:
			if activePipe == nil {
				continue
			}
			b.bindDrawState(activePipe, boundGroups, boundVBs, boundIB, false, 0, 0)
			mode := glPrimitiveMode(activePipe.desc.Primitive.Topology)
			gl.Call("drawArrays", int(mode), int(c.U1), int(c.U0))

		case driver.GPUCmdDrawIndexed:
			if activePipe == nil {
				continue
			}
			if !b.bindDrawState(activePipe, boundGroups, boundVBs, boundIB, true, c.I0, 0) {
				continue
			}
			mode := glPrimitiveMode(activePipe.desc.Primitive.Topology)
			idxType, elemBytes := glIndexType(boundIB.format)
			byteOff := boundIB.offset + uint64(c.U1)*elemBytes
			gl.Call("drawElements", int(mode), int(c.U0), int(idxType), int(byteOff))

		case driver.GPUCmdDrawInstanced:
			if activePipe == nil {
				continue
			}
			b.bindDrawState(activePipe, boundGroups, boundVBs, boundIB, false, 0, c.U3)
			mode := glPrimitiveMode(activePipe.desc.Primitive.Topology)
			gl.Call("drawArraysInstanced", int(mode), int(c.U2), int(c.U0), int(c.U1))

		case driver.GPUCmdDrawIndexedInstanced:
			if activePipe == nil {
				continue
			}
			if !b.bindDrawState(activePipe, boundGroups, boundVBs, boundIB, true, c.I0, c.U3) {
				continue
			}
			mode := glPrimitiveMode(activePipe.desc.Primitive.Topology)
			idxType, elemBytes := glIndexType(boundIB.format)
			byteOff := boundIB.offset + uint64(c.U2)*elemBytes
			gl.Call("drawElementsInstanced", int(mode), int(c.U0), int(idxType), int(byteOff), int(c.U1))
		}
	}

	// Perform MSAA resolve blits for offscreen attachments with ResolveTarget.
	if !isSwapchain {
		for _, ca := range pass.ColorAttachments {
			if ca.Texture == 0 || ca.ResolveTarget == 0 {
				continue
			}
			src := b.textures[ca.Texture]
			dst := b.textures[ca.ResolveTarget]
			if src == nil || dst == nil {
				continue
			}
			rfbo := b.ensureReadFBO()
			dfbo := b.ensureDrawFBO()
			gl.Call("bindFramebuffer", int(glReadFramebuffer), rfbo)
			b.attachTextureToFBO(glReadFramebuffer, glColorAttachment0, src, ca.MipLevel, ca.ArrayLayer)
			gl.Call("bindFramebuffer", int(glDrawFramebuffer), dfbo)
			b.attachTextureToFBO(glDrawFramebuffer, glColorAttachment0, dst, 0, 0)
			gl.Call("blitFramebuffer", 0, 0, int(src.desc.Width), int(src.desc.Height),
				0, 0, int(dst.desc.Width), int(dst.desc.Height), int(glColorBufferBit), int(glNearest))
			gl.Call("bindFramebuffer", int(glReadFramebuffer), js.Null())
			gl.Call("bindFramebuffer", int(glDrawFramebuffer), js.Null())
		}
	}

	gl.Call("bindVertexArray", js.Null())
	gl.Call("bindFramebuffer", int(glFramebuffer), js.Null())
}

func (b *webgl2Backend) applyPipelineState(p *gl2Pipeline) {
	gl := b.gl
	gl.Call("useProgram", p.prog)

	// Face culling and winding order.
	gl.Call("frontFace", int(glFrontFace(p.desc.Primitive.FrontFace)))
	if enabled, face := glCullFace(p.desc.Primitive.CullMode); enabled {
		gl.Call("enable", int(glCullFaceEnable))
		gl.Call("cullFace", int(face))
	} else {
		gl.Call("disable", int(glCullFaceEnable))
	}

	// Depth testing and polygon offset.
	if ds := p.desc.DepthStencil; ds != nil {
		gl.Call("enable", int(glDepthTest))
		gl.Call("depthFunc", int(glCompareFunc(ds.DepthCompare)))
		gl.Call("depthMask", ds.DepthWrite)
		if ds.DepthBias != 0 || ds.DepthBiasSlopeScale != 0 {
			gl.Call("enable", int(glPolygonOffsetFill))
			gl.Call("polygonOffset", float64(ds.DepthBiasSlopeScale), float64(ds.DepthBias))
		} else {
			gl.Call("disable", int(glPolygonOffsetFill))
		}
	} else {
		gl.Call("disable", int(glDepthTest))
		gl.Call("depthMask", false)
		gl.Call("disable", int(glPolygonOffsetFill))
	}

	// Blending and color write mask (from target 0 in WebGL 2).
	if len(p.desc.Targets) > 0 {
		t0 := p.desc.Targets[0]
		mask := t0.WriteMask
		if mask == 0 {
			mask = driver.GPUColorWriteAll
		}
		gl.Call("colorMask",
			mask&driver.GPUColorWriteRed != 0,
			mask&driver.GPUColorWriteGreen != 0,
			mask&driver.GPUColorWriteBlue != 0,
			mask&driver.GPUColorWriteAlpha != 0,
		)
		if t0.Blend != nil {
			gl.Call("enable", int(glBlendEnable))
			gl.Call("blendFuncSeparate",
				int(glBlendFactor(t0.Blend.Color.SrcFactor)),
				int(glBlendFactor(t0.Blend.Color.DstFactor)),
				int(glBlendFactor(t0.Blend.Alpha.SrcFactor)),
				int(glBlendFactor(t0.Blend.Alpha.DstFactor)),
			)
			gl.Call("blendEquationSeparate",
				int(glBlendEq(t0.Blend.Color.Operation)),
				int(glBlendEq(t0.Blend.Alpha.Operation)),
			)
		} else {
			gl.Call("disable", int(glBlendEnable))
		}
	} else {
		gl.Call("colorMask", true, true, true, true)
		gl.Call("disable", int(glBlendEnable))
	}
}

func (b *webgl2Backend) bindDrawState(
	p *gl2Pipeline,
	boundGroups map[uint32]gl2BoundGroup,
	boundVBs map[uint32]gl2BoundVB,
	boundIB gl2BoundIB,
	needIndex bool,
	baseVertex int32,
	firstInstance uint32,
) bool {
	gl := b.gl
	gl.Call("bindVertexArray", p.vao)

	// Bind vertex buffers and attributes into the pipeline's VAO.
	for slot, vbLayout := range p.desc.VertexBuffers {
		bvb, ok := boundVBs[uint32(slot)]
		if !ok {
			continue
		}
		buf, ok := b.buffers[bvb.id]
		if !ok {
			continue
		}
		gl.Call("bindBuffer", int(glArrayBuffer), buf.glBuf)
		divisor := 0
		extraOff := int64(0)
		if vbLayout.StepMode == driver.GPUStepInstance {
			divisor = 1
			extraOff = int64(firstInstance) * int64(vbLayout.Stride)
		} else if baseVertex != 0 {
			extraOff = int64(baseVertex) * int64(vbLayout.Stride)
		}
		for _, a := range vbLayout.Attributes {
			info := glVertexAttrInfo(a.Format)
			ptrOff := int64(bvb.offset) + int64(a.Offset) + extraOff
			if ptrOff < 0 {
				ptrOff = 0
			}
			gl.Call("enableVertexAttribArray", int(a.Location))
			if info.Integer {
				gl.Call("vertexAttribIPointer", int(a.Location), info.Components, int(info.GLType), int(vbLayout.Stride), int(ptrOff))
			} else {
				gl.Call("vertexAttribPointer", int(a.Location), info.Components, int(info.GLType), info.Normalized, int(vbLayout.Stride), int(ptrOff))
			}
			gl.Call("vertexAttribDivisor", int(a.Location), divisor)
		}
	}

	if needIndex {
		ibuf, ok := b.buffers[boundIB.id]
		if !ok {
			return false
		}
		gl.Call("bindBuffer", int(glElementArrayBuffer), ibuf.glBuf)
	}

	// Bind uniform buffers according to compiled uniform block slot table.
	for _, ub := range p.uniformBlocks {
		bgRef, ok := boundGroups[ub.Group]
		if !ok {
			continue
		}
		bg, ok := b.bindGroups[bgRef.id]
		if !ok {
			continue
		}
		entry, ok := bg.entries[ub.Binding]
		if !ok || entry.Buffer == 0 {
			continue
		}
		buf, ok := b.buffers[entry.Buffer]
		if !ok {
			continue
		}
		dynOff := uint64(0)
		if len(bgRef.dynamicOffsets) > 0 {
			if layout := b.layouts[bg.layoutID]; layout != nil {
				if dm := resolveDynamicOffsets(layout.Entries, bgRef.dynamicOffsets); dm != nil {
					dynOff = uint64(dm[ub.Binding])
				}
			}
		}
		offset := entry.Offset + dynOff
		size := entry.Size
		if size == 0 && offset < buf.size {
			size = buf.size - offset
		}
		if offset == 0 && (size == 0 || size == buf.size) {
			gl.Call("bindBufferBase", int(glUniformBuffer), int(ub.Slot), buf.glBuf)
		} else {
			gl.Call("bindBufferRange", int(glUniformBuffer), int(ub.Slot), buf.glBuf, int(offset), int(size))
		}
	}

	// Bind combined (texture, sampler) pairs on their assigned texture units.
	for _, sp := range p.samplerPairs {
		gl.Call("activeTexture", int(glTexture0+sp.Unit))
		var boundTex *gl2Texture
		if bgRef, ok := boundGroups[sp.TextureGroup]; ok {
			if bg, ok := b.bindGroups[bgRef.id]; ok {
				if te, ok := bg.entries[sp.TextureBinding]; ok && te.Texture != 0 {
					if tex, ok := b.textures[te.Texture]; ok && !tex.isRB {
						gl.Call("bindTexture", int(tex.target), tex.glTex)
						boundTex = tex
					}
				}
			}
		}
		boundSampler := false
		if bgRef, ok := boundGroups[sp.SamplerGroup]; ok {
			if bg, ok := b.bindGroups[bgRef.id]; ok {
				if se, ok := bg.entries[sp.SamplerBinding]; ok && se.Sampler != 0 {
					if samp, ok := b.samplers[se.Sampler]; ok {
						glSamp := samp.glSampler
						if boundTex != nil && boundTex.desc.MipLevels > 1 {
							glSamp = samp.glSamplerMip
						}
						gl.Call("bindSampler", int(sp.Unit), glSamp)
						boundSampler = true
					}
				}
			}
		}
		if !boundSampler {
			gl.Call("bindSampler", int(sp.Unit), js.Null())
		}
	}

	return true
}

func (b *webgl2Backend) execReadTexture(op driver.GPUResourceOp) {
	gl := b.gl
	gen := b.g.ctrl.generation
	if op.ReadbackSurface {
		w, h := max(b.width, 1), max(b.height, 1)
		gl.Call("bindFramebuffer", int(glFramebuffer), js.Null())
		arr := js.Global().Get("Uint8Array").New(w * h * 4)
		gl.Call("readPixels", 0, 0, w, h, int(glRGBA), int(glUnsignedByte), arr)
		pix := make([]byte, w*h*4)
		js.CopyBytesToGo(pix, arr)
		glFlipRowsRGBA(pix, w, h)
		b.g.emitEvent(&driver.GPUEvent{
			Kind:       driver.GPUEventReadback,
			Generation: gen,
			ReadbackID: op.ReadbackID,
			Pixels:     pix,
		})
		return
	}

	tex, ok := b.textures[op.ID]
	if !ok {
		b.g.emitEvent(&driver.GPUEvent{
			Kind:       driver.GPUEventReadback,
			Generation: gen,
			ReadbackID: op.ReadbackID,
			Message:    fmt.Sprintf("webgl2: readback texture %d not found", op.ID),
		})
		return
	}
	x, y := int(op.ReadbackRegion.X), int(op.ReadbackRegion.Y)
	rw, rh := int(op.ReadbackRegion.Width), int(op.ReadbackRegion.Height)
	texW := int(mipDim(tex.desc.Width, op.ReadbackRegion.MipLevel))
	texH := int(mipDim(tex.desc.Height, op.ReadbackRegion.MipLevel))
	if rw == 0 {
		rw = max(texW-x, 1)
	}
	if rh == 0 {
		rh = max(texH-y, 1)
	}

	fbo := b.ensureReadFBO()
	gl.Call("bindFramebuffer", int(glFramebuffer), fbo)
	layer := op.ReadbackRegion.ArrayLayer
	if tex.desc.Dimension == driver.GPUDim3D {
		layer = op.ReadbackRegion.Z
	}
	b.attachTextureToFBO(glFramebuffer, glColorAttachment0, tex, op.ReadbackRegion.MipLevel, layer)

	isRenderTarget := tex.desc.Usage&driver.GPUTextureRenderAttachment != 0
	glY := y
	if isRenderTarget {
		glY = max(texH-(y+rh), 0)
	}
	arr := js.Global().Get("Uint8Array").New(rw * rh * 4)
	gl.Call("readPixels", x, glY, rw, rh, int(glRGBA), int(glUnsignedByte), arr)
	gl.Call("bindFramebuffer", int(glFramebuffer), js.Null())

	pix := make([]byte, rw*rh*4)
	js.CopyBytesToGo(pix, arr)
	if isRenderTarget {
		glFlipRowsRGBA(pix, rw, rh)
	}
	b.g.emitEvent(&driver.GPUEvent{
		Kind:       driver.GPUEventReadback,
		Generation: gen,
		ReadbackID: op.ReadbackID,
		Pixels:     pix,
	})
}

func (b *webgl2Backend) clearGLObjectMaps() {
	clear(b.buffers)
	clear(b.textures)
	clear(b.samplers)
	clear(b.layouts)
	clear(b.bindGroups)
	clear(b.pipelines)
	b.passFBO = js.Undefined()
	b.passFBOColorCount = 0
	b.readFBO = js.Undefined()
	b.drawFBO = js.Undefined()
}

func (b *webgl2Backend) destroy() {
	if b.destroyed {
		return
	}
	b.destroyed = true
	b.canvasListeners.release()
	if b.gl.Truthy() {
		gl := b.gl
		for id, buf := range b.buffers {
			gl.Call("deleteBuffer", buf.glBuf)
			delete(b.buffers, id)
		}
		for id, tex := range b.textures {
			if tex.isRB {
				gl.Call("deleteRenderbuffer", tex.glRB)
			} else {
				gl.Call("deleteTexture", tex.glTex)
			}
			delete(b.textures, id)
		}
		for id, s := range b.samplers {
			gl.Call("deleteSampler", s.glSampler)
			gl.Call("deleteSampler", s.glSamplerMip)
			delete(b.samplers, id)
		}
		for id, p := range b.pipelines {
			gl.Call("deleteProgram", p.prog)
			gl.Call("deleteVertexArray", p.vao)
			delete(b.pipelines, id)
		}
		if b.passFBO.Truthy() {
			gl.Call("deleteFramebuffer", b.passFBO)
		}
		if b.readFBO.Truthy() {
			gl.Call("deleteFramebuffer", b.readFBO)
		}
		if b.drawFBO.Truthy() {
			gl.Call("deleteFramebuffer", b.drawFBO)
		}
		if lose := gl.Call("getExtension", "WEBGL_lose_context"); lose.Truthy() {
			lose.Call("loseContext")
		}
		b.gl = js.Undefined()
	}
	b.clearGLObjectMaps()
}
