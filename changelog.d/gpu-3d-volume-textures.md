### Added

- Support 3D volume textures (`gpu.Dim3D`, `TextureDesc.Depth`, `TextureCopy.OriginZ`, `TextureWrite.OriginZ`/`Depth`, `Device.SupportsTexture3D`, `Limits.MaxTextureDimension3D`, and WGSL `texture_3d<f32>`) across Metal, Direct3D 11, OpenGL, WebGPU, WebGL 2, Android GLES 3.0, and the software GPU device (#2655).
