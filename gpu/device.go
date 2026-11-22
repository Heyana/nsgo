package gpu

import (
	"fmt"
	"image"
	"slices"
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
	lost       bool

	limits            Limits
	features          Features
	compressedFormats []TextureFormat
	surfaceFormat     TextureFormat
	width, height     int

	pendingOps       []driver.GPUResourceOp
	readbacks        map[uint64]func(rgba []byte, err error)
	lastPassTimings  []PassTiming
	timestampQueries map[driver.GPUID]*TimestampQuery
}

func newDevice(surf *core.GPUSurface, scope *core.Scope, gen uint32, w, h int, fmt TextureFormat, lim Limits, feat Features, compressed []TextureFormat) *Device {
	if fmt == FormatUndefined {
		fmt = FormatRGBA8Unorm
	}
	if len(compressed) == 0 {
		compressed = compressedFormatsFromFeatures(feat)
	} else {
		compressed = slices.Clone(compressed)
	}
	return &Device{
		surf:              surf,
		scope:             scope,
		generation:        gen,
		limits:            lim,
		features:          feat,
		compressedFormats: compressed,
		surfaceFormat:     fmt,
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
	if d == nil || d.lost {
		return false
	}
	if d.surf != nil && d.surf.Disposed() {
		return false
	}
	return true
}

func (d *Device) invalidate(reason DeviceLostReason, msg string) {
	if d == nil || d.lost {
		return
	}
	d.lost = true
	d.pendingOps = nil
	d.lastPassTimings = nil
	for id, q := range d.timestampQueries {
		delete(d.timestampQueries, id)
		q.closed = true
	}
	err := Error{Code: ErrDeviceLost, Message: msg}
	for id, fn := range d.readbacks {
		delete(d.readbacks, id)
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

func (d *Device) completeReadback(id uint64, pixels []byte, errMsg string) {
	if d == nil {
		return
	}
	fn, ok := d.readbacks[id]
	if !ok {
		return
	}
	delete(d.readbacks, id)
	if fn == nil {
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
	d.pendingOps = append(d.pendingOps, op)
}

func (d *Device) flushOpsOnly() {
	if !d.alive() || len(d.pendingOps) == 0 {
		return
	}
	ops := d.pendingOps
	d.pendingOps = nil
	d.dispatchFrame(driver.GPUFrame{
		Generation: d.generation,
		Ops:        ops,
	})
}

func (d *Device) dispatchFrame(f driver.GPUFrame) {
	if !d.alive() {
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

// SurfaceFormat returns the swapchain's preferred color texture format.
func (d *Device) SurfaceFormat() TextureFormat { return d.surfaceFormat }

// SurfaceSize returns the current drawable size in physical pixels.
func (d *Device) SurfaceSize() (width, height int) { return d.width, d.height }

// Generation returns the device generation counter (incremented after device loss).
func (d *Device) Generation() uint32 { return d.generation }

// --- Retained GPU resources ---

// Buffer is a retained GPU vertex, index, uniform, or storage buffer.
type Buffer struct {
	dev    *Device
	id     driver.GPUID
	size   uint64
	usage  BufferUsage
	closed bool
}

// ID returns the buffer's process-unique numeric handle.
func (b *Buffer) ID() uint64 { return uint64(b.id) }

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
	if b.dev != nil && b.dev.alive() {
		b.dev.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpDestroyBuffer, ID: b.id})
	}
	return nil
}

// Texture is a retained GPU 2D, cube, array, or depth texture.
type Texture struct {
	dev    *Device
	id     driver.GPUID
	desc   TextureDesc
	closed bool
}

// ID returns the texture's numeric handle.
func (t *Texture) ID() uint64 { return uint64(t.id) }

// Desc returns the texture's descriptor.
func (t *Texture) Desc() TextureDesc { return t.desc }

// Close releases the texture immediately.
func (t *Texture) Close() error {
	if t == nil || t.closed {
		return nil
	}
	t.closed = true
	if t.dev != nil && t.dev.alive() {
		t.dev.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpDestroyTexture, ID: t.id})
	}
	return nil
}

// RenderTarget pairs an offscreen color Texture with an optional depth Texture.
type RenderTarget struct {
	Color  *Texture
	Depth  *Texture
	Width  uint32
	Height uint32
	closed bool
}

// Close releases the render target's color and depth textures.
func (rt *RenderTarget) Close() error {
	if rt == nil || rt.closed {
		return nil
	}
	rt.closed = true
	if rt.Color != nil {
		_ = rt.Color.Close()
	}
	if rt.Depth != nil {
		_ = rt.Depth.Close()
	}
	return nil
}

// Sampler is a retained GPU texture sampler.
type Sampler struct {
	dev    *Device
	id     driver.GPUID
	desc   SamplerDesc
	closed bool
}

// ID returns the sampler's numeric handle.
func (s *Sampler) ID() uint64 { return uint64(s.id) }

// Desc returns the sampler's descriptor.
func (s *Sampler) Desc() SamplerDesc { return s.desc }

// Close releases the sampler immediately.
func (s *Sampler) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	if s.dev != nil && s.dev.alive() {
		s.dev.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpDestroySampler, ID: s.id})
	}
	return nil
}

