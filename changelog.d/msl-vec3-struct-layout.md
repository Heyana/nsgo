### Fixed

- `msl`: lay out `vec3<f32>`, `vec3<i32>`, and `vec3<u32>` members of uniform and storage buffer structs as packed Metal vectors with explicit WGSL padding so following scalars and array strides match WGSL on Metal (#2340).
