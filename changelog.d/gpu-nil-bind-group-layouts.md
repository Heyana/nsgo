### Added

- `gpu`: `PipelineDesc.Layouts` and `ComputePipelineDesc.Layouts` accept `nil` entries to leave unused `@group(N)` slots empty without calling `SetBindGroup` (#2337).
