### Fixed

- Refuse a pipeline at `RenderPass.SetPipeline` whose colour targets or depth format do not fit the pass's attachments, skipping its draws instead of recording draws a strict backend rejects (#2341 by @Laotree).
