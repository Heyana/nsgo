package driver

import (
	"encoding/binary"
	"fmt"
	"go/constant"
	"image"
	"math"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

// GPUDebugEvent records a debug label, group, or marker observed by GPUSoftwareDevice during resource creation or command execution.
type GPUDebugEvent struct {
	Kind  string
	Label string
	Depth int
}

// GPUSoftReadback holds the completed CPU pixels of a GPUOpReadTexture op.
type GPUSoftReadback struct {
	ID     uint64
	Pixels []byte
	Err    string
}

type softBuffer struct {
	desc   GPUBufferDesc
	data   []byte
	shared bool
}

type softTexture struct {
	desc    GPUTextureDesc
	samples int
	color   []float32 // w * h * samples * 4 linear RGBA float32
	depth   []float32 // w * h * samples when Format.IsDepth()
	stencil []uint8   // w * h * samples when Format.HasStencil()
	// sub holds one view per (array layer, mip level), indexed layer*mips + mip. Each view is a single-layer, single-mip softTexture whose desc carries the mip size; sub[0] shares its buffers with the fields above, so code that only knows level 0 keeps working.
	layers, mips uint32
	sub          []*softTexture
}

// level returns the view of array layer (or cube face) layer at mip level mip, or nil when out of range.
func (t *softTexture) level(layer, mip uint32) *softTexture {
	if t == nil {
		return nil
	}
	if len(t.sub) == 0 {
		if layer == 0 && mip == 0 {
			return t
		}
		return nil
	}
	if layer >= t.layers || mip >= t.mips {
		return nil
	}
	return t.sub[layer*t.mips+mip]
}

func newSoftTextureLevel(desc GPUTextureDesc, w, h, samples int) *softTexture {
	desc.Width, desc.Height, desc.Layers, desc.MipLevels = uint32(w), uint32(h), 1, 1
	st := &softTexture{desc: desc, samples: samples}
	nSamp := w * h * samples
	if desc.Format.IsDepth() {
		st.depth = make([]float32, nSamp)
		for i := range st.depth {
			st.depth[i] = 1.0
		}
	}
	if desc.Format.HasStencil() {
		st.stencil = make([]uint8, nSamp)
	}
	if !desc.Format.IsDepthStencil() {
		st.color = make([]float32, nSamp*4)
	}
	return st
}

type softSampler struct {
	desc GPUSamplerDesc
}

type softBindGroupLayout struct {
	desc           GPUBindGroupLayoutDesc
	dynamicEntries []GPUBindGroupLayoutEntry // sorted ascending by Binding
}

type softBindGroup struct {
	desc    GPUBindGroupDesc
	entries map[uint32]GPUBindGroupEntry
}

type softAttrBinding struct {
	slotIdx  uint32
	stride   uint64
	stepMode GPUVertexStepMode
	stepRate uint32
	offset   uint64
	format   GPUVertexFormat
	irIdx    int
}

type softPipeline struct {
	desc     GPUPipelineDesc
	combined *shaderir.Program
	attrs    []softAttrBinding
}

type softWorkItem struct {
	frame GPUFrame
	stop  bool
}

type softComputePipeline struct {
	desc GPUComputePipelineDesc
}

// GPUSoftwareDevice is a deterministic pure-Go software 3D GPU rasterizer
// and shaderir interpreter used by headless nsgotest and unit tests.
type GPUSoftwareDevice struct {
	mu            sync.Mutex
	width, height int
	cfg           GPUViewConfig
	swapSamples   int
	colorBuf      []byte
	swapColor     []float32
	depthBuf      []float32
	stencilBuf    []uint8

	buffers          map[GPUID]*softBuffer
	textures         map[GPUID]*softTexture
	samplers         map[GPUID]*softSampler
	layouts          map[GPUID]*softBindGroupLayout
	bindGroups       map[GPUID]*softBindGroup
	pipelines        map[GPUID]*softPipeline
	computePipelines map[GPUID]*softComputePipeline

	frames      []GPUFrame
	lastTimings []GPUPassTiming
	debugEvents []GPUDebugEvent
	debugDepth  int
	labels      map[GPUID]string
	lastErr     error

	// Optional pipelined worker goroutine and frame lifecycle hooks.
	pipelined   bool
	queue       chan softWorkItem
	workerDone  chan struct{}
	inFlightWG  sync.WaitGroup
	beforePass  func(GPUFrame)
	afterFrame  func(GPUFrame)
	onReadbacks func([]GPUSoftReadback, error)
}

// NewGPUSoftwareDevice creates a software GPU device for a surface of width x height pixels.
func NewGPUSoftwareDevice(width, height int, cfg GPUViewConfig) *GPUSoftwareDevice {
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}
	cfg.FramesInFlight = NormalizeFramesInFlight(cfg.FramesInFlight)
	samples := 1
	if cfg.SampleCount == 4 {
		samples = 4
	}
	if cfg.ColorFormat == GPUFormatUndefined {
		cfg.ColorFormat = GPUFormatRGBA8Unorm
	}
	d := &GPUSoftwareDevice{
		cfg:              cfg,
		swapSamples:      samples,
		buffers:          map[GPUID]*softBuffer{},
		textures:         map[GPUID]*softTexture{},
		samplers:         map[GPUID]*softSampler{},
		layouts:          map[GPUID]*softBindGroupLayout{},
		bindGroups:       map[GPUID]*softBindGroup{},
		pipelines:        map[GPUID]*softPipeline{},
		computePipelines: map[GPUID]*softComputePipeline{},
		labels:           map[GPUID]string{},
	}
	d.Resize(width, height)
	return d
}

// FramesInFlight returns the normalized frame-in-flight count configured on d.
func (d *GPUSoftwareDevice) FramesInFlight() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return NormalizeFramesInFlight(d.cfg.FramesInFlight)
}

// SetPipelined toggles background GPU-worker execution. When enabled, Submit
// enqueues frames onto a dedicated GPU worker goroutine and returns immediately,
// allowing the caller to overlap CPU frame N+1 ring-buffer writes with GPU frame N
// rasterization while frame-slot fences guard slot reuse.
func (d *GPUSoftwareDevice) SetPipelined(enabled bool) {
	d.mu.Lock()
	if d.pipelined == enabled {
		d.mu.Unlock()
		return
	}
	if enabled {
		q := make(chan softWorkItem, 32)
		done := make(chan struct{}, 1)
		d.queue = q
		d.workerDone = done
		d.pipelined = true
		d.mu.Unlock()
		go d.workerLoop(q, done)
		return
	}
	q := d.queue
	done := d.workerDone
	d.queue = nil
	d.workerDone = nil
	d.pipelined = false
	d.mu.Unlock()
	if q != nil {
		q <- softWorkItem{stop: true}
		<-done
	}
}

// SetFrameHooks installs optional callbacks invoked on the GPU execution thread
// immediately before a frame's render passes run (after resource ops/copies)
// and immediately after the frame completes (before signaling its completion fence).
func (d *GPUSoftwareDevice) SetFrameHooks(beforePass, afterFrame func(GPUFrame)) {
	d.mu.Lock()
	d.beforePass = beforePass
	d.afterFrame = afterFrame
	d.mu.Unlock()
}

// SetReadbackHandler installs an optional callback invoked with completed
// readbacks or execution errors when running in pipelined mode.
func (d *GPUSoftwareDevice) SetReadbackHandler(fn func([]GPUSoftReadback, error)) {
	d.mu.Lock()
	d.onReadbacks = fn
	d.mu.Unlock()
}

// WaitIdle waits until all in-flight frames submitted in pipelined mode complete.
func (d *GPUSoftwareDevice) WaitIdle() {
	d.inFlightWG.Wait()
}

// Close flushes pending work and stops any pipelined worker goroutine.
func (d *GPUSoftwareDevice) Close() {
	d.WaitIdle()
	d.SetPipelined(false)
}

// LastError returns the most recent execution error recorded by d.
func (d *GPUSoftwareDevice) LastError() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lastErr
}

func (d *GPUSoftwareDevice) workerLoop(q chan softWorkItem, done chan struct{}) {
	defer func() { done <- struct{}{} }()
	for item := range q {
		if item.stop {
			return
		}
		frame := item.frame
		rbs, err := d.execFrame(frame)
		d.mu.Lock()
		onRB := d.onReadbacks
		d.mu.Unlock()
		if onRB != nil && (len(rbs) > 0 || err != nil) {
			onRB(rbs, err)
		}
		if frame.OnComplete != nil {
			frame.OnComplete()
		}
		d.inFlightWG.Done()
	}
}

// Resize reallocates the swapchain color, depth, and stencil buffers for a new pixel size.
func (d *GPUSoftwareDevice) Resize(width, height int) {
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.width == width && d.height == height && len(d.colorBuf) == width*height*4 {
		return
	}
	d.width, d.height = width, height
	d.colorBuf = make([]byte, width*height*4)
	nSamp := width * height * max(d.swapSamples, 1)
	d.swapColor = make([]float32, nSamp*4)
	d.depthBuf = make([]float32, nSamp)
	for i := range d.depthBuf {
		d.depthBuf[i] = 1.0
	}
	d.stencilBuf = make([]uint8, nSamp)
}

// Size returns the current swapchain dimensions in pixels.
func (d *GPUSoftwareDevice) Size() (int, int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.width, d.height
}

// Frames returns all GPUFrames submitted to this device, oldest first.
func (d *GPUSoftwareDevice) Frames() []GPUFrame {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.frames)
}

// LastPassTimings returns a copy of the pass timings measured during the most recent Submit call.
func (d *GPUSoftwareDevice) LastPassTimings() []GPUPassTiming {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.lastTimings)
}

// DebugEvents returns a copy of all debug labels, groups, and markers observed by this software device.
func (d *GPUSoftwareDevice) DebugEvents() []GPUDebugEvent {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.debugEvents)
}

// ResourceLabel returns the debug label recorded for resource id, if any.
func (d *GPUSoftwareDevice) ResourceLabel(id GPUID) string {
	return d.labels[id]
}

func (d *GPUSoftwareDevice) recordResourceLabel(kind string, id GPUID, label string) {
	if label == "" {
		return
	}
	d.labels[id] = label
	d.debugEvents = append(d.debugEvents, GPUDebugEvent{Kind: kind, Label: label, Depth: d.debugDepth})
}

func (d *GPUSoftwareDevice) applyEncoderDebugOps(ops []GPUDebugOp, cursor *int, passIndex int) {
	for *cursor < len(ops) && ops[*cursor].PassIndex <= passIndex {
		op := ops[*cursor]
		*cursor++
		switch op.Kind {
		case GPUDebugPushGroup:
			d.debugEvents = append(d.debugEvents, GPUDebugEvent{Kind: "push_group", Label: op.Label, Depth: d.debugDepth})
			d.debugDepth++
		case GPUDebugPopGroup:
			if d.debugDepth > 0 {
				d.debugDepth--
			}
			d.debugEvents = append(d.debugEvents, GPUDebugEvent{Kind: "pop_group", Depth: d.debugDepth})
		case GPUDebugInsertMarker:
			d.debugEvents = append(d.debugEvents, GPUDebugEvent{Kind: "marker", Label: op.Label, Depth: d.debugDepth})
		}
	}
}

// ReadBuffer returns a copy of buffer id's contents, if present.
func (d *GPUSoftwareDevice) ReadBuffer(id GPUID) ([]byte, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	b, ok := d.buffers[id]
	if !ok {
		return nil, false
	}
	return slices.Clone(b.data), true
}

// SurfaceImage returns an independent RGBA snapshot of the swapchain color buffer.
func (d *GPUSoftwareDevice) SurfaceImage() *image.RGBA {
	d.mu.Lock()
	defer d.mu.Unlock()
	img := image.NewRGBA(image.Rect(0, 0, d.width, d.height))
	copy(img.Pix, d.colorBuf)
	return img
}

// Submit executes all resource ops, copies, and render passes in frame.
// When SetPipelined(true) is active, Submit enqueues frame onto the GPU worker
// goroutine and returns immediately without blocking the caller.
func (d *GPUSoftwareDevice) Submit(frame GPUFrame) ([]GPUSoftReadback, error) {
	d.mu.Lock()
	if d.pipelined && d.queue != nil {
		q := d.queue
		d.inFlightWG.Add(1)
		d.mu.Unlock()
		q <- softWorkItem{frame: frame}
		return nil, nil
	}
	d.mu.Unlock()

	rbs, err := d.execFrame(frame)
	if frame.OnComplete != nil {
		frame.OnComplete()
	}
	return rbs, err
}

func (d *GPUSoftwareDevice) execFrame(frame GPUFrame) ([]GPUSoftReadback, error) {
	d.mu.Lock()
	d.frames = append(d.frames, frame)
	d.lastTimings = nil
	var readbacks []GPUSoftReadback
	var err error
	for _, op := range frame.Ops {
		var rb *GPUSoftReadback
		rb, err = d.execOp(op)
		if err != nil {
			d.lastErr = err
			break
		}
		if rb != nil {
			readbacks = append(readbacks, *rb)
		}
	}
	if err == nil {
		for _, cp := range frame.Copies {
			if err = d.execCopy(cp); err != nil {
				d.lastErr = err
				break
			}
		}
	}
	beforeHook := d.beforePass
	afterHook := d.afterFrame
	d.mu.Unlock()

	if err == nil && len(frame.Passes) > 0 && beforeHook != nil {
		beforeHook(frame)
	}
	d.mu.Lock()
	debugCursor := 0
	if err == nil {
		for passIdx, pass := range frame.Passes {
			d.applyEncoderDebugOps(frame.DebugOps, &debugCursor, passIdx)
			if pass.Label != "" {
				d.debugEvents = append(d.debugEvents, GPUDebugEvent{Kind: "pass", Label: pass.Label, Depth: d.debugDepth})
			}
			measure := pass.TimingLabel != "" || pass.TimestampQueryID != 0
			var start time.Time
			if measure {
				start = time.Now()
			}
			if err = d.execPass(pass); err != nil {
				d.lastErr = err
				break
			}
			if measure {
				d.lastTimings = append(d.lastTimings, GPUPassTiming{
					Label:     pass.TimingLabel,
					PassIndex: passIdx,
					QueryID:   pass.TimestampQueryID,
					Duration:  max(time.Since(start), 0),
				})
			}
		}
		if err == nil {
			d.applyEncoderDebugOps(frame.DebugOps, &debugCursor, len(frame.Passes)+1)
		}
	}
	d.mu.Unlock()
	if afterHook != nil && len(frame.Passes) > 0 {
		afterHook(frame)
	}
	return readbacks, err
}

func (d *GPUSoftwareDevice) execOp(op GPUResourceOp) (*GPUSoftReadback, error) {
	switch op.Kind {
	case GPUOpCreateBuffer:
		if op.Buffer == nil {
			return nil, fmt.Errorf("gpu: missing buffer descriptor")
		}
		if op.Buffer.Shared != nil {
			d.buffers[op.ID] = &softBuffer{desc: *op.Buffer, data: op.Buffer.Shared, shared: true}
			return nil, nil
		}
		size := op.Buffer.Size
		if size == 0 {
			size = uint64(len(op.Buffer.Data))
		}
		buf := make([]byte, size)
		copy(buf, op.Buffer.Data)
		d.buffers[op.ID] = &softBuffer{desc: *op.Buffer, data: buf}
		d.recordResourceLabel("buffer", op.ID, op.Buffer.Label)
	case GPUOpWriteBuffer:
		b, ok := d.buffers[op.ID]
		if !ok {
			return nil, fmt.Errorf("gpu: write to unknown buffer %d", op.ID)
		}
		if b.shared && len(op.Data) == 0 {
			return nil, nil
		}
		if op.Offset+uint64(len(op.Data)) > uint64(len(b.data)) {
			return nil, fmt.Errorf("gpu: buffer %d write out of bounds (%d+%d > %d)", op.ID, op.Offset, len(op.Data), len(b.data))
		}
		copy(b.data[op.Offset:], op.Data)
	case GPUOpDestroyBuffer:
		delete(d.buffers, op.ID)
	case GPUOpCreateTexture:
		if op.Texture == nil {
			return nil, fmt.Errorf("gpu: missing texture descriptor")
		}
		w, h := int(op.Texture.Width), int(op.Texture.Height)
		if w <= 0 || h <= 0 {
			return nil, fmt.Errorf("gpu: invalid texture size %dx%d", w, h)
		}
		if op.Texture.Format.IsCompressed() {
			bw, bh := op.Texture.Format.BlockSize()
			if bw == 0 || bh == 0 || op.Texture.Width%bw != 0 || op.Texture.Height%bh != 0 {
				return nil, fmt.Errorf("gpu: compressed texture %dx%d must be a multiple of block size %dx%d", op.Texture.Width, op.Texture.Height, bw, bh)
			}
		}
		samples := max(op.Texture.SampleCount, 1)
		if samples != 1 && samples != 4 {
			return nil, fmt.Errorf("gpu: unsupported sample count %d", samples)
		}
		desc := *op.Texture
		if desc.Layers == 0 {
			desc.Layers = 1
			if desc.Dimension == GPUDimCube {
				desc.Layers = 6
			}
		}
		desc.MipLevels = max(desc.MipLevels, 1)
		if desc.Dimension == GPUDimCube && (w != h || desc.Layers != 6) {
			return nil, fmt.Errorf("gpu: cubemap texture must be square with 6 layers, got %dx%d with %d layers", w, h, desc.Layers)
		}
		if maxMips := GPUMipLevels(uint32(w), uint32(h)); desc.MipLevels > maxMips {
			return nil, fmt.Errorf("gpu: texture mip levels %d exceed maximum %d for %dx%d", desc.MipLevels, maxMips, w, h)
		}
		st := &softTexture{desc: desc, samples: samples, layers: desc.Layers, mips: desc.MipLevels}
		st.sub = make([]*softTexture, desc.Layers*desc.MipLevels)
		for layer := uint32(0); layer < desc.Layers; layer++ {
			for mip := uint32(0); mip < desc.MipLevels; mip++ {
				mw, mh := GPUMipSize(uint32(w), uint32(h), mip)
				st.sub[layer*desc.MipLevels+mip] = newSoftTextureLevel(desc, int(mw), int(mh), samples)
			}
		}
		st.color, st.depth, st.stencil = st.sub[0].color, st.sub[0].depth, st.sub[0].stencil
		d.textures[op.ID] = st
		d.recordResourceLabel("texture", op.ID, op.Texture.Label)
	case GPUOpWriteTexture:
		base, ok := d.textures[op.TextureDst.Texture]
		if !ok {
			return nil, fmt.Errorf("gpu: write to unknown texture %d", op.TextureDst.Texture)
		}
		tex := base.level(op.TextureDst.ArrayLayer, op.TextureDst.MipLevel)
		if tex == nil {
			return nil, fmt.Errorf("gpu: texture %d write subresource out of bounds (layer %d, mip %d)", op.TextureDst.Texture, op.TextureDst.ArrayLayer, op.TextureDst.MipLevel)
		}
		if tex.color == nil {
			return nil, nil
		}
		if tex.desc.Format.IsCompressed() {
			regionW := tex.desc.Width - op.TextureDst.OriginX
			regionH := tex.desc.Height - op.TextureDst.OriginY
			if op.DataLayout.RowsPerImage > 0 {
				_, bh := tex.desc.Format.BlockSize()
				if op.DataLayout.RowsPerImage*bh < regionH {
					regionH = op.DataLayout.RowsPerImage * bh
				}
			}
			if err := decodeCompressedIntoTexture(tex, op.TextureDst.OriginX, op.TextureDst.OriginY, regionW, regionH, op.DataLayout, op.Data); err != nil {
				return nil, err
			}
			return nil, nil
		}
		w, h := int(tex.desc.Width), int(tex.desc.Height)
		src := op.Data
		if op.DataLayout.Offset < uint64(len(src)) {
			src = src[op.DataLayout.Offset:]
		}
		bpp := int(tex.desc.Format.BytesPerPixel())
		if bpp == 0 {
			bpp = 4
		}
		ox, oy := int(op.TextureDst.OriginX), int(op.TextureDst.OriginY)
		srcStride := int(op.DataLayout.BytesPerRow)
		if srcStride == 0 {
			srcStride = (w - ox) * bpp
		}
		// As on the native backends, the written width is BytesPerRow / bytes-per-texel (clamped to the edge) and RowsPerImage bounds the rows.
		copyW := min(srcStride/bpp, w-ox)
		copyH := h - oy
		if op.DataLayout.RowsPerImage > 0 {
			copyH = min(copyH, int(op.DataLayout.RowsPerImage))
		}
		for y := 0; y < copyH; y++ {
			rowStart := y * srcStride
			if rowStart >= len(src) {
				break
			}
			for x := 0; x < copyW; x++ {
				si := rowStart + x*bpp
				if si+bpp > len(src) {
					break
				}
				c := quantizeColorForFormat(tex.desc.Format, decodePixelBytes(src[si:si+bpp], tex.desc.Format))
				pixIdx := (y+oy)*w + (x + ox)
				for s := 0; s < tex.samples; s++ {
					di := (pixIdx*tex.samples + s) * 4
					tex.color[di+0] = c[0]
					tex.color[di+1] = c[1]
					tex.color[di+2] = c[2]
					tex.color[di+3] = c[3]
				}
			}
		}
	case GPUOpDestroyTexture:
		delete(d.textures, op.ID)
	case GPUOpGenerateMipmaps:
		tex, ok := d.textures[op.ID]
		if !ok {
			return nil, fmt.Errorf("gpu: generate mipmaps on unknown texture %d", op.ID)
		}
		if tex.color == nil || max(tex.samples, 1) > 1 {
			return nil, nil
		}
		for layer := uint32(0); layer < tex.layers; layer++ {
			for mip := uint32(1); mip < tex.mips; mip++ {
				downsampleSoftLevel(tex.level(layer, mip-1), tex.level(layer, mip))
			}
		}
	case GPUOpCreateSampler:
		if op.Sampler == nil {
			return nil, fmt.Errorf("gpu: missing sampler descriptor")
		}
		d.samplers[op.ID] = &softSampler{desc: *op.Sampler}
		d.recordResourceLabel("sampler", op.ID, op.Sampler.Label)
	case GPUOpDestroySampler:
		delete(d.samplers, op.ID)
	case GPUOpCreateBindGroupLayout:
		if op.BindGroupLayout == nil {
			return nil, fmt.Errorf("gpu: missing bind group layout descriptor")
		}
		var dynEntries []GPUBindGroupLayoutEntry
		for _, e := range op.BindGroupLayout.Entries {
			if e.HasDynamicOffset {
				dynEntries = append(dynEntries, e)
			}
		}
		sort.Slice(dynEntries, func(i, j int) bool {
			return dynEntries[i].Binding < dynEntries[j].Binding
		})
		d.layouts[op.ID] = &softBindGroupLayout{desc: *op.BindGroupLayout, dynamicEntries: dynEntries}
		d.recordResourceLabel("bind_group_layout", op.ID, op.BindGroupLayout.Label)
	case GPUOpDestroyBindGroupLayout:
		delete(d.layouts, op.ID)
	case GPUOpCreateBindGroup:
		if op.BindGroup == nil {
			return nil, fmt.Errorf("gpu: missing bind group descriptor")
		}
		m := make(map[uint32]GPUBindGroupEntry, len(op.BindGroup.Entries))
		for _, e := range op.BindGroup.Entries {
			m[e.Binding] = e
		}
		d.bindGroups[op.ID] = &softBindGroup{desc: *op.BindGroup, entries: m}
		d.recordResourceLabel("bind_group", op.ID, op.BindGroup.Label)
	case GPUOpDestroyBindGroup:
		delete(d.bindGroups, op.ID)
	case GPUOpCreatePipeline:
		if op.Pipeline == nil {
			return nil, fmt.Errorf("gpu: missing pipeline descriptor")
		}
		for _, ir := range []*shaderir.Program{op.Pipeline.Vertex.IR, op.Pipeline.Fragment.IR} {
			if b := unsupportedSoftBuiltin(ir); b != "" {
				return nil, fmt.Errorf("gpu: the software device does not implement the %q builtin", b)
			}
		}
		comb := mergeShaderPrograms(op.Pipeline.Vertex.IR, op.Pipeline.Fragment.IR)
		var attrMaps []softAttrBinding
		if comb != nil {
			for slotIdx, layout := range op.Pipeline.VertexBuffers {
				sr := layout.StepRate
				if layout.StepMode == GPUStepInstance && sr == 0 {
					sr = 1
				}
				for _, attr := range layout.Attributes {
					irIdx := -1
					for ai := range comb.Attributes {
						loc := uint32(ai)
						if ai < len(comb.AttributeMeta) {
							loc = comb.AttributeMeta[ai].Location
						}
						if loc == attr.Location {
							irIdx = ai
							break
						}
					}
					if irIdx < 0 {
						continue
					}
					attrMaps = append(attrMaps, softAttrBinding{
						slotIdx:  uint32(slotIdx),
						stride:   layout.Stride,
						stepMode: layout.StepMode,
						stepRate: sr,
						offset:   attr.Offset,
						format:   attr.Format,
						irIdx:    irIdx,
					})
				}
			}
		}
		d.pipelines[op.ID] = &softPipeline{desc: *op.Pipeline, combined: comb, attrs: attrMaps}
		d.recordResourceLabel("pipeline", op.ID, op.Pipeline.Label)
		if op.Pipeline.Vertex.Label != "" {
			d.debugEvents = append(d.debugEvents, GPUDebugEvent{Kind: "shader", Label: op.Pipeline.Vertex.Label, Depth: d.debugDepth})
		}
		if op.Pipeline.Fragment.Label != "" && op.Pipeline.Fragment.Label != op.Pipeline.Vertex.Label {
			d.debugEvents = append(d.debugEvents, GPUDebugEvent{Kind: "shader", Label: op.Pipeline.Fragment.Label, Depth: d.debugDepth})
		}
	case GPUOpDestroyPipeline:
		delete(d.pipelines, op.ID)
	case GPUOpCreateComputePipeline:
		if op.ComputePipeline == nil || op.ComputePipeline.Compute.IR == nil || op.ComputePipeline.Compute.IR.ComputeFunc.Block == nil {
			return nil, fmt.Errorf("gpu: missing compute pipeline shader")
		}
		if b := unsupportedSoftBuiltin(op.ComputePipeline.Compute.IR); b != "" {
			return nil, fmt.Errorf("gpu: the software device does not implement the %q builtin", b)
		}
		d.computePipelines[op.ID] = &softComputePipeline{desc: *op.ComputePipeline}
	case GPUOpDestroyComputePipeline:
		delete(d.computePipelines, op.ID)
	case GPUOpReadBuffer:
		b, ok := d.buffers[op.ID]
		if !ok {
			return &GPUSoftReadback{ID: op.ReadbackID, Err: fmt.Sprintf("unknown buffer %d", op.ID)}, nil
		}
		off := op.Offset
		if off > uint64(len(b.data)) {
			return &GPUSoftReadback{ID: op.ReadbackID, Err: "buffer readback offset out of bounds"}, nil
		}
		sz := op.ReadbackSize
		if sz == 0 {
			sz = uint64(len(b.data)) - off
		}
		if off+sz > uint64(len(b.data)) {
			return &GPUSoftReadback{ID: op.ReadbackID, Err: "buffer readback range out of bounds"}, nil
		}
		return &GPUSoftReadback{ID: op.ReadbackID, Pixels: slices.Clone(b.data[off : off+sz])}, nil
	case GPUOpReadTexture:
		if op.ReadbackSurface || op.ID == 0 {
			tw, th := d.width, d.height
			rx, ry := int(op.ReadbackRegion.X), int(op.ReadbackRegion.Y)
			rw, rh := int(op.ReadbackRegion.Width), int(op.ReadbackRegion.Height)
			if rw == 0 {
				rw = tw - rx
			}
			if rh == 0 {
				rh = th - ry
			}
			if rx < 0 || ry < 0 || rw <= 0 || rh <= 0 || rx+rw > tw || ry+rh > th {
				return &GPUSoftReadback{ID: op.ReadbackID, Err: "readback region out of bounds"}, nil
			}
			out := make([]byte, rw*rh*4)
			for y := 0; y < rh; y++ {
				copy(out[y*rw*4:(y+1)*rw*4], d.colorBuf[((ry+y)*tw+rx)*4:((ry+y)*tw+rx+rw)*4])
			}
			return &GPUSoftReadback{ID: op.ReadbackID, Pixels: out}, nil
		}
		base, ok := d.textures[op.ID]
		if !ok {
			return &GPUSoftReadback{ID: op.ReadbackID, Err: fmt.Sprintf("unknown texture %d", op.ID)}, nil
		}
		tex := base.level(op.ReadbackRegion.ArrayLayer, op.ReadbackRegion.MipLevel)
		if tex == nil {
			return &GPUSoftReadback{ID: op.ReadbackID, Err: "readback subresource out of bounds"}, nil
		}
		if tex.color == nil && tex.depth == nil {
			return &GPUSoftReadback{ID: op.ReadbackID, Err: "cannot read back a stencil-only texture"}, nil
		}
		tw, th := int(tex.desc.Width), int(tex.desc.Height)
		rx, ry := int(op.ReadbackRegion.X), int(op.ReadbackRegion.Y)
		rw, rh := int(op.ReadbackRegion.Width), int(op.ReadbackRegion.Height)
		if rw == 0 {
			rw = tw - rx
		}
		if rh == 0 {
			rh = th - ry
		}
		if rx < 0 || ry < 0 || rw <= 0 || rh <= 0 || rx+rw > tw || ry+rh > th {
			return &GPUSoftReadback{ID: op.ReadbackID, Err: "readback region out of bounds"}, nil
		}
		if tex.color == nil {
			// Depth textures read back as one little-endian float32 per texel (sample 0 of multisampled depth).
			out := make([]byte, rw*rh*4)
			for y := 0; y < rh; y++ {
				for x := 0; x < rw; x++ {
					bits := math.Float32bits(tex.depth[((ry+y)*tw+(rx+x))*max(tex.samples, 1)])
					binary.LittleEndian.PutUint32(out[(y*rw+x)*4:], bits)
				}
			}
			return &GPUSoftReadback{ID: op.ReadbackID, Pixels: out}, nil
		}
		bpp := readbackBytesPerPixel(tex.desc.Format)
		out := make([]byte, rw*rh*bpp)
		invSamp := float32(1.0) / float32(max(tex.samples, 1))
		for y := 0; y < rh; y++ {
			for x := 0; x < rw; x++ {
				pixIdx := (ry+y)*tw + (rx + x)
				var avg [4]float32
				for s := 0; s < tex.samples; s++ {
					si := (pixIdx*tex.samples + s) * 4
					avg[0] += tex.color[si+0]
					avg[1] += tex.color[si+1]
					avg[2] += tex.color[si+2]
					avg[3] += tex.color[si+3]
				}
				avg[0] *= invSamp
				avg[1] *= invSamp
				avg[2] *= invSamp
				avg[3] *= invSamp
				di := (y*rw + x) * bpp
				encodePixelBytes(out[di:di+bpp], tex.desc.Format, avg)
			}
		}
		return &GPUSoftReadback{ID: op.ReadbackID, Pixels: out}, nil
	}
	return nil, nil
}

