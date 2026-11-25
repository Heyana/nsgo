### Fixed

- Dereference a closed `BindGroup` or `BindGroupLayout` from its device's restore lists at once, so a frame that creates and closes them no longer grows the device's memory for its lifetime (#2346 by @Laotree).
