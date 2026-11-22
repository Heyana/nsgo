package android

import (
	"fmt"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shaderir/glsl"
)

// GPU view protocol encoding and decoding (docs/architecture/gpu.md, #1109). This file builds on every OS so unit tests can verify GLSL ES 3.00 pipeline compilation, binary GPU frame serialization, upload chunking, and GPU event decoding without an Android device.

// maxGPUChunk bounds a single buffer upload op so large static meshes split cleanly into sequential pipe messages under maxMessage (64 MiB).
const maxGPUChunk = 16 << 20

// maxGPUFrameBudget bounds the raw payload bytes accumulated in one GPUFrame message before flushing a preceding non-presenting batch.
const maxGPUFrameBudget = maxMessage / 2

// writeGPUViewConfig writes an ItemGPU payload as NsgoView.apply reads it.
func writeGPUViewConfig(w *writer, c *driver.GPUViewConfig) {
	var cfg driver.GPUViewConfig
	if c != nil {
		cfg = *c
	}
	w.byte(byte(cfg.ColorFormat))
	w.byte(byte(cfg.DepthFormat))
	w.u32(uint32(max(cfg.SampleCount, 1)))
}

// compileGPUPipeline compiles a GPUPipelineDesc's shaderir stages into GLSL ES 3.00 vertex and fragment sources plus flattened uniform block slots and combined sampler pairs for Android's GpuView.java.
func compileGPUPipeline(desc *driver.GPUPipelineDesc) (glsl.PipelineSource, error) {
	if desc == nil {
		return glsl.PipelineSource{}, fmt.Errorf("android gpu: nil pipeline descriptor")
	}
	if desc.Vertex.IR == nil {
		return glsl.PipelineSource{}, fmt.Errorf("android gpu: pipeline missing vertex shader IR")
	}
	// Common case: single WGSL module containing both @vertex and @fragment.
	if desc.Fragment.IR == nil || desc.Fragment.IR == desc.Vertex.IR {
		src, err := glsl.CompilePipeline(desc.Vertex.IR, glsl.GLSLVersionES300)
		if err != nil {
			return glsl.PipelineSource{}, fmt.Errorf("android gpu: compile GLSL ES 3.00: %w", err)
		}
		if desc.Fragment.IR == nil {
			src.FragmentSource = ""
		}
		return src, nil
	}

	// Separate vertex and fragment shaderir programs: compile each and merge their uniform block and combined-sampler binding tables deterministically.
	vs, err := glsl.CompilePipeline(desc.Vertex.IR, glsl.GLSLVersionES300)
	if err != nil {
		return glsl.PipelineSource{}, fmt.Errorf("android gpu: compile vertex GLSL ES 3.00: %w", err)
	}
	fs, err := glsl.CompilePipeline(desc.Fragment.IR, glsl.GLSLVersionES300)
	if err != nil {
		return glsl.PipelineSource{}, fmt.Errorf("android gpu: compile fragment GLSL ES 3.00: %w", err)
	}
	merged := glsl.PipelineSource{
		VertexSource:   vs.VertexSource,
		FragmentSource: fs.FragmentSource,
	}
	seenUB := map[string]bool{}
	var nextSlot uint32
	for _, ub := range append(vs.UniformBlocks, fs.UniformBlocks...) {
		key := fmt.Sprintf("%d:%d:%s", ub.Group, ub.Binding, ub.BlockName)
		if seenUB[key] {
			continue
		}
		seenUB[key] = true
		ub.Slot = nextSlot
		nextSlot++
		merged.UniformBlocks = append(merged.UniformBlocks, ub)
	}
	seenSP := map[string]bool{}
	var nextUnit uint32
	for _, sp := range append(vs.SamplerPairs, fs.SamplerPairs...) {
		if seenSP[sp.Name] {
			continue
		}
		seenSP[sp.Name] = true
		sp.Unit = nextUnit
		nextUnit++
		merged.SamplerPairs = append(merged.SamplerPairs, sp)
	}
	return merged, nil
}

