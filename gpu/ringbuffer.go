package gpu

import (
	"fmt"
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

var fenceIDGen atomic.Uint64

// FrameFence tracks GPU completion of one submitted frame and its ring-buffer
// slot. Waiting on the previous FrameFence for slot (frame % FramesInFlight)
// before writing that slot guarantees the CPU never overwrites memory the GPU
// is still reading.
type FrameFence struct {
	id         uint64
	frameIndex uint64
	slot       int
	done       chan struct{}
	once       sync.Once
	signaled   atomic.Bool
}

func newFrameFence(id, frameIndex uint64, slot int) *FrameFence {
	return &FrameFence{
		id:         id,
		frameIndex: frameIndex,
		slot:       slot,
		done:       make(chan struct{}),
	}
}

// ID returns the fence's monotonic identifier.
func (f *FrameFence) ID() uint64 {
	if f == nil {
		return 0
	}
	return f.id
}

// FrameIndex returns the monotonic frame index associated with this fence.
func (f *FrameFence) FrameIndex() uint64 {
	if f == nil {
		return 0
	}
	return f.frameIndex
}

// FrameSlot returns the ring-buffer slot index [0, FramesInFlight) guarded by f.
func (f *FrameFence) FrameSlot() int {
	if f == nil {
		return 0
	}
	return f.slot
}

// Signaled reports whether the GPU has finished executing the frame guarded by f.
func (f *FrameFence) Signaled() bool {
	if f == nil {
		return true
	}
	return f.signaled.Load()
}

// Wait blocks until the GPU signals completion of the frame guarded by f.
func (f *FrameFence) Wait() {
	if f == nil {
		return
	}
	<-f.done
}

// WaitTimeout waits up to d for the fence to be signaled, reporting whether it
// became signaled before the timeout elapsed.
func (f *FrameFence) WaitTimeout(d time.Duration) bool {
	if f == nil {
		return true
	}
	if f.signaled.Load() {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-f.done:
		return true
	case <-timer.C:
		return f.signaled.Load()
	}
}

func (f *FrameFence) signal() {
	if f == nil {
		return
	}
	f.once.Do(func() {
		f.signaled.Store(true)
		close(f.done)
	})
}

// RingBuffer is a multi-buffered per-frame streaming allocator backed by one
// persistent GPU Buffer partitioned into Device.FramesInFlight() disjoint slots.
//
// During frame k (slot s = k % N), Alloc and Write sub-allocate aligned slices
// within slot s while the GPU may still read slot (k-1) % N with zero stalls
// and zero write-while-read corruption. Device.BeginFrame waits on slot s's
// previous FrameFence before resetting slot s's write cursor, and Frame.Submit /
// Device.EndFrame flushes slot s's dirty range in a single bulk upload.
type RingBuffer struct {
	dev            *Device
	buf            *Buffer
	label          string
	usage          BufferUsage
	framesInFlight int
	alignment      uint64
	slotSize       uint64
	totalSize      uint64
	backing        []byte
	slotUsed       []uint64
	slotDirty      []bool
	cleanup        *cleanupGroup
	closed         bool
}

// CreateRingBuffer allocates a per-frame streaming RingBuffer partitioned across
// d.FramesInFlight() slots, scoped to cx.
func (d *Device) CreateRingBuffer(cx *core.Scope, desc RingBufferDesc) (*RingBuffer, error) {
	if !d.alive() {
		return nil, Error{Code: ErrDeviceLost, Message: "device is lost or disposed"}
	}
	if desc.Usage == 0 {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "ring buffer requires at least one Usage flag"}
	}
	if desc.Usage&UsageStorage != 0 && !d.features.StorageBuffers {
		return nil, Error{Code: ErrUnsupported, Message: "storage buffers are not supported on this device"}
	}
	if desc.SlotSize == 0 {
		return nil, Error{Code: ErrInvalidDescriptor, Message: "ring buffer SlotSize must be greater than zero"}
	}

	minUniform := uint64(d.limits.MinUniformBufferAlignment)
	if minUniform == 0 {
		minUniform = 256
	}
	align := desc.Alignment
	if align == 0 {
		if desc.Usage&UsageUniform != 0 {
			align = minUniform
		} else {
			align = 16
		}
	}
	if align == 0 || align&(align-1) != 0 {
		return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("ring buffer Alignment %d must be a power of two", align)}
	}
	if desc.Usage&UsageUniform != 0 && align < minUniform {
		return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("uniform ring buffer Alignment %d is smaller than device minimum %d", align, minUniform)}
	}

	slotSize := alignUp64(desc.SlotSize, align)
	slots := d.framesInFlight
	totalSize := slotSize * uint64(slots)
	if totalSize > math.MaxUint32 {
		return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("ring buffer total size %d exceeds 32-bit dynamic offset range", totalSize)}
	}
	if d.limits.MaxBufferSize > 0 && totalSize > d.limits.MaxBufferSize {
		return nil, Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf("ring buffer total size %d exceeds limit %d", totalSize, d.limits.MaxBufferSize)}
	}

	usage := desc.Usage | UsageCopyDst
	backing := make([]byte, totalSize)
	id := d.nextID()
	dd := driver.GPUBufferDesc{
		Label: desc.Label,
		Size:  totalSize,
		Usage: driver.GPUBufferUsage(usage),
	}
	if d.directMapped {
		dd.Shared = backing
	}
	d.enqueueOp(driver.GPUResourceOp{Kind: driver.GPUOpCreateBuffer, ID: id, Buffer: &dd})
	buf := &Buffer{dev: d, id: id, epoch: d.generation, label: desc.Label, size: totalSize, usage: usage}

	rb := &RingBuffer{
		dev:            d,
		buf:            buf,
		label:          desc.Label,
		usage:          usage,
		framesInFlight: slots,
		alignment:      align,
		slotSize:       slotSize,
		totalSize:      totalSize,
		backing:        backing,
		slotUsed:       make([]uint64, slots),
		slotDirty:      make([]bool, slots),
	}
	d.ringBuffers[rb] = struct{}{}
	rb.cleanup = d.bindCleanup(cx, rb)
	return rb, nil
}

