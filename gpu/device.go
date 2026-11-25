package gpu

import (
	"fmt"
	"image"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	fallbackIDGen atomic.Uint64
	readbackIDGen atomic.Uint64
)

// Device owns persistent GPU resources and submits recorded command buffers
// for one mounted gpu.View (or a standalone SoftwareRenderer).
// All methods are called on the engine goroutine.
type Device struct {
	surf       *core.GPUSurface
	submitFn   func(driver.GPUFrame)
	scope      *core.Scope
	generation uint32
	lost       atomic.Bool
	suspended  atomic.Bool

	retainResources bool
	cache           *PipelineCache

	limits            Limits
	features          Features
	compressedFormats []TextureFormat
	surfaceFormat     TextureFormat
	surfaceColorSpace ColorSpace
	width, height     int
	framesInFlight    int
	directMapped      bool

	frameIndex       uint64
	inFrame          bool
	activeFrameIndex uint64
	activeFrameSlot  int
	activeFence      *FrameFence
	slotFences       []*FrameFence

	fenceMu sync.Mutex
	fences  map[uint64]*FrameFence

	ringBuffers      map[*RingBuffer]struct{}
	opMu             sync.Mutex
	pendingOps       []driver.GPUResourceOp
	readbacks        map[uint64]func(rgba []byte, err error)
	lastPassTimings  []PassTiming
	timestampQueries map[driver.GPUID]*TimestampQuery

	// Retained resource lists for opt-in automatic recreation after device loss. retainedMu guards the two lists a close removes from, so a frame that creates and closes resources cannot race a restore walking them.
	retainedMu         sync.Mutex
	retainedBuffers    []*Buffer
	retainedTextures   []*Texture
	retainedSamplers   []*Sampler
	retainedLayouts    []*BindGroupLayout
	retainedBindGroups []*BindGroup
	retainedPipelines  []*Pipeline
}

func newDevice(surf *core.GPUSurface, scope *core.Scope, gen uint32, w, h int, fmt TextureFormat, colorSpace ColorSpace, framesInFlight int, lim Limits, feat Features, compressed []TextureFormat) *Device {
	if fmt == FormatUndefined {
		fmt = FormatRGBA8Unorm
	}
	if colorSpace == ColorSpaceDefault {
		colorSpace = ColorSpaceSRGB
	}
	if len(compressed) == 0 {
		compressed = compressedFormatsFromFeatures(feat)
	} else {
		compressed = slices.Clone(compressed)
	}
	n := driver.NormalizeFramesInFlight(framesInFlight)
	return &Device{
		framesInFlight:    n,
		slotFences:        make([]*FrameFence, n),
		fences:            map[uint64]*FrameFence{},
		ringBuffers:       map[*RingBuffer]struct{}{},
		surf:              surf,
		scope:             scope,
		generation:        gen,
		limits:            lim,
		features:          feat,
		compressedFormats: compressed,
		surfaceFormat:     fmt,
		surfaceColorSpace: colorSpace,
		width:             w,
		height:            h,
		readbacks:         map[uint64]func([]byte, error){},
		timestampQueries:  map[driver.GPUID]*TimestampQuery{},
	}
}

func (d *Device) nextID() driver.GPUID {
	if d.surf != nil {
		return driver.GPUID(d.surf.NextID())
	}
	return driver.GPUID(fallbackIDGen.Add(1))
}

func (d *Device) alive() bool {
	if d == nil || d.lost.Load() {
		return false
	}
	if d.surf != nil && d.surf.Disposed() {
		return false
	}
	return true
}

func (d *Device) setSuspended(v bool) {
	if d == nil {
		return
	}
	d.suspended.Store(v)
}

func (d *Device) invalidate(reason DeviceLostReason, msg string) {
	if d == nil || !d.lost.CompareAndSwap(false, true) {
		return
	}
	d.opMu.Lock()
	d.pendingOps = nil
	d.lastPassTimings = nil
	for id, q := range d.timestampQueries {
		delete(d.timestampQueries, id)
		q.closed = true
	}
	rbs := d.readbacks
	d.readbacks = map[uint64]func([]byte, error){}
	d.opMu.Unlock()

	d.inFrame = false
	if d.activeFence != nil {
		d.activeFence.signal()
		d.activeFence = nil
	}
	d.fenceMu.Lock()
	pendingFences := make([]*FrameFence, 0, len(d.fences))
	for id, f := range d.fences {
		delete(d.fences, id)
		pendingFences = append(pendingFences, f)
	}
	d.fenceMu.Unlock()
	for _, f := range pendingFences {
		f.signal()
	}
	for _, f := range d.slotFences {
		f.signal()
	}
	err := Error{Code: ErrDeviceLost, Message: msg}
	for _, fn := range rbs {
		if fn != nil {
			fn(nil, err)
		}
	}
}

func (d *Device) completePassTimings(raw []driver.GPUPassTiming) []PassTiming {
	if d == nil {
		return nil
	}
	if len(raw) == 0 {
		d.lastPassTimings = nil
		return nil
	}
	out := make([]PassTiming, len(raw))
	for i, rt := range raw {
		pt := PassTiming{
			Label:     rt.Label,
			PassIndex: rt.PassIndex,
			Duration:  rt.Duration,
		}
		out[i] = pt
		if rt.QueryID != 0 {
			if q := d.timestampQueries[rt.QueryID]; q != nil && !q.closed {
				q.timings = append(q.timings, pt)
			}
		}
	}
	d.lastPassTimings = out
	return slices.Clone(out)
}

func (d *Device) registerFence(f *FrameFence) {
	if d == nil || f == nil {
		return
	}
	d.fenceMu.Lock()
	d.fences[f.id] = f
	d.fenceMu.Unlock()
}

func (d *Device) completeFence(id uint64) {
	if d == nil || id == 0 {
		return
	}
	d.fenceMu.Lock()
	f := d.fences[id]
	delete(d.fences, id)
	d.fenceMu.Unlock()
	if f != nil {
		f.signal()
	}
}

func (d *Device) hasDirtyRingBuffers(slot int) bool {
	for rb := range d.ringBuffers {
		if rb.isDirty(slot) {
			return true
		}
	}
	return false
}

func (d *Device) flushRingBuffers(slot int) {
	for rb := range d.ringBuffers {
		rb.flushSlot(slot)
	}
}

func (d *Device) completeReadback(id uint64, pixels []byte, errMsg string) {
	if d == nil {
		return
	}
	d.opMu.Lock()
	fn, ok := d.readbacks[id]
	if ok {
		delete(d.readbacks, id)
	}
	d.opMu.Unlock()
	if !ok || fn == nil {
		return
	}
	if errMsg != "" {
		fn(nil, Error{Code: ErrFailed, Message: errMsg})
		return
	}
	fn(pixels, nil)
}

func (d *Device) enqueueOp(op driver.GPUResourceOp) {
	if !d.alive() {
		return
	}
	d.opMu.Lock()
	d.pendingOps = append(d.pendingOps, op)
	d.opMu.Unlock()
}

func (d *Device) hasPendingOps() bool {
	if d == nil {
		return false
	}
	d.opMu.Lock()
	n := len(d.pendingOps)
	d.opMu.Unlock()
	return n > 0
}

func (d *Device) flushOpsOnly() {
	if !d.alive() || d.suspended.Load() {
		return
	}
	d.opMu.Lock()
	if len(d.pendingOps) == 0 {
		d.opMu.Unlock()
		return
	}
	ops := d.pendingOps
	d.pendingOps = nil
	d.opMu.Unlock()
	d.dispatchFrame(driver.GPUFrame{
		Generation: d.generation,
		Ops:        ops,
	})
}

func (d *Device) dispatchFrame(f driver.GPUFrame) {
	if !d.alive() || d.suspended.Load() {
		return
	}
	if d.submitFn != nil {
		d.submitFn(f)
		return
	}
	if d.surf != nil {
		d.surf.Submit(f)
	}
}

func (d *Device) bindCleanup(cx *core.Scope, fn func()) {
	owner := cx
	if owner == nil {
		owner = d.scope
	}
	if owner != nil {
		owner.Cleanup(fn)
	}
}

// Limits returns the device's capability limits.
func (d *Device) Limits() Limits { return d.limits }

// Features returns optional features supported by the device.
func (d *Device) Features() Features { return d.features }

// Capabilities returns the device's limits, optional features, GPU timestamp query support, and supported compressed texture formats.
func (d *Device) Capabilities() DeviceCapabilities {
	return DeviceCapabilities{
		Limits:            d.limits,
		Features:          d.features,
		TimestampQueries:  d.features.TimestampQueries,
		CompressedFormats: slices.Clone(d.compressedFormats),
	}
}

// CompressedFormats returns the compressed texture formats supported by d.
func (d *Device) CompressedFormats() []TextureFormat {
	return slices.Clone(d.compressedFormats)
}

// SupportsCompressedFormat reports whether d supports uploading and sampling compressed textures in format.
func (d *Device) SupportsCompressedFormat(format TextureFormat) bool {
	if d == nil || !format.IsCompressed() {
		return false
	}
	return slices.Contains(d.compressedFormats, format)
}

// SupportsFormat reports whether d can create textures in format. Compressed formats follow SupportsCompressedFormat, the import-only FormatR16Unorm and FormatRG16Unorm are never created, and the extended formats (FormatBGRA8UnormSRGB, FormatRG11B10Float, FormatR32Float, FormatRGBA32Float, FormatStencil8) require Features.ExtendedFormats.
func (d *Device) SupportsFormat(format TextureFormat) bool {
	switch {
	case d == nil || format == FormatUndefined:
		return false
	case format.IsCompressed():
		return d.SupportsCompressedFormat(format)
	case driver.GPUTextureFormat(format).IsImportOnly():
		return false
	case driver.GPUTextureFormat(format).IsExtended():
		return d.features.ExtendedFormats
	default:
		return format.String() != "Unknown"
	}
}

// SupportsTimestampQueries reports whether d supports per-pass GPU timing queries.
func (d *Device) SupportsTimestampQueries() bool {
	return d != nil && d.features.TimestampQueries
}

// LastPassTimings returns the most recently completed GPU frame's pass timings.
func (d *Device) LastPassTimings() []PassTiming {
	if d == nil {
		return nil
	}
	return slices.Clone(d.lastPassTimings)
}

// SupportsCompute reports whether the active device supports @compute shader dispatch and read/write storage buffers. Targets without compute support (such as WebGL 2 and OpenGL ES 3.0) return false.
func (d *Device) SupportsCompute() bool {
	return d != nil && d.features.Compute
}

// SurfaceFormat returns the swapchain's negotiated color texture format.
func (d *Device) SurfaceFormat() TextureFormat { return d.surfaceFormat }

// SurfaceColorSpace returns the swapchain's negotiated color space.
func (d *Device) SurfaceColorSpace() ColorSpace { return d.surfaceColorSpace }

// SurfaceSize returns the current drawable size in physical pixels.
func (d *Device) SurfaceSize() (width, height int) { return d.width, d.height }

// Generation returns the device generation counter (incremented after device loss).
func (d *Device) Generation() uint32 { return d.generation }

// FramesInFlight returns the configured ring-buffer frame-in-flight count (2 or 3).
func (d *Device) FramesInFlight() int {
	if d == nil || d.framesInFlight == 0 {
		return DefaultFramesInFlight
	}
	return d.framesInFlight
}

// FrameIndex returns the monotonic frame index of the active frame, or the next
// frame to be started by BeginFrame.
func (d *Device) FrameIndex() uint64 {
	if d == nil {
		return 0
	}
	if d.inFrame {
		return d.activeFrameIndex
	}
	return d.frameIndex
}

// FrameSlot returns the ring-buffer slot index [0, FramesInFlight) for the
// active frame, or the next frame to be started by BeginFrame.
func (d *Device) FrameSlot() int {
	if d == nil {
		return 0
	}
	if d.inFrame {
		return d.activeFrameSlot
	}
	return int(d.frameIndex % uint64(d.FramesInFlight()))
}

// SlotFence returns the most recent FrameFence submitted for ring slot, or nil
// if slot has not yet been submitted.
func (d *Device) SlotFence(slot int) *FrameFence {
	if d == nil || slot < 0 || slot >= len(d.slotFences) {
		return nil
	}
	return d.slotFences[slot]
}

// WaitForFrame blocks until the GPU completes any in-flight frame using slot.
func (d *Device) WaitForFrame(slot int) {
	if f := d.SlotFence(slot); f != nil {
		f.Wait()
	}
}

// WaitIdle blocks until all in-flight frame slots on d have completed.
func (d *Device) WaitIdle() {
	if d == nil {
		return
	}
	for _, f := range d.slotFences {
		if f != nil {
			f.Wait()
		}
	}
}

// Epoch returns the device epoch counter (alias of Generation, incremented on every device loss and surface recreation).
func (d *Device) Epoch() uint32 { return d.generation }

// IsLost reports whether this Device has been invalidated by device/context loss or view disposal.
func (d *Device) IsLost() bool { return d == nil || d.lost.Load() }

// IsSuspended reports whether GPU frame submission is currently paused because the owning app or surface is in the background.
func (d *Device) IsSuspended() bool { return d != nil && d.suspended.Load() }

// SetPipelineCache attaches a PipelineCache to d so CreatePipeline, CreatePipelineAsync, and PrecompilePipelines reuse compiled artifacts across launches and device restores.
func (d *Device) SetPipelineCache(cache *PipelineCache) {
	if d == nil {
		return
	}
	d.cache = cache
}

// PipelineCache returns the PipelineCache attached to d, or nil.
func (d *Device) PipelineCache() *PipelineCache {
	if d == nil {
		return nil
	}
	return d.cache
}

// CacheStats returns the hit/miss/compilation counters of the attached PipelineCache, or a zero PipelineCacheStats if no cache is attached.
func (d *Device) CacheStats() PipelineCacheStats {
	if d == nil || d.cache == nil {
		return PipelineCacheStats{}
	}
	return d.cache.CacheStats()
}

// --- Retained GPU resources ---

// Buffer is a retained GPU vertex, index, uniform, or storage buffer.
type Buffer struct {
	dev      *Device
	id       driver.GPUID
	epoch    uint32
	label    string
	size     uint64
	usage    BufferUsage
	retained bool
	shadow   []byte
	closed   bool
}

// ID returns the buffer's process-unique numeric handle.
func (b *Buffer) ID() uint64 { return uint64(b.id) }

// Epoch returns the device epoch this buffer currently belongs to.
func (b *Buffer) Epoch() uint32 {
	if b == nil {
		return 0
	}
	return b.epoch
}

// Valid reports whether b is open and bound to a live device at its current epoch.
func (b *Buffer) Valid() bool {
	return b != nil && !b.closed && b.dev != nil && b.dev.alive() && b.epoch == b.dev.generation
}

// Size returns the buffer's byte length.
func (b *Buffer) Size() uint64 { return b.size }

// Usage returns the buffer's usage flags.
func (b *Buffer) Usage() BufferUsage { return b.usage }

// Close releases the buffer immediately. Safe to call more than once.
func (b *Buffer) Close() error {
	if b == nil || b.closed {
		return nil
	}
	b.closed = true
	b.shadow = nil
	if b.dev != nil && b.dev.alive() && b.epoch == b.dev.generation {
		b.dev.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpDestroyBuffer, ID: b.id})
	}
	return nil
}

type retainedTextureWrite struct {
	dst    TextureCopy
	layout DataLayout
	data   []byte
}

// Texture is a retained GPU 2D, cube, array, or depth texture.
type Texture struct {
	dev          *Device
	id           driver.GPUID
	epoch        uint32
	desc         TextureDesc
	retained     bool
	shadowWrites []retainedTextureWrite
	closed       bool
	// external marks a texture from ImportTexture, which is only sampled.
	external bool
}

// ID returns the texture's numeric handle.
func (t *Texture) ID() uint64 { return uint64(t.id) }

// Epoch returns the device epoch this texture currently belongs to.
func (t *Texture) Epoch() uint32 {
	if t == nil {
		return 0
	}
	return t.epoch
}