// splitGPUFrames expands oversized buffer uploads into chunks <= maxGPUChunk and splits multi-op frames so every encoded extGPUFrame stays safely below maxMessage. Only the final frame preserves f.Copies, f.Passes, and f.Present.
func splitGPUFrames(f driver.GPUFrame) []driver.GPUFrame {
	var expanded []driver.GPUResourceOp
	for _, op := range f.Ops {
		switch op.Kind {
		case driver.GPUOpCreateBuffer:
			if op.Buffer != nil && len(op.Buffer.Data) > maxGPUChunk {
				desc := *op.Buffer
				data := desc.Data
				desc.Data = nil
				expanded = append(expanded, driver.GPUResourceOp{
					Kind:   driver.GPUOpCreateBuffer,
					ID:     op.ID,
					Buffer: &desc,
				})
				for off := 0; off < len(data); off += maxGPUChunk {
					end := min(off+maxGPUChunk, len(data))
					expanded = append(expanded, driver.GPUResourceOp{
						Kind:   driver.GPUOpWriteBuffer,
						ID:     op.ID,
						Offset: uint64(off),
						Data:   data[off:end],
					})
				}
				continue
			}
		case driver.GPUOpWriteBuffer:
			if len(op.Data) > maxGPUChunk {
				for off := 0; off < len(op.Data); off += maxGPUChunk {
					end := min(off+maxGPUChunk, len(op.Data))
					expanded = append(expanded, driver.GPUResourceOp{
						Kind:   driver.GPUOpWriteBuffer,
						ID:     op.ID,
						Offset: op.Offset + uint64(off),
						Data:   op.Data[off:end],
					})
				}
				continue
			}
		case driver.GPUOpWriteTexture:
			if len(op.Data) > maxGPUChunk && op.DataLayout.BytesPerRow > 0 && op.DataLayout.Offset < uint64(len(op.Data)) {
				data := op.Data[op.DataLayout.Offset:]
				bpr := int(op.DataLayout.BytesPerRow)
				totalRows := len(data) / bpr
				if op.DataLayout.RowsPerImage > 0 && int(op.DataLayout.RowsPerImage) < totalRows {
					totalRows = int(op.DataLayout.RowsPerImage)
				}
				rowsPerChunk := max(maxGPUChunk/bpr, 1)
				if totalRows > rowsPerChunk {
					for row := 0; row < totalRows; row += rowsPerChunk {
						nRows := min(rowsPerChunk, totalRows-row)
						dst := op.TextureDst
						dst.OriginY += uint32(row)
						expanded = append(expanded, driver.GPUResourceOp{
							Kind:       driver.GPUOpWriteTexture,
							ID:         op.ID,
							TextureDst: dst,
							DataLayout: driver.GPUDataLayout{
								Offset:       0,
								BytesPerRow:  op.DataLayout.BytesPerRow,
								RowsPerImage: uint32(nRows),
							},
							Data: data[row*bpr : (row+nRows)*bpr],
						})
					}
					continue
				}
			}
		}
		expanded = append(expanded, op)
	}

	if len(expanded) == 0 {
		return []driver.GPUFrame{f}
	}

	var out []driver.GPUFrame
	var cur []driver.GPUResourceOp
	curBytes := 0
	for _, op := range expanded {
		n := opPayloadSize(op)
		if len(cur) > 0 && curBytes+n > maxGPUFrameBudget {
			out = append(out, driver.GPUFrame{
				SurfaceID:  f.SurfaceID,
				ViewID:     f.ViewID,
				Generation: f.Generation,
				Ops:        cur,
			})
			cur = nil
			curBytes = 0
		}
		cur = append(cur, op)
		curBytes += n
	}
	final := f
	final.Ops = cur
	out = append(out, final)
	return out
}

func opPayloadSize(op driver.GPUResourceOp) int {
	n := 64
	switch op.Kind {
	case driver.GPUOpCreateBuffer:
		if op.Buffer != nil {
			n += len(op.Buffer.Data)
		}
	case driver.GPUOpWriteBuffer, driver.GPUOpWriteTexture:
		n += len(op.Data)
	}
	return n
}

// encodeGPUFrame writes one driver.GPUFrame in the binary format GpuView.java executes on the shared GL thread. Pipelines are compiled to GLSL ES 3.00 here.
func encodeGPUFrame(w *writer, f driver.GPUFrame) error {
	w.u32(f.SurfaceID)
	w.u32(uint32(f.ViewID))
	w.u32(f.Generation)
	w.flags(f.Present)

	// 1. Resource ops.
	w.u32(uint32(len(f.Ops)))
	for i := range f.Ops {
		if err := writeGPUResourceOp(w, &f.Ops[i]); err != nil {
			return err
		}
	}

	// 2. Copy ops.
	w.u32(uint32(len(f.Copies)))
	for i := range f.Copies {
		writeGPUCopyOp(w, &f.Copies[i])
	}

	// 3. Render passes.
	w.u32(uint32(len(f.Passes)))
	for i := range f.Passes {
		writeGPUPass(w, &f.Passes[i])
	}
	return nil
}