// Buffer returns the underlying retained GPU Buffer backing all frame slots.
// Bind it once in a BindGroup with HasDynamicOffset: true (for UsageUniform) or
// pass it with slice.Offset to RenderPass.SetVertexBuffer / SetIndexBuffer.
func (r *RingBuffer) Buffer() *Buffer {
	if r == nil {
		return nil
	}
	return r.buf
}

// SlotSize returns the aligned byte capacity available in each frame slot.
func (r *RingBuffer) SlotSize() uint64 { return r.slotSize }

// TotalSize returns the total byte size of the backing GPU buffer across all slots.
func (r *RingBuffer) TotalSize() uint64 { return r.totalSize }

// Alignment returns the byte alignment applied to every Alloc/Write call.
func (r *RingBuffer) Alignment() uint64 { return r.alignment }

// FramesInFlight returns the number of rotating frame slots in r.
func (r *RingBuffer) FramesInFlight() int { return r.framesInFlight }

// CurrentSlot returns the active frame-in-flight slot index on the owning Device.
func (r *RingBuffer) CurrentSlot() int {
	if r == nil || r.dev == nil {
		return 0
	}
	return r.dev.FrameSlot()
}

// SlotOffset returns the base byte offset of slot within the underlying Buffer.
func (r *RingBuffer) SlotOffset(slot int) uint64 {
	if r == nil || slot < 0 || slot >= r.framesInFlight {
		return 0
	}
	return uint64(slot) * r.slotSize
}

// Used returns the aligned byte count allocated so far in the current frame slot.
func (r *RingBuffer) Used() uint64 {
	if r == nil || r.closed {
		return 0
	}
	return r.slotUsed[r.CurrentSlot()]
}

// Remaining returns the unallocated byte capacity remaining in the current frame slot.
func (r *RingBuffer) Remaining() uint64 {
	if r == nil || r.closed {
		return 0
	}
	used := r.slotUsed[r.CurrentSlot()]
	if used >= r.slotSize {
		return 0
	}
	return r.slotSize - used
}