// Valid reports whether t is open and bound to a live device at its current epoch.
func (t *Texture) Valid() bool {
	return t != nil && !t.closed && t.dev != nil && t.dev.alive() && t.epoch == t.dev.generation
}

// Desc returns the texture's descriptor.
func (t *Texture) Desc() TextureDesc { return t.desc }

// Close releases the texture immediately.
func (t *Texture) Close() error {
	if t == nil || t.closed {
		return nil
	}
	t.closed = true
	t.shadowWrites = nil
	if t.dev != nil && t.dev.alive() && t.epoch == t.dev.generation {
		t.dev.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpDestroyTexture, ID: t.id})
	}
	return nil
}

// RenderTarget pairs an offscreen color Texture with optional MSAA Resolve and depth/stencil Textures.
type RenderTarget struct {
	Color   *Texture
	Resolve *Texture // non-nil when created with SampleCount > 1
	Depth   *Texture
	Width   uint32
	Height  uint32
	closed  bool
}

// Epoch returns the device epoch of the render target's attachment.
func (rt *RenderTarget) Epoch() uint32 {
	if rt == nil {
		return 0
	}
	if rt.Color != nil {
		return rt.Color.Epoch()
	}
	if rt.Depth != nil {
		return rt.Depth.Epoch()
	}
	if rt.Resolve != nil {
		return rt.Resolve.Epoch()
	}
	return 0
}

// Valid reports whether rt is open and its underlying textures are valid.
func (rt *RenderTarget) Valid() bool {
	if rt == nil || rt.closed {
		return false
	}
	hasAttachment := false
	for _, texture := range []*Texture{rt.Color, rt.Resolve, rt.Depth} {
		if texture == nil {
			continue
		}
		hasAttachment = true
		if !texture.Valid() {
			return false
		}
	}
	if !hasAttachment {
		return false
	}
	return true
}

// Close releases the render target's color, resolve, and depth/stencil textures.
func (rt *RenderTarget) Close() error {
	if rt == nil || rt.closed {
		return nil
	}
	rt.closed = true
	if rt.Color != nil {
		_ = rt.Color.Close()
	}
	if rt.Resolve != nil {
		_ = rt.Resolve.Close()
	}
	if rt.Depth != nil {
		_ = rt.Depth.Close()
	}
	return nil
}

// Sampler is a retained GPU texture sampler.
type Sampler struct {
	dev      *Device
	id       driver.GPUID
	epoch    uint32
	desc     SamplerDesc
	retained bool
	closed   bool
}

// ID returns the sampler's numeric handle.
func (s *Sampler) ID() uint64 { return uint64(s.id) }

// Desc returns the sampler's descriptor.
func (s *Sampler) Desc() SamplerDesc { return s.desc }

// Epoch returns the device epoch this sampler currently belongs to.
func (s *Sampler) Epoch() uint32 {
	if s == nil {
		return 0
	}
	return s.epoch
}

// Valid reports whether s is open and bound to a live device at its current epoch.
func (s *Sampler) Valid() bool {
	return s != nil && !s.closed && s.dev != nil && s.dev.alive() && s.epoch == s.dev.generation
}

// Close releases the sampler immediately.
func (s *Sampler) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	if s.dev != nil && s.dev.alive() && s.epoch == s.dev.generation {
		s.dev.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpDestroySampler, ID: s.id})
	}
	return nil
}

// BindGroupLayout describes the resource schema of a BindGroup slot.
type BindGroupLayout struct {
	dev     *Device
	id      driver.GPUID
	epoch   uint32
	entries []BindGroupLayoutEntry
	// label is the debug label the layout was created with, kept for a restore.
	label       string
	retained    bool
	closed      bool
	retainedIdx int
}

// ID returns the layout's numeric handle.
func (l *BindGroupLayout) ID() uint64 { return uint64(l.id) }

// Epoch returns the device epoch this layout currently belongs to.
func (l *BindGroupLayout) Epoch() uint32 {
	if l == nil {
		return 0
	}
	return l.epoch
}

// Valid reports whether l is open and bound to a live device at its current epoch.
func (l *BindGroupLayout) Valid() bool {
	return l != nil && !l.closed && l.dev != nil && l.dev.alive() && l.epoch == l.dev.generation
}

// Close releases the bind group layout immediately.
func (l *BindGroupLayout) Close() error {
	if l == nil || l.closed {
		return nil
	}
	l.closed = true
	if l.dev != nil {
		l.dev.forgetRetainedLayout(l)
	}
	if l.dev != nil && l.dev.alive() && l.epoch == l.dev.generation {
		l.dev.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpDestroyBindGroupLayout, ID: l.id})
	}
	return nil
}

// BindGroup binds buffers, textures, and samplers for one shader @group(G).
type BindGroup struct {
	dev         *Device
	id          driver.GPUID
	epoch       uint32
	layout      *BindGroupLayout
	entries     []BindGroupEntry
	retained    bool
	closed      bool
	retainedIdx int
}

// ID returns the bind group's numeric handle.
func (g *BindGroup) ID() uint64 { return uint64(g.id) }

// Epoch returns the device epoch this bind group currently belongs to.
func (g *BindGroup) Epoch() uint32 {
	if g == nil {
		return 0
	}
	return g.epoch
}

// Valid reports whether g is open and bound to a live device at its current epoch.
func (g *BindGroup) Valid() bool {
	return g != nil && !g.closed && g.dev != nil && g.dev.alive() && g.epoch == g.dev.generation
}

// Close releases the bind group immediately.
func (g *BindGroup) Close() error {
	if g == nil || g.closed {
		return nil
	}
	g.closed = true
	if g.dev != nil {
		g.dev.forgetRetainedBindGroup(g)
	}
	if g.dev != nil && g.dev.alive() && g.epoch == g.dev.generation {
		g.dev.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpDestroyBindGroup, ID: g.id})
	}
	return nil
}

// keepRetainedLayout records a layout in the device's restore list, whose index it remembers so a close removes it in constant time.
func (d *Device) keepRetainedLayout(l *BindGroupLayout) {
	d.retainedMu.Lock()
	l.retainedIdx = len(d.retainedLayouts)
	d.retainedLayouts = append(d.retainedLayouts, l)
	d.retainedMu.Unlock()
}

// forgetRetainedLayout drops a closed layout from the device's restore list, so the layout and what it held become unreachable without a restore.
func (d *Device) forgetRetainedLayout(l *BindGroupLayout) {
	d.retainedMu.Lock()
	defer d.retainedMu.Unlock()
	i := l.retainedIdx
	if i < 0 || i >= len(d.retainedLayouts) || d.retainedLayouts[i] != l {
		return
	}
	last := len(d.retainedLayouts) - 1
	d.retainedLayouts[i] = d.retainedLayouts[last]
	d.retainedLayouts[last] = nil
	d.retainedLayouts = d.retainedLayouts[:last]
	if i != last {
		d.retainedLayouts[i].retainedIdx = i
	}
	l.retainedIdx = -1
}

// keepRetainedBindGroup records a bind group in the device's restore list, whose index it remembers so a close removes it in constant time.
func (d *Device) keepRetainedBindGroup(g *BindGroup) {
	d.retainedMu.Lock()
	g.retainedIdx = len(d.retainedBindGroups)
	d.retainedBindGroups = append(d.retainedBindGroups, g)
	d.retainedMu.Unlock()
}

// forgetRetainedBindGroup drops a closed bind group from the device's restore list, so the group, its entries and the resources they bound become unreachable without a restore.
func (d *Device) forgetRetainedBindGroup(g *BindGroup) {
	d.retainedMu.Lock()
	defer d.retainedMu.Unlock()
	i := g.retainedIdx
	if i < 0 || i >= len(d.retainedBindGroups) || d.retainedBindGroups[i] != g {
		return
	}
	last := len(d.retainedBindGroups) - 1
	d.retainedBindGroups[i] = d.retainedBindGroups[last]
	d.retainedBindGroups[last] = nil
	d.retainedBindGroups = d.retainedBindGroups[:last]
	if i != last {
		d.retainedBindGroups[i].retainedIdx = i
	}
	g.retainedIdx = -1
}

// Pipeline is an immutable vertex+fragment GPU render pipeline.
type Pipeline struct {
	dev        *Device
	id         driver.GPUID
	epoch      uint32
	desc       PipelineDesc
	cacheKey   string
	retained   bool
	ready      atomic.Bool
	done       chan struct{}
	compileErr error
	closed     bool
}

// ID returns the pipeline's numeric handle.
func (p *Pipeline) ID() uint64 { return uint64(p.id) }

// Epoch returns the device epoch this pipeline currently belongs to.
func (p *Pipeline) Epoch() uint32 {
	if p == nil {
		return 0
	}
	return p.epoch
}

// Valid reports whether p is open and bound to a live device at its current epoch.
func (p *Pipeline) Valid() bool {
	return p != nil && !p.closed && p.dev != nil && p.dev.alive() && p.epoch == p.dev.generation
}

// Ready reports whether p has finished compiling (for asynchronous precompilation) and is valid for binding in a RenderPass.
func (p *Pipeline) Ready() bool {
	return p.Valid() && p.ready.Load()
}

// CacheKey returns the deterministic pipeline cache key computed from the pipeline's shaders and render state.
func (p *Pipeline) CacheKey() string {
	if p == nil {
		return ""
	}
	return p.cacheKey
}

// Err returns the asynchronous compilation error, if any, once Done is closed.
func (p *Pipeline) Err() error {
	if p == nil {
		return nil
	}
	select {
	case <-p.done:
		return p.compileErr
	default:
		return nil
	}
}

// Done returns a channel that closes when asynchronous pipeline compilation completes (or is already closed for synchronously created pipelines).
func (p *Pipeline) Done() <-chan struct{} {
	if p == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return p.done
}

// Close releases the pipeline immediately.
func (p *Pipeline) Close() error {
	if p == nil || p.closed {
		return nil
	}
	p.closed = true
	p.ready.Store(false)
	if p.dev != nil && p.dev.alive() && p.epoch == p.dev.generation {
		p.dev.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpDestroyPipeline, ID: p.id})
	}
	return nil
}

// TimestampQuery collects per-pass GPU timing measurements for render passes that reference it via RenderPassDesc.TimestampQuery.
type TimestampQuery struct {
	dev     *Device
	id      driver.GPUID
	label   string
	timings []PassTiming
	closed  bool
}

// ID returns the query's numeric handle.
func (q *TimestampQuery) ID() uint64 { return uint64(q.id) }

// Label returns the query's default timing label.
func (q *TimestampQuery) Label() string { return q.label }

// Timings returns a copy of all completed pass timings recorded into q.
func (q *TimestampQuery) Timings() []PassTiming {
	if q == nil {
		return nil
	}
	return slices.Clone(q.timings)
}

// LastDuration returns the most recently recorded pass duration, or 0 if none.
func (q *TimestampQuery) LastDuration() time.Duration {
	if q == nil || len(q.timings) == 0 {
		return 0
	}
	return q.timings[len(q.timings)-1].Duration
}

// Reset clears previously accumulated timing entries from q.
func (q *TimestampQuery) Reset() {
	if q != nil {
		q.timings = q.timings[:0]
	}
}

// Close releases the timestamp query. Safe to call more than once.
func (q *TimestampQuery) Close() error {
	if q == nil || q.closed {
		return nil
	}
	q.closed = true
	if q.dev != nil {
		delete(q.dev.timestampQueries, q.id)
	}
	return nil
}

// CreateTimestampQuery allocates a GPU timestamp query scoped to cx. Returns ErrUnsupported when Device.Capabilities().TimestampQueries is false.
func (d *Device) CreateTimestampQuery(cx *core.Scope, label string) (*TimestampQuery, error) {
	if !d.alive() {
		return nil, Error{Code: ErrDeviceLost, Message: "device is lost or disposed"}
	}
	if !d.features.TimestampQueries {
		return nil, Error{Code: ErrUnsupported, Message: "GPU timestamp queries are not supported on this device"}
	}
	id := d.nextID()
	q := &TimestampQuery{dev: d, id: id, label: label}
	d.timestampQueries[id] = q
	d.bindCleanup(cx, func() { _ = q.Close() })
	return q, nil
}

// ComputePipeline is an immutable GPU compute shader pipeline.
type ComputePipeline struct {
	dev    *Device
	id     driver.GPUID
	epoch  uint32
	desc   ComputePipelineDesc
	closed bool
}

// ID returns the compute pipeline's numeric handle.
func (p *ComputePipeline) ID() uint64 { return uint64(p.id) }

// Valid reports whether p is open and bound to a live device at its current epoch. Compute pipelines are not restored after device loss; recreate them when Valid returns false.
func (p *ComputePipeline) Valid() bool {
	return p != nil && !p.closed && p.dev != nil && p.dev.alive() && p.epoch == p.dev.generation
}

// Close releases the compute pipeline immediately. Safe to call more than once.
func (p *ComputePipeline) Close() error {
	if p == nil || p.closed {
		return nil
	}
	p.closed = true
	if p.dev != nil && p.dev.alive() && p.epoch == p.dev.generation {
		p.dev.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpDestroyComputePipeline, ID: p.id})
	}
	return nil
}

// --- Resource constructors with synchronous validation ---

// CreateBuffer allocates a retained GPU buffer scoped to cx.
func (d *Device) CreateBuffer(cx *core.Scope, desc BufferDesc) (*Buffer, error) {
	if !d.alive() {
		return nil, Error{Code: ErrDeviceLost, Message: "device is lost or disposed"}
	}
	if desc.Usage == 0 {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "buffer requires at least one Usage flag"}
	}
	if desc.Usage&UsageStorage != 0 && !d.features.StorageBuffers {
		return nil, Error{Code: ErrUnsupported, Message: "storage buffers are not supported on this device"}
	}
	size := desc.Size
	if size == 0 {
		size = uint64(len(desc.Data))
	}
	if size == 0 {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "buffer size must be greater than zero"}
	}
	if uint64(len(desc.Data)) > size {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "buffer initial Data exceeds Size"}
	}
	if d.limits.MaxBufferSize > 0 && size > d.limits.MaxBufferSize {
		return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("buffer size %d exceeds limit %d", size, d.limits.MaxBufferSize)}
	}
	id := d.nextID()
	dataCopy := slices.Clone(desc.Data)
	dd := driver.GPUBufferDesc{
		Label: debugLabel(desc.Label),
		Size:  size,
		Usage: driver.GPUBufferUsage(desc.Usage),
		Data:  dataCopy,
	}
	d.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpCreateBuffer, ID: id, Buffer: &dd})
	retained := desc.Retained || d.retainResources
	var shadow []byte
	if retained {
		shadow = make([]byte, size)
		copy(shadow, desc.Data)
	}
	buf := &Buffer{
		dev:      d,
		id:       id,
		epoch:    d.generation,
		label:    desc.Label,
		size:     size,
		usage:    desc.Usage,
		retained: retained,
		shadow:   shadow,
	}
	if retained {
		d.retainedBuffers = append(d.retainedBuffers, buf)
	}
	d.bindCleanup(cx, func() { _ = buf.Close() })
	return buf, nil
}