func writeGPUResourceOp(w *writer, op *driver.GPUResourceOp) error {
	w.byte(byte(op.Kind))
	w.u64(uint64(op.ID))
	switch op.Kind {
	case driver.GPUOpCreateBuffer:
		var b driver.GPUBufferDesc
		if op.Buffer != nil {
			b = *op.Buffer
		}
		w.u64(b.Size)
		w.u32(uint32(b.Usage))
		w.u32(uint32(len(b.Data)))
		w.buf = append(w.buf, b.Data...)
	case driver.GPUOpWriteBuffer:
		w.u64(op.Offset)
		w.u32(uint32(len(op.Data)))
		w.buf = append(w.buf, op.Data...)
	case driver.GPUOpDestroyBuffer:
	case driver.GPUOpCreateTexture:
		var t driver.GPUTextureDesc
		if op.Texture != nil {
			t = *op.Texture
		}
		w.byte(byte(t.Dimension))
		w.u32(t.Width)
		w.u32(t.Height)
		w.u32(max(t.Layers, 1))
		w.u32(max(t.MipLevels, 1))
		w.u32(uint32(max(t.SampleCount, 1)))
		w.byte(byte(t.Format))
		w.u32(uint32(t.Usage))
	case driver.GPUOpWriteTexture:
		w.u32(op.TextureDst.MipLevel)
		w.u32(op.TextureDst.ArrayLayer)
		w.u32(op.TextureDst.OriginX)
		w.u32(op.TextureDst.OriginY)
		w.u64(op.DataLayout.Offset)
		w.u32(op.DataLayout.BytesPerRow)
		w.u32(op.DataLayout.RowsPerImage)
		w.u32(uint32(len(op.Data)))
		w.buf = append(w.buf, op.Data...)
	case driver.GPUOpDestroyTexture:
	case driver.GPUOpCreateSampler:
		var s driver.GPUSamplerDesc
		if op.Sampler != nil {
			s = *op.Sampler
		}
		w.byte(byte(s.MinFilter))
		w.byte(byte(s.MagFilter))
		w.byte(byte(s.MipmapFilter))
		w.byte(byte(s.AddressU))
		w.byte(byte(s.AddressV))
		w.byte(byte(s.AddressW))
		w.byte(byte(s.Compare))
		w.u16(s.MaxAnisotropy)
	case driver.GPUOpDestroySampler:
	case driver.GPUOpCreateBindGroupLayout:
		var entries []driver.GPUBindGroupLayoutEntry
		if op.BindGroupLayout != nil {
			entries = op.BindGroupLayout.Entries
		}
		w.u32(uint32(len(entries)))
		for _, e := range entries {
			w.u32(e.Binding)
			w.byte(byte(e.Visibility))
			w.byte(byte(e.Type))
			w.flags(e.HasDynamicOffset)
		}
	case driver.GPUOpDestroyBindGroupLayout:
	case driver.GPUOpCreateBindGroup:
		var bg driver.GPUBindGroupDesc
		if op.BindGroup != nil {
			bg = *op.BindGroup
		}
		w.u64(uint64(bg.Layout))
		w.u32(uint32(len(bg.Entries)))
		for _, e := range bg.Entries {
			w.u32(e.Binding)
			w.u64(uint64(e.Buffer))
			w.u64(e.Offset)
			w.u64(e.Size)
			w.u64(uint64(e.Texture))
			w.u64(uint64(e.Sampler))
		}
	case driver.GPUOpDestroyBindGroup:
	case driver.GPUOpCreatePipeline:
		src, err := compileGPUPipeline(op.Pipeline)
		if err != nil {
			return err
		}
		p := op.Pipeline
		w.str(src.VertexSource)
		w.str(src.FragmentSource)
		w.u32(uint32(len(src.UniformBlocks)))
		for _, ub := range src.UniformBlocks {
			w.u32(ub.Group)
			w.u32(ub.Binding)
			w.str(ub.BlockName)
			w.u32(ub.Slot)
		}
		w.u32(uint32(len(src.SamplerPairs)))
		for _, sp := range src.SamplerPairs {
			w.str(sp.Name)
			w.u32(sp.Unit)
			w.u32(sp.TextureGroup)
			w.u32(sp.TextureBinding)
			w.u32(sp.SamplerGroup)
			w.u32(sp.SamplerBinding)
		}
		w.u32(uint32(len(p.VertexBuffers)))
		for _, vb := range p.VertexBuffers {
			w.u64(vb.Stride)
			w.byte(byte(vb.StepMode))
			w.u32(uint32(len(vb.Attributes)))
			for _, a := range vb.Attributes {
				w.byte(byte(a.Format))
				w.u64(a.Offset)
				w.u32(a.Location)
			}
		}
		w.byte(byte(p.Primitive.Topology))
		w.byte(byte(p.Primitive.FrontFace))
		w.byte(byte(p.Primitive.CullMode))
		w.flags(p.DepthStencil != nil, p.DepthStencil != nil && p.DepthStencil.DepthWrite)
		if p.DepthStencil != nil {
			w.byte(byte(p.DepthStencil.Format))
			w.byte(byte(p.DepthStencil.DepthCompare))
			w.u32(uint32(p.DepthStencil.DepthBias))
			w.float(p.DepthStencil.DepthBiasSlopeScale)
		}
		w.u32(uint32(len(p.Targets)))
		for _, t := range p.Targets {
			w.byte(byte(t.Format))
			mask := t.WriteMask
			if mask == 0 {
				mask = driver.GPUColorWriteAll
			}
			w.byte(byte(mask))
			w.flags(t.Blend != nil)
			if t.Blend != nil {
				w.byte(byte(t.Blend.Color.SrcFactor))
				w.byte(byte(t.Blend.Color.DstFactor))
				w.byte(byte(t.Blend.Color.Operation))
				w.byte(byte(t.Blend.Alpha.SrcFactor))
				w.byte(byte(t.Blend.Alpha.DstFactor))
				w.byte(byte(t.Blend.Alpha.Operation))
			}
		}
		w.u32(uint32(max(p.SampleCount, 1)))
	case driver.GPUOpDestroyPipeline:
	case driver.GPUOpReadTexture:
		w.u64(op.ReadbackID)
		w.flags(op.ReadbackSurface)
		w.u32(op.ReadbackRegion.MipLevel)
		w.u32(op.ReadbackRegion.ArrayLayer)
		w.u32(op.ReadbackRegion.X)
		w.u32(op.ReadbackRegion.Y)
		w.u32(op.ReadbackRegion.Width)
		w.u32(op.ReadbackRegion.Height)
	default:
		return fmt.Errorf("android gpu: unknown resource op %d", op.Kind)
	}
	return nil
}

