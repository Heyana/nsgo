//go:build darwin

package apple

import (
	"fmt"
	"math"
	"runtime"
	"slices"
	"structs"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
	"github.com/nsgo-dev/nsgo/internal/shaderir/msl"
)

// A gpu.View on macOS and iOS draws with Metal (docs/architecture/gpu.md). Its GPULayer is a CAMetalLayer whose render goroutine owns every Metal resource the view's frames create: frames arrive from the engine goroutine through Submit, independent of view-tree commits, and run in order on the layer's command queue. Passes that target the view draw into a persistent surface texture, which a presenting frame copies into the layer's next drawable, so a pass that loads the surface sees the last frame and ReadSurface reads what the view shows.

// GPUSupported reports whether the machine has a Metal device for gpu.View. Call it on the main thread.
func GPUSupported() bool { return loadMetal() }

// gpuGenerations numbers the devices of every GPU layer, so a frame recorded for a released layer's device never runs on a later one's.
var gpuGenerations atomic.Uint32

// GPULayer is a gpu.View's Metal surface.
type GPULayer struct {
	Layer         objc.ID
	render        *gpuRender
	width, height int // the drawable's pixel size, on the main thread
	ready         bool
}

type gpuRender struct {
	layer, queue objc.ID
	cfg          driver.GPUViewConfig
	format       driver.GPUTextureFormat // the surface's color format
	generation   uint32
	emit         func(*driver.GPUEvent)
	wake         chan struct{}
	done         chan struct{}

	mu            sync.Mutex
	pending       []driver.GPUFrame
	closed        bool
	width, height int // the surface's pixel size, which the main thread sets

	// The rest belongs to the render goroutine.
	buffers   map[driver.GPUID]objc.ID
	textures  map[driver.GPUID]*gpuTexture
	samplers  map[driver.GPUID]objc.ID
	layouts   map[driver.GPUID]*driver.GPUBindGroupLayoutDesc
	groups    map[driver.GPUID]*driver.GPUBindGroupDesc
	pipelines map[driver.GPUID]*gpuPipeline
	// surface is what passes targeting the view draw into, resolved from msaa when the view samples more than once; depth is its depth buffer. fresh says their contents are undefined until a pass clears them.
	surface, msaa, depth *gpuTexture
	freshColor           bool
	freshDepth           bool
	last                 objc.ID // the command buffer committed last
}

type gpuTexture struct {
	object objc.ID
	desc   driver.GPUTextureDesc
}

type gpuPipeline struct {
	desc             *driver.GPUPipelineDesc
	vertex, fragment objc.ID // MTLFunctions
	slots            []msl.BindingSlot
	depthState       objc.ID
	// states are the pipeline's Metal states by the depth attachment's pixel format, which Metal fixes in each.
	states map[uint]objc.ID
}

func (p *gpuPipeline) release() {
	p.vertex.Send(selRelease)
	p.fragment.Send(selRelease)
	p.depthState.Send(selRelease)
	for _, s := range p.states {
		s.Send(selRelease)
	}
}

// NewGPULayer makes a GPU view's layer on the main thread, or returns nil where the machine has no Metal device. emit reports the view's events from any goroutine.
func NewGPULayer(cfg driver.GPUViewConfig, emit func(*driver.GPUEvent)) *GPULayer {
	if !GPUSupported() {
		return nil
	}
	queue := mtlDevice.Send(sel("newCommandQueue"))
	if queue == 0 {
		return nil
	}
	format := driver.GPUFormatBGRA8Unorm
	if cfg.ColorFormat == driver.GPUFormatRGBA16Float {
		format = cfg.ColorFormat
	}
	if cfg.SampleCount != 4 {
		cfg.SampleCount = 1
	}
	layer := objc.ID(Class("CAMetalLayer")).Send(selNew)
	layer.Send(sel("setDevice:"), mtlDevice)
	layer.Send(sel("setPixelFormat:"), gpuPixelFormat(format))
	// A presenting frame copies the surface into the drawable.
	layer.Send(sel("setFramebufferOnly:"), false)
	layer.Send(sel("setOpaque:"), false)
	layer.Send(sel("setColorspace:"), srgb)
	r := &gpuRender{
		layer: layer, queue: queue, cfg: cfg, format: format, generation: gpuGenerations.Add(1), emit: emit,
		wake: make(chan struct{}, 1), done: make(chan struct{}),
		buffers: map[driver.GPUID]objc.ID{}, textures: map[driver.GPUID]*gpuTexture{}, samplers: map[driver.GPUID]objc.ID{},
		layouts: map[driver.GPUID]*driver.GPUBindGroupLayoutDesc{}, groups: map[driver.GPUID]*driver.GPUBindGroupDesc{},
		pipelines: map[driver.GPUID]*gpuPipeline{},
	}
	go r.loop()
	return &GPULayer{Layer: layer, render: r}
}

// Resize sizes the layer's drawable for w by h points at scale, on the main thread. The first nonzero size makes the view's device ready; later ones report a resize.
func (l *GPULayer) Resize(w, h, scale float64) {
	l.Layer.Send(sel("setContentsScale:"), scale)
	width, height := int(math.Round(w*scale)), int(math.Round(h*scale))
	if width <= 0 || height <= 0 || l.ready && width == l.width && height == l.height {
		return
	}
	l.width, l.height = width, height
	l.Layer.Send(sel("setDrawableSize:"), CGSize{Width: float64(width), Height: float64(height)})
	r := l.render
	r.mu.Lock()
	r.width, r.height = width, height
	r.mu.Unlock()
	ev := &driver.GPUEvent{Kind: driver.GPUEventResize, Generation: r.generation, Width: width, Height: height}
	if !l.ready {
		l.ready = true
		ev.Kind = driver.GPUEventDeviceReady
		ev.SurfaceFormat = r.format
		ev.Limits = driver.DefaultGPULimits()
		ev.Limits.MaxTextureDimension2D = 8192
		ev.Features = driver.GPUFeatures{StorageBuffers: true, MSAA4x: true, Depth32Float: true, Anisotropy: true}
	}
	r.emit(ev)
}

