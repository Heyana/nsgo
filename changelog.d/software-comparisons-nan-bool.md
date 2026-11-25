### Fixed

- Compare as WGSL and the GPU backends do on the software device: a NaN is ordered by nothing, equal to nothing and unequal to itself, and `==` / `!=` on bools compare their values (#2335 by @Laotree).
