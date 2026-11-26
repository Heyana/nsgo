---
title: Native libraries
description: Ship prebuilt shared libraries and Apple frameworks with a Go package through NSGo's normal packaging, license and signing steps.
group: Toolchain
order: 23
---

A Go package can ship the shared libraries it loads through purego by placing `nsgo-natives.yaml` in its package directory, and from version 2 its frameworks for iOS and tvOS. When an app imports the package, NSGo selects its declarations for the target, verifies the files and includes them before signing and making installers. The app needs no extra configuration or packaging command.

Desktop app builds and target tests disable cgo for app code. Native code must use purego and a declared `nsgo-natives.yaml` library. NSGo rejects app packages that contain cgo files before the build starts.

## Declare libraries

```yaml
version: 2
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
  ios:
    - name: Example.framework
      url: https://downloads.example.org/v1.0/ios/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  ios_simulator:
    - name: Example.framework
      path: lib/ios-simulator/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  tvos: []
  tvos_simulator: []
```

Version 2 takes the keys `macos_arm64`, `macos_x64`, `windows_arm64`, `windows_x64`, `linux_arm64`, `linux_x64`, `ios`, `ios_simulator`, `tvos` and `tvos_simulator`. Version 1 takes the six desktop keys alone; a file is read only for its version's keys, so iOS and tvOS builds package nothing from a version 1 file. Declare every target on which the Go package is selected; `[]` explicitly says that target needs no bundled library. Go build tags can instead keep the package out of targets it does not support. `macos_universal` takes the two macOS entries, and a terminal target its desktop's: `tui_macos_arm64` takes `macos_arm64`.

Each library has:

- `name`: its installed file name, ending in `.dylib` on macOS, `.dll` on Windows, or `.so` or a versioned `.so.*` on Linux. On iOS and tvOS it is a framework's, `<Name>.framework`, where `Name` is ASCII letters, digits, `.` and `-`, beginning and ending with a letter or digit.
- Exactly one of `path`, a file relative to this package directory, or `url`, an HTTP(S) URL for the library itself. Version 2 takes only https URLs, and a download fails when a redirect leaves https. Publish extracted libraries rather than archive URLs, and for a framework its binary alone. URLs omit embedded usernames and passwords.
- `sha256`: the SHA-256 of the original file, required for either source. For example, `shasum -a 256 lib/macos-arm64/libexample.dylib` prints it on macOS.
- `licenses`: nonempty paths to the original license and notice files, relative to this package directory. These texts ship in `THIRD_PARTY_NOTICES.txt` and reach `core.ThirdPartyNotices()` in release builds. The app's existing [license policy](building.md#license-notices), including exact module exceptions, applies independently to native licenses.

Local paths and symbolic links stay inside the declaring package. NSGo rejects unknown fields, checksum mismatches, files for another operating system or architecture, and conflicting installed names. Two packages can share an identically named file with the same checksum. A file is limited to 1 GiB.

## Package and load

| Target | Library directory |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows, Linux and terminal targets | Beside the executable |
| iOS and tvOS | `<app>.app/Frameworks/<Name>.framework/<Name>` |

The package loads its library from that directory, using the executable's location rather than the working directory. Prebuild dependencies so their peers load from the directory they ship in. Declare each required peer file and each file name the loader needs.

NSGo checks cached downloads again before reuse and coordinates concurrent builds through a file lock. The content cache lives under the operating system's user-cache directory, in `nsgo/natives`. Local files are verified as they are copied.

A universal macOS package requires the same library names for both architectures. NSGo joins the slices with `lipo`; a publisher's already-universal library can serve either entry. Libraries are signed before the app, then pass through the app's normal disk-image, signing and notarization steps. Windows signing includes declared DLLs even with `build.installers: false`. Linux installers include the same files and notices as the executable directory. Terminal targets sign macOS libraries ad hoc.

Development, `-run` and target tests use the same package selection and copy steps, on desktops, iOS and tvOS. Declarations are selected with the build's target, tags and generated Go overlay, including local replacements and vendored packages. Android, HarmonyOS, watchOS, the web and mini programs package nothing from this file.

## iOS and tvOS frameworks

NSGo checks each framework's binary and stops the build, naming the reason, unless it is:

- an arm64 Mach-O dynamic library built for the target's platform, devices or the simulator, as its `LC_BUILD_VERSION` says; a universal file gives its arm64 slice;
- for a minimum OS no newer than the app's, `apple.info.MinimumOSVersion` or else iOS 15.0 and tvOS 17.0 ([Building](building.md#nsgoyaml));
- named `@rpath/<Name>.framework/<Name>` by its install name;
- loading through `@rpath` only frameworks that the app's packages declare for the same target.

NSGo writes the framework's `Info.plist`, whose `CFBundleIdentifier` is the app's ID, a dot and `Name` in lower case, with the platform and the app's minimum OS. Simulator builds sign the framework ad hoc. A device build leaves it to the app's signing: `sign.profile` signs it with the app's identity before the app, as Xcode does for `nsgo build -run` and `nsgo dev` on a device. Give a framework that loads its peers an `@loader_path/..` run path, so that it finds them in `Frameworks`.

`nsgo build -embed` stops when the app links a package that declares iOS frameworks, naming the package: the XCFramework for a host app cannot carry them yet.

## Preserve files when vendoring

Go's vendor operation needs embedded data to retain files below a package directory. A package can mark its manifest and licenses for inclusion without using their bytes at runtime:

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

Include local library paths in that embed directive too when using `path`, for example `lib/*`. URL downloads need only the manifest and license files in the Go package.
