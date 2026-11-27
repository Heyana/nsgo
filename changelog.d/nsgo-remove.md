### Added

- `nsgo remove name|@registry/name|module…`, which resolves built-in index and custom registry names, refuses copied components and libraries still imported by any package under any build tag, and previews `go mod tidy` on temporary copies of `go.mod` and `go.sum` before confirming and applying, marking unrelated tidied lines and explaining libraries kept by another dependency or tool (#2465).
