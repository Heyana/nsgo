### Changed

- Link a backend's MSL, HLSL or GLSL writer only when an app mounts a `core.GPUView`, through `driver.GPUEnabler` and `driver.GPUSubmitter` tables that hold the item, resize, frame and release entry points (#2508 by @Laotree).
