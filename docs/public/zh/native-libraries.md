---
title: 原生库
description: 通过 NSGo 常规的打包、许可证与签名流程，随 Go 包一同分发预编译的共享库和 Apple 框架，并声明它们所需的 Linux 软件包。
group: 工具链
order: 23
---

Go 包只需在自身包目录中放置 `nsgo-natives.yaml`，即可随包分发其通过 purego 加载的共享库；从版本 2 起，还可以分发 iOS 和 tvOS 的框架，并声明所需的 Linux 软件包。当应用导入该包时，NSGo 会为目标平台筛选对应的声明，校验这些文件，并在签名和制作安装程序之前将其打包进去。应用本身无需任何额外配置或打包命令。

桌面应用构建和目标测试对应用代码禁用 cgo。原生代码必须使用 purego，并声明 `nsgo-natives.yaml` 库。NSGo 会在构建开始前拒绝含有 cgo 文件的应用包。

## 声明库

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

版本 2 支持的键包括 `macos_arm64`、`macos_x64`、`windows_arm64`、`windows_x64`、`linux_arm64`、`linux_x64`、`ios`、`ios_simulator`、`tvos` 和 `tvos_simulator`。版本 1 只支持其中六个桌面键；文件只按其版本的键读取，因此 iOS 和 tvOS 构建不会从版本 1 的文件打包任何内容。请声明该 Go 包会被选中的每一个目标平台；`[]` 明确表示该目标不需要捆绑库。此外，也可以使用 Go 构建标签将该包排除在它不支持的目标平台之外。`macos_universal` 会使用两个 macOS 条目，终端目标则使用其桌面平台的条目：`tui_macos_arm64` 使用 `macos_arm64`。

每个库包含以下字段：

