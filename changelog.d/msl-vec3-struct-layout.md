### Fixed

- `msl`: lay out `vec3<f32>`, `vec3<i32>`, and `vec3<u32>` members of uniform and storage buffer structs as packed Metal vectors with explicit WGSL padding so following scalars and array strides match WGSL on Metal (#2340 by @owocc).
- WGSL: refuse uniform buffer arrays whose elements are not a multiple of 16 bytes apart, as WGSL requires, rather than reading them at other offsets on Metal (#2403).