// decodeCompressedIntoTexture decodes a block-compressed upload region to RGBA8 and stores it in tex as linear floats (sRGB variants are decoded to linear like uncompressed sRGB textures).
func decodeCompressedIntoTexture(tex *softTexture, originX, originY, regionW, regionH uint32, layout GPUDataLayout, data []byte) error {
	tw := int(tex.desc.Width)
	rgba := make([]byte, tw*int(tex.desc.Height)*4)
	if err := decodeCompressedToRGBA(tex.desc.Format, rgba, tex.desc.Width, tex.desc.Height, originX, originY, regionW, regionH, layout, data); err != nil {
		return err
	}
	srgb := tex.desc.Format.IsSRGB()
	for y := int(originY); y < int(originY+regionH) && y < int(tex.desc.Height); y++ {
		for x := int(originX); x < int(originX+regionW) && x < tw; x++ {
			si := (y*tw + x) * 4
			c := [4]float32{float32(rgba[si]) / 255, float32(rgba[si+1]) / 255, float32(rgba[si+2]) / 255, float32(rgba[si+3]) / 255}
			if srgb {
				c[0], c[1], c[2] = srgbToLinear(c[0]), srgbToLinear(c[1]), srgbToLinear(c[2])
			}
			tex.storeTexel(y*tw+x, c)
		}
	}
	return nil
}

// storeTexel writes linear color c to every sample of pixel pixIdx.
func (t *softTexture) storeTexel(pixIdx int, c [4]float32) {
	n := max(t.samples, 1)
	for k := 0; k < n; k++ {
		copy(t.color[(pixIdx*n+k)*4:], c[:])
	}
}

func (d *GPUSoftwareDevice) execCopy(cp GPUCopyOp) error {
	switch cp.Kind {
	case GPUCopyBufferToBuffer:
		s, ok1 := d.buffers[cp.SrcID]
		t, ok2 := d.buffers[cp.DstID]
		if !ok1 || !ok2 {
			return fmt.Errorf("gpu: CopyBufferToBuffer references unknown buffer")
		}
		if cp.SrcOffset+cp.Size > uint64(len(s.data)) || cp.DstOffset+cp.Size > uint64(len(t.data)) {
			return fmt.Errorf("gpu: CopyBufferToBuffer out of bounds")
		}
		copy(t.data[cp.DstOffset:cp.DstOffset+cp.Size], s.data[cp.SrcOffset:cp.SrcOffset+cp.Size])
	case GPUCopyBufferToTexture:
		s, ok1 := d.buffers[cp.SrcID]
		tb, ok2 := d.textures[cp.DstTex.Texture]
		t := tb.level(cp.DstTex.ArrayLayer, cp.DstTex.MipLevel)
		if !ok1 || !ok2 || t == nil || t.color == nil {
			return fmt.Errorf("gpu: CopyBufferToTexture references unknown resource")
		}
		if t.desc.Format.IsCompressed() {
			return decodeCompressedIntoTexture(t, cp.DstTex.OriginX, cp.DstTex.OriginY, cp.Width, cp.Height, cp.SrcLayout, s.data)
		}
		bpp := int(t.desc.Format.BytesPerPixel())
		if bpp == 0 {
			bpp = 4
		}
		stride := int(cp.SrcLayout.BytesPerRow)
		if stride == 0 {
			stride = int(cp.Width) * bpp
		}
		src := s.data
		if cp.SrcLayout.Offset > uint64(len(src)) {
			return fmt.Errorf("gpu: CopyBufferToTexture offset out of bounds")
		}
		src = src[cp.SrcLayout.Offset:]
		dw := int(t.desc.Width)
		ox, oy := int(cp.DstTex.OriginX), int(cp.DstTex.OriginY)
		for y := 0; y < int(cp.Height) && oy+y < int(t.desc.Height); y++ {
			rowStart := y * stride
			if rowStart >= len(src) {
				break
			}
			for x := 0; x < int(cp.Width) && ox+x < dw; x++ {
				si := rowStart + x*bpp
				if si+bpp > len(src) {
					break
				}
				t.storeTexel((oy+y)*dw+(ox+x), quantizeColorForFormat(t.desc.Format, decodePixelBytes(src[si:si+bpp], t.desc.Format)))
			}
		}
	case GPUCopyTextureToTexture:
		sb, ok1 := d.textures[cp.SrcTex.Texture]
		tb, ok2 := d.textures[cp.DstTex.Texture]
		if !ok1 || !ok2 {
			return fmt.Errorf("gpu: CopyTextureToTexture references unknown texture")
		}
		for l := uint32(0); l < max(cp.Layers, 1); l++ {
			s, t := sb.level(cp.SrcTex.ArrayLayer+l, cp.SrcTex.MipLevel), tb.level(cp.DstTex.ArrayLayer+l, cp.DstTex.MipLevel)
			if s == nil || t == nil {
				return fmt.Errorf("gpu: CopyTextureToTexture subresource out of bounds")
			}
			if err := copySoftTextureRegion(s, t, cp); err != nil {
				return err
			}
		}
	}
	return nil
}

// downsampleSoftLevel fills dst with a 2x2 box filter of src. Colors are stored linear, so sRGB formats are averaged in linear space before quantizing for dst's format.
func downsampleSoftLevel(src, dst *softTexture) {
	if src == nil || dst == nil || src.color == nil || dst.color == nil {
		return
	}
	sw, sh := int(src.desc.Width), int(src.desc.Height)
	dw, dh := int(dst.desc.Width), int(dst.desc.Height)
	for y := range dh {
		y0 := min(y*2, sh-1)
		y1 := min(y0+1, sh-1)
		for x := range dw {
			x0 := min(x*2, sw-1)
			x1 := min(x0+1, sw-1)
			var c [4]float32
			for _, p := range [4]int{y0*sw + x0, y0*sw + x1, y1*sw + x0, y1*sw + x1} {
				for ch := range 4 {
					c[ch] += src.color[p*4+ch] * 0.25
				}
			}
			dst.storeTexel(y*dw+x, quantizeColorForFormat(dst.desc.Format, c))
		}
	}
}

// copySoftTextureRegion copies a cp.Width x cp.Height region between two single-subresource texture views, clipped to both.
func copySoftTextureRegion(s, t *softTexture, cp GPUCopyOp) error {
	cp.Width = min(cp.Width, s.desc.Width-min(cp.SrcTex.OriginX, s.desc.Width), t.desc.Width-min(cp.DstTex.OriginX, t.desc.Width))
	cp.Height = min(cp.Height, s.desc.Height-min(cp.SrcTex.OriginY, s.desc.Height), t.desc.Height-min(cp.DstTex.OriginY, t.desc.Height))
	sw, dw := int(s.desc.Width), int(t.desc.Width)
	if s.depth != nil && t.depth != nil {
		sn, tn := max(s.samples, 1), max(t.samples, 1)
		for y := 0; y < int(cp.Height); y++ {
			for x := 0; x < int(cp.Width); x++ {
				v := s.depth[((int(cp.SrcTex.OriginY)+y)*sw+int(cp.SrcTex.OriginX)+x)*sn]
				dp := (int(cp.DstTex.OriginY)+y)*dw + int(cp.DstTex.OriginX) + x
				for k := 0; k < tn; k++ {
					t.depth[dp*tn+k] = v
				}
			}
		}
		return nil
	}
	if s.color == nil || t.color == nil {
		return fmt.Errorf("gpu: CopyTextureToTexture format mismatch between color and depth textures")
	}
	for y := 0; y < int(cp.Height); y++ {
		for x := 0; x < int(cp.Width); x++ {
			sp := (int(cp.SrcTex.OriginY)+y)*sw + int(cp.SrcTex.OriginX) + x
			dp := (int(cp.DstTex.OriginY)+y)*dw + int(cp.DstTex.OriginX) + x
			si := sp * max(s.samples, 1) * 4
			t.storeTexel(dp, quantizeColorForFormat(t.desc.Format, [4]float32{s.color[si], s.color[si+1], s.color[si+2], s.color[si+3]}))
		}
	}
	return nil
}

func mergeShaderPrograms(vs, fs *shaderir.Program) *shaderir.Program {
	if vs == fs && vs != nil {
		return vs
	}
	if vs == nil {
		return fs
	}
	if fs == nil {
		return vs
	}
	// Separate VS and FS modules: keep VS's attributes/bindings/VertexFunc and FS's FragmentFunc.
	out := *vs
	out.FragmentFunc = fs.FragmentFunc
	out.FragmentOutputs = fs.FragmentOutputs
	out.FragmentOutputMeta = fs.FragmentOutputMeta
	if len(fs.Varyings) > len(out.Varyings) {
		out.Varyings = fs.Varyings
		out.VaryingMeta = fs.VaryingMeta
	}
	return &out
}

type softColorTarget struct {
	color      []float32
	format     GPUTextureFormat
	samples    int
	width      int
	height     int
	isSwap     bool
	resolveTex *softTexture
}

var (
	msaa1Offsets = [1][2]float32{{0.5, 0.5}}
	msaa4Offsets = [4][2]float32{
		{0.375, 0.125},
		{0.875, 0.375},
		{0.125, 0.625},
		{0.625, 0.875},
	}
)

