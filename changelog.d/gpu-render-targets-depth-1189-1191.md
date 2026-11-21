### Added

- Support offscreen color, color+depth, and depth-only `gpu.RenderTarget` passes (`LoadClear`/`LoadLoad`), readable `texture_depth_2d` textures (`textureSample`, `textureSampleLevel`, `textureLoad`, depth copy and readback), and shadow-map comparison samplers (`sampler_comparison`, `textureSampleCompare`, `textureSampleCompareLevel`, 2x2 PCF, and depth bias) across `gpu`, `internal/shader/wgsl`, `internal/shaderir` MSL/HLSL/GLSL/WGSL emitters, and the headless software renderer (#1189, #1191).
