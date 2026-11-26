package gpu

import (
	"sync/atomic"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// liveScopeCleanups counts the cleanups the package has registered on scopes and not taken off or run, by wrapping core's hook for the test.
func liveScopeCleanups(t *testing.T) *atomic.Int64 {
	orig := driver.ScopeCleanup
	live := new(atomic.Int64)
	driver.ScopeCleanup = func(scope any, fn func()) func() {
		live.Add(1)
		var done atomic.Bool
		end := func() {
			if done.CompareAndSwap(false, true) {
				live.Add(-1)
			}
		}
		remove := orig(scope, func() { end(); fn() })
		return func() { end(); remove() }
	}
	t.Cleanup(func() { driver.ScopeCleanup = orig })
	return live
}

// A frame that creates and closes bind groups must not grow the device for its lifetime: a closed group and its layout leave the lists a restore walks.
func TestClosedBindGroupsLeaveTheRestoreLists(t *testing.T) {
	d := newDevice(nil, nil, 1, 64, 64, FormatRGBA8Unorm, ColorSpaceDefault, 2, Limits{}, Features{}, nil)
	layouts, groups := len(d.retainedLayouts), len(d.retainedBindGroups)
	for range 16 {
		l, err := d.CreateBindGroupLayout(nil, nil)
		if err != nil {
			t.Fatalf("CreateBindGroupLayout = %v", err)
		}
		g, err := d.CreateBindGroupDesc(nil, BindGroupDesc{Layout: l})
		if err != nil {
			t.Fatalf("CreateBindGroupDesc = %v", err)
		}
		if err := g.Close(); err != nil {
			t.Fatalf("BindGroup.Close = %v", err)
		}
		if err := l.Close(); err != nil {
			t.Fatalf("BindGroupLayout.Close = %v", err)
		}
	}
	if got := len(d.retainedLayouts); got != layouts {
		t.Fatalf("retainedLayouts after the closes = %d, want %d", got, layouts)
	}
	if got := len(d.retainedBindGroups); got != groups {
		t.Fatalf("retainedBindGroups after the closes = %d, want %d", got, groups)
	}
}

// A restore rebuilds every open bind group and layout and none that a close removed.
func TestRestoreKeepsOnlyOpenBindGroups(t *testing.T) {
	d := newDevice(nil, nil, 1, 64, 64, FormatRGBA8Unorm, ColorSpaceDefault, 2, Limits{}, Features{}, nil)
	d.retainResources = true
	openLayout, err := d.CreateBindGroupLayout(nil, nil)
	if err != nil {
		t.Fatalf("CreateBindGroupLayout = %v", err)
	}
	openGroup, err := d.CreateBindGroupDesc(nil, BindGroupDesc{Layout: openLayout})
	if err != nil {
		t.Fatalf("CreateBindGroupDesc = %v", err)
	}
	closedLayout, err := d.CreateBindGroupLayout(nil, nil)
	if err != nil {
		t.Fatalf("CreateBindGroupLayout = %v", err)
	}
	closedGroup, err := d.CreateBindGroupDesc(nil, BindGroupDesc{Layout: closedLayout})
	if err != nil {
		t.Fatalf("CreateBindGroupDesc = %v", err)
	}
	if err := closedGroup.Close(); err != nil {
		t.Fatalf("BindGroup.Close = %v", err)
	}
	if err := closedLayout.Close(); err != nil {
		t.Fatalf("BindGroupLayout.Close = %v", err)
	}

	restored := newDevice(nil, nil, 2, 64, 64, FormatRGBA8Unorm, ColorSpaceDefault, 2, Limits{}, Features{}, nil)
	restored.retainResources = true
	restored.restoreRetainedFrom(d)

	if len(restored.retainedLayouts) != 1 || restored.retainedLayouts[0] != openLayout {
		t.Fatalf("restored layouts = %v, want the open one", restored.retainedLayouts)
	}
	if len(restored.retainedBindGroups) != 1 || restored.retainedBindGroups[0] != openGroup {
		t.Fatalf("restored bind groups = %v, want the open one", restored.retainedBindGroups)
	}
	if !openGroup.Valid() {
		t.Fatal("the restored bind group is not valid on the new device")
	}
}

// Creating and closing resources N times under one scope keeps both the scope's cleanups and the device's retained lists at a constant size (#2339).
func TestClosedResourcesLeaveScopeAndRetainedLists(t *testing.T) {
	live := liveScopeCleanups(t)
	cx := core.NewRoot(core.RootOptions{})
	defer cx.Dispose()

	d := newDevice(nil, cx, 1, 64, 64, FormatRGBA8Unorm, ColorSpaceDefault, 2, Limits{MaxBindGroups: 4}, Features{
		TimestampQueries: true,
		Compute:          true,
	}, nil)
	d.retainResources = true

	const vertWGSL = `
@vertex
fn vs_main(@builtin(vertex_index) idx: u32) -> @builtin(position) vec4<f32> {
	return vec4<f32>(0.0, 0.0, 0.0, 1.0);
}
`
	const fragWGSL = `
@fragment
fn fs_main() -> @location(0) vec4<f32> {
	return vec4<f32>(1.0, 0.0, 0.0, 1.0);
}
`
	const compWGSL = `
@compute @workgroup_size(1)
fn cs_main() {}
`
	vs, err := CompileShader(ShaderDesc{Source: vertWGSL})
	if err != nil {
		t.Fatalf("CompileShader vert: %v", err)
	}
	fs, err := CompileShader(ShaderDesc{Source: fragWGSL})
	if err != nil {
		t.Fatalf("CompileShader frag: %v", err)
	}
	cs, err := CompileShader(ShaderDesc{Source: compWGSL})
	if err != nil {
		t.Fatalf("CompileShader comp: %v", err)
	}

	for range 20 {
		buf, err := d.CreateBuffer(cx, BufferDesc{Size: 16, Usage: UsageVertex, Retained: true})
		if err != nil {
			t.Fatalf("CreateBuffer: %v", err)
		}
		rt, err := d.CreateRenderTarget(cx, RenderTargetDesc{
			Width:       32,
			Height:      32,
			ColorFormat: FormatRGBA8Unorm,
			DepthFormat: FormatDepth24Plus,
			Retained:    true,
		})
		if err != nil {
			t.Fatalf("CreateRenderTarget: %v", err)
		}
		samp, err := d.CreateSampler(cx, SamplerDesc{Retained: true})
		if err != nil {
			t.Fatalf("CreateSampler: %v", err)
		}
		layout, err := d.CreateBindGroupLayout(cx, nil)
		if err != nil {
			t.Fatalf("CreateBindGroupLayout: %v", err)
		}
		bg, err := d.CreateBindGroupDesc(cx, BindGroupDesc{Layout: layout})
		if err != nil {
			t.Fatalf("CreateBindGroupDesc: %v", err)
		}
		pipe, err := d.CreatePipeline(cx, PipelineDesc{
			Vertex:   ShaderStage{Module: vs},
			Fragment: ShaderStage{Module: fs},
			Retained: true,
		})
		if err != nil {
			t.Fatalf("CreatePipeline: %v", err)
		}
		cp, err := d.CreateComputePipeline(cx, ComputePipelineDesc{
			Compute: ShaderStage{Module: cs},
		})
		if err != nil {
			t.Fatalf("CreateComputePipeline: %v", err)
		}
		tq, err := d.CreateTimestampQuery(cx, "pass")
		if err != nil {
			t.Fatalf("CreateTimestampQuery: %v", err)
		}
		rb, err := d.CreateRingBuffer(cx, RingBufferDesc{
			Usage:    UsageUniform,
			SlotSize: 64,
		})
		if err != nil {
			t.Fatalf("CreateRingBuffer: %v", err)
		}

		for _, c := range []interface{ Close() error }{rb, tq, cp, pipe, bg, layout, samp, rt, buf} {
			if err := c.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
		}
	}

	if got := live.Load(); got != 0 {
		t.Fatalf("closed resources left %d cleanups on the scope, want 0", got)
	}
	if len(d.retainedBuffers) != 0 || len(d.retainedTextures) != 0 ||
		len(d.retainedSamplers) != 0 || len(d.retainedLayouts) != 0 ||
		len(d.retainedBindGroups) != 0 || len(d.retainedPipelines) != 0 {
		t.Fatalf("retained lists not empty: bufs=%d texs=%d samps=%d layouts=%d bgs=%d pipes=%d",
			len(d.retainedBuffers), len(d.retainedTextures), len(d.retainedSamplers),
			len(d.retainedLayouts), len(d.retainedBindGroups), len(d.retainedPipelines))
	}
}

// Disposing a scope closes every resource still open on it, last created first, and leaves later Close calls as no-ops.
func TestScopeDisposalClosesRemainingResources(t *testing.T) {
	live := liveScopeCleanups(t)
	root := core.NewRoot(core.RootOptions{})
	defer root.Dispose()
	child := root.Child()

	d := newDevice(nil, root, 1, 64, 64, FormatRGBA8Unorm, ColorSpaceDefault, 2, Limits{}, Features{}, nil)

	closedEarly, err := d.CreateBuffer(child, BufferDesc{Size: 16, Usage: UsageVertex, Retained: true})
	if err != nil {
		t.Fatalf("CreateBuffer early: %v", err)
	}
	if err := closedEarly.Close(); err != nil {
		t.Fatalf("Close early: %v", err)
	}

	openBuf, err := d.CreateBuffer(child, BufferDesc{Size: 16, Usage: UsageVertex, Retained: true})
	if err != nil {
		t.Fatalf("CreateBuffer open: %v", err)
	}
	openTex, err := d.CreateTexture(child, TextureDesc{Width: 4, Height: 4, Format: FormatRGBA8Unorm, Usage: TextureSampled, Retained: true})
	if err != nil {
		t.Fatalf("CreateTexture open: %v", err)
	}

	if got := live.Load(); got != 2 {
		t.Fatalf("live cleanups before Dispose = %d, want 2", got)
	}
	ops := len(d.pendingOps)
	child.Dispose()

	if got := d.pendingOps[ops:]; len(got) != 2 || got[0].Kind != driver.GPUOpDestroyTexture || got[1].Kind != driver.GPUOpDestroyBuffer {
		t.Fatalf("Dispose destroyed %v, want the texture, then the buffer", got)
	}
	if openBuf.Valid() || !openBuf.closed {
		t.Fatal("openBuf should be closed after scope disposal")
	}
	if openTex.Valid() || !openTex.closed {
		t.Fatal("openTex should be closed after scope disposal")
	}
	if got := live.Load(); got != 0 {
		t.Fatalf("live cleanups after Dispose = %d, want 0", got)
	}
	if len(d.retainedBuffers) != 0 || len(d.retainedTextures) != 0 {
		t.Fatalf("retained lists after Dispose: bufs=%d texs=%d, want 0", len(d.retainedBuffers), len(d.retainedTextures))
	}
	if err := openBuf.Close(); err != nil {
		t.Fatalf("post-dispose Close(openBuf): %v", err)
	}
	if err := openTex.Close(); err != nil {
		t.Fatalf("post-dispose Close(openTex): %v", err)
	}
}

// Resources created while an effect runs belong to the effect, as Scope.Cleanup's do, whichever scope is passed: they close when it runs again, and do not take a resource created under the scope outside it with them.
func TestResourcesCreatedInAnEffectBelongToIt(t *testing.T) {
	live := liveScopeCleanups(t)
	var (
		d        *Device
		inEffect []*Buffer
		long     *Buffer
		size     *core.Signal[int]
	)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		size = core.NewSignal(cx, 16)
		return View(cx, ViewProps{
			TestID: "effect-gpu",
			Style:  core.Style{Width: core.Pt(32), Height: core.Pt(32)},
			OnReady: func(dev *Device) {
				d = dev
				cx.Effect(func() {
					b, err := d.CreateBuffer(cx, BufferDesc{Size: uint64(size.Get()), Usage: UsageVertex})
					if err != nil {
						t.Errorf("CreateBuffer in effect: %v", err)
						return
					}
					inEffect = append(inEffect, b)
				})
				var err error
				if long, err = d.CreateBuffer(cx, BufferDesc{Size: 16, Usage: UsageVertex}); err != nil {
					t.Errorf("CreateBuffer: %v", err)
				}
			},
		})
	}, nsgotest.Size(32, 32))
	app.Settle()
	if d == nil || len(inEffect) != 1 || long == nil {
		t.Fatalf("device ready %v, effect buffers %d, long-lived %v", d != nil, len(inEffect), long != nil)
	}
	base := live.Load()
	for i := range 5 {
		app.Post(func() { size.Set(32 + 16*i) })
		app.Settle()
	}
	if len(inEffect) != 6 {
		t.Fatalf("effect ran %d times, want 6", len(inEffect))
	}
	for i, b := range inEffect[:5] {
		if b.Valid() {
			t.Fatalf("buffer %d from an earlier effect run is still open", i)
		}
	}
	if !inEffect[5].Valid() || !long.Valid() {
		t.Fatalf("open buffers closed: latest effect's %v, long-lived %v", inEffect[5].Valid(), long.Valid())
	}
	if got := live.Load(); got != base {
		t.Fatalf("live cleanups went from %d to %d over the effect's runs", base, got)
	}
}