// CreateTexture allocates a retained GPU texture scoped to cx.
func (d *Device) CreateTexture(cx *core.Scope, desc TextureDesc) (*Texture, error) {
	if !d.alive() {
		return nil, Error{Code: ErrDeviceLost, Message: "device is lost or disposed"}
	}
	if desc.Width == 0 || desc.Height == 0 {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "texture width and height must be positive"}
	}
	if desc.Format == FormatUndefined {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "texture format must be specified"}
	}
	if desc.Usage == 0 {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "texture requires at least one Usage flag"}
	}
	if desc.Usage&TextureStorageBinding != 0 && !d.features.StorageTextures {
		return nil, Error{Code: ErrUnsupported, Message: "storage textures are not supported on this device"}
	}
	if d.limits.MaxTextureDimension2D > 0 && (desc.Width > d.limits.MaxTextureDimension2D || desc.Height > d.limits.MaxTextureDimension2D) {
		return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("texture %dx%d exceeds limit %d", desc.Width, desc.Height, d.limits.MaxTextureDimension2D)}
	}
	if desc.Format.IsCompressed() {
		if !d.SupportsCompressedFormat(desc.Format) {
			return nil, Error{Code: ErrUnsupported, Message: fmt.Sprintf("compressed texture format %s is not supported on this device", desc.Format)}
		}
		if desc.Usage&TextureRenderAttachment != 0 {
			return nil, Error{Code: ErrInvalidDescriptor, Message: "compressed textures cannot have TextureRenderAttachment usage"}
		}
		if desc.SampleCount > 1 {
			return nil, Error{Code: ErrInvalidDescriptor, Message: "compressed textures cannot be multisampled"}
		}
		bw, bh := desc.Format.BlockSize()
		if bw == 0 || bh == 0 || desc.Width%bw != 0 || desc.Height%bh != 0 {
			return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("compressed texture dimensions %dx%d must be multiples of block size %dx%d", desc.Width, desc.Height, bw, bh)}
		}
	}
	if desc.Layers == 0 {
		if desc.Dimension == DimCube {
			desc.Layers = 6
		} else {
			desc.Layers = 1
		}
	}
	switch desc.Dimension {
	case Dim2D:
		if desc.Layers != 1 {
			return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("2D texture must have 1 layer, got %d (use Dim2DArray)", desc.Layers)}
		}
	case DimCube:
		if desc.Width != desc.Height {
			return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("cubemap texture must be square, got %dx%d", desc.Width, desc.Height)}
		}
		if desc.Layers != 6 {
			return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("cubemap texture must have 6 layers, got %d", desc.Layers)}
		}
	case Dim2DArray:
		if d.limits.MaxTextureArrayLayers > 0 && desc.Layers > d.limits.MaxTextureArrayLayers {
			return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("texture array layers %d exceed limit %d", desc.Layers, d.limits.MaxTextureArrayLayers)}
		}
	default:
		return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("unsupported texture dimension %d", desc.Dimension)}
	}
	if desc.MipLevels == 0 {
		desc.MipLevels = 1
	}
	if maxMips := MaxMipLevels(desc.Width, desc.Height); desc.MipLevels > maxMips {
		return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("texture mip levels %d exceed maximum %d for %dx%d", desc.MipLevels, maxMips, desc.Width, desc.Height)}
	}
	if desc.SampleCount <= 0 {
		desc.SampleCount = 1
	}
	if desc.SampleCount != 1 && desc.SampleCount != 4 {
		return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("unsupported texture SampleCount %d (must be 1 or 4)", desc.SampleCount)}
	}
	if desc.SampleCount == 4 {
		if !d.features.MSAA4x {
			return nil, Error{Code: ErrUnsupported, Message: "4x MSAA textures are not supported on this device"}
		}
		if desc.Dimension != Dim2D || desc.MipLevels != 1 || desc.Layers != 1 {
			return nil, Error{Code: ErrInvalidDescriptor, Message: "MSAA textures must be single-layer Dim2D with MipLevels == 1"}
		}
		if desc.Usage&TextureRenderAttachment == 0 {
			return nil, Error{Code: ErrInvalidDescriptor, Message: "MSAA textures require TextureRenderAttachment usage"}
		}
	}
	if !d.SupportsFormat(desc.Format) {
		return nil, Error{Code: ErrUnsupported, Message: fmt.Sprintf("texture format %s is not supported on this device", desc.Format)}
	}
	lbl := debugLabel(desc.Label)
	desc.Label = lbl
	id := d.nextID()
	dd := driver.GPUTextureDesc{
		Label:       lbl,
		Dimension:   driver.GPUTextureDimension(desc.Dimension),
		Width:       desc.Width,
		Height:      desc.Height,
		Layers:      desc.Layers,
		MipLevels:   desc.MipLevels,
		SampleCount: desc.SampleCount,
		Format:      driver.GPUTextureFormat(desc.Format),
		Usage:       driver.GPUTextureUsage(desc.Usage),
	}
	d.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpCreateTexture, ID: id, Texture: &dd})
	retained := desc.Retained || d.retainResources
	tex := &Texture{dev: d, id: id, epoch: d.generation, desc: desc, retained: retained}
	if retained {
		d.retainedTextures = append(d.retainedTextures, tex)
	}
	d.bindCleanup(cx, func() { _ = tex.Close() })
	return tex, nil
}

// CreateTextureFromImage allocates a 2D texture from an ImageSource with Pixels and uploads its RGBA pixels.
func (d *Device) CreateTextureFromImage(cx *core.Scope, src core.ImageSource, desc TextureDesc) (*Texture, error) {
	if src.Pixels == nil {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "CreateTextureFromImage requires ImageSource.Pixels"}
	}
	if desc.Format.IsCompressed() {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "CreateTextureFromImage requires an uncompressed format"}
	}
	b := src.Pixels.Bounds()
	if desc.Width == 0 {
		desc.Width = uint32(b.Dx())
	}
	if desc.Height == 0 {
		desc.Height = uint32(b.Dy())
	}
	if desc.Format == FormatUndefined {
		desc.Format = FormatRGBA8Unorm
	}
	if desc.Usage == 0 {
		desc.Usage = TextureSampled | TextureCopyDst
	}
	tex, err := d.CreateTexture(cx, desc)
	if err != nil {
		return nil, err
	}
	d.WriteTextureRGBA(tex, 0, 0, src.Pixels)
	return tex, nil
}

// CreateRenderTarget allocates an offscreen render target (color-only, color+depth, depth-only shadow map, or depth/stencil) whose attachments can be rendered to and sampled by subsequent passes, scoped to cx. With SampleCount 4 the color texture is multisampled and a single-sampled Resolve texture is created for sampling; pass Resolve as the color attachment's ResolveTarget (or call Encoder.ResolveTexture) to fill it.
func (d *Device) CreateRenderTarget(cx *core.Scope, desc RenderTargetDesc) (*RenderTarget, error) {
	if desc.ColorFormat == FormatUndefined && desc.DepthFormat == FormatUndefined {
		desc.ColorFormat = FormatRGBA8Unorm
	}
	samples := max(desc.SampleCount, 1)
	var colorTex, resolveTex *Texture
	closeAll := func() {
		for _, t := range []*Texture{colorTex, resolveTex} {
			if t != nil {
				_ = t.Close()
			}
		}
	}
	if desc.ColorFormat != FormatUndefined {
		if desc.ColorFormat.IsDepthStencil() {
			return nil, Error{Code: ErrInvalidDescriptor, Message: "RenderTargetDesc.ColorFormat must not be a depth or stencil format"}
		}
		colorUsage := TextureRenderAttachment | TextureSampled | TextureCopySrc
		if samples > 1 {
			colorUsage = TextureRenderAttachment
		}
		var err error
		colorTex, err = d.CreateTexture(cx, TextureDesc{
			Label:       debugLabelJoin(desc.Label, ".color"),
			Dimension:   Dim2D,
			Width:       desc.Width,
			Height:      desc.Height,
			SampleCount: samples,
			Format:      desc.ColorFormat,
			Usage:       colorUsage,
			Retained:    desc.Retained,
		})
		if err != nil {
			return nil, err
		}
		if samples > 1 {
			resolveTex, err = d.CreateTexture(cx, TextureDesc{
				Label:     debugLabelJoin(desc.Label, ".resolve"),
				Dimension: Dim2D,
				Width:     desc.Width,
				Height:    desc.Height,
				Format:    desc.ColorFormat,
				Usage:     TextureRenderAttachment | TextureSampled | TextureCopySrc,
				Retained:  desc.Retained,
			})
			if err != nil {
				closeAll()
				return nil, err
			}
		}
	}
	var depthTex *Texture
	if desc.DepthFormat != FormatUndefined {
		if !desc.DepthFormat.IsDepthStencil() {
			closeAll()
			return nil, Error{Code: ErrInvalidDescriptor, Message: "RenderTargetDesc.DepthFormat must be a depth or stencil format"}
		}
		depthUsage := TextureRenderAttachment | TextureSampled | TextureCopySrc
		if samples > 1 {
			depthUsage = TextureRenderAttachment
		}
		var err error
		depthTex, err = d.CreateTexture(cx, TextureDesc{
			Label:       debugLabelJoin(desc.Label, ".depth"),
			Dimension:   Dim2D,
			Width:       desc.Width,
			Height:      desc.Height,
			SampleCount: samples,
			Format:      desc.DepthFormat,
			Usage:       depthUsage,
			Retained:    desc.Retained,
		})
		if err != nil {
			closeAll()
			return nil, err
		}
	}
	return &RenderTarget{Color: colorTex, Resolve: resolveTex, Depth: depthTex, Width: desc.Width, Height: desc.Height}, nil
}

// CreateSampler allocates a retained GPU sampler scoped to cx.
func (d *Device) CreateSampler(cx *core.Scope, desc SamplerDesc) (*Sampler, error) {
	if !d.alive() {
		return nil, Error{Code: ErrDeviceLost, Message: "device is lost or disposed"}
	}
	if desc.MinLOD < 0 {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "sampler MinLOD must be non-negative"}
	}
	if desc.MaxLOD < 0 {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "sampler MaxLOD must be non-negative"}
	}
	if desc.MaxLOD == 0 {
		desc.MaxLOD = 32.0
	}
	if desc.MaxLOD < desc.MinLOD {
		return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("sampler MaxLOD (%g) must be >= MinLOD (%g)", desc.MaxLOD, desc.MinLOD)}
	}
	lbl := debugLabel(desc.Label)
	desc.Label = lbl
	id := d.nextID()
	dd := driver.GPUSamplerDesc{
		Label:         lbl,
		MinFilter:     driver.GPUFilterMode(desc.MinFilter),
		MagFilter:     driver.GPUFilterMode(desc.MagFilter),
		MipmapFilter:  driver.GPUFilterMode(desc.MipmapFilter),
		AddressU:      driver.GPUAddressMode(desc.AddressU),
		AddressV:      driver.GPUAddressMode(desc.AddressV),
		AddressW:      driver.GPUAddressMode(desc.AddressW),
		MinLOD:        desc.MinLOD,
		MaxLOD:        desc.MaxLOD,
		Compare:       driver.GPUCompareFunc(desc.Compare),
		MaxAnisotropy: desc.MaxAnisotropy,
	}
	d.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpCreateSampler, ID: id, Sampler: &dd})
	retained := desc.Retained || d.retainResources
	s := &Sampler{dev: d, id: id, epoch: d.generation, desc: desc, retained: retained}
	if retained {
		d.retainedSamplers = append(d.retainedSamplers, s)
	}
	d.bindCleanup(cx, func() { _ = s.Close() })
	return s, nil
}

// CreateBindGroupLayout defines the binding schema for one @group(G) slot.
func (d *Device) CreateBindGroupLayout(cx *core.Scope, entries []BindGroupLayoutEntry) (*BindGroupLayout, error) {
	return d.CreateBindGroupLayoutDesc(cx, BindGroupLayoutDesc{Entries: entries})
}

// CreateBindGroupLayoutDesc defines a labeled binding schema for one @group(G) slot.
func (d *Device) CreateBindGroupLayoutDesc(cx *core.Scope, desc BindGroupLayoutDesc) (*BindGroupLayout, error) {
	if !d.alive() {
		return nil, Error{Code: ErrDeviceLost, Message: "device is lost or disposed"}
	}
	entries := desc.Entries
	seen := map[uint32]bool{}
	dEntries := make([]driver.GPUBindGroupLayoutEntry, len(entries))
	for i, e := range entries {
		if seen[e.Binding] {
			return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("duplicate binding %d in BindGroupLayout", e.Binding)}
		}
		seen[e.Binding] = true
		if e.HasDynamicOffset && e.Type != BindingUniform && e.Type != BindingStorageRead {
			return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d HasDynamicOffset is only valid for uniform or storage buffer bindings", e.Binding)}
		}
		if (e.Type == BindingStorage || e.Type == BindingStorageRead) && !d.features.StorageBuffers {
			return nil, Error{Code: ErrUnsupported, Message: fmt.Sprintf("binding %d requires storage buffers, which are unsupported on this device", e.Binding)}
		}
		if e.Type == BindingStorage && !d.features.Compute {
			return nil, Error{Code: ErrUnsupported, Message: fmt.Sprintf("binding %d requires read/write storage buffers, which require compute support", e.Binding)}
		}
		if e.Type == BindingStorageTexture && !d.features.StorageTextures {
			return nil, Error{Code: ErrUnsupported, Message: fmt.Sprintf("binding %d requires storage textures, which are unsupported on this device", e.Binding)}
		}
		if e.Visibility == StageCompute && !d.features.Compute {
			return nil, Error{Code: ErrUnsupported, Message: fmt.Sprintf("binding %d targets compute stage, which is unsupported on this device", e.Binding)}
		}
		vis := e.Visibility
		if vis == 0 {
			vis = StageAll
		}
		dEntries[i] = driver.GPUBindGroupLayoutEntry{
			Binding:          e.Binding,
			Visibility:       driver.GPUShaderStageFlags(vis),
			Type:             driver.GPUBindingType(e.Type),
			HasDynamicOffset: e.HasDynamicOffset,
		}
	}
	id := d.nextID()
	dd := driver.GPUBindGroupLayoutDesc{Label: debugLabel(desc.Label), Entries: dEntries}
	d.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpCreateBindGroupLayout, ID: id, BindGroupLayout: &dd})
	l := &BindGroupLayout{
		dev:         d,
		id:          id,
		epoch:       d.generation,
		entries:     slices.Clone(entries),
		label:       desc.Label,
		retained:    desc.Retained || d.retainResources,
		retainedIdx: -1,
	}
	d.keepRetainedLayout(l)
	d.bindCleanup(cx, func() { _ = l.Close() })
	return l, nil
}

// CreateBindGroup binds resources matching layout, scoped to cx.
func (d *Device) CreateBindGroup(cx *core.Scope, layout *BindGroupLayout, entries []BindGroupEntry) (*BindGroup, error) {
	return d.CreateBindGroupDesc(cx, BindGroupDesc{Layout: layout, Entries: entries})
}

