### Fixed

- Release a key held with `KeyCodeDown` by its name and code when `nsgotest`'s `BlurWindow` takes the focus away, so `core.KeyCodesDown` empties and the focused view's `OnKeyUp` hears it (#2248 by @Laotree).