func writeGPUCopyOp(w *writer, cp *driver.GPUCopyOp) {
	w.byte(byte(cp.Kind))
	w.u64(uint64(cp.SrcID))
	w.u64(uint64(cp.DstID))
	switch cp.Kind {
	case driver.GPUCopyBufferToBuffer:
		w.u64(cp.SrcOffset)
		w.u64(cp.DstOffset)
		w.u64(cp.Size)
	case driver.GPUCopyBufferToTexture:
		w.u64(cp.SrcLayout.Offset)
		w.u32(cp.SrcLayout.BytesPerRow)
		w.u32(cp.SrcLayout.RowsPerImage)
		w.u32(cp.DstTex.MipLevel)
		w.u32(cp.DstTex.ArrayLayer)
		w.u32(cp.DstTex.OriginX)
		w.u32(cp.DstTex.OriginY)
		w.u32(cp.Width)
		w.u32(cp.Height)
		w.u32(max(cp.Layers, 1))
	case driver.GPUCopyTextureToTexture:
		w.u32(cp.SrcTex.MipLevel)
		w.u32(cp.SrcTex.ArrayLayer)
		w.u32(cp.SrcTex.OriginX)
		w.u32(cp.SrcTex.OriginY)
		w.u32(cp.DstTex.MipLevel)
		w.u32(cp.DstTex.ArrayLayer)
		w.u32(cp.DstTex.OriginX)
		w.u32(cp.DstTex.OriginY)
		w.u32(cp.Width)
		w.u32(cp.Height)
		w.u32(max(cp.Layers, 1))
	}
}