// CreateBindGroupDesc binds resources matching desc.Layout, with an optional debug label stripped in release builds, scoped to cx.
func (d *Device) CreateBindGroupDesc(cx *core.Scope, desc BindGroupDesc) (*BindGroup, error) {
	if !d.alive() {
		return nil, Error{Code: ErrDeviceLost, Message: "device is lost or disposed"}
	}
	layout := desc.Layout
	entries := desc.Entries
	if layout != nil && (layout.closed || layout.dev != d) {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "BindGroupLayout belongs to another or closed device"}
	}
	layoutMap := map[uint32]BindGroupLayoutEntry{}
	if layout != nil {
		for _, le := range layout.entries {
			layoutMap[le.Binding] = le
		}
	}
	dEntries := make([]driver.GPUBindGroupEntry, len(entries))
	for i, e := range entries {
		if layout != nil {
			le, ok := layoutMap[e.Binding]
			if !ok {
				return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d not present in BindGroupLayout", e.Binding)}
			}
			switch le.Type {
			case BindingUniform, BindingStorageRead, BindingStorage:
				if e.Buffer == nil || e.Buffer.closed || e.Buffer.dev != d {
					return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d expects a live Buffer", e.Binding)}
				}
				if le.Type == BindingUniform && e.Buffer.usage&UsageUniform == 0 {
					return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d buffer lacks UsageUniform", e.Binding)}
				}
				if (le.Type == BindingStorageRead || le.Type == BindingStorage) && e.Buffer.usage&UsageStorage == 0 {
					return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d buffer lacks UsageStorage", e.Binding)}
				}
				if e.Offset > e.Buffer.size {
					return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d buffer offset out of range", e.Binding)}
				}
				if le.Type == BindingUniform {
					if d.limits.MinUniformBufferAlignment > 0 && e.Offset%uint64(d.limits.MinUniformBufferAlignment) != 0 {
						return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d uniform buffer offset %d is not aligned to %d bytes", e.Binding, e.Offset, d.limits.MinUniformBufferAlignment)}
					}
					bindSize := e.Size
					if bindSize == 0 {
						bindSize = e.Buffer.size - e.Offset
					}
					if d.limits.MaxUniformBufferSize > 0 {
						if e.Size > 0 && bindSize > d.limits.MaxUniformBufferSize {
							return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d uniform range size %d exceeds limit %d", e.Binding, bindSize, d.limits.MaxUniformBufferSize)}
						}
						if e.Size == 0 && !le.HasDynamicOffset && bindSize > d.limits.MaxUniformBufferSize {
							return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d uniform buffer size %d exceeds limit %d (set Size with HasDynamicOffset for ring buffers)", e.Binding, bindSize, d.limits.MaxUniformBufferSize)}
						}
					}
				}
				if e.Size > 0 && e.Offset+e.Size > e.Buffer.size {
					return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d buffer range out of bounds", e.Binding)}
				}
				if (le.Type == BindingStorageRead || le.Type == BindingStorage) && d.limits.MaxStorageBufferBindingSize > 0 {
					bindSize := e.Size
					if bindSize == 0 {
						bindSize = e.Buffer.size - e.Offset
					}
					if bindSize > d.limits.MaxStorageBufferBindingSize {
						return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d storage range %d exceeds limit %d", e.Binding, bindSize, d.limits.MaxStorageBufferBindingSize)}
					}
				}
			case BindingTexture:
				if e.Texture == nil || e.Texture.closed || e.Texture.dev != d {
					return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d expects a live Texture", e.Binding)}
				}
				if e.Texture.desc.Usage&TextureSampled == 0 {
					return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d texture lacks TextureSampled usage", e.Binding)}
				}
			case BindingStorageTexture:
				if e.Texture == nil || e.Texture.closed || e.Texture.dev != d {
					return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d expects a live storage Texture", e.Binding)}
				}
				if e.Texture.desc.Usage&TextureStorageBinding == 0 {
					return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d texture lacks TextureStorageBinding", e.Binding)}
				}
			case BindingSampler:
				if e.Sampler == nil || e.Sampler.closed || e.Sampler.dev != d {
					return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d expects a live Sampler", e.Binding)}
				}
				if e.Sampler.desc.Compare != CompareUndefined {
					return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d expects a non-comparison Sampler (Compare must be CompareUndefined)", e.Binding)}
				}
			case BindingComparisonSampler:
				if e.Sampler == nil || e.Sampler.closed || e.Sampler.dev != d {
					return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d expects a live comparison Sampler", e.Binding)}
				}
				if e.Sampler.desc.Compare == CompareUndefined {
					return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d comparison sampler requires a non-zero Compare function", e.Binding)}
				}
			}
		}
		de := driver.GPUBindGroupEntry{Binding: e.Binding, Offset: e.Offset, Size: e.Size}
		if e.Buffer != nil {
			de.Buffer = e.Buffer.id
			if de.Size == 0 {
				de.Size = e.Buffer.size - e.Offset
				if layout != nil {
					if le, ok := layoutMap[e.Binding]; ok && le.Type == BindingUniform && le.HasDynamicOffset && d.limits.MaxUniformBufferSize > 0 && de.Size > d.limits.MaxUniformBufferSize {
						de.Size = d.limits.MaxUniformBufferSize
					}
				}
			}
		}
		if e.Texture != nil {
			de.Texture = e.Texture.id
		}
		if e.Sampler != nil {
			de.Sampler = e.Sampler.id
		}
		dEntries[i] = de
	}
	id := d.nextID()
	var layoutID driver.GPUID
	if layout != nil {
		layoutID = layout.id
	}
	dd := driver.GPUBindGroupDesc{Label: debugLabel(desc.Label), Layout: layoutID, Entries: dEntries}
	d.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpCreateBindGroup, ID: id, BindGroup: &dd})
	bg := &BindGroup{
		dev:         d,
		id:          id,
		epoch:       d.generation,
		layout:      layout,
		entries:     slices.Clone(entries),
		retained:    d.retainResources,
		retainedIdx: -1,
	}
	d.keepRetainedBindGroup(bg)
	d.bindCleanup(cx, func() { _ = bg.Close() })
	return bg, nil
}

func (d *Device) preparePipelineDesc(desc PipelineDesc) (driver.GPUPipelineDesc, string, error) {
	if !d.alive() {
		return driver.GPUPipelineDesc{}, "", Error{Code: ErrDeviceLost, Message: "device is lost or disposed"}
	}
	if desc.Vertex.Module == nil || !desc.Vertex.Module.HasVertexEntry(desc.Vertex.EntryPoint) {
		return driver.GPUPipelineDesc{}, "", Error{Code: ErrInvalidDescriptor, Message: "pipeline requires a valid Vertex shader stage"}
	}
	if desc.Fragment.Module != nil && !desc.Fragment.Module.HasFragmentEntry(desc.Fragment.EntryPoint) {
		return driver.GPUPipelineDesc{}, "", Error{Code: ErrInvalidDescriptor, Message: "pipeline Fragment entry point not found in module"}
	}
	if d.limits.MaxVertexBuffers > 0 && uint32(len(desc.VertexBuffers)) > d.limits.MaxVertexBuffers {
		return driver.GPUPipelineDesc{}, "", Error{Code: ErrInvalidDescriptor, Message: "too many vertex buffers in pipeline"}
	}
	if d.limits.MaxBindGroups > 0 && uint32(len(desc.Layouts)) > d.limits.MaxBindGroups {
		return driver.GPUPipelineDesc{}, "", Error{Code: ErrInvalidDescriptor, Message: "too many bind group layouts in pipeline"}
	}
	var totalAttrs uint32
	dVBs := make([]driver.GPUVertexBufferLayout, len(desc.VertexBuffers))
	for i, vb := range desc.VertexBuffers {
		if vb.StepMode != StepVertex && vb.StepMode != StepInstance {
			return driver.GPUPipelineDesc{}, "", Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("vertex buffer %d has invalid StepMode %d", i, vb.StepMode)}
		}
		if vb.StepMode == StepVertex && vb.StepRate > 1 {
			return driver.GPUPipelineDesc{}, "", Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("vertex buffer %d StepRate %d requires StepInstance", i, vb.StepRate)}
		}
		stepRate := vb.StepRate
		if vb.StepMode == StepInstance && stepRate == 0 {
			stepRate = 1
		}
		attrs := make([]driver.GPUVertexAttribute, len(vb.Attributes))
		for j, a := range vb.Attributes {
			sz := driver.GPUVertexFormat(a.Format).ByteSize()
			if sz == 0 {
				return driver.GPUPipelineDesc{}, "", Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("invalid vertex attribute format %d", a.Format)}
			}
			if vb.Stride > 0 && a.Offset+sz > vb.Stride {
				return driver.GPUPipelineDesc{}, "", Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("vertex attribute at location %d exceeds buffer stride %d", a.Location, vb.Stride)}
			}
			attrs[j] = driver.GPUVertexAttribute{
				Format:   driver.GPUVertexFormat(a.Format),
				Offset:   a.Offset,
				Location: a.Location,
			}
			totalAttrs++
		}
		dVBs[i] = driver.GPUVertexBufferLayout{
			Stride:     vb.Stride,
			StepMode:   driver.GPUVertexStepMode(vb.StepMode),
			StepRate:   stepRate,
			Attributes: attrs,
		}
	}
	if d.limits.MaxVertexAttributes > 0 && totalAttrs > d.limits.MaxVertexAttributes {
		return driver.GPUPipelineDesc{}, "", Error{Code: ErrInvalidDescriptor, Message: "too many vertex attributes in pipeline"}
	}

	layoutIDs := make([]driver.GPUID, len(desc.Layouts))
	for i, l := range desc.Layouts {
		if l == nil || !l.Valid() || l.dev != d {
			return driver.GPUPipelineDesc{}, "", Error{Code: ErrInvalidDescriptor, Message: "pipeline BindGroupLayout is nil or invalid"}
		}
		layoutIDs[i] = l.id
	}

	samples := max(desc.SampleCount, 1)
	if samples != 1 && samples != 4 {
		return driver.GPUPipelineDesc{}, "", Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("unsupported pipeline SampleCount %d (must be 1 or 4)", samples)}
	}
	if samples == 4 && !d.features.MSAA4x {
		return driver.GPUPipelineDesc{}, "", Error{Code: ErrUnsupported, Message: "4x MSAA pipelines are not supported on this device"}
	}
	if uint32(len(desc.Targets)) > max(d.limits.MaxColorAttachments, 1) {
		return driver.GPUPipelineDesc{}, "", Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("pipeline has %d color targets, exceeding limit %d", len(desc.Targets), max(d.limits.MaxColorAttachments, 1))}
	}

	dTargets := make([]driver.GPUColorTargetState, len(desc.Targets))
	for i, t := range desc.Targets {
		fmtVal := t.Format
		if fmtVal == FormatUndefined {
			fmtVal = d.surfaceFormat
		}
		if !d.SupportsFormat(fmtVal) {
			return driver.GPUPipelineDesc{}, "", Error{Code: ErrUnsupported, Message: fmt.Sprintf("color target format %s is not supported on this device", fmtVal)}
		}
		var blend *driver.GPUBlendState
		if t.Blend != nil {
			blend = &driver.GPUBlendState{
				Color: driver.GPUBlendComponent{
					SrcFactor: driver.GPUBlendFactor(t.Blend.Color.SrcFactor),
					DstFactor: driver.GPUBlendFactor(t.Blend.Color.DstFactor),
					Operation: driver.GPUBlendOperation(t.Blend.Color.Operation),
				},
				Alpha: driver.GPUBlendComponent{
					SrcFactor: driver.GPUBlendFactor(t.Blend.Alpha.SrcFactor),
					DstFactor: driver.GPUBlendFactor(t.Blend.Alpha.DstFactor),
					Operation: driver.GPUBlendOperation(t.Blend.Alpha.Operation),
				},
			}
		}
		mask := t.WriteMask
		if mask == 0 {
			mask = ColorWriteAll
		}
		dTargets[i] = driver.GPUColorTargetState{
			Format:    driver.GPUTextureFormat(fmtVal),
			Blend:     blend,
			WriteMask: driver.GPUColorWriteMask(mask),
		}
	}

	var dDS *driver.GPUDepthStencilState
	if desc.DepthStencil != nil {
		if desc.DepthStencil.Format != FormatUndefined && !desc.DepthStencil.Format.IsDepthStencil() {
			return driver.GPUPipelineDesc{}, "", Error{Code: ErrInvalidDescriptor, Message: "DepthStencil.Format must be a depth or stencil format"}
		}
		if f := desc.DepthStencil.Format; f != FormatUndefined && !d.SupportsFormat(f) {
			return driver.GPUPipelineDesc{}, "", Error{Code: ErrUnsupported, Message: fmt.Sprintf("depth/stencil format %s is not supported on this device", f)}
		}
		if desc.DepthStencil.usesStencilTest() && !d.features.StencilTest {
			return driver.GPUPipelineDesc{}, "", Error{Code: ErrUnsupported, Message: "pipeline stencil testing is not supported on this device"}
		}
		cmp := desc.DepthStencil.DepthCompare
		if cmp == CompareUndefined {
			cmp = CompareLess
		}
		dDS = &driver.GPUDepthStencilState{
			Format:       driver.GPUTextureFormat(desc.DepthStencil.Format),
			DepthWrite:   desc.DepthStencil.DepthWrite,
			DepthCompare: driver.GPUCompareFunc(cmp),
			StencilFront: driver.GPUStencilFaceState{
				Compare:     driver.GPUCompareFunc(desc.DepthStencil.StencilFront.Compare),
				FailOp:      driver.GPUStencilOperation(desc.DepthStencil.StencilFront.FailOp),
				DepthFailOp: driver.GPUStencilOperation(desc.DepthStencil.StencilFront.DepthFailOp),
				PassOp:      driver.GPUStencilOperation(desc.DepthStencil.StencilFront.PassOp),
			},
			StencilBack: driver.GPUStencilFaceState{
				Compare:     driver.GPUCompareFunc(desc.DepthStencil.StencilBack.Compare),
				FailOp:      driver.GPUStencilOperation(desc.DepthStencil.StencilBack.FailOp),
				DepthFailOp: driver.GPUStencilOperation(desc.DepthStencil.StencilBack.DepthFailOp),
				PassOp:      driver.GPUStencilOperation(desc.DepthStencil.StencilBack.PassOp),
			},
			StencilReadMask:     desc.DepthStencil.StencilReadMask,
			StencilWriteMask:    desc.DepthStencil.StencilWriteMask,
			DepthBias:           desc.DepthStencil.DepthBias,
			DepthBiasSlopeScale: desc.DepthStencil.DepthBiasSlopeScale,
			DepthBiasClamp:      desc.DepthStencil.DepthBiasClamp,
		}
	}

	fragStage := driver.GPUShaderStage{}
	if desc.Fragment.Module != nil {
		fragStage = driver.GPUShaderStage{
			Label:      debugLabel(desc.Fragment.Module.label),
			IR:         desc.Fragment.Module.ir,
			EntryPoint: desc.Fragment.EntryPoint,
		}
	}
	lbl := debugLabel(desc.Label)
	desc.Label = lbl
	dd := driver.GPUPipelineDesc{
		Label: lbl,
		Vertex: driver.GPUShaderStage{
			Label:      debugLabel(desc.Vertex.Module.label),
			IR:         desc.Vertex.Module.ir,
			EntryPoint: desc.Vertex.EntryPoint,
		},
		Fragment:      fragStage,
		Layouts:       layoutIDs,
		VertexBuffers: dVBs,
		Primitive: driver.GPUPrimitiveState{
			Topology:  driver.GPUPrimitiveTopology(desc.Primitive.Topology),
			FrontFace: driver.GPUFrontFace(desc.Primitive.FrontFace),
			CullMode:  driver.GPUCullMode(desc.Primitive.CullMode),
		},
		DepthStencil: dDS,
		Targets:      dTargets,
		SampleCount:  max(desc.SampleCount, 1),
	}
	key := computePipelineCacheKey(desc, dd)
	dd.CacheKey = key
	return dd, key, nil
}

// CreatePipeline compiles and validates an immutable render pipeline scoped to cx. When a PipelineCache is attached to d, compiled target shader artifacts are reused across launches without recompilation.
func (d *Device) CreatePipeline(cx *core.Scope, desc PipelineDesc) (*Pipeline, error) {
	dd, key, err := d.preparePipelineDesc(desc)
	if err != nil {
		return nil, err
	}
	if d.cache != nil {
		art, err := compileOrLoadPipelineArtifacts(d.cache, key, dd)
		if err != nil {
			return nil, err
		}
		dd.CachedArtifacts = art
	}
	id := d.nextID()
	d.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpCreatePipeline, ID: id, Pipeline: &dd})
	done := make(chan struct{})
	close(done)
	retained := desc.Retained || d.retainResources
	p := &Pipeline{
		dev:      d,
		id:       id,
		epoch:    d.generation,
		desc:     desc,
		cacheKey: key,
		retained: retained,
		done:     done,
	}
	p.ready.Store(true)
	if retained {
		d.retainedPipelines = append(d.retainedPipelines, p)
	}
	d.bindCleanup(cx, func() { _ = p.Close() })
	return p, nil
}

