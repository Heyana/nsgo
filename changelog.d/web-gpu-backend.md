### Added

- Add the web `gpu.View` backend (`internal/platform/web`) with WebGPU (`navigator.gpu` + WGSL) primary rendering and automatic WebGL 2 (`GLSL ES 3.00`) fallback on `<canvas>`, supporting off-commit `SubmitGPU`, full retained resource lifecycles, offscreen/MSAA/depth passes, async readback, `GPUDevice.lost` and `webglcontextlost`/`webglcontextrestored` recovery, and `Env.SupportsGPU()` detection (#1108).