func writeGPUPass(w *writer, pass *driver.GPUPass) {
	w.u32(uint32(len(pass.ColorAttachments)))
	for _, ca := range pass.ColorAttachments {
		w.u64(uint64(ca.Texture))
		w.u64(uint64(ca.ResolveTarget))
		w.u32(ca.MipLevel)
		w.u32(ca.ArrayLayer)
		w.byte(byte(ca.LoadOp))
		w.byte(byte(ca.StoreOp))
		w.u32(uint32(ca.ClearColor))
	}
	w.flags(pass.DepthStencil != nil)
	if ds := pass.DepthStencil; ds != nil {
		w.u64(uint64(ds.Texture))
		w.byte(byte(ds.DepthLoadOp))
		w.byte(byte(ds.DepthStoreOp))
		w.float(ds.ClearDepth)
		w.byte(byte(ds.StencilLoadOp))
		w.byte(byte(ds.StencilStoreOp))
		w.u32(ds.ClearStencil)
	}
	w.u32(uint32(len(pass.Commands)))
	for i := range pass.Commands {
		c := &pass.Commands[i]
		w.byte(byte(c.Kind))
		switch c.Kind {
		case driver.GPUCmdSetViewport:
			w.floats(c.F0, c.F1, c.F2, c.F3, c.F4, c.F5)
		case driver.GPUCmdSetScissor:
			w.u32(uint32(c.I0))
			w.u32(uint32(c.I1))
			w.u32(uint32(c.I2))
			w.u32(uint32(c.I3))
		case driver.GPUCmdSetPipeline:
			w.u64(uint64(c.ID))
		case driver.GPUCmdSetBindGroup:
			w.u32(c.Slot)
			w.u64(uint64(c.ID))
			w.u32(uint32(len(c.DynamicOffsets)))
			for _, off := range c.DynamicOffsets {
				w.u32(off)
			}
		case driver.GPUCmdSetVertexBuffer:
			w.u32(c.Slot)
			w.u64(uint64(c.ID))
			w.u64(c.Offset)
		case driver.GPUCmdSetIndexBuffer:
			w.u64(uint64(c.ID))
			w.byte(byte(c.IndexFormat))
			w.u64(c.Offset)
		case driver.GPUCmdSetStencilReference:
			w.u32(c.U0)
		case driver.GPUCmdSetBlendConstant:
			w.floats(c.F0, c.F1, c.F2, c.F3)
		case driver.GPUCmdDraw:
			w.u32(c.U0)
			w.u32(c.U1)
		case driver.GPUCmdDrawIndexed:
			w.u32(c.U0)
			w.u32(c.U1)
			w.u32(uint32(c.I0))
		case driver.GPUCmdDrawInstanced:
			w.u32(c.U0)
			w.u32(c.U1)
			w.u32(c.U2)
			w.u32(c.U3)
		case driver.GPUCmdDrawIndexedInstanced:
			w.u32(c.U0)
			w.u32(c.U1)
			w.u32(c.U2)
			w.u32(c.U3)
			w.u32(uint32(c.I0))
		}
	}
}

// readGPUEvent decodes a msgGPUEvent sent by GpuView.java into a surface ID and a driver.Event carrying *driver.GPUEvent.
func readGPUEvent(r *reader) (uint32, driver.Event) {
	surfID := r.u32()
	viewID := driver.NodeID(r.u32())
	ge := driver.GPUEvent{
		Kind:       driver.GPUEventKind(r.byte()),
		Generation: r.u32(),
	}
	switch ge.Kind {
	case driver.GPUEventDeviceReady:
		ge.Width = int(r.u32())
		ge.Height = int(r.u32())
		ge.SurfaceFormat = driver.GPUTextureFormat(r.byte())
		ge.Limits = driver.GPULimits{
			MaxTextureDimension2D:     r.u32(),
			MaxBufferSize:             r.u64(),
			MaxUniformBufferSize:      r.u64(),
			MaxBindGroups:             r.u32(),
			MaxVertexBuffers:          r.u32(),
			MaxVertexAttributes:       r.u32(),
			MinUniformBufferAlignment: r.u32(),
		}
		feats := r.byte()
		ge.Features = driver.GPUFeatures{
			StorageBuffers: feats&1 != 0,
			Compute:        feats&2 != 0,
			MSAA4x:         feats&4 != 0,
			Depth32Float:   feats&8 != 0,
			Anisotropy:     feats&16 != 0,
		}
	case driver.GPUEventResize:
		ge.Width = int(r.u32())
		ge.Height = int(r.u32())
	case driver.GPUEventDeviceLost:
		ge.LostReason = driver.GPUDeviceLostReason(r.byte())
		ge.Message = r.str()
	case driver.GPUEventError:
		ge.ErrorCode = driver.GPUErrorCode(r.byte())
		ge.Message = r.str()
	case driver.GPUEventReadback:
		ge.ReadbackID = r.u64()
		ge.Message = r.str()
		pix := r.bytes()
		if len(pix) > 0 {
			ge.Pixels = append([]byte(nil), pix...)
		}
	}
	return surfID, driver.Event{
		Kind: driver.EventGPU,
		ID:   viewID,
		Data: &ge,
	}
}