// CreatePipelineAsync validates desc synchronously and compiles target shader artifacts asynchronously off the engine goroutine without blocking frames. The returned *Pipeline starts with Ready() == false and flips to Ready() == true once compilation finishes and the pipeline is submitted to the backend; onReady then runs on the engine goroutine (or synchronously on a standalone renderer).
func (d *Device) CreatePipelineAsync(cx *core.Scope, desc PipelineDesc, onReady func(*Pipeline, error)) *Pipeline {
	postScope := cx
	if postScope == nil {
		postScope = d.scope
	}
	dd, key, err := d.preparePipelineDesc(desc)
	if err != nil {
		if onReady != nil {
			if postScope != nil {
				postScope.Post(func() { onReady(nil, err) })
			} else {
				onReady(nil, err)
			}
		}
		return nil
	}
	id := d.nextID()
	retained := desc.Retained || d.retainResources
	p := &Pipeline{
		dev:      d,
		id:       id,
		epoch:    d.generation,
		desc:     desc,
		cacheKey: key,
		retained: retained,
		done:     make(chan struct{}),
	}
	if retained {
		d.retainedPipelines = append(d.retainedPipelines, p)
	}
	d.bindCleanup(cx, func() { _ = p.Close() })

	cache := d.cache
	gen := d.generation
	go func() {
		art, compileErr := compileOrLoadPipelineArtifacts(cache, key, dd)
		finish := func() {
			if p.closed || !d.alive() || gen != d.generation {
				errOut := compileErr
				if errOut == nil {
					errOut = Error{Code: ErrDeviceLost, Message: "device was lost or pipeline closed during compilation"}
				}
				p.compileErr = errOut
				if onReady != nil {
					onReady(nil, errOut)
				}
				close(p.done)
				return
			}
			if compileErr != nil {
				p.compileErr = compileErr
				if onReady != nil {
					onReady(nil, compileErr)
				}
				close(p.done)
				return
			}
			dd.CachedArtifacts = art
			d.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpCreatePipeline, ID: p.id, Pipeline: &dd})
			d.flushOpsOnly()
			p.ready.Store(true)
			if onReady != nil {
				onReady(p, nil)
			}
			close(p.done)
		}
		if postScope != nil {
			postScope.Post(finish)
		} else {
			finish()
		}
	}()
	return p
}

// PrecompilePipelines compiles a batch of pipelines asynchronously ahead of first draw without blocking the engine goroutine. It returns the handles in descriptor order immediately; onDone fires once all pipelines have finished compiling (with the first error encountered, if any).
func (d *Device) PrecompilePipelines(cx *core.Scope, descs []PipelineDesc, onDone func([]*Pipeline, error)) []*Pipeline {
	postScope := cx
	if postScope == nil {
		postScope = d.scope
	}
	if len(descs) == 0 {
		if onDone != nil {
			if postScope != nil {
				postScope.Post(func() { onDone(nil, nil) })
			} else {
				onDone(nil, nil)
			}
		}
		return nil
	}
	pipes := make([]*Pipeline, len(descs))
	var (
		mu        sync.Mutex
		firstErr  error
		remaining atomic.Int64
	)
	remaining.Store(int64(len(descs)))
	for i, desc := range descs {
		idx := i
		pipes[idx] = d.CreatePipelineAsync(cx, desc, func(p *Pipeline, err error) {
			mu.Lock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			if p != nil {
				pipes[idx] = p
			}
			errSnap := firstErr
			mu.Unlock()
			if remaining.Add(-1) == 0 && onDone != nil {
				onDone(slices.Clone(pipes), errSnap)
			}
		})
	}
	return pipes
}

// restoreRetainedFrom recreates all open retained resources from a previous lost Device onto d in dependency order (buffers -> textures -> samplers -> bind group layouts -> bind groups -> pipelines) so apps opting into retention continue rendering seamlessly after device or surface restoration.
func (d *Device) restoreRetainedFrom(prev *Device) {
	prev.retainedMu.Lock()
	prevLayouts := slices.Clone(prev.retainedLayouts)
	prevBindGroups := slices.Clone(prev.retainedBindGroups)
	prev.retainedMu.Unlock()
	if d == nil || prev == nil || !d.alive() {
		return
	}
	restoredBufs := map[*Buffer]bool{}
	for _, b := range prev.retainedBuffers {
		if b == nil || b.closed || !b.retained {
			continue
		}
		id := d.nextID()
		dd := driver.GPUBufferDesc{
			Label: b.label,
			Size:  b.size,
			Usage: driver.GPUBufferUsage(b.usage),
			Data:  slices.Clone(b.shadow),
		}
		d.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpCreateBuffer, ID: id, Buffer: &dd})
		b.dev = d
		b.id = id
		b.epoch = d.generation
		d.retainedBuffers = append(d.retainedBuffers, b)
		restoredBufs[b] = true
	}

	restoredTexs := map[*Texture]bool{}
	for _, t := range prev.retainedTextures {
		if t == nil || t.closed || !t.retained {
			continue
		}
		id := d.nextID()
		dd := driver.GPUTextureDesc{
			Label:       t.desc.Label,
			Dimension:   driver.GPUTextureDimension(t.desc.Dimension),
			Width:       t.desc.Width,
			Height:      t.desc.Height,
			Layers:      t.desc.Layers,
			MipLevels:   t.desc.MipLevels,
			SampleCount: t.desc.SampleCount,
			Format:      driver.GPUTextureFormat(t.desc.Format),
			Usage:       driver.GPUTextureUsage(t.desc.Usage),
		}
		d.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpCreateTexture, ID: id, Texture: &dd})
		for _, w := range t.shadowWrites {
			d.enqueueOp(driver.GPUResourceOp{
				Kind: driver.GPUOpWriteTexture,
				ID:   id,
				Data: slices.Clone(w.data),
				TextureDst: driver.GPUTextureCopy{
					Texture:    id,
					MipLevel:   w.dst.MipLevel,
					ArrayLayer: w.dst.ArrayLayer,
					OriginX:    w.dst.OriginX,
					OriginY:    w.dst.OriginY,
				},
				DataLayout: driver.GPUDataLayout{
					Offset:       w.layout.Offset,
					BytesPerRow:  w.layout.BytesPerRow,
					RowsPerImage: w.layout.RowsPerImage,
				},
			})
		}
		t.dev = d
		t.id = id
		t.epoch = d.generation
		d.retainedTextures = append(d.retainedTextures, t)
		restoredTexs[t] = true
	}

	restoredSamps := map[*Sampler]bool{}
	for _, s := range prev.retainedSamplers {
		if s == nil || s.closed || !s.retained {
			continue
		}
		id := d.nextID()
		dd := driver.GPUSamplerDesc{
			MinFilter:     driver.GPUFilterMode(s.desc.MinFilter),
			MagFilter:     driver.GPUFilterMode(s.desc.MagFilter),
			MipmapFilter:  driver.GPUFilterMode(s.desc.MipmapFilter),
			AddressU:      driver.GPUAddressMode(s.desc.AddressU),
			AddressV:      driver.GPUAddressMode(s.desc.AddressV),
			AddressW:      driver.GPUAddressMode(s.desc.AddressW),
			Compare:       driver.GPUCompareFunc(s.desc.Compare),
			MaxAnisotropy: s.desc.MaxAnisotropy,
			MinLOD:        s.desc.MinLOD,
			MaxLOD:        s.desc.MaxLOD,
		}
		d.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpCreateSampler, ID: id, Sampler: &dd})
		s.dev = d
		s.id = id
		s.epoch = d.generation
		d.retainedSamplers = append(d.retainedSamplers, s)
		restoredSamps[s] = true
	}

	// Determine which BindGroupLayouts are needed by retained bind groups or pipelines.
	neededLayouts := map[*BindGroupLayout]bool{}
	for _, p := range prev.retainedPipelines {
		if p == nil || p.closed || !p.retained {
			continue
		}
		for _, l := range p.desc.Layouts {
			if l != nil && !l.closed {
				neededLayouts[l] = true
			}
		}
	}
	canRestoreBG := func(bg *BindGroup) bool {
		if bg == nil || bg.closed {
			return false
		}
		if len(bg.entries) == 0 {
			return bg.retained || d.retainResources
		}
		hasBound := false
		for _, e := range bg.entries {
			if e.Buffer != nil {
				hasBound = true
				if !restoredBufs[e.Buffer] {
					return false
				}
			}
			if e.Texture != nil {
				hasBound = true
				if !restoredTexs[e.Texture] {
					return false
				}
			}
			if e.Sampler != nil {
				hasBound = true
				if !restoredSamps[e.Sampler] {
					return false
				}
			}
		}
		return hasBound || bg.retained || d.retainResources
	}
	for _, bg := range prevBindGroups {
		if canRestoreBG(bg) && bg.layout != nil && !bg.layout.closed {
			neededLayouts[bg.layout] = true
		}
	}

	restoredLayouts := map[*BindGroupLayout]bool{}
	for _, l := range prevLayouts {
		if l == nil || l.closed || (!l.retained && !d.retainResources && !neededLayouts[l]) {
			continue
		}
		dEntries := make([]driver.GPUBindGroupLayoutEntry, len(l.entries))
		for i, e := range l.entries {
			vis := e.Visibility
			if vis == 0 {
				vis = StageAll
			}
			dEntries[i] = driver.GPUBindGroupLayoutEntry{
				Binding:          e.Binding,
				Visibility:       driver.GPUShaderStageFlags(vis),
				Type:             driver.GPUBindingType(e.Type),
				HasDynamicOffset: e.HasDynamicOffset,
			}
		}
		id := d.nextID()
		dd := driver.GPUBindGroupLayoutDesc{Label: debugLabel(l.label), Entries: dEntries}
		d.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpCreateBindGroupLayout, ID: id, BindGroupLayout: &dd})
		l.dev = d
		l.id = id
		l.epoch = d.generation
		d.keepRetainedLayout(l)
		restoredLayouts[l] = true
	}

	for _, bg := range prevBindGroups {
		if !canRestoreBG(bg) {
			continue
		}
		if bg.layout != nil && !restoredLayouts[bg.layout] {
			continue
		}
		dEntries := make([]driver.GPUBindGroupEntry, len(bg.entries))
		for i, e := range bg.entries {
			de := driver.GPUBindGroupEntry{Binding: e.Binding, Offset: e.Offset, Size: e.Size}
			if e.Buffer != nil {
				de.Buffer = e.Buffer.id
				if de.Size == 0 {
					de.Size = e.Buffer.size - e.Offset
				}
			}
			if e.Texture != nil {
				de.Texture = e.Texture.id
			}
			if e.Sampler != nil {
				de.Sampler = e.Sampler.id
			}
			dEntries[i] = de
		}
		var layoutID driver.GPUID
		if bg.layout != nil {
			layoutID = bg.layout.id
		}
		id := d.nextID()
		dd := driver.GPUBindGroupDesc{Layout: layoutID, Entries: dEntries}
		d.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpCreateBindGroup, ID: id, BindGroup: &dd})
		bg.dev = d
		bg.id = id
		bg.epoch = d.generation
		d.keepRetainedBindGroup(bg)
	}

	for _, p := range prev.retainedPipelines {
		if p == nil || p.closed || !p.retained || !p.ready.Load() {
			continue
		}
		dd, key, err := d.preparePipelineDesc(p.desc)
		if err != nil {
			continue
		}
		if d.cache != nil {
			if art, err := compileOrLoadPipelineArtifacts(d.cache, key, dd); err == nil {
				dd.CachedArtifacts = art
			}
		}
		id := d.nextID()
		d.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpCreatePipeline, ID: id, Pipeline: &dd})
		p.dev = d
		p.id = id
		p.epoch = d.generation
		p.cacheKey = key
		p.ready.Store(true)
		d.retainedPipelines = append(d.retainedPipelines, p)
	}
}

// CreateComputePipeline compiles and validates an immutable compute pipeline scoped to cx. Returns ErrUnsupported on devices where SupportsCompute() is false (such as WebGL 2 and OpenGL ES 3.0).
func (d *Device) CreateComputePipeline(cx *core.Scope, desc ComputePipelineDesc) (*ComputePipeline, error) {
	if !d.alive() {
		return nil, Error{Code: ErrDeviceLost, Message: "device is lost or disposed"}
	}
	if !d.features.Compute {
		return nil, Error{Code: ErrUnsupported, Message: "compute shaders are not supported on this device (WebGL 2 / GLES 3.0 requires CPU or render-pass fallback)"}
	}
	if desc.Compute.Module == nil || !desc.Compute.Module.HasComputeEntry(desc.Compute.EntryPoint) {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "compute pipeline requires a valid @compute shader stage"}
	}
	ws := desc.Compute.Module.WorkgroupSize()
	if d.limits.MaxComputeWorkgroupSizeX > 0 && ws[0] > d.limits.MaxComputeWorkgroupSizeX {
		return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("workgroup_size.x %d exceeds limit %d", ws[0], d.limits.MaxComputeWorkgroupSizeX)}
	}
	if d.limits.MaxComputeWorkgroupSizeY > 0 && ws[1] > d.limits.MaxComputeWorkgroupSizeY {
		return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("workgroup_size.y %d exceeds limit %d", ws[1], d.limits.MaxComputeWorkgroupSizeY)}
	}
	if d.limits.MaxComputeWorkgroupSizeZ > 0 && ws[2] > d.limits.MaxComputeWorkgroupSizeZ {
		return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("workgroup_size.z %d exceeds limit %d", ws[2], d.limits.MaxComputeWorkgroupSizeZ)}
	}
	if d.limits.MaxComputeInvocationsPerGroup > 0 && uint64(ws[0])*uint64(ws[1])*uint64(ws[2]) > uint64(d.limits.MaxComputeInvocationsPerGroup) {
		return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("workgroup invocations %d exceed limit %d", uint64(ws[0])*uint64(ws[1])*uint64(ws[2]), d.limits.MaxComputeInvocationsPerGroup)}
	}
	if d.limits.MaxBindGroups > 0 && uint32(len(desc.Layouts)) > d.limits.MaxBindGroups {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "too many bind group layouts in compute pipeline"}
	}
	layoutIDs := make([]driver.GPUID, len(desc.Layouts))
	for i, l := range desc.Layouts {
		if l == nil || l.closed || l.dev != d {
			return nil, Error{Code: ErrInvalidDescriptor, Message: "compute pipeline BindGroupLayout is nil or invalid"}
		}
		layoutIDs[i] = l.id
	}
	entry := desc.Compute.EntryPoint
	if entry == "" && desc.Compute.Module.ir != nil {
		entry = desc.Compute.Module.ir.ComputeEntry
	}
	dd := driver.GPUComputePipelineDesc{
		Label: desc.Label,
		Compute: driver.GPUShaderStage{
			IR:         desc.Compute.Module.ir,
			EntryPoint: entry,
		},
		Layouts: layoutIDs,
	}
	id := d.nextID()
	d.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpCreateComputePipeline, ID: id, ComputePipeline: &dd})
	cp := &ComputePipeline{dev: d, id: id, epoch: d.generation, desc: desc}
	d.bindCleanup(cx, func() { _ = cp.Close() })
	return cp, nil
}

// --- Data uploads ---

// WriteBuffer copies data into buf starting at byte offset. Data is cloned before returning so callers may immediately reuse the slice.
func (d *Device) WriteBuffer(buf *Buffer, offset uint64, data []byte) {
	if !d.alive() || buf == nil || !buf.Valid() || buf.dev != d || len(data) == 0 {
		return
	}
	if offset+uint64(len(data)) > buf.size {
		return
	}
	if buf.retained && uint64(len(buf.shadow)) >= offset+uint64(len(data)) {
		copy(buf.shadow[offset:], data)
	}
	d.enqueueOp(driver.GPUResourceOp{
		Kind:   driver.GPUOpWriteBuffer,
		ID:     buf.id,
		Offset: offset,
		Data:   slices.Clone(data),
	})
}

