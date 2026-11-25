### Added

- `BindGroupLayoutDesc.Retained`, so a bind group layout opts into recreation after device loss on its own, as the other resources do, instead of only through `ViewProps.RetainResources` (#2338 by @Laotree).
