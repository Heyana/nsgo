### Added

- Add GPU instanced drawing (`VertexStepInstance`, `VertexBufferLayout.StepRate`, `@builtin(instance_index)` across WGSL/MSL/HLSL/GLSL emitters and software rasterization, `DrawInstanced` and `DrawIndexedInstanced` with `firstInstance`) and stall-free per-frame buffer pipelining (`ViewProps.FramesInFlight` / `Device.FramesInFlight`, `Device.CreateRingBuffer` with aligned `Alloc`/`Write` and dynamic uniform offsets in `SetBindGroup`, and `FrameFence` / `BeginFrame` / `EndFrame` slot synchronization) (#1193, #1237). (#1193, #1237 by @houko).
