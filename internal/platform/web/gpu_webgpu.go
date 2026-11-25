//go:build js && wasm

package web

import (
	"fmt"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type webgpuBuffer struct {
	gpuBuf js.Value
	desc   driver.GPUBufferDesc
	size   uint64
}

type webgpuTexture struct {
	gpuTex  js.Value
	gpuView js.Value
	desc    driver.GPUTextureDesc
}

type webgpuSampler struct {
	gpuSampler js.Value
	desc       driver.GPUSamplerDesc
}

type webgpuBindGroupLayout struct {
	gpuLayout js.Value
	desc      driver.GPUBindGroupLayoutDesc
}

type webgpuBindGroup struct {
	gpuBG   js.Value
	desc    driver.GPUBindGroupDesc
	autoBGs map[uint64]js.Value // (pipeID<<16 | slot) -> GPUBindGroup for auto-layout pipelines
}

type webgpuPipeline struct {
	id            driver.GPUID
	gpuPipe       js.Value
	desc          driver.GPUPipelineDesc
	hasAutoLayout bool
}

type webgpuBackend struct {
	g      *gpuView
	device js.Value
	queue  js.Value
	ctx    js.Value

	surfaceFormat    driver.GPUTextureFormat
	surfaceFormatStr string
	depthFormat      driver.GPUTextureFormat
	depthFormatStr   string
	sampleCount      int
	width, height    int

	depthTex      js.Value
	depthView     js.Value
	msaaColorTex  js.Value
	msaaColorView js.Value

	buffers    map[driver.GPUID]*webgpuBuffer
	textures   map[driver.GPUID]*webgpuTexture
	samplers   map[driver.GPUID]*webgpuSampler
	layouts    map[driver.GPUID]*webgpuBindGroupLayout
	bindGroups map[driver.GPUID]*webgpuBindGroup
	pipelines  map[driver.GPUID]*webgpuPipeline

	// emptyLayout is a zero-entry bind group layout created on first use, standing in for nil PipelineDesc.Layouts slots.
	emptyLayout js.Value

	devListeners listeners
	destroyed    bool
}

func (g *gpuView) initWebGPU() {
	navGPU := js.Global().Get("navigator").Get("gpu")
	if !navGPU.Truthy() {
		g.initWebGL2()
		return
	}
	adapterPromise := navGPU.Call("requestAdapter")
	g.promiseThen(adapterPromise, func(adapter js.Value) {
		if g.ctrl.state == gpuStateDestroyed {
			return
		}
		if !adapter.Truthy() {
			g.initWebGL2()
			return
		}
		devicePromise := adapter.Call("requestDevice")
		g.promiseThen(devicePromise, func(device js.Value) {
			if g.ctrl.state == gpuStateDestroyed {
				if device.Truthy() {
					device.Call("destroy")
				}
				return
			}
			if !device.Truthy() {
				g.initWebGL2()
				return
			}
			ctx := g.canvas.Call("getContext", "webgpu")
			if !ctx.Truthy() {
				device.Call("destroy")
				g.initWebGL2()
				return
			}
			g.finishWebGPUInit(navGPU, device, ctx)
		}, func(js.Value) {
			if g.ctrl.state == gpuStateDestroyed {
				return
			}
			g.initWebGL2()
		})
	}, func(js.Value) {
		if g.ctrl.state == gpuStateDestroyed {
			return
		}
		g.initWebGL2()
	})
}

func (g *gpuView) finishWebGPUInit(navGPU, device, ctx js.Value) {
	prefStr := "bgra8unorm"
	if fn := navGPU.Get("getPreferredCanvasFormat"); fn.Type() == js.TypeFunction {
		if s := navGPU.Call("getPreferredCanvasFormat").String(); s != "" {
			prefStr = s
		}
	}
	surfFmt := driverFormatFromWebGPU(prefStr)

	cfgObj := map[string]any{
		"device":    device,
		"format":    prefStr,
		"alphaMode": "premultiplied",
		"usage":     int(webgpuTexRenderAttachment | webgpuTexCopySrc),
	}
	ctx.Call("configure", cfgObj)

	samples := g.ctrl.cfg.SampleCount
	if samples <= 0 {
		samples = 1
	}
	if samples > 1 {
		samples = 4
	}
	depthFmt := g.ctrl.cfg.DepthFormat
	depthFmtStr := ""
	if depthFmt.IsDepth() {
		depthFmtStr = wgslTextureFormat(depthFmt)
	}

	b := &webgpuBackend{
		g:                g,
		device:           device,
		queue:            device.Get("queue"),
		ctx:              ctx,
		surfaceFormat:    surfFmt,
		surfaceFormatStr: prefStr,
		depthFormat:      depthFmt,
		depthFormatStr:   depthFmtStr,
		sampleCount:      samples,
		width:            g.ctrl.width,
		height:           g.ctrl.height,
		buffers:          map[driver.GPUID]*webgpuBuffer{},
		textures:         map[driver.GPUID]*webgpuTexture{},
		samplers:         map[driver.GPUID]*webgpuSampler{},
		layouts:          map[driver.GPUID]*webgpuBindGroupLayout{},
		bindGroups:       map[driver.GPUID]*webgpuBindGroup{},
		pipelines:        map[driver.GPUID]*webgpuPipeline{},
	}
	b.recreateSwapchainAttachments()

	// Listen for asynchronous validation / OOM errors.
	b.devListeners.on(device, "uncapturederror", func(e js.Value) {
		if b.destroyed || g.ctrl.state == gpuStateDestroyed {
			return
		}
		errVal := e.Get("error")
		code := driver.GPUErrorFailed
		if errVal.Truthy() {
			if ctor := errVal.Get("constructor"); ctor.Truthy() {
				if ctor.Get("name").String() == "GPUOutOfMemoryError" {
					code = driver.GPUErrorOutOfMemory
				}
			}
		}
		g.emitEvent(g.ctrl.makeError(code, jsErrorMessage(errVal)))
	})

	// Watch device.lost -> EventGPU (GPUEventDeviceLost) and recover on resets.
	if lostPromise := device.Get("lost"); lostPromise.Truthy() {
		g.promiseThen(lostPromise, func(info js.Value) {
			if b.destroyed || g.ctrl.state == gpuStateDestroyed {
				return
			}
			reasonStr := ""
			msg := "WebGPU device lost"
			if info.Truthy() {
				if r := info.Get("reason"); r.Type() == js.TypeString {
					reasonStr = r.String()
				}
				if m := info.Get("message"); m.Type() == js.TypeString && m.String() != "" {
					msg = m.String()
				}
			}
			reason := webgpuLostReason(reasonStr)
			b.cleanupResources(false)
			if g.backend == b {
				g.backend = nil
			}
			if ev := g.ctrl.markLost(reason, msg); ev != nil {
				g.emitEvent(ev)
			}
			// Recover automatically on non-explicit destruction (e.g. driver/GPU reset).
			if reason != driver.GPULostDestroyed && g.ctrl.state == gpuStateLost {
				g.startInit()
			}
		}, nil)
	}

	g.backend = b
	limits := readWebGPULimits(device)
	features := driver.GPUFeatures{
		StorageBuffers: true,
		Compute:        false,
		MSAA4x:         true,
		Depth32Float:   true,
		Anisotropy:     true,
	}
	if ev := g.ctrl.markReady(gpuBackendWebGPU, surfFmt, limits, features); ev != nil {
		g.emitEvent(ev)
	}
}

func readWebGPULimits(device js.Value) driver.GPULimits {
	var l driver.GPULimits
	lim := device.Get("limits")
	if lim.Truthy() {
		if v := lim.Get("maxTextureDimension2D"); v.Type() == js.TypeNumber {
			l.MaxTextureDimension2D = uint32(v.Int())
		}
		if v := lim.Get("maxBufferSize"); v.Type() == js.TypeNumber {
			l.MaxBufferSize = uint64(v.Float())
		}
		if v := lim.Get("maxUniformBufferBindingSize"); v.Type() == js.TypeNumber {
			l.MaxUniformBufferSize = uint64(v.Float())
		}
		if v := lim.Get("maxBindGroups"); v.Type() == js.TypeNumber {
			l.MaxBindGroups = uint32(v.Int())
		}
		if v := lim.Get("maxVertexBuffers"); v.Type() == js.TypeNumber {
			l.MaxVertexBuffers = uint32(v.Int())
		}
		if v := lim.Get("maxVertexAttributes"); v.Type() == js.TypeNumber {
			l.MaxVertexAttributes = uint32(v.Int())
		}
		if v := lim.Get("minUniformBufferOffsetAlignment"); v.Type() == js.TypeNumber {
			l.MinUniformBufferAlignment = uint32(v.Int())
		}
	}
	return clampGPULimits(l)
}

func (b *webgpuBackend) destroySwapchainAttachments() {
	if b.depthTex.Truthy() {
		b.depthTex.Call("destroy")
		b.depthTex = js.Undefined()
		b.depthView = js.Undefined()
	}
	if b.msaaColorTex.Truthy() {
		b.msaaColorTex.Call("destroy")
		b.msaaColorTex = js.Undefined()
		b.msaaColorView = js.Undefined()
	}
}

func (b *webgpuBackend) recreateSwapchainAttachments() {
	b.destroySwapchainAttachments()
	if b.destroyed || !b.device.Truthy() || b.width <= 0 || b.height <= 0 {
		return
	}
	if b.depthFormatStr != "" {
		b.depthTex = b.device.Call("createTexture", map[string]any{
			"size":        []any{b.width, b.height, 1},
			"format":      b.depthFormatStr,
			"sampleCount": b.sampleCount,
			"usage":       int(webgpuTexRenderAttachment),
		})
		b.depthView = b.depthTex.Call("createView")
	}
	if b.sampleCount > 1 {
		b.msaaColorTex = b.device.Call("createTexture", map[string]any{
			"size":        []any{b.width, b.height, 1},
			"format":      b.surfaceFormatStr,
			"sampleCount": b.sampleCount,
			"usage":       int(webgpuTexRenderAttachment),
		})
		b.msaaColorView = b.msaaColorTex.Call("createView")
	}
}

func (b *webgpuBackend) ensureDefaultDepth() {
	if b.depthView.Truthy() || b.destroyed {
		return
	}
	if b.depthFormatStr == "" {
		b.depthFormat = driver.GPUFormatDepth24Plus
		b.depthFormatStr = "depth24plus"
	}
	b.depthTex = b.device.Call("createTexture", map[string]any{
		"size":        []any{max(b.width, 1), max(b.height, 1), 1},
		"format":      b.depthFormatStr,
		"sampleCount": b.sampleCount,
		"usage":       int(webgpuTexRenderAttachment),
	})
	b.depthView = b.depthTex.Call("createView")
}

func (b *webgpuBackend) resize(width, height int) {
	if b.destroyed {
		return
	}
	b.width = max(width, 1)
	b.height = max(height, 1)
	b.recreateSwapchainAttachments()
}

func (b *webgpuBackend) submit(frame driver.GPUFrame) {
	if b.destroyed || !b.device.Truthy() {
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

	if len(frame.Copies) > 0 || len(frame.Passes) > 0 {
		enc := b.device.Call("createCommandEncoder")
		for _, cp := range frame.Copies {
			b.execCopy(enc, cp)
		}
		for _, pass := range frame.Passes {
			b.execPass(enc, pass)
		}
		cmdBuf := enc.Call("finish")
		b.queue.Call("submit", []any{cmdBuf})
	}

	for _, rb := range readbacks {
		b.execReadTexture(rb)
	}
}

func (b *webgpuBackend) execOp(op driver.GPUResourceOp) {
	switch op.Kind {
	case driver.GPUOpCreateBuffer:
		if op.Buffer == nil {
			return
		}
		desc := *op.Buffer
		alignedSize := alignUp(desc.Size, 4)
		if alignedSize == 0 {
			alignedSize = 4
		}
		mapped := len(desc.Data) > 0
		bufDesc := map[string]any{
			"size":             int(alignedSize),
			"usage":            int(webgpuBufferUsage(desc.Usage)),
			"mappedAtCreation": mapped,
		}
		if desc.Label != "" {
			bufDesc["label"] = desc.Label
		}
		gpuBuf := b.device.Call("createBuffer", bufDesc)
		if mapped {
			arr := js.Global().Get("Uint8Array").New(gpuBuf.Call("getMappedRange"))
			js.CopyBytesToJS(arr, desc.Data)
			gpuBuf.Call("unmap")
		}
		b.buffers[op.ID] = &webgpuBuffer{gpuBuf: gpuBuf, desc: desc, size: alignedSize}

	case driver.GPUOpWriteBuffer:
		buf, ok := b.buffers[op.ID]
		if !ok || len(op.Data) == 0 {
			return
		}
		data := op.Data
		if rem := len(data) % 4; rem != 0 {
			padded := make([]byte, len(data)+(4-rem))
			copy(padded, data)
			data = padded
		}
		arr := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(arr, data)
		b.queue.Call("writeBuffer", buf.gpuBuf, int(op.Offset), arr)

	case driver.GPUOpDestroyBuffer:
		if buf, ok := b.buffers[op.ID]; ok {
			buf.gpuBuf.Call("destroy")
			delete(b.buffers, op.ID)
		}

	case driver.GPUOpCreateTexture:
		if op.Texture == nil {
			return
		}
		desc := *op.Texture
		layers := max(desc.Layers, 1)
		mips := max(desc.MipLevels, 1)
		samples := max(desc.SampleCount, 1)
		texDesc := map[string]any{
			"size":          []any{int(desc.Width), int(desc.Height), int(layers)},
			"mipLevelCount": int(mips),
			"sampleCount":   samples,
			"dimension":     "2d",
			"format":        wgslTextureFormat(desc.Format),
			"usage":         int(webgpuTextureUsage(desc.Usage, samples, desc.Format.IsDepth())),
		}
		if desc.Label != "" {
			texDesc["label"] = desc.Label
		}
		gpuTex := b.device.Call("createTexture", texDesc)
		viewDesc := map[string]any{
			"dimension": webgpuTextureViewDimension(desc.Dimension),
		}
		gpuView := gpuTex.Call("createView", viewDesc)
		b.textures[op.ID] = &webgpuTexture{gpuTex: gpuTex, gpuView: gpuView, desc: desc}

	case driver.GPUOpWriteTexture:
		tex, ok := b.textures[op.ID]
		if !ok || len(op.Data) == 0 {
			return
		}
		bpp := tex.desc.Format.BytesPerPixel()
		if bpp == 0 {
			bpp = 4
		}
		off := int(op.DataLayout.Offset)
		if off >= len(op.Data) {
			return
		}
		src := op.Data[off:]
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
		dstObj := map[string]any{
			"texture":  tex.gpuTex,
			"mipLevel": int(op.TextureDst.MipLevel),
			"origin":   []any{int(op.TextureDst.OriginX), int(op.TextureDst.OriginY), int(op.TextureDst.ArrayLayer)},
		}
		layoutObj := map[string]any{
			"offset":       0,
			"bytesPerRow":  int(bpr),
			"rowsPerImage": int(rows),
		}
		extentObj := []any{int(w), int(h), 1}
		b.queue.Call("writeTexture", dstObj, arr, layoutObj, extentObj)

	case driver.GPUOpDestroyTexture:
		if tex, ok := b.textures[op.ID]; ok {
			tex.gpuTex.Call("destroy")
			delete(b.textures, op.ID)
		}

	case driver.GPUOpCreateSampler:
		if op.Sampler == nil {
			return
		}
		desc := *op.Sampler
		sDesc := map[string]any{
			"minFilter":     webgpuFilterMode(desc.MinFilter),
			"magFilter":     webgpuFilterMode(desc.MagFilter),
			"mipmapFilter":  webgpuFilterMode(desc.MipmapFilter),
			"addressModeU":  webgpuAddressMode(desc.AddressU),
			"addressModeV":  webgpuAddressMode(desc.AddressV),
			"addressModeW":  webgpuAddressMode(desc.AddressW),
			"maxAnisotropy": int(max(desc.MaxAnisotropy, 1)),
		}
		if desc.Compare != driver.GPUCompareUndefined {
			sDesc["compare"] = webgpuCompareFunc(desc.Compare)
		}
		gpuSamp := b.device.Call("createSampler", sDesc)
		b.samplers[op.ID] = &webgpuSampler{gpuSampler: gpuSamp, desc: desc}

	case driver.GPUOpDestroySampler:
		delete(b.samplers, op.ID)

	case driver.GPUOpCreateBindGroupLayout:
		if op.BindGroupLayout == nil {
			return
		}
		desc := *op.BindGroupLayout
		entries := make([]any, 0, len(desc.Entries))
		for _, e := range desc.Entries {
			entry := map[string]any{
				"binding":    int(e.Binding),
				"visibility": int(webgpuVisibility(e.Visibility)),
			}
			switch e.Type {
			case driver.GPUBindingUniform:
				entry["buffer"] = map[string]any{
					"type":             "uniform",
					"hasDynamicOffset": e.HasDynamicOffset,
				}
			case driver.GPUBindingStorageRead:
				entry["buffer"] = map[string]any{
					"type":             "read-only-storage",
					"hasDynamicOffset": e.HasDynamicOffset,
				}
			case driver.GPUBindingTexture:
				entry["texture"] = map[string]any{
					"sampleType":    "float",
					"viewDimension": "2d",
				}
			case driver.GPUBindingSampler:
				entry["sampler"] = map[string]any{
					"type": "filtering",
				}
			case driver.GPUBindingComparisonSampler:
				entry["sampler"] = map[string]any{
					"type": "comparison",
				}
			}
			entries = append(entries, entry)
		}
		bglDesc := map[string]any{"entries": entries}
		if desc.Label != "" {
			bglDesc["label"] = desc.Label
		}
		gpuLayout := b.device.Call("createBindGroupLayout", bglDesc)
		b.layouts[op.ID] = &webgpuBindGroupLayout{gpuLayout: gpuLayout, desc: desc}

	case driver.GPUOpDestroyBindGroupLayout:
		delete(b.layouts, op.ID)

	case driver.GPUOpCreateBindGroup:
		if op.BindGroup == nil {
			return
		}
		desc := *op.BindGroup
		bg := &webgpuBindGroup{desc: desc}
		if l, ok := b.layouts[desc.Layout]; ok {
			bg.gpuBG = b.createBindGroupWithLayout(l.gpuLayout, desc)
		}
		b.bindGroups[op.ID] = bg

	case driver.GPUOpDestroyBindGroup:
		delete(b.bindGroups, op.ID)

	case driver.GPUOpCreatePipeline:
		if op.Pipeline == nil {
			return
		}
		b.createPipeline(op.ID, *op.Pipeline)

	case driver.GPUOpDestroyPipeline:
		delete(b.pipelines, op.ID)
		for _, bg := range b.bindGroups {
			for k := range bg.autoBGs {
				if driver.GPUID(k>>16) == op.ID {
					delete(bg.autoBGs, k)
				}
			}
		}
	}
}

func (b *webgpuBackend) createBindGroupWithLayout(gpuLayout js.Value, desc driver.GPUBindGroupDesc) js.Value {
	entries := make([]any, 0, len(desc.Entries))
	for _, e := range desc.Entries {
		var res any
		switch {
		case e.Buffer != 0:
			buf, ok := b.buffers[e.Buffer]
			if !ok {
				continue
			}
			bufRes := map[string]any{
				"buffer": buf.gpuBuf,
				"offset": int(e.Offset),
			}
			if e.Size > 0 {
				bufRes["size"] = int(e.Size)
			}
			res = bufRes
		case e.Texture != 0:
			tex, ok := b.textures[e.Texture]
			if !ok {
				continue
			}
			res = tex.gpuView
		case e.Sampler != 0:
			samp, ok := b.samplers[e.Sampler]
			if !ok {
				continue
			}
			res = samp.gpuSampler
		default:
			continue
		}
		entries = append(entries, map[string]any{
			"binding":  int(e.Binding),
			"resource": res,
		})
	}
	bgDesc := map[string]any{
		"layout":  gpuLayout,
		"entries": entries,
	}
	if desc.Label != "" {
		bgDesc["label"] = desc.Label
	}
	return b.device.Call("createBindGroup", bgDesc)
}

func (b *webgpuBackend) resolveBindGroup(bg *webgpuBindGroup, pipe *webgpuPipeline, slot uint32) js.Value {
	if pipe != nil && pipe.hasAutoLayout && bg.desc.Layout == 0 {
		key := (uint64(pipe.id) << 16) | uint64(slot)
		if cached, ok := bg.autoBGs[key]; ok && cached.Truthy() {
			return cached
		}
		autoLayout := pipe.gpuPipe.Call("getBindGroupLayout", int(slot))
		gpuBG := b.createBindGroupWithLayout(autoLayout, bg.desc)
		if bg.autoBGs == nil {
			bg.autoBGs = map[uint64]js.Value{}
		}
		bg.autoBGs[key] = gpuBG
		return gpuBG
	}
	if bg.gpuBG.Truthy() {
		return bg.gpuBG
	}
	if pipe != nil {
		key := (uint64(pipe.id) << 16) | uint64(slot)
		if cached, ok := bg.autoBGs[key]; ok && cached.Truthy() {
			return cached
		}
		pipeLayout := pipe.gpuPipe.Call("getBindGroupLayout", int(slot))
		gpuBG := b.createBindGroupWithLayout(pipeLayout, bg.desc)
		if bg.autoBGs == nil {
			bg.autoBGs = map[uint64]js.Value{}
		}
		bg.autoBGs[key] = gpuBG
		return gpuBG
	}
	return js.Undefined()
}

func (b *webgpuBackend) createPipeline(id driver.GPUID, desc driver.GPUPipelineDesc) {
	compiled, err := compileWebGPUShaders(desc.Vertex, desc.Fragment)
	if err != nil {
		b.g.emitEvent(b.g.ctrl.makeError(driver.GPUErrorShaderCompile, err.Error()))
		return
	}
	vsMod := b.device.Call("createShaderModule", map[string]any{"code": compiled.VertexWGSL})
	var fsMod js.Value
	if compiled.SharedModule {
		fsMod = vsMod
	} else if compiled.FragmentWGSL != "" {
		fsMod = b.device.Call("createShaderModule", map[string]any{"code": compiled.FragmentWGSL})
	}

	var pipeLayout any = "auto"
	hasAutoLayout := true
	if len(desc.Layouts) > 0 {
		hasAutoLayout = false
		bgls := webgpuPipelineBindGroupLayouts(desc.Layouts, func(lid driver.GPUID) (any, bool) {
			l, ok := b.layouts[lid]
			if !ok {
				return nil, false
			}
			return l.gpuLayout, true
		}, func() any { return b.emptyBindGroupLayout() })
		pipeLayout = b.device.Call("createPipelineLayout", map[string]any{
			"bindGroupLayouts": bgls,
		})
	}

	vbLayouts := make([]any, 0, len(desc.VertexBuffers))
	for _, vb := range desc.VertexBuffers {
		attrs := make([]any, 0, len(vb.Attributes))
		for _, a := range vb.Attributes {
			attrs = append(attrs, map[string]any{
				"format":         webgpuVertexFormat(a.Format),
				"offset":         int(a.Offset),
				"shaderLocation": int(a.Location),
			})
		}
		vbLayouts = append(vbLayouts, map[string]any{
			"arrayStride": int(vb.Stride),
			"stepMode":    webgpuVertexStepMode(vb.StepMode),
			"attributes":  attrs,
		})
	}

	pipeDesc := map[string]any{
		"layout": pipeLayout,
		"vertex": map[string]any{
			"module":     vsMod,
			"entryPoint": compiled.VertexEntry,
			"buffers":    vbLayouts,
		},
		"primitive": map[string]any{
			"topology":  webgpuTopology(desc.Primitive.Topology),
			"frontFace": webgpuFrontFace(desc.Primitive.FrontFace),
			"cullMode":  webgpuCullMode(desc.Primitive.CullMode),
		},
		"multisample": map[string]any{
			"count": max(desc.SampleCount, 1),
		},
	}
	if desc.Label != "" {
		pipeDesc["label"] = desc.Label
	}

	if fsMod.Truthy() {
		targets := make([]any, 0, max(len(desc.Targets), 1))
		if len(desc.Targets) == 0 {
			targets = append(targets, map[string]any{
				"format":    b.surfaceFormatStr,
				"writeMask": int(driver.GPUColorWriteAll),
			})
		} else {
			for _, t := range desc.Targets {
				fmtStr := b.surfaceFormatStr
				if t.Format != driver.GPUFormatUndefined {
					fmtStr = wgslTextureFormat(t.Format)
				}
				mask := t.WriteMask
				if mask == 0 {
					mask = driver.GPUColorWriteAll
				}
				// GPUColorWriteNone is a bit outside the channel mask; keep only the channel bits so it disables all writes.
				mask &= driver.GPUColorWriteAll
				tObj := map[string]any{
					"format":    fmtStr,
					"writeMask": int(mask),
				}
				if t.Blend != nil {
					tObj["blend"] = map[string]any{
						"color": map[string]any{
							"srcFactor": webgpuBlendFactor(t.Blend.Color.SrcFactor),
							"dstFactor": webgpuBlendFactor(t.Blend.Color.DstFactor),
							"operation": webgpuBlendOp(t.Blend.Color.Operation),
						},
						"alpha": map[string]any{
							"srcFactor": webgpuBlendFactor(t.Blend.Alpha.SrcFactor),
							"dstFactor": webgpuBlendFactor(t.Blend.Alpha.DstFactor),
							"operation": webgpuBlendOp(t.Blend.Alpha.Operation),
						},
					}
				}
				targets = append(targets, tObj)
			}
		}
		pipeDesc["fragment"] = map[string]any{
			"module":     fsMod,
			"entryPoint": compiled.FragmentEntry,
			"targets":    targets,
		}
	}

	if desc.DepthStencil != nil {
		dfStr := b.depthFormatStr
		if desc.DepthStencil.Format.IsDepth() {
			dfStr = wgslTextureFormat(desc.DepthStencil.Format)
		}
		if dfStr == "" {
			dfStr = "depth24plus"
		}
		pipeDesc["depthStencil"] = map[string]any{
			"format":              dfStr,
			"depthWriteEnabled":   desc.DepthStencil.DepthWrite,
			"depthCompare":        webgpuCompareFunc(desc.DepthStencil.DepthCompare),
			"depthBias":           int(desc.DepthStencil.DepthBias),
			"depthBiasSlopeScale": float64(desc.DepthStencil.DepthBiasSlopeScale),
		}
	}

	gpuPipe := b.device.Call("createRenderPipeline", pipeDesc)
	b.pipelines[id] = &webgpuPipeline{id: id, gpuPipe: gpuPipe, desc: desc, hasAutoLayout: hasAutoLayout}
}

func (b *webgpuBackend) execCopy(enc js.Value, cp driver.GPUCopyOp) {
	switch cp.Kind {
	case driver.GPUCopyBufferToBuffer:
		src, ok1 := b.buffers[cp.SrcID]
		dst, ok2 := b.buffers[cp.DstID]
		if !ok1 || !ok2 {
			return
		}
		enc.Call("copyBufferToBuffer", src.gpuBuf, int(cp.SrcOffset), dst.gpuBuf, int(cp.DstOffset), int(cp.Size))

	case driver.GPUCopyBufferToTexture:
		src, ok1 := b.buffers[cp.SrcID]
		dst, ok2 := b.textures[cp.DstID]
		if !ok1 || !ok2 {
			return
		}
		bpp := dst.desc.Format.BytesPerPixel()
		if bpp == 0 {
			bpp = 4
		}
		bpr := cp.SrcLayout.BytesPerRow
		if bpr == 0 {
			bpr = cp.Width * bpp
		}
		rows := cp.SrcLayout.RowsPerImage
		if rows == 0 {
			rows = cp.Height
		}
		srcObj := map[string]any{
			"buffer":       src.gpuBuf,
			"offset":       int(cp.SrcLayout.Offset),
			"bytesPerRow":  int(bpr),
			"rowsPerImage": int(rows),
		}
		dstObj := map[string]any{
			"texture":  dst.gpuTex,
			"mipLevel": int(cp.DstTex.MipLevel),
			"origin":   []any{int(cp.DstTex.OriginX), int(cp.DstTex.OriginY), int(cp.DstTex.ArrayLayer)},
		}
		enc.Call("copyBufferToTexture", srcObj, dstObj, []any{int(cp.Width), int(cp.Height), int(max(cp.Layers, 1))})

	case driver.GPUCopyTextureToTexture:
		src, ok1 := b.textures[cp.SrcID]
		dst, ok2 := b.textures[cp.DstID]
		if !ok1 || !ok2 {
			return
		}
		srcObj := map[string]any{
			"texture":  src.gpuTex,
			"mipLevel": int(cp.SrcTex.MipLevel),
			"origin":   []any{int(cp.SrcTex.OriginX), int(cp.SrcTex.OriginY), int(cp.SrcTex.ArrayLayer)},
		}
		dstObj := map[string]any{
			"texture":  dst.gpuTex,
			"mipLevel": int(cp.DstTex.MipLevel),
			"origin":   []any{int(cp.DstTex.OriginX), int(cp.DstTex.OriginY), int(cp.DstTex.ArrayLayer)},
		}
		enc.Call("copyTextureToTexture", srcObj, dstObj, []any{int(cp.Width), int(cp.Height), int(max(cp.Layers, 1))})
	}
}

func (b *webgpuBackend) execPass(enc js.Value, pass driver.GPUPass) {
	colorAtts := make([]any, 0, len(pass.ColorAttachments))
	for _, ca := range pass.ColorAttachments {
		var view, resolveView js.Value
		if ca.Texture == 0 {
			swapView := b.ctx.Call("getCurrentTexture").Call("createView")
			if b.sampleCount > 1 && b.msaaColorView.Truthy() {
				view = b.msaaColorView
				resolveView = swapView
			} else {
				view = swapView
			}
		} else if tex, ok := b.textures[ca.Texture]; ok {
			if ca.MipLevel == 0 && ca.ArrayLayer == 0 && tex.desc.Dimension == driver.GPUDim2D {
				view = tex.gpuView
			} else {
				view = tex.gpuTex.Call("createView", map[string]any{
					"dimension":       "2d",
					"baseMipLevel":    int(ca.MipLevel),
					"mipLevelCount":   1,
					"baseArrayLayer":  int(ca.ArrayLayer),
					"arrayLayerCount": 1,
				})
			}
		}
		if !view.Truthy() {
			continue
		}
		if ca.ResolveTarget != 0 {
			if rt, ok := b.textures[ca.ResolveTarget]; ok {
				resolveView = rt.gpuView
			}
		}
		r, g, bl, a := ca.ClearColor.Components()
		att := map[string]any{
			"view":       view,
			"loadOp":     webgpuLoadOp(ca.LoadOp),
			"storeOp":    webgpuStoreOp(ca.StoreOp),
			"clearValue": map[string]any{"r": r, "g": g, "b": bl, "a": a},
		}
		if resolveView.Truthy() {
			att["resolveTarget"] = resolveView
		}
		colorAtts = append(colorAtts, att)
	}

	rpDesc := map[string]any{
		"colorAttachments": colorAtts,
	}

	if pass.DepthStencil != nil {
		ds := pass.DepthStencil
		var dView js.Value
		var dFmt driver.GPUTextureFormat
		if ds.Texture == 0 {
			b.ensureDefaultDepth()
			dView = b.depthView
			dFmt = b.depthFormat
		} else if tex, ok := b.textures[ds.Texture]; ok {
			dView = tex.gpuView
			dFmt = tex.desc.Format
		}
		if dView.Truthy() {
			dsObj := map[string]any{
				"view":            dView,
				"depthLoadOp":     webgpuLoadOp(ds.DepthLoadOp),
				"depthStoreOp":    webgpuStoreOp(ds.DepthStoreOp),
				"depthClearValue": float64(ds.ClearDepth),
			}
			if dFmt == driver.GPUFormatDepth24PlusStencil8 {
				dsObj["stencilLoadOp"] = webgpuLoadOp(ds.StencilLoadOp)
				dsObj["stencilStoreOp"] = webgpuStoreOp(ds.StencilStoreOp)
				dsObj["stencilClearValue"] = int(ds.ClearStencil)
			}
			rpDesc["depthStencilAttachment"] = dsObj
		}
	}

	rp := enc.Call("beginRenderPass", rpDesc)
	var activePipe *webgpuPipeline
	for _, c := range pass.Commands {
		switch c.Kind {
		case driver.GPUCmdSetViewport:
			rp.Call("setViewport", float64(c.F0), float64(c.F1), float64(c.F2), float64(c.F3), float64(c.F4), float64(c.F5))
		case driver.GPUCmdSetScissor:
			sx, sy, sw, sh := clampScissorRect(c.I0, c.I1, c.I2, c.I3)
			rp.Call("setScissorRect", int(sx), int(sy), int(sw), int(sh))
		case driver.GPUCmdSetPipeline:
			if p, ok := b.pipelines[c.ID]; ok {
				activePipe = p
				rp.Call("setPipeline", p.gpuPipe)
			}
		case driver.GPUCmdSetBindGroup:
			if bg, ok := b.bindGroups[c.ID]; ok {
				gpuBG := b.resolveBindGroup(bg, activePipe, c.Slot)
				if !gpuBG.Truthy() {
					continue
				}
				if len(c.DynamicOffsets) > 0 {
					offs := make([]any, len(c.DynamicOffsets))
					for i, o := range c.DynamicOffsets {
						offs[i] = int(o)
					}
					rp.Call("setBindGroup", int(c.Slot), gpuBG, offs)
				} else {
					rp.Call("setBindGroup", int(c.Slot), gpuBG)
				}
			}
		case driver.GPUCmdSetVertexBuffer:
			if buf, ok := b.buffers[c.ID]; ok {
				rp.Call("setVertexBuffer", int(c.Slot), buf.gpuBuf, int(c.Offset))
			}
		case driver.GPUCmdSetIndexBuffer:
			if buf, ok := b.buffers[c.ID]; ok {
				rp.Call("setIndexBuffer", buf.gpuBuf, webgpuIndexFormat(c.IndexFormat), int(c.Offset))
			}
		case driver.GPUCmdSetStencilReference:
			rp.Call("setStencilReference", int(c.U0))
		case driver.GPUCmdSetBlendConstant:
			rp.Call("setBlendConstant", map[string]any{"r": float64(c.F0), "g": float64(c.F1), "b": float64(c.F2), "a": float64(c.F3)})
		case driver.GPUCmdDraw:
			rp.Call("draw", int(c.U0), 1, int(c.U1), 0)
		case driver.GPUCmdDrawIndexed:
			rp.Call("drawIndexed", int(c.U0), 1, int(c.U1), int(c.I0), 0)
		case driver.GPUCmdDrawInstanced:
			rp.Call("draw", int(c.U0), int(c.U1), int(c.U2), int(c.U3))
		case driver.GPUCmdDrawIndexedInstanced:
			rp.Call("drawIndexed", int(c.U0), int(c.U1), int(c.U2), int(c.I0), int(c.U3))
		}
	}
	rp.Call("end")
}

func (b *webgpuBackend) execReadTexture(op driver.GPUResourceOp) {
	var (
		srcTex    js.Value
		fmtKind   driver.GPUTextureFormat
		x, y      uint32
		rw, rh    uint32
		mipLevel  uint32
		arrayLay  uint32
		readbackG = b.g.ctrl.generation
	)
	if op.ReadbackSurface {
		srcTex = b.ctx.Call("getCurrentTexture")
		fmtKind = b.surfaceFormat
		rw, rh = uint32(max(b.width, 1)), uint32(max(b.height, 1))
	} else {
		tex, ok := b.textures[op.ID]
		if !ok {
			b.g.emitEvent(&driver.GPUEvent{
				Kind:       driver.GPUEventReadback,
				Generation: readbackG,
				ReadbackID: op.ReadbackID,
				Message:    fmt.Sprintf("webgpu: readback texture %d not found", op.ID),
			})
			return
		}
		srcTex = tex.gpuTex
		fmtKind = tex.desc.Format
		mipLevel = op.ReadbackRegion.MipLevel
		arrayLay = op.ReadbackRegion.ArrayLayer
		mipW := mipDim(tex.desc.Width, mipLevel)
		mipH := mipDim(tex.desc.Height, mipLevel)
		x, y = op.ReadbackRegion.X, op.ReadbackRegion.Y
		rw, rh = op.ReadbackRegion.Width, op.ReadbackRegion.Height
		if rw == 0 {
			rw = mipW - min(x, mipW)
		}
		if rh == 0 {
			rh = mipH - min(y, mipH)
		}
	}

	if rw == 0 || rh == 0 {
		b.g.emitEvent(&driver.GPUEvent{
			Kind:       driver.GPUEventReadback,
			Generation: readbackG,
			ReadbackID: op.ReadbackID,
		})
		return
	}

	bpp := fmtKind.BytesPerPixel()
	if bpp == 0 {
		bpp = 4
	}
	alignedBPR := webgpuReadbackBytesPerRow(rw, bpp)
	bufSize := alignedBPR * rh
	staging := b.device.Call("createBuffer", map[string]any{
		"size":  int(bufSize),
		"usage": int(webgpuBufCopyDst | webgpuBufMapRead),
	})

	enc := b.device.Call("createCommandEncoder")
	enc.Call("copyTextureToBuffer",
		map[string]any{
			"texture":  srcTex,
			"mipLevel": int(mipLevel),
			"origin":   []any{int(x), int(y), int(arrayLay)},
		},
		map[string]any{
			"buffer":       staging,
			"offset":       0,
			"bytesPerRow":  int(alignedBPR),
			"rowsPerImage": int(rh),
		},
		[]any{int(rw), int(rh), 1},
	)
	b.queue.Call("submit", []any{enc.Call("finish")})

	mapPromise := staging.Call("mapAsync", int(webgpuMapRead))
	swizzle := fmtKind == driver.GPUFormatBGRA8Unorm
	b.g.promiseThen(mapPromise, func(js.Value) {
		if b.destroyed || b.g.ctrl.state == gpuStateDestroyed {
			staging.Call("destroy")
			return
		}
		mapped := js.Global().Get("Uint8Array").New(staging.Call("getMappedRange"))
		raw := make([]byte, int(bufSize))
		js.CopyBytesToGo(raw, mapped)
		staging.Call("unmap")
		staging.Call("destroy")
		pixels := stripRowPadding(raw, int(rw), int(rh), int(bpp), int(alignedBPR), swizzle)
		b.g.emitEvent(&driver.GPUEvent{
			Kind:       driver.GPUEventReadback,
			Generation: readbackG,
			ReadbackID: op.ReadbackID,
			Pixels:     pixels,
		})
	}, func(errVal js.Value) {
		staging.Call("destroy")
		if b.destroyed || b.g.ctrl.state == gpuStateDestroyed {
			return
		}
		b.g.emitEvent(&driver.GPUEvent{
			Kind:       driver.GPUEventReadback,
			Generation: readbackG,
			ReadbackID: op.ReadbackID,
			Message:    jsErrorMessage(errVal),
		})
	})
}

func (b *webgpuBackend) cleanupResources(destroyDevice bool) {
	b.destroyed = true
	b.devListeners.release()
	for id, buf := range b.buffers {
		buf.gpuBuf.Call("destroy")
		delete(b.buffers, id)
	}
	for id, tex := range b.textures {
		tex.gpuTex.Call("destroy")
		delete(b.textures, id)
	}
	clear(b.samplers)
	clear(b.layouts)
	clear(b.bindGroups)
	clear(b.pipelines)
	b.destroySwapchainAttachments()
	if b.ctx.Truthy() {
		if fn := b.ctx.Get("unconfigure"); fn.Type() == js.TypeFunction {
			b.ctx.Call("unconfigure")
		}
		b.ctx = js.Undefined()
	}
	if destroyDevice && b.device.Truthy() {
		b.device.Call("destroy")
	}
	b.device = js.Undefined()
	b.queue = js.Undefined()
}

func (b *webgpuBackend) destroy() {
	b.cleanupResources(true)
}

// emptyBindGroupLayout returns the device's zero-entry bind group layout, creating it on first use.
func (b *webgpuBackend) emptyBindGroupLayout() js.Value {
	if !b.emptyLayout.Truthy() {
		b.emptyLayout = b.device.Call("createBindGroupLayout", map[string]any{"entries": []any{}})
	}
	return b.emptyLayout
}
