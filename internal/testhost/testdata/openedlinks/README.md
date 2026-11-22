# Opened links on Android and the web

Build this owned fixture from the repository root after rebuilding Java:

```sh
PATH="$JAVA_HOME/bin:$PATH" ANDROID_HOME=<sdk> internal/platform/android/runtime/build.sh
go tool nsgo build -target android_arm64 internal/testhost/testdata/openedlinks
```

Install its APK on an owned Android device. For a routing test, approve only this fixture's declared synthetic domain, then deliver an implicit HTTPS intent without naming a package or component:

```sh
adb -s <serial> shell pm set-app-links --package dev.nsgo.opened1475 2 links1475.example.com
adb -s <serial> shell "am start -W -a android.intent.action.VIEW -c android.intent.category.BROWSABLE -d 'https://links1475.example.com/orders/%E4%B8%AD?table=12&x=a%20b#cold'"
adb -s <serial> shell "am start -W -a android.intent.action.VIEW -c android.intent.category.BROWSABLE -d 'https://links1475.example.com/orders/next?table=24#expanded'"
adb -s <serial> shell "am start -W -a android.intent.action.VIEW -c android.intent.category.BROWSABLE -d 'nsgo1475://order/private?value=%E4%B8%AD#private'"
```

`FIRST1475` must already contain the complete first URL before the root's first render. Every `OPEN1475` must show the same URL in `current`, and warm delivery must retain the same Go PID. An explicit intent to this fixture with an undeclared HTTPS host must leave its last URL unchanged. Home and reopening the fixture must preserve it through environment updates. Uninstall `dev.nsgo.opened1475` afterward, which removes its temporary domain approval. Leave user notification and other device policy unchanged.

On 2026-11-23 these checks passed on a physical Pixel 3 XL, Android 12/API 31: implicit HTTPS intents resolved to `NsgoActivity`, first render and warm callbacks carried the complete addresses, and all deliveries retained Go PID 19406. The undeclared host was ignored and background/reopen retained the private-scheme URL. Domain approval was injected for this isolated package; this proves Android intent routing, not a public website's Digital Asset Links hosting or network verification.

Browser coverage uses the actual page address, hash and History APIs:

```sh
go tool nsgo test -target web -run 'TestOpenedURL|TestEmbeddedOpenedURL|TestPageAddress' ./internal/platform/web -- -count=3
```

It checks the initial root read, hash changes, app writes, back/forward traversal and an embedded surface that leaves the containing address out. No Android runtime binaries belong in the PR; CI rebuilds Java and maintainers publish them separately.

The browser run passed 21 checks. Compiling the unchanged environment/history sources through a Go compiler overlay makes the new first-render regression fail with an empty URL. The existing reload regression also failed against those sources: two consecutive players had identical timestamp-based session IDs. Fresh random session IDs make the reload and address checks pass through three runs each.
