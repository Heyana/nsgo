---
title: Development setup
description: Build NSGo from source, run the gallery and test a change.
group: Contributing
order: 80
---

## Requirements

macOS with Xcode and Go 1.27.3 or later builds every target for arm64, Android's APK included. Running on devices needs what [Setup](getting-started.md#requirements) lists, and the x86_64 Android emulator's target, `android_x64`, an Android NDK, since Go links android/amd64 only with cgo. The Windows installer needs NSIS (`brew install makensis`); without it `build` leaves its script.

```sh
git clone https://github.com/nsgo-dev/nsgo.git
cd nsgo
```

Read [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) and [AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md) before changing code.

## The gallery

The gallery has a demo of each element, `base` and `ui` component and service. A wide window shows the demos under tabs in a sidebar; a narrow one shows the tabs and the settings in a bottom bar, with demos pushed over them. Run it from the repository's root:

```sh
go tool nsgo dev examples/gallery                   # nsgo.yaml's dev target, exiting with the app
go tool nsgo dev examples/gallery-tsx               # the gallery's components and apps, written in TSX
go tool nsgo dev -target web examples/gallery       # in the browser, serving until interrupted
go tool nsgo dev -target ios_simulator examples/gallery
go tool nsgo dev -target ios examples/gallery       # on the connected iPhone, signed by Xcode
go tool nsgo dev -target android examples/gallery   # on the connected device or emulator, over adb
go tool nsgo build -target all examples/gallery     # every platform's packages in examples/gallery/dist, what to hand out in dist/releases
```

## Check and test

```sh
go tool nsgo check                 # vet every package
go tool nsgo check -target all     # vet every available Go/Xcode target instead
go tool nsgo test                  # every package's tests
go tool nsgo test -bench . ./core  # core's benchmarks
(cd benchmark && go run ./runner)  # measure against other frameworks
# capacity curve for NSGo's 600-cell grid, without rebuilding it
(cd benchmark && go run ./runner -frameworks nsgo -cases grid -grid-cols 20 -grid-rows 15 -out /tmp/grid.json)
```

`-target` vets the targets it names instead of the host and, in NSGo's repository, the dev client, which the plain `check` vets. To vet the platforms a change touches, name them all in one check, such as `check -target web,android -jobs 2`, rather than starting several: `-jobs n` vets up to n targets at once, each `go vet` with n compile workers, where the default is one target at a time with Go's default, and a check started while another runs in the same module waits for it and says so. CI's `check.yml` vets every target.

Run at the repository root, or in `internal/dev`, the headless command tests `internal/dev` a second time with the `nsgodev` build tag, which the dev client's files need, so its tests run locally and in CI. Both runs take the same filters and flags and share one tally and JUnit report, where the second run's package is named `internal/dev [nsgodev]`.

`check -target all` vets the macOS, Windows, Linux, Android arm64 and web targets on any host. It also vets iOS, tvOS and watchOS when their Xcode SDKs are available, and lists the ones it skips. An explicitly named target, such as `check -target ios`, fails if its tools are unavailable, including when combined with `all`. A failed target does not prevent checking the remaining targets, and the command fails if any required check fails. `build -target all` still needs macOS with Xcode. In NSGo's own repository, each named target also vets its platform test hosts with `nsgotest`; CI names the preview `harmonyos` target explicitly, since it is outside `all`.

The Linux CI job runs the HarmonyOS Go backend's `js/wasm` tests with Node.js and Go's Wasm launcher. Run the gallery test locally with a HarmonyOS emulator connected through `hdc`. Compiling the [HarmonyOS host templates](building.md#harmonyos) requires OpenHarmony SDK API 26.

```sh
go tool nsgo test -env GOOS=js -env GOARCH=wasm ./internal/platform/harmonyos \
  -- "-exec=env -i \"PATH=$PATH\" \"$(go env GOROOT)/lib/wasm/go_js_wasm_exec\"" -count=1
go tool nsgo test -target harmonyos -run TestHarmonyOSPages -timeout 10m ./examples/gallery
```

## Releases

NSGo releases on demand when a downstream project needs a tagged version or maintainers have changes ready to ship. When requesting a release for a library, link the dependent work and name the commit that contains the last NSGo change it needs. The first release tag that contains that commit is the version the library can require.

To test a merged change before it is tagged, run `go get github.com/nsgo-dev/nsgo@main` from the dependent module. You can use a commit ID in place of `main` to select the revision you verified. Go records the resulting pseudo-version in `go.mod`.

A maintainer gives the changes their version and date in `CHANGELOG.md`, leaves a fresh Unreleased section, and tags the resulting commit on `main`. The [release workflow](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/release.yml) checks the tag and publishes it after the test workflow passes.

## NSH

NSGo reads [Native Shell](https://github.com/longbridge/nsh) as a design reference; none of its tests need it. To read it, clone it beside the repository and check out its latest release tag, never `main`:

```sh
git clone https://github.com/longbridge/nsh.git ../nsh
git -C ../nsh checkout "$(git -C ../nsh describe --tags --abbrev=0)"
```

## Next

- [The codebase](contributing/codebase.md): where each part of NSGo lives.
- [Making a change](contributing/changes.md): what a pull request needs.