// WriteTexture copies raw texel bytes into dst according to layout. The written width is BytesPerRow / bytes-per-texel (clamped to the mip edge), and the row count follows the data length and RowsPerImage, on every backend.
func (d *Device) WriteTexture(tex *Texture, dst TextureCopy, data []byte, layout DataLayout) {
	if !d.alive() || tex == nil || !tex.Valid() || tex.dev != d || tex.external || len(data) == 0 {
		return
	}
	cloned := slices.Clone(data)
	if tex.retained {
		// Replace any earlier write to the exact same subresource/origin to bound memory.
		replaced := false
		for i := range tex.shadowWrites {
			w := &tex.shadowWrites[i]
			if w.dst.MipLevel == dst.MipLevel && w.dst.ArrayLayer == dst.ArrayLayer &&
				w.dst.OriginX == dst.OriginX && w.dst.OriginY == dst.OriginY &&
				len(w.data) == len(cloned) {
				w.layout = layout
				w.data = slices.Clone(cloned)
				replaced = true
				break
			}
		}
		if !replaced {
			tex.shadowWrites = append(tex.shadowWrites, retainedTextureWrite{
				dst:    dst,
				layout: layout,
				data:   slices.Clone(cloned),
			})
		}
	}
	d.enqueueOp(driver.GPUResourceOp{
		Kind: driver.GPUOpWriteTexture,
		ID:   tex.id,
		Data: cloned,
		TextureDst: driver.GPUTextureCopy{
			Texture:    tex.id,
			MipLevel:   dst.MipLevel,
			ArrayLayer: dst.ArrayLayer,
			OriginX:    dst.OriginX,
			OriginY:    dst.OriginY,
		},
		DataLayout: driver.GPUDataLayout{
			Offset:       layout.Offset,
			BytesPerRow:  layout.BytesPerRow,
			RowsPerImage: layout.RowsPerImage,
		},
	})
}

// WriteTextureRegion copies raw texel bytes into a specific mip level, array layer (or cubemap face), and optional sub-rectangle of dst.Texture. Width and Height default to the rest of the mip level from the origin. The source rows (layout.BytesPerRow apart, starting at layout.Offset) are repacked into tight rows of the sub-rectangle width, so only the sub-rectangle is written on every backend. Compressed formats are forwarded to WriteTexture unchanged.
func (d *Device) WriteTextureRegion(dst TextureWrite, data []byte, layout DataLayout) {
	tex := dst.Texture
	if !d.alive() || tex == nil || !tex.Valid() || tex.dev != d || len(data) == 0 {
		return
	}
	if dst.MipLevel >= tex.desc.MipLevels || dst.ArrayLayer >= tex.desc.Layers {
		return
	}
	copyDst := TextureCopy{Texture: tex, MipLevel: dst.MipLevel, ArrayLayer: dst.ArrayLayer, OriginX: dst.OriginX, OriginY: dst.OriginY}
	bpp := driver.GPUTextureFormat(tex.desc.Format).BytesPerPixel()
	if bpp == 0 {
		d.WriteTexture(tex, copyDst, data, layout)
		return
	}
	mw, mh := MipSize(tex.desc.Width, tex.desc.Height, dst.MipLevel)
	if dst.OriginX >= mw || dst.OriginY >= mh {
		return
	}
	w := dst.Width
	if w == 0 || dst.OriginX+w > mw {
		w = mw - dst.OriginX
	}
	h := dst.Height
	if h == 0 || dst.OriginY+h > mh {
		h = mh - dst.OriginY
	}
	row := uint64(w) * uint64(bpp)
	stride := uint64(layout.BytesPerRow)
	if stride == 0 {
		stride = row
	}
	if stride < row || layout.Offset >= uint64(len(data)) {
		return
	}
	src := data[layout.Offset:]
	rows := uint64(h)
	if avail := (uint64(len(src)) - row) / stride; uint64(len(src)) < row {
		return
	} else if avail+1 < rows {
		rows = avail + 1
	}
	packed := make([]byte, 0, rows*row)
	for y := uint64(0); y < rows; y++ {
		packed = append(packed, src[y*stride:y*stride+row]...)
	}
	d.WriteTexture(tex, copyDst, packed, DataLayout{BytesPerRow: uint32(row), RowsPerImage: uint32(rows)})
}

// WriteTextureMip uploads packed texel bytes for an entire mip level and array layer (or cubemap face 0..5). If bytesPerRow is 0, rows are assumed tightly packed.
func (d *Device) WriteTextureMip(tex *Texture, mipLevel, arrayLayer uint32, data []byte, bytesPerRow uint32) {
	if tex == nil {
		return
	}
	mw, mh := MipSize(tex.desc.Width, tex.desc.Height, mipLevel)
	if bytesPerRow == 0 {
		bpp := driver.GPUTextureFormat(tex.desc.Format).BytesPerPixel()
		if bpp == 0 {
			bpp = 4
		}
		bytesPerRow = mw * bpp
	}
	d.WriteTextureRegion(TextureWrite{
		Texture:    tex,
		MipLevel:   mipLevel,
		ArrayLayer: arrayLayer,
		Width:      mw,
		Height:     mh,
	}, data, DataLayout{
		BytesPerRow:  bytesPerRow,
		RowsPerImage: mh,
	})
}

// WriteTextureRGBA uploads img's RGBA pixels into tex at mipLevel and arrayLayer.
func (d *Device) WriteTextureRGBA(tex *Texture, mipLevel, arrayLayer uint32, img *image.RGBA) {
	if img == nil || tex == nil {
		return
	}
	b := img.Bounds()
	d.WriteTextureRegion(TextureWrite{
		Texture:    tex,
		MipLevel:   mipLevel,
		ArrayLayer: arrayLayer,
		Width:      uint32(b.Dx()),
		Height:     uint32(b.Dy()),
	}, img.Pix, DataLayout{
		BytesPerRow:  uint32(img.Stride),
		RowsPerImage: uint32(b.Dy()),
	})
}

// WriteCubeFaceRGBA uploads img's RGBA pixels into one face (CubeFacePosX..CubeFaceNegZ) of a cubemap texture at mipLevel.
func (d *Device) WriteCubeFaceRGBA(tex *Texture, face CubeFace, mipLevel uint32, img *image.RGBA) {
	if tex == nil || tex.desc.Dimension != DimCube || face >= 6 {
		return
	}
	d.WriteTextureRGBA(tex, mipLevel, face, img)
}

// GenerateMipmaps populates mip levels 1..MipLevels-1 from level 0 for every array layer (or cubemap face) of tex using a 2x2 box filter. It returns ErrUnsupported where Features.MipmapGeneration is false (the native backends today); there, build the chain on the CPU with GenerateMipChainRGBA and upload each level with WriteTextureMip.
func (d *Device) GenerateMipmaps(tex *Texture) error {
	if !d.alive() {
		return Error{Code: ErrDeviceLost, Message: "device is lost or disposed"}
	}
	if !d.features.MipmapGeneration {
		return Error{Code: ErrUnsupported, Message: "mipmap generation is not supported on this device"}
	}
	if !tex.Valid() || tex.dev != d {
		return Error{Code: ErrInvalidDescriptor, Message: "GenerateMipmaps requires a live Texture"}
	}
	if tex.desc.Format.IsDepthStencil() || tex.desc.Format.IsCompressed() || tex.desc.SampleCount > 1 {
		return Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("GenerateMipmaps does not support %s textures with %d samples", tex.desc.Format, tex.desc.SampleCount)}
	}
	if tex.desc.MipLevels <= 1 {
		return nil
	}
	d.enqueueOp(driver.GPUResourceOp{
		Kind: driver.GPUOpGenerateMipmaps,
		ID:   tex.id,
	})
	return nil
}

// --- Command encoding, Frame, and RenderPass ---

// CommandBuffer is an immutable batch of recorded copy commands and render passes.
type CommandBuffer struct {
	dev      *Device
	epoch    uint32
	copies   []driver.GPUCopyOp
	passes   []driver.GPUPass
	debugOps []driver.GPUDebugOp
}

// Encoder records render passes and copy commands for one Device.Submit call.
type Encoder struct {
	dev      *Device
	epoch    uint32
	copies   []driver.GPUCopyOp
	passes   []driver.GPUPass
	debugOps []driver.GPUDebugOp
	finished bool
}

// BeginEncoder starts recording a new CommandBuffer.
func (d *Device) BeginEncoder() *Encoder {
	var epoch uint32
	if d != nil {
		epoch = d.generation
	}
	return &Encoder{dev: d, epoch: epoch}
}

// PushDebugGroup begins a nested debug group in GPU captures (Metal/RenderDoc/WebGPU). It is a zero-cost no-op in release builds (without nsgodev or nsgotest).
func (e *Encoder) PushDebugGroup(label string) { e.pushDebugGroup(label) }

// PopDebugGroup ends the most recent Encoder.PushDebugGroup. It is a zero-cost no-op in release builds (without nsgodev or nsgotest).
func (e *Encoder) PopDebugGroup() { e.popDebugGroup() }

// InsertDebugMarker inserts an instantaneous debug marker into GPU captures. It is a zero-cost no-op in release builds (without nsgodev or nsgotest).
func (e *Encoder) InsertDebugMarker(label string) { e.insertDebugMarker(label) }

// BeginRenderPass starts a render pass on e. Call RenderPass.End when done.
func (e *Encoder) BeginRenderPass(desc RenderPassDesc) *RenderPass {
	invalid := e == nil || e.finished || e.dev == nil || !e.dev.alive() || e.epoch != e.dev.generation
	var cas []driver.GPUColorAttachment
	if len(desc.ColorAttachments) == 0 && desc.DepthStencil == nil {
		cas = []driver.GPUColorAttachment{{
			LoadOp:  driver.GPULoadLoad,
			StoreOp: driver.GPUStoreStore,
		}}
	} else if len(desc.ColorAttachments) > 0 {
		if e != nil && e.dev != nil && uint32(len(desc.ColorAttachments)) > max(e.dev.limits.MaxColorAttachments, 1) {
			// More color attachments than the device can render to: drop the pass instead of handing the backend an attachment list it cannot bind.
			invalid = true
		}
		cas = make([]driver.GPUColorAttachment, len(desc.ColorAttachments))
		for i, ca := range desc.ColorAttachments {
			var tid, rid driver.GPUID
			if ca.Texture != nil {
				if !ca.Texture.Valid() || ca.Texture.external || ca.Texture.dev != e.dev || ca.Texture.epoch != e.epoch {
					invalid = true
				} else {
					tid = ca.Texture.id
				}
			}
			if ca.ResolveTarget != nil {
				if e == nil || !ca.ResolveTarget.Valid() || ca.ResolveTarget.dev != e.dev || ca.ResolveTarget.epoch != e.epoch {
					invalid = true
				} else {
					rid = ca.ResolveTarget.id
				}
			}
			cas[i] = driver.GPUColorAttachment{
				Texture:       tid,
				ResolveTarget: rid,
				MipLevel:      ca.MipLevel,
				ArrayLayer:    ca.ArrayLayer,
				LoadOp:        driver.GPULoadOp(ca.LoadOp),
				StoreOp:       driver.GPUStoreOp(ca.StoreOp),
				ClearColor:    ca.ClearColor,
			}
		}
	}
	var ds *driver.GPUDepthStencilAttachment
	if desc.DepthStencil != nil {
		var tid driver.GPUID
		if desc.DepthStencil.Texture != nil {
			if e == nil || !desc.DepthStencil.Texture.Valid() || desc.DepthStencil.Texture.dev != e.dev || desc.DepthStencil.Texture.epoch != e.epoch {
				invalid = true
			} else {
				tid = desc.DepthStencil.Texture.id
			}
		}
		ds = &driver.GPUDepthStencilAttachment{
			Texture:        tid,
			DepthLoadOp:    driver.GPULoadOp(desc.DepthStencil.DepthLoadOp),
			DepthStoreOp:   driver.GPUStoreOp(desc.DepthStencil.DepthStoreOp),
			ClearDepth:     desc.DepthStencil.ClearDepth,
			StencilLoadOp:  driver.GPULoadOp(desc.DepthStencil.StencilLoadOp),
			StencilStoreOp: driver.GPUStoreOp(desc.DepthStencil.StencilStoreOp),
			ClearStencil:   desc.DepthStencil.ClearStencil,
		}
	}
	var timingLabel string
	var queryID driver.GPUID
	if e != nil && e.dev != nil && e.dev.features.TimestampQueries {
		timingLabel = desc.TimingLabel
		if desc.TimestampQuery != nil && !desc.TimestampQuery.closed && desc.TimestampQuery.dev == e.dev {
			queryID = desc.TimestampQuery.id
			if timingLabel == "" {
				timingLabel = desc.TimestampQuery.label
			}
		}
	}
	formats := make([]TextureFormat, len(cas))
	samples := make([]int, len(cas))
	depth := FormatUndefined
	surface := FormatUndefined
	if !invalid && e != nil && e.dev != nil {
		surface = e.dev.SurfaceFormat()
	}
	for i := range cas {
		if len(desc.ColorAttachments) == 0 || desc.ColorAttachments[i].Texture == nil {
			// The swapchain's own attachment: its sample count belongs to the surface.
			formats[i], samples[i] = surface, -1
			continue
		}
		ca := desc.ColorAttachments[i]
		formats[i], samples[i] = ca.Texture.desc.Format, max(ca.Texture.desc.SampleCount, 1)
	}
	if desc.DepthStencil != nil && desc.DepthStencil.Texture != nil {
		depth = desc.DepthStencil.Texture.desc.Format
	}
	return &RenderPass{
		enc:          e,
		invalid:      invalid,
		colorFormats: formats,
		colorSamples: samples,
		depthFormat:  depth,
		pass: driver.GPUPass{
			Label:            debugLabel(desc.Label),
			TimingLabel:      timingLabel,
			TimestampQueryID: queryID,
			ColorAttachments: cas,
			DepthStencil:     ds,
		},
	}
}

// BeginComputePass starts a compute pass on e. Call ComputePass.End when done. On a device where SupportsCompute() is false the pass records nothing and Dispatch returns ErrUnsupported, so backends without compute never receive a compute pass.
func (e *Encoder) BeginComputePass() *ComputePass {
	p := &ComputePass{
		enc:  e,
		pass: driver.GPUPass{IsCompute: true},
	}
	if e == nil || e.finished || e.dev == nil || !e.dev.alive() || !e.dev.SupportsCompute() || e.epoch != e.dev.generation {
		p.invalid = true
	}
	return p
}

// ComputePass records compute pipeline state, bind groups, and workgroup dispatches for one compute pass.
type ComputePass struct {
	enc     *Encoder
	pass    driver.GPUPass
	ended   bool
	invalid bool
}

// SetPipeline binds the active compute pipeline. Closed pipelines and stale handles from a lost device are ignored.
func (p *ComputePass) SetPipeline(pipe *ComputePipeline) {
	if p == nil || p.ended || p.invalid || !pipe.Valid() || pipe.dev != p.enc.dev || pipe.epoch != p.enc.epoch {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind: driver.GPUCmdSetComputePipeline,
		ID:   pipe.id,
	})
}

// SetBindGroup binds group to @group(slot) with optional dynamic offsets.
func (p *ComputePass) SetBindGroup(slot uint32, group *BindGroup, dynamicOffsets ...uint32) {
	if p == nil || p.ended || p.invalid || group == nil || !group.Valid() || group.dev != p.enc.dev || group.epoch != p.enc.epoch {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind:           driver.GPUCmdSetBindGroup,
		Slot:           slot,
		ID:             group.id,
		DynamicOffsets: slices.Clone(dynamicOffsets),
	})
}

func (p *ComputePass) checkDispatch() error {
	if p == nil || p.ended || p.enc == nil || p.enc.dev == nil || p.enc.finished {
		return Error{Code: ErrInvalidDescriptor, Message: "compute pass is closed"}
	}
	dev := p.enc.dev
	if !dev.alive() || p.enc.epoch != dev.generation {
		return Error{Code: ErrDeviceLost, Message: "device is lost or disposed"}
	}
	if !dev.SupportsCompute() {
		return Error{Code: ErrUnsupported, Message: "compute dispatch is not supported on this device (WebGL 2 / GLES 3.0 requires CPU or render-pass fallback)"}
	}
	if p.invalid {
		return Error{Code: ErrInvalidDescriptor, Message: "compute pass is invalid"}
	}
	return nil
}

