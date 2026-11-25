### Added

- Run an unknown command `nsgo X` as an extension: the module's `go.mod` tool `X` through `go tool`, else `nsgo-X` on `PATH`, with its arguments, streams and exit status, and list the extensions found in the usage (#2366 by @houko).
- Add `nsgo game`, which runs the game layer's command, the module's tool or else `go run` of its release, with `NSGO_GAME_VERSION` to choose another (#2366 by @houko).
- Run the module's pinned `go tool nsgo` in place of an `nsgo` of another version, with `NSGO_NO_DELEGATE=1` to opt out, and add `nsgo version`, which says which one ran (#2366 by @houko).
