### Added

- `nsgo test -json`, which writes `go test`'s event stream to stdout for tools such as `benchstat`.

### Fixed

- Report every repetition of a benchmark under `-count`, and every repetition in the JUnit report, instead of only the first (#2273 by @Laotree).
