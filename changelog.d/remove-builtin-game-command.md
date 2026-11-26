### Removed

- Remove the built-in `nsgo game` command and `NSGO_GAME_VERSION`; `nsgo game` now runs through NSGo's standard extension mechanism (`go tool` for a module's `go.mod` tool named `game`, or `nsgo-game` on `PATH`).