func (d *GPUSoftwareDevice) execPass(pass GPUPass) error {
	if pass.IsCompute {
		return d.execComputePass(pass)
	}
	var colorTargets []softColorTarget
	var depthPix []float32
	var stencilPix []uint8
	var dsFormat GPUTextureFormat
	var targetW, targetH int
	passSamples := 1

	attachSwap := func(ca GPUColorAttachment) {
		ct := softColorTarget{
			color:   d.swapColor,
			format:  d.cfg.ColorFormat,
			samples: max(d.swapSamples, 1),
			width:   d.width,
			height:  d.height,
			isSwap:  true,
		}
		if ct.format == GPUFormatUndefined {
			ct.format = GPUFormatRGBA8Unorm
		}
		if ca.LoadOp == GPULoadClear {
			r, g, b, a := ca.ClearColor.Components()
			clearColorBuffer(ct.color, ct.format, [4]float32{float32(r), float32(g), float32(b), float32(a)})
		}
		colorTargets = append(colorTargets, ct)
		if targetW == 0 {
			targetW, targetH, passSamples = ct.width, ct.height, ct.samples
		}
	}

	if len(pass.ColorAttachments) > maxSoftColorAttachments {
		return fmt.Errorf("gpu: render pass has %d color attachments, the software device supports %d", len(pass.ColorAttachments), maxSoftColorAttachments)
	}
	for _, ca := range pass.ColorAttachments {
		if ca.Texture == 0 {
			attachSwap(ca)
			continue
		}
		base, ok := d.textures[ca.Texture]
		if !ok {
			return fmt.Errorf("gpu: render pass color attachment %d not found", ca.Texture)
		}
		tex := base.level(ca.ArrayLayer, ca.MipLevel)
		if tex == nil {
			return fmt.Errorf("gpu: render pass color attachment %d has no layer %d mip %d", ca.Texture, ca.ArrayLayer, ca.MipLevel)
		}
		if tex.color == nil {
			return fmt.Errorf("gpu: render pass color attachment %d is a depth texture", ca.Texture)
		}
		if tex.desc.Usage&GPUTextureRenderAttachment == 0 {
			return fmt.Errorf("gpu: render pass color attachment %d lacks TextureRenderAttachment usage", ca.Texture)
		}
		var resolveTex *softTexture
		if ca.ResolveTarget != 0 {
			rt, ok := d.textures[ca.ResolveTarget]
			if !ok {
				return fmt.Errorf("gpu: render pass resolve target %d not found", ca.ResolveTarget)
			}
			if rt.color == nil {
				return fmt.Errorf("gpu: render pass resolve target %d is not a color texture", ca.ResolveTarget)
			}
			resolveTex = rt
		}
		ct := softColorTarget{
			color:      tex.color,
			format:     tex.desc.Format,
			samples:    max(tex.samples, 1),
			width:      int(tex.desc.Width),
			height:     int(tex.desc.Height),
			resolveTex: resolveTex,
		}
		if ca.LoadOp == GPULoadClear {
			r, g, b, a := ca.ClearColor.Components()
			clearColorBuffer(ct.color, ct.format, [4]float32{float32(r), float32(g), float32(b), float32(a)})
		}
		colorTargets = append(colorTargets, ct)
		if targetW == 0 {
			targetW, targetH, passSamples = ct.width, ct.height, ct.samples
		} else if ct.width != targetW || ct.height != targetH {
			return fmt.Errorf("gpu: render pass color attachments have different sizes")
		}
	}
	if len(pass.ColorAttachments) == 0 && pass.DepthStencil == nil {
		// A pass with neither color nor depth draws into the swapchain; a pass with only a depth/stencil attachment is depth-only.
		attachSwap(GPUColorAttachment{LoadOp: GPULoadLoad, StoreOp: GPUStoreStore})
	}

	useSwapDepth := func() {
		dsFormat = d.cfg.DepthFormat
		if dsFormat.IsDepth() {
			depthPix = d.depthBuf
		}
		if dsFormat.HasStencil() {
			stencilPix = d.stencilBuf
		}
	}
	if pass.DepthStencil != nil {
		ds := pass.DepthStencil
		if ds.Texture == 0 {
			useSwapDepth()
			if targetW == 0 || targetH == 0 {
				targetW, targetH, passSamples = d.width, d.height, max(d.swapSamples, 1)
			}
		} else {
			tex, ok := d.textures[ds.Texture]
			if !ok {
				return fmt.Errorf("gpu: render pass depth attachment %d not found", ds.Texture)
			}
			if tex.depth == nil && tex.stencil == nil {
				return fmt.Errorf("gpu: render pass depth attachment %d is not a depth texture", ds.Texture)
			}
			if tex.desc.Usage&GPUTextureRenderAttachment == 0 {
				return fmt.Errorf("gpu: render pass depth attachment %d lacks TextureRenderAttachment usage", ds.Texture)
			}
			if targetW == 0 || targetH == 0 {
				targetW, targetH, passSamples = int(tex.desc.Width), int(tex.desc.Height), max(tex.samples, 1)
			} else if int(tex.desc.Width) != targetW || int(tex.desc.Height) != targetH {
				return fmt.Errorf("gpu: render pass depth attachment %dx%d does not match color attachment %dx%d",
					tex.desc.Width, tex.desc.Height, targetW, targetH)
			} else if max(tex.samples, 1) != passSamples {
				return fmt.Errorf("gpu: render pass depth attachment sample count %d does not match color attachment sample count %d", max(tex.samples, 1), passSamples)
			}
			dsFormat = tex.desc.Format
			depthPix = tex.depth
			stencilPix = tex.stencil
		}
		if depthPix != nil && ds.DepthLoadOp == GPULoadClear {
			for i := range depthPix {
				depthPix[i] = ds.ClearDepth
			}
		}
		if stencilPix != nil && ds.StencilLoadOp == GPULoadClear {
			v := uint8(ds.ClearStencil & 0xFF)
			for i := range stencilPix {
				stencilPix[i] = v
			}
		}
	} else if d.cfg.DepthFormat.IsDepthStencil() && len(colorTargets) > 0 && colorTargets[0].isSwap {
		useSwapDepth()
	}

	// Render pass state
	vpX, vpY, vpW, vpH := float32(0), float32(0), float32(targetW), float32(targetH)
	vpMinD, vpMaxD := float32(0), float32(1)
	scX, scY, scW, scH := int32(0), int32(0), int32(targetW), int32(targetH)
	var pipe *softPipeline
	bindGroups := map[uint32]softBoundGroup{}
	type vbSlot struct {
		buf    *softBuffer
		offset uint64
	}
	vbufs := map[uint32]vbSlot{}
	var idxBuf *softBuffer
	var idxFmt GPUIndexFormat
	var idxOff uint64
	blendConst := [4]float32{0, 0, 0, 0}
	var stencilRef uint8

	// Reusable scratch slices across draws inside a pass.
	var (
		scratchAttrs        []irVal
		scratchVLocals      []irVal
		scratchFLocals      []irVal
		scratchVaryings     []irVal
		scratchBindVal      []irVal
		scratchBindHit      []bool
		v0Out, v1Out, v2Out shadedVertex
	)

	draw := func(indexed bool, count, instances, first, firstInst uint32, baseVertex int32) error {
		if pipe == nil || pipe.combined == nil || pipe.combined.VertexFunc.Block == nil {
			return fmt.Errorf("gpu: draw without bound render pipeline")
		}
		if count == 0 || instances == 0 {
			return nil
		}
		prog := pipe.combined
		nb := len(prog.Bindings)
		if cap(scratchBindVal) < nb {
			scratchBindVal = make([]irVal, nb)
			scratchBindHit = make([]bool, nb)
		} else {
			scratchBindVal = scratchBindVal[:nb]
			scratchBindHit = scratchBindHit[:nb]
			clear(scratchBindHit)
		}
		nv := len(prog.Varyings)
		if cap(scratchVaryings) < nv {
			scratchVaryings = make([]irVal, nv)
		} else {
			scratchVaryings = scratchVaryings[:nv]
		}
		if cap(v0Out.varyings) < nv {
			v0Out.varyings = make([]irVal, nv)
			v1Out.varyings = make([]irVal, nv)
			v2Out.varyings = make([]irVal, nv)
		} else {
			v0Out.varyings = v0Out.varyings[:nv]
			v1Out.varyings = v1Out.varyings[:nv]
			v2Out.varyings = v2Out.varyings[:nv]
		}
		na := len(prog.Attributes)
		if cap(scratchAttrs) < na {
			scratchAttrs = make([]irVal, na)
		} else {
			scratchAttrs = scratchAttrs[:na]
		}
		vLocalN := na + nv + 1 + len(prog.VertexFunc.Block.LocalVars)
		if cap(scratchVLocals) < vLocalN {
			scratchVLocals = make([]irVal, vLocalN)
		} else {
			scratchVLocals = scratchVLocals[:vLocalN]
		}
		fLocalN := nv + 1 + len(prog.FragmentOutputs)
		if prog.FragmentFunc.Block != nil {
			fLocalN += len(prog.FragmentFunc.Block.LocalVars)
		}
		if cap(scratchFLocals) < fLocalN {
			scratchFLocals = make([]irVal, fLocalN)
		} else {
			scratchFLocals = scratchFLocals[:fLocalN]
		}

		ctx := &softDrawCtx{
			dev:            d,
			prog:           prog,
			pipe:           pipe,
			bindGroups:     bindGroups,
			bindingCache:   scratchBindVal,
			bindingCached:  scratchBindHit,
			vertLocals:     scratchVLocals,
			fragLocals:     scratchFLocals,
			interpVaryings: scratchVaryings,
			colorTargets:   colorTargets,
			depthPix:       depthPix,
			stencilPix:     stencilPix,
			dsFormat:       dsFormat,
			sampleCount:    passSamples,
			targetW:        targetW,
			targetH:        targetH,
			vpX:            vpX, vpY: vpY, vpW: vpW, vpH: vpH,
			vpMinD: vpMinD, vpMaxD: vpMaxD,
			scX: int(scX), scY: int(scY), scW: int(scW), scH: int(scH),
			blendConst: blendConst,
			stencilRef: stencilRef,
		}
		fetchIndex := func(i uint32) (uint32, error) {
			if !indexed {
				return first + i, nil
			}
			if idxBuf == nil {
				return 0, fmt.Errorf("gpu: DrawIndexed without bound index buffer")
			}
			if idxFmt == GPUIndexUint16 {
				off := idxOff + uint64(first+i)*2
				if off+2 > uint64(len(idxBuf.data)) {
					return 0, fmt.Errorf("gpu: index buffer out of bounds")
				}
				return uint32(int32(binary.LittleEndian.Uint16(idxBuf.data[off:off+2])) + baseVertex), nil
			}
			off := idxOff + uint64(first+i)*4
			if off+4 > uint64(len(idxBuf.data)) {
				return 0, fmt.Errorf("gpu: index buffer out of bounds")
			}
			return uint32(int32(binary.LittleEndian.Uint32(idxBuf.data[off:off+4])) + baseVertex), nil
		}

		fetchAttrs := func(vertIdx, instOffset uint32) []irVal {
			for i, at := range prog.Attributes {
				scratchAttrs[i] = zeroIRVal(at)
			}
			for _, am := range pipe.attrs {
				vb, ok := vbufs[am.slotIdx]
				if !ok || vb.buf == nil {
					continue
				}
				elemIdx := vertIdx
				if am.stepMode == GPUStepInstance {
					sr := am.stepRate
					if sr == 0 {
						sr = 1
					}
					elemIdx = firstInst + instOffset/sr
				}
				off := vb.offset + uint64(elemIdx)*am.stride + am.offset
				if off+am.format.ByteSize() > uint64(len(vb.buf.data)) {
					continue
				}
				scratchAttrs[am.irIdx] = decodeVertexAttr(vb.buf.data[off:], am.format)
			}
			return scratchAttrs
		}

		for inst := uint32(0); inst < instances; inst++ {
			instID := firstInst + inst
			switch pipe.desc.Primitive.Topology {
			case GPUTopologyTriangleStrip:
				if count < 3 {
					continue
				}
				for t := uint32(0); t+2 < count; t++ {
					i0, err := fetchIndex(t)
					if err != nil {
						return err
					}
					i1, err := fetchIndex(t + 1)
					if err != nil {
						return err
					}
					i2, err := fetchIndex(t + 2)
					if err != nil {
						return err
					}
					if t%2 == 1 {
						i1, i2 = i2, i1
					}
					ctx.runVertexInto(&v0Out, fetchAttrs(i0, inst), i0, instID)
					ctx.runVertexInto(&v1Out, fetchAttrs(i1, inst), i1, instID)
					ctx.runVertexInto(&v2Out, fetchAttrs(i2, inst), i2, instID)
					ctx.rasterTriangle(v0Out, v1Out, v2Out)
				}
			default: // TriangleList
				for t := uint32(0); t+2 < count; t += 3 {
					i0, err := fetchIndex(t)
					if err != nil {
						return err
					}
					i1, err := fetchIndex(t + 1)
					if err != nil {
						return err
					}
					i2, err := fetchIndex(t + 2)
					if err != nil {
						return err
					}
					ctx.runVertexInto(&v0Out, fetchAttrs(i0, inst), i0, instID)
					ctx.runVertexInto(&v1Out, fetchAttrs(i1, inst), i1, instID)
					ctx.runVertexInto(&v2Out, fetchAttrs(i2, inst), i2, instID)
					ctx.rasterTriangle(v0Out, v1Out, v2Out)
				}
			}
		}
		return nil
	}

	for _, cmd := range pass.Commands {
		switch cmd.Kind {
		case GPUCmdSetViewport:
			vpX, vpY, vpW, vpH, vpMinD, vpMaxD = cmd.F0, cmd.F1, cmd.F2, cmd.F3, cmd.F4, cmd.F5
		case GPUCmdSetScissor:
			scX, scY, scW, scH = cmd.I0, cmd.I1, cmd.I2, cmd.I3
		case GPUCmdSetPipeline:
			p, ok := d.pipelines[cmd.ID]
			if !ok {
				return fmt.Errorf("gpu: unknown pipeline %d", cmd.ID)
			}
			pipe = p
		case GPUCmdSetBindGroup:
			bound, err := d.boundGroup(cmd)
			if err != nil {
				return err
			}
			bindGroups[cmd.Slot] = bound
		case GPUCmdSetVertexBuffer:
			b, ok := d.buffers[cmd.ID]
			if !ok {
				return fmt.Errorf("gpu: unknown vertex buffer %d", cmd.ID)
			}
			vbufs[cmd.Slot] = vbSlot{buf: b, offset: cmd.Offset}
		case GPUCmdSetIndexBuffer:
			b, ok := d.buffers[cmd.ID]
			if !ok {
				return fmt.Errorf("gpu: unknown index buffer %d", cmd.ID)
			}
			idxBuf, idxFmt, idxOff = b, cmd.IndexFormat, cmd.Offset
		case GPUCmdSetStencilReference:
			stencilRef = uint8(cmd.U0 & 0xFF)
		case GPUCmdSetBlendConstant:
			blendConst = [4]float32{cmd.F0, cmd.F1, cmd.F2, cmd.F3}
		case GPUCmdDraw:
			if err := draw(false, cmd.U0, 1, cmd.U1, 0, 0); err != nil {
				return err
			}
		case GPUCmdDrawIndexed:
			if err := draw(true, cmd.U0, 1, cmd.U1, 0, cmd.I0); err != nil {
				return err
			}
		case GPUCmdDrawInstanced:
			if err := draw(false, cmd.U0, cmd.U1, cmd.U2, cmd.U3, 0); err != nil {
				return err
			}
		case GPUCmdDrawIndexedInstanced:
			if err := draw(true, cmd.U0, cmd.U1, cmd.U2, cmd.U3, cmd.I0); err != nil {
				return err
			}
		case GPUCmdPushDebugGroup:
			d.debugEvents = append(d.debugEvents, GPUDebugEvent{Kind: "pass_push_group", Label: cmd.Label, Depth: d.debugDepth})
			d.debugDepth++
		case GPUCmdPopDebugGroup:
			if d.debugDepth > 0 {
				d.debugDepth--
			}
			d.debugEvents = append(d.debugEvents, GPUDebugEvent{Kind: "pass_pop_group", Depth: d.debugDepth})
		case GPUCmdInsertDebugMarker:
			d.debugEvents = append(d.debugEvents, GPUDebugEvent{Kind: "pass_marker", Label: cmd.Label, Depth: d.debugDepth})
		}
	}

	// End-of-pass MSAA resolve and swapchain snapshot flush. Resolves run even when the pass recorded no draws, which is how Encoder.ResolveTexture is expressed.
	for _, ct := range colorTargets {
		if ct.isSwap {
			d.flushSwapchain()
		} else if ct.resolveTex != nil {
			resolveColorBuffers(ct.color, ct.samples, ct.width, ct.height, ct.resolveTex.color, ct.resolveTex.desc.Format, int(ct.resolveTex.desc.Width), int(ct.resolveTex.desc.Height))
		}
	}
	return nil
}

func (d *GPUSoftwareDevice) flushSwapchain() {
	n := d.width * d.height
	samples := max(d.swapSamples, 1)
	invSamp := float32(1.0) / float32(samples)
	fmtVal := d.cfg.ColorFormat
	if fmtVal == GPUFormatUndefined {
		fmtVal = GPUFormatRGBA8Unorm
	}
	for p := 0; p < n; p++ {
		var avg [4]float32
		for s := 0; s < samples; s++ {
			si := (p*samples + s) * 4
			avg[0] += d.swapColor[si+0]
			avg[1] += d.swapColor[si+1]
			avg[2] += d.swapColor[si+2]
			avg[3] += d.swapColor[si+3]
		}
		avg[0] *= invSamp
		avg[1] *= invSamp
		avg[2] *= invSamp
		avg[3] *= invSamp
		di := p * 4
		if fmtVal.IsSRGB() {
			d.colorBuf[di+0] = floatToByte(linearToSRGB(avg[0]))
			d.colorBuf[di+1] = floatToByte(linearToSRGB(avg[1]))
			d.colorBuf[di+2] = floatToByte(linearToSRGB(avg[2]))
			d.colorBuf[di+3] = floatToByte(avg[3])
		} else {
			d.colorBuf[di+0] = floatToByte(avg[0])
			d.colorBuf[di+1] = floatToByte(avg[1])
			d.colorBuf[di+2] = floatToByte(avg[2])
			d.colorBuf[di+3] = floatToByte(avg[3])
		}
	}
}

func resolveColorBuffers(src []float32, srcSamples, srcW, srcH int, dst []float32, dstFmt GPUTextureFormat, dstW, dstH int) {
	w := min(srcW, dstW)
	h := min(srcH, dstH)
	samples := max(srcSamples, 1)
	invSamp := float32(1.0) / float32(samples)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sp := y*srcW + x
			var avg [4]float32
			for s := 0; s < samples; s++ {
				si := (sp*samples + s) * 4
				avg[0] += src[si+0]
				avg[1] += src[si+1]
				avg[2] += src[si+2]
				avg[3] += src[si+3]
			}
			avg[0] *= invSamp
			avg[1] *= invSamp
			avg[2] *= invSamp
			avg[3] *= invSamp
			q := quantizeColorForFormat(dstFmt, avg)
			di := (y*dstW + x) * 4
			dst[di+0] = q[0]
			dst[di+1] = q[1]
			dst[di+2] = q[2]
			dst[di+3] = q[3]
		}
	}
}

func clearColorBuffer(buf []float32, format GPUTextureFormat, clearRGBA [4]float32) {
	q := quantizeColorForFormat(format, clearRGBA)
	for i := 0; i+3 < len(buf); i += 4 {
		buf[i+0] = q[0]
		buf[i+1] = q[1]
		buf[i+2] = q[2]
		buf[i+3] = q[3]
	}
}

// boundGroup resolves a SetBindGroup command into the bind group and its validated dynamic offsets, shared by render and compute passes.
func (d *GPUSoftwareDevice) boundGroup(cmd GPUCommand) (softBoundGroup, error) {
	bg, ok := d.bindGroups[cmd.ID]
	if !ok {
		return softBoundGroup{}, fmt.Errorf("gpu: unknown bind group %d", cmd.ID)
	}
	minAlign := uint64(DefaultGPULimits().MinUniformBufferAlignment)
	var dynOffsets map[uint32]uint64
	if layout := d.layouts[bg.desc.Layout]; layout != nil && (len(layout.dynamicEntries) > 0 || len(cmd.DynamicOffsets) > 0) {
		if len(cmd.DynamicOffsets) != len(layout.dynamicEntries) {
			return softBoundGroup{}, fmt.Errorf("gpu: SetBindGroup slot %d got %d dynamic offsets, want %d", cmd.Slot, len(cmd.DynamicOffsets), len(layout.dynamicEntries))
		}
		dynOffsets = make(map[uint32]uint64, len(layout.dynamicEntries))
		for i, le := range layout.dynamicEntries {
			off := uint64(cmd.DynamicOffsets[i])
			if le.Type == GPUBindingUniform && minAlign > 0 && off%minAlign != 0 {
				return softBoundGroup{}, fmt.Errorf("gpu: dynamic uniform offset %d is not aligned to %d bytes", off, minAlign)
			}
			if entry, ok := bg.entries[le.Binding]; ok {
				if buf := d.buffers[entry.Buffer]; buf != nil && entry.Offset+off > uint64(len(buf.data)) {
					return softBoundGroup{}, fmt.Errorf("gpu: dynamic offset %d out of bounds for buffer %d", off, entry.Buffer)
				}
			}
			dynOffsets[le.Binding] = off
		}
	}
	return softBoundGroup{bg: bg, dynamicOffsets: dynOffsets}, nil
}

func (d *GPUSoftwareDevice) execComputePass(pass GPUPass) error {
	var pipe *softComputePipeline
	bindGroups := map[uint32]softBoundGroup{}

	dispatch := func(numX, numY, numZ uint32) error {
		if pipe == nil || pipe.desc.Compute.IR == nil || pipe.desc.Compute.IR.ComputeFunc.Block == nil {
			return fmt.Errorf("gpu: dispatch without bound compute pipeline")
		}
		if numX == 0 || numY == 0 || numZ == 0 {
			return nil
		}
		prog := pipe.desc.Compute.IR
		ws := prog.ComputeFunc.WorkgroupSize
		if ws[0] == 0 {
			ws[0] = 1
		}
		if ws[1] == 0 {
			ws[1] = 1
		}
		if ws[2] == 0 {
			ws[2] = 1
		}
		ctx := &softDrawCtx{
			dev:        d,
			prog:       prog,
			bindGroups: bindGroups,
		}
		block := prog.ComputeFunc.Block
		for wgZ := uint32(0); wgZ < numZ; wgZ++ {
			for wgY := uint32(0); wgY < numY; wgY++ {
				for wgX := uint32(0); wgX < numX; wgX++ {
					for lz := uint32(0); lz < ws[2]; lz++ {
						for ly := uint32(0); ly < ws[1]; ly++ {
							for lx := uint32(0); lx < ws[0]; lx++ {
								locals := make([]irVal, len(block.LocalVars))
								for i, lt := range block.LocalVars {
									locals[i] = zeroIRVal(lt)
								}
								vm := &irVM{
									ctx:           ctx,
									locals:        locals,
									globalID:      [3]uint32{wgX*ws[0] + lx, wgY*ws[1] + ly, wgZ*ws[2] + lz},
									localID:       [3]uint32{lx, ly, lz},
									workgroupID:   [3]uint32{wgX, wgY, wgZ},
									numWorkgroups: [3]uint32{numX, numY, numZ},
									localIdx:      lz*ws[1]*ws[0] + ly*ws[0] + lx,
								}
								vm.execBlock(block)
							}
						}
					}
				}
			}
		}
		return nil
	}

	for _, cmd := range pass.Commands {
		switch cmd.Kind {
		case GPUCmdSetComputePipeline:
			p, ok := d.computePipelines[cmd.ID]
			if !ok {
				return fmt.Errorf("gpu: unknown compute pipeline %d", cmd.ID)
			}
			pipe = p
		case GPUCmdSetBindGroup:
			bound, err := d.boundGroup(cmd)
			if err != nil {
				return err
			}
			bindGroups[cmd.Slot] = bound
		case GPUCmdDispatch:
			if err := dispatch(cmd.U0, cmd.U1, cmd.U2); err != nil {
				return err
			}
		case GPUCmdDispatchIndirect:
			b, ok := d.buffers[cmd.ID]
			if !ok {
				return fmt.Errorf("gpu: unknown indirect dispatch buffer %d", cmd.ID)
			}
			if cmd.Offset+12 > uint64(len(b.data)) {
				return fmt.Errorf("gpu: indirect dispatch buffer out of bounds")
			}
			x := binary.LittleEndian.Uint32(b.data[cmd.Offset : cmd.Offset+4])
			y := binary.LittleEndian.Uint32(b.data[cmd.Offset+4 : cmd.Offset+8])
			z := binary.LittleEndian.Uint32(b.data[cmd.Offset+8 : cmd.Offset+12])
			if err := dispatch(x, y, z); err != nil {
				return err
			}
		}
	}

	return nil
}

// --- Software rasterizer & shaderir VM ---

type softBoundGroup struct {
	bg             *softBindGroup
	dynamicOffsets map[uint32]uint64
}

type softDrawCtx struct {
	dev            *GPUSoftwareDevice
	prog           *shaderir.Program
	pipe           *softPipeline
	bindGroups     map[uint32]softBoundGroup
	bindingCache   []irVal
	bindingCached  []bool
	vertLocals     []irVal
	fragLocals     []irVal
	interpVaryings []irVal
	colorTargets   []softColorTarget
	depthPix       []float32
	stencilPix     []uint8
	dsFormat       GPUTextureFormat
	sampleCount    int
	targetW        int
	targetH        int
	vpX, vpY       float32
	vpW, vpH       float32
	vpMinD         float32
	vpMaxD         float32
	scX, scY       int
	scW, scH       int
	blendConst     [4]float32
	stencilRef     uint8
}

type shadedVertex struct {
	pos      [4]float32
	varyings []irVal
}

func (c *softDrawCtx) runVertexInto(out *shadedVertex, attrs []irVal, vertID, instID uint32) {
	p := c.prog
	na, nv := len(p.Attributes), len(p.Varyings)
	pseudo := na + nv + 1
	locals := c.vertLocals
	copy(locals, attrs)
	locals[na] = irVal{typ: shaderir.Type{Main: shaderir.Vec4}, f: [16]float32{0, 0, 0, 1}}
	for i, vt := range p.Varyings {
		locals[na+1+i] = zeroIRVal(vt)
	}
	for i, lt := range p.VertexFunc.Block.LocalVars {
		locals[pseudo+i] = zeroIRVal(lt)
	}
	vm := irVM{
		ctx:    c,
		locals: locals,
		vertID: vertID,
		instID: instID,
	}
	vm.execBlock(p.VertexFunc.Block)
	out.pos = [4]float32{locals[na].f[0], locals[na].f[1], locals[na].f[2], locals[na].f[3]}
	for i := range nv {
		out.varyings[i] = locals[na+1+i]
	}
}

const maxSoftColorAttachments = 4

func (c *softDrawCtx) runFragment(fragCoord [4]float32, varyings []irVal, frontFacing bool) ([maxSoftColorAttachments][4]float32, bool) {
	var outs [maxSoftColorAttachments][4]float32
	p := c.prog
	if p.FragmentFunc.Block == nil {
		outs[0] = [4]float32{1, 1, 1, 1}
		return outs, false
	}
	nv := len(p.Varyings)
	no := len(p.FragmentOutputs)
	pseudo := nv + 1 + no
	locals := c.fragLocals
	locals[0] = irVal{typ: shaderir.Type{Main: shaderir.Vec4}, f: [16]float32{fragCoord[0], fragCoord[1], fragCoord[2], fragCoord[3]}}
	for i := 0; i < nv && i < len(varyings); i++ {
		locals[1+i] = varyings[i]
	}
	for i, ot := range p.FragmentOutputs {
		locals[nv+1+i] = zeroIRVal(ot)
	}
	for i, lt := range p.FragmentFunc.Block.LocalVars {
		locals[pseudo+i] = zeroIRVal(lt)
	}
	vm := irVM{
		ctx:         c,
		locals:      locals,
		frontFacing: frontFacing,
		fragCoord:   fragCoord,
	}
	vm.execBlock(p.FragmentFunc.Block)
	if vm.discarded {
		return outs, true
	}
	if no == 0 {
		outs[0] = [4]float32{vm.retVal.f[0], vm.retVal.f[1], vm.retVal.f[2], vm.retVal.f[3]}
		return outs, false
	}
	for i := 0; i < no; i++ {
		loc := uint32(i)
		if i < len(p.FragmentOutputMeta) {
			loc = p.FragmentOutputMeta[i].Location
		}
		if loc < maxSoftColorAttachments {
			v := locals[nv+1+i]
			outs[loc] = [4]float32{v.f[0], v.f[1], v.f[2], v.f[3]}
			if p.FragmentOutputs[i].Main == shaderir.Float {
				outs[loc][3] = 1.0
			}
		}
	}
	return outs, false
}