// Submit queues frame for the render goroutine, from any goroutine. A frame recorded for another device is dropped.
func (l *GPULayer) Submit(frame driver.GPUFrame) {
	r := l.render
	if frame.Generation != 0 && frame.Generation != r.generation {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.pending = append(r.pending, frame)
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// Release detaches the layer, on the main thread; its render goroutine then lets every Metal resource go.
func (l *GPULayer) Release() {
	l.Layer.Send(sel("removeFromSuperlayer"))
	r := l.render
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		r.pending = nil
		close(r.wake)
	}
	r.mu.Unlock()
}

func (r *gpuRender) fail(code driver.GPUErrorCode, format string, args ...any) {
	r.emit(&driver.GPUEvent{Kind: driver.GPUEventError, Generation: r.generation, ErrorCode: code, Message: fmt.Sprintf(format, args...)})
}

func (r *gpuRender) loop() {
	defer close(r.done)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for range r.wake {
		r.mu.Lock()
		frames := r.pending
		r.pending = nil
		r.mu.Unlock()
		for _, f := range frames {
			r.mu.Lock()
			closed := r.closed
			r.mu.Unlock()
			if closed {
				break
			}
			pool := poolPush()
			r.run(f)
			poolPop(pool)
		}
	}
	pool := poolPush()
	defer poolPop(pool)
	if r.last != 0 {
		r.last.Send(sel("waitUntilCompleted"))
		r.last.Send(selRelease)
	}
	for _, b := range r.buffers {
		b.Send(selRelease)
	}
	for _, t := range r.textures {
		t.object.Send(selRelease)
	}
	for _, s := range r.samplers {
		s.Send(selRelease)
	}
	for _, p := range r.pipelines {
		p.release()
	}
	clear(r.buffers)
	clear(r.textures)
	clear(r.samplers)
	clear(r.layouts)
	clear(r.groups)
	clear(r.pipelines)
	for _, t := range []*gpuTexture{r.surface, r.msaa, r.depth} {
		if t != nil {
			t.object.Send(selRelease)
		}
	}
	r.surface, r.msaa, r.depth = nil, nil, nil
	r.layer.Send(selRelease)
	r.queue.Send(selRelease)
}

// Metal's enumerations, as MTLPixelFormat, MTLTextureType and the others name them.
const (
	mtlPixelFormatRGBA8UnormSRGB      = 71
	mtlPixelFormatRG8Unorm            = 30
	mtlPixelFormatR8Unorm             = 10
	mtlPixelFormatRGBA16Float         = 115
	mtlPixelFormatDepth32Float        = 252
	mtlPixelFormatDepth32FloatStencil = 260

	mtlTextureType2D            = 2
	mtlTextureType2DArray       = 3
	mtlTextureType2DMultisample = 4
	mtlTextureTypeCube          = 5

	mtlTextureUsageShaderRead   = 1
	mtlTextureUsageRenderTarget = 4
	mtlStorageModePrivate       = 2

	mtlLoadActionLoad                = 1
	mtlStoreActionDontCare           = 0
	mtlStoreActionMultisampleResolve = 2
	mtlStoreActionStoreAndResolve    = 3
	mtlCommandBufferStatusCompleted  = 4
	mtlCommandBufferStatusError      = 5
	mtlCommandBufferErrorOutOfMemory = 8
	mtlVertexStepFunctionPerVertex   = 1
	mtlVertexStepFunctionPerInstance = 2
	mtlSamplerMipFilterNearest       = 1 // MTLSamplerMipFilter has NotMipmapped first
	mtlReadbackAlignment             = 256
)

func gpuPixelFormat(f driver.GPUTextureFormat) uint {
	switch f {
	case driver.GPUFormatRGBA8Unorm:
		return mtlPixelFormatRGBA8Unorm
	case driver.GPUFormatRGBA8UnormSRGB:
		return mtlPixelFormatRGBA8UnormSRGB
	case driver.GPUFormatBGRA8Unorm:
		return mtlPixelFormatBGRA8Unorm
	case driver.GPUFormatRG8Unorm:
		return mtlPixelFormatRG8Unorm
	case driver.GPUFormatR8Unorm:
		return mtlPixelFormatR8Unorm
	case driver.GPUFormatRGBA16Float:
		return mtlPixelFormatRGBA16Float
	case driver.GPUFormatDepth24Plus, driver.GPUFormatDepth32Float:
		// Apple GPUs have no 24-bit depth; WebGPU lets Depth24Plus be wider.
		return mtlPixelFormatDepth32Float
	case driver.GPUFormatDepth24PlusStencil8:
		return mtlPixelFormatDepth32FloatStencil
	default:
		return 0
	}
}

// gpuCompare maps a comparison onto MTLCompareFunction, which lacks driver's leading Undefined.
func gpuCompare(c driver.GPUCompareFunc, undefined uint) uint {
	if c == driver.GPUCompareUndefined {
		return undefined
	}
	return uint(c) - 1
}

func gpuNewTexture(desc driver.GPUTextureDesc) objc.ID {
	d := objc.ID(Class("MTLTextureDescriptor")).Send(selNew)
	defer d.Send(selRelease)
	kind, layers := uint(mtlTextureType2D), uint(1)
	switch {
	case desc.SampleCount > 1:
		kind = mtlTextureType2DMultisample
		d.Send(sel("setSampleCount:"), uint(desc.SampleCount))
	case desc.Dimension == driver.GPUDimCube:
		kind = mtlTextureTypeCube
	case desc.Dimension == driver.GPUDim2DArray:
		kind, layers = mtlTextureType2DArray, uint(max(desc.Layers, 1))
	}
	d.Send(sel("setTextureType:"), kind)
	d.Send(sel("setPixelFormat:"), gpuPixelFormat(desc.Format))
	d.Send(sel("setWidth:"), uint(desc.Width))
	d.Send(sel("setHeight:"), uint(desc.Height))
	d.Send(sel("setArrayLength:"), layers)
	if desc.SampleCount <= 1 {
		d.Send(sel("setMipmapLevelCount:"), uint(max(desc.MipLevels, 1)))
	}
	var usage uint
	if desc.Usage&driver.GPUTextureSampled != 0 || desc.Usage&driver.GPUTextureRenderAttachment == 0 {
		usage |= mtlTextureUsageShaderRead
	}
	if desc.Usage&driver.GPUTextureRenderAttachment != 0 {
		usage |= mtlTextureUsageRenderTarget
	}
	d.Send(sel("setUsage:"), usage)
	// Uploads and readbacks blit through buffers, so no texture needs the CPU.
	d.Send(sel("setStorageMode:"), uint(mtlStorageModePrivate))
	return mtlDevice.Send(sel("newTextureWithDescriptor:"), d)
}

func gpuNewSampler(s *driver.GPUSamplerDesc) objc.ID {
	d := objc.ID(Class("MTLSamplerDescriptor")).Send(selNew)
	defer d.Send(selRelease)
	d.Send(sel("setMinFilter:"), uint(s.MinFilter))
	d.Send(sel("setMagFilter:"), uint(s.MagFilter))
	d.Send(sel("setMipFilter:"), uint(s.MipmapFilter)+mtlSamplerMipFilterNearest)
	address := [...]uint{0, 2, 3} // ClampToEdge, Repeat, MirrorRepeat
	if int(s.AddressU) < len(address) && int(s.AddressV) < len(address) && int(s.AddressW) < len(address) {
		d.Send(sel("setSAddressMode:"), address[s.AddressU])
		d.Send(sel("setTAddressMode:"), address[s.AddressV])
		d.Send(sel("setRAddressMode:"), address[s.AddressW])
	}
	if s.Compare != driver.GPUCompareUndefined {
		d.Send(sel("setCompareFunction:"), gpuCompare(s.Compare, 0))
	}
	if s.MaxAnisotropy > 1 {
		d.Send(sel("setMaxAnisotropy:"), uint(min(s.MaxAnisotropy, 16)))
	}
	return mtlDevice.Send(sel("newSamplerStateWithDescriptor:"), d)
}

// gpuProgram is the program a pipeline's stages compile from: one module's, or the vertex module's with the fragment module's entry, as the software device merges them.
func gpuProgram(p *driver.GPUPipelineDesc) *shaderir.Program {
	vs, fs := p.Vertex.IR, p.Fragment.IR
	if fs == nil || vs == fs {
		return vs
	}
	out := *vs
	out.FragmentFunc = fs.FragmentFunc
	out.FragmentEntry = fs.FragmentEntry
	if len(fs.Varyings) > len(out.Varyings) {
		out.Varyings, out.VaryingMeta = fs.Varyings, fs.VaryingMeta
	}
	return &out
}

func gpuNewPipeline(p *driver.GPUPipelineDesc) (*gpuPipeline, error) {
	prog := gpuProgram(p)
	if prog == nil || prog.VertexFunc.Block == nil {
		return nil, fmt.Errorf("the pipeline has no vertex stage")
	}
	src, err := msl.CompilePipeline(prog)
	if err != nil {
		return nil, err
	}
	var errObj objc.ID
	lib := mtlDevice.Send(sel("newLibraryWithSource:options:error:"), NSString(src.Source), objc.ID(0), unsafe.Pointer(&errObj))
	if lib == 0 {
		return nil, fmt.Errorf("%s", errorText(errObj))
	}
	defer lib.Send(selRelease)
	gp := &gpuPipeline{desc: p, slots: src.Slots, states: map[uint]objc.ID{}}
	gp.vertex = lib.Send(sel("newFunctionWithName:"), NSString(src.VertexEntry))
	if p.Fragment.IR != nil {
		gp.fragment = lib.Send(sel("newFunctionWithName:"), NSString(src.FragmentEntry))
	}
	if gp.vertex == 0 || p.Fragment.IR != nil && gp.fragment == 0 {
		gp.release()
		return nil, fmt.Errorf("the Metal library lacks the pipeline's entry points")
	}
	if ds := p.DepthStencil; ds != nil {
		d := objc.ID(Class("MTLDepthStencilDescriptor")).Send(selNew)
		d.Send(sel("setDepthCompareFunction:"), gpuCompare(ds.DepthCompare, 1))
		d.Send(sel("setDepthWriteEnabled:"), ds.DepthWrite)
		gp.depthState = mtlDevice.Send(sel("newDepthStencilStateWithDescriptor:"), d)
		d.Send(selRelease)
	}
	return gp, nil
}

// state returns p's Metal state for a pass whose depth attachment has pixel format depth (0 for none), making it the first time.
func (p *gpuPipeline) state(depth uint) (objc.ID, error) {
	if s, ok := p.states[depth]; ok {
		return s, nil
	}
	d := objc.ID(Class("MTLRenderPipelineDescriptor")).Send(selNew)
	defer d.Send(selRelease)
	d.Send(sel("setVertexFunction:"), p.vertex)
	d.Send(sel("setFragmentFunction:"), p.fragment)
	d.Send(sel("setRasterSampleCount:"), uint(max(p.desc.SampleCount, 1)))
	for i, t := range p.desc.Targets {
		a := d.Send(selColorAttachments).Send(selObjectAtIndexedSubscript, uint(i))
		a.Send(sel("setPixelFormat:"), gpuPixelFormat(t.Format))
		// MTLColorWriteMask counts its channels from alpha up.
		var mask uint
		for bit := range 4 {
			if t.WriteMask&(1<<bit) != 0 {
				mask |= 8 >> bit
			}
		}
		a.Send(sel("setWriteMask:"), mask)
		if b := t.Blend; b != nil {
			a.Send(sel("setBlendingEnabled:"), true)
			a.Send(sel("setSourceRGBBlendFactor:"), gpuBlendFactor(b.Color.SrcFactor))
			a.Send(sel("setDestinationRGBBlendFactor:"), gpuBlendFactor(b.Color.DstFactor))
			a.Send(sel("setRgbBlendOperation:"), uint(b.Color.Operation))
			a.Send(sel("setSourceAlphaBlendFactor:"), gpuBlendFactor(b.Alpha.SrcFactor))
			a.Send(sel("setDestinationAlphaBlendFactor:"), gpuBlendFactor(b.Alpha.DstFactor))
			a.Send(sel("setAlphaBlendOperation:"), uint(b.Alpha.Operation))
		}
	}
	d.Send(sel("setDepthAttachmentPixelFormat:"), depth)
	if depth == mtlPixelFormatDepth32FloatStencil {
		d.Send(sel("setStencilAttachmentPixelFormat:"), depth)
	}
	if len(p.desc.VertexBuffers) > 0 {
		vd := objc.ID(Class("MTLVertexDescriptor")).Send(sel("vertexDescriptor"))
		formats := [...]uint{0, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 9}
		for i, b := range p.desc.VertexBuffers {
			l := vd.Send(sel("layouts")).Send(selObjectAtIndexedSubscript, uint(i))
			l.Send(sel("setStride:"), uint(b.Stride))
			if b.StepMode == driver.GPUStepInstance {
				l.Send(sel("setStepFunction:"), uint(mtlVertexStepFunctionPerInstance))
			} else {
				l.Send(sel("setStepFunction:"), uint(mtlVertexStepFunctionPerVertex))
			}
			for _, a := range b.Attributes {
				if int(a.Format) >= len(formats) {
					return 0, fmt.Errorf("vertex attribute %d has an unknown format", a.Location)
				}
				at := vd.Send(sel("attributes")).Send(selObjectAtIndexedSubscript, uint(a.Location))
				at.Send(sel("setBufferIndex:"), uint(i))
				at.Send(sel("setOffset:"), uint(a.Offset))
				at.Send(sel("setFormat:"), formats[a.Format])
			}
		}
		d.Send(sel("setVertexDescriptor:"), vd)
	}
	var errObj objc.ID
	s := mtlDevice.Send(sel("newRenderPipelineStateWithDescriptor:error:"), d, unsafe.Pointer(&errObj))
	if s == 0 {
		return 0, fmt.Errorf("%s", errorText(errObj))
	}
	p.states[depth] = s
	return s, nil
}

func gpuBlendFactor(f driver.GPUBlendFactor) uint {
	switch f {
	case driver.GPUBlendConstant:
		return 11 // MTLBlendFactorBlendColor
	case driver.GPUBlendOneMinusConstant:
		return 12
	default:
		return uint(f)
	}
}

func gpuBytes(buffer objc.ID, length int) []byte {
	p := objc.Send[unsafe.Pointer](buffer, sel("contents"))
	return unsafe.Slice((*byte)(p), length)
}

type gpuOrigin struct {
	_       structs.HostLayout
	X, Y, Z uint
}

type gpuViewport struct {
	_                                       structs.HostLayout
	X, Y, Width, Height, MinDepth, MaxDepth float64
}

type gpuScissor struct {
	_                   structs.HostLayout
	X, Y, Width, Height uint
}

// gpuReadback is a texture region copied into a buffer, which becomes a readback event once its command buffer completes.
type gpuReadback struct {
	id        uint64
	buffer    objc.ID
	row, rows uint
	stride    uint
	format    driver.GPUTextureFormat
	failure   string
}

// frameRun is one frame's command buffer and the blit encoder its uploads and copies share.
type frameRun struct {
	r         *gpuRender
	cb, blit  objc.ID
	readbacks []gpuReadback
}

func (f *frameRun) commandBuffer() objc.ID {
	if f.cb == 0 {
		f.cb = f.r.queue.Send(sel("commandBuffer"))
	}
	return f.cb
}

func (f *frameRun) blitter() objc.ID {
	if f.blit == 0 {
		f.blit = f.commandBuffer().Send(sel("blitCommandEncoder"))
	}
	return f.blit
}

func (f *frameRun) endBlit() {
	if f.blit != 0 {
		f.blit.Send(sel("endEncoding"))
		f.blit = 0
	}
}

// staging returns a shared buffer holding data, which the command buffer keeps while it runs.
func (f *frameRun) staging(data []byte) objc.ID {
	b := mtlDevice.Send(sel("newBufferWithBytes:length:options:"), unsafe.Pointer(&data[0]), uint(len(data)), uint(0))
	if b != 0 {
		b.Send(sel("autorelease"))
	}
	return b
}

// run executes one frame: its resource operations, then its copies, then its passes, then presents the surface.
func (r *gpuRender) run(frame driver.GPUFrame) {
	r.mu.Lock()
	width, height := r.width, r.height
	r.mu.Unlock()
	r.sizeSurface(width, height)
	f := &frameRun{r: r}
	for i := range frame.Ops {
		r.op(f, &frame.Ops[i])
	}
	for i := range frame.Copies {
		r.copyOp(f, &frame.Copies[i])
	}
	f.endBlit()
	for i := range frame.Passes {
		r.pass(f, &frame.Passes[i])
	}
	if frame.Present && len(frame.Passes) > 0 && r.surface != nil {
		r.present(f)
	}
	ok := true
	if f.cb != 0 {
		r.commit(f.cb)
		if len(f.readbacks) > 0 {
			f.cb.Send(sel("waitUntilCompleted"))
			ok = f.cb.Send(sel("status")) == mtlCommandBufferStatusCompleted
		}
	}
	for _, rb := range f.readbacks {
		ev := &driver.GPUEvent{Kind: driver.GPUEventReadback, Generation: r.generation, ReadbackID: rb.id, Message: rb.failure}
		switch {
		case rb.failure != "":
		case !ok:
			ev.Message = "Metal could not read the texture back: " + errorText(f.cb.Send(sel("error")))
		default:
			raw := gpuBytes(rb.buffer, int(rb.stride*rb.rows))
			ev.Pixels = make([]byte, rb.row*rb.rows)
			for y := range rb.rows {
				copy(ev.Pixels[y*rb.row:(y+1)*rb.row], raw[y*rb.stride:y*rb.stride+rb.row])
			}
			if rb.format == driver.GPUFormatBGRA8Unorm {
				for i := 0; i < len(ev.Pixels); i += 4 {
					ev.Pixels[i], ev.Pixels[i+2] = ev.Pixels[i+2], ev.Pixels[i]
				}
			}
		}
		if rb.buffer != 0 {
			rb.buffer.Send(selRelease)
		}
		r.emit(ev)
	}
}

// sizeSurface keeps the surface textures at the drawable's size.
func (r *gpuRender) sizeSurface(width, height int) {
	if width <= 0 || height <= 0 || r.surface != nil && int(r.surface.desc.Width) == width && int(r.surface.desc.Height) == height {
		return
	}
	for _, t := range []*gpuTexture{r.surface, r.msaa, r.depth} {
		if t != nil {
			t.object.Send(selRelease)
		}
	}
	r.surface, r.msaa, r.depth = nil, nil, nil
	alloc := func(desc driver.GPUTextureDesc) *gpuTexture {
		desc.Width, desc.Height, desc.Layers, desc.MipLevels = uint32(width), uint32(height), 1, 1
		if o := gpuNewTexture(desc); o != 0 {
			return &gpuTexture{object: o, desc: desc}
		}
		r.fail(driver.GPUErrorOutOfMemory, "Metal could not allocate the %dx%d surface", width, height)
		return nil
	}
	attachment := driver.GPUTextureRenderAttachment | driver.GPUTextureSampled
	r.surface = alloc(driver.GPUTextureDesc{Format: r.format, Usage: attachment, SampleCount: 1})
	if r.cfg.SampleCount > 1 {
		r.msaa = alloc(driver.GPUTextureDesc{Format: r.format, Usage: driver.GPUTextureRenderAttachment, SampleCount: r.cfg.SampleCount})
	}
	if r.cfg.DepthFormat.IsDepth() {
		r.depth = alloc(driver.GPUTextureDesc{Format: r.cfg.DepthFormat, Usage: driver.GPUTextureRenderAttachment, SampleCount: r.cfg.SampleCount})
	}
	r.freshColor, r.freshDepth = true, true
}

func (r *gpuRender) op(f *frameRun, op *driver.GPUResourceOp) {
	switch op.Kind {
	case driver.GPUOpCreateBuffer:
		if op.Buffer == nil {
			return
		}
		size := max(op.Buffer.Size, uint64(len(op.Buffer.Data)), 4)
		b := mtlDevice.Send(sel("newBufferWithLength:options:"), uint(size), uint(0))
		if b == 0 {
			r.fail(driver.GPUErrorOutOfMemory, "Metal could not allocate a %d-byte buffer", size)
			return
		}
		// Nothing has used the buffer yet, so its first contents go in directly.
		copy(gpuBytes(b, int(size)), op.Buffer.Data)
		r.replace(r.buffers, op.ID, b)
	case driver.GPUOpWriteBuffer:
		b := r.buffers[op.ID]
		if b == 0 || len(op.Data) == 0 {
			return
		}
		if op.Offset+uint64(len(op.Data)) > uint64(b.Send(sel("length"))) {
			r.fail(driver.GPUErrorInvalidDescriptor, "a write to buffer %d runs past its end", op.ID)
			return
		}
		// A blit keeps the write after the frames already committed, which may still read the buffer.
		if s := f.staging(op.Data); s != 0 {
			f.blitter().Send(sel("copyFromBuffer:sourceOffset:toBuffer:destinationOffset:size:"), s, uint(0), b, uint(op.Offset), uint(len(op.Data)))
		}
	case driver.GPUOpDestroyBuffer:
		r.buffers[op.ID].Send(selRelease)
		delete(r.buffers, op.ID)
	case driver.GPUOpCreateTexture:
		if op.Texture == nil {
			return
		}
		o := gpuNewTexture(*op.Texture)
		if o == 0 {
			r.fail(driver.GPUErrorOutOfMemory, "Metal could not allocate a %dx%d texture", op.Texture.Width, op.Texture.Height)
			return
		}
		if old := r.textures[op.ID]; old != nil {
			old.object.Send(selRelease)
		}
		r.textures[op.ID] = &gpuTexture{object: o, desc: *op.Texture}
	case driver.GPUOpWriteTexture:
		r.writeTexture(f, op)
	case driver.GPUOpDestroyTexture:
		if t := r.textures[op.ID]; t != nil {
			t.object.Send(selRelease)
		}
		delete(r.textures, op.ID)
	case driver.GPUOpCreateSampler:
		if op.Sampler == nil {
			return
		}
		r.replace(r.samplers, op.ID, gpuNewSampler(op.Sampler))
	case driver.GPUOpDestroySampler:
		r.samplers[op.ID].Send(selRelease)
		delete(r.samplers, op.ID)
	case driver.GPUOpCreateBindGroupLayout:
		r.layouts[op.ID] = op.BindGroupLayout
	case driver.GPUOpDestroyBindGroupLayout:
		delete(r.layouts, op.ID)
	case driver.GPUOpCreateBindGroup:
		r.groups[op.ID] = op.BindGroup
	case driver.GPUOpDestroyBindGroup:
		delete(r.groups, op.ID)
	case driver.GPUOpCreatePipeline:
		if op.Pipeline == nil {
			return
		}
		p, err := gpuNewPipeline(op.Pipeline)
		if err != nil {
			r.fail(driver.GPUErrorShaderCompile, "Metal could not compile pipeline %q: %v", op.Pipeline.Label, err)
			return
		}
		if old := r.pipelines[op.ID]; old != nil {
			old.release()
		}
		r.pipelines[op.ID] = p
	case driver.GPUOpDestroyPipeline:
		if p := r.pipelines[op.ID]; p != nil {
			p.release()
		}
		delete(r.pipelines, op.ID)
	case driver.GPUOpReadTexture:
		r.readTexture(f, op)
	}
}

func (r *gpuRender) replace(m map[driver.GPUID]objc.ID, id driver.GPUID, o objc.ID) {
	if o == 0 {
		r.fail(driver.GPUErrorOutOfMemory, "Metal could not allocate GPU resource %d", id)
		return
	}
	m[id].Send(selRelease)
	m[id] = o
}

// mipSize is a texture's size at level.
func mipSize(desc driver.GPUTextureDesc, level uint32) (uint32, uint32) {
	return max(desc.Width>>level, 1), max(desc.Height>>level, 1)
}

func (r *gpuRender) writeTexture(f *frameRun, op *driver.GPUResourceOp) {
	t := r.textures[op.TextureDst.Texture]
	if t == nil {
		return
	}
	bpp := t.desc.Format.BytesPerPixel()
	data := op.Data
	if op.DataLayout.Offset >= uint64(len(data)) || bpp == 0 {
		return
	}
	data = data[op.DataLayout.Offset:]
	w, h := mipSize(t.desc, op.TextureDst.MipLevel)
	ox, oy := op.TextureDst.OriginX, op.TextureDst.OriginY
	if ox >= w || oy >= h {
		return
	}
	stride := op.DataLayout.BytesPerRow
	if stride == 0 {
		stride = (w - ox) * bpp
	}
	width := min(stride/bpp, w-ox)
	if width == 0 || uint64(width*bpp) > uint64(len(data)) {
		return
	}
	// The rows the data holds whole, up to the texture's edge.
	rows := min(uint32((uint64(len(data))-uint64(width*bpp))/uint64(stride))+1, h-oy)
	if op.DataLayout.RowsPerImage > 0 {
		rows = min(rows, op.DataLayout.RowsPerImage)
	}
	s := f.staging(data)
	if s == 0 {
		return
	}
	f.blitter().Send(sel("copyFromBuffer:sourceOffset:sourceBytesPerRow:sourceBytesPerImage:sourceSize:toTexture:destinationSlice:destinationLevel:destinationOrigin:"),
		s, uint(0), uint(stride), uint(stride*rows), gpuOrigin{X: uint(width), Y: uint(rows), Z: 1},
		t.object, uint(op.TextureDst.ArrayLayer), uint(op.TextureDst.MipLevel), gpuOrigin{X: uint(ox), Y: uint(oy)})
}

func (r *gpuRender) readTexture(f *frameRun, op *driver.GPUResourceOp) {
	rb := gpuReadback{id: op.ReadbackID}
	defer func() { f.readbacks = append(f.readbacks, rb) }()
	t := r.surface
	if !op.ReadbackSurface && op.ID != 0 {
		t = r.textures[op.ID]
	}
	if t == nil {
		rb.failure = fmt.Sprintf("there is no texture %d to read", op.ID)
		return
	}
	rb.format = t.desc.Format
	switch t.desc.Format {
	case driver.GPUFormatRGBA8Unorm, driver.GPUFormatRGBA8UnormSRGB, driver.GPUFormatBGRA8Unorm:
	default:
		rb.failure = "Metal reads back only 8-bit RGBA and BGRA textures"
		return
	}
	if t.desc.SampleCount > 1 {
		rb.failure = "Metal cannot read back a multisampled texture"
		return
	}
	reg := op.ReadbackRegion
	tw, th := mipSize(t.desc, reg.MipLevel)
	if reg.Width == 0 {
		reg.Width = tw - min(reg.X, tw)
	}
	if reg.Height == 0 {
		reg.Height = th - min(reg.Y, th)
	}
	if reg.Width == 0 || reg.Height == 0 || reg.X+reg.Width > tw || reg.Y+reg.Height > th {
		rb.failure = "the readback region is out of bounds"
		return
	}
	rb.row, rb.rows = uint(reg.Width)*4, uint(reg.Height)
	rb.stride = (rb.row + mtlReadbackAlignment - 1) &^ (mtlReadbackAlignment - 1)
	rb.buffer = mtlDevice.Send(sel("newBufferWithLength:options:"), rb.stride*rb.rows, uint(0))
	if rb.buffer == 0 {
		rb.failure = "Metal could not allocate the readback buffer"
		return
	}
	f.blitter().Send(sel("copyFromTexture:sourceSlice:sourceLevel:sourceOrigin:sourceSize:toBuffer:destinationOffset:destinationBytesPerRow:destinationBytesPerImage:"),
		t.object, uint(reg.ArrayLayer), uint(reg.MipLevel), gpuOrigin{X: uint(reg.X), Y: uint(reg.Y)},
		gpuOrigin{X: uint(reg.Width), Y: uint(reg.Height), Z: 1}, rb.buffer, uint(0), rb.stride, rb.stride*rb.rows)
}

func (r *gpuRender) copyOp(f *frameRun, cp *driver.GPUCopyOp) {
	switch cp.Kind {
	case driver.GPUCopyBufferToBuffer:
		src, dst := r.buffers[cp.SrcID], r.buffers[cp.DstID]
		if src == 0 || dst == 0 {
			r.fail(driver.GPUErrorInvalidDescriptor, "a buffer copy names a buffer that does not exist")
			return
		}
		f.blitter().Send(sel("copyFromBuffer:sourceOffset:toBuffer:destinationOffset:size:"), src, uint(cp.SrcOffset), dst, uint(cp.DstOffset), uint(cp.Size))
	case driver.GPUCopyBufferToTexture:
		src, dst := r.buffers[cp.SrcID], r.textures[cp.DstTex.Texture]
		if src == 0 || dst == nil {
			r.fail(driver.GPUErrorInvalidDescriptor, "a buffer-to-texture copy names a resource that does not exist")
			return
		}
		stride := uint(cp.SrcLayout.BytesPerRow)
		if stride == 0 {
			stride = uint(cp.Width * dst.desc.Format.BytesPerPixel())
		}
		image := stride * uint(max(cp.SrcLayout.RowsPerImage, cp.Height))
		for l := range uint(max(cp.Layers, 1)) {
			f.blitter().Send(sel("copyFromBuffer:sourceOffset:sourceBytesPerRow:sourceBytesPerImage:sourceSize:toTexture:destinationSlice:destinationLevel:destinationOrigin:"),
				src, uint(cp.SrcLayout.Offset)+l*image, stride, image, gpuOrigin{X: uint(cp.Width), Y: uint(cp.Height), Z: 1},
				dst.object, uint(cp.DstTex.ArrayLayer)+l, uint(cp.DstTex.MipLevel), gpuOrigin{X: uint(cp.DstTex.OriginX), Y: uint(cp.DstTex.OriginY)})
		}
	case driver.GPUCopyTextureToTexture:
		src, dst := r.textures[cp.SrcTex.Texture], r.textures[cp.DstTex.Texture]
		if src == nil || dst == nil {
			r.fail(driver.GPUErrorInvalidDescriptor, "a texture copy names a texture that does not exist")
			return
		}
		for l := range uint(max(cp.Layers, 1)) {
			f.blitter().Send(sel("copyFromTexture:sourceSlice:sourceLevel:sourceOrigin:sourceSize:toTexture:destinationSlice:destinationLevel:destinationOrigin:"),
				src.object, uint(cp.SrcTex.ArrayLayer)+l, uint(cp.SrcTex.MipLevel), gpuOrigin{X: uint(cp.SrcTex.OriginX), Y: uint(cp.SrcTex.OriginY)},
				gpuOrigin{X: uint(cp.Width), Y: uint(cp.Height), Z: 1},
				dst.object, uint(cp.DstTex.ArrayLayer)+l, uint(cp.DstTex.MipLevel), gpuOrigin{X: uint(cp.DstTex.OriginX), Y: uint(cp.DstTex.OriginY)})
		}
	}
}

func gpuLoad(op driver.GPULoadOp, fresh bool) uint {
	if op == driver.GPULoadClear || fresh {
		return mtlLoadActionClear
	}
	return mtlLoadActionLoad
}

func gpuStore(op driver.GPUStoreOp, resolve bool) uint {
	switch {
	case resolve && op == driver.GPUStoreDiscard:
		return mtlStoreActionMultisampleResolve
	case resolve:
		return mtlStoreActionStoreAndResolve
	case op == driver.GPUStoreDiscard:
		return mtlStoreActionDontCare
	default:
		return mtlStoreActionStore
	}
}

// passState is what a render pass has bound so far.
type passState struct {
	enc           objc.ID
	width, height uint32
	depthFormat   uint
	pipeline      *gpuPipeline
	groups        map[uint32]driver.GPUCommand
	dirty         bool
	index         objc.ID
	indexOffset   uint64
	indexFormat   driver.GPUIndexFormat
	noScissor     bool // the scissor rectangle is empty, so draws draw nothing
}

func (r *gpuRender) pass(f *frameRun, p *driver.GPUPass) {
	desc := objc.ID(Class("MTLRenderPassDescriptor")).Send(sel("renderPassDescriptor"))
	ps := passState{groups: map[uint32]driver.GPUCommand{}}
	colors := p.ColorAttachments
	if len(colors) == 0 {
		colors = []driver.GPUColorAttachment{{LoadOp: driver.GPULoadLoad}}
	}
	surface := false
	for i, ca := range colors {
		a := desc.Send(selColorAttachments).Send(selObjectAtIndexedSubscript, uint(i))
		var target, resolve *gpuTexture
		fresh := false
		if ca.Texture == 0 {
			surface, fresh = true, r.freshColor
			target = r.surface
			if r.msaa != nil {
				target, resolve = r.msaa, r.surface
			}
		} else {
			target, resolve = r.textures[ca.Texture], r.textures[ca.ResolveTarget]
		}
		if target == nil {
			if ca.Texture != 0 {
				r.fail(driver.GPUErrorInvalidDescriptor, "a render pass targets texture %d, which does not exist", ca.Texture)
			}
			return
		}
		a.Send(sel("setTexture:"), target.object)
		a.Send(sel("setLevel:"), uint(ca.MipLevel))
		a.Send(sel("setSlice:"), uint(ca.ArrayLayer))
		a.Send(sel("setLoadAction:"), gpuLoad(ca.LoadOp, fresh))
		a.Send(sel("setStoreAction:"), gpuStore(ca.StoreOp, resolve != nil))
		if resolve != nil {
			a.Send(sel("setResolveTexture:"), resolve.object)
		}
		red, green, blue, alpha := ca.ClearColor.Components()
		a.Send(sel("setClearColor:"), mtlClearColor{R: red, G: green, B: blue, A: alpha})
		if i == 0 {
			ps.width, ps.height = mipSize(target.desc, ca.MipLevel)
		}
	}
	ds := p.DepthStencil
	if ds == nil && surface && r.depth != nil {
		ds = &driver.GPUDepthStencilAttachment{DepthLoadOp: driver.GPULoadLoad, StencilLoadOp: driver.GPULoadLoad}
	}
	depthFresh := false
	if ds != nil {
		depth := r.textures[ds.Texture]
		if ds.Texture == 0 {
			depth, depthFresh = r.depth, r.freshDepth
		}
		if depth != nil {
			ps.depthFormat = gpuPixelFormat(depth.desc.Format)
			a := desc.Send(sel("depthAttachment"))
			a.Send(sel("setTexture:"), depth.object)
			a.Send(sel("setLoadAction:"), gpuLoad(ds.DepthLoadOp, depthFresh))
			a.Send(sel("setStoreAction:"), gpuStore(ds.DepthStoreOp, false))
			clearDepth := float64(ds.ClearDepth)
			if ds.DepthLoadOp != driver.GPULoadClear {
				clearDepth = 1
			}
			a.Send(sel("setClearDepth:"), clearDepth)
			if ps.depthFormat == mtlPixelFormatDepth32FloatStencil {
				s := desc.Send(sel("stencilAttachment"))
				s.Send(sel("setTexture:"), depth.object)
				s.Send(sel("setLoadAction:"), gpuLoad(ds.StencilLoadOp, depthFresh))
				s.Send(sel("setStoreAction:"), gpuStore(ds.StencilStoreOp, false))
				s.Send(sel("setClearStencil:"), uint32(ds.ClearStencil))
			}
		}
	}
	ps.enc = f.commandBuffer().Send(sel("renderCommandEncoderWithDescriptor:"), desc)
	if ps.enc == 0 {
		r.fail(driver.GPUErrorFailed, "Metal could not begin a render pass")
		return
	}
	if surface {
		r.freshColor = false
	}
	if depthFresh {
		r.freshDepth = false
	}
	for _, c := range p.Commands {
		if err := r.command(&ps, c); err != nil {
			r.fail(driver.GPUErrorInvalidDescriptor, "%v", err)
			break
		}
	}
	ps.enc.Send(sel("endEncoding"))
}

func (r *gpuRender) command(ps *passState, c driver.GPUCommand) error {
	e := ps.enc
	switch c.Kind {
	case driver.GPUCmdSetViewport:
		e.Send(sel("setViewport:"), gpuViewport{X: float64(c.F0), Y: float64(c.F1), Width: float64(c.F2), Height: float64(c.F3), MinDepth: float64(c.F4), MaxDepth: float64(c.F5)})
	case driver.GPUCmdSetScissor:
		// Metal wants the rectangle within the attachment.
		x0, y0 := min(max(c.I0, 0), int32(ps.width)), min(max(c.I1, 0), int32(ps.height))
		x1, y1 := min(max(c.I0+c.I2, 0), int32(ps.width)), min(max(c.I1+c.I3, 0), int32(ps.height))
		ps.noScissor = x1 <= x0 || y1 <= y0
		if !ps.noScissor {
			e.Send(sel("setScissorRect:"), gpuScissor{X: uint(x0), Y: uint(y0), Width: uint(x1 - x0), Height: uint(y1 - y0)})
		}
	case driver.GPUCmdSetPipeline:
		p := r.pipelines[c.ID]
		if p == nil {
			return fmt.Errorf("a render pass sets pipeline %d, which does not exist", c.ID)
		}
		state, err := p.state(ps.depthFormat)
		if err != nil {
			return fmt.Errorf("Metal could not make pipeline %q's state: %v", p.desc.Label, err)
		}
		ps.pipeline, ps.dirty = p, true
		e.Send(sel("setRenderPipelineState:"), state)
		e.Send(sel("setDepthStencilState:"), p.depthState)
		// MTLWinding has clockwise first.
		e.Send(sel("setFrontFacingWinding:"), uint(1-p.desc.Primitive.FrontFace))
		e.Send(sel("setCullMode:"), uint(p.desc.Primitive.CullMode))
		if ds := p.desc.DepthStencil; ds != nil && (ds.DepthBias != 0 || ds.DepthBiasSlopeScale != 0) {
			e.Send(sel("setDepthBias:slopeScale:clamp:"), float32(ds.DepthBias), ds.DepthBiasSlopeScale, float32(0))
		}
	case driver.GPUCmdSetBindGroup:
		if r.groups[c.ID] == nil {
			return fmt.Errorf("a render pass sets bind group %d, which does not exist", c.ID)
		}
		ps.groups[c.Slot], ps.dirty = c, true
	case driver.GPUCmdSetVertexBuffer:
		b := r.buffers[c.ID]
		if b == 0 {
			return fmt.Errorf("a render pass sets vertex buffer %d, which does not exist", c.ID)
		}
		e.Send(sel("setVertexBuffer:offset:atIndex:"), b, uint(c.Offset), uint(c.Slot))
	case driver.GPUCmdSetIndexBuffer:
		b := r.buffers[c.ID]
		if b == 0 {
			return fmt.Errorf("a render pass sets index buffer %d, which does not exist", c.ID)
		}
		ps.index, ps.indexOffset, ps.indexFormat = b, c.Offset, c.IndexFormat
	case driver.GPUCmdSetStencilReference:
		e.Send(sel("setStencilReferenceValue:"), c.U0)
	case driver.GPUCmdSetBlendConstant:
		e.Send(sel("setBlendColorRed:green:blue:alpha:"), c.F0, c.F1, c.F2, c.F3)
	case driver.GPUCmdDraw, driver.GPUCmdDrawInstanced, driver.GPUCmdDrawIndexed, driver.GPUCmdDrawIndexedInstanced:
		return r.draw(ps, c)
	}
	return nil
}

func (r *gpuRender) draw(ps *passState, c driver.GPUCommand) error {
	if ps.pipeline == nil {
		return fmt.Errorf("a render pass draws without a pipeline")
	}
	if ps.dirty {
		r.bindGroups(ps)
		ps.dirty = false
	}
	if ps.noScissor {
		return nil
	}
	e := ps.enc
	topology := [...]uint{3, 4, 1, 2, 0} // TriangleList, TriangleStrip, LineList, LineStrip, PointList as MTLPrimitiveType
	if int(ps.pipeline.desc.Primitive.Topology) >= len(topology) {
		return fmt.Errorf("pipeline %q has an unknown topology", ps.pipeline.desc.Label)
	}
	primitive := topology[ps.pipeline.desc.Primitive.Topology]
	switch c.Kind {
	case driver.GPUCmdDraw:
		e.Send(sel("drawPrimitives:vertexStart:vertexCount:"), primitive, uint(c.U1), uint(c.U0))
	case driver.GPUCmdDrawInstanced:
		if c.U3 == 0 {
			e.Send(sel("drawPrimitives:vertexStart:vertexCount:instanceCount:"), primitive, uint(c.U2), uint(c.U0), uint(c.U1))
		} else {
			e.Send(sel("drawPrimitives:vertexStart:vertexCount:instanceCount:baseInstance:"), primitive, uint(c.U2), uint(c.U0), uint(c.U1), uint(c.U3))
		}
	default:
		if ps.index == 0 {
			return fmt.Errorf("a render pass draws indexed without an index buffer")
		}
		count, instances, first, firstInstance := c.U0, uint32(1), c.U1, uint32(0)
		if c.Kind == driver.GPUCmdDrawIndexedInstanced {
			instances, first, firstInstance = c.U1, c.U2, c.U3
		}
		size := uint64(2)
		if ps.indexFormat == driver.GPUIndexUint32 {
			size = 4
		}
		offset := uint(ps.indexOffset + uint64(first)*size)
		if c.I0 == 0 && firstInstance == 0 {
			e.Send(sel("drawIndexedPrimitives:indexCount:indexType:indexBuffer:indexBufferOffset:instanceCount:"),
				primitive, uint(count), uint(ps.indexFormat), ps.index, offset, uint(instances))
		} else {
			e.Send(sel("drawIndexedPrimitives:indexCount:indexType:indexBuffer:indexBufferOffset:instanceCount:baseVertex:baseInstance:"),
				primitive, uint(count), uint(ps.indexFormat), ps.index, offset, uint(instances), int(c.I0), uint(firstInstance))
		}
	}
	return nil
}

// bindGroups binds the pass's groups' resources to the Metal slots the bound pipeline's shader gave their @group and @binding, in both stages.
func (r *gpuRender) bindGroups(ps *passState) {
	e := ps.enc
	for group, set := range ps.groups {
		g := r.groups[set.ID]
		if g == nil {
			continue
		}
		dynamic := r.dynamicOffsets(g, set.DynamicOffsets)
		for _, entry := range g.Entries {
			for _, s := range ps.pipeline.slots {
				if s.Group != group || s.Binding != entry.Binding {
					continue
				}
				switch s.Kind {
				case shaderir.BindingUniformBuffer, shaderir.BindingStorageBufferRead:
					if b := r.buffers[entry.Buffer]; b != 0 {
						offset := uint(entry.Offset + uint64(dynamic[entry.Binding]))
						e.Send(sel("setVertexBuffer:offset:atIndex:"), b, offset, uint(s.Slot))
						e.Send(sel("setFragmentBuffer:offset:atIndex:"), b, offset, uint(s.Slot))
					}
				case shaderir.BindingTexture:
					if t := r.textures[entry.Texture]; t != nil {
						e.Send(sel("setVertexTexture:atIndex:"), t.object, uint(s.Slot))
						e.Send(sel("setFragmentTexture:atIndex:"), t.object, uint(s.Slot))
					}
				case shaderir.BindingSampler:
					if sm := r.samplers[entry.Sampler]; sm != 0 {
						e.Send(sel("setVertexSamplerState:atIndex:"), sm, uint(s.Slot))
						e.Send(sel("setFragmentSamplerState:atIndex:"), sm, uint(s.Slot))
					}
				}
			}
		}
	}
}

// dynamicOffsets pairs offsets with g's layout entries that take them, in binding order, as WebGPU does.
func (r *gpuRender) dynamicOffsets(g *driver.GPUBindGroupDesc, offsets []uint32) map[uint32]uint32 {
	l := r.layouts[g.Layout]
	if l == nil || len(offsets) == 0 {
		return nil
	}
	var bindings []uint32
	for _, e := range l.Entries {
		if e.HasDynamicOffset {
			bindings = append(bindings, e.Binding)
		}
	}
	slices.Sort(bindings)
	out := make(map[uint32]uint32, len(bindings))
	for i, b := range bindings {
		if i < len(offsets) {
			out[b] = offsets[i]
		}
	}
	return out
}

// present copies the surface into the layer's next drawable and presents it.
func (r *gpuRender) present(f *frameRun) {
	drawable := r.layer.Send(selNextDrawable)
	if drawable == 0 {
		return
	}
	target := drawable.Send(sel("texture"))
	w := min(uint(target.Send(sel("width"))), uint(r.surface.desc.Width))
	h := min(uint(target.Send(sel("height"))), uint(r.surface.desc.Height))
	if w > 0 && h > 0 {
		blit := f.commandBuffer().Send(sel("blitCommandEncoder"))
		blit.Send(sel("copyFromTexture:sourceSlice:sourceLevel:sourceOrigin:sourceSize:toTexture:destinationSlice:destinationLevel:destinationOrigin:"),
			r.surface.object, uint(0), uint(0), gpuOrigin{}, gpuOrigin{X: w, Y: h, Z: 1}, target, uint(0), uint(0), gpuOrigin{})
		blit.Send(sel("endEncoding"))
	}
	f.cb.Send(sel("presentDrawable:"), drawable)
}

// commit commits cb, reporting a failure the GPU meets running it.
func (r *gpuRender) commit(cb objc.ID) {
	cb.Send(sel("retain"))
	gen, emit := r.generation, r.emit
	completed := objcblock.New(func(_ objc.Block, buffer objc.ID) {
		if buffer.Send(sel("status")) != mtlCommandBufferStatusError {
			return
		}
		err := buffer.Send(sel("error"))
		code := driver.GPUErrorFailed
		if err.Send(sel("code")) == mtlCommandBufferErrorOutOfMemory {
			code = driver.GPUErrorOutOfMemory
		}
		emit(&driver.GPUEvent{Kind: driver.GPUEventError, Generation: gen, ErrorCode: code, Message: errorText(err)})
	})
	cb.Send(sel("addCompletedHandler:"), completed)
	completed.Release()
	cb.Send(sel("commit"))
	if r.last != 0 {
		r.last.Send(selRelease)
	}
	r.last = cb
}