- `name`：安装后的文件名，在 macOS 上以 `.dylib` 结尾，在 Windows 上以 `.dll` 结尾，在 Linux 上以 `.so` 或带版本号的 `.so.*` 结尾。在 iOS 和 tvOS 上则是框架名 `<Name>.framework`，其中 `Name` 由 ASCII 字母、数字、`.` 和 `-` 组成，且以字母或数字开头和结尾。
- 必须且仅能指定 `path` 或 `url` 中的一个：`path` 为相对于当前包目录的文件路径，`url` 为指向库文件本身的 HTTP(S) URL。版本 2 只接受 https URL，重定向离开 https 时下载失败。请直接发布解压后的库文件，而不要提供归档压缩包的 URL；框架只发布其二进制文件。URL 中不得嵌入用户名和密码。
- `sha256`：原始文件的 SHA-256 校验和，无论使用哪种来源都必须填写。例如，在 macOS 上可以通过 `shasum -a 256 lib/macos-arm64/libexample.dylib` 打印该值。
- `licenses`：非空的路径列表，指向相对于当前包目录的原始许可证和声明文件。这些文本会打包进 `THIRD_PARTY_NOTICES.txt`，并在发布构建中通过 `core.ThirdPartyNotices()` 提供。应用现有的[许可证策略](building.md#许可证声明)（包括精确模块例外规则）同样独立适用于原生库许可证。

本地路径和符号链接必须保持在声明该库的包目录内部。NSGo 会拒绝未知字段、校验和不匹配的文件、属于其他操作系统或架构的文件，以及存在冲突的安装文件名。两个包可以共用具有相同校验和的同名文件。单个文件大小上限为 1 GiB。

## 打包与加载

| 目标 | 库目录 |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows、Linux 和终端目标 | 与可执行文件同级 |
| iOS 和 tvOS | `<app>.app/Frameworks/<Name>.framework/<Name>` |

包会基于可执行文件的位置（而非当前工作目录）从该目录加载库文件。请提前构建好依赖项，使其同级依赖库能够从其所在分发目录加载。请声明每一个所需的同级文件以及加载器需要的每一个文件名。

NSGo 在复用已缓存的下载文件之前会再次进行校验，并通过文件锁协调并发构建。内容缓存位于操作系统用户缓存目录下的 `nsgo/natives` 中。本地文件会在复制过程中完成校验。

macOS 通用包（universal package）要求两种架构使用相同的库文件名。NSGo 会使用 `lipo` 合并各个架构切片；发布者预先制作好的通用库可直接用于任一条目。库文件会在应用之前完成签名，随后走完应用常规的磁盘映像、签名与公证流程。即使设置了 `build.installers: false`，Windows 签名仍会包含已声明的 DLL。Linux 安装程序会包含与可执行文件目录相同的库文件和声明文件。在 Mac 上，终端目标会对 macOS 库进行临时（ad hoc）签名；在其他主机上则按发布时的原样打包（通用库整体保留），并拒绝尚未签名的 arm64 库。

开发模式、`-run` 和目标测试使用相同的包筛选与复制步骤，桌面平台以及 iOS 和 tvOS 均是如此。声明会根据构建的目标平台、构建标签和生成的 Go overlay（包括本地替换模块和 vendor 包）进行筛选。Android、鸿蒙、watchOS、Web 和小程序不会从此文件打包任何内容。

## iOS 和 tvOS 框架

NSGo 会检查每个框架的二进制文件，不满足以下条件时停止构建并说明原因：

- 是为目标平台（真机或模拟器）构建的 arm64 Mach-O 动态库，以其 `LC_BUILD_VERSION` 为准；通用文件取其 arm64 切片；arm64e 会被拒绝；
- 最低系统版本不高于应用的最低版本，即 `apple.info.MinimumOSVersion`，未设置时为 iOS 15.0 和 tvOS 17.0（[构建](building.md#nsgoyaml)）；
- 安装名为 `@rpath/<Name>.framework/<Name>`；
- 通过 `@rpath` 加载的框架，都是应用的包为同一目标声明的框架。

NSGo 会为框架写入 `Info.plist`，其 `CFBundleIdentifier` 为应用 ID 加一个点再加小写的 `Name`，并带有平台和应用的最低系统版本。模拟器构建会对框架进行临时签名。真机构建将框架留给应用的签名流程：`sign.profile` 会在签名应用之前用应用的身份为其签名，Xcode 在真机上执行 `nsgo build -run` 和 `nsgo dev` 时也是如此。应用的可执行文件没有运行路径，因此会加载同级框架的框架需要带有 `@loader_path/..` 或 `@executable_path/Frameworks` 运行路径，才能在 `Frameworks` 中找到它们；除非带有 `/usr/lib/swift` 这类系统运行路径，否则不能通过 `@rpath` 加载其他内容。若框架的 bundle ID 与应用中另一个 bundle 相同（不区分大小写），例如与 NSGo 的 `NSGoStoreKit.framework` 并存的 `StoreKit.framework`，构建会停止。

当应用链接了声明 iOS 框架的包时，`nsgo build -embed` 会停止并指出该包：供宿主应用使用的 XCFramework 目前还无法携带这些框架。

## Linux 软件包

从版本 2 起，`linux` 按 Linux 架构列出系统软件包：链接该 Go 包的应用，其 Debian 和 Arch 软件包会依赖或推荐这些软件包。

```yaml
version: 2
libraries:
  # ...
linux:
  linux_x64:
    depends:
      deb: [libva2, libva-drm2, libpulse0]
      arch: [libva, libpulse]
    recommends:
      deb: [va-driver-all]
      arch: ["intel-media-driver: hardware decoding on Intel GPUs"]
  linux_arm64:
    depends:
      deb: [libavcodec61, libavformat61, libpulse0]
      arch: [ffmpeg, libpulse]
```

- `linux` 为可选项。其键为 `linux_x64` 和 `linux_arm64`，因此在一种架构上捆绑库、在另一种架构上加载系统库的包，可以分别列出各自所需的软件包。
- `depends` 和 `recommends` 各自接受 `deb` 和 `arch` 列表，与应用在 [nsgo.yaml](building.md#nsgoyaml) 中的 `linux.depends` 和 `linux.recommends` 相同：每个条目按 dpkg 或 pacman 的写法填写，例如 `libsoup-3.0-0 | libsoup3`、`ffmpeg (>= 7)` 或 `libpulse: audio output`。
- `.deb` 的 `Depends` 和 `Recommends` 字段以及 Arch 软件包的 `depend` 和 `optdepend` 行，依次列出 NSGo 自身的软件包、应用的软件包，以及构建所链接的每个 Go 包的条目，后者按 Go 列出包的顺序排列，已列出的条目不再重复。被目标平台或构建标签排除的 Go 包不会添加任何内容。
- 条目为空或含有换行符时，构建会停止并指出该 Go 包；版本 1 文件中出现 `linux`，或使用这两个以外的键，同样如此。
- AppImage 和商店构建的 Flatpak 不使用 `linux` 中的任何内容。

## 在 vendor 时保留文件

Go 的 vendor 操作需要通过嵌入数据来保留包目录下的文件。包可以将其清单文件和许可证文件标记为包含，而无需在运行时读取它们的字节内容：

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

使用 `path` 时，也请将本地库路径包含到该嵌入指令中，例如 `lib/*`。使用 URL 下载时，Go 包中只需包含清单文件和许可证文件。