func (c *softDrawCtx) rasterTriangle(v0, v1, v2 shadedVertex) {
	// Skip triangles with non-positive W (behind camera).
	if v0.pos[3] <= 1e-6 || v1.pos[3] <= 1e-6 || v2.pos[3] <= 1e-6 {
		return
	}
	invW0, invW1, invW2 := 1.0/v0.pos[3], 1.0/v1.pos[3], 1.0/v2.pos[3]

	// Canonical WebGPU NDC: X in [-1, 1], Y in [-1, 1] (+Y up), Z in [0, 1].
	// Screen coordinates: X right, Y down.
	toScreen := func(p [4]float32, invW float32) (sx, sy, sz float32) {
		nx, ny, nz := p[0]*invW, p[1]*invW, p[2]*invW
		sx = c.vpX + (nx*0.5+0.5)*c.vpW
		sy = c.vpY + (0.5-ny*0.5)*c.vpH
		sz = c.vpMinD + nz*(c.vpMaxD-c.vpMinD)
		return
	}
	x0, y0, z0 := toScreen(v0.pos, invW0)
	x1, y1, z1 := toScreen(v1.pos, invW1)
	x2, y2, z2 := toScreen(v2.pos, invW2)

	// Signed screen-space area. Because screen Y goes DOWN while NDC Y goes UP,
	// a CCW triangle in NDC has NEGATIVE signed area in screen coordinates.
	area := (x1-x0)*(y2-y0) - (y1-y0)*(x2-x0)
	if math.Abs(float64(area)) < 1e-6 {
		return
	}
	isCCW := area < 0
	frontFacing := isCCW
	if c.pipe.desc.Primitive.FrontFace == GPUFrontCW {
		frontFacing = !isCCW
	}
	switch c.pipe.desc.Primitive.CullMode {
	case GPUCullBack:
		if !frontFacing {
			return
		}
	case GPUCullFront:
		if frontFacing {
			return
		}
	}

	minX := int(math.Floor(float64(min(x0, min(x1, x2)))))
	maxX := int(math.Ceil(float64(max(x0, max(x1, x2)))))
	minY := int(math.Floor(float64(min(y0, min(y1, y2)))))
	maxY := int(math.Ceil(float64(max(y0, max(y1, y2)))))

	// Clip to viewport, scissor, and target bounds.
	clipMinX := max(0, max(int(math.Floor(float64(c.vpX))), c.scX))
	clipMinY := max(0, max(int(math.Floor(float64(c.vpY))), c.scY))
	clipMaxX := min(c.targetW, min(int(math.Ceil(float64(c.vpX+c.vpW))), c.scX+c.scW))
	clipMaxY := min(c.targetH, min(int(math.Ceil(float64(c.vpY+c.vpH))), c.scY+c.scH))
	minX = max(minX, clipMinX)
	minY = max(minY, clipMinY)
	maxX = min(maxX, clipMaxX-1)
	maxY = min(maxY, clipMaxY-1)
	if minX > maxX || minY > maxY {
		return
	}

	invArea := 1.0 / area
	nv := len(c.prog.Varyings)
	interpVaryings := c.interpVaryings
	ds := c.pipe.desc.DepthStencil

	// Per-triangle depth bias (constant + slope-scaled, clamped).
	var depthBias float32
	hasDepthBias := ds != nil && (ds.DepthBias != 0 || ds.DepthBiasSlopeScale != 0)
	if hasDepthBias {
		dzdx := ((y1-y2)*z0 + (y2-y0)*z1 + (y0-y1)*z2) * invArea
		dzdy := ((x2-x1)*z0 + (x0-x2)*z1 + (x1-x0)*z2) * invArea
		maxSlope := max(float32(math.Abs(float64(dzdx))), float32(math.Abs(float64(dzdy))))
		r := float32(1.0) / float32((1<<24)-1)
		if c.dsFormat == GPUFormatDepth32Float {
			maxZ := max(z0, max(z1, z2))
			r = max(float32(1.0)/float32(1<<23), math.Nextafter32(maxZ, 2.0)-maxZ)
		}
		depthBias = float32(ds.DepthBias)*r + ds.DepthBiasSlopeScale*maxSlope
		if ds.DepthBiasClamp > 0 && depthBias > ds.DepthBiasClamp {
			depthBias = ds.DepthBiasClamp
		} else if ds.DepthBiasClamp < 0 && depthBias < ds.DepthBiasClamp {
			depthBias = ds.DepthBiasClamp
		}
	}

	// Active stencil face state & masks.
	var stencilFace GPUStencilFaceState
	var stencilReadMask, stencilWriteMask uint8 = 0xFF, 0xFF
	if ds != nil {
		if frontFacing {
			stencilFace = ds.StencilFront
		} else {
			stencilFace = ds.StencilBack
		}
		if ds.StencilReadMask != 0 {
			stencilReadMask = uint8(ds.StencilReadMask & 0xFF)
		}
		if ds.StencilWriteMask != 0 {
			stencilWriteMask = uint8(ds.StencilWriteMask & 0xFF)
		}
	}

	sampleCount := max(c.sampleCount, 1)
	var sampleOffsets [][2]float32
	if sampleCount == 4 {
		sampleOffsets = msaa4Offsets[:]
	} else {
		sampleCount = 1
		sampleOffsets = msaa1Offsets[:]
	}
	hasDepthTest := ds != nil && c.depthPix != nil && c.dsFormat.IsDepth()
	hasStencilTest := ds != nil && c.stencilPix != nil && c.dsFormat.HasStencil()

	baryAt := func(fx, fy float32) (w0, w1, w2 float32) {
		w0 = ((x1-fx)*(y2-fy) - (y1-fy)*(x2-fx)) * invArea
		w1 = ((x2-fx)*(y0-fy) - (y2-fy)*(x0-fx)) * invArea
		w2 = 1.0 - w0 - w1
		return
	}

	var sampleCovered [4]bool
	var sampleDepth [4]float32
	// A depth-only pass (no color targets) with no fragment shader skips shading entirely, as shadow-map passes do.
	skipFragment := len(c.colorTargets) == 0 && c.prog.FragmentFunc.Block == nil

	for py := minY; py <= maxY; py++ {
		for px := minX; px <= maxX; px++ {
			coveredCount := 0
			var centX, centY float32
			for s := 0; s < sampleCount; s++ {
				sx := float32(px) + sampleOffsets[s][0]
				sy := float32(py) + sampleOffsets[s][1]
				w0, w1, w2 := baryAt(sx, sy)
				if w0 < -1e-5 || w1 < -1e-5 || w2 < -1e-5 {
					sampleCovered[s] = false
					continue
				}
				z := w0*z0 + w1*z1 + w2*z2
				if hasDepthBias {
					z = clamp01(z + depthBias)
				}
				if z < 0 || z > 1 {
					sampleCovered[s] = false
					continue
				}
				sampleCovered[s] = true
				sampleDepth[s] = z
				coveredCount++
				centX += sx
				centY += sy
			}
			if coveredCount == 0 {
				continue
			}
			pixIdx := py*c.targetW + px
			if hasDepthTest && !hasStencilTest {
				// Early depth rejection: without stencil side effects, a pixel whose covered samples all fail the depth test needs no shading.
				anyPass := false
				for s := 0; s < sampleCount && !anyPass; s++ {
					anyPass = sampleCovered[s] && compareDepth(ds.DepthCompare, sampleDepth[s], c.depthPix[pixIdx*sampleCount+s])
				}
				if !anyPass {
					continue
				}
			}

			// Evaluate fragment shader at pixel center (or centroid of covered samples if center is outside).
			fx := float32(px) + 0.5
			fy := float32(py) + 0.5
			w0, w1, w2 := baryAt(fx, fy)
			if w0 < -1e-5 || w1 < -1e-5 || w2 < -1e-5 {
				fx = centX / float32(coveredCount)
				fy = centY / float32(coveredCount)
				w0, w1, w2 = baryAt(fx, fy)
			}
			centerDepth := w0*z0 + w1*z1 + w2*z2
			if hasDepthBias {
				centerDepth = clamp01(centerDepth + depthBias)
			}

			var fragOuts [maxSoftColorAttachments][4]float32
			if !skipFragment {
				// Perspective-correct interpolation weights.
				pw0, pw1, pw2 := w0*invW0, w1*invW1, w2*invW2
				invSum := float32(1.0) / (pw0 + pw1 + pw2)
				for vi := range nv {
					flat := vi < len(c.prog.VaryingMeta) && c.prog.VaryingMeta[vi].Interpolation == shaderir.InterpFlat
					if flat {
						interpVaryings[vi] = v0.varyings[vi]
					} else {
						interpVaryings[vi] = lerpIRVal(v0.varyings[vi], v1.varyings[vi], v2.varyings[vi], pw0*invSum, pw1*invSum, pw2*invSum)
					}
				}

				outs, discarded := c.runFragment([4]float32{fx, fy, centerDepth, invSum}, interpVaryings, frontFacing)
				if discarded {
					continue
				}
				fragOuts = outs
			}

			for s := 0; s < sampleCount; s++ {
				if !sampleCovered[s] {
					continue
				}
				sampIdx := pixIdx*sampleCount + s
				z := sampleDepth[s]

				// Stencil test (per WebGPU spec: compare(ref & readMask, buf & readMask)).
				if hasStencilTest {
					cur := c.stencilPix[sampIdx]
					if !compareStencil(stencilFace.Compare, c.stencilRef&stencilReadMask, cur&stencilReadMask) {
						next := applyStencilOp(stencilFace.FailOp, cur, c.stencilRef)
						c.stencilPix[sampIdx] = (cur &^ stencilWriteMask) | (next & stencilWriteMask)
						continue
					}
				}

				// Depth test.
				if hasDepthTest {
					if !compareDepth(ds.DepthCompare, z, c.depthPix[sampIdx]) {
						if hasStencilTest {
							cur := c.stencilPix[sampIdx]
							next := applyStencilOp(stencilFace.DepthFailOp, cur, c.stencilRef)
							c.stencilPix[sampIdx] = (cur &^ stencilWriteMask) | (next & stencilWriteMask)
						}
						continue
					}
				}

				// Both stencil and depth passed.
				if hasStencilTest {
					cur := c.stencilPix[sampIdx]
					next := applyStencilOp(stencilFace.PassOp, cur, c.stencilRef)
					c.stencilPix[sampIdx] = (cur &^ stencilWriteMask) | (next & stencilWriteMask)
				}
				if hasDepthTest && ds.DepthWrite {
					c.depthPix[sampIdx] = z
				}

				// Write all MRT color attachments for this sample.
				for ti := range c.colorTargets {
					ct := &c.colorTargets[ti]
					if ct.color == nil || ti >= maxSoftColorAttachments {
						continue
					}
					var writeMask GPUColorWriteMask = GPUColorWriteAll
					var blend *GPUBlendState
					if ti < len(c.pipe.desc.Targets) {
						ts := c.pipe.desc.Targets[ti]
						if ts.WriteMask != 0 {
							writeMask = ts.WriteMask
						}
						blend = ts.Blend
					}
					if writeMask == GPUColorWriteNone {
						continue
					}
					src := fragOuts[ti]
					isFloat := ct.format.IsFloat()
					if !isFloat {
						src = [4]float32{clamp01(src[0]), clamp01(src[1]), clamp01(src[2]), clamp01(src[3])}
					}
					off := sampIdx * 4
					if blend != nil {
						dst := [4]float32{ct.color[off+0], ct.color[off+1], ct.color[off+2], ct.color[off+3]}
						r, g, b, a := applyBlend(blend, src, dst, c.blendConst, !isFloat)
						src = [4]float32{r, g, b, a}
					}
					q := quantizeColorForFormat(ct.format, src)
					if writeMask&GPUColorWriteRed != 0 {
						ct.color[off+0] = q[0]
					}
					if writeMask&GPUColorWriteGreen != 0 {
						ct.color[off+1] = q[1]
					}
					if writeMask&GPUColorWriteBlue != 0 {
						ct.color[off+2] = q[2]
					}
					if writeMask&GPUColorWriteAlpha != 0 {
						ct.color[off+3] = q[3]
					}
				}
			}
		}
	}
}

func compareDepth(fn GPUCompareFunc, src, dst float32) bool {
	switch fn {
	case GPUCompareNever:
		return false
	case GPUCompareLess, GPUCompareUndefined:
		return src < dst
	case GPUCompareEqual:
		return src == dst
	case GPUCompareLessEqual:
		return src <= dst
	case GPUCompareGreater:
		return src > dst
	case GPUCompareNotEqual:
		return src != dst
	case GPUCompareGreaterEqual:
		return src >= dst
	case GPUCompareAlways:
		return true
	default:
		return src < dst
	}
}

func compareStencil(fn GPUCompareFunc, ref, buf uint8) bool {
	switch fn {
	case GPUCompareNever:
		return false
	case GPUCompareLess:
		return ref < buf
	case GPUCompareEqual:
		return ref == buf
	case GPUCompareLessEqual:
		return ref <= buf
	case GPUCompareGreater:
		return ref > buf
	case GPUCompareNotEqual:
		return ref != buf
	case GPUCompareGreaterEqual:
		return ref >= buf
	case GPUCompareAlways, GPUCompareUndefined:
		return true
	default:
		return true
	}
}

func applyStencilOp(op GPUStencilOperation, cur, ref uint8) uint8 {
	switch op {
	case GPUStencilZero:
		return 0
	case GPUStencilReplace:
		return ref
	case GPUStencilInvert:
		return ^cur
	case GPUStencilIncrementClamp:
		if cur < 255 {
			return cur + 1
		}
		return 255
	case GPUStencilDecrementClamp:
		if cur > 0 {
			return cur - 1
		}
		return 0
	case GPUStencilIncrementWrap:
		return cur + 1
	case GPUStencilDecrementWrap:
		return cur - 1
	default: // GPUStencilKeep
		return cur
	}
}

func blendFactorVal(f GPUBlendFactor, src, dst, c [4]float32) [4]float32 {
	switch f {
	case GPUBlendZero:
		return [4]float32{0, 0, 0, 0}
	case GPUBlendOne:
		return [4]float32{1, 1, 1, 1}
	case GPUBlendSrc:
		return src
	case GPUBlendOneMinusSrc:
		return [4]float32{1 - src[0], 1 - src[1], 1 - src[2], 1 - src[3]}
	case GPUBlendSrcAlpha:
		return [4]float32{src[3], src[3], src[3], src[3]}
	case GPUBlendOneMinusSrcAlpha:
		a := 1 - src[3]
		return [4]float32{a, a, a, a}
	case GPUBlendDst:
		return dst
	case GPUBlendOneMinusDst:
		return [4]float32{1 - dst[0], 1 - dst[1], 1 - dst[2], 1 - dst[3]}
	case GPUBlendDstAlpha:
		return [4]float32{dst[3], dst[3], dst[3], dst[3]}
	case GPUBlendOneMinusDstAlpha:
		a := 1 - dst[3]
		return [4]float32{a, a, a, a}
	case GPUBlendConstant:
		return c
	case GPUBlendOneMinusConstant:
		return [4]float32{1 - c[0], 1 - c[1], 1 - c[2], 1 - c[3]}
	default:
		return [4]float32{1, 1, 1, 1}
	}
}

func blendCombine(op GPUBlendOperation, s, d float32, clamp bool) float32 {
	var out float32
	switch op {
	case GPUBlendSubtract:
		out = s - d
	case GPUBlendReverseSubtract:
		out = d - s
	case GPUBlendMin:
		out = min(s, d)
	case GPUBlendMax:
		out = max(s, d)
	default:
		out = s + d
	}
	if clamp {
		return clamp01(out)
	}
	return out
}

func applyBlend(bs *GPUBlendState, src, dst, c [4]float32, clamp bool) (float32, float32, float32, float32) {
	if bs.Color.Operation == GPUBlendMin || bs.Color.Operation == GPUBlendMax {
		r := blendCombine(bs.Color.Operation, src[0], dst[0], clamp)
		g := blendCombine(bs.Color.Operation, src[1], dst[1], clamp)
		b := blendCombine(bs.Color.Operation, src[2], dst[2], clamp)
		var a float32
		if bs.Alpha.Operation == GPUBlendMin || bs.Alpha.Operation == GPUBlendMax {
			a = blendCombine(bs.Alpha.Operation, src[3], dst[3], clamp)
		} else {
			as := blendFactorVal(bs.Alpha.SrcFactor, src, dst, c)
			ad := blendFactorVal(bs.Alpha.DstFactor, src, dst, c)
			a = blendCombine(bs.Alpha.Operation, src[3]*as[3], dst[3]*ad[3], clamp)
		}
		return r, g, b, a
	}
	cs := blendFactorVal(bs.Color.SrcFactor, src, dst, c)
	cd := blendFactorVal(bs.Color.DstFactor, src, dst, c)
	r := blendCombine(bs.Color.Operation, src[0]*cs[0], dst[0]*cd[0], clamp)
	g := blendCombine(bs.Color.Operation, src[1]*cs[1], dst[1]*cd[1], clamp)
	b := blendCombine(bs.Color.Operation, src[2]*cs[2], dst[2]*cd[2], clamp)
	var a float32
	if bs.Alpha.Operation == GPUBlendMin || bs.Alpha.Operation == GPUBlendMax {
		a = blendCombine(bs.Alpha.Operation, src[3], dst[3], clamp)
	} else {
		as := blendFactorVal(bs.Alpha.SrcFactor, src, dst, c)
		ad := blendFactorVal(bs.Alpha.DstFactor, src, dst, c)
		a = blendCombine(bs.Alpha.Operation, src[3]*as[3], dst[3]*ad[3], clamp)
	}
	return r, g, b, a
}

// --- Format quantization, sRGB, float16, and RG11B10F conversion ---

func srgbToLinear(c float32) float32 {
	c = clamp01(c)
	if c <= 0.04045 {
		return c / 12.92
	}
	return float32(math.Pow(float64((c+0.055)/1.055), 2.4))
}

func linearToSRGB(c float32) float32 {
	c = clamp01(c)
	if c <= 0.0031308 {
		return c * 12.92
	}
	return float32(1.055*math.Pow(float64(c), 1.0/2.4) - 0.055)
}

func float32ToFloat16(f float32) uint16 {
	bits := math.Float32bits(f)
	sign := uint16((bits >> 16) & 0x8000)
	exp := int((bits>>23)&0xFF) - 127 + 15
	mant := bits & 0x007FFFFF

	if exp <= 0 {
		if exp < -10 {
			return sign
		}
		mant |= 0x00800000
		shift := uint(14 - exp)
		half := uint16(mant >> shift)
		// Round to nearest even
		roundBit := uint32(1) << (shift - 1)
		if (mant&roundBit) != 0 && ((mant&(roundBit-1)) != 0 || (half&1) != 0) {
			half++
		}
		return sign | half
	}
	if exp >= 31 {
		if ((bits>>23)&0xFF) == 0xFF && mant != 0 {
			return sign | 0x7E00 // NaN
		}
		return sign | 0x7C00 // Inf
	}
	half := uint16((exp << 10) | int(mant>>13))
	if (mant&0x1000) != 0 && ((mant&0x0FFF) != 0 || (half&1) != 0) {
		half++
	}
	return sign | half
}

func float16ToFloat32(h uint16) float32 {
	sign := uint32(h&0x8000) << 16
	exp := uint32((h >> 10) & 0x1F)
	mant := uint32(h & 0x03FF)
	switch exp {
	case 0:
		if mant == 0 {
			return math.Float32frombits(sign)
		}
		// Subnormal
		e := uint32(127 - 14)
		for (mant & 0x0400) == 0 {
			mant <<= 1
			e--
		}
		mant &= 0x03FF
		return math.Float32frombits(sign | (e << 23) | (mant << 13))
	case 31:
		if mant == 0 {
			return math.Float32frombits(sign | 0x7F800000)
		}
		return math.Float32frombits(sign | 0x7FC00000 | (mant << 13))
	default:
		return math.Float32frombits(sign | ((exp + (127 - 15)) << 23) | (mant << 13))
	}
}

func encodeUnsignedFloat(f float32, mantBits uint) uint32 {
	if math.IsNaN(float64(f)) || f <= 0 {
		return 0
	}
	bits := math.Float32bits(f)
	exp := int((bits>>23)&0xFF) - 127 + 15
	mant := bits & 0x007FFFFF
	maxVal := uint32((30 << mantBits) | ((1 << mantBits) - 1))
	if exp <= 0 {
		if exp < -int(mantBits) {
			return 0
		}
		mant |= 0x00800000
		shift := uint(24-mantBits) - uint(exp)
		return mant >> shift
	}
	if exp >= 31 {
		return maxVal
	}
	shift := uint(23 - mantBits)
	out := uint32(exp<<mantBits) | (mant >> shift)
	if shift > 0 && (mant&(1<<(shift-1))) != 0 {
		if out < maxVal {
			out++
		}
	}
	return out
}

func decodeUnsignedFloat(v uint32, mantBits uint) float32 {
	exp := (v >> mantBits) & 0x1F
	mant := v & ((1 << mantBits) - 1)
	if exp == 0 {
		if mant == 0 {
			return 0
		}
		return float32(mant) * float32(math.Ldexp(1, -14-int(mantBits)))
	}
	if exp == 31 {
		if mant == 0 {
			return float32(math.Inf(1))
		}
		return float32(math.NaN())
	}
	return float32(math.Ldexp(1.0+float64(mant)/float64(uint32(1)<<mantBits), int(exp)-15))
}

func packRG11B10F(r, g, b float32) uint32 {
	ru := encodeUnsignedFloat(r, 6) & 0x7FF
	gu := encodeUnsignedFloat(g, 6) & 0x7FF
	bu := encodeUnsignedFloat(b, 5) & 0x3FF
	return ru | (gu << 11) | (bu << 22)
}

func unpackRG11B10F(v uint32) (r, g, b float32) {
	r = decodeUnsignedFloat(v&0x7FF, 6)
	g = decodeUnsignedFloat((v>>11)&0x7FF, 6)
	b = decodeUnsignedFloat((v>>22)&0x3FF, 5)
	return
}

func quantizeColorForFormat(format GPUTextureFormat, c [4]float32) [4]float32 {
	switch format {
	case GPUFormatRGBA8UnormSRGB, GPUFormatBGRA8UnormSRGB:
		return [4]float32{
			srgbToLinear(float32(floatToByte(linearToSRGB(c[0]))) / 255.0),
			srgbToLinear(float32(floatToByte(linearToSRGB(c[1]))) / 255.0),
			srgbToLinear(float32(floatToByte(linearToSRGB(c[2]))) / 255.0),
			float32(floatToByte(c[3])) / 255.0,
		}
	case GPUFormatR8Unorm:
		return [4]float32{float32(floatToByte(c[0])) / 255.0, 0, 0, 1}
	case GPUFormatRG8Unorm:
		return [4]float32{float32(floatToByte(c[0])) / 255.0, float32(floatToByte(c[1])) / 255.0, 0, 1}
	case GPUFormatRGBA16Float:
		return [4]float32{
			float16ToFloat32(float32ToFloat16(c[0])),
			float16ToFloat32(float32ToFloat16(c[1])),
			float16ToFloat32(float32ToFloat16(c[2])),
			float16ToFloat32(float32ToFloat16(c[3])),
		}
	case GPUFormatRG11B10Float:
		r, g, b := unpackRG11B10F(packRG11B10F(c[0], c[1], c[2]))
		return [4]float32{r, g, b, 1}
	case GPUFormatR32Float:
		return [4]float32{c[0], 0, 0, 1}
	case GPUFormatRGBA32Float:
		return c
	case GPUFormatRGB10A2Unorm:
		q10 := func(v float32) float32 {
			if v <= 0 {
				return 0
			}
			if v >= 1 {
				return 1
			}
			return float32(uint32(math.Round(float64(v)*1023))) / 1023.0
		}
		q2 := func(v float32) float32 {
			if v <= 0 {
				return 0
			}
			if v >= 1 {
				return 1
			}
			return float32(uint32(math.Round(float64(v)*3))) / 3.0
		}
		return [4]float32{q10(c[0]), q10(c[1]), q10(c[2]), q2(c[3])}
	default: // RGBA8Unorm, BGRA8Unorm
		return [4]float32{
			float32(floatToByte(c[0])) / 255.0,
			float32(floatToByte(c[1])) / 255.0,
			float32(floatToByte(c[2])) / 255.0,
			float32(floatToByte(c[3])) / 255.0,
		}
	}
}

