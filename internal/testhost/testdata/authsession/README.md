# Native authentication fixture

`TestAuthSessionProviderAndCancel` exercises the public API through the native backend. Its local test provider validates an S256 authorization request, redirects with synthetic code and state, and checks the verifier at its token endpoint. A second session reaches a waiting provider page before the app cancels it. It never signs into an account or exchanges a real token.

On an isolated lab desktop with its own browser profile:

```sh
go tool nsgo test -target linux -env NSGO_TEST_AUTH_BROWSER=1 ./internal/testhost/testdata/authsession
```

Use `windows` or `macos` for those desktops. On macOS or an iOS simulator, add `-env NSGO_TEST_AUTH_SCHEME=nsgo-auth1396` to use `ASWebAuthenticationSession`; accept only this fixture's system consent sheet. Physical iPhone acceptance still needs a signed install and native run.

Add `-env NSGO_TEST_AUTH_PATH=/登录/../戻る` on a desktop loopback run to exercise URL escaping and dot-segment normalization through the real browser too.

Use an isolated desktop/browser profile because default-browser loopback sessions may leave their two test tabs open. Closing such a tab does not notify a desktop app; only the explicit cancel or timeout ends the session. The fixture verifies app cancellation, not the user closing an Apple sheet. Run the browser backend's `TestAuthPopupCallbackAndClose` separately for actual popup navigation and user close detection.