// BindGroupLayout describes the resource schema of a BindGroup slot.
type BindGroupLayout struct {
	dev     *Device
	id      driver.GPUID
	entries []BindGroupLayoutEntry
	closed  bool
}

// ID returns the layout's numeric handle.
func (l *BindGroupLayout) ID() uint64 { return uint64(l.id) }

// Close releases the bind group layout immediately.
func (l *BindGroupLayout) Close() error {
	if l == nil || l.closed {
		return nil
	}
	l.closed = true
	if l.dev != nil && l.dev.alive() {
		l.dev.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpDestroyBindGroupLayout, ID: l.id})
	}
	return nil
}

// BindGroup binds buffers, textures, and samplers for one shader @group(G).
type BindGroup struct {
	dev    *Device
	id     driver.GPUID
	layout *BindGroupLayout
	closed bool
}

// ID returns the bind group's numeric handle.
func (g *BindGroup) ID() uint64 { return uint64(g.id) }

// Close releases the bind group immediately.
func (g *BindGroup) Close() error {
	if g == nil || g.closed {
		return nil
	}
	g.closed = true
	if g.dev != nil && g.dev.alive() {
		g.dev.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpDestroyBindGroup, ID: g.id})
	}
	return nil
}

// Pipeline is an immutable vertex+fragment GPU render pipeline.
type Pipeline struct {
	dev    *Device
	id     driver.GPUID
	desc   PipelineDesc
	closed bool
}

// ID returns the pipeline's numeric handle.
func (p *Pipeline) ID() uint64 { return uint64(p.id) }

// Close releases the pipeline immediately.
func (p *Pipeline) Close() error {
	if p == nil || p.closed {
		return nil
	}
	p.closed = true
	if p.dev != nil && p.dev.alive() {
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
	if desc.Usage&UsageUniform != 0 && d.limits.MaxUniformBufferSize > 0 && size > d.limits.MaxUniformBufferSize {
		return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("uniform buffer size %d exceeds limit %d", size, d.limits.MaxUniformBufferSize)}
	}
	id := d.nextID()
	dd := driver.GPUBufferDesc{
		Label: debugLabel(desc.Label),
		Size:  size,
		Usage: driver.GPUBufferUsage(desc.Usage),
		Data:  slices.Clone(desc.Data),
	}
	d.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpCreateBuffer, ID: id, Buffer: &dd})
	buf := &Buffer{dev: d, id: id, size: size, usage: desc.Usage}
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
	if desc.MipLevels == 0 {
		desc.MipLevels = 1
	}
	if desc.SampleCount <= 0 {
		desc.SampleCount = 1
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
	tex := &Texture{dev: d, id: id, desc: desc}
	d.bindCleanup(cx, func() { _ = tex.Close() })
	return tex, nil
}

// CreateTextureFromImage allocates a 2D texture from an ImageSource with Pixels
// and uploads its RGBA pixels.
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