// Alloc reserves size bytes in the active frame slot, aligned to r.Alignment()
// (256 bytes by default for uniform ring buffers), and returns the BufferSlice
// together with its uint32 dynamic offset for RenderPass.SetBindGroup.
func (r *RingBuffer) Alloc(size uint64) (BufferSlice, uint32, error) {
	if r == nil || r.closed {
		return BufferSlice{}, 0, Error{Code: ErrInvalidDescriptor, Message: "ring buffer is nil or closed"}
	}
	if !r.dev.alive() {
		return BufferSlice{}, 0, Error{Code: ErrDeviceLost, Message: "device is lost or disposed"}
	}
	if size == 0 {
		return BufferSlice{}, 0, Error{Code: ErrInvalidDescriptor, Message: "ring buffer allocation size must be greater than zero"}
	}
	slot := r.CurrentSlot()
	localOff := alignUp64(r.slotUsed[slot], r.alignment)
	end := localOff + size
	if end > r.slotSize {
		rem := uint64(0)
		if localOff < r.slotSize {
			rem = r.slotSize - localOff
		}
		return BufferSlice{}, 0, Error{
			Code:    ErrOutOfMemory,
			Message: fmt.Sprintf("ring buffer %q slot %d exhausted (need %d bytes, remaining %d)", r.label, slot, size, rem),
		}
	}
	absOff := uint64(slot)*r.slotSize + localOff
	nextUsed := alignUp64(end, r.alignment)
	if nextUsed > r.slotSize {
		nextUsed = r.slotSize
	}
	r.slotUsed[slot] = nextUsed
	r.slotDirty[slot] = true
	slice := BufferSlice{
		Buffer: r.buf,
		Offset: absOff,
		Size:   size,
		Slot:   slot,
		Data:   r.backing[absOff : absOff+size],
	}
	return slice, uint32(absOff), nil
}

// Write allocates len(data) aligned bytes in the active frame slot, copies data
// into the slot, and returns the BufferSlice and its uint32 dynamic offset.
func (r *RingBuffer) Write(data []byte) (BufferSlice, uint32, error) {
	slice, off, err := r.Alloc(uint64(len(data)))
	if err != nil {
		return BufferSlice{}, 0, err
	}
	copy(slice.Data, data)
	return slice, off, nil
}

func (r *RingBuffer) resetSlot(slot int) {
	if r == nil || r.closed || slot < 0 || slot >= r.framesInFlight {
		return
	}
	r.slotUsed[slot] = 0
	r.slotDirty[slot] = false
}

func (r *RingBuffer) isDirty(slot int) bool {
	if r == nil || r.closed || slot < 0 || slot >= r.framesInFlight {
		return false
	}
	return r.slotDirty[slot] && r.slotUsed[slot] > 0
}

func (r *RingBuffer) flushSlot(slot int) {
	if !r.isDirty(slot) || r.buf == nil || r.buf.closed {
		return
	}
	used := r.slotUsed[slot]
	base := uint64(slot) * r.slotSize
	r.slotDirty[slot] = false
	if r.dev.directMapped {
		// Backing slice is shared directly with the software GPU buffer; emit a
		// zero-copy sync marker without cloning or aliasing op.Data.
		r.dev.enqueueOp(driver.GPUResourceOp{
			Kind:   driver.GPUOpWriteBuffer,
			ID:     r.buf.id,
			Offset: base,
		})
		return
	}
	r.dev.enqueueOp(driver.GPUResourceOp{
		Kind:   driver.GPUOpWriteBuffer,
		ID:     r.buf.id,
		Offset: base,
		Data:   slices.Clone(r.backing[base : base+used]),
	})
}

// Close releases the ring buffer and its underlying GPU Buffer. Safe to call
// more than once.
func (r *RingBuffer) Close() error {
	if r == nil || r.closed {
		return nil
	}
	r.closed = true
	r.cleanup.remove(r)
	r.cleanup = nil
	if r.dev != nil {
		delete(r.dev.ringBuffers, r)
	}
	if r.buf != nil {
		return r.buf.Close()
	}
	return nil
}

func alignUp64(n, align uint64) uint64 {
	if align <= 1 {
		return n
	}
	return (n + align - 1) &^ (align - 1)
}