func decodePixelBytes(src []byte, format GPUTextureFormat) [4]float32 {
	switch format {
	case GPUFormatRGBA8UnormSRGB:
		return [4]float32{
			srgbToLinear(float32(src[0]) / 255.0),
			srgbToLinear(float32(src[1]) / 255.0),
			srgbToLinear(float32(src[2]) / 255.0),
			float32(src[3]) / 255.0,
		}
	case GPUFormatBGRA8Unorm:
		return [4]float32{
			float32(src[2]) / 255.0,
			float32(src[1]) / 255.0,
			float32(src[0]) / 255.0,
			float32(src[3]) / 255.0,
		}
	case GPUFormatBGRA8UnormSRGB:
		return [4]float32{
			srgbToLinear(float32(src[2]) / 255.0),
			srgbToLinear(float32(src[1]) / 255.0),
			srgbToLinear(float32(src[0]) / 255.0),
			float32(src[3]) / 255.0,
		}
	case GPUFormatR8Unorm:
		return [4]float32{float32(src[0]) / 255.0, 0, 0, 1}
	case GPUFormatRG8Unorm:
		return [4]float32{float32(src[0]) / 255.0, float32(src[1]) / 255.0, 0, 1}
	case GPUFormatRGBA16Float:
		return [4]float32{
			float16ToFloat32(binary.LittleEndian.Uint16(src[0:2])),
			float16ToFloat32(binary.LittleEndian.Uint16(src[2:4])),
			float16ToFloat32(binary.LittleEndian.Uint16(src[4:6])),
			float16ToFloat32(binary.LittleEndian.Uint16(src[6:8])),
		}
	case GPUFormatRG11B10Float:
		r, g, b := unpackRG11B10F(binary.LittleEndian.Uint32(src[0:4]))
		return [4]float32{r, g, b, 1}
	case GPUFormatR32Float:
		return [4]float32{math.Float32frombits(binary.LittleEndian.Uint32(src[0:4])), 0, 0, 1}
	case GPUFormatRGBA32Float:
		return [4]float32{
			math.Float32frombits(binary.LittleEndian.Uint32(src[0:4])),
			math.Float32frombits(binary.LittleEndian.Uint32(src[4:8])),
			math.Float32frombits(binary.LittleEndian.Uint32(src[8:12])),
			math.Float32frombits(binary.LittleEndian.Uint32(src[12:16])),
		}
	case GPUFormatRGB10A2Unorm:
		v := binary.LittleEndian.Uint32(src[0:4])
		return [4]float32{
			float32(v&0x3ff) / 1023.0,
			float32((v>>10)&0x3ff) / 1023.0,
			float32((v>>20)&0x3ff) / 1023.0,
			float32((v>>30)&0x3) / 3.0,
		}
	default:
		return [4]float32{
			float32(src[0]) / 255.0,
			float32(src[1]) / 255.0,
			float32(src[2]) / 255.0,
			float32(src[3]) / 255.0,
		}
	}
}

func readbackBytesPerPixel(format GPUTextureFormat) int {
	switch format {
	case GPUFormatRGBA16Float:
		return 8
	case GPUFormatRG11B10Float, GPUFormatR32Float:
		return 4
	case GPUFormatRGBA32Float:
		return 16
	default:
		return 4
	}
}

func encodePixelBytes(dst []byte, format GPUTextureFormat, c [4]float32) {
	switch format {
	case GPUFormatRGBA8UnormSRGB, GPUFormatBGRA8UnormSRGB:
		dst[0] = floatToByte(linearToSRGB(c[0]))
		dst[1] = floatToByte(linearToSRGB(c[1]))
		dst[2] = floatToByte(linearToSRGB(c[2]))
		dst[3] = floatToByte(c[3])
	case GPUFormatRGBA16Float:
		binary.LittleEndian.PutUint16(dst[0:2], float32ToFloat16(c[0]))
		binary.LittleEndian.PutUint16(dst[2:4], float32ToFloat16(c[1]))
		binary.LittleEndian.PutUint16(dst[4:6], float32ToFloat16(c[2]))
		binary.LittleEndian.PutUint16(dst[6:8], float32ToFloat16(c[3]))
	case GPUFormatRG11B10Float:
		binary.LittleEndian.PutUint32(dst[0:4], packRG11B10F(c[0], c[1], c[2]))
	case GPUFormatR32Float:
		binary.LittleEndian.PutUint32(dst[0:4], math.Float32bits(c[0]))
	case GPUFormatRGBA32Float:
		binary.LittleEndian.PutUint32(dst[0:4], math.Float32bits(c[0]))
		binary.LittleEndian.PutUint32(dst[4:8], math.Float32bits(c[1]))
		binary.LittleEndian.PutUint32(dst[8:12], math.Float32bits(c[2]))
		binary.LittleEndian.PutUint32(dst[12:16], math.Float32bits(c[3]))
	case GPUFormatRGB10A2Unorm:
		u10 := func(v float32) uint32 {
			if v <= 0 {
				return 0
			}
			if v >= 1 {
				return 1023
			}
			return uint32(math.Round(float64(v) * 1023))
		}
		u2 := func(v float32) uint32 {
			if v <= 0 {
				return 0
			}
			if v >= 1 {
				return 3
			}
			return uint32(math.Round(float64(v) * 3))
		}
		packed := u10(c[0]) | (u10(c[1]) << 10) | (u10(c[2]) << 20) | (u2(c[3]) << 30)
		binary.LittleEndian.PutUint32(dst[0:4], packed)
	default:
		dst[0] = floatToByte(c[0])
		dst[1] = floatToByte(c[1])
		dst[2] = floatToByte(c[2])
		dst[3] = floatToByte(c[3])
	}
}

// --- Pure-Go shaderir Interpreter ---

type irVal struct {
	typ    shaderir.Type
	f      [16]float32
	i      [4]int32
	u      [4]uint32
	b      bool
	fields []irVal // for Struct and Array
}

func zeroIRVal(t shaderir.Type) irVal {
	v := irVal{typ: t}
	switch t.Main {
	case shaderir.Struct:
		v.fields = make([]irVal, len(t.Sub))
		for idx, st := range t.Sub {
			v.fields[idx] = zeroIRVal(st)
		}
	case shaderir.Array:
		v.fields = make([]irVal, t.Length)
		for idx := range t.Length {
			v.fields[idx] = zeroIRVal(t.Sub[0])
		}
	}
	return v
}

func cloneIRVal(v irVal) irVal {
	if len(v.fields) > 0 {
		f := make([]irVal, len(v.fields))
		for i := range v.fields {
			f[i] = cloneIRVal(v.fields[i])
		}
		v.fields = f
	}
	return v
}

func lerpIRVal(a, b, c irVal, wa, wb, wc float32) irVal {
	out := irVal{typ: a.typ}
	for i := 0; i < 4; i++ {
		out.f[i] = a.f[i]*wa + b.f[i]*wb + c.f[i]*wc
	}
	return out
}

func decodeVertexAttr(data []byte, format GPUVertexFormat) irVal {
	f32 := func(off int) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(data[off : off+4])) }
	u32 := func(off int) uint32 { return binary.LittleEndian.Uint32(data[off : off+4]) }
	i32 := func(off int) int32 { return int32(binary.LittleEndian.Uint32(data[off : off+4])) }
	switch format {
	case GPUVertexFloat32:
		return irVal{typ: shaderir.Type{Main: shaderir.Float}, f: [16]float32{f32(0)}}
	case GPUVertexFloat32x2:
		return irVal{typ: shaderir.Type{Main: shaderir.Vec2}, f: [16]float32{f32(0), f32(4)}}
	case GPUVertexFloat32x3:
		return irVal{typ: shaderir.Type{Main: shaderir.Vec3}, f: [16]float32{f32(0), f32(4), f32(8)}}
	case GPUVertexFloat32x4:
		return irVal{typ: shaderir.Type{Main: shaderir.Vec4}, f: [16]float32{f32(0), f32(4), f32(8), f32(12)}}
	case GPUVertexUnorm8x4:
		return irVal{typ: shaderir.Type{Main: shaderir.Vec4}, f: [16]float32{
			float32(data[0]) / 255.0, float32(data[1]) / 255.0, float32(data[2]) / 255.0, float32(data[3]) / 255.0,
		}}
	case GPUVertexUint32:
		return irVal{typ: shaderir.Type{Main: shaderir.Uint}, u: [4]uint32{u32(0)}}
	case GPUVertexUint32x2:
		return irVal{typ: shaderir.Type{Main: shaderir.UVec2}, u: [4]uint32{u32(0), u32(4)}}
	case GPUVertexUint32x3:
		return irVal{typ: shaderir.Type{Main: shaderir.UVec3}, u: [4]uint32{u32(0), u32(4), u32(8)}}
	case GPUVertexUint32x4:
		return irVal{typ: shaderir.Type{Main: shaderir.UVec4}, u: [4]uint32{u32(0), u32(4), u32(8), u32(12)}}
	case GPUVertexSint32:
		return irVal{typ: shaderir.Type{Main: shaderir.Int}, i: [4]int32{i32(0)}}
	case GPUVertexSint32x2:
		return irVal{typ: shaderir.Type{Main: shaderir.IVec2}, i: [4]int32{i32(0), i32(4)}}
	case GPUVertexSint32x3:
		return irVal{typ: shaderir.Type{Main: shaderir.IVec3}, i: [4]int32{i32(0), i32(4), i32(8)}}
	case GPUVertexSint32x4:
		return irVal{typ: shaderir.Type{Main: shaderir.IVec4}, i: [4]int32{i32(0), i32(4), i32(8), i32(12)}}
	}
	return irVal{}
}

func decodeUniformBytes(t shaderir.Type, data []byte, offset int) irVal {
	return decodeBytes(t, data, offset, false)
}

func decodeBytes(t shaderir.Type, data []byte, offset int, std430 bool) irVal {
	readF32 := func(off int) float32 {
		if off < 0 || off+4 > len(data) {
			return 0
		}
		return math.Float32frombits(binary.LittleEndian.Uint32(data[off : off+4]))
	}
	readU32 := func(off int) uint32 {
		if off < 0 || off+4 > len(data) {
			return 0
		}
		return binary.LittleEndian.Uint32(data[off : off+4])
	}
	v := irVal{typ: t}
	switch t.Main {
	case shaderir.Float:
		v.f[0] = readF32(offset)
	case shaderir.Int:
		v.i[0] = int32(readU32(offset))
		v.f[0] = float32(v.i[0])
	case shaderir.Uint:
		v.u[0] = readU32(offset)
		v.f[0] = float32(v.u[0])
	case shaderir.Bool:
		v.b = readU32(offset) != 0
	case shaderir.Vec2:
		v.f[0], v.f[1] = readF32(offset), readF32(offset+4)
	case shaderir.Vec3:
		v.f[0], v.f[1], v.f[2] = readF32(offset), readF32(offset+4), readF32(offset+8)
	case shaderir.Vec4:
		v.f[0], v.f[1], v.f[2], v.f[3] = readF32(offset), readF32(offset+4), readF32(offset+8), readF32(offset+12)
	case shaderir.IVec2, shaderir.IVec3, shaderir.IVec4:
		n := t.VectorElementCount()
		for k := 0; k < n; k++ {
			v.i[k] = int32(readU32(offset + k*4))
			v.f[k] = float32(v.i[k])
		}
	case shaderir.UVec2, shaderir.UVec3, shaderir.UVec4:
		n := t.VectorElementCount()
		for k := 0; k < n; k++ {
			v.u[k] = readU32(offset + k*4)
			v.f[k] = float32(v.u[k])
		}
	case shaderir.Mat2, shaderir.Mat3, shaderir.Mat4,
		shaderir.Mat2x3, shaderir.Mat2x4, shaderir.Mat3x2, shaderir.Mat3x4, shaderir.Mat4x2, shaderir.Mat4x3:
		cols, rows := t.MatrixCols(), t.MatrixRows()
		colStride := rows * 4
		if rows == 3 {
			colStride = 16 // WGSL/std140/std430 column alignment for vec3
		}
		for c := 0; c < cols; c++ {
			for r := 0; r < rows; r++ {
				v.f[c*rows+r] = readF32(offset + c*colStride + r*4)
			}
		}
	case shaderir.Struct:
		v.fields = make([]irVal, len(t.Sub))
		for i, ft := range t.Sub {
			foff := t.StructFieldOffset(i)
			if std430 {
				foff = t.StorageStructFieldOffset(i)
			}
			v.fields[i] = decodeBytes(ft, data, offset+foff, std430)
		}
	case shaderir.Array:
		v.fields = make([]irVal, t.Length)
		elem := t.Sub[0]
		var stride int
		if std430 {
			stride = elem.StorageStride()
		} else {
			a := max(elem.AlignBytes(), 16)
			stride = ((elem.SizeBytes() + a - 1) / a) * a
		}
		for i := 0; i < t.Length; i++ {
			v.fields[i] = decodeBytes(elem, data, offset+i*stride, std430)
		}
	}
	return v
}

func encodeBytes(t shaderir.Type, data []byte, offset int, v irVal, std430 bool) {
	writeF32 := func(off int, val float32) {
		if off < 0 || off+4 > len(data) {
			return
		}
		binary.LittleEndian.PutUint32(data[off:off+4], math.Float32bits(val))
	}
	writeU32 := func(off int, val uint32) {
		if off < 0 || off+4 > len(data) {
			return
		}
		binary.LittleEndian.PutUint32(data[off:off+4], val)
	}
	switch t.Main {
	case shaderir.Float:
		writeF32(offset, v.f[0])
	case shaderir.Int:
		writeU32(offset, uint32(v.i[0]))
	case shaderir.Uint:
		writeU32(offset, v.u[0])
	case shaderir.Bool:
		if v.b {
			writeU32(offset, 1)
		} else {
			writeU32(offset, 0)
		}
	case shaderir.Vec2:
		writeF32(offset, v.f[0])
		writeF32(offset+4, v.f[1])
	case shaderir.Vec3:
		writeF32(offset, v.f[0])
		writeF32(offset+4, v.f[1])
		writeF32(offset+8, v.f[2])
	case shaderir.Vec4:
		writeF32(offset, v.f[0])
		writeF32(offset+4, v.f[1])
		writeF32(offset+8, v.f[2])
		writeF32(offset+12, v.f[3])
	case shaderir.IVec2, shaderir.IVec3, shaderir.IVec4:
		n := t.VectorElementCount()
		for k := 0; k < n; k++ {
			writeU32(offset+k*4, uint32(v.i[k]))
		}
	case shaderir.UVec2, shaderir.UVec3, shaderir.UVec4:
		n := t.VectorElementCount()
		for k := 0; k < n; k++ {
			writeU32(offset+k*4, v.u[k])
		}
	case shaderir.Mat2, shaderir.Mat3, shaderir.Mat4,
		shaderir.Mat2x3, shaderir.Mat2x4, shaderir.Mat3x2, shaderir.Mat3x4, shaderir.Mat4x2, shaderir.Mat4x3:
		cols, rows := t.MatrixCols(), t.MatrixRows()
		colStride := rows * 4
		if rows == 3 {
			colStride = 16
		}
		for c := 0; c < cols; c++ {
			for r := 0; r < rows; r++ {
				writeF32(offset+c*colStride+r*4, v.f[c*rows+r])
			}
		}
	case shaderir.Struct:
		for i, ft := range t.Sub {
			if i >= len(v.fields) {
				break
			}
			foff := t.StructFieldOffset(i)
			if std430 {
				foff = t.StorageStructFieldOffset(i)
			}
			encodeBytes(ft, data, offset+foff, v.fields[i], std430)
		}
	case shaderir.Array:
		elem := t.Sub[0]
		var stride int
		if std430 {
			stride = elem.StorageStride()
		} else {
			a := max(elem.AlignBytes(), 16)
			stride = ((elem.SizeBytes() + a - 1) / a) * a
		}
		for i := 0; i < t.Length && i < len(v.fields); i++ {
			encodeBytes(elem, data, offset+i*stride, v.fields[i], std430)
		}
	}
}

type irVM struct {
	ctx           *softDrawCtx
	locals        []irVal
	vertID        uint32
	instID        uint32
	frontFacing   bool
	fragCoord     [4]float32
	globalID      [3]uint32
	localID       [3]uint32
	workgroupID   [3]uint32
	numWorkgroups [3]uint32
	localIdx      uint32
	returned      bool
	discarded     bool
	broken        bool
	continued     bool
	retVal        irVal
}

func (vm *irVM) execBlock(b *shaderir.Block) {
	if b == nil {
		return
	}
	for _, s := range b.Stmts {
		if vm.returned || vm.discarded || vm.broken || vm.continued {
			return
		}
		switch s.Type {
		case shaderir.ExprStmt:
			vm.evalExpr(s.Exprs[0])
		case shaderir.BlockStmt:
			vm.execBlock(s.Blocks[0])
		case shaderir.Init:
			if s.InitIndex >= 0 && s.InitIndex < len(vm.locals) {
				vm.locals[s.InitIndex] = zeroIRVal(vm.locals[s.InitIndex].typ)
			}
		case shaderir.Assign:
			val := vm.evalExpr(s.Exprs[1])
			vm.assignExpr(s.Exprs[0], cloneIRVal(val))
		case shaderir.If:
			cond := vm.evalExpr(s.Exprs[0])
			if cond.b {
				vm.execBlock(s.Blocks[0])
			} else if len(s.Blocks) > 1 {
				vm.execBlock(s.Blocks[1])
			}
		case shaderir.For:
			vm.execFor(s)
		case shaderir.Break:
			vm.broken = true
		case shaderir.Continue:
			vm.continued = true
		case shaderir.Return:
			if len(s.Exprs) > 0 {
				vm.retVal = vm.evalExpr(s.Exprs[0])
			}
			vm.returned = true
		case shaderir.Discard:
			vm.discarded = true
		}
	}
}

func (vm *irVM) execFor(s shaderir.Stmt) {
	var cur, end, delta int64
	if len(s.Exprs) >= 2 {
		v0 := vm.evalExpr(s.Exprs[0])
		v1 := vm.evalExpr(s.Exprs[1])
		if v0.typ.Main == shaderir.Uint {
			cur = int64(v0.u[0])
		} else {
			cur = int64(v0.i[0])
		}
		if v1.typ.Main == shaderir.Uint {
			end = int64(v1.u[0])
		} else {
			end = int64(v1.i[0])
		}
	} else {
		cur, _ = constant.Int64Val(s.ForInit)
		end, _ = constant.Int64Val(s.ForEnd)
	}
	delta, _ = constant.Int64Val(s.ForDelta)
	if delta == 0 {
		return
	}
	for iter := 0; iter < 4096; iter++ {
		var ok bool
		switch s.ForOp {
		case shaderir.LessThanOp:
			ok = cur < end
		case shaderir.LessThanEqualOp:
			ok = cur <= end
		case shaderir.GreaterThanOp:
			ok = cur > end
		case shaderir.GreaterThanEqualOp:
			ok = cur >= end
		default:
			ok = cur < end
		}
		if !ok {
			break
		}
		if s.ForVarIndex >= 0 && s.ForVarIndex < len(vm.locals) {
			vm.locals[s.ForVarIndex] = irVal{typ: s.ForVarType, i: [4]int32{int32(cur)}, u: [4]uint32{uint32(cur)}, f: [16]float32{float32(cur)}}
		}
		vm.execBlock(s.Blocks[0])
		if vm.returned || vm.discarded {
			return
		}
		if vm.broken {
			vm.broken = false
			return
		}
		vm.continued = false
		cur += delta
	}
}

type storageAddr struct {
	buf      *softBuffer
	offset   int
	boundEnd int
	typ      shaderir.Type
	swizzle  string
	writable bool
}

func (vm *irVM) resolveStorageAddr(e shaderir.Expr) (storageAddr, bool) {
	switch e.Type {
	case shaderir.BindingVariable:
		if e.Index < 0 || e.Index >= len(vm.ctx.prog.Bindings) {
			return storageAddr{}, false
		}
		b := vm.ctx.prog.Bindings[e.Index]
		if b.Kind != shaderir.BindingStorageBufferRead && b.Kind != shaderir.BindingStorageBufferReadWrite {
			return storageAddr{}, false
		}
		bound := vm.ctx.bindGroups[b.Group]
		if bound.bg == nil {
			return storageAddr{}, false
		}
		entry, ok := bound.bg.entries[b.Binding]
		if !ok {
			return storageAddr{}, false
		}
		buf := vm.ctx.dev.buffers[entry.Buffer]
		if buf == nil {
			return storageAddr{}, false
		}
		start := entry.Offset
		if bound.dynamicOffsets != nil {
			start += bound.dynamicOffsets[b.Binding]
		}
		boundEnd := len(buf.data)
		if entry.Size > 0 && int(start+entry.Size) < boundEnd {
			boundEnd = int(start + entry.Size)
		}
		return storageAddr{
			buf:      buf,
			offset:   int(start),
			boundEnd: boundEnd,
			typ:      b.Type,
			writable: b.Kind == shaderir.BindingStorageBufferReadWrite,
		}, true
	case shaderir.FieldSelector:
		addr, ok := vm.resolveStorageAddr(e.Exprs[0])
		if !ok {
			return storageAddr{}, false
		}
		switch m := e.Exprs[1]; m.Type {
		case shaderir.StructMember:
			if addr.typ.Main != shaderir.Struct || m.Index < 0 || m.Index >= len(addr.typ.Sub) {
				return storageAddr{}, false
			}
			addr.offset += addr.typ.StorageStructFieldOffset(m.Index)
			addr.typ = addr.typ.Sub[m.Index]
			addr.swizzle = ""
			return addr, true
		case shaderir.SwizzlingExpr:
			addr.swizzle = m.Swizzling
			return addr, true
		}
	case shaderir.Index:
		addr, ok := vm.resolveStorageAddr(e.Exprs[0])
		if !ok {
			return storageAddr{}, false
		}
		idxVal := vm.evalExpr(e.Exprs[1])
		idx := int(idxVal.i[0])
		if idxVal.typ.Main == shaderir.Uint {
			idx = int(idxVal.u[0])
		}
		if idx < 0 {
			return storageAddr{}, false
		}
		switch {
		case addr.typ.Main == shaderir.Array && len(addr.typ.Sub) > 0:
			elem := addr.typ.Sub[0]
			addr.offset += idx * elem.StorageStride()
			addr.typ = elem
			addr.swizzle = ""
			return addr, true
		case addr.typ.IsFloatVector() || addr.typ.IsIntegerVector():
			addr.offset += idx * 4
			addr.typ = shaderir.Type{Main: addr.typ.Scalar()}
			addr.swizzle = ""
			return addr, true
		case addr.typ.IsMatrix():
			rows := addr.typ.MatrixRows()
			colStride := rows * 4
			if rows == 3 {
				colStride = 16
			}
			addr.offset += idx * colStride
			addr.typ = shaderir.Type{Main: shaderir.VectorOf(shaderir.Float, rows)}
			addr.swizzle = ""
			return addr, true
		}
	}
	return storageAddr{}, false
}

func (vm *irVM) assignExpr(lhs shaderir.Expr, val irVal) {
	if addr, ok := vm.resolveStorageAddr(lhs); ok {
		if !addr.writable || addr.buf == nil || addr.boundEnd > len(addr.buf.data) {
			return
		}
		slice := addr.buf.data[:addr.boundEnd]
		if addr.swizzle != "" {
			cur := decodeBytes(addr.typ, slice, addr.offset, true)
			for i, ch := range addr.swizzle {
				ci := swizzleIdx(byte(ch))
				if ci >= 0 && ci < 4 {
					cur.f[ci] = val.f[i]
					cur.i[ci] = val.i[i]
					cur.u[ci] = val.u[i]
				}
			}
			encodeBytes(addr.typ, slice, addr.offset, cur, true)
			return
		}
		encodeBytes(addr.typ, slice, addr.offset, val, true)
		return
	}
	switch lhs.Type {
	case shaderir.LocalVariable:
		if lhs.Index >= 0 && lhs.Index < len(vm.locals) {
			vm.locals[lhs.Index] = val
		}
	case shaderir.FieldSelector:
		base := vm.evalPtr(lhs.Exprs[0])
		if base == nil {
			return
		}
		switch m := lhs.Exprs[1]; m.Type {
		case shaderir.StructMember:
			if m.Index >= 0 && m.Index < len(base.fields) {
				base.fields[m.Index] = val
			}
		case shaderir.SwizzlingExpr:
			for i, ch := range m.Swizzling {
				ci := swizzleIdx(byte(ch))
				if ci >= 0 && ci < 4 {
					base.f[ci] = val.f[i]
					base.i[ci] = val.i[i]
					base.u[ci] = val.u[i]
				}
			}
		}
	case shaderir.Index:
		base := vm.evalPtr(lhs.Exprs[0])
		if base == nil {
			return
		}
		idxVal := vm.evalExpr(lhs.Exprs[1])
		idx := int(idxVal.i[0])
		if idxVal.typ.Main == shaderir.Uint {
			idx = int(idxVal.u[0])
		}
		if base.typ.Main == shaderir.Array && idx >= 0 && idx < len(base.fields) {
			base.fields[idx] = val
		} else if idx >= 0 && idx < 4 {
			base.f[idx] = val.f[0]
			base.i[idx] = val.i[0]
			base.u[idx] = val.u[0]
		}
	}
}

