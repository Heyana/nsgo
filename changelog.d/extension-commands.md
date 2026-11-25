### Added

- Run an unknown command `nsgo X` as an extension: the module's `go.mod` tool `X` through `go tool`, else `nsgo-X` on `PATH`, with its arguments, streams and exit status, and list the extensions found in the usage (#2366 by @houko).
