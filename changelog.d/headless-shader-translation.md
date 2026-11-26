### Added

- `nsgotest`, `gpu`: add `nsgotest.TranslateShader`, `nsgotest.ShaderSources`, and `gpu.TranslateShaderModule` (`gpu.PipelineSources`) to translate fragment and 3D shaders headlessly across MSL, HLSL, GLSL, GLES, and WGSL while reporting per-backend errors and recovered panics (#2266).

### Fixed

- `gpu`: report shader translator panics and pipelines that produce no backend source as compilation errors instead of silently submitting empty artifacts.