func (vm *irVM) evalPtr(e shaderir.Expr) *irVal {
	switch e.Type {
	case shaderir.LocalVariable:
		if e.Index >= 0 && e.Index < len(vm.locals) {
			return &vm.locals[e.Index]
		}
	case shaderir.FieldSelector:
		base := vm.evalPtr(e.Exprs[0])
		if base != nil && e.Exprs[1].Type == shaderir.StructMember {
			idx := e.Exprs[1].Index
			if idx >= 0 && idx < len(base.fields) {
				return &base.fields[idx]
			}
		}
	case shaderir.Index:
		base := vm.evalPtr(e.Exprs[0])
		if base != nil && base.typ.Main == shaderir.Array {
			idx := int(vm.evalExpr(e.Exprs[1]).i[0])
			if idx >= 0 && idx < len(base.fields) {
				return &base.fields[idx]
			}
		}
	}
	return nil
}

func (vm *irVM) evalExpr(e shaderir.Expr) irVal {
	switch e.Type {
	case shaderir.NumberExpr:
		switch e.Const.Kind() {
		case constant.Bool:
			return irVal{typ: shaderir.Type{Main: shaderir.Bool}, b: constant.BoolVal(e.Const)}
		case constant.Int:
			if e.Unsigned {
				v, _ := constant.Uint64Val(e.Const)
				return irVal{typ: shaderir.Type{Main: shaderir.Uint}, u: [4]uint32{uint32(v)}, f: [16]float32{float32(v)}}
			}
			v, _ := constant.Int64Val(e.Const)
			return irVal{typ: shaderir.Type{Main: shaderir.Int}, i: [4]int32{int32(v)}, f: [16]float32{float32(v)}}
		case constant.Float:
			f, _ := constant.Float64Val(e.Const)
			return irVal{typ: shaderir.Type{Main: shaderir.Float}, f: [16]float32{float32(f)}}
		}
	case shaderir.LocalVariable:
		if e.Index >= 0 && e.Index < len(vm.locals) {
			return vm.locals[e.Index]
		}
	case shaderir.BindingVariable:
		return vm.evalBinding(e.Index)
	case shaderir.BuiltinInput:
		switch shaderir.StageBuiltin(e.Index) {
		case shaderir.BuiltinVertexIndex:
			return irVal{typ: shaderir.Type{Main: shaderir.Uint}, u: [4]uint32{vm.vertID}, f: [16]float32{float32(vm.vertID)}}
		case shaderir.BuiltinInstanceIndex:
			return irVal{typ: shaderir.Type{Main: shaderir.Uint}, u: [4]uint32{vm.instID}, f: [16]float32{float32(vm.instID)}}
		case shaderir.BuiltinFrontFacing:
			return irVal{typ: shaderir.Type{Main: shaderir.Bool}, b: vm.frontFacing}
		case shaderir.BuiltinPosition:
			return irVal{typ: shaderir.Type{Main: shaderir.Vec4}, f: [16]float32{vm.fragCoord[0], vm.fragCoord[1], vm.fragCoord[2], vm.fragCoord[3]}}
		case shaderir.BuiltinGlobalInvocationID:
			return irVal{
				typ: shaderir.Type{Main: shaderir.UVec3},
				u:   [4]uint32{vm.globalID[0], vm.globalID[1], vm.globalID[2]},
				f:   [16]float32{float32(vm.globalID[0]), float32(vm.globalID[1]), float32(vm.globalID[2])},
			}
		case shaderir.BuiltinLocalInvocationID:
			return irVal{
				typ: shaderir.Type{Main: shaderir.UVec3},
				u:   [4]uint32{vm.localID[0], vm.localID[1], vm.localID[2]},
				f:   [16]float32{float32(vm.localID[0]), float32(vm.localID[1]), float32(vm.localID[2])},
			}
		case shaderir.BuiltinWorkgroupID:
			return irVal{
				typ: shaderir.Type{Main: shaderir.UVec3},
				u:   [4]uint32{vm.workgroupID[0], vm.workgroupID[1], vm.workgroupID[2]},
				f:   [16]float32{float32(vm.workgroupID[0]), float32(vm.workgroupID[1]), float32(vm.workgroupID[2])},
			}
		case shaderir.BuiltinNumWorkgroups:
			return irVal{
				typ: shaderir.Type{Main: shaderir.UVec3},
				u:   [4]uint32{vm.numWorkgroups[0], vm.numWorkgroups[1], vm.numWorkgroups[2]},
				f:   [16]float32{float32(vm.numWorkgroups[0]), float32(vm.numWorkgroups[1]), float32(vm.numWorkgroups[2])},
			}
		case shaderir.BuiltinLocalInvocationIndex:
			return irVal{
				typ: shaderir.Type{Main: shaderir.Uint},
				u:   [4]uint32{vm.localIdx},
				f:   [16]float32{float32(vm.localIdx)},
			}
		}
	case shaderir.StructConstruct:
		v := zeroIRVal(*e.StructType)
		for i, a := range e.Exprs {
			if i < len(v.fields) {
				v.fields[i] = vm.evalExpr(a)
			}
		}
		return v
	case shaderir.Unary:
		x := vm.evalExpr(e.Exprs[0])
		switch e.Op {
		case shaderir.Sub:
			out := x
			for i := 0; i < 4; i++ {
				out.f[i] = -x.f[i]
				out.i[i] = -x.i[i]
			}
			return out
		case shaderir.NotOp:
			return irVal{typ: shaderir.Type{Main: shaderir.Bool}, b: !x.b}
		}
	case shaderir.Binary:
		return vm.evalBinary(e.Op, vm.evalExpr(e.Exprs[0]), vm.evalExpr(e.Exprs[1]))
	case shaderir.Selection:
		if vm.evalExpr(e.Exprs[0]).b {
			return vm.evalExpr(e.Exprs[1])
		}
		return vm.evalExpr(e.Exprs[2])
	case shaderir.FieldSelector:
		if addr, ok := vm.resolveStorageAddr(e); ok {
			v := decodeBytes(addr.typ, addr.buf.data[:addr.boundEnd], addr.offset, true)
			if addr.swizzle != "" {
				return evalSwizzle(v, addr.swizzle)
			}
			return v
		}
		base := vm.evalExpr(e.Exprs[0])
		switch m := e.Exprs[1]; m.Type {
		case shaderir.StructMember:
			if m.Index >= 0 && m.Index < len(base.fields) {
				return base.fields[m.Index]
			}
		case shaderir.SwizzlingExpr:
			return evalSwizzle(base, m.Swizzling)
		}
	case shaderir.Index:
		if addr, ok := vm.resolveStorageAddr(e); ok {
			return decodeBytes(addr.typ, addr.buf.data[:addr.boundEnd], addr.offset, true)
		}
		base := vm.evalExpr(e.Exprs[0])
		idxVal := vm.evalExpr(e.Exprs[1])
		idx := int(idxVal.i[0])
		if idxVal.typ.Main == shaderir.Uint {
			idx = int(idxVal.u[0])
		}
		switch {
		case base.typ.Main == shaderir.Array:
			if idx >= 0 && idx < len(base.fields) {
				return base.fields[idx]
			}
		case base.typ.IsMatrix():
			rows := base.typ.MatrixRows()
			out := irVal{typ: shaderir.Type{Main: shaderir.VectorOf(shaderir.Float, rows)}}
			for r := 0; r < rows; r++ {
				out.f[r] = base.f[idx*rows+r]
			}
			return out
		default:
			if idx >= 0 && idx < 4 {
				return irVal{typ: shaderir.Type{Main: base.typ.Scalar()}, f: [16]float32{base.f[idx]}, i: [4]int32{base.i[idx]}, u: [4]uint32{base.u[idx]}}
			}
		}
	case shaderir.Call:
		return vm.evalCall(e)
	}
	return irVal{}
}

func (vm *irVM) evalBinding(idx int) irVal {
	if idx < 0 || idx >= len(vm.ctx.prog.Bindings) {
		return irVal{}
	}
	if idx < len(vm.ctx.bindingCached) && vm.ctx.bindingCached[idx] {
		return vm.ctx.bindingCache[idx]
	}
	b := vm.ctx.prog.Bindings[idx]
	bound := vm.ctx.bindGroups[b.Group]
	if bound.bg == nil {
		return zeroIRVal(b.Type)
	}
	entry, ok := bound.bg.entries[b.Binding]
	if !ok {
		return zeroIRVal(b.Type)
	}
	var val irVal
	switch b.Kind {
	case shaderir.BindingUniformBuffer:
		buf := vm.ctx.dev.buffers[entry.Buffer]
		if buf == nil {
			val = zeroIRVal(b.Type)
		} else {
			effOffset := entry.Offset
			if bound.dynamicOffsets != nil {
				effOffset += bound.dynamicOffsets[b.Binding]
			}
			val = decodeUniformBytes(b.Type, buf.data, int(effOffset))
		}
	case shaderir.BindingStorageBufferRead, shaderir.BindingStorageBufferReadWrite:
		buf := vm.ctx.dev.buffers[entry.Buffer]
		switch {
		case buf == nil:
			val = zeroIRVal(b.Type)
		case b.Type.Main == shaderir.Array && b.Type.Length == 0:
			// Runtime-sized arrays are only accessed element-wise through resolveStorageAddr.
			val = irVal{typ: b.Type}
		default:
			effOffset := entry.Offset
			if bound.dynamicOffsets != nil {
				effOffset += bound.dynamicOffsets[b.Binding]
			}
			val = decodeBytes(b.Type, buf.data, int(effOffset), true)
		}
		if b.Kind == shaderir.BindingStorageBufferReadWrite {
			// Read/write storage changes while the shader runs, so it is never cached.
			return val
		}
	default:
		val = zeroIRVal(b.Type)
	}
	if idx < len(vm.ctx.bindingCached) {
		vm.ctx.bindingCache[idx] = val
		vm.ctx.bindingCached[idx] = true
	}
	return val
}

func (vm *irVM) evalBinary(op shaderir.Op, a, b irVal) irVal {
	if op == shaderir.MatrixMul {
		return evalMatMul(a, b)
	}
	switch op {
	case shaderir.AndAnd:
		return irVal{typ: shaderir.Type{Main: shaderir.Bool}, b: a.b && b.b}
	case shaderir.OrOr:
		return irVal{typ: shaderir.Type{Main: shaderir.Bool}, b: a.b || b.b}
	case shaderir.LessThanOp, shaderir.LessThanEqualOp, shaderir.GreaterThanOp,
		shaderir.GreaterThanEqualOp, shaderir.EqualOp, shaderir.NotEqualOp:
		return irVal{typ: shaderir.Type{Main: shaderir.Bool}, b: compareValues(op, a, b)}
	case shaderir.VectorEqualOp, shaderir.VectorNotEqualOp:
		// The backends write these as all(a == b) and !all(a == b): one bool for the whole vector.
		return irVal{typ: shaderir.Type{Main: shaderir.Bool}, b: vectorsEqual(a, b) == (op == shaderir.VectorEqualOp)}
	}
	// Component-wise float/int/uint arithmetic with scalar broadcast.
	resT := a.typ
	n := max(a.typ.VectorElementCount(), b.typ.VectorElementCount())
	if n < 1 {
		n = 1
	}
	if b.typ.VectorElementCount() > a.typ.VectorElementCount() {
		resT = b.typ
	}
	out := irVal{typ: resT}
	aScalar := a.typ.VectorElementCount() < 2
	bScalar := b.typ.VectorElementCount() < 2
	for k := 0; k < n; k++ {
		ai, bi := k, k
		if aScalar {
			ai = 0
		}
		if bScalar {
			bi = 0
		}
		af, bf := a.f[ai], b.f[bi]
		if a.typ.Scalar() == shaderir.Int {
			iv, jv := a.i[ai], b.i[bi]
			switch op {
			case shaderir.Add:
				out.i[k] = iv + jv
			case shaderir.Sub:
				out.i[k] = iv - jv
			case shaderir.ComponentWiseMul:
				out.i[k] = iv * jv
			case shaderir.Div:
				if jv != 0 {
					out.i[k] = iv / jv
				}
			case shaderir.ModOp:
				if jv != 0 {
					out.i[k] = iv % jv
				}
			case shaderir.And:
				out.i[k] = iv & jv
			case shaderir.Or:
				out.i[k] = iv | jv
			case shaderir.Xor:
				out.i[k] = iv ^ jv
			case shaderir.LeftShift:
				out.i[k] = iv << uint(jv&31)
			case shaderir.RightShift:
				out.i[k] = iv >> uint(jv&31)
			}
			out.f[k] = float32(out.i[k])
			continue
		}
		if a.typ.Scalar() == shaderir.Uint {
			uv, wv := a.u[ai], b.u[bi]
			switch op {
			case shaderir.Add:
				out.u[k] = uv + wv
			case shaderir.Sub:
				out.u[k] = uv - wv
			case shaderir.ComponentWiseMul:
				out.u[k] = uv * wv
			case shaderir.Div:
				if wv != 0 {
					out.u[k] = uv / wv
				}
			case shaderir.ModOp:
				if wv != 0 {
					out.u[k] = uv % wv
				}
			case shaderir.And:
				out.u[k] = uv & wv
			case shaderir.Or:
				out.u[k] = uv | wv
			case shaderir.Xor:
				out.u[k] = uv ^ wv
			case shaderir.LeftShift:
				out.u[k] = uv << (wv & 31)
			case shaderir.RightShift:
				out.u[k] = uv >> (wv & 31)
			}
			out.f[k] = float32(out.u[k])
			continue
		}
		switch op {
		case shaderir.Add:
			out.f[k] = af + bf
		case shaderir.Sub:
			out.f[k] = af - bf
		case shaderir.ComponentWiseMul:
			out.f[k] = af * bf
		case shaderir.Div:
			if bf != 0 {
				out.f[k] = af / bf
			}
		}
	}
	return out
}

// vectorsEqual reports whether every lane of a equals the same lane of b, which VectorEqualOp means for the whole vector.
func vectorsEqual(a, b irVal) bool {
	n := max(a.typ.VectorElementCount(), b.typ.VectorElementCount())
	if n < 1 {
		n = 1
	}
	for i := 0; i < n; i++ {
		switch a.typ.Scalar() {
		case shaderir.Int:
			if a.i[i] != b.i[i] {
				return false
			}
		case shaderir.Uint:
			if a.u[i] != b.u[i] {
				return false
			}
		case shaderir.Bool:
			if a.i[i] != b.i[i] {
				return false
			}
		default:
			if a.f[i] != b.f[i] {
				return false
			}
		}
	}
	return true
}

// compareValues evaluates a comparison of the operands' first element. A float comparison with a NaN is unordered, so every ordered comparison is false and only != is true, as IEEE 754 and WGSL require; a bool compares its value, which a three-way compare never read.
func compareValues(op shaderir.Op, a, b irVal) bool {
	switch a.typ.Scalar() {
	case shaderir.Bool:
		switch op {
		case shaderir.EqualOp:
			return a.b == b.b
		case shaderir.NotEqualOp:
			return a.b != b.b
		}
		return false
	case shaderir.Int:
		x, y := a.i[0], b.i[0]
		switch op {
		case shaderir.LessThanOp:
			return x < y
		case shaderir.LessThanEqualOp:
			return x <= y
		case shaderir.GreaterThanOp:
			return x > y
		case shaderir.GreaterThanEqualOp:
			return x >= y
		case shaderir.EqualOp:
			return x == y
		case shaderir.NotEqualOp:
			return x != y
		}
		return false
	case shaderir.Uint:
		x, y := a.u[0], b.u[0]
		switch op {
		case shaderir.LessThanOp:
			return x < y
		case shaderir.LessThanEqualOp:
			return x <= y
		case shaderir.GreaterThanOp:
			return x > y
		case shaderir.GreaterThanEqualOp:
			return x >= y
		case shaderir.EqualOp:
			return x == y
		case shaderir.NotEqualOp:
			return x != y
		}
		return false
	}
	x, y := a.f[0], b.f[0]
	switch op {
	case shaderir.LessThanOp:
		return x < y
	case shaderir.LessThanEqualOp:
		return x <= y
	case shaderir.GreaterThanOp:
		return x > y
	case shaderir.GreaterThanEqualOp:
		return x >= y
	case shaderir.EqualOp:
		return x == y
	case shaderir.NotEqualOp:
		return x != y
	}
	return false
}

func evalMatMul(a, b irVal) irVal {
	// Column-major matrices: M[col*rows + row]
	switch {
	case a.typ.IsMatrix() && b.typ.IsFloatVector():
		cols, rows := a.typ.MatrixCols(), a.typ.MatrixRows()
		out := irVal{typ: shaderir.Type{Main: shaderir.VectorOf(shaderir.Float, rows)}}
		for r := 0; r < rows; r++ {
			var sum float32
			for c := 0; c < cols; c++ {
				sum += a.f[c*rows+r] * b.f[c]
			}
			out.f[r] = sum
		}
		return out
	case a.typ.IsFloatVector() && b.typ.IsMatrix():
		cols, rows := b.typ.MatrixCols(), b.typ.MatrixRows()
		out := irVal{typ: shaderir.Type{Main: shaderir.VectorOf(shaderir.Float, cols)}}
		for c := 0; c < cols; c++ {
			var sum float32
			for r := 0; r < rows; r++ {
				sum += a.f[r] * b.f[c*rows+r]
			}
			out.f[c] = sum
		}
		return out
	case a.typ.IsMatrix() && b.typ.IsMatrix():
		ac, ar := a.typ.MatrixCols(), a.typ.MatrixRows()
		bc, br := b.typ.MatrixCols(), b.typ.MatrixRows()
		out := irVal{typ: shaderir.Type{Main: shaderir.MatrixOf(bc, ar)}}
		for c := 0; c < bc; c++ {
			for r := 0; r < ar; r++ {
				var sum float32
				for k := 0; k < ac && k < br; k++ {
					sum += a.f[k*ar+r] * b.f[c*br+k]
				}
				out.f[c*ar+r] = sum
			}
		}
		return out
	case a.typ.IsMatrix():
		out := a
		n := a.typ.MatrixCols() * a.typ.MatrixRows()
		for i := 0; i < n; i++ {
			out.f[i] *= b.f[0]
		}
		return out
	case b.typ.IsMatrix():
		out := b
		n := b.typ.MatrixCols() * b.typ.MatrixRows()
		for i := 0; i < n; i++ {
			out.f[i] *= a.f[0]
		}
		return out
	}
	return irVal{}
}

// softBuiltins are the builtin calls the interpreter evaluates. A pipeline that calls another one is refused when it is created, rather than drawing a zero where the call is (#2334). Derivatives are here because the rasterizer draws one fragment at a time and returns zero for them.
var softBuiltins = map[shaderir.BuiltinFunc]bool{
	// Constructors and casts.
	shaderir.FloatF: true, shaderir.IntF: true, shaderir.UintF: true,
	shaderir.Vec2F: true, shaderir.Vec3F: true, shaderir.Vec4F: true,
	shaderir.IVec2F: true, shaderir.IVec3F: true, shaderir.IVec4F: true,
	shaderir.UVec2F: true, shaderir.UVec3F: true, shaderir.UVec4F: true,
	shaderir.Mat2F: true, shaderir.Mat3F: true, shaderir.Mat4F: true,
	shaderir.Mat2x3F: true, shaderir.Mat2x4F: true, shaderir.Mat3x2F: true,
	shaderir.Mat3x4F: true, shaderir.Mat4x2F: true, shaderir.Mat4x3F: true,
	shaderir.FloatBitsToInt: true, shaderir.FloatBitsToUint: true,
	shaderir.IntBitsToFloat: true, shaderir.UintBitsToFloat: true,
	// Arithmetic.
	shaderir.Sin: true, shaderir.Cos: true, shaderir.Tan: true,
	shaderir.Asin: true, shaderir.Acos: true, shaderir.Atan: true, shaderir.Atan2: true,
	shaderir.Pow: true, shaderir.Exp: true, shaderir.Exp2: true, shaderir.Log: true, shaderir.Log2: true,
	shaderir.Sinh: true, shaderir.Cosh: true, shaderir.Tanh: true,
	shaderir.Sqrt: true, shaderir.Inversesqrt: true, shaderir.Abs: true, shaderir.Sign: true,
	shaderir.Floor: true, shaderir.Ceil: true, shaderir.Fract: true, shaderir.Trunc: true, shaderir.RoundEven: true,
	shaderir.Mod: true, shaderir.Radians: true, shaderir.Degrees: true,
	shaderir.Min: true, shaderir.Max: true, shaderir.Clamp: true, shaderir.Mix: true,
	shaderir.Step: true, shaderir.Smoothstep: true,
	// Geometry.
	shaderir.Length: true, shaderir.Distance: true, shaderir.Dot: true, shaderir.Cross: true,
	shaderir.Normalize: true, shaderir.Faceforward: true, shaderir.Reflect: true, shaderir.Refract: true,
	shaderir.Transpose: true, shaderir.Determinant: true,
	// Derivatives, zero on the software rasterizer.
	shaderir.Dfdx: true, shaderir.Dfdy: true, shaderir.Fwidth: true,
	// Textures, arrays and barriers.
	shaderir.TextureSample: true, shaderir.TextureSampleLevel: true, shaderir.TextureSampleBias: true,
	shaderir.TextureSampleGrad: true, shaderir.TextureSampleCompare: true, shaderir.TextureSampleCompareLevel: true,
	shaderir.TextureLoad: true, shaderir.TextureDimensions: true, shaderir.TextureStore: true,
	shaderir.ArrayLength: true, shaderir.WorkgroupBarrier: true, shaderir.StorageBarrier: true,
}

// unsupportedSoftBuiltin returns the first builtin a program calls that the interpreter does not evaluate, or "".
func unsupportedSoftBuiltin(p *shaderir.Program) shaderir.BuiltinFunc {
	var visit func(e shaderir.Expr) (shaderir.BuiltinFunc, bool)
	visit = func(e shaderir.Expr) (shaderir.BuiltinFunc, bool) {
		if e.Type == shaderir.BuiltinFuncExpr && e.BuiltinFunc != "" && !softBuiltins[e.BuiltinFunc] {
			return e.BuiltinFunc, true
		}
		for _, sub := range e.Exprs {
			if b, ok := visit(sub); ok {
				return b, true
			}
		}
		return "", false
	}
	var visitBlock func(b *shaderir.Block) (shaderir.BuiltinFunc, bool)
	visitBlock = func(b *shaderir.Block) (shaderir.BuiltinFunc, bool) {
		if b == nil {
			return "", false
		}
		for _, s := range b.Stmts {
			for _, e := range s.Exprs {
				if bi, ok := visit(e); ok {
					return bi, true
				}
			}
			for _, sub := range s.Blocks {
				if bi, ok := visitBlock(sub); ok {
					return bi, true
				}
			}
		}
		return "", false
	}
	if p == nil {
		return ""
	}
	if b, ok := visitBlock(p.VertexFunc.Block); ok {
		return b
	}
	if b, ok := visitBlock(p.FragmentFunc.Block); ok {
		return b
	}
	if b, ok := visitBlock(p.ComputeFunc.Block); ok {
		return b
	}
	for i := range p.Funcs {
		if b, ok := visitBlock(p.Funcs[i].Block); ok {
			return b
		}
	}
	return ""
}

