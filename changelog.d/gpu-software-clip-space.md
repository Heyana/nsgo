### Fixed

- `gpu`: clip triangles in homogeneous clip space, clamp fragment depth to the viewport depth range, cover shared edges once with a top-left fill rule, and interpolate `@interpolate(linear)` varyings in screen space in the software renderer (#2344 by @owocc).