// Dispatch dispatches (x, y, z) compute workgroups. Returns ErrUnsupported when the device does not support compute shaders, or ErrInvalidDescriptor if the pass has ended or dispatch dimensions are invalid.
func (p *ComputePass) Dispatch(x, y, z uint32) error {
	if err := p.checkDispatch(); err != nil {
		return err
	}
	if x == 0 || y == 0 || z == 0 {
		return Error{Code: ErrInvalidDescriptor, Message: "dispatch workgroup counts (x, y, z) must be positive"}
	}
	maxDim := p.enc.dev.limits.MaxComputeWorkgroupsPerDim
	if maxDim > 0 && (x > maxDim || y > maxDim || z > maxDim) {
		return Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("dispatch (%d, %d, %d) exceeds max workgroups per dimension %d", x, y, z, maxDim)}
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind: driver.GPUCmdDispatch,
		U0:   x, U1: y, U2: z,
	})
	return nil
}

// DispatchIndirect dispatches workgroups whose (x, y, z) uint32 counts are read from buf at byte offset (12 bytes, 4-byte aligned). buf must have UsageIndirect.
func (p *ComputePass) DispatchIndirect(buf *Buffer, offset uint64) error {
	if err := p.checkDispatch(); err != nil {
		return err
	}
	dev := p.enc.dev
	if !buf.Valid() || buf.dev != dev || buf.epoch != p.enc.epoch {
		return Error{Code: ErrInvalidDescriptor, Message: "DispatchIndirect requires a live Buffer"}
	}
	if buf.usage&UsageIndirect == 0 {
		return Error{Code: ErrInvalidDescriptor, Message: "DispatchIndirect buffer lacks UsageIndirect"}
	}
	if offset%4 != 0 || offset+12 > buf.size {
		return Error{Code: ErrInvalidDescriptor, Message: "DispatchIndirect offset must be 4-byte aligned with at least 12 bytes remaining"}
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind:   driver.GPUCmdDispatchIndirect,
		ID:     buf.id,
		Offset: offset,
	})
	return nil
}

// End finishes the compute pass and appends it to its owning Encoder.
func (p *ComputePass) End() {
	if p == nil || p.ended {
		return
	}
	p.ended = true
	if !p.invalid && p.enc != nil && !p.enc.finished {
		p.enc.passes = append(p.enc.passes, p.pass)
	}
}

// CopyBufferToBuffer records a buffer-to-buffer copy.
func (e *Encoder) CopyBufferToBuffer(src *Buffer, srcOffset uint64, dst *Buffer, dstOffset, size uint64) {
	if e == nil || e.finished || e.dev == nil || !e.dev.alive() || size == 0 {
		return
	}
	if !src.Valid() || !dst.Valid() || src.dev != e.dev || dst.dev != e.dev || src.epoch != e.epoch || dst.epoch != e.epoch {
		return
	}
	e.copies = append(e.copies, driver.GPUCopyOp{
		Kind:      driver.GPUCopyBufferToBuffer,
		SrcID:     src.id,
		DstID:     dst.id,
		SrcOffset: srcOffset,
		DstOffset: dstOffset,
		Size:      size,
	})
}

// CopyBufferToTexture records a buffer-to-texture copy.
func (e *Encoder) CopyBufferToTexture(src BufferCopy, dst TextureCopy, width, height, layers uint32) {
	if e == nil || e.finished || e.dev == nil || !e.dev.alive() {
		return
	}
	if !src.Buffer.Valid() || !dst.Texture.Valid() || dst.Texture.external || src.Buffer.dev != e.dev || dst.Texture.dev != e.dev || src.Buffer.epoch != e.epoch || dst.Texture.epoch != e.epoch {
		return
	}
	e.copies = append(e.copies, driver.GPUCopyOp{
		Kind:   driver.GPUCopyBufferToTexture,
		SrcID:  src.Buffer.id,
		DstID:  dst.Texture.id,
		Width:  width,
		Height: height,
		Layers: max(layers, 1),
		SrcLayout: driver.GPUDataLayout{
			Offset:       src.Layout.Offset,
			BytesPerRow:  src.Layout.BytesPerRow,
			RowsPerImage: src.Layout.RowsPerImage,
		},
		DstTex: driver.GPUTextureCopy{
			Texture:    dst.Texture.id,
			MipLevel:   dst.MipLevel,
			ArrayLayer: dst.ArrayLayer,
			OriginX:    dst.OriginX,
			OriginY:    dst.OriginY,
		},
	})
}

// CopyTextureToTexture records a texture-to-texture copy.
func (e *Encoder) CopyTextureToTexture(src, dst TextureCopy, width, height, layers uint32) {
	if e == nil || e.finished || e.dev == nil || !e.dev.alive() {
		return
	}
	if !src.Texture.Valid() || !dst.Texture.Valid() || src.Texture.external || dst.Texture.external || src.Texture.dev != e.dev || dst.Texture.dev != e.dev || src.Texture.epoch != e.epoch || dst.Texture.epoch != e.epoch {
		return
	}
	e.copies = append(e.copies, driver.GPUCopyOp{
		Kind:   driver.GPUCopyTextureToTexture,
		SrcID:  src.Texture.id,
		DstID:  dst.Texture.id,
		Width:  width,
		Height: height,
		Layers: max(layers, 1),
		SrcTex: driver.GPUTextureCopy{
			Texture:    src.Texture.id,
			MipLevel:   src.MipLevel,
			ArrayLayer: src.ArrayLayer,
			OriginX:    src.OriginX,
			OriginY:    src.OriginY,
		},
		DstTex: driver.GPUTextureCopy{
			Texture:    dst.Texture.id,
			MipLevel:   dst.MipLevel,
			ArrayLayer: dst.ArrayLayer,
			OriginX:    dst.OriginX,
			OriginY:    dst.OriginY,
		},
	})
}

// ResolveTexture records a resolve of the multisampled src texture into the single-sampled dst texture (mip level 0, layer 0). It is recorded as a render pass that loads src and resolves it into dst at the end, so it runs in order with the encoder's other passes on every backend that supports MSAA resolve targets. Mismatched sizes or formats, or a src that is not multisampled, record nothing.
func (e *Encoder) ResolveTexture(src, dst TextureCopy) {
	if e == nil || e.finished || e.dev == nil || !e.dev.alive() {
		return
	}
	s, t := src.Texture, dst.Texture
	if !s.Valid() || !t.Valid() || s.dev != e.dev || t.dev != e.dev || s.epoch != e.epoch || t.epoch != e.epoch {
		return
	}
	if s.desc.SampleCount <= 1 || t.desc.SampleCount > 1 || s.desc.Format != t.desc.Format || s.desc.Width != t.desc.Width || s.desc.Height != t.desc.Height || dst.MipLevel != 0 || dst.ArrayLayer != 0 {
		return
	}
	e.passes = append(e.passes, driver.GPUPass{
		Label: debugLabel("ResolveTexture"),
		ColorAttachments: []driver.GPUColorAttachment{{
			Texture:       s.id,
			ResolveTarget: t.id,
			LoadOp:        driver.GPULoadLoad,
			StoreOp:       driver.GPUStoreStore,
		}},
	})
}

// Finish seals the encoder into an immutable CommandBuffer.
func (e *Encoder) Finish() *CommandBuffer {
	e.finished = true
	return &CommandBuffer{dev: e.dev, epoch: e.epoch, copies: e.copies, passes: e.passes, debugOps: e.debugOps}
}

// RenderPass records drawing state and draw calls for one render pass.
type RenderPass struct {
	enc       *Encoder
	pass      driver.GPUPass
	ended     bool
	invalid   bool
	pipeValid bool
	// pipe is the pipeline a draw records with, whose vertex and bind group slots a draw needs, and requiredGroups is the number of group slots it reads.
	pipe           *Pipeline
	requiredGroups int
	// vertexMissing and bindGroupsMissing hold the slots a rejected bind left without a usable resource, and indexMissing says the same of the index buffer; a draw skips only while one of the slots its pipeline declares is missing.
	vertexMissing     map[uint32]bool
	bindGroupsMissing map[uint32]bool
	indexMissing      bool
	// colorFormats and sampleSamples are the formats and sample counts of the pass's colour attachments, and depthFormat the depth attachment's, which a pipeline's targets must match. A swapchain attachment's sample count is -1, the surface's own.
	colorFormats []TextureFormat
	colorSamples []int
	depthFormat  TextureFormat
}

// PushDebugGroup begins a nested debug group inside this render pass. It is a zero-cost no-op in release builds (without nsgodev or nsgotest).
func (p *RenderPass) PushDebugGroup(label string) { p.pushDebugGroup(label) }

// PopDebugGroup closes the most recent RenderPass.PushDebugGroup. It is a zero-cost no-op in release builds (without nsgodev or nsgotest).
func (p *RenderPass) PopDebugGroup() { p.popDebugGroup() }

// InsertDebugMarker inserts an instantaneous debug marker inside this render pass. It is a zero-cost no-op in release builds (without nsgodev or nsgotest).
func (p *RenderPass) InsertDebugMarker(label string) { p.insertDebugMarker(label) }

// ClearColor configures color attachment 0 to clear to c at pass start.
func (p *RenderPass) ClearColor(c core.Color) {
	if p.ended || p.invalid || len(p.pass.ColorAttachments) == 0 {
		return
	}
	p.pass.ColorAttachments[0].LoadOp = driver.GPULoadClear
	p.pass.ColorAttachments[0].ClearColor = c
}

// ClearDepth configures the depth attachment to clear to depth at pass start.
func (p *RenderPass) ClearDepth(depth float32) {
	if p.ended || p.invalid {
		return
	}
	if p.pass.DepthStencil == nil {
		p.pass.DepthStencil = &driver.GPUDepthStencilAttachment{
			DepthStoreOp:   driver.GPUStoreStore,
			StencilStoreOp: driver.GPUStoreStore,
		}
	}
	p.pass.DepthStencil.DepthLoadOp = driver.GPULoadClear
	p.pass.DepthStencil.ClearDepth = depth
}

// ClearStencil configures the stencil attachment to clear to stencil at pass start.
func (p *RenderPass) ClearStencil(stencil uint32) {
	if p.ended {
		return
	}
	if p.pass.DepthStencil == nil {
		p.pass.DepthStencil = &driver.GPUDepthStencilAttachment{
			DepthStoreOp:   driver.GPUStoreStore,
			StencilStoreOp: driver.GPUStoreStore,
		}
	}
	p.pass.DepthStencil.StencilLoadOp = driver.GPULoadClear
	p.pass.DepthStencil.ClearStencil = stencil
}

// SetViewport sets the rasterizer viewport rectangle and depth range.
func (p *RenderPass) SetViewport(x, y, width, height, minDepth, maxDepth float32) {
	if p.ended || p.invalid {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind: driver.GPUCmdSetViewport,
		F0:   x, F1: y, F2: width, F3: height, F4: minDepth, F5: maxDepth,
	})
}

// SetScissor restricts rendering to the given pixel scissor rectangle.
func (p *RenderPass) SetScissor(x, y, width, height int32) {
	if p.ended || p.invalid {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind: driver.GPUCmdSetScissor,
		I0:   x, I1: y, I2: width, I3: height,
	})
}

// SetPipeline binds the active render pipeline. Stale handles from a lost device or pipelines still compiling asynchronously are safely ignored, and subsequent draws are skipped until a valid ready pipeline is bound. A pipeline whose colour targets do not fit the pass's attachments — another number of targets, another format, a sample count the attachment does not have, or a depth format the pass's depth attachment does not have — is ignored the same way, since the backends would refuse its draws.
func (p *RenderPass) SetPipeline(pipe *Pipeline) {
	if p.ended || p.invalid {
		return
	}
	if pipe == nil || !pipe.Ready() || p.enc == nil || pipe.dev != p.enc.dev || pipe.epoch != p.enc.epoch {
		p.pipeValid = false
		p.pipe = nil
		return
	}
	if !p.matchesPipeline(pipe) {
		// The strictest backends refuse a draw whose pipeline does not fit the pass's attachments; skip it here too, and leave a marker naming the mismatch.
		p.pipeValid = false
		p.enc.debugOps = append(p.enc.debugOps, driver.GPUDebugOp{Kind: driver.GPUDebugInsertMarker, Label: "pipeline targets do not match the render pass attachments", PassIndex: len(p.enc.passes)})
		return
	}
	p.pipeValid = true
	p.pipe = pipe
	p.requiredGroups = pipelineBindGroups(pipe)
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind: driver.GPUCmdSetPipeline,
		ID:   pipe.id,
	})
}

// SetBindGroup binds group to @group(slot) with optional dynamic offsets.
// matchesPipeline reports whether the pipeline's colour targets and depth format fit the pass's attachments: the same number of colour targets, the same formats and sample counts, and the same depth format where the pass has a depth attachment. A pipeline target left FormatUndefined takes the device's surface format, as CreatePipeline resolves it.
func (p *RenderPass) matchesPipeline(pipe *Pipeline) bool {
	targets := pipe.desc.Targets
	if len(targets) != len(p.colorFormats) {
		return false
	}
	for i, target := range targets {
		format := target.Format
		if format == FormatUndefined {
			format = p.enc.dev.SurfaceFormat()
		}
		if format != p.colorFormats[i] {
			return false
		}
		if n := p.colorSamples[i]; n > 0 && pipe.desc.SampleCount != 0 && pipe.desc.SampleCount != n {
			return false
		}
	}
	if p.depthFormat != FormatUndefined && (pipe.desc.DepthStencil == nil || pipe.desc.DepthStencil.Format != p.depthFormat) {
		return false
	}
	return true
}

func (p *RenderPass) SetBindGroup(slot uint32, group *BindGroup, dynamicOffsets ...uint32) {
	if p.ended || p.invalid {
		return
	}
	if group == nil || !group.Valid() || p.enc == nil || group.dev != p.enc.dev || group.epoch != p.enc.epoch {
		if p.bindGroupsMissing == nil {
			p.bindGroupsMissing = map[uint32]bool{}
		}
		p.bindGroupsMissing[slot] = true
		return
	}
	delete(p.bindGroupsMissing, slot)
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind:           driver.GPUCmdSetBindGroup,
		Slot:           slot,
		ID:             group.id,
		DynamicOffsets: slices.Clone(dynamicOffsets),
	})
}

// SetVertexBuffer binds buf to vertex buffer slot starting at byte offset.
func (p *RenderPass) SetVertexBuffer(slot uint32, buf *Buffer, offset uint64) {
	if p.ended || p.invalid {
		return
	}
	if buf == nil || !buf.Valid() || p.enc == nil || buf.dev != p.enc.dev || buf.epoch != p.enc.epoch {
		if p.vertexMissing == nil {
			p.vertexMissing = map[uint32]bool{}
		}
		p.vertexMissing[slot] = true
		return
	}
	delete(p.vertexMissing, slot)
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind:   driver.GPUCmdSetVertexBuffer,
		Slot:   slot,
		ID:     buf.id,
		Offset: offset,
	})
}

// SetIndexBuffer binds buf as the active index buffer.
func (p *RenderPass) SetIndexBuffer(buf *Buffer, format IndexFormat, offset uint64) {
	if p.ended || p.invalid {
		return
	}
	if buf == nil || !buf.Valid() || p.enc == nil || buf.dev != p.enc.dev || buf.epoch != p.enc.epoch {
		p.indexMissing = true
		return
	}
	p.indexMissing = false
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind:        driver.GPUCmdSetIndexBuffer,
		ID:          buf.id,
		IndexFormat: driver.GPUIndexFormat(format),
		Offset:      offset,
	})
}

// SetStencilReference sets the stencil reference value.
func (p *RenderPass) SetStencilReference(ref uint32) {
	if p.ended || p.invalid {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind: driver.GPUCmdSetStencilReference,
		U0:   ref,
	})
}

// SetBlendConstant sets the constant blend color.
func (p *RenderPass) SetBlendConstant(r, g, b, a float32) {
	if p.ended || p.invalid {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind: driver.GPUCmdSetBlendConstant,
		F0:   r, F1: g, F2: b, F3: a,
	})
}