// CreateRenderTarget allocates an offscreen render target (color-only,
// color+depth, or depth-only shadow map) whose attachments can be rendered to
// and sampled by subsequent passes, scoped to cx.
func (d *Device) CreateRenderTarget(cx *core.Scope, desc RenderTargetDesc) (*RenderTarget, error) {
	if desc.ColorFormat == FormatUndefined && desc.DepthFormat == FormatUndefined {
		desc.ColorFormat = FormatRGBA8Unorm
	}
	var colorTex *Texture
	if desc.ColorFormat != FormatUndefined {
		if desc.ColorFormat.IsDepth() {
			return nil, Error{Code: ErrInvalidDescriptor, Message: "RenderTargetDesc.ColorFormat must not be a depth format"}
		}
		var err error
		colorTex, err = d.CreateTexture(cx, TextureDesc{
			Label:       debugLabelJoin(desc.Label, ".color"),
			Dimension:   Dim2D,
			Width:       desc.Width,
			Height:      desc.Height,
			SampleCount: desc.SampleCount,
			Format:      desc.ColorFormat,
			Usage:       TextureRenderAttachment | TextureSampled | TextureCopySrc,
		})
		if err != nil {
			return nil, err
		}
	}
	var depthTex *Texture
	if desc.DepthFormat != FormatUndefined {
		if !desc.DepthFormat.IsDepth() {
			if colorTex != nil {
				_ = colorTex.Close()
			}
			return nil, Error{Code: ErrInvalidDescriptor, Message: "RenderTargetDesc.DepthFormat must be a depth format"}
		}
		var err error
		depthTex, err = d.CreateTexture(cx, TextureDesc{
			Label:       debugLabelJoin(desc.Label, ".depth"),
			Dimension:   Dim2D,
			Width:       desc.Width,
			Height:      desc.Height,
			SampleCount: desc.SampleCount,
			Format:      desc.DepthFormat,
			Usage:       TextureRenderAttachment | TextureSampled | TextureCopySrc,
		})
		if err != nil {
			if colorTex != nil {
				_ = colorTex.Close()
			}
			return nil, err
		}
	}
	return &RenderTarget{Color: colorTex, Depth: depthTex, Width: desc.Width, Height: desc.Height}, nil
}

// CreateSampler allocates a retained GPU sampler scoped to cx.
func (d *Device) CreateSampler(cx *core.Scope, desc SamplerDesc) (*Sampler, error) {
	if !d.alive() {
		return nil, Error{Code: ErrDeviceLost, Message: "device is lost or disposed"}
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
		Compare:       driver.GPUCompareFunc(desc.Compare),
		MaxAnisotropy: desc.MaxAnisotropy,
	}
	d.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpCreateSampler, ID: id, Sampler: &dd})
	s := &Sampler{dev: d, id: id, desc: desc}
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
	l := &BindGroupLayout{dev: d, id: id, entries: slices.Clone(entries)}
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
			case BindingUniform, BindingStorageRead:
				if e.Buffer == nil || e.Buffer.closed || e.Buffer.dev != d {
					return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d expects a live Buffer", e.Binding)}
				}
				if le.Type == BindingUniform && e.Buffer.usage&UsageUniform == 0 {
					return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d buffer lacks UsageUniform", e.Binding)}
				}
				if e.Offset > e.Buffer.size {
					return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d buffer offset out of range", e.Binding)}
				}
			case BindingTexture:
				if e.Texture == nil || e.Texture.closed || e.Texture.dev != d {
					return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d expects a live Texture", e.Binding)}
				}
				if e.Texture.desc.Usage&TextureSampled == 0 {
					return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("binding %d texture lacks TextureSampled usage", e.Binding)}
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
	bg := &BindGroup{dev: d, id: id, layout: layout}
	d.bindCleanup(cx, func() { _ = bg.Close() })
	return bg, nil
}

