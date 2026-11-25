### Fixed

- `gpu`: a render pass whose depth attachment names no texture, the surface's own depth buffer, takes the surface's depth format again, so depth-tested pipelines draw in it instead of being skipped (#2395).