// Losing and restoring the device again and again leaves the view's scope holding what it held after the first restore, as a restore reuses each resource's cleanup instead of registering a new one per device.
func TestRepeatedDeviceLossKeepsTheScopeBounded(t *testing.T) {
	live := liveScopeCleanups(t)
	var kept *Buffer
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return View(cx, ViewProps{
			TestID:          "loss-gpu",
			RetainResources: true,
			Style:           core.Style{Width: core.Pt(32), Height: core.Pt(32)},
			OnReady: func(d *Device) {
				if d.Epoch() > 1 {
					return
				}
				var err error
				if kept, err = d.CreateBuffer(cx, BufferDesc{Size: 16, Usage: UsageVertex}); err != nil {
					t.Errorf("CreateBuffer: %v", err)
				}
			},
		})
	}, nsgotest.Size(32, 32))
	app.TriggerGPUDeviceLostWithReason("id:loss-gpu", nsgotest.GPULostReset, "reset")
	app.RestoreGPUDevice("id:loss-gpu")
	base := live.Load()
	for range 5 {
		app.TriggerGPUDeviceLostWithReason("id:loss-gpu", nsgotest.GPULostReset, "reset")
		app.RestoreGPUDevice("id:loss-gpu")
	}
	if kept == nil || !kept.Valid() || kept.Epoch() != 7 {
		t.Fatalf("retained buffer not restored: %v", kept)
	}
	if got := live.Load(); got != base {
		t.Fatalf("live cleanups went from %d to %d over five losses", base, got)
	}
}
