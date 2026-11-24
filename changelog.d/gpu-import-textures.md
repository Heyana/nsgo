### Added

- Sample hardware decoders' frames in `gpu.View` without copying them, through `Device.ImportTexture`: Direct3D 11 shared textures on Windows and DMA-BUFs on Linux (#2296 by @oneo-me).