// CreatePipeline compiles and validates an immutable render pipeline scoped to cx.
func (d *Device) CreatePipeline(cx *core.Scope, desc PipelineDesc) (*Pipeline, error) {
	if !d.alive() {
		return nil, Error{Code: ErrDeviceLost, Message: "device is lost or disposed"}
	}
	if desc.Vertex.Module == nil || !desc.Vertex.Module.HasVertexEntry(desc.Vertex.EntryPoint) {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "pipeline requires a valid Vertex shader stage"}
	}
	if desc.Fragment.Module != nil && !desc.Fragment.Module.HasFragmentEntry(desc.Fragment.EntryPoint) {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "pipeline Fragment entry point not found in module"}
	}
	if d.limits.MaxVertexBuffers > 0 && uint32(len(desc.VertexBuffers)) > d.limits.MaxVertexBuffers {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "too many vertex buffers in pipeline"}
	}
	if d.limits.MaxBindGroups > 0 && uint32(len(desc.Layouts)) > d.limits.MaxBindGroups {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "too many bind group layouts in pipeline"}
	}
	var totalAttrs uint32
	dVBs := make([]driver.GPUVertexBufferLayout, len(desc.VertexBuffers))
	for i, vb := range desc.VertexBuffers {
		attrs := make([]driver.GPUVertexAttribute, len(vb.Attributes))
		for j, a := range vb.Attributes {
			sz := driver.GPUVertexFormat(a.Format).ByteSize()
			if sz == 0 {
				return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("invalid vertex attribute format %d", a.Format)}
			}
			if vb.Stride > 0 && a.Offset+sz > vb.Stride {
				return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("vertex attribute at location %d exceeds buffer stride %d", a.Location, vb.Stride)}
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
			Attributes: attrs,
		}
	}
	if d.limits.MaxVertexAttributes > 0 && totalAttrs > d.limits.MaxVertexAttributes {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "too many vertex attributes in pipeline"}
	}

	layoutIDs := make([]driver.GPUID, len(desc.Layouts))
	for i, l := range desc.Layouts {
		if l == nil || l.closed || l.dev != d {
			return nil, Error{Code: ErrInvalidDescriptor, Message: "pipeline BindGroupLayout is nil or invalid"}
		}
		layoutIDs[i] = l.id
	}

	dTargets := make([]driver.GPUColorTargetState, len(desc.Targets))
	for i, t := range desc.Targets {
		fmtVal := t.Format
		if fmtVal == FormatUndefined {
			fmtVal = d.surfaceFormat
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
		if desc.DepthStencil.Format != FormatUndefined && !desc.DepthStencil.Format.IsDepth() {
			return nil, Error{Code: ErrInvalidDescriptor, Message: "DepthStencil.Format must be a depth format"}
		}
		cmp := desc.DepthStencil.DepthCompare
		if cmp == CompareUndefined {
			cmp = CompareLess
		}
		dDS = &driver.GPUDepthStencilState{
			Format:              driver.GPUTextureFormat(desc.DepthStencil.Format),
			DepthWrite:          desc.DepthStencil.DepthWrite,
			DepthCompare:        driver.GPUCompareFunc(cmp),
			DepthBias:           desc.DepthStencil.DepthBias,
			DepthBiasSlopeScale: desc.DepthStencil.DepthBiasSlopeScale,
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
	id := d.nextID()
	d.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpCreatePipeline, ID: id, Pipeline: &dd})
	p := &Pipeline{dev: d, id: id, desc: desc}
	d.bindCleanup(cx, func() { _ = p.Close() })
	return p, nil
}

// --- Data uploads ---

// WriteBuffer copies data into buf starting at byte offset. Data is cloned
// before returning so callers may immediately reuse the slice.
func (d *Device) WriteBuffer(buf *Buffer, offset uint64, data []byte) {
	if !d.alive() || buf == nil || buf.closed || buf.dev != d || len(data) == 0 {
		return
	}
	if offset+uint64(len(data)) > buf.size {
		return
	}
	d.enqueueOp(driver.GPUResourceOp{
		Kind:   driver.GPUOpWriteBuffer,
		ID:     buf.id,
		Offset: offset,
		Data:   slices.Clone(data),
	})
}