// pipelineBindGroups returns the @group slots a pipeline reads: the layouts it declares, or every group its shaders bind when it declares fewer, as an automatic layout leaves them implicit.
func pipelineBindGroups(pipe *Pipeline) int {
	n := len(pipe.desc.Layouts)
	for _, stage := range []ShaderStage{pipe.desc.Vertex, pipe.desc.Fragment} {
		if stage.Module == nil || stage.Module.ir == nil {
			continue
		}
		for _, b := range stage.Module.ir.Bindings {
			if int(b.Group)+1 > n {
				n = int(b.Group) + 1
			}
		}
	}
	return n
}

// drawnBound reports whether the pass holds what its pipeline reads, so a draw records only when every vertex buffer slot and bind group slot the pipeline declares has a usable resource, plus the index buffer an indexed draw reads.
func (p *RenderPass) drawnBound(indexed bool) bool {
	if !p.pipeValid || p.pipe == nil {
		return false
	}
	for slot := range p.vertexMissing {
		if int(slot) < len(p.pipe.desc.VertexBuffers) {
			return false
		}
	}
	for slot := range p.bindGroupsMissing {
		if int(slot) < p.requiredGroups {
			return false
		}
	}
	return !indexed || !p.indexMissing
}

// Draw draws vertexCount non-indexed vertices starting at firstVertex.
func (p *RenderPass) Draw(vertexCount, firstVertex uint32) {
	if p.ended || p.invalid || !p.drawnBound(false) {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind: driver.GPUCmdDraw,
		U0:   vertexCount, U1: firstVertex,
	})
}

// DrawIndexed draws indexCount indexed vertices.
func (p *RenderPass) DrawIndexed(indexCount, firstIndex uint32, baseVertex int32) {
	if p.ended || p.invalid || !p.drawnBound(true) {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind: driver.GPUCmdDrawIndexed,
		U0:   indexCount, U1: firstIndex, I0: baseVertex,
	})
}

// DrawInstanced draws instanceCount instances of vertexCount vertices.
func (p *RenderPass) DrawInstanced(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	if p.ended || p.invalid || !p.drawnBound(false) {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind: driver.GPUCmdDrawInstanced,
		U0:   vertexCount, U1: instanceCount, U2: firstVertex, U3: firstInstance,
	})
}

// DrawIndexedInstanced draws instanceCount instances of indexCount indexed vertices.
func (p *RenderPass) DrawIndexedInstanced(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	if p.ended || p.invalid || !p.drawnBound(true) {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind: driver.GPUCmdDrawIndexedInstanced,
		U0:   indexCount, U1: instanceCount, U2: firstIndex, U3: firstInstance, I0: baseVertex,
	})
}

// End finishes the render pass and appends it to its owning Encoder.
func (p *RenderPass) End() {
	if p.ended {
		return
	}
	p.ended = true
	if !p.invalid && p.enc != nil && !p.enc.finished {
		p.enc.passes = append(p.enc.passes, p.pass)
	}
}

// Submit hands pending resource ops, flushed ring-buffer slices, and cmds
// directly to the platform backend off-commit (zero UI view-tree commits per frame).
// Submissions are suppressed while the device is lost or suspended (app in background).
func (d *Device) Submit(cmds ...*CommandBuffer) {
	if !d.alive() || d.suspended.Load() {
		if d != nil && d.inFrame {
			if d.activeFence != nil {
				d.activeFence.signal()
				d.activeFence = nil
			}
			d.inFrame = false
		}
		return
	}
	var (
		frameIdx  uint64
		frameSlot int
		fence     *FrameFence
	)
	if d.inFrame {
		frameIdx = d.activeFrameIndex
		frameSlot = d.activeFrameSlot
		fence = d.activeFence
		d.inFrame = false
		d.activeFence = nil
		d.frameIndex++
	} else {
		frameIdx = d.frameIndex
		frameSlot = int(frameIdx % uint64(d.FramesInFlight()))
		fence = newFrameFence(fenceIDGen.Add(1), frameIdx, frameSlot)
	}
	d.flushRingBuffers(frameSlot)

	d.opMu.Lock()
	ops := d.pendingOps
	d.pendingOps = nil
	d.opMu.Unlock()

	frame := driver.GPUFrame{
		Generation: d.generation,
		FrameIndex: frameIdx,
		FrameSlot:  frameSlot,
		FenceID:    fence.id,
		Ops:        ops,
		Present:    len(cmds) > 0,
	}
	for _, cb := range cmds {
		if cb == nil || cb.dev != d || cb.epoch != d.generation {
			continue
		}
		passOffset := len(frame.Passes)
		appendFrameDebugOps(&frame, cb.debugOps, passOffset)
		frame.Copies = append(frame.Copies, cb.copies...)
		frame.Passes = append(frame.Passes, cb.passes...)
	}
	if len(frame.Ops) == 0 && len(frame.Copies) == 0 && len(frame.Passes) == 0 && len(frame.DebugOps) == 0 {
		fence.signal()
		d.slotFences[frameSlot] = fence
		return
	}
	d.registerFence(fence)
	d.slotFences[frameSlot] = fence
	fid := fence.id
	frame.OnComplete = func() {
		d.completeFence(fid)
	}
	if d.submitFn == nil && d.surf == nil {
		d.completeFence(fid)
		return
	}
	d.dispatchFrame(frame)
}

// ReadTexture asynchronously copies region of tex back to CPU RGBA bytes and
// invokes fn on the engine goroutine.
func (d *Device) ReadTexture(tex *Texture, region TextureRegion, fn func(rgba []byte, err error)) {
	if !d.alive() {
		if fn != nil {
			fn(nil, Error{Code: ErrDeviceLost, Message: "device is lost or disposed"})
		}
		return
	}
	if tex == nil || !tex.Valid() || tex.dev != d {
		if fn != nil {
			fn(nil, Error{Code: ErrInvalidDescriptor, Message: "ReadTexture requires a live Texture"})
		}
		return
	}
	if tex.external {
		if fn != nil {
			fn(nil, Error{Code: ErrInvalidDescriptor, Message: "ReadTexture cannot read an imported texture; render it to a texture and read that"})
		}
		return
	}
	rid := readbackIDGen.Add(1)
	d.opMu.Lock()
	d.readbacks[rid] = fn
	d.pendingOps = append(d.pendingOps, driver.GPUResourceOp{
		Kind:       driver.GPUOpReadTexture,
		ID:         tex.id,
		ReadbackID: rid,
		ReadbackRegion: driver.GPUTextureRegion{
			MipLevel:   region.MipLevel,
			ArrayLayer: region.ArrayLayer,
			X:          region.X,
			Y:          region.Y,
			Width:      region.Width,
			Height:     region.Height,
		},
	})
	d.opMu.Unlock()
	d.flushOpsOnly()
}

// ReadBuffer asynchronously copies size bytes from buf starting at offset back to CPU memory and invokes fn on the engine goroutine. If size is 0, it reads from offset to the end of buf. buf must have UsageCopySrc. Buffer readback is part of the compute feature set: it returns ErrUnsupported when SupportsCompute() is false.
func (d *Device) ReadBuffer(buf *Buffer, offset, size uint64, fn func(data []byte, err error)) {
	if !d.alive() {
		if fn != nil {
			fn(nil, Error{Code: ErrDeviceLost, Message: "device is lost or disposed"})
		}
		return
	}
	if !d.features.Compute {
		if fn != nil {
			fn(nil, Error{Code: ErrUnsupported, Message: "buffer readback is not supported on this device"})
		}
		return
	}
	if !buf.Valid() || buf.dev != d {
		if fn != nil {
			fn(nil, Error{Code: ErrInvalidDescriptor, Message: "ReadBuffer requires a live Buffer"})
		}
		return
	}
	if buf.usage&UsageCopySrc == 0 {
		if fn != nil {
			fn(nil, Error{Code: ErrInvalidDescriptor, Message: "ReadBuffer requires UsageCopySrc on the buffer"})
		}
		return
	}
	if offset > buf.size {
		if fn != nil {
			fn(nil, Error{Code: ErrInvalidDescriptor, Message: "ReadBuffer offset out of bounds"})
		}
		return
	}
	if size == 0 {
		size = buf.size - offset
	}
	if offset+size > buf.size {
		if fn != nil {
			fn(nil, Error{Code: ErrInvalidDescriptor, Message: "ReadBuffer range out of bounds"})
		}
		return
	}
	rid := readbackIDGen.Add(1)
	d.opMu.Lock()
	d.readbacks[rid] = fn
	d.pendingOps = append(d.pendingOps, driver.GPUResourceOp{
		Kind:         driver.GPUOpReadBuffer,
		ID:           buf.id,
		Offset:       offset,
		ReadbackID:   rid,
		ReadbackSize: size,
	})
	d.opMu.Unlock()
	d.flushOpsOnly()
}

// ReadSurface asynchronously copies the view's current swapchain color buffer
// back to CPU RGBA bytes and invokes fn on the engine goroutine.
func (d *Device) ReadSurface(fn func(rgba []byte, err error)) {
	if !d.alive() {
		if fn != nil {
			fn(nil, Error{Code: ErrDeviceLost, Message: "device is lost or disposed"})
		}
		return
	}
	rid := readbackIDGen.Add(1)
	d.opMu.Lock()
	d.readbacks[rid] = fn
	d.pendingOps = append(d.pendingOps, driver.GPUResourceOp{
		Kind:            driver.GPUOpReadTexture,
		ReadbackID:      rid,
		ReadbackSurface: true,
	})
	d.opMu.Unlock()
	d.flushOpsOnly()
}

// Frame is the per-frame recording context handed to ViewProps.OnFrame or
// returned by Device.BeginFrame.
type Frame struct {
	dev        *Device
	enc        *Encoder
	at         time.Time
	delta      time.Duration
	frameIndex uint64
	frameSlot  int
	fence      *FrameFence
	submitted  bool
	ended      bool
}

// BeginFrame acquires the next ring-buffer slot (frame % FramesInFlight),
// waits on that slot's previous FrameFence so the CPU never overwrites memory
// still being read by the GPU, resets all live RingBuffers for that slot, and
// returns a Frame ready for recording.
func (d *Device) BeginFrame(at time.Time) *Frame {
	idx := d.frameIndex
	slot := int(idx % uint64(d.FramesInFlight()))
	if prev := d.SlotFence(slot); prev != nil {
		prev.Wait()
	}
	for rb := range d.ringBuffers {
		rb.resetSlot(slot)
	}
	fence := newFrameFence(fenceIDGen.Add(1), idx, slot)
	d.inFrame = true
	d.activeFrameIndex = idx
	d.activeFrameSlot = slot
	d.activeFence = fence
	return &Frame{
		dev:        d,
		enc:        d.BeginEncoder(),
		at:         at,
		frameIndex: idx,
		frameSlot:  slot,
		fence:      fence,
	}
}

// EndFrame finalizes f. If f has not yet been submitted and has recorded passes,
// copies, pending ops, or ring-buffer writes, EndFrame submits it; otherwise it
// completes f's frame slot and advances to the next slot cleanly. Safe to call
// more than once or after Frame.Submit.
func (d *Device) EndFrame(f *Frame) {
	if f == nil || f.ended {
		return
	}
	if !f.submitted {
		hasWork := len(f.enc.passes) > 0 || len(f.enc.copies) > 0 || len(d.pendingOps) > 0 || d.hasDirtyRingBuffers(f.frameSlot)
		if hasWork {
			f.Submit()
			return
		}
		f.submitted = true
		f.ended = true
		if d.inFrame && d.activeFence == f.fence {
			d.inFrame = false
			d.activeFence = nil
			d.frameIndex++
		}
		f.fence.signal()
		if f.frameSlot >= 0 && f.frameSlot < len(d.slotFences) {
			d.slotFences[f.frameSlot] = f.fence
		}
		return
	}
	f.ended = true
}

// Device returns the owning GPU Device.
// Device returns the device the encoder records for, or nil for a nil encoder.
func (e *Encoder) Device() *Device {
	if e == nil {
		return nil
	}
	return e.dev
}

// Device returns the device the pass records into, or nil for a nil pass.
func (p *RenderPass) Device() *Device {
	if p == nil {
		return nil
	}
	return p.enc.Device()
}

// Device returns the device the pass records into, or nil for a nil pass.
func (p *ComputePass) Device() *Device {
	if p == nil {
		return nil
	}
	return p.enc.Device()
}

func (f *Frame) Device() *Device { return f.dev }

// Time returns the frame's display timestamp.
func (f *Frame) Time() time.Time { return f.at }

// Delta returns the elapsed time since the previous frame.
func (f *Frame) Delta() time.Duration { return f.delta }

// Size returns the current swapchain dimensions in physical pixels.
func (f *Frame) Size() (width, height int) { return f.dev.SurfaceSize() }

// PassTimings returns the most recently completed GPU frame's per-pass timings (or the current frame's timings after Submit on synchronous software devices).
func (f *Frame) PassTimings() []PassTiming {
	if f == nil || f.dev == nil {
		return nil
	}
	return f.dev.LastPassTimings()
}

// PushDebugGroup begins a nested debug group on this frame's encoder. It is a zero-cost no-op in release builds (without nsgodev or nsgotest).
func (f *Frame) PushDebugGroup(label string) {
	if f != nil && f.enc != nil {
		f.enc.PushDebugGroup(label)
	}
}

// PopDebugGroup closes the most recent Frame.PushDebugGroup. It is a zero-cost no-op in release builds (without nsgodev or nsgotest).
func (f *Frame) PopDebugGroup() {
	if f != nil && f.enc != nil {
		f.enc.PopDebugGroup()
	}
}

// InsertDebugMarker inserts an instantaneous debug marker on this frame's encoder. It is a zero-cost no-op in release builds (without nsgodev or nsgotest).
func (f *Frame) InsertDebugMarker(label string) {
	if f != nil && f.enc != nil {
		f.enc.InsertDebugMarker(label)
	}
}

// FrameIndex returns this frame's monotonic frame index.
func (f *Frame) FrameIndex() uint64 { return f.frameIndex }

// FrameSlot returns this frame's ring-buffer slot index [0, FramesInFlight).
func (f *Frame) FrameSlot() int { return f.frameSlot }

// Fence returns the FrameFence signaled when the GPU finishes executing f.
func (f *Frame) Fence() *FrameFence { return f.fence }

// BeginRenderPass starts a RenderPass recorded into this frame.
func (f *Frame) BeginRenderPass(desc RenderPassDesc) *RenderPass {
	return f.enc.BeginRenderPass(desc)
}

// BeginComputePass starts a ComputePass recorded into this frame.
func (f *Frame) BeginComputePass() *ComputePass {
	return f.enc.BeginComputePass()
}

// CopyBufferToBuffer records a buffer copy into this frame.
func (f *Frame) CopyBufferToBuffer(src *Buffer, srcOffset uint64, dst *Buffer, dstOffset, size uint64) {
	f.enc.CopyBufferToBuffer(src, srcOffset, dst, dstOffset, size)
}

// CopyBufferToTexture records a buffer-to-texture copy into this frame.
func (f *Frame) CopyBufferToTexture(src BufferCopy, dst TextureCopy, width, height, layers uint32) {
	f.enc.CopyBufferToTexture(src, dst, width, height, layers)
}

// CopyTextureToTexture records a texture-to-texture copy into this frame.
func (f *Frame) CopyTextureToTexture(src, dst TextureCopy, width, height, layers uint32) {
	f.enc.CopyTextureToTexture(src, dst, width, height, layers)
}

// ResolveTexture explicitly resolves a multisampled texture into a single-sampled texture.
func (f *Frame) ResolveTexture(src, dst TextureCopy) {
	f.enc.ResolveTexture(src, dst)
}

// Submit submits this frame's recorded commands off-commit and ends the frame.
// Safe to call once; ViewProps.OnFrame automatically finishes the frame if the
// callback does not call Submit or End.
func (f *Frame) Submit() {
	if f == nil || f.submitted {
		return
	}
	f.submitted = true
	f.ended = true
	f.dev.Submit(f.enc.Finish())
}

// End finalizes this frame via Device.EndFrame.
func (f *Frame) End() {
	if f == nil || f.dev == nil {
		return
	}
	f.dev.EndFrame(f)
}
