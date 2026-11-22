# Watch authentication fixture

This opt-in fixture uses the public `AuthSession` API through the watch SwiftUI host. A local synthetic provider validates an S256 request, returns code and state through an app-owned callback scheme, checks the verifier exchange, and serves a second page until the app cancels the session. No account or real token is used.

Run on an isolated watch simulator:

```sh
go tool nsgo test -target watchos_simulator -device <owned-watch-id> -env NSGO_TEST_WATCH_AUTH=1 ./internal/testhost/testdata/watchauth
```

Accept only this fixture’s system consent sheet if one appears. A provider connection or presentation failure in an unpaired simulator does not establish physical-watch behavior. A signed physical-watch run and user dismissal of the authentication sheet remain separate acceptance checks.

`TestTVAuthSessionUnavailable` runs with `-target tvos_simulator` on its own owned TV simulator and asserts that TV answers unavailable before calling the authorization URL builder. Its builder returns an empty string even during a regression, so that check never opens a provider or an authentication UI.
