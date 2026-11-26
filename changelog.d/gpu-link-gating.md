### Changed

- Link a backend's MSL, HLSL or GLSL writer only when an app mounts a `core.GPUView` or asks for a glass effect, through `driver.GPUEnabler`, `driver.GPUSubmitter` and `driver.GlassEnabler` tables that hold the item, resize, frame, release and effect entry points (by @Laotree).