func (vm *irVM) evalCall(e shaderir.Expr) irVal {
	callee := e.Exprs[0]
	if callee.Type == shaderir.FunctionExpr {
		var fn *shaderir.Func
		for i := range vm.ctx.prog.Funcs {
			if vm.ctx.prog.Funcs[i].Index == callee.Index {
				fn = &vm.ctx.prog.Funcs[i]
				break
			}
		}
		if fn == nil {
			return irVal{}
		}
		locals := make([]irVal, len(fn.InParams)+len(fn.Block.LocalVars))
		for i, a := range e.Exprs[1:] {
			if i < len(fn.InParams) {
				locals[i] = vm.evalExpr(a)
			}
		}
		for i, lt := range fn.Block.LocalVars {
			locals[len(fn.InParams)+i] = zeroIRVal(lt)
		}
		sub := &irVM{
			ctx:           vm.ctx,
			locals:        locals,
			vertID:        vm.vertID,
			instID:        vm.instID,
			frontFacing:   vm.frontFacing,
			fragCoord:     vm.fragCoord,
			globalID:      vm.globalID,
			localID:       vm.localID,
			workgroupID:   vm.workgroupID,
			numWorkgroups: vm.numWorkgroups,
			localIdx:      vm.localIdx,
		}
		sub.execBlock(fn.Block)
		if sub.discarded {
			vm.discarded = true
		}
		return sub.retVal
	}

	// Resource/compute builtins needing raw binding/storage expressions before evaluating args.
	switch callee.BuiltinFunc {
	case shaderir.TextureSample, shaderir.TextureSampleLevel, shaderir.TextureSampleBias, shaderir.TextureSampleGrad:
		return vm.evalTextureSampleCall(e, callee.BuiltinFunc)
	case shaderir.TextureSampleCompare, shaderir.TextureSampleCompareLevel:
		var offX, offY int
		if len(e.Exprs) >= 6 {
			off := vm.evalExpr(e.Exprs[5])
			offX, offY = int(off.i[0]), int(off.i[1])
		}
		return vm.evalTextureSampleCompare(e.Exprs[1].Index, e.Exprs[2].Index, vm.evalExpr(e.Exprs[3]), vm.evalExpr(e.Exprs[4]).f[0], offX, offY)
	case shaderir.TextureLoad:
		var layer, extra irVal
		if base := vm.lookupTexture(e.Exprs[1].Index); base != nil && base.desc.Dimension == GPUDim2DArray {
			if len(e.Exprs) > 3 {
				layer = vm.evalExpr(e.Exprs[3])
			}
			if len(e.Exprs) > 4 {
				extra = vm.evalExpr(e.Exprs[4])
			}
		} else if len(e.Exprs) > 3 {
			extra = vm.evalExpr(e.Exprs[3])
		}
		return vm.evalTextureLoad(e.Exprs[1].Index, vm.evalExpr(e.Exprs[2]), layer, extra)
	case shaderir.TextureDimensions:
		var lvl irVal
		if len(e.Exprs) > 2 {
			lvl = vm.evalExpr(e.Exprs[2])
		}
		return vm.evalTextureDimensions(e.Exprs[1].Index, lvl)
	case shaderir.TextureStore:
		vm.evalTextureStore(e.Exprs[1].Index, vm.evalExpr(e.Exprs[2]), vm.evalExpr(e.Exprs[3]))
		return irVal{}
	case shaderir.ArrayLength:
		if len(e.Exprs) > 1 {
			if addr, ok := vm.resolveStorageAddr(e.Exprs[1]); ok && addr.typ.Main == shaderir.Array && len(addr.typ.Sub) > 0 {
				stride := addr.typ.Sub[0].StorageStride()
				if stride > 0 && addr.boundEnd >= addr.offset {
					cnt := uint32((addr.boundEnd - addr.offset) / stride)
					return irVal{typ: shaderir.Type{Main: shaderir.Uint}, u: [4]uint32{cnt}, f: [16]float32{float32(cnt)}}
				}
			}
		}
		return irVal{typ: shaderir.Type{Main: shaderir.Uint}}
	case shaderir.WorkgroupBarrier, shaderir.StorageBarrier:
		return irVal{}
	}

	var argBuf [8]irVal
	var args []irVal
	if n := len(e.Exprs) - 1; n <= len(argBuf) {
		args = argBuf[:n]
	} else {
		args = make([]irVal, n)
	}
	for i, a := range e.Exprs[1:] {
		args[i] = vm.evalExpr(a)
	}

	switch callee.BuiltinFunc {
	case shaderir.FloatF:
		if args[0].typ.Main == shaderir.Int {
			return irVal{typ: shaderir.Type{Main: shaderir.Float}, f: [16]float32{float32(args[0].i[0])}}
		}
		if args[0].typ.Main == shaderir.Uint {
			return irVal{typ: shaderir.Type{Main: shaderir.Float}, f: [16]float32{float32(args[0].u[0])}}
		}
		return irVal{typ: shaderir.Type{Main: shaderir.Float}, f: [16]float32{args[0].f[0]}}
	case shaderir.IntF:
		var v int32
		switch args[0].typ.Scalar() {
		case shaderir.Int:
			v = args[0].i[0]
		case shaderir.Uint:
			v = int32(args[0].u[0])
		default:
			v = int32(args[0].f[0])
		}
		return irVal{typ: shaderir.Type{Main: shaderir.Int}, i: [4]int32{v}, f: [16]float32{float32(v)}}
	case shaderir.UintF:
		var v uint32
		switch args[0].typ.Scalar() {
		case shaderir.Uint:
			v = args[0].u[0]
		case shaderir.Int:
			v = uint32(args[0].i[0])
		default:
			v = uint32(args[0].f[0])
		}
		return irVal{typ: shaderir.Type{Main: shaderir.Uint}, u: [4]uint32{v}, f: [16]float32{float32(v)}}
	case shaderir.Vec2F:
		return packFloatVec(shaderir.Vec2, 2, args)
	case shaderir.Vec3F:
		return packFloatVec(shaderir.Vec3, 3, args)
	case shaderir.Vec4F:
		return packFloatVec(shaderir.Vec4, 4, args)
	case shaderir.IVec2F:
		return packIntVec(shaderir.IVec2, 2, args)
	case shaderir.IVec3F:
		return packIntVec(shaderir.IVec3, 3, args)
	case shaderir.IVec4F:
		return packIntVec(shaderir.IVec4, 4, args)
	case shaderir.UVec2F:
		return packUintVec(shaderir.UVec2, 2, args)
	case shaderir.UVec3F:
		return packUintVec(shaderir.UVec3, 3, args)
	case shaderir.UVec4F:
		return packUintVec(shaderir.UVec4, 4, args)
	case shaderir.Mat3F:
		return packMat(shaderir.Mat3, 3, args)
	case shaderir.Mat4F:
		return packMat(shaderir.Mat4, 4, args)
	case shaderir.Mat2F:
		return packMat(shaderir.Mat2, 2, args)
	case shaderir.Mat2x3F:
		return packMatrix(shaderir.Mat2x3, 2, 3, args)
	case shaderir.Mat2x4F:
		return packMatrix(shaderir.Mat2x4, 2, 4, args)
	case shaderir.Mat3x2F:
		return packMatrix(shaderir.Mat3x2, 3, 2, args)
	case shaderir.Mat3x4F:
		return packMatrix(shaderir.Mat3x4, 3, 4, args)
	case shaderir.Mat4x2F:
		return packMatrix(shaderir.Mat4x2, 4, 2, args)
	case shaderir.Mat4x3F:
		return packMatrix(shaderir.Mat4x3, 4, 3, args)
	case shaderir.Transpose:
		src := args[0]
		cols, rows := src.typ.MatrixCols(), src.typ.MatrixRows()
		out := irVal{typ: shaderir.Type{Main: shaderir.MatrixOf(rows, cols)}}
		for c := 0; c < cols; c++ {
			for r := 0; r < rows; r++ {
				out.f[r*cols+c] = src.f[c*rows+r]
			}
		}
		return out
	case shaderir.Determinant:
		m := args[0]
		switch m.typ.MatrixCols() {
		case 2:
			return irVal{typ: shaderir.Type{Main: shaderir.Float}, f: [16]float32{m.f[0]*m.f[3] - m.f[2]*m.f[1]}}
		case 3:
			return irVal{typ: shaderir.Type{Main: shaderir.Float}, f: [16]float32{determinant3(m.f[:])}}
		case 4:
			return irVal{typ: shaderir.Type{Main: shaderir.Float}, f: [16]float32{determinant4(m.f[:])}}
		}
		return irVal{typ: shaderir.Type{Main: shaderir.Float}}
	case shaderir.Dot:
		n := max(args[0].typ.VectorElementCount(), 1)
		var sum float32
		for i := 0; i < n; i++ {
			sum += args[0].f[i] * args[1].f[i]
		}
		return irVal{typ: shaderir.Type{Main: shaderir.Float}, f: [16]float32{sum}}
	case shaderir.Cross:
		a, b := args[0].f, args[1].f
		return irVal{typ: shaderir.Type{Main: shaderir.Vec3}, f: [16]float32{
			a[1]*b[2] - a[2]*b[1],
			a[2]*b[0] - a[0]*b[2],
			a[0]*b[1] - a[1]*b[0],
		}}
	case shaderir.Length:
		n := max(args[0].typ.VectorElementCount(), 1)
		var sum float32
		for i := 0; i < n; i++ {
			sum += args[0].f[i] * args[0].f[i]
		}
		return irVal{typ: shaderir.Type{Main: shaderir.Float}, f: [16]float32{float32(math.Sqrt(float64(sum)))}}
	case shaderir.Normalize:
		n := max(args[0].typ.VectorElementCount(), 1)
		var sum float32
		for i := 0; i < n; i++ {
			sum += args[0].f[i] * args[0].f[i]
		}
		inv := float32(0)
		if sum > 1e-12 {
			inv = float32(1.0 / math.Sqrt(float64(sum)))
		}
		out := irVal{typ: args[0].typ}
		for i := 0; i < n; i++ {
			out.f[i] = args[0].f[i] * inv
		}
		return out
	case shaderir.Reflect:
		n := max(args[0].typ.VectorElementCount(), 1)
		var d float32
		for i := 0; i < n; i++ {
			d += args[0].f[i] * args[1].f[i]
		}
		out := irVal{typ: args[0].typ}
		for i := 0; i < n; i++ {
			out.f[i] = args[0].f[i] - 2*d*args[1].f[i]
		}
		return out
	case shaderir.Distance:
		n := max(args[0].typ.VectorElementCount(), 1)
		var sum float32
		for i := 0; i < n; i++ {
			d := args[0].f[i] - args[1].f[i]
			sum += d * d
		}
		return irVal{typ: shaderir.Type{Main: shaderir.Float}, f: [16]float32{float32(math.Sqrt(float64(sum)))}}
	case shaderir.Faceforward:
		n := max(args[0].typ.VectorElementCount(), 1)
		var d float32
		for i := 0; i < n; i++ {
			d += args[2].f[i] * args[1].f[i]
		}
		if d < 0 {
			return args[0]
		}
		return mapVec1(args[0], func(x float32) float32 { return -x })
	case shaderir.Refract:
		n := max(args[0].typ.VectorElementCount(), 1)
		eta := args[2].f[0]
		var d float32
		for i := 0; i < n; i++ {
			d += args[1].f[i] * args[0].f[i]
		}
		k := 1 - eta*eta*(1-d*d)
		out := irVal{typ: args[0].typ}
		if k < 0 {
			return out
		}
		scale := eta*d + float32(math.Sqrt(float64(k)))
		for i := 0; i < n; i++ {
			out.f[i] = eta*args[0].f[i] - scale*args[1].f[i]
		}
		return out
	case shaderir.Min:
		if args[0].typ.Scalar() == shaderir.Uint {
			return mapUint2(args[0], args[1], func(a, b uint32) uint32 { return min(a, b) })
		}
		if args[0].typ.Scalar() == shaderir.Int {
			return mapInt2(args[0], args[1], func(a, b int32) int32 { return min(a, b) })
		}
		return mapVec2(args[0], args[1], func(a, b float32) float32 { return min(a, b) })
	case shaderir.Max:
		if args[0].typ.Scalar() == shaderir.Uint {
			return mapUint2(args[0], args[1], func(a, b uint32) uint32 { return max(a, b) })
		}
		if args[0].typ.Scalar() == shaderir.Int {
			return mapInt2(args[0], args[1], func(a, b int32) int32 { return max(a, b) })
		}
		return mapVec2(args[0], args[1], func(a, b float32) float32 { return max(a, b) })
	case shaderir.Clamp:
		if args[0].typ.Scalar() == shaderir.Uint {
			return mapUint3(args[0], args[1], args[2], func(x, lo, hi uint32) uint32 { return min(max(x, lo), hi) })
		}
		if args[0].typ.Scalar() == shaderir.Int {
			return mapInt3(args[0], args[1], args[2], func(x, lo, hi int32) int32 { return min(max(x, lo), hi) })
		}
		return mapVec3(args[0], args[1], args[2], func(x, lo, hi float32) float32 { return min(max(x, lo), hi) })
	case shaderir.Mix:
		return mapVec3(args[0], args[1], args[2], func(a, b, t float32) float32 { return a*(1-t) + b*t })
	case shaderir.Abs:
		return mapVec1(args[0], func(x float32) float32 { return float32(math.Abs(float64(x))) })
	case shaderir.Floor:
		return mapVec1(args[0], func(x float32) float32 { return float32(math.Floor(float64(x))) })
	case shaderir.Ceil:
		return mapVec1(args[0], func(x float32) float32 { return float32(math.Ceil(float64(x))) })
	case shaderir.Fract:
		return mapVec1(args[0], func(x float32) float32 { return x - float32(math.Floor(float64(x))) })
	case shaderir.Sqrt:
		return mapVec1(args[0], func(x float32) float32 { return float32(math.Sqrt(float64(x))) })
	case shaderir.Inversesqrt:
		return mapVec1(args[0], func(x float32) float32 { return float32(1.0 / math.Sqrt(float64(x))) })
	case shaderir.Sin:
		return mapVec1(args[0], func(x float32) float32 { return float32(math.Sin(float64(x))) })
	case shaderir.Cos:
		return mapVec1(args[0], func(x float32) float32 { return float32(math.Cos(float64(x))) })
	case shaderir.Tan:
		return mapVec1(args[0], func(x float32) float32 { return float32(math.Tan(float64(x))) })
	case shaderir.Radians:
		return mapVec1(args[0], func(x float32) float32 { return x * float32(math.Pi/180) })
	case shaderir.Degrees:
		return mapVec1(args[0], func(x float32) float32 { return x * float32(180/math.Pi) })
	case shaderir.Asin:
		return mapVec1(args[0], func(x float32) float32 { return float32(math.Asin(float64(x))) })
	case shaderir.Acos:
		return mapVec1(args[0], func(x float32) float32 { return float32(math.Acos(float64(x))) })
	case shaderir.Atan:
		return mapVec1(args[0], func(x float32) float32 { return float32(math.Atan(float64(x))) })
	case shaderir.Atan2:
		return mapVec2(args[0], args[1], func(y, x float32) float32 { return float32(math.Atan2(float64(y), float64(x))) })
	case shaderir.Exp:
		return mapVec1(args[0], func(x float32) float32 { return float32(math.Exp(float64(x))) })
	case shaderir.Exp2:
		return mapVec1(args[0], func(x float32) float32 { return float32(math.Exp2(float64(x))) })
	case shaderir.Log:
		return mapVec1(args[0], func(x float32) float32 { return float32(math.Log(float64(x))) })
	case shaderir.Log2:
		return mapVec1(args[0], func(x float32) float32 { return float32(math.Log2(float64(x))) })
	case shaderir.Sinh:
		return mapVec1(args[0], func(x float32) float32 { return float32(math.Sinh(float64(x))) })
	case shaderir.Cosh:
		return mapVec1(args[0], func(x float32) float32 { return float32(math.Cosh(float64(x))) })
	case shaderir.Tanh:
		return mapVec1(args[0], func(x float32) float32 { return float32(math.Tanh(float64(x))) })
	case shaderir.Trunc:
		return mapVec1(args[0], func(x float32) float32 { return float32(math.Trunc(float64(x))) })
	case shaderir.RoundEven:
		return mapVec1(args[0], func(x float32) float32 { return float32(math.RoundToEven(float64(x))) })
	case shaderir.Sign:
		return mapVec1(args[0], func(x float32) float32 {
			switch {
			case x > 0:
				return 1
			case x < 0:
				return -1
			}
			return 0
		})
	case shaderir.Mod:
		return mapVec2(args[0], args[1], func(x, y float32) float32 { return x - y*float32(math.Floor(float64(x/y))) })
	case shaderir.Dfdx, shaderir.Dfdy, shaderir.Fwidth:
		// The software rasterizer draws one fragment at a time, so it has no neighbouring pixels to compare and a derivative is zero here (#2334).
		return mapVec1(args[0], func(float32) float32 { return 0 })
	case shaderir.FloatBitsToInt:
		return floatBits(args[0], shaderir.Int)
	case shaderir.FloatBitsToUint:
		return floatBits(args[0], shaderir.Uint)
	case shaderir.IntBitsToFloat:
		return intBits(args[0])
	case shaderir.UintBitsToFloat:
		return uintBits(args[0])
	case shaderir.Pow:
		return mapVec2(args[0], args[1], func(a, b float32) float32 { return float32(math.Pow(float64(a), float64(b))) })
	case shaderir.Step:
		return mapVec2(args[1], args[0], func(x, edge float32) float32 {
			if x < edge {
				return 0
			}
			return 1
		})
	case shaderir.Smoothstep:
		return mapVec3(args[2], args[0], args[1], func(x, e0, e1 float32) float32 {
			t := clamp01((x - e0) / (e1 - e0))
			return t * t * (3 - 2*t)
		})
	}
	return irVal{}
}

func (vm *irVM) lookupTexture(bindIdx int) *softTexture {
	if bindIdx < 0 || bindIdx >= len(vm.ctx.prog.Bindings) {
		return nil
	}
	b := vm.ctx.prog.Bindings[bindIdx]
	bound := vm.ctx.bindGroups[b.Group]
	if bound.bg == nil {
		return nil
	}
	e, ok := bound.bg.entries[b.Binding]
	if !ok {
		return nil
	}
	return vm.ctx.dev.textures[e.Texture]
}

func (vm *irVM) lookupSampler(bindIdx int) *softSampler {
	if bindIdx < 0 || bindIdx >= len(vm.ctx.prog.Bindings) {
		return nil
	}
	b := vm.ctx.prog.Bindings[bindIdx]
	bound := vm.ctx.bindGroups[b.Group]
	if bound.bg == nil {
		return nil
	}
	e, ok := bound.bg.entries[b.Binding]
	if !ok {
		return nil
	}
	return vm.ctx.dev.samplers[e.Sampler]
}

func wrapCoord(u float32, mode GPUAddressMode) float32 {
	switch mode {
	case GPUAddressRepeat:
		return u - float32(math.Floor(float64(u)))
	case GPUAddressMirrorRepeat:
		f := u - float32(math.Floor(float64(u*0.5)))*2
		if f > 1 {
			return 2 - f
		}
		return f
	default:
		return clamp01(u)
	}
}

// irValFloat returns component 0 of v as a float32, converting integer scalars.
func irValFloat(v irVal) float32 {
	switch v.typ.Scalar() {
	case shaderir.Int:
		return float32(v.i[0])
	case shaderir.Uint:
		return float32(v.u[0])
	default:
		return v.f[0]
	}
}

// cubeDirToFaceUV maps a cubemap direction to a face index (ArrayLayer 0..5) and face-local [0, 1] coordinates using the WebGPU / Metal / Direct3D / OpenGL face conventions.
func cubeDirToFaceUV(x, y, z float32) (face uint32, u, v float32) {
	ax := float32(math.Abs(float64(x)))
	ay := float32(math.Abs(float64(y)))
	az := float32(math.Abs(float64(z)))
	var sc, tc, ma float32
	switch {
	case ax >= ay && ax >= az:
		ma = ax
		if x >= 0 {
			face = GPUCubeFacePosX
			sc, tc = -z, -y
		} else {
			face = GPUCubeFaceNegX
			sc, tc = z, -y
		}
	case ay >= ax && ay >= az:
		ma = ay
		if y >= 0 {
			face = GPUCubeFacePosY
			sc, tc = x, z
		} else {
			face = GPUCubeFaceNegY
			sc, tc = x, -z
		}
	default:
		ma = az
		if z >= 0 {
			face = GPUCubeFacePosZ
			sc, tc = x, -y
		} else {
			face = GPUCubeFaceNegZ
			sc, tc = -x, -y
		}
	}
	if ma < 1e-12 {
		return GPUCubeFacePosX, 0.5, 0.5
	}
	return face, 0.5 * (sc/ma + 1.0), 0.5 * (tc/ma + 1.0)
}

// evalTextureSampleCall evaluates textureSample, textureSampleLevel, textureSampleBias and textureSampleGrad. Arguments follow the IR order (texture, sampler, coords[, layer for 2D arrays], level/bias/gradients[, offset]). Without screen-space derivatives the software device takes the implicit LOD as 0, so textureSample reads the base level clamped by the sampler's MinLOD/MaxLOD.
func (vm *irVM) evalTextureSampleCall(e shaderir.Expr, fn shaderir.BuiltinFunc) irVal {
	opaque := irVal{typ: shaderir.Type{Main: shaderir.Vec4}, f: [16]float32{0, 0, 0, 1}}
	if len(e.Exprs) < 4 {
		return opaque
	}
	base := vm.lookupTexture(e.Exprs[1].Index)
	if base == nil {
		return opaque
	}
	var desc GPUSamplerDesc
	if s := vm.lookupSampler(e.Exprs[2].Index); s != nil {
		desc = s.desc
	}
	coord := vm.evalExpr(e.Exprs[3])
	next := 4
	arg := func() (irVal, bool) {
		if next >= len(e.Exprs) {
			return irVal{}, false
		}
		v := vm.evalExpr(e.Exprs[next])
		next++
		return v, true
	}
	var layer uint32
	u, v := coord.f[0], coord.f[1]
	cube := base.desc.Dimension == GPUDimCube
	switch {
	case cube:
		layer, u, v = cubeDirToFaceUV(coord.f[0], coord.f[1], coord.f[2])
	case base.desc.Dimension == GPUDim2DArray:
		if l, ok := arg(); ok {
			layer = uint32(min(max(int(irValFloat(l)), 0), int(max(base.layers, 1))-1))
		}
	}
	var lod float32
	switch fn {
	case shaderir.TextureSampleLevel, shaderir.TextureSampleBias:
		if l, ok := arg(); ok {
			lod = irValFloat(l)
		}
	case shaderir.TextureSampleGrad:
		ddx, ok1 := arg()
		ddy, ok2 := arg()
		if ok1 && ok2 {
			tw, th := float64(base.desc.Width), float64(base.desc.Height)
			rho := max(math.Hypot(float64(ddx.f[0])*tw, float64(ddx.f[1])*th), math.Hypot(float64(ddy.f[0])*tw, float64(ddy.f[1])*th))
			if rho > 1e-8 {
				lod = float32(math.Log2(rho))
			}
		}
	}
	var offX, offY int
	if off, ok := arg(); ok && !cube {
		offX, offY = int(off.i[0]), int(off.i[1])
	}
	maxLOD := desc.MaxLOD
	if maxLOD <= 0 {
		maxLOD = 32
	}
	lod = min(max(lod, max(desc.MinLOD, 0)), maxLOD)
	mips := max(base.mips, 1)
	lod = min(max(lod, 0), float32(mips-1))
	if cube {
		// Cube faces never repeat: sampling clamps at the face edge.
		desc.AddressU, desc.AddressV = GPUAddressClampToEdge, GPUAddressClampToEdge
	}
	sampleAt := func(mip uint32) irVal {
		return sampleSoftLevel(base.level(layer, mip), desc, u, v, offX, offY, lod > 0)
	}
	if mips <= 1 || desc.MipmapFilter != GPUFilterLinear {
		return sampleAt(uint32(math.Round(float64(lod))))
	}
	lo := uint32(math.Floor(float64(lod)))
	hi := min(lo+1, mips-1)
	frac := lod - float32(lo)
	c0 := sampleAt(lo)
	if hi == lo || frac <= 1e-6 {
		return c0
	}
	c1 := sampleAt(hi)
	for ch := range 4 {
		c0.f[ch] = c0.f[ch]*(1-frac) + c1.f[ch]*frac
	}
	return c0
}

