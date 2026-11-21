### Added

- Add the Windows Direct3D 11 backend for `gpu.View` via pure-Go `syscall` COM vtables and `D3DCompile`, compositing GPU surfaces through Direct2D with off-commit `SubmitGPU` frames, 16-byte-aligned constant buffers, `DXGI_FORMAT_D32_FLOAT` depth testing, device-removed recovery, and COM release on unmount (#1106 by @houko).