// WriteTexture copies raw texel bytes into dst according to layout.
func (d *Device) WriteTexture(tex *Texture, dst TextureCopy, data []byte, layout DataLayout) {
	if !d.alive() || tex == nil || tex.closed || tex.dev != d || len(data) == 0 {
		return
	}
	d.enqueueOp(driver.GPUResourceOp{
		Kind: driver.GPUOpWriteTexture,
		ID:   tex.id,
		Data: slices.Clone(data),
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

// WriteTextureRGBA uploads img's RGBA pixels into tex at mipLevel and arrayLayer.
func (d *Device) WriteTextureRGBA(tex *Texture, mipLevel, arrayLayer uint32, img *image.RGBA) {
	if img == nil || tex == nil {
		return
	}
	b := img.Bounds()
	d.WriteTexture(tex, TextureCopy{
		Texture:    tex,
		MipLevel:   mipLevel,
		ArrayLayer: arrayLayer,
	}, img.Pix, DataLayout{
		BytesPerRow:  uint32(img.Stride),
		RowsPerImage: uint32(b.Dy()),
	})
}

// --- Command encoding, Frame, and RenderPass ---

// CommandBuffer is an immutable batch of recorded copy commands and render passes.
type CommandBuffer struct {
	dev      *Device
	copies   []driver.GPUCopyOp
	passes   []driver.GPUPass
	debugOps []driver.GPUDebugOp
}

// Encoder records render passes and copy commands for one Device.Submit call.
type Encoder struct {
	dev      *Device
	copies   []driver.GPUCopyOp
	passes   []driver.GPUPass
	debugOps []driver.GPUDebugOp
	finished bool
}

// BeginEncoder starts recording a new CommandBuffer.
func (d *Device) BeginEncoder() *Encoder {
	return &Encoder{dev: d}
}

// PushDebugGroup begins a nested debug group in GPU captures (Metal/RenderDoc/WebGPU). It is a zero-cost no-op in release builds (without nsgodev or nsgotest).
func (e *Encoder) PushDebugGroup(label string) { e.pushDebugGroup(label) }

// PopDebugGroup ends the most recent Encoder.PushDebugGroup. It is a zero-cost no-op in release builds (without nsgodev or nsgotest).
func (e *Encoder) PopDebugGroup() { e.popDebugGroup() }

// InsertDebugMarker inserts an instantaneous debug marker into GPU captures. It is a zero-cost no-op in release builds (without nsgodev or nsgotest).
func (e *Encoder) InsertDebugMarker(label string) { e.insertDebugMarker(label) }

// BeginRenderPass starts a render pass on e. Call RenderPass.End when done.
func (e *Encoder) BeginRenderPass(desc RenderPassDesc) *RenderPass {
	var cas []driver.GPUColorAttachment
	if len(desc.ColorAttachments) == 0 && desc.DepthStencil == nil {
		cas = []driver.GPUColorAttachment{{
			LoadOp:  driver.GPULoadLoad,
			StoreOp: driver.GPUStoreStore,
		}}
	} else if len(desc.ColorAttachments) > 0 {
		cas = make([]driver.GPUColorAttachment, len(desc.ColorAttachments))
		for i, ca := range desc.ColorAttachments {
			var tid, rid driver.GPUID
			if ca.Texture != nil {
				tid = ca.Texture.id
			}
			if ca.ResolveTarget != nil {
				rid = ca.ResolveTarget.id
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
			tid = desc.DepthStencil.Texture.id
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
	if e.dev != nil && e.dev.features.TimestampQueries {
		timingLabel = desc.TimingLabel
		if desc.TimestampQuery != nil && !desc.TimestampQuery.closed && desc.TimestampQuery.dev == e.dev {
			queryID = desc.TimestampQuery.id
			if timingLabel == "" {
				timingLabel = desc.TimestampQuery.label
			}
		}
	}
	return &RenderPass{
		enc: e,
		pass: driver.GPUPass{
			Label:            debugLabel(desc.Label),
			TimingLabel:      timingLabel,
			TimestampQueryID: queryID,
			ColorAttachments: cas,
			DepthStencil:     ds,
		},
	}
}

// CopyBufferToBuffer records a buffer-to-buffer copy.
func (e *Encoder) CopyBufferToBuffer(src *Buffer, srcOffset uint64, dst *Buffer, dstOffset, size uint64) {
	if e.finished || src == nil || dst == nil || size == 0 {
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
	if e.finished || src.Buffer == nil || dst.Texture == nil {
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
	if e.finished || src.Texture == nil || dst.Texture == nil {
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

// Finish seals the encoder into an immutable CommandBuffer.
func (e *Encoder) Finish() *CommandBuffer {
	e.finished = true
	return &CommandBuffer{dev: e.dev, copies: e.copies, passes: e.passes, debugOps: e.debugOps}
}

// RenderPass records drawing state and draw calls for one render pass.
type RenderPass struct {
	enc   *Encoder
	pass  driver.GPUPass
	ended bool
}

// PushDebugGroup begins a nested debug group inside this render pass. It is a zero-cost no-op in release builds (without nsgodev or nsgotest).
func (p *RenderPass) PushDebugGroup(label string) { p.pushDebugGroup(label) }

// PopDebugGroup closes the most recent RenderPass.PushDebugGroup. It is a zero-cost no-op in release builds (without nsgodev or nsgotest).
func (p *RenderPass) PopDebugGroup() { p.popDebugGroup() }

// InsertDebugMarker inserts an instantaneous debug marker inside this render pass. It is a zero-cost no-op in release builds (without nsgodev or nsgotest).
func (p *RenderPass) InsertDebugMarker(label string) { p.insertDebugMarker(label) }

// ClearColor configures color attachment 0 to clear to c at pass start.
func (p *RenderPass) ClearColor(c core.Color) {
	if p.ended || len(p.pass.ColorAttachments) == 0 {
		return
	}
	p.pass.ColorAttachments[0].LoadOp = driver.GPULoadClear
	p.pass.ColorAttachments[0].ClearColor = c
}

// ClearDepth configures the depth attachment to clear to depth at pass start.
func (p *RenderPass) ClearDepth(depth float32) {
	if p.ended {
		return
	}
	if p.pass.DepthStencil == nil {
		p.pass.DepthStencil = &driver.GPUDepthStencilAttachment{
			DepthStoreOp: driver.GPUStoreStore,
		}
	}
	p.pass.DepthStencil.DepthLoadOp = driver.GPULoadClear
	p.pass.DepthStencil.ClearDepth = depth
}

// SetViewport sets the rasterizer viewport rectangle and depth range.
func (p *RenderPass) SetViewport(x, y, width, height, minDepth, maxDepth float32) {
	if p.ended {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind: driver.GPUCmdSetViewport,
		F0:   x, F1: y, F2: width, F3: height, F4: minDepth, F5: maxDepth,
	})
}

// SetScissor restricts rendering to the given pixel scissor rectangle.
func (p *RenderPass) SetScissor(x, y, width, height int32) {
	if p.ended {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind: driver.GPUCmdSetScissor,
		I0:   x, I1: y, I2: width, I3: height,
	})
}

// SetPipeline binds the active render pipeline.
func (p *RenderPass) SetPipeline(pipe *Pipeline) {
	if p.ended || pipe == nil || pipe.closed {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind: driver.GPUCmdSetPipeline,
		ID:   pipe.id,
	})
}

// SetBindGroup binds group to @group(slot) with optional dynamic offsets.
func (p *RenderPass) SetBindGroup(slot uint32, group *BindGroup, dynamicOffsets ...uint32) {
	if p.ended || group == nil || group.closed {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind:           driver.GPUCmdSetBindGroup,
		Slot:           slot,
		ID:             group.id,
		DynamicOffsets: slices.Clone(dynamicOffsets),
	})
}

// SetVertexBuffer binds buf to vertex buffer slot starting at byte offset.
func (p *RenderPass) SetVertexBuffer(slot uint32, buf *Buffer, offset uint64) {
	if p.ended || buf == nil || buf.closed {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind:   driver.GPUCmdSetVertexBuffer,
		Slot:   slot,
		ID:     buf.id,
		Offset: offset,
	})
}

// SetIndexBuffer binds buf as the active index buffer.
func (p *RenderPass) SetIndexBuffer(buf *Buffer, format IndexFormat, offset uint64) {
	if p.ended || buf == nil || buf.closed {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind:        driver.GPUCmdSetIndexBuffer,
		ID:          buf.id,
		IndexFormat: driver.GPUIndexFormat(format),
		Offset:      offset,
	})
}

// SetStencilReference sets the stencil reference value.
func (p *RenderPass) SetStencilReference(ref uint32) {
	if p.ended {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind: driver.GPUCmdSetStencilReference,
		U0:   ref,
	})
}

// SetBlendConstant sets the constant blend color.
func (p *RenderPass) SetBlendConstant(r, g, b, a float32) {
	if p.ended {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind: driver.GPUCmdSetBlendConstant,
		F0:   r, F1: g, F2: b, F3: a,
	})
}

// Draw draws vertexCount non-indexed vertices starting at firstVertex.
func (p *RenderPass) Draw(vertexCount, firstVertex uint32) {
	if p.ended {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind: driver.GPUCmdDraw,
		U0:   vertexCount, U1: firstVertex,
	})
}

// DrawIndexed draws indexCount indexed vertices.
func (p *RenderPass) DrawIndexed(indexCount, firstIndex uint32, baseVertex int32) {
	if p.ended {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind: driver.GPUCmdDrawIndexed,
		U0:   indexCount, U1: firstIndex, I0: baseVertex,
	})
}