// sampleSoftLevel filters one texture subresource at normalized (u, v). minified selects MinFilter instead of MagFilter.
func sampleSoftLevel(tex *softTexture, desc GPUSamplerDesc, uu, vv float32, offX, offY int, minified bool) irVal {
	if tex == nil {
		return irVal{typ: shaderir.Type{Main: shaderir.Vec4}, f: [16]float32{0, 0, 0, 1}}
	}
	linear := desc.MagFilter == GPUFilterLinear
	if minified {
		linear = desc.MinFilter == GPUFilterLinear
	}
	w, h := int(tex.desc.Width), int(tex.desc.Height)
	u := wrapCoord(uu, desc.AddressU)
	v := wrapCoord(vv, desc.AddressV)
	samples := max(tex.samples, 1)

	if tex.depth != nil {
		readDepth := func(x, y int) float32 {
			x = min(max(x+offX, 0), w-1)
			y = min(max(y+offY, 0), h-1)
			return tex.depth[(y*w+x)*samples]
		}
		if linear {
			fx := u*float32(w) - 0.5
			fy := v*float32(h) - 0.5
			x0 := int(math.Floor(float64(fx)))
			y0 := int(math.Floor(float64(fy)))
			tx := fx - float32(x0)
			ty := fy - float32(y0)
			d00 := readDepth(x0, y0)
			d10 := readDepth(x0+1, y0)
			d01 := readDepth(x0, y0+1)
			d11 := readDepth(x0+1, y0+1)
			top := d00*(1-tx) + d10*tx
			bot := d01*(1-tx) + d11*tx
			return irVal{typ: shaderir.Type{Main: shaderir.Float}, f: [16]float32{top*(1-ty) + bot*ty}}
		}
		ix := int(math.Floor(float64(u * float32(w))))
		iy := int(math.Floor(float64(v * float32(h))))
		return irVal{typ: shaderir.Type{Main: shaderir.Float}, f: [16]float32{readDepth(ix, iy)}}
	}
	if tex.color == nil {
		return irVal{typ: shaderir.Type{Main: shaderir.Vec4}, f: [16]float32{0, 0, 0, 1}}
	}

	// Colors are stored as linear floats (sRGB formats were decoded on upload), so sampling reads them directly.
	readPx := func(x, y int) [4]float32 {
		x = min(max(x+offX, 0), w-1)
		y = min(max(y+offY, 0), h-1)
		off := (y*w + x) * samples * 4
		return [4]float32{
			tex.color[off+0],
			tex.color[off+1],
			tex.color[off+2],
			tex.color[off+3],
		}
	}

	if linear {
		fx := u*float32(w) - 0.5
		fy := v*float32(h) - 0.5
		x0 := int(math.Floor(float64(fx)))
		y0 := int(math.Floor(float64(fy)))
		tx := fx - float32(x0)
		ty := fy - float32(y0)
		c00 := readPx(x0, y0)
		c10 := readPx(x0+1, y0)
		c01 := readPx(x0, y0+1)
		c11 := readPx(x0+1, y0+1)
		out := irVal{typ: shaderir.Type{Main: shaderir.Vec4}}
		for ch := 0; ch < 4; ch++ {
			top := c00[ch]*(1-tx) + c10[ch]*tx
			bot := c01[ch]*(1-tx) + c11[ch]*tx
			out.f[ch] = top*(1-ty) + bot*ty
		}
		return out
	}

	ix := int(math.Floor(float64(u * float32(w))))
	iy := int(math.Floor(float64(v * float32(h))))
	c := readPx(ix, iy)
	return irVal{typ: shaderir.Type{Main: shaderir.Vec4}, f: [16]float32{c[0], c[1], c[2], c[3]}}
}

func (vm *irVM) evalTextureSampleCompare(texIdx, sampIdx int, uv irVal, depthRef float32, offX, offY int) irVal {
	tex := vm.lookupTexture(texIdx)
	if tex == nil || tex.depth == nil {
		return irVal{typ: shaderir.Type{Main: shaderir.Float}, f: [16]float32{0}}
	}
	var desc GPUSamplerDesc
	if s := vm.lookupSampler(sampIdx); s != nil {
		desc = s.desc
	}
	cmpFn := desc.Compare
	if cmpFn == GPUCompareUndefined {
		cmpFn = GPUCompareLessEqual
	}
	w, h := int(tex.desc.Width), int(tex.desc.Height)
	samples := max(tex.samples, 1)
	u := wrapCoord(uv.f[0], desc.AddressU)
	v := wrapCoord(uv.f[1], desc.AddressV)

	// Hardware comparison sampling compares depthRef against each sampled texel BEFORE bilinear filtering (percentage-closer filtering / PCF).
	cmpTexel := func(x, y int) float32 {
		x = min(max(x+offX, 0), w-1)
		y = min(max(y+offY, 0), h-1)
		if compareDepth(cmpFn, depthRef, tex.depth[(y*w+x)*samples]) {
			return 1.0
		}
		return 0.0
	}

	if desc.MagFilter == GPUFilterLinear || desc.MinFilter == GPUFilterLinear {
		fx := u*float32(w) - 0.5
		fy := v*float32(h) - 0.5
		x0 := int(math.Floor(float64(fx)))
		y0 := int(math.Floor(float64(fy)))
		tx := fx - float32(x0)
		ty := fy - float32(y0)
		c00 := cmpTexel(x0, y0)
		c10 := cmpTexel(x0+1, y0)
		c01 := cmpTexel(x0, y0+1)
		c11 := cmpTexel(x0+1, y0+1)
		top := c00*(1-tx) + c10*tx
		bot := c01*(1-tx) + c11*tx
		return irVal{typ: shaderir.Type{Main: shaderir.Float}, f: [16]float32{top*(1-ty) + bot*ty}}
	}

	ix := int(math.Floor(float64(u * float32(w))))
	iy := int(math.Floor(float64(v * float32(h))))
	return irVal{typ: shaderir.Type{Main: shaderir.Float}, f: [16]float32{cmpTexel(ix, iy)}}
}

// evalTextureLoad reads one texel; layer selects the 2D array layer, and extra is the sample index for multisampled textures or the mip level otherwise.
func (vm *irVM) evalTextureLoad(texIdx int, coord, layer, extra irVal) irVal {
	base := vm.lookupTexture(texIdx)
	if base == nil {
		return irVal{typ: shaderir.Type{Main: shaderir.Vec4}, f: [16]float32{0, 0, 0, 1}}
	}
	tex := base
	if max(base.samples, 1) == 1 {
		// For single-sampled textures the extra argument is the mip level.
		l := uint32(max(int(irValFloat(layer)), 0))
		m := uint32(max(int(irValFloat(extra)), 0))
		tex = base.level(min(l, max(base.layers, 1)-1), min(m, max(base.mips, 1)-1))
	}
	if tex == nil {
		return irVal{typ: shaderir.Type{Main: shaderir.Vec4}, f: [16]float32{0, 0, 0, 1}}
	}
	w, h := int(tex.desc.Width), int(tex.desc.Height)
	cx, cy := int(coord.i[0]), int(coord.i[1])
	if coord.typ.Scalar() == shaderir.Uint {
		cx, cy = int(coord.u[0]), int(coord.u[1])
	}
	ix := min(max(cx, 0), w-1)
	iy := min(max(cy, 0), h-1)
	samples := max(tex.samples, 1)
	samp := 0
	if samples > 1 {
		samp = int(extra.i[0])
		if extra.typ.Scalar() == shaderir.Uint {
			samp = int(extra.u[0])
		}
		samp = min(max(samp, 0), samples-1)
	}
	if tex.depth != nil {
		return irVal{typ: shaderir.Type{Main: shaderir.Float}, f: [16]float32{tex.depth[(iy*w+ix)*samples+samp]}}
	}
	if tex.color == nil {
		return irVal{typ: shaderir.Type{Main: shaderir.Vec4}, f: [16]float32{0, 0, 0, 1}}
	}
	off := ((iy*w+ix)*samples + samp) * 4
	return irVal{typ: shaderir.Type{Main: shaderir.Vec4}, f: [16]float32{
		tex.color[off+0],
		tex.color[off+1],
		tex.color[off+2],
		tex.color[off+3],
	}}
}

func (vm *irVM) evalTextureDimensions(texIdx int, level irVal) irVal {
	base := vm.lookupTexture(texIdx)
	if base == nil {
		return irVal{typ: shaderir.Type{Main: shaderir.UVec2}}
	}
	tex := base.level(0, min(uint32(max(int(irValFloat(level)), 0)), max(base.mips, 1)-1))
	if tex == nil {
		tex = base
	}
	return irVal{typ: shaderir.Type{Main: shaderir.UVec2}, u: [4]uint32{tex.desc.Width, tex.desc.Height}, f: [16]float32{float32(tex.desc.Width), float32(tex.desc.Height)}}
}

func (vm *irVM) evalTextureStore(texIdx int, coord, val irVal) {
	tex := vm.lookupTexture(texIdx)
	if tex == nil || tex.color == nil {
		return
	}
	w, h := int(tex.desc.Width), int(tex.desc.Height)
	var ix, iy int
	if coord.typ.Scalar() == shaderir.Uint {
		ix, iy = int(coord.u[0]), int(coord.u[1])
	} else {
		ix, iy = int(coord.i[0]), int(coord.i[1])
	}
	if ix < 0 || ix >= w || iy < 0 || iy >= h {
		return
	}
	tex.storeTexel(iy*w+ix, quantizeColorForFormat(tex.desc.Format, [4]float32{val.f[0], val.f[1], val.f[2], val.f[3]}))
}

func packFloatVec(bt shaderir.BasicType, n int, args []irVal) irVal {
	out := irVal{typ: shaderir.Type{Main: bt}}
	if len(args) == 0 {
		return out
	}
	if len(args) == 1 && args[0].typ.VectorElementCount() < 2 {
		for i := 0; i < n; i++ {
			out.f[i] = args[0].f[0]
		}
		return out
	}
	k := 0
	for _, a := range args {
		cnt := a.typ.VectorElementCount()
		if cnt < 1 {
			cnt = 1
		}
		for i := 0; i < cnt && k < n; i++ {
			out.f[k] = a.f[i]
			k++
		}
	}
	return out
}

func irValIntAt(v irVal, idx int) int32 {
	switch v.typ.Scalar() {
	case shaderir.Int:
		return v.i[idx]
	case shaderir.Uint:
		return int32(v.u[idx])
	default:
		return int32(v.f[idx])
	}
}

func irValUintAt(v irVal, idx int) uint32 {
	switch v.typ.Scalar() {
	case shaderir.Uint:
		return v.u[idx]
	case shaderir.Int:
		return uint32(v.i[idx])
	default:
		return uint32(v.f[idx])
	}
}

func packIntVec(bt shaderir.BasicType, n int, args []irVal) irVal {
	out := irVal{typ: shaderir.Type{Main: bt}}
	if len(args) == 0 {
		return out
	}
	if len(args) == 1 && args[0].typ.VectorElementCount() < 2 {
		v := irValIntAt(args[0], 0)
		for i := 0; i < n; i++ {
			out.i[i] = v
			out.f[i] = float32(v)
		}
		return out
	}
	k := 0
	for _, a := range args {
		cnt := max(a.typ.VectorElementCount(), 1)
		for i := 0; i < cnt && k < n; i++ {
			out.i[k] = irValIntAt(a, i)
			out.f[k] = float32(out.i[k])
			k++
		}
	}
	return out
}

func packUintVec(bt shaderir.BasicType, n int, args []irVal) irVal {
	out := irVal{typ: shaderir.Type{Main: bt}}
	if len(args) == 0 {
		return out
	}
	if len(args) == 1 && args[0].typ.VectorElementCount() < 2 {
		v := irValUintAt(args[0], 0)
		for i := 0; i < n; i++ {
			out.u[i] = v
			out.f[i] = float32(v)
		}
		return out
	}
	k := 0
	for _, a := range args {
		cnt := max(a.typ.VectorElementCount(), 1)
		for i := 0; i < cnt && k < n; i++ {
			out.u[k] = irValUintAt(a, i)
			out.f[k] = float32(out.u[k])
			k++
		}
	}
	return out
}

func packMat(bt shaderir.BasicType, dim int, args []irVal) irVal {
	out := irVal{typ: shaderir.Type{Main: bt}}
	if len(args) == 1 && args[0].typ.IsMatrix() {
		src := args[0]
		sc, sr := src.typ.MatrixCols(), src.typ.MatrixRows()
		for c := 0; c < dim && c < sc; c++ {
			for r := 0; r < dim && r < sr; r++ {
				out.f[c*dim+r] = src.f[c*sr+r]
			}
		}
		return out
	}
	k := 0
	for _, a := range args {
		cnt := a.typ.VectorElementCount()
		if cnt < 1 {
			cnt = 1
		}
		for i := 0; i < cnt && k < dim*dim; i++ {
			out.f[k] = a.f[i]
			k++
		}
	}
	return out
}

func evalSwizzle(v irVal, sw string) irVal {
	if len(sw) == 1 {
		idx := swizzleIdx(sw[0])
		if idx < 0 || idx >= 4 {
			idx = 0
		}
		return irVal{typ: shaderir.Type{Main: v.typ.Scalar()}, f: [16]float32{v.f[idx]}, i: [4]int32{v.i[idx]}, u: [4]uint32{v.u[idx]}}
	}
	out := irVal{typ: shaderir.Type{Main: shaderir.VectorOf(v.typ.Scalar(), len(sw))}}
	for i := 0; i < len(sw) && i < 4; i++ {
		idx := swizzleIdx(sw[i])
		if idx >= 0 && idx < 4 {
			out.f[i] = v.f[idx]
			out.i[i] = v.i[idx]
			out.u[i] = v.u[idx]
		}
	}
	return out
}

func swizzleIdx(c byte) int {
	switch c {
	case 'x', 'r', 's':
		return 0
	case 'y', 'g', 't':
		return 1
	case 'z', 'b', 'p':
		return 2
	case 'w', 'a', 'q':
		return 3
	}
	return -1
}

// packMatrix builds a cols x rows matrix from a matrix of another shape or from scalars and column vectors, as its constructor is written.
func packMatrix(bt shaderir.BasicType, cols, rows int, args []irVal) irVal {
	out := irVal{typ: shaderir.Type{Main: bt}}
	if len(args) == 1 && args[0].typ.IsMatrix() {
		src := args[0]
		sc, sr := src.typ.MatrixCols(), src.typ.MatrixRows()
		for c := 0; c < cols && c < sc; c++ {
			for r := 0; r < rows && r < sr; r++ {
				out.f[c*rows+r] = src.f[c*sr+r]
			}
		}
		return out
	}
	k := 0
	for _, a := range args {
		cnt := max(a.typ.VectorElementCount(), 1)
		for i := 0; i < cnt && k < cols*rows; i++ {
			out.f[k] = a.f[i]
			k++
		}
	}
	return out
}

// floatBits reinterprets each float lane's bits as an int or uint lane.
func floatBits(a irVal, main shaderir.BasicType) irVal {
	n := max(a.typ.VectorElementCount(), 1)
	out := irVal{typ: shaderir.Type{Main: shaderir.VectorOf(main, n)}}
	for i := 0; i < n; i++ {
		bits := math.Float32bits(a.f[i])
		if main == shaderir.Int {
			out.i[i] = int32(bits)
			out.f[i] = float32(out.i[i])
		} else {
			out.u[i] = bits
			out.f[i] = float32(out.u[i])
		}
	}
	return out
}

// intBits reinterprets each int lane's bits as a float lane.
func intBits(a irVal) irVal {
	n := max(a.typ.VectorElementCount(), 1)
	out := irVal{typ: shaderir.Type{Main: shaderir.VectorOf(shaderir.Float, n)}}
	for i := 0; i < n; i++ {
		out.f[i] = math.Float32frombits(uint32(a.i[i]))
	}
	return out
}

// uintBits reinterprets each uint lane's bits as a float lane.
func uintBits(a irVal) irVal {
	n := max(a.typ.VectorElementCount(), 1)
	out := irVal{typ: shaderir.Type{Main: shaderir.VectorOf(shaderir.Float, n)}}
	for i := 0; i < n; i++ {
		out.f[i] = math.Float32frombits(a.u[i])
	}
	return out
}

// determinant3 is the determinant of a 3x3 matrix stored by column.
func determinant3(f []float32) float32 {
	return f[0]*(f[4]*f[8]-f[7]*f[5]) - f[3]*(f[1]*f[8]-f[7]*f[2]) + f[6]*(f[1]*f[5]-f[4]*f[2])
}

// determinant4 is the determinant of a 4x4 matrix stored by column, by cofactor expansion along its first column.
func determinant4(f []float32) float32 {
	var det float32
	for r := 0; r < 4; r++ {
		sub := make([]float32, 0, 9)
		for c := 1; c < 4; c++ {
			for rr := 0; rr < 4; rr++ {
				if rr == r {
					continue
				}
				sub = append(sub, f[c*4+rr])
			}
		}
		term := f[r] * determinant3(sub)
		if r%2 == 1 {
			det -= term
		} else {
			det += term
		}
	}
	return det
}

func mapVec1(a irVal, fn func(float32) float32) irVal {
	n := max(a.typ.VectorElementCount(), 1)
	out := irVal{typ: a.typ}
	for i := 0; i < n; i++ {
		out.f[i] = fn(a.f[i])
	}
	return out
}

func mapVec2(a, b irVal, fn func(float32, float32) float32) irVal {
	resT := a.typ
	n := max(a.typ.VectorElementCount(), b.typ.VectorElementCount())
	if n < 1 {
		n = 1
	}
	if b.typ.VectorElementCount() > a.typ.VectorElementCount() {
		resT = b.typ
	}
	out := irVal{typ: resT}
	aS, bS := a.typ.VectorElementCount() < 2, b.typ.VectorElementCount() < 2
	for i := 0; i < n; i++ {
		ai, bi := i, i
		if aS {
			ai = 0
		}
		if bS {
			bi = 0
		}
		out.f[i] = fn(a.f[ai], b.f[bi])
	}
	return out
}

func mapVec3(a, b, c irVal, fn func(float32, float32, float32) float32) irVal {
	n := max(a.typ.VectorElementCount(), max(b.typ.VectorElementCount(), c.typ.VectorElementCount()))
	if n < 1 {
		n = 1
	}
	out := irVal{typ: a.typ}
	if n > 1 && a.typ.VectorElementCount() < 2 {
		out.typ = shaderir.Type{Main: shaderir.VectorOf(shaderir.Float, n)}
	}
	aS, bS, cS := a.typ.VectorElementCount() < 2, b.typ.VectorElementCount() < 2, c.typ.VectorElementCount() < 2
	for i := 0; i < n; i++ {
		ai, bi, ci := i, i, i
		if aS {
			ai = 0
		}
		if bS {
			bi = 0
		}
		if cS {
			ci = 0
		}
		out.f[i] = fn(a.f[ai], b.f[bi], c.f[ci])
	}
	return out
}

func mapInt2(a, b irVal, fn func(int32, int32) int32) irVal {
	n := max(a.typ.VectorElementCount(), b.typ.VectorElementCount())
	if n < 1 {
		n = 1
	}
	resT := a.typ
	if b.typ.VectorElementCount() > a.typ.VectorElementCount() {
		resT = b.typ
	}
	out := irVal{typ: resT}
	aS, bS := a.typ.VectorElementCount() < 2, b.typ.VectorElementCount() < 2
	for i := 0; i < n; i++ {
		ai, bi := i, i
		if aS {
			ai = 0
		}
		if bS {
			bi = 0
		}
		out.i[i] = fn(a.i[ai], b.i[bi])
		out.f[i] = float32(out.i[i])
	}
	return out
}

func mapUint2(a, b irVal, fn func(uint32, uint32) uint32) irVal {
	n := max(a.typ.VectorElementCount(), b.typ.VectorElementCount())
	if n < 1 {
		n = 1
	}
	resT := a.typ
	if b.typ.VectorElementCount() > a.typ.VectorElementCount() {
		resT = b.typ
	}
	out := irVal{typ: resT}
	aS, bS := a.typ.VectorElementCount() < 2, b.typ.VectorElementCount() < 2
	for i := 0; i < n; i++ {
		ai, bi := i, i
		if aS {
			ai = 0
		}
		if bS {
			bi = 0
		}
		out.u[i] = fn(a.u[ai], b.u[bi])
		out.f[i] = float32(out.u[i])
	}
	return out
}

func mapInt3(a, b, c irVal, fn func(int32, int32, int32) int32) irVal {
	n := max(a.typ.VectorElementCount(), max(b.typ.VectorElementCount(), c.typ.VectorElementCount()))
	if n < 1 {
		n = 1
	}
	out := irVal{typ: a.typ}
	aS, bS, cS := a.typ.VectorElementCount() < 2, b.typ.VectorElementCount() < 2, c.typ.VectorElementCount() < 2
	for i := 0; i < n; i++ {
		ai, bi, ci := i, i, i
		if aS {
			ai = 0
		}
		if bS {
			bi = 0
		}
		if cS {
			ci = 0
		}
		out.i[i] = fn(a.i[ai], b.i[bi], c.i[ci])
		out.f[i] = float32(out.i[i])
	}
	return out
}

func mapUint3(a, b, c irVal, fn func(uint32, uint32, uint32) uint32) irVal {
	n := max(a.typ.VectorElementCount(), max(b.typ.VectorElementCount(), c.typ.VectorElementCount()))
	if n < 1 {
		n = 1
	}
	out := irVal{typ: a.typ}
	aS, bS, cS := a.typ.VectorElementCount() < 2, b.typ.VectorElementCount() < 2, c.typ.VectorElementCount() < 2
	for i := 0; i < n; i++ {
		ai, bi, ci := i, i, i
		if aS {
			ai = 0
		}
		if bS {
			bi = 0
		}
		if cS {
			ci = 0
		}
		out.u[i] = fn(a.u[ai], b.u[bi], c.u[ci])
		out.f[i] = float32(out.u[i])
	}
	return out
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func floatToByte(v float32) byte {
	v = clamp01(v)
	return byte(math.Round(float64(v * 255.0)))
}
