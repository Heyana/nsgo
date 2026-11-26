### Added

- `nsgo add module[@version]…`, a `go get` that prints its plan first, planned on copies of `go.mod` and `go.sum` with the module's own toolchain and module proxies only for public modules, and writes both only once confirmed in a terminal or with `-y`, refusing to move the app's NSGo off its releases (#2460).