// DrawInstanced draws instanceCount instances of vertexCount vertices.
func (p *RenderPass) DrawInstanced(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	if p.ended {
		return
	}
	p.pass.Commands = append(p.pass.Commands, driver.GPUCommand{
		Kind: driver.GPUCmdDrawInstanced,
		U0:   vertexCount, U1: instanceCount, U2: firstVertex, U3: firstInstance,
	})
}

// DrawIndexedInstanced draws instanceCount instances of indexCount indexed vertices.
func (p *RenderPass) DrawIndexedInstanced(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	if p.ended {
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
	if p.enc != nil && !p.enc.finished {
		p.enc.passes = append(p.enc.passes, p.pass)
	}
}

// Submit hands pending resource ops and cmds directly to the platform backend
// off-commit (zero UI view-tree commits per frame).
func (d *Device) Submit(cmds ...*CommandBuffer) {
	if !d.alive() {
		return
	}
	frame := driver.GPUFrame{
		Generation: d.generation,
		Ops:        d.pendingOps,
		Present:    len(cmds) > 0,
	}
	d.pendingOps = nil
	for _, cb := range cmds {
		if cb == nil {
			continue
		}
		passOffset := len(frame.Passes)
		appendFrameDebugOps(&frame, cb.debugOps, passOffset)
		frame.Copies = append(frame.Copies, cb.copies...)
		frame.Passes = append(frame.Passes, cb.passes...)
	}
	if len(frame.Ops) == 0 && len(frame.Copies) == 0 && len(frame.Passes) == 0 && len(frame.DebugOps) == 0 {
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
	if tex == nil || tex.closed || tex.dev != d {
		if fn != nil {
			fn(nil, Error{Code: ErrInvalidDescriptor, Message: "ReadTexture requires a live Texture"})
		}
		return
	}
	rid := readbackIDGen.Add(1)
	d.readbacks[rid] = fn
	d.enqueueOp(driver.GPUResourceOp{
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
	d.readbacks[rid] = fn
	d.enqueueOp(driver.GPUResourceOp{
		Kind:            driver.GPUOpReadTexture,
		ReadbackID:      rid,
		ReadbackSurface: true,
	})
	d.flushOpsOnly()
}

// Frame is the per-frame recording context handed to ViewProps.OnFrame.
type Frame struct {
	dev       *Device
	enc       *Encoder
	at        time.Time
	delta     time.Duration
	submitted bool
}

// BeginFrame starts a new Frame on d at timestamp at.
func (d *Device) BeginFrame(at time.Time) *Frame {
	return &Frame{dev: d, enc: d.BeginEncoder(), at: at}
}

// Device returns the owning GPU Device.
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

// BeginRenderPass starts a RenderPass recorded into this frame.
func (f *Frame) BeginRenderPass(desc RenderPassDesc) *RenderPass {
	return f.enc.BeginRenderPass(desc)
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

// Submit submits this frame's recorded commands off-commit. Safe to call once;
// ViewProps.OnFrame automatically submits if the callback does not call Submit.
func (f *Frame) Submit() {
	if f == nil || f.submitted {
		return
	}
	f.submitted = true
	f.dev.Submit(f.enc.Finish())
}
