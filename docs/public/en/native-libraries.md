---
title: Native libraries
description: Ship prebuilt shared libraries with a Go package through NSGo's normal packaging, license and signing steps.
group: Toolchain
order: 23
---

A Go package can ship the shared libraries it loads through purego by placing `nsgo-natives.yaml` in its package directory. When an app imports the package, NSGo selects its declarations for the target, verifies the files and includes them before signing and making installers. The app needs no extra configuration or packaging command.

## Declare libraries

```yaml
version: 1
libraries:
  macos_arm64:
    - name: libexample.dylib
      path: lib/macos-arm64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  macos_x64:
    - name: libexample.dylib
      url: https://downloads.example.org/v1.0/macos-x64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  windows_arm64: []
  windows_x64: []
  linux_arm64: []
  linux_x64: []
```

The supported keys are `macos_arm64`, `macos_x64`, `windows_arm64`, `windows_x64`, `linux_arm64` and `linux_x64`. Declare every target on which the Go package is selected; `[]` explicitly says that target needs no bundled library. Go build tags can instead keep the package out of targets it does not support. `macos_universal` takes the two macOS entries.

Each library has:

- `name`: its installed file name, ending in `.dylib` on macOS, `.dll` on Windows, or `.so` or a versioned `.so.*` on Linux.
- Exactly one of `path`, a file relative to this package directory, or `url`, an HTTP(S) URL for the library itself. Publish extracted libraries rather than archive URLs. URLs omit embedded usernames and passwords.
- `sha256`: the SHA-256 of the original file, required for either source. For example, `shasum -a 256 lib/macos-arm64/libexample.dylib` prints it on macOS.
- `licenses`: nonempty paths to the original license and notice files, relative to this package directory. These texts ship in `THIRD_PARTY_NOTICES.txt` and reach `core.ThirdPartyNotices()` in release builds. The app's existing [license policy](building.md#license-notices), including exact module exceptions, applies independently to native licenses.

Local paths and symbolic links stay inside the declaring package. NSGo rejects unknown fields, checksum mismatches, files for another operating system or architecture, and conflicting installed names. Two packages can share an identically named file with the same checksum. A file is limited to 1 GiB.

## Package and load

| Target | Library directory |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows and Linux | Beside the executable |

The package loads its library from that directory, using the executable's location rather than the working directory. Prebuild dependencies so their peers load from the directory they ship in. Declare each required peer file and each file name the loader needs.

NSGo checks cached downloads again before reuse and coordinates concurrent builds through a file lock. The content cache lives under the operating system's user-cache directory, in `nsgo/natives`. Local files are verified as they are copied.

A universal macOS package requires the same library names for both architectures. NSGo joins the slices with `lipo`; a publisher's already-universal library can serve either entry. Libraries are signed before the app, then pass through the app's normal disk-image, signing and notarization steps. Windows signing includes declared DLLs even with `build.installers: false`. Linux installers include the same files and notices as the executable directory.

Development and native desktop target tests use the same package selection and copy steps. Declarations are selected with the build's target, tags and generated Go overlay, including local replacements and vendored packages. Other targets currently package no libraries from this file.

## Preserve files when vendoring

Go's vendor operation needs embedded data to retain files below a package directory. A package can mark its manifest and licenses for inclusion without using their bytes at runtime:

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

Include local library paths in that embed directive too when using `path`, for example `lib/*`. URL downloads need only the manifest and license files in the Go package.
