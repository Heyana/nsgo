### Added

- Declare `ui`'s eight subpackage components (`chat`, `code-editor`, `commit-graph`, `diff-view`, `file-picker`, `flow-diagram`, `rich-editor` and `terminal`) in `nsgo-module.yaml`, list them in `cmd/nsgo/registry.yaml` for `nsgo add -list`, and drop `internal/driver` test imports so they copy cleanly into apps (#2471).
